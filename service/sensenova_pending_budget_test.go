package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSenseNovaPendingBudgetRejectsImpossiblePacingDuringRecovery(t *testing.T) {
	channel, _ := senseNovaAdmissionFixture(t)
	t.Setenv("SENSENOVA_UNKNOWN_TPM_INTERVAL_SECONDS", "300")
	t.Setenv("SENSENOVA_ADMISSION_WAIT_SECONDS", "75")
	channel.Key = "fake-account-a"
	channel.ChannelInfo.MultiKeySize = 1
	require.NoError(t, model.DB.Save(channel).Error)
	seedSenseNovaDueRecoveries(t, channel)
	holder := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(holder)
	require.Nil(t, AdmitSenseNovaAttempt(holder))
	require.True(t, getSenseNovaAttempt(holder).snapshot.RecoveryLease)
	MarkSenseNovaBudgetDispatched(holder)

	waiting := senseNovaLargeAdmissionContext(t, channel)
	defer FinishSenseNovaAdmission(waiting)
	ctx, cancel := context.WithTimeout(waiting.Request.Context(), 100*time.Millisecond)
	defer cancel()
	waiting.Request = waiting.Request.WithContext(ctx)
	admissionErr := AdmitSenseNovaAttempt(waiting)
	require.NotNil(t, admissionErr)
	assert.Equal(t, http.StatusServiceUnavailable, admissionErr.StatusCode)
	assert.NoError(t, ctx.Err(), "even immediate recovery completion cannot clear a 300-second pacing timer within 75 seconds")
	assert.GreaterOrEqual(t, waiting.GetInt64("sensenova_admission_retry_after"), int64(299))
	assert.Nil(t, currentSenseNovaAdmission(waiting).reservation, "pending health checks must not acquire a temporary budget reservation")
}
