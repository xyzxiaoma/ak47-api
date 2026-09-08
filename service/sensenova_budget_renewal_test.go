package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Hold the actual Redis renewal at its command boundary until normal shutdown
// cancels its child context. No sleep or network timing selects the interleaving.
type senseNovaBlockedBudgetRenewal struct{ entered chan struct{} }

func (hook *senseNovaBlockedBudgetRenewal) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	hook.entered <- struct{}{}
	<-ctx.Done()
	return ctx, ctx.Err()
}
func (*senseNovaBlockedBudgetRenewal) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (*senseNovaBlockedBudgetRenewal) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (*senseNovaBlockedBudgetRenewal) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestSenseNovaBudgetRenewalNormalShutdownPreservesRequest(t *testing.T) {
	setupSenseNovaBudgetRedis(t)
	request := senseNovaTestBudgetRequest("normal-shutdown", 10)
	reservation, _, err := reserveSenseNovaBudget(context.Background(), request, senseNovaTestBudgetPolicy())
	require.NoError(t, err)
	require.NotNil(t, reservation)
	hook := &senseNovaBlockedBudgetRenewal{entered: make(chan struct{}, 1)}
	common.RDB.AddHook(hook)
	renewalContext, stop := context.WithCancel(context.Background())
	upstreamContext, cancelUpstream := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSenseNovaBudgetRenewal(renewalContext, cancelUpstream, reservation, "", request, time.Millisecond)
	}()
	t.Cleanup(func() { stop(); cancelUpstream(); <-done })
	select {
	case <-hook.entered:
	case <-time.After(time.Second):
		t.Fatal("renewal did not enter Redis")
	}
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("normal renewal shutdown did not finish")
	}
	assert.NoError(t, upstreamContext.Err(), "canceling the renewal worker must not cancel a valid request")
}

func TestSenseNovaBudgetRenewalOwnershipLossCancelsRequest(t *testing.T) {
	for _, poolRecovery := range []bool{false, true} {
		name := "budget-owner"
		if poolRecovery {
			name = "pool-recovery-owner"
		}
		t.Run(name, func(t *testing.T) {
			server, advance := setupSenseNovaBudgetRedis(t)
			request := senseNovaTestBudgetRequest("lost-owner", 10)
			reservation, _, err := reserveSenseNovaBudget(context.Background(), request, senseNovaTestBudgetPolicy())
			require.NoError(t, err)
			require.NotNil(t, reservation)
			recoveryOwner := ""
			if poolRecovery {
				recoveryOwner = reservation.owner
				claimed, _, err := claimSenseNovaCapacityRecovery(context.Background(), request.ChannelID, request.Model, recoveryOwner, time.Second)
				require.NoError(t, err)
				require.True(t, claimed)
				advance(2 * time.Second)
			} else {
				server.Del(reservation.keys[4])
			}
			renewalContext, stop := context.WithCancel(context.Background())
			upstreamContext, cancelUpstream := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				runSenseNovaBudgetRenewal(renewalContext, cancelUpstream, reservation, recoveryOwner, request, time.Millisecond)
			}()
			t.Cleanup(func() { stop(); cancelUpstream(); <-done })
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("lost ownership did not stop renewal")
			}
			assert.ErrorIs(t, upstreamContext.Err(), context.Canceled, "real ownership loss must cancel dispatch and body reads")
		})
	}
}
