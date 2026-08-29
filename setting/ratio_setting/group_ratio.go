package ratio_setting

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

// modelGroupRatioMap 保存模型级绝对有效倍率。配置后替换该模型选中的分组倍率，
// 未配置时回退到既有分组或特殊分组倍率。
var modelGroupRatioMap = types.NewRWMap[string, float64]()
var modelCompletionGroupRatioMap = types.NewRWMap[string, float64]()
var modelCacheGroupRatioMap = types.NewRWMap[string, float64]()
var modelCreateCacheGroupRatioMap = types.NewRWMap[string, float64]()

type ModelPricingGroupRatios struct {
	Input       float64
	Completion  float64
	Cache       float64
	CreateCache float64
}

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func ModelGroupRatio2JSONString() string {
	return modelGroupRatioMap.MarshalJSONString()
}

func UpdateModelGroupRatioByJSONString(jsonStr string) error {
	if err := CheckModelGroupRatio(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelGroupRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetModelGroupRatioCopy() map[string]float64 {
	return modelGroupRatioMap.ReadAll()
}

func GetModelGroupRatio(modelName string) (float64, bool) {
	modelName = FormatMatchingModelName(modelName)
	ratio, ok := modelGroupRatioMap.Get(modelName)
	return ratio, ok
}

// GetEffectiveGroupRatio 优先解析模型级覆盖，否则保留调用方传入的分组倍率。
func GetEffectiveGroupRatio(modelName string, fallback float64) float64 {
	if ratio, ok := GetModelGroupRatio(modelName); ok {
		return ratio
	}
	return fallback
}

func ModelCompletionGroupRatio2JSONString() string {
	return modelCompletionGroupRatioMap.MarshalJSONString()
}

func UpdateModelCompletionGroupRatioByJSONString(jsonStr string) error {
	if err := checkModelPricingGroupRatio(jsonStr, "model completion group ratio"); err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelCompletionGroupRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetModelCompletionGroupRatioCopy() map[string]float64 {
	return modelCompletionGroupRatioMap.ReadAll()
}

func GetModelCompletionGroupRatio(modelName string) (float64, bool) {
	return modelCompletionGroupRatioMap.Get(FormatMatchingModelName(modelName))
}

func ModelCacheGroupRatio2JSONString() string {
	return modelCacheGroupRatioMap.MarshalJSONString()
}

func UpdateModelCacheGroupRatioByJSONString(jsonStr string) error {
	if err := checkModelPricingGroupRatio(jsonStr, "model cache group ratio"); err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelCacheGroupRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetModelCacheGroupRatioCopy() map[string]float64 {
	return modelCacheGroupRatioMap.ReadAll()
}

func GetModelCacheGroupRatio(modelName string) (float64, bool) {
	return modelCacheGroupRatioMap.Get(FormatMatchingModelName(modelName))
}

func ModelCreateCacheGroupRatio2JSONString() string {
	return modelCreateCacheGroupRatioMap.MarshalJSONString()
}

func UpdateModelCreateCacheGroupRatioByJSONString(jsonStr string) error {
	if err := checkModelPricingGroupRatio(jsonStr, "model create-cache group ratio"); err != nil {
		return err
	}
	return types.LoadFromJsonStringWithCallback(modelCreateCacheGroupRatioMap, jsonStr, InvalidateExposedDataCache)
}

func GetModelCreateCacheGroupRatioCopy() map[string]float64 {
	return modelCreateCacheGroupRatioMap.ReadAll()
}

func GetModelCreateCacheGroupRatio(modelName string) (float64, bool) {
	return modelCreateCacheGroupRatioMap.Get(FormatMatchingModelName(modelName))
}

// GetEffectiveModelPricingGroupRatios resolves all token billing items through
// the same compatibility chain: item override -> model input override -> group.
func GetEffectiveModelPricingGroupRatios(modelName string, fallback float64) ModelPricingGroupRatios {
	input := GetEffectiveGroupRatio(modelName, fallback)
	completion := input
	if ratio, ok := GetModelCompletionGroupRatio(modelName); ok {
		completion = ratio
	}
	cache := input
	if ratio, ok := GetModelCacheGroupRatio(modelName); ok {
		cache = ratio
	}
	createCache := input
	if ratio, ok := GetModelCreateCacheGroupRatio(modelName); ok {
		createCache = ratio
	}
	return ModelPricingGroupRatios{
		Input:       input,
		Completion:  completion,
		Cache:       cache,
		CreateCache: createCache,
	}
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := common.UnmarshalJsonStr(jsonStr, &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

func CheckModelGroupRatio(jsonStr string) error {
	return checkModelPricingGroupRatio(jsonStr, "model group ratio")
}

func checkModelPricingGroupRatio(jsonStr string, label string) error {
	checkModelGroupRatio := make(map[string]float64)
	if err := common.UnmarshalJsonStr(jsonStr, &checkModelGroupRatio); err != nil {
		return err
	}
	for name, ratio := range checkModelGroupRatio {
		if ratio < 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return errors.New(label + " must be finite and not less than 0: " + name)
		}
	}
	return nil
}
