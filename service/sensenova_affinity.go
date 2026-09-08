package service

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// An explicit client hint, never an authentication/authorization input. The
// normal API token middleware supplies id/token_id; web login sessions and
// customer prompts are deliberately not reused as conversation identities.
const senseNovaConversationHeader = "X-AK47-Conversation-ID"

func senseNovaConversationIdentity(c *gin.Context, channel int, name string) (scope, session string) {
	if c == nil || c.Request == nil || c.GetInt("id") <= 0 || c.GetInt("token_id") <= 0 || channel <= 0 || name == "" {
		return "", ""
	}
	values := c.Request.Header.Values(senseNovaConversationHeader)
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 128 {
		return "", ""
	}
	for _, ch := range values[0] {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
			return "", ""
		}
	}
	scope = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%s", c.GetInt("id"), c.GetInt("token_id"), channel, name))))
	session = fmt.Sprintf("%x", sha256.Sum256([]byte(scope+":"+values[0])))
	return scope, session
}

// Bounded to 128 recent conversations per authenticated user/token/channel/model
// scope, expiring after 15 minutes. Redis contains hashes and fingerprints only.
var senseNovaAffinityScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local expired = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now)
for _, member in ipairs(expired) do redis.call('ZREM', KEYS[1], member); redis.call('HDEL', KEYS[2], member) end
if ARGV[2] ~= '' then
  redis.call('ZADD', KEYS[1], now + 900000, ARGV[1])
  redis.call('HSET', KEYS[2], ARGV[1], ARGV[2])
  local overflow = redis.call('ZCARD', KEYS[1]) - 128
  if overflow > 0 then
    local oldest = redis.call('ZRANGE', KEYS[1], 0, overflow - 1)
    for _, member in ipairs(oldest) do redis.call('ZREM', KEYS[1], member); redis.call('HDEL', KEYS[2], member) end
  end
  redis.call('PEXPIRE', KEYS[1], 900000); redis.call('PEXPIRE', KEYS[2], 900000)
end
return redis.call('HGET', KEYS[2], ARGV[1]) or ''
`)

func senseNovaConversationPreference(ctx context.Context, scope, session, fingerprint string) (string, error) {
	if scope == "" || session == "" {
		return "", nil
	}
	if common.RDB == nil {
		return "", errSenseNovaBudgetUnavailable
	}
	base := "sensenova:affinity:{" + scope + "}:"
	return senseNovaAffinityScript.Run(ctx, common.RDB, []string{base + "expiry", base + "keys"}, session, fingerprint).Text()
}
