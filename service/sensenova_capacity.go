package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

type senseNovaCapacityObservation struct {
	Verified   bool
	Failures   int64
	RetryAfter time.Duration
}

// Size classes describe observed workloads, never provider quotas. Unsigned
// buckets allow the upper bucket (2^63) to represent every valid int64 input.
// v2 ignores pre-completion-validation success and misclassified 429001 TPM
// evidence. Old hashes expire naturally; budget, health and recovery ownership
// retain their existing identities throughout a rolling deployment.
func senseNovaCapacityShape(request senseNovaBudgetRequest) string {
	input := uint64(8192)
	for request.PromptTokens > 0 && input < uint64(request.PromptTokens) {
		input *= 2
	}
	output := "absent"
	if request.HasOutputLimit {
		if request.OutputTokens == 0 {
			output = "0"
		} else {
			bucket := uint64(1024)
			for request.OutputTokens > 0 && bucket < uint64(request.OutputTokens) {
				bucket *= 2
			}
			output = fmt.Sprint(bucket)
		}
	}
	return fmt.Sprintf("v2:i%d:o%s", input, output)
}

const senseNovaCapacityReadLua = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local expires = tonumber(redis.call('HGET', KEYS[1], 'expires') or '0')
if expires <= now then return {0, 0, 0} end
local verified = tonumber(redis.call('HGET', KEYS[1], 'verified') or '0')
local failures = tonumber(redis.call('HGET', KEYS[1], 'failures') or '0')
local retry = tonumber(redis.call('HGET', KEYS[1], 'retry') or '0')
return {verified, failures, math.max(0, math.min(86400000, retry - now))}
`

var senseNovaCapacityRecordScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
-- An uncertain network result can cause the same command to be retried.
-- The first accepted outcome is final for this owner, including its expiry.
if redis.call('HGET', KEYS[2], 'owner') == ARGV[1] then return 1 end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local verified, failures, penalty = 0, 0, 0
if ARGV[2] == '1' then
  verified = 1
else
  local expires = tonumber(redis.call('HGET', KEYS[2], 'expires') or '0')
  local previousRetry = 0
  if expires > now then
    failures = tonumber(redis.call('HGET', KEYS[2], 'failures') or '0')
    previousRetry = tonumber(redis.call('HGET', KEYS[2], 'retry') or '0') - now
  end
  failures = math.min(9007199254740991, failures + 1)
  if failures == 1 then penalty = 60000
  elseif failures == 2 then penalty = 120000
  elseif failures == 3 then penalty = 240000
  else penalty = 300000 end
  penalty = math.max(penalty, tonumber(ARGV[3]), math.min(86400000, previousRetry))
end
local ttl = math.max(900000, penalty)
redis.call('HSET', KEYS[2], 'owner', ARGV[1], 'verified', verified, 'failures', failures, 'retry', now + penalty, 'expires', now + ttl)
redis.call('PEXPIRE', KEYS[2], ttl)
return 1
`)

func readSenseNovaCapacity(ctx context.Context, requests []senseNovaBudgetRequest) ([]senseNovaCapacityObservation, error) {
	if common.RDB == nil {
		return nil, errSenseNovaBudgetUnavailable
	}
	observations := make([]senseNovaCapacityObservation, len(requests))
	if len(requests) == 0 {
		return observations, nil
	}
	// Each command addresses one slot; the pipeline avoids a network round-trip
	// per candidate without making a cross-slot script or refreshing evidence.
	commands := make([]*redis.Cmd, len(requests))
	_, err := common.RDB.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for i, request := range requests {
			if request.ChannelID <= 0 || request.Fingerprint == "" || request.Model == "" || request.PromptTokens < 0 || request.OutputTokens < 0 {
				return errSenseNovaBudgetUnavailable
			}
			key := senseNovaBudgetBaseKey(request) + "capacity:" + senseNovaCapacityShape(request)
			commands[i] = pipe.Eval(ctx, senseNovaCapacityReadLua, []string{key})
		}
		return nil
	})
	if err != nil {
		return nil, errSenseNovaBudgetUnavailable
	}
	for i, command := range commands {
		result, err := command.Slice()
		if err != nil || len(result) != 3 {
			return nil, errSenseNovaBudgetUnavailable
		}
		verified, verifiedOK := result[0].(int64)
		failures, failuresOK := result[1].(int64)
		retry, retryOK := result[2].(int64)
		if !verifiedOK || !failuresOK || !retryOK || (verified != 0 && verified != 1) || failures < 0 || retry < 0 || retry > 86400000 {
			return nil, errSenseNovaBudgetUnavailable
		}
		observations[i] = senseNovaCapacityObservation{Verified: verified == 1, Failures: failures, RetryAfter: time.Duration(retry) * time.Millisecond}
	}
	return observations, nil
}

func recordSenseNovaCapacity(ctx context.Context, reservation *senseNovaBudgetReservation, request senseNovaBudgetRequest, success bool, tpm bool, retryAfter int64) error {
	// Other outcomes make no statement about token capacity.
	if !success && !tpm {
		return nil
	}
	if common.RDB == nil {
		return errSenseNovaBudgetUnavailable
	}
	if reservation == nil || len(reservation.keys) < 5 || reservation.owner == "" ||
		request.ChannelID <= 0 || request.Fingerprint == "" || request.Model == "" || request.PromptTokens < 0 || request.OutputTokens < 0 {
		return errSenseNovaBudgetLeaseLost
	}
	base := senseNovaBudgetBaseKey(request)
	if reservation.keys[4] != base+"lease" {
		return errSenseNovaBudgetLeaseLost
	}
	if retryAfter < 0 {
		retryAfter = 0
	}
	if retryAfter > 86400 {
		retryAfter = 86400
	}
	verified := 0
	if success {
		verified = 1
	}
	// Both keys share the budget hash tag. Owner validation and evidence writes
	// are indivisible, so expired work cannot overwrite a newer admission.
	result, err := senseNovaCapacityRecordScript.Run(ctx, common.RDB,
		[]string{reservation.keys[4], base + "capacity:" + senseNovaCapacityShape(request)},
		reservation.owner, verified, retryAfter*1000).Int()
	if err != nil {
		return errSenseNovaBudgetUnavailable
	}
	if result != 1 {
		return errSenseNovaBudgetLeaseLost
	}
	return nil
}

var senseNovaCapacityRecoveryClaimScript = redis.NewScript(`
local active = redis.call('GET', KEYS[1])
if active == ARGV[1] then return {1, 0} end
if active then return {0, math.max(1, redis.call('PTTL', KEYS[1]))} end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return {1, 0}
`)

var senseNovaCapacityRecoveryReleaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then redis.call('DEL', KEYS[1]) end
return 1
`)

// The recovery lock spans every key and size class for this channel/model.
// Hash both the scope and attempt owner; no credentials or raw IDs are stored.
func senseNovaCapacityRecoveryIdentity(channelID int, name, owner string) (string, string, error) {
	if channelID <= 0 || name == "" || owner == "" {
		return "", "", errSenseNovaBudgetUnavailable
	}
	scope := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", channelID, name)))
	ownerHash := sha256.Sum256([]byte(owner))
	return fmt.Sprintf("sensenova:capacity-recovery:{%x}", scope), fmt.Sprintf("%x", ownerHash), nil
}

func claimSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string, lease time.Duration) (bool, time.Duration, error) {
	if common.RDB == nil || lease <= 0 {
		return false, 0, errSenseNovaBudgetUnavailable
	}
	key, ownerHash, err := senseNovaCapacityRecoveryIdentity(channelID, name, owner)
	if err != nil {
		return false, 0, err
	}
	result, err := senseNovaCapacityRecoveryClaimScript.Run(ctx, common.RDB, []string{key}, ownerHash, senseNovaBudgetMilliseconds(lease)).Slice()
	if err != nil || len(result) != 2 {
		return false, 0, errSenseNovaBudgetUnavailable
	}
	claimed, claimedOK := result[0].(int64)
	wait, waitOK := result[1].(int64)
	if !claimedOK || !waitOK || (claimed != 0 && claimed != 1) || wait < 0 || wait > int64((time.Duration(1<<63-1))/time.Millisecond) {
		return false, 0, errSenseNovaBudgetUnavailable
	}
	return claimed == 1, time.Duration(wait) * time.Millisecond, nil
}

func renewSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string, lease time.Duration) (bool, error) {
	if common.RDB == nil || lease <= 0 {
		return false, errSenseNovaBudgetUnavailable
	}
	key, ownerHash, err := senseNovaCapacityRecoveryIdentity(channelID, name, owner)
	if err != nil {
		return false, err
	}
	result, err := senseNovaBudgetRenewScript.Run(ctx, common.RDB, []string{key}, ownerHash, senseNovaBudgetMilliseconds(lease)).Int()
	if err != nil {
		return false, errSenseNovaBudgetUnavailable
	}
	return result == 1, nil
}

func releaseSenseNovaCapacityRecovery(ctx context.Context, channelID int, name, owner string) error {
	if common.RDB == nil {
		return errSenseNovaBudgetUnavailable
	}
	key, ownerHash, err := senseNovaCapacityRecoveryIdentity(channelID, name, owner)
	if err != nil {
		return err
	}
	if _, err := senseNovaCapacityRecoveryReleaseScript.Run(ctx, common.RDB, []string{key}, ownerHash).Result(); err != nil {
		return errSenseNovaBudgetUnavailable
	}
	return nil
}
