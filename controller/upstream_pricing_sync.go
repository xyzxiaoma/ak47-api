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
	if !item.Active || strings.EqualFold(strings.TrimSpace(item.Category), "public") {
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

// convertUpstreamPricingCatalog converts upstream CNY/USD numbers into the
// existing New API ratio representation without currency conversion. New API
// displays model_ratio*2 as the source input number, so model_ratio is input/2.
// When a source has peak/off-peak prices, the off-peak values are used because
// they are the stable single baseline used by our national-model pricing.
func convertUpstreamPricingCatalog(catalog upstreamPricingCatalog) (map[string]any, int, error) {
	modelRatios := map[string]any{}
	completionRatios := map[string]any{}
	cacheRatios := map[string]any{}
	createCacheRatios := map[string]any{}
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
		if item.Prices.TimeOfDayEnabled && item.Prices.OffPeak.Input.Original > 0 && item.Prices.OffPeak.Output.Original > 0 {
			input = item.Prices.OffPeak.Input.Original
			output = item.Prices.OffPeak.Output.Original
			cacheHit = item.Prices.OffPeak.CacheHit.Original
			cacheCreation = item.Prices.OffPeak.CacheCreation.Original
		}
		if input <= 0 || output <= 0 || !finiteNonNegative(input) || !finiteNonNegative(output) || !finiteNonNegative(cacheHit) || !finiteNonNegative(cacheCreation) {
			return nil, skipped, fmt.Errorf("invalid upstream pricing for model %s", modelName)
		}

		modelRatios[modelName] = input / 2
		completionRatios[modelName] = output / input
		if cacheHit > 0 {
			cacheRatios[modelName] = cacheHit / input
		}
		if cacheCreation > 0 {
			createCacheRatios[modelName] = cacheCreation / input
		}
	}
	if len(modelRatios) == 0 {
		return nil, skipped, fmt.Errorf("upstream pricing contains no valid supported models")
	}
	return map[string]any{
		"model_ratio":        modelRatios,
		"completion_ratio":   completionRatios,
		"cache_ratio":        cacheRatios,
		"create_cache_ratio": createCacheRatios,
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
	mergeFloatPricingMap(modelRatios, data["model_ratio"])
	mergeFloatPricingMap(completionRatios, data["completion_ratio"])
	mergeFloatPricingMap(cacheRatios, data["cache_ratio"])
	mergeFloatPricingMap(createCacheRatios, data["create_cache_ratio"])

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
	for key, value := range map[string]string{
		"ModelRatio":       modelRatioJSON,
		"CompletionRatio":  completionRatioJSON,
		"CacheRatio":       cacheRatioJSON,
		"CreateCacheRatio": createCacheRatioJSON,
	} {
		if err := model.UpdateOption(key, value); err != nil {
			return fmt.Errorf("persist %s: %w", key, err)
		}
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
