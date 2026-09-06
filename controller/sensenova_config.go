package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// Key list indexes are editable presentation details. Preserve administrator
// state by exact identity when import, deduplication or reordering changes them.
func prepareSenseNovaKeys(channel, origin *model.Channel) error {
	keys := make([]string, 0)
	seen := make(map[string]bool)
	for _, item := range strings.Split(channel.Key, "\n") {
		key := strings.TrimSpace(item)
		if key == "" || seen[key] {
			continue
		}
		if strings.ContainsAny(key, " \t\r[]\"{}") {
			return errors.New("SenseNova keys must be plain API keys, one per line")
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if len(keys) == 0 || len(keys) > 200 {
		return errors.New("SenseNova pool requires 1 to 200 unique API keys")
	}
	info := model.ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling, MultiKeySize: len(keys), MultiKeyStatusList: map[int]int{}, MultiKeyDisabledReason: map[int]string{}, MultiKeyDisabledTime: map[int]int64{}}
	if origin != nil {
		indexes := make(map[string]int)
		for index, key := range origin.GetKeys() {
			indexes[key] = index
		}
		for index, key := range keys {
			if old, ok := indexes[key]; ok {
				if status, exists := origin.ChannelInfo.MultiKeyStatusList[old]; exists {
					info.MultiKeyStatusList[index] = status
				}
				if reason, exists := origin.ChannelInfo.MultiKeyDisabledReason[old]; exists {
					info.MultiKeyDisabledReason[index] = reason
				}
				if at, exists := origin.ChannelInfo.MultiKeyDisabledTime[old]; exists {
					info.MultiKeyDisabledTime[index] = at
				}
			}
		}
	}
	channel.Key, channel.Keys, channel.ChannelInfo = strings.Join(keys, "\n"), nil, info
	return nil
}
