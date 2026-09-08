package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func affinityTestContext(user, token int, session string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("id", user)
	c.Set("token_id", token)
	c.Request.Header.Set(senseNovaConversationHeader, session)
	return c
}

func TestSenseNovaAffinityIdentityIsolation(t *testing.T) {
	baseline, session := senseNovaConversationIdentity(affinityTestContext(1, 2, "conversation-a"), 7, "deepseek-v4-pro")
	require.Len(t, baseline, 64)
	require.Len(t, session, 64)
	for _, tc := range []struct {
		user, token, channel int
		model, session       string
	}{
		{2, 2, 7, "deepseek-v4-pro", "conversation-a"}, {1, 3, 7, "deepseek-v4-pro", "conversation-a"},
		{1, 2, 8, "deepseek-v4-pro", "conversation-a"}, {1, 2, 7, "glm-5.2", "conversation-a"},
		{1, 2, 7, "deepseek-v4-pro", "conversation-b"},
	} {
		_, other := senseNovaConversationIdentity(affinityTestContext(tc.user, tc.token, tc.session), tc.channel, tc.model)
		require.NotEmpty(t, other)
		assert.NotEqual(t, session, other)
	}
	for _, c := range []*gin.Context{affinityTestContext(0, 2, "valid"), affinityTestContext(1, 0, "valid"), affinityTestContext(1, 2, ""), affinityTestContext(1, 2, "invalid session")} {
		scope, session := senseNovaConversationIdentity(c, 7, "deepseek-v4-pro")
		assert.Empty(t, scope)
		assert.Empty(t, session)
	}
	duplicate := affinityTestContext(1, 2, "one")
	duplicate.Request.Header.Add(senseNovaConversationHeader, "two")
	_, session = senseNovaConversationIdentity(duplicate, 7, "deepseek-v4-pro")
	assert.Empty(t, session)
}

func TestSenseNovaAffinityBoundedExpiryAndPrivacy(t *testing.T) {
	server, advance := setupSenseNovaBudgetRedis(t)
	ctx := context.Background()
	for i := 0; i < 130; i++ {
		scope, session := senseNovaConversationIdentity(affinityTestContext(1, 2, fmt.Sprint("private-session-", i)), 7, "deepseek-v4-pro")
		_, err := senseNovaConversationPreference(ctx, scope, session, model.SenseNovaFingerprint("private-key"))
		require.NoError(t, err)
		advance(time.Millisecond)
	}
	for _, key := range server.Keys() {
		assert.NotContains(t, key, "private")
		assert.NotContains(t, key, "deepseek")
		if server.Type(key) == "hash" {
			values, err := common.RDB.HGetAll(ctx, key).Result()
			require.NoError(t, err)
			assert.Len(t, values, 128)
			for k, v := range values {
				assert.Len(t, k, 64)
				assert.Len(t, v, 64)
			}
		}
	}
	scope, session := senseNovaConversationIdentity(affinityTestContext(1, 2, "private-session-0"), 7, "deepseek-v4-pro")
	selected, err := senseNovaConversationPreference(ctx, scope, session, "")
	require.NoError(t, err)
	assert.Empty(t, selected)
	advance(15 * time.Minute)
	scope, session = senseNovaConversationIdentity(affinityTestContext(1, 2, "private-session-129"), 7, "deepseek-v4-pro")
	selected, err = senseNovaConversationPreference(ctx, scope, session, "")
	require.NoError(t, err)
	assert.Empty(t, selected)
}

func TestSenseNovaAffinityRoutingPrefersThenEscapesUnavailableKey(t *testing.T) {
	for _, unavailable := range []string{"none", "busy", "disabled", "removed", "cooling"} {
		t.Run(unavailable, func(t *testing.T) {
			channel, _ := senseNovaAdmissionFixture(t)
			t.Setenv("SENSENOVA_CONVERSATION_AFFINITY_ENABLED", "true")
			c := senseNovaAdmissionContext(t, channel)
			c.Set("id", 1)
			c.Set("token_id", 2)
			c.Request.Header.Set(senseNovaConversationHeader, "same-conversation")
			scope, session := senseNovaConversationIdentity(c, channel.Id, "deepseek-v4-pro")
			fingerprint := model.SenseNovaFingerprint("fake-account-b")
			_, err := senseNovaConversationPreference(context.Background(), scope, session, fingerprint)
			require.NoError(t, err)
			switch unavailable {
			case "busy":
				req := senseNovaTestBudgetRequest("other-active", 0)
				req.ChannelID = channel.Id
				req.Fingerprint = fingerprint
				policy := senseNovaTestBudgetPolicy()
				policy.TokensPerMinute = 0
				busy, _, err := reserveSenseNovaBudget(context.Background(), req, policy)
				require.NoError(t, err)
				require.NotNil(t, busy)
			case "disabled":
				channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusManuallyDisabled}
				require.NoError(t, model.DB.Save(channel).Error)
			case "removed":
				channel.Key = "fake-account-a"
				channel.ChannelInfo.MultiKeySize = 1
				require.NoError(t, model.DB.Save(channel).Error)
			case "cooling":
				snapshot, err := model.SenseNovaKeySnapshot(channel.Id, "fake-account-b", "deepseek-v4-pro")
				require.NoError(t, err)
				_, err = model.RecordSenseNovaFailure(snapshot, "fake-account-b", "deepseek-v4-pro", "rate_limited", false, 60, time.Now().Unix())
				require.NoError(t, err)
			}
			require.Nil(t, AdmitSenseNovaAttempt(c))
			defer FinishSenseNovaAdmission(c)
			expected := "fake-account-a"
			if unavailable == "none" {
				expected = "fake-account-b"
			}
			assert.Equal(t, expected, getSenseNovaAttempt(c).key)
			assert.Equal(t, 1, getSenseNovaAttempt(c).number)
		})
	}
}

func TestSenseNovaFollowupConfigurationConservativeDefaults(t *testing.T) {
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "true")
	t.Setenv("SENSENOVA_ADMISSION_MODELS", "deepseek-v4-pro")
	t.Setenv("SENSENOVA_CONVERSATION_AFFINITY_ENABLED", "")
	t.Setenv("SENSENOVA_CANARY_FOLLOWUPS", "")
	t.Setenv("SENSENOVA_CANARY_FOLLOWUP_INTERVAL_SECONDS", "")
	cfg, err := senseNovaAdmissionConfigFor("deepseek-v4-pro")
	require.NoError(t, err)
	assert.False(t, cfg.affinity)
	assert.Zero(t, cfg.followups)
	assert.Equal(t, 5*time.Second, cfg.followupInterval)
	for _, settings := range []struct{ enabled, allowance, interval string }{{"true", "3", "5"}, {"true", "1", "4"}, {"false", "1", "5"}, {"invalid", "0", "5"}} {
		t.Setenv("SENSENOVA_CONVERSATION_AFFINITY_ENABLED", settings.enabled)
		t.Setenv("SENSENOVA_CANARY_FOLLOWUPS", settings.allowance)
		t.Setenv("SENSENOVA_CANARY_FOLLOWUP_INTERVAL_SECONDS", settings.interval)
		_, err := senseNovaAdmissionConfigFor("deepseek-v4-pro")
		assert.Error(t, err)
	}
}
