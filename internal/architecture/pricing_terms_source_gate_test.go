package architecture

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestParcelPricingReadsPartyCommercialOnlyToCheckACitedDirectionAuthorization 守 ADR-0260「认价卡」在依赖图上的那一半，
// 并给价卡入册前核对方向授权留出唯一的消费侧位置。评价按范围从 parcel-pricing 自己的价卡登记册选卡，体积系数、金额取整
// 与换算序列只从卡上读，不读商业价格政策的正文，也不读已解析商业依据。价卡入册前要核它引用的授权是不是一版已发布、
// 时刻仍有效、方向相同的规则，方向从那条规则指名的商业价格政策读出来——这件事只许发生在
// internal/parcelpricing/adapters/partycommercial，评价的编排文件不得导入参与方与商业，也不得导入这只适配器。
func TestParcelPricingReadsPartyCommercialOnlyToCheckACitedDirectionAuthorization(t *testing.T) {
	t.Parallel()

	pricing := modulePath + "/internal/parcelpricing/"
	commercial := modulePath + "/internal/partycommercial"
	adapter := pricing + "adapters/partycommercial"
	for _, file := range selectSources(t, "parcel-pricing 读 party-commercial 的位置", func(pkg string) bool {
		return strings.HasPrefix(pkg+"/", pricing)
	}) {
		for _, imported := range file.imports {
			reachesCommercial := imported == commercial || strings.HasPrefix(imported, commercial+"/")
			reachesAdapter := imported == adapter || strings.HasPrefix(imported, adapter+"/")
			if !reachesCommercial && !reachesAdapter {
				continue
			}
			if (file.pkg == adapter || strings.HasPrefix(file.pkg, adapter+"/")) && reachesCommercial {
				continue
			}
			t.Errorf("%s：导入 %q；参与方与商业只从价卡入册前的消费侧适配器读，评价不读（ADR-0260 决定一至三）", file.path, imported)
		}
		if isPricingEvaluationSource(file.path) {
			for _, imported := range file.imports {
				if imported == adapter || strings.HasPrefix(imported, adapter+"/") ||
					imported == commercial || strings.HasPrefix(imported, commercial+"/") {
					t.Errorf("%s：评价路径导入 %q", file.path, imported)
				}
			}
		}
	}
}

func isPricingEvaluationSource(path string) bool {
	base := filepath.Base(path)
	switch {
	case strings.HasPrefix(base, "evaluate_pricing"):
		return true
	case strings.HasPrefix(base, "form_evaluation_"):
		return true
	case strings.HasPrefix(base, "replay_pricing_evaluation"):
		return true
	default:
		return false
	}
}
