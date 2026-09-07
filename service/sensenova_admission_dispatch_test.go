package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSenseNovaDispatchHonorsCancellation(t *testing.T) {
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SenseNovaKeyState{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previous; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	t.Setenv("SENSENOVA_ADMISSION_ENABLED", "true")
	t.Setenv("SENSENOVA_ADMISSION_MODELS", "deepseek-v4-pro")
	base := "https://token.sensenova.cn"
	pool := &model.Channel{Type: 1, BaseURL: &base, Key: "fake-key", Models: "deepseek-v4-pro", Status: 1, SenseNovaPool: true}
	require.NoError(t, db.Create(pool).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, _, selectedErr := service.SelectSenseNovaKey(c, pool, "deepseek-v4-pro")
	require.Nil(t, selectedErr)
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	cancel()

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	service.InitHttpClient()
	req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
	require.NoError(t, err)
	response, err := channel.DoRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	if response != nil {
		_ = response.Body.Close()
	}
	assert.Error(t, err)
	assert.Zero(t, calls.Load(), "a terminated SenseNova request must not reach upstream")
}
