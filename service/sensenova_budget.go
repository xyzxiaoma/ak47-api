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
	TokensPerMinute int64
	OutputAllowance int64
	Interval        time.Duration
	Window          time.Duration
	Lease           time.Duration
}

type senseNovaBudgetRequest struct {
	ChannelID                  int
	Fingerprint, Model, ID     string
	PromptTokens, OutputTokens int64
	HasOutputLimit             bool
}

type senseNovaBudgetReservation struct {
	keys     []string
	id       string
	owner    string
	lease    time.Duration
	window   time.Duration
	estimate int64
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
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now)
for _, member in ipairs(expired) do
  redis.call('ZREM', KEYS[1], member)
  redis.call('HDEL', KEYS[2], member)
  redis.call('HDEL', KEYS[3], member)
  redis.call('HDEL', KEYS[4], member)
end
local active = redis.call('GET', KEYS[5])
if active and string.sub(active, 1, string.len(id) + 1) == id .. ':' then
  if ARGV[8] == '0' then return {0, 0, '', 0} end
  return {1, 0, active, 0}
end
local wait, fixed = 0, 0
if active then wait = math.max(1, redis.call('PTTL', KEYS[5])) end
if cap == 0 and redis.call('EXISTS', KEYS[6]) == 1 then
  fixed = math.max(1, redis.call('PTTL', KEYS[6]))
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
if wait > 0 then return {0, wait, '', fixed} end
-- Pending health checks need the same bounds without temporarily owning or
-- consuming capacity. Expired-entry cleanup above is safe for both modes.
if ARGV[8] == '0' then return {0, 0, '', 0} end
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
if cap == 0 then redis.call('SET', KEYS[6], candidate, 'PX', interval) end
return {1, 0, candidate, 0}
`)

var senseNovaBudgetFinishScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local id, owner = ARGV[1], ARGV[2]
local actual, unused = tonumber(ARGV[3]), ARGV[4] == '1'
local expiry = tonumber(redis.call('ZSCORE', KEYS[1], id) or '0')
local matches = redis.call('HGET', KEYS[3], id) == owner
local finished = redis.call('HEXISTS', KEYS[4], id) == 1
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
  redis.call('DEL', KEYS[6])
end
return 1
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
	keys := []string{base + "expiry", base + "debits", base + "owners", base + "finished", base + "lease", base + "pace"}
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
		senseNovaBudgetMilliseconds(policy.Window), senseNovaBudgetMilliseconds(policy.Interval), senseNovaBudgetMilliseconds(policy.Lease), reserveValue).Slice()
	if err != nil {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	if len(result) != 4 {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	status, statusOK := result[0].(int64)
	wait, waitOK := result[1].(int64)
	storedOwner, ownerOK := result[2].(string)
	fixed, fixedOK := result[3].(int64)
	if !statusOK || !waitOK || !ownerOK || !fixedOK {
		return nil, 0, 0, errSenseNovaBudgetUnavailable
	}
	if status == 0 {
		return nil, time.Duration(wait) * time.Millisecond, time.Duration(fixed) * time.Millisecond, nil
	}
	return &senseNovaBudgetReservation{keys: keys, id: id, owner: storedOwner, lease: policy.Lease, window: policy.Window, estimate: estimate}, 0, 0, nil
}

func senseNovaBudgetBaseKey(request senseNovaBudgetRequest) string {
	// Length prefixes prevent delimiter collisions; only non-secret identifiers
	// enter this hash, and Redis stores no credentials, model names or request IDs.
	scope := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s", request.ChannelID, len(request.Fingerprint), request.Fingerprint, request.Model)))
	return fmt.Sprintf("sensenova:budget:{%x}:", scope)
}

func finishSenseNovaBudget(ctx context.Context, reservation *senseNovaBudgetReservation, actualTokens int64, unused bool) error {
	if reservation == nil {
		return nil
	}
	if common.RDB == nil {
		return errSenseNovaBudgetUnavailable
	}
	if actualTokens < -1 {
		return errors.New("invalid sensenova actual token usage")
	}
	if actualTokens > senseNovaBudgetMaxExactTokens {
		actualTokens = senseNovaBudgetMaxExactTokens
	}
	unusedValue := 0
	if unused {
		unusedValue = 1
	}
	if _, err := senseNovaBudgetFinishScript.Run(ctx, common.RDB, reservation.keys, reservation.id, reservation.owner, actualTokens, unusedValue,
		senseNovaBudgetMilliseconds(reservation.window), reservation.estimate).Result(); err != nil {
		return errSenseNovaBudgetUnavailable
	}
	return nil
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
