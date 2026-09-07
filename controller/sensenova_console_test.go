package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSenseNovaController(t *testing.T) *model.Channel {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMemoryCache, oldRedis := common.MemoryCacheEnabled, common.RedisEnabled
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupModelListControllerTestDB(t)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = oldMemoryCache, oldRedis
		common.SetDatabaseTypes(oldMainType, oldLogType)
	})
	common.MemoryCacheEnabled = false
	require.NoError(t, db.AutoMigrate(&model.SenseNovaKeyState{}, &model.Log{}))
	channel := &model.Channel{
		Name: "SenseNova independent accounts", Type: 1, SenseNovaPool: true, Status: 1,
		Key:    "fixture-account-a\nfixture-account-b\nfixture-account-c",
		Models: "glm-5.2,deepseek-v4-flash", Group: "default",
		ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 3, MultiKeyMode: "polling"},
	}
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func callSenseNovaManage(t *testing.T, request MultiKeyManageRequest, role int) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("role", role)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi_key/manage", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ManageMultiKeys(ctx)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response.Success
}

func TestSenseNovaConsoleIncludesPendingModelRecovery(t *testing.T) {
	states := []model.SenseNovaKeyState{
		{Fingerprint: "fingerprint", State: model.SenseNovaUsable, LastSuccessAt: 100},
		{Fingerprint: "fingerprint", Scope: "deepseek-v4-pro", State: model.SenseNovaUntested,
			Reason: "rate_limited", Failures: 2, LastSuccessAt: 90, LeaseUntil: 1000, Version: 7},
	}
	health := projectSenseNovaHealth(states)["fingerprint"]
	require.NotNil(t, health)
	require.Len(t, health.ModelStates, 1, "pending capacity verification must not disappear after a tiny probe")
	assert.Equal(t, model.SenseNovaUntested, health.ModelStates[0].State)
	assert.Equal(t, "rate_limited", health.ModelStates[0].Reason)
	assert.Equal(t, int64(100), health.LastSuccessAt)
	body, err := common.Marshal(health)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "lease")
	assert.NotContains(t, string(body), "version")
}

func TestSenseNovaConsoleHealthAndOpaqueIdentity(t *testing.T) {
	channel := setupSenseNovaController(t)
	keys := channel.GetKeys()
	channel.ChannelInfo.MultiKeyStatusList = map[int]int{1: common.ChannelStatusManuallyDisabled}
	require.NoError(t, channel.SaveChannelInfo())
	states := []model.SenseNovaKeyState{
		{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(keys[0]), State: model.SenseNovaCooling, Reason: "quota_exhausted", LastFailureAt: 100, NextProbeAt: 160},
		{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(keys[1]), State: model.SenseNovaUsable, LastSuccessAt: 90},
		{ChannelID: channel.Id, Fingerprint: model.SenseNovaFingerprint(keys[2]), Scope: "glm-5.2", State: model.SenseNovaCooling, Reason: "model_unavailable", NextProbeAt: 170},
	}
	require.NoError(t, model.DB.Create(&states).Error)

	recorder, success := callSenseNovaManage(t, MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status"}, common.RoleRootUser)
	require.True(t, success)
	var response struct {
		Data MultiKeyStatusResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data.Keys, 3)
	assert.Equal(t, map[string]int{"cooling": 1, "untested": 1}, response.Data.HealthCounts)
	assert.Equal(t, model.SenseNovaFingerprint(keys[0]), response.Data.Keys[0].KeyID)
	assert.Equal(t, "quota_exhausted", response.Data.Keys[0].Health.Reason)
	assert.Equal(t, int64(160), response.Data.Keys[0].Health.NextProbeAt)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, response.Data.Keys[1].Status)
	require.Len(t, response.Data.Keys[2].Health.ModelStates, 1)
	assert.Equal(t, "glm-5.2", response.Data.Keys[2].Health.ModelStates[0].Model)
	for _, key := range keys {
		assert.NotContains(t, recorder.Body.String(), key)
	}
	assert.NotContains(t, recorder.Body.String(), "lease_until")
	assert.NotContains(t, recorder.Body.String(), "version")

	recorder, success = callSenseNovaManage(t, MultiKeyManageRequest{ChannelId: channel.Id, Action: "get_key_status", HealthState: "cooling"}, common.RoleRootUser)
	require.True(t, success)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Keys, 1)
	assert.Equal(t, model.SenseNovaFingerprint(keys[0]), response.Data.Keys[0].KeyID)
	assert.Equal(t, 1, response.Data.ManualDisabledCount)
}

func TestSenseNovaConsoleRejectsStaleKeyIdentityForEveryRowAction(t *testing.T) {
	channel := setupSenseNovaController(t)
	oldFingerprint := model.SenseNovaFingerprint(channel.GetKeys()[0])
	require.NoError(t, model.DB.Model(channel).Update("key", "fixture-account-b\nfixture-account-a\nfixture-account-c").Error)
	index := 0
	for _, action := range []string{"disable_key", "enable_key", "delete_key", "test_key"} {
		t.Run(action, func(t *testing.T) {
			_, success := callSenseNovaManage(t, MultiKeyManageRequest{ChannelId: channel.Id, Action: action, KeyIndex: &index, KeyID: oldFingerprint}, common.RoleRootUser)
			assert.False(t, success)
		})
	}
	current, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "fixture-account-b\nfixture-account-a\nfixture-account-c", current.Key)
	assert.Empty(t, current.ChannelInfo.MultiKeyStatusList)
}

func TestSenseNovaConsoleProbeRequiresSensitivePermissionAndIdentity(t *testing.T) {
	channel := setupSenseNovaController(t)
	index := 0
	_, success := callSenseNovaManage(t, MultiKeyManageRequest{ChannelId: channel.Id, Action: "test_key", KeyIndex: &index, KeyID: model.SenseNovaFingerprint(channel.GetKeys()[0])}, common.RoleCommonUser)
	assert.False(t, success)
	_, success = callSenseNovaManage(t, MultiKeyManageRequest{ChannelId: channel.Id, Action: "test_key", KeyIndex: &index}, common.RoleRootUser)
	assert.False(t, success)
}
