package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	upstreamPricingSyncTaskDefaultIntervalMinutes = 60
	defaultUpstreamPricingSyncURL                 = "https://api.cheaptokens.shop/api/models"
	maxUpstreamPricingResponseBytes               = 10 << 20
	// upstreamPricingMarkupRatio adds one full discount step: e.g. an upstream
	// 2-discount (0.2 ratio) is sold as 3-discount (0.3 ratio).
	upstreamPricingMarkupRatio = 0.1
)

type upstreamPricingCatalog struct {
	Items []upstreamPricingItem `json:"items"`
}

type upstreamPricingItem struct {
	ModelID  string `json:"model_id"`
	Series   string `json:"series"`
	Category string `json:"category"`
	Active   bool   `json:"is_active"`
	Prices   struct {
		Input struct {
			Original float64 `json:"original"`
			Platform float64 `json:"platform"`
		} `json:"input"`
		Output struct {
			Original float64 `json:"original"`
			Platform float64 `json:"platform"`
		} `json:"output"`
		CacheCreation struct {
			Original float64 `json:"original"`
			Platform float64 `json:"platform"`
		} `json:"cache_creation"`
		CacheHit struct {
			Original float64 `json:"original"`
			Platform float64 `json:"platform"`
		} `json:"cache_hit"`
		OriginalCurrency string `json:"original_currency"`
		TieredEnabled    bool   `json:"tiered_enabled"`
		TimeOfDayEnabled bool   `json:"time_of_day_enabled"`
		OffPeak          struct {
			Input struct {
				Original float64 `json:"original"`
			} `json:"input"`
			Output struct {
				Original float64 `json:"original"`
			} `json:"output"`
			CacheCreation struct {
				Original float64 `json:"original"`
			} `json:"cache_creation"`
			CacheHit struct {
				Original float64 `json:"original"`
			} `json:"cache_hit"`
		} `json:"off_peak"`
	} `json:"prices"`
}

type upstreamPricingSyncResult struct {
	URL           string `json:"url"`
	UpdatedModels int    `json:"updated_models"`
	SkippedModels int    `json:"skipped_models"`
	FetchedAt     string `json:"fetched_at,omitempty"`
}

func upstreamPricingSyncURL() string {
	if value := strings.TrimSpace(os.Getenv("UPSTREAM_PRICING_SYNC_URL")); value != "" {
		return value
	}
	return defaultUpstreamPricingSyncURL
}

func fetchUpstreamPricingCatalog(ctx context.Context, endpoint string) (upstreamPricingCatalog, error) {
	if strings.TrimSpace(endpoint) == "" {
		return upstreamPricingCatalog{}, fmt.Errorf("upstream pricing URL is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return upstreamPricingCatalog{}, fmt.Errorf("build upstream pricing request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var response *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		response, err = client.Do(request)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return upstreamPricingCatalog{}, fmt.Errorf("fetch upstream pricing: %w", ctx.Err())
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(200*(1<<attempt)) * time.Millisecond)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return upstreamPricingCatalog{}, fmt.Errorf("fetch upstream pricing: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}
	if err != nil {
		return upstreamPricingCatalog{}, fmt.Errorf("fetch upstream pricing: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return upstreamPricingCatalog{}, fmt.Errorf("upstream pricing returned %s", response.Status)
	}
	var catalog upstreamPricingCatalog
	limited := io.LimitReader(response.Body, maxUpstreamPricingResponseBytes)
	if err := json.NewDecoder(limited).Decode(&catalog); err != nil {
		return upstreamPricingCatalog{}, fmt.Errorf("decode upstream pricing: %w", err)
	}
	return catalog, nil
}

func isSupportedUpstreamPricingItem(item upstreamPricingItem) bool {
	if !item.Active ||
		strings.EqualFold(strings.TrimSpace(item.Category), "public") ||
		strings.EqualFold(strings.TrimSpace(item.ModelID), "free/ds-v4-flash") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Series)) {
	case "deepseek", "glm", "gpt", "kimi", "qwen":
		return true
	default:
		return false
	}
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func usesUpstreamDiscountMarkup(item upstreamPricingItem) bool {
	switch strings.ToLower(strings.TrimSpace(item.Series)) {
	case "deepseek", "glm", "kimi", "qwen":
		return true
	default:
		return false
	}
}

func markedUpstreamRatio(original float64, platform float64) (float64, bool) {
	if original <= 0 || !finiteNonNegative(original) || !finiteNonNegative(platform) {
		return 0, false
	}
	ratio := math.Min(platform/original+upstreamPricingMarkupRatio, 1)
	// Keep persisted JSON stable when upstream decimals produce floating-point
	// noise such as 0.30000000000000004.
	return math.Round(ratio*1e12) / 1e12, true
}

// convertUpstreamPricingCatalog converts upstream CNY/USD numbers into the
// existing New API ratio representation without currency conversion. New API
// displays model_ratio*2 as the source input number, so model_ratio is input/2.
// Time-of-day off-peak prices are intentionally ignored: DeepSeek is sold
// against the peak original price so the displayed and billed ceiling is safe.
func convertUpstreamPricingCatalog(catalog upstreamPricingCatalog) (map[string]any, int, error) {
	modelRatios := map[string]any{}
	completionRatios := map[string]any{}
	cacheRatios := map[string]any{}
	createCacheRatios := map[string]any{}
	modelGroupRatios := map[string]any{}
	modelCompletionGroupRatios := map[string]any{}
	modelCacheGroupRatios := map[string]any{}
	modelCreateCacheGroupRatios := map[string]any{}
	skipped := 0

	for _, item := range catalog.Items {
		modelName := strings.TrimSpace(item.ModelID)
		if !isSupportedUpstreamPricingItem(item) {
			skipped++
			continue
		}
		if modelName == "" {
			return nil, skipped, fmt.Errorf("upstream pricing item has empty model_id")
		}
		// Tiered prices need context-aware billing expressions that this sync
		// path cannot represent safely; leave them for explicit configuration.
		if item.Prices.TieredEnabled {
			skipped++
			continue
		}
		input := item.Prices.Input.Original
		output := item.Prices.Output.Original
		cacheHit := item.Prices.CacheHit.Original
		cacheCreation := item.Prices.CacheCreation.Original
		if input <= 0 || output <= 0 || !finiteNonNegative(input) || !finiteNonNegative(output) || !finiteNonNegative(cacheHit) || !finiteNonNegative(cacheCreation) {
			return nil, skipped, fmt.Errorf("invalid upstream pricing for model %s", modelName)
		}

		modelRatios[modelName] = input / 2
		completionRatios[modelName] = output / input
		usesMarkup := usesUpstreamDiscountMarkup(item)
		if usesMarkup {
			inputGroupRatio, ok := markedUpstreamRatio(input, item.Prices.Input.Platform)
			if !ok {
				return nil, skipped, fmt.Errorf("invalid upstream input platform price for model %s", modelName)
			}
			completionGroupRatio, ok := markedUpstreamRatio(output, item.Prices.Output.Platform)
			if !ok {
				return nil, skipped, fmt.Errorf("invalid upstream output platform price for model %s", modelName)
			}
			modelGroupRatios[modelName] = inputGroupRatio
			modelCompletionGroupRatios[modelName] = completionGroupRatio
		}
		if cacheHit > 0 {
			cacheRatios[modelName] = cacheHit / input
			if usesMarkup {
				cacheGroupRatio, ok := markedUpstreamRatio(cacheHit, item.Prices.CacheHit.Platform)
				if !ok {
					return nil, skipped, fmt.Errorf("invalid upstream cache-hit platform price for model %s", modelName)
				}
				modelCacheGroupRatios[modelName] = cacheGroupRatio
			}
		}
		if cacheCreation > 0 {
			createCacheRatios[modelName] = cacheCreation / input
			if usesMarkup {
				createCacheGroupRatio, ok := markedUpstreamRatio(cacheCreation, item.Prices.CacheCreation.Platform)
				if !ok {
					return nil, skipped, fmt.Errorf("invalid upstream cache-creation platform price for model %s", modelName)
				}
				modelCreateCacheGroupRatios[modelName] = createCacheGroupRatio
			}
		}
	}
	if len(modelRatios) == 0 {
		return nil, skipped, fmt.Errorf("upstream pricing contains no valid supported models")
	}
	return map[string]any{
		"model_ratio":                    modelRatios,
		"completion_ratio":               completionRatios,
		"cache_ratio":                    cacheRatios,
		"create_cache_ratio":             createCacheRatios,
		"model_group_ratio":              modelGroupRatios,
		"model_completion_group_ratio":   modelCompletionGroupRatios,
		"model_cache_group_ratio":        modelCacheGroupRatios,
		"model_create_cache_group_ratio": modelCreateCacheGroupRatios,
	}, skipped, nil
}

func mergeFloatPricingMap(current map[string]float64, incoming any) {
	for modelName, raw := range valueMap(incoming) {
		if value, ok := asFloat64(raw); ok && finiteNonNegative(value) {
			current[modelName] = value
		}
	}
}

func persistUpstreamPricingData(data map[string]any) error {
	modelRatios := ratio_setting.GetModelRatioCopy()
	completionRatios := ratio_setting.GetCompletionRatioCopy()
	cacheRatios := ratio_setting.GetCacheRatioCopy()
	createCacheRatios := ratio_setting.GetCreateCacheRatioCopy()
	modelGroupRatios := ratio_setting.GetModelGroupRatioCopy()
	modelCompletionGroupRatios := ratio_setting.GetModelCompletionGroupRatioCopy()
	modelCacheGroupRatios := ratio_setting.GetModelCacheGroupRatioCopy()
	modelCreateCacheGroupRatios := ratio_setting.GetModelCreateCacheGroupRatioCopy()
	mergeFloatPricingMap(modelRatios, data["model_ratio"])
	mergeFloatPricingMap(completionRatios, data["completion_ratio"])
	mergeFloatPricingMap(cacheRatios, data["cache_ratio"])
	mergeFloatPricingMap(createCacheRatios, data["create_cache_ratio"])
	mergeFloatPricingMap(modelGroupRatios, data["model_group_ratio"])
	mergeFloatPricingMap(modelCompletionGroupRatios, data["model_completion_group_ratio"])
	mergeFloatPricingMap(modelCacheGroupRatios, data["model_cache_group_ratio"])
	mergeFloatPricingMap(modelCreateCacheGroupRatios, data["model_create_cache_group_ratio"])

	marshal := func(value any) (string, error) {
		encoded, err := common.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
	modelRatioJSON, err := marshal(modelRatios)
	if err != nil {
		return fmt.Errorf("encode model ratios: %w", err)
	}
	completionRatioJSON, err := marshal(completionRatios)
	if err != nil {
		return fmt.Errorf("encode completion ratios: %w", err)
	}
	cacheRatioJSON, err := marshal(cacheRatios)
	if err != nil {
		return fmt.Errorf("encode cache ratios: %w", err)
	}
	createCacheRatioJSON, err := marshal(createCacheRatios)
	if err != nil {
		return fmt.Errorf("encode create-cache ratios: %w", err)
	}
	modelGroupRatioJSON, err := marshal(modelGroupRatios)
	if err != nil {
		return fmt.Errorf("encode model group ratios: %w", err)
	}
	modelCompletionGroupRatioJSON, err := marshal(modelCompletionGroupRatios)
	if err != nil {
		return fmt.Errorf("encode model completion group ratios: %w", err)
	}
	modelCacheGroupRatioJSON, err := marshal(modelCacheGroupRatios)
	if err != nil {
		return fmt.Errorf("encode model cache group ratios: %w", err)
	}
	modelCreateCacheGroupRatioJSON, err := marshal(modelCreateCacheGroupRatios)
	if err != nil {
		return fmt.Errorf("encode model create-cache group ratios: %w", err)
	}
	values := map[string]string{
		"ModelRatio":                 modelRatioJSON,
		"CompletionRatio":            completionRatioJSON,
		"CacheRatio":                 cacheRatioJSON,
		"CreateCacheRatio":           createCacheRatioJSON,
		"ModelGroupRatio":            modelGroupRatioJSON,
		"ModelCompletionGroupRatio":  modelCompletionGroupRatioJSON,
		"ModelCacheGroupRatio":       modelCacheGroupRatioJSON,
		"ModelCreateCacheGroupRatio": modelCreateCacheGroupRatioJSON,
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		return fmt.Errorf("persist upstream pricing options: %w", err)
	}
	model.InvalidatePricingCache()
	return nil
}

func runUpstreamPricingSync(ctx context.Context) (upstreamPricingSyncResult, error) {
	endpoint := upstreamPricingSyncURL()
	result := upstreamPricingSyncResult{URL: endpoint}
	catalog, err := fetchUpstreamPricingCatalog(ctx, endpoint)
	if err != nil {
		return result, err
	}
	data, skipped, err := convertUpstreamPricingCatalog(catalog)
	result.SkippedModels = skipped
	if err != nil {
		return result, err
	}
	if err := persistUpstreamPricingData(data); err != nil {
		return result, err
	}
	result.UpdatedModels = len(valueMap(data["model_ratio"]))
	result.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	common.SysLog(fmt.Sprintf("upstream pricing sync succeeded: url=%s updated_models=%d skipped_models=%d", endpoint, result.UpdatedModels, result.SkippedModels))
	return result, nil
}
