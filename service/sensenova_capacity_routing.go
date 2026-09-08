package service

import (
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type senseNovaCapacityCandidate struct {
	key           string
	index         int
	needsRecovery bool
	request       senseNovaBudgetRequest
	observation   senseNovaCapacityObservation
	rank          int
}

// Numeric request metrics exist at admission, after middleware's initial key
// selection. Stable tiers preserve rotation among equally suitable candidates.
func senseNovaCapacityCandidates(c *gin.Context, channel *model.Channel, request senseNovaBudgetRequest, startKey string) ([]senseNovaCapacityCandidate, error) {
	keys := channel.GetKeys()
	hints, err := model.SenseNovaRoutingCandidates(channel, request.Model, common.GetTimestamp())
	if err != nil {
		return nil, err
	}
	start := 0
	for i, key := range keys {
		if key == startKey {
			start = i
			break
		}
	}
	excludedValue, _ := c.Get(senseNovaExcludedContext)
	excluded, _ := excludedValue.(map[string]bool)
	candidates := make([]senseNovaCapacityCandidate, 0, len(keys))
	requests := make([]senseNovaBudgetRequest, 0, len(keys))
	for offset := range keys {
		index := (start + offset) % len(keys)
		key := keys[index]
		if status, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok && status != common.ChannelStatusEnabled {
			continue
		}
		fingerprint := model.SenseNovaFingerprint(key)
		if excluded[strconv.Itoa(channel.Id)+":"+fingerprint] {
			continue
		}
		hint := hints[fingerprint]
		if !hint.Available {
			continue
		}
		candidateRequest := request
		candidateRequest.Fingerprint = fingerprint
		candidates = append(candidates, senseNovaCapacityCandidate{key: key, index: index, needsRecovery: hint.NeedsRecovery, request: candidateRequest})
		requests = append(requests, candidateRequest)
	}
	observations, err := readSenseNovaCapacity(c.Request.Context(), requests)
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		candidate := &candidates[i]
		candidate.observation = observations[i]
		candidate.rank = 1 // fresh ordinary capacity remains eligible
		if candidate.needsRecovery || candidate.observation.Failures > 0 {
			candidate.rank = 2
		} else if candidate.observation.Verified {
			candidate.rank = 0
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].rank < candidates[j].rank })
	return candidates, nil
}
