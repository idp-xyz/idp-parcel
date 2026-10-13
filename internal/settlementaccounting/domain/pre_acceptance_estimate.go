package domain

import "fmt"

// PreAcceptanceTrial 是一件声明包裹的一份受理前试算，在本上下文词汇里（ADR-0179 决定二、六）。试算由 parcel-pricing 形成，这里只
// 保存估价要用与留痕要存的那几格：哪一件、哪一份评价、它冻结了哪些版本、合计多少。没有合计（待判断、不可计价、冲突、失败，或合计
// 立不成本币种的最小币单位）即 hasTotal 为假——不拿零顶。
type PreAcceptanceTrial struct {
	parcel     string
	evaluation string
	versions   []string
	totalMinor int64
	currency   CurrencyCode
	hasTotal   bool
}

// NewPreAcceptanceTrial 造一份有合计的试算。件、评价与币种不许空白；版本清单原样保存，复算靠它（决定六）。
func NewPreAcceptanceTrial(parcel, evaluation string, versions []string, totalMinor int64, currency CurrencyCode) (PreAcceptanceTrial, error) {
	if parcel == "" || evaluation == "" || !currency.valid() {
		return PreAcceptanceTrial{}, fmt.Errorf("%w: pre-acceptance trial", ErrBlankValue)
	}
	return PreAcceptanceTrial{
		parcel: parcel, evaluation: evaluation, versions: append([]string(nil), versions...),
		totalMinor: totalMinor, currency: currency, hasTotal: true,
	}, nil
}

// NewUntotaledPreAcceptanceTrial 造一份没有合计的试算。评价标识照样保存：没有合计的那一份同样要能被指名。
func NewUntotaledPreAcceptanceTrial(parcel, evaluation string, versions []string) (PreAcceptanceTrial, error) {
	if parcel == "" || evaluation == "" {
		return PreAcceptanceTrial{}, fmt.Errorf("%w: pre-acceptance trial", ErrBlankValue)
	}
	return PreAcceptanceTrial{parcel: parcel, evaluation: evaluation, versions: append([]string(nil), versions...)}, nil
}

func (trial PreAcceptanceTrial) Parcel() string     { return trial.parcel }
func (trial PreAcceptanceTrial) Evaluation() string { return trial.evaluation }
func (trial PreAcceptanceTrial) Versions() []string { return append([]string(nil), trial.versions...) }

// Total 的第三个返回值为假即这一份没有合计。
func (trial PreAcceptanceTrial) Total() (int64, CurrencyCode, bool) {
	return trial.totalMinor, trial.currency, trial.hasTotal
}

// PreAcceptanceEstimateGap 是逐件 SELL 试算合计没形成时停在哪一格（决定二的停法）。各格都答未形成，分开是为了留下停在哪里，不是
// 为了换恢复动作。
type PreAcceptanceEstimateGap uint8

const (
	PreAcceptanceEstimateGapInvalid PreAcceptanceEstimateGap = iota
	PreAcceptanceEstimateFormed
	// PreAcceptanceEstimateHasNoTrial：一份试算都没有。
	PreAcceptanceEstimateHasNoTrial
	// PreAcceptanceEstimateTrialWithoutTotal：任一份评价没有合计。
	PreAcceptanceEstimateTrialWithoutTotal
	// PreAcceptanceEstimateCurrencyMismatch：任一份合计的币种不是控制作用域的币种——结算不换算。
	PreAcceptanceEstimateCurrencyMismatch
	// PreAcceptanceEstimateNotPositive：合计不是正数，或加不进 int64。
	PreAcceptanceEstimateNotPositive
)

func (gap PreAcceptanceEstimateGap) String() string {
	switch gap {
	case PreAcceptanceEstimateFormed:
		return "FORMED"
	case PreAcceptanceEstimateHasNoTrial:
		return "NO_TRIAL"
	case PreAcceptanceEstimateTrialWithoutTotal:
		return "TRIAL_WITHOUT_TOTAL"
	case PreAcceptanceEstimateCurrencyMismatch:
		return "CURRENCY_MISMATCH"
	case PreAcceptanceEstimateNotPositive:
		return "NOT_POSITIVE"
	default:
		return ""
	}
}

// PreAcceptanceEstimate 是一次形成了的受理前估价，连同它的来历（决定六）：形态、计价范围、价卡版本引用、逐件试算与合计。它随控制
// 结果留痕；同一输入与版本清单复算出同一合计。它不是费用、预估费用或应收应付（决定一）。
type PreAcceptanceEstimate struct {
	form       PreAcceptanceEstimateForm
	scope      EstimatePricingScope
	plan       string
	trials     []PreAcceptanceTrial
	totalMinor int64
	currency   CurrencyCode
}

func (estimate PreAcceptanceEstimate) Form() PreAcceptanceEstimateForm    { return estimate.form }
func (estimate PreAcceptanceEstimate) PricingScope() EstimatePricingScope { return estimate.scope }
func (estimate PreAcceptanceEstimate) PriceCard() string                  { return estimate.plan }
func (estimate PreAcceptanceEstimate) TotalMinor() int64                  { return estimate.totalMinor }
func (estimate PreAcceptanceEstimate) Currency() CurrencyCode             { return estimate.currency }
func (estimate PreAcceptanceEstimate) Trials() []PreAcceptanceTrial {
	return append([]PreAcceptanceTrial(nil), estimate.trials...)
}

// FormPerParcelSellTrialSum 按「逐件 SELL 试算合计」形态把各件试算合成控制金额（ADR-0179 决定二）。停法照搬旧执行器：任一份没有
// 合计、任一份合计的币种不是控制作用域的币种、合计不是正数，都答未形成并指名停在哪一格；不拿零、部分合计或换算顶替。缺件那一格
// 在试算那一侧就答了，到不了这里。
func FormPerParcelSellTrialSum(
	adoption PreAcceptanceEstimateAdoption,
	priceCard string,
	currency CurrencyCode,
	trials []PreAcceptanceTrial,
) (PreAcceptanceEstimate, PreAcceptanceEstimateGap, error) {
	if adoption.Form() != PerParcelSellTrialSum {
		return PreAcceptanceEstimate{}, PreAcceptanceEstimateGapInvalid, fmt.Errorf("%w: estimate form %d is not per-parcel SELL trial sum", ErrBlankValue, uint8(adoption.Form()))
	}
	if priceCard == "" || !currency.valid() {
		return PreAcceptanceEstimate{}, PreAcceptanceEstimateGapInvalid, fmt.Errorf("%w: pre-acceptance estimate", ErrBlankValue)
	}
	if len(trials) == 0 {
		return PreAcceptanceEstimate{}, PreAcceptanceEstimateHasNoTrial, nil
	}
	var total int64
	for _, trial := range trials {
		minor, trialCurrency, present := trial.Total()
		if !present {
			return PreAcceptanceEstimate{}, PreAcceptanceEstimateTrialWithoutTotal, nil
		}
		if trialCurrency.String() != currency.String() {
			return PreAcceptanceEstimate{}, PreAcceptanceEstimateCurrencyMismatch, nil
		}
		if minor < 0 || total > maxEstimateMinor-minor {
			return PreAcceptanceEstimate{}, PreAcceptanceEstimateNotPositive, nil
		}
		total += minor
	}
	if total <= 0 {
		return PreAcceptanceEstimate{}, PreAcceptanceEstimateNotPositive, nil
	}
	return PreAcceptanceEstimate{
		form:       adoption.Form(),
		scope:      adoption.PricingScope(),
		plan:       priceCard,
		trials:     append([]PreAcceptanceTrial(nil), trials...),
		totalMinor: total,
		currency:   currency,
	}, PreAcceptanceEstimateFormed, nil
}

const maxEstimateMinor = int64(^uint64(0) >> 1)
