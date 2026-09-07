package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type senseNovaRecoveryRenewal struct {
	parent context.Context
	stop   context.CancelFunc
	cancel context.CancelFunc
	done   chan struct{}
}

func startSenseNovaRecoveryRenewal(c *gin.Context, attempt *senseNovaAttempt) {
	if !attempt.snapshot.RecoveryLease || attempt.recovery != nil {
		return
	}
	parent := c.Request.Context()
	requestContext, cancelRequest := context.WithCancel(parent)
	renewalContext, stop := context.WithCancel(parent)
	renewal := &senseNovaRecoveryRenewal{parent: parent, stop: stop, cancel: cancelRequest, done: make(chan struct{})}
	attempt.recovery = renewal
	c.Request = c.Request.WithContext(requestContext)
	snapshot, key := attempt.snapshot, attempt.key
	go func() {
		defer close(renewal.done)
		runSenseNovaRecoveryRenewal(renewalContext, cancelRequest, snapshot, key, 30*time.Second)
	}()
}

func runSenseNovaRecoveryRenewal(ctx context.Context, cancelRequest context.CancelFunc, snapshot *model.SenseNovaSnapshot, key string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			owned, err := model.RenewSenseNovaRecovery(renewCtx, snapshot, key, time.Now().Unix())
			cancel()
			// Normal outcome cleanup cancels renewal too. Only a failed live
			// renewal should terminate upstream work and prevent a retry.
			if ctx.Err() != nil {
				return
			}
			if err != nil || !owned {
				common.SysError("SenseNova recovery lease lost or renewal failed")
				cancelRequest()
				return
			}
		}
	}
}

// Stop renewal before publishing the outcome, then release the lease in the
// normal attempt cleanup. Restoring the parent must not revive a canceled
// upstream request or permit a retry after lease loss.
func stopSenseNovaRecoveryRenewal(c *gin.Context) {
	attempt := getSenseNovaAttempt(c)
	if attempt == nil || attempt.recovery == nil {
		return
	}
	renewal := attempt.recovery
	renewal.stop()
	<-renewal.done
	canceled := c.Request.Context().Err() != nil
	renewal.cancel()
	if !canceled {
		c.Request = c.Request.WithContext(renewal.parent)
	}
	attempt.recovery = nil
}
