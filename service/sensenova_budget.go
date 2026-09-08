package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

var (
	errSenseNovaBudgetTooLarge    = errors.New("sensenova request exceeds configured token budget")
	errSenseNovaBudgetUnavailable = errors.New("sensenova admission state unavailable")
	errSenseNovaBudgetLeaseLost   = errors.New("sensenova admission lease lost")
)

// Redis Lua numbers are doubles. Bound arithmetic to exact integers; larger
// actual usage is conservatively saturated, never wrapped into a credit.
const senseNovaBudgetMaxExactTokens int64 = 1<<53 - 1

type senseNovaBudgetPolicy struct {
	TokensPerMinute  int64
	OutputAllowance  int64
	Interval         time.Duration
	Window           time.Duration
	Lease            time.Duration
	Followups        int
	FollowupInterval time.Duration
}

type senseNovaBudgetRequest struct {
	ChannelID                  int
	Fingerprint, Model, ID     string
	PromptTokens, OutputTokens int64
	HasOutputLimit             bool
	Conversation               string
}

type senseNovaBudgetReservation struct {
	keys         []string
	id           string
	owner        string
	lease        time.Duration
	window       time.Duration
	estimate     int64
	conversation string
	followups    int
	usedFollowup bool
}

// All scripts use Redis TIME, including expiry scores. Client clocks never
// determine shared admission. History is bounded to 4096 entries per scope,
// including zero-token requests, and each key has a TTL. Hash tags keep the
// keys for one channel/fingerprint/exact-model scope in a single Redis slot.
var senseNovaBudgetReserveScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local id, candidate = ARGV[1], ARGV[2]
local cap, estimate = tonumber(ARGV[3]), tonumber(ARGV[4])
local window, interval, lease = tonumber(ARGV[5]), tonumber(ARGV[6]), tonumber(ARGV[7])
local session, allowance, fast = ARGV[9], tonumber(ARGV[10]), tonumber(ARGV[11])
local followup = cap == 0 and allowance > 0 and session ~= '' and
  redis.call('HGET', KEYS[7], 'session') == session and
  tonumber(redis.call('HGET', KEYS[7], 'remaining') or '0') > 0
redis.call('ZREMRANGEBYSCORE', KEYS[8], '-inf', now - 60000)
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now)
for _, member in ipairs(expired) do
  redis.call('ZREM', KEYS[1], member)
  redis.call('HDEL', KEYS[2], member)
  redis.call('HDEL', KEYS[3], member)
  redis.call('HDEL', KEYS[4], member)
  redis.call('HDEL', KEYS[10], member)
  redis.call('HDEL', KEYS[10], member .. ':pace', member .. ':pace-expiry')
end
local active = redis.call('GET', KEYS[5])
if active and string.sub(active, 1, string.len(id) + 1) == id .. ':' then
  if ARGV[8] == '0' then return {0, 0, '', 0, 0} end
  return {1, 0, active, 0, redis.call('HGET', KEYS[10], id) == '1' and 1 or 0}
end
local wait, fixed = 0, 0
if active then wait = math.max(1, redis.call('PTTL', KEYS[5])) end
if cap == 0 and not followup and redis.call('EXISTS', KEYS[6]) == 1 then
  fixed = math.max(1, redis.call('PTTL', KEYS[6]))
  wait = math.max(wait, fixed)
end
if cap == 0 and allowance > 0 then
  local starts = redis.call('ZRANGE', KEYS[8], 0, -1, 'WITHSCORES')
  if #starts >= (allowance + 1) * 2 then
    local index = #starts - allowance * 2
    fixed = math.max(fixed, tonumber(starts[index]) + 60000 - now)
  end
  if redis.call('EXISTS', KEYS[9]) == 1 then fixed = math.max(fixed, redis.call('PTTL', KEYS[9])) end
  wait = math.max(wait, fixed)
end
local existing = redis.call('ZSCORE', KEYS[1], id)
if existing then
  wait = math.max(wait, tonumber(existing) - now)
  if redis.call('HEXISTS', KEYS[4], id) == 1 then fixed = math.max(fixed, tonumber(existing) - now) end
end
local entries = redis.call('ZRANGE', KEYS[1], 0, -1, 'WITHSCORES')
if #entries >= 8192 then
  if #entries > 0 then wait = math.max(wait, tonumber(entries[2]) - now) end
end
if cap > 0 then
  -- Walk newest to oldest: retain the newest suffix that fits alongside this
  -- request. The first older debit that does not fit determines the earliest
  -- sufficient expiry, rather than waking at an unrelated small/zero debit.
  -- Subtract from the remaining allowance instead of summing large usage.
  local remaining = cap - estimate
  for i = #entries - 1, 1, -2 do
    local debit = tonumber(redis.call('HGET', KEYS[2], entries[i]) or '0')
    if debit > remaining then
      wait = math.max(wait, tonumber(entries[i + 1]) - now)
      break
    end
    remaining = remaining - debit
  end
end
if active then
  -- Active/unfinished estimates can still reconcile or be released unused.
  -- Only completed debits establish an immutable lower bound while a lease
  -- is active; subtract safely and keep the sufficient rolling expiry.
  local remaining = cap - estimate
  local finishedCount, oldestFinished = 0, 0
  for i = #entries - 1, 1, -2 do
    if redis.call('HEXISTS', KEYS[4], entries[i]) == 1 then
      finishedCount = finishedCount + 1
      oldestFinished = tonumber(entries[i + 1])
      if cap > 0 and remaining then
        local debit = tonumber(redis.call('HGET', KEYS[2], entries[i]) or '0')
        if debit > remaining then
          fixed = math.max(fixed, oldestFinished - now)
          remaining = nil
        else
          remaining = remaining - debit
        end
      end
    end
  end
  if finishedCount >= 4096 then fixed = math.max(fixed, oldestFinished - now) end
else
  fixed = wait
end
if wait > 0 then return {0, wait, '', fixed, 0} end
-- Pending health checks need the same bounds without temporarily owning or
-- consuming capacity. Expired-entry cleanup above is safe for both modes.
if ARGV[8] == '0' then return {0, 0, '', 0, 0} end
local retention = window
if cap == 0 then retention = math.max(window, interval) end
redis.call('ZADD', KEYS[1], now + retention, id)
redis.call('HSET', KEYS[2], id, ARGV[4])
redis.call('HSET', KEYS[3], id, candidate)
redis.call('HDEL', KEYS[4], id)
for i = 1, 4 do
  if redis.call('EXISTS', KEYS[i]) == 1 then
    redis.call('PEXPIRE', KEYS[i], math.max(retention, redis.call('PTTL', KEYS[i])))
  end
end
redis.call('SET', KEYS[5], candidate, 'PX', lease)
if followup then
  -- Preserve the prior dispatched start's deadline for owner-checked rollback
  -- if billing/validation cancels this reservation before transport dispatch.
  local previous = redis.call('GET', KEYS[6]) or ''
  local remaining = math.max(0, redis.call('PTTL', KEYS[6]))
  redis.call('HSET', KEYS[10], id .. ':pace', previous, id .. ':pace-expiry', now + remaining)
end
if cap == 0 then redis.call('SET', KEYS[6], candidate, 'PX', interval) end
-- Every reserved start consumes local request demand; only explicitly unsent
-- work can refund it. This rolling policy is not a provider RPM assertion.
redis.call('ZADD', KEYS[8], now, id)
redis.call('PEXPIRE', KEYS[8], 60000)
redis.call('HSET', KEYS[10], id, followup and '1' or '0')
redis.call('PEXPIRE', KEYS[10], math.max(retention, lease))
if cap == 0 and allowance > 0 then redis.call('SET', KEYS[9], candidate, 'PX', fast) end
if followup then redis.call('HINCRBY', KEYS[7], 'remaining', -1) end
return {1, 0, candidate, 0, followup and 1 or 0}
`)

var senseNovaBudgetFinishScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local id, owner = ARGV[1], ARGV[2]
local actual, unused = tonumber(ARGV[3]), ARGV[4] == '1'
local expiry = tonumber(redis.call('ZSCORE', KEYS[1], id) or '0')
local matches = redis.call('HGET', KEYS[3], id) == owner
local finished = redis.call('HEXISTS', KEYS[4], id) == 1
local live = redis.call('GET', KEYS[5]) == owner
local verified = live and not finished and not unused and ARGV[7] == '1'
-- Completion grants require a live owner and explicit verified success.
-- Successful followups do not replenish or extend their allowance/window.
if live and not finished then
  if unused then
    redis.call('ZREM', KEYS[8], id)
    if ARGV[10] == '1' and redis.call('HGET', KEYS[7], 'session') == ARGV[8] then
      redis.call('HINCRBY', KEYS[7], 'remaining', 1)
    end
    if redis.call('GET', KEYS[9]) == owner then redis.call('DEL', KEYS[9]) end
  elseif ARGV[7] ~= '1' then
    redis.call('DEL', KEYS[7])
  elseif tonumber(ARGV[9]) > 0 and ARGV[8] ~= '' and ARGV[10] ~= '1' and redis.call('EXISTS', KEYS[7]) == 0 then
    redis.call('HSET', KEYS[7], 'session', ARGV[8], 'remaining', ARGV[9])
    redis.call('PEXPIRE', KEYS[7], 60000)
  end
end
-- A stream can outlive the rolling history while renewing its in-flight
-- lease. Only that live owner may recreate its expired debit. Account a late
-- success (or uncertain estimate) for a fresh window from completion; a stale
-- completion must never recreate history belonging to a newer admission.
if expiry <= now and not unused and redis.call('GET', KEYS[5]) == owner then
  local window = tonumber(ARGV[5])
  local debit = ARGV[3]
  if actual < 0 then debit = ARGV[6] end
  expiry = now + window
  redis.call('ZADD', KEYS[1], expiry, id)
  redis.call('HSET', KEYS[2], id, debit)
  redis.call('HSET', KEYS[3], id, owner)
  redis.call('HDEL', KEYS[4], id)
  for i = 1, 3 do
    redis.call('PEXPIRE', KEYS[i], math.max(window, redis.call('PTTL', KEYS[i])))
  end
  matches, finished = true, false
end
if matches and expiry > now and not finished then
  if unused then
    redis.call('ZREM', KEYS[1], id)
    redis.call('HDEL', KEYS[2], id)
    redis.call('HDEL', KEYS[3], id)
  else
    if actual >= 0 then redis.call('HSET', KEYS[2], id, ARGV[3]) end
    redis.call('HSET', KEYS[4], id, '1')
    redis.call('PEXPIRE', KEYS[4], math.max(1, redis.call('PTTL', KEYS[1])))
  end
end
if redis.call('GET', KEYS[5]) == owner then redis.call('DEL', KEYS[5]) end
-- A duplicate completion cannot relabel dispatched work as unused. Pacing may
-- outlive history during policy changes, so also require ownership of pace.
if unused and not finished and redis.call('GET', KEYS[6]) == owner then
  if ARGV[10] ~= '1' then
    redis.call('DEL', KEYS[6])
  elseif live then
    local previous = redis.call('HGET', KEYS[10], id .. ':pace') or ''
    local deadline = tonumber(redis.call('HGET', KEYS[10], id .. ':pace-expiry') or '0')
    if previous ~= '' and deadline > now then
      redis.call('SET', KEYS[6], previous, 'PX', deadline - now)
    else
      redis.call('DEL', KEYS[6])
    end
  end
end
-- Unused rows leave the expiry index, so later pruning cannot discover their
-- auxiliary fields. Clean after pace restoration, using the captured owner
-- match even when an unsent lease expired before its longer-lived ledger row.
if unused and not finished and (matches or live) then
  redis.call('HDEL', KEYS[10], id, id .. ':pace', id .. ':pace-expiry')
end
return verified and 1 or 0
`)

var senseNovaBudgetRenewScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return 1
`)

func reserveSenseNovaBudget(ctx context.Context, request senseNovaBudgetRequest, policy senseNovaBudgetPolicy) (*senseNovaBudgetReservation, time.Duration, error) {
	reservation, wait, _, err := reserveSenseNovaBudgetWithWait(ctx, request, policy)
	return reservation, wait, err
}

// fixedMinimum excludes waits that an active request may shorten by finishing
// or reconciling usage. Unknown start pacing remains fixed for dispatched work;
// an explicitly unused reservation can still release its pacing on cleanup.
func reserveSenseNovaBudgetWithWait(ctx context.Context, request senseNovaBudgetRequest, policy senseNovaBudgetPolicy) (*senseNovaBudgetReservation, time.Duration, time.Duration, error) {
	return senseNovaBudgetAdmission(ctx, request, policy, true)
}

// readSenseNovaBudgetWait shares validation and atomic wait calculations with
// admission, but never creates or extends a reservation, pace, or debit.
func readSenseNovaBudgetWait(ctx context.Context, request senseNovaBudgetRequest, policy senseNovaBudgetPolicy) (time.Duration, time.Duration, error) {
	_, wait, fixed, err := senseNovaBudgetAdmission(ctx, request, policy, false)
	return wait, fixed, err
}

func senseNovaBudgetAdmission(ctx context.Context, request senseNovaBudgetRequest, policy senseNovaBudgetPolicy, reserve bool) (*senseNovaBudgetReservation, time.Duration, time.Duration, error) {
	if common.RDB == nil {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	if request.ChannelID <= 0 || request.Fingerprint == "" || request.Model == "" || request.ID == "" ||
		request.PromptTokens < 0 || request.OutputTokens < 0 || policy.OutputAllowance < 0 ||
		policy.TokensPerMinute < 0 || policy.TokensPerMinute > senseNovaBudgetMaxExactTokens ||
		policy.Followups < 0 || policy.Followups > 2 || (policy.Followups > 0 && (policy.FollowupInterval < 5*time.Second || policy.FollowupInterval > time.Minute)) ||
		policy.Window <= 0 || policy.Lease <= 0 || (policy.TokensPerMinute == 0 && policy.Interval <= 0) {
		return nil, 0, 0, errors.New("invalid sensenova admission policy or request")
	}
	var estimate int64
	if policy.TokensPerMinute > 0 {
		output := policy.OutputAllowance
		if request.HasOutputLimit {
			output = request.OutputTokens
		}
		if request.PromptTokens > policy.TokensPerMinute || output > policy.TokensPerMinute-request.PromptTokens {
			return nil, 0, 0, errSenseNovaBudgetTooLarge
		}
		estimate = request.PromptTokens + output
	}
	base := senseNovaBudgetBaseKey(request)
	keys := []string{base + "expiry", base + "debits", base + "owners", base + "finished", base + "lease", base + "pace", base + "followup", base + "starts", base + "fastpace", base + "followup-owners"}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(request.ID)))
	owner, reserveValue := "", 0
	if reserve {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, 0, 0, errSenseNovaBudgetUnavailable
		}
		owner, reserveValue = fmt.Sprintf("%s:%x", id, nonce), 1
	}
	result, err := senseNovaBudgetReserveScript.Run(ctx, common.RDB, keys, id, owner, policy.TokensPerMinute, estimate,
		senseNovaBudgetMilliseconds(policy.Window), senseNovaBudgetMilliseconds(policy.Interval), senseNovaBudgetMilliseconds(policy.Lease), reserveValue,
		request.Conversation, policy.Followups, senseNovaBudgetMilliseconds(policy.FollowupInterval)).Slice()
	if err != nil {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	if len(result) != 5 {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	status, statusOK := result[0].(int64)
	wait, waitOK := result[1].(int64)
	storedOwner, ownerOK := result[2].(string)
	fixed, fixedOK := result[3].(int64)
	usedFollowup, followupOK := result[4].(int64)
	if !statusOK || !waitOK || !ownerOK || !fixedOK || !followupOK {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	if status == 0 {
		return nil, time.Duration(wait) * time.Millisecond, time.Duration(fixed) * time.Millisecond, nil
	}
	followups := policy.Followups
	if policy.TokensPerMinute > 0 {
		followups = 0
	}
	return &senseNovaBudgetReservation{keys: keys, id: id, owner: storedOwner, lease: policy.Lease, window: policy.Window, estimate: estimate,
		conversation: request.Conversation, followups: followups, usedFollowup: usedFollowup == 1}, 0, 0, nil
}

func senseNovaBudgetBaseKey(request senseNovaBudgetRequest) string {
	// Length prefixes prevent delimiter collisions; only non-secret identifiers
	// enter this hash, and Redis stores no credentials, model names or request IDs.
	scope := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s", request.ChannelID, len(request.Fingerprint), request.Fingerprint, request.Model)))
	return fmt.Sprintf("sensenova:budget:{%x}:", scope)
}

func finishSenseNovaBudget(ctx context.Context, reservation *senseNovaBudgetReservation, actualTokens int64, unused bool) error {
	return finishSenseNovaBudgetVerified(ctx, reservation, actualTokens, unused, false)
}

func finishSenseNovaBudgetVerified(ctx context.Context, reservation *senseNovaBudgetReservation, actualTokens int64, unused, verified bool) error {
	_, err := finishSenseNovaBudgetOutcome(ctx, reservation, actualTokens, unused, verified)
	return err
}

// The result authorizes publishing a conversation preference only after the
// shared ledger accepted this live owner's verified completion exactly once.
func finishSenseNovaBudgetOutcome(ctx context.Context, reservation *senseNovaBudgetReservation, actualTokens int64, unused, verified bool) (bool, error) {
	if reservation == nil {
		return false, nil
	}
	if common.RDB == nil {
		return false, errSenseNovaBudgetUnavailable
	}
	if actualTokens < -1 {
		return false, errors.New("invalid sensenova actual token usage")
	}
	if actualTokens > senseNovaBudgetMaxExactTokens {
		actualTokens = senseNovaBudgetMaxExactTokens
	}
	unusedValue := 0
	if unused {
		unusedValue = 1
	}
	verifiedValue, followupValue := 0, 0
	if verified {
		verifiedValue = 1
	}
	if reservation.usedFollowup {
		followupValue = 1
	}
	result, err := senseNovaBudgetFinishScript.Run(ctx, common.RDB, reservation.keys, reservation.id, reservation.owner, actualTokens, unusedValue,
		senseNovaBudgetMilliseconds(reservation.window), reservation.estimate, verifiedValue, reservation.conversation, reservation.followups, followupValue).Int()
	if err != nil {
		return false, errSenseNovaBudgetUnavailable
	}
	return result == 1, nil
}

func renewSenseNovaBudget(ctx context.Context, reservation *senseNovaBudgetReservation) error {
	if reservation == nil {
		return nil
	}
	if common.RDB == nil {
		return errSenseNovaBudgetUnavailable
	}
	result, err := senseNovaBudgetRenewScript.Run(ctx, common.RDB, []string{reservation.keys[4]}, reservation.owner, senseNovaBudgetMilliseconds(reservation.lease)).Int()
	if err != nil {
		return errSenseNovaBudgetUnavailable
	}
	if result != 1 {
		return errSenseNovaBudgetLeaseLost
	}
	return nil
}

// Round TTLs upward without overflowing time.Duration near its upper bound.
func senseNovaBudgetMilliseconds(duration time.Duration) int64 {
	millis := duration.Milliseconds()
	if duration%time.Millisecond > 0 {
		millis++
	}
	return millis
}

var senseNovaQueueAcquireScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
local id, limit, ttl = ARGV[1], tonumber(ARGV[2]), tonumber(ARGV[3])
if not redis.call('ZSCORE', KEYS[1], id) and redis.call('ZCARD', KEYS[1]) >= limit then return 0 end
local previous = tonumber(redis.call('ZSCORE', KEYS[1], id) or '0')
redis.call('ZADD', KEYS[1], math.max(previous, now + ttl), id)
local last = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
redis.call('PEXPIRE', KEYS[1], math.max(1, tonumber(last[2]) - now))
return 1
`)

func acquireSenseNovaQueue(ctx context.Context, channelID int, id string, limit int, ttl time.Duration) (bool, error) {
	if common.RDB == nil {
		return false, errSenseNovaBudgetUnavailable
	}
	if channelID <= 0 || id == "" || limit <= 0 || ttl <= 0 {
		return false, errors.New("invalid sensenova waiting lease")
	}
	key := fmt.Sprintf("sensenova:queue:{%d}", channelID)
	member := fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
	result, err := senseNovaQueueAcquireScript.Run(ctx, common.RDB, []string{key}, member, limit, senseNovaBudgetMilliseconds(ttl)).Int()
	if err != nil {
		return false, errSenseNovaBudgetUnavailable
	}
	return result == 1, nil
}

func releaseSenseNovaQueue(ctx context.Context, channelID int, id string) error {
	if common.RDB == nil {
		return errSenseNovaBudgetUnavailable
	}
	if channelID <= 0 || id == "" {
		return errors.New("invalid sensenova waiting lease")
	}
	key := fmt.Sprintf("sensenova:queue:{%d}", channelID)
	member := fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
	if err := common.RDB.ZRem(ctx, key, member).Err(); err != nil {
		return errSenseNovaBudgetUnavailable
	}
	return nil
}
