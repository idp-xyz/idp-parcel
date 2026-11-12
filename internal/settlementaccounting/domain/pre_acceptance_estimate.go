package domain

import (
	"fmt"
	"strings"
)

// PreAcceptanceTrial 是一件声明包裹的一份受理前试算，在本上下文词汇里（ADR-0179 决定二、六）。试算由 parcel-pricing 形成，这里只
// 保存估价要用与留痕要存的那几格：哪一件、哪一份评价、它冻结了哪些版本、合计多少。没有合计（待判断、不可计价、冲突、失败，或合计
// 立不成本币种的最小币单位）即 hasTotal 为假——不拿零顶。
//
// 计价重量与费用行是补记的两格（ADR-0379 决定一），只作留痕：它们不形成费用、预估费用或应收应付，合计也不从它们加——合计仍是评价
// 自己的合计（决定二）。补记之前形成的估价没有这两格，各自答缺席。
type PreAcceptanceTrial struct {
	parcel     string
	evaluation string
	versions   []string
	totalMinor int64
	currency   CurrencyCode
	hasTotal   bool
	weight     PreAcceptanceTrialPricingWeight
	hasWeight  bool
	lines      []PreAcceptanceTrialChargeLine
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

// WithPricingWeight 交回补记了计价重量的副本。
func (trial PreAcceptanceTrial) WithPricingWeight(weight PreAcceptanceTrialPricingWeight) PreAcceptanceTrial {
	trial.weight, trial.hasWeight = weight, true
	return trial
}

// WithChargeLines 交回补记了费用行的副本。至少一行：一份有合计的评价必有费用行，空列表只会让「没记」与「记了零行」分不开。
func (trial PreAcceptanceTrial) WithChargeLines(lines []PreAcceptanceTrialChargeLine) (PreAcceptanceTrial, error) {
	if len(lines) == 0 {
		return PreAcceptanceTrial{}, fmt.Errorf("%w: pre-acceptance trial charge lines", ErrBlankValue)
	}
	trial.lines = append([]PreAcceptanceTrialChargeLine(nil), lines...)
	return trial, nil
}

// PricingWeight 的第二个返回值为假即这一份没记计价重量。
func (trial PreAcceptanceTrial) PricingWeight() (PreAcceptanceTrialPricingWeight, bool) {
	return trial.weight, trial.hasWeight
}

// ChargeLines 的第二个返回值为假即这一份没记费用行。
func (trial PreAcceptanceTrial) ChargeLines() ([]PreAcceptanceTrialChargeLine, bool) {
	return append([]PreAcceptanceTrialChargeLine(nil), trial.lines...), len(trial.lines) > 0
}

// PreAcceptanceTrialPricingWeight 是一份试算评价的计价重量：值与单位照计价交来的原文。本上下文不比较、不换算它，只留痕。
type PreAcceptanceTrialPricingWeight struct {
	value string
	unit  string
}

func NewPreAcceptanceTrialPricingWeight(value, unit string) (PreAcceptanceTrialPricingWeight, error) {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(unit) == "" {
		return PreAcceptanceTrialPricingWeight{}, fmt.Errorf("%w: pre-acceptance trial pricing weight", ErrBlankValue)
	}
	return PreAcceptanceTrialPricingWeight{value: value, unit: unit}, nil
}

func (weight PreAcceptanceTrialPricingWeight) Value() string { return weight.value }
func (weight PreAcceptanceTrialPricingWeight) Unit() string  { return weight.unit }

// ChargeLineEffect 是一条费用行的增减方向，照计价的两个词。
type ChargeLineEffect uint8

const (
	ChargeLineEffectInvalid ChargeLineEffect = iota
	ChargeLineAdd
	ChargeLineDeduct
)

func (effect ChargeLineEffect) String() string {
	switch effect {
	case ChargeLineAdd:
		return "ADD"
	case ChargeLineDeduct:
		return "DEDUCT"
	default:
		return ""
	}
}

// PreAcceptanceTrialChargeLine 是一份试算评价上的一条费用行：费用代码、增减方向、金额与币种。
//
// 金额是评价算出的十进制原文，不换写成最小币单位：费用行按卡本币计、不随合计换算，逐行取整又是卡上可选的一步，一行的位数可以多于
// 币种的最小币单位。换写就得截断或拒收，前者改了留痕里的数，后者会让一份本已形成的估价因为留痕停下。金额的文法归计价，本上下文不再
// 立一道格式门（判据同 PricingChargeCode）。
type PreAcceptanceTrialChargeLine struct {
	code     PricingChargeCode
	effect   ChargeLineEffect
	amount   string
	currency CurrencyCode
}

func NewPreAcceptanceTrialChargeLine(code PricingChargeCode, effect ChargeLineEffect, amount string, currency CurrencyCode) (PreAcceptanceTrialChargeLine, error) {
	if !code.valid() || effect.String() == "" || strings.TrimSpace(amount) == "" || !currency.valid() {
		return PreAcceptanceTrialChargeLine{}, fmt.Errorf("%w: pre-acceptance trial charge line", ErrBlankValue)
	}
	return PreAcceptanceTrialChargeLine{code: code, effect: effect, amount: amount, currency: currency}, nil
}

func (line PreAcceptanceTrialChargeLine) Code() PricingChargeCode  { return line.code }
func (line PreAcceptanceTrialChargeLine) Effect() ChargeLineEffect { return line.effect }
func (line PreAcceptanceTrialChargeLine) Amount() string           { return line.amount }
func (line PreAcceptanceTrialChargeLine) Currency() CurrencyCode   { return line.currency }

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
	// PreAcceptanceEstimateNotPositive：合计不是正数。
	PreAcceptanceEstimateNotPositive
	// PreAcceptanceEstimateTrialNegative：任一份合计为负，哪怕各件加起来仍是正数（见 FormPerParcelSellTrialSum）。
	PreAcceptanceEstimateTrialNegative
	// PreAcceptanceEstimateSumOverflow：各件合计加不进 int64。
	PreAcceptanceEstimateSumOverflow
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
	case PreAcceptanceEstimateTrialNegative:
		return "TRIAL_NEGATIVE"
	case PreAcceptanceEstimateSumOverflow:
		return "SUM_OVERFLOW"
	default:
		return ""
	}
}

// PreAcceptanceEstimate 是一次形成了的受理前估价，连同它的来历（决定六）：形态、计价范围、价卡版本引用、逐件试算与合计。它随控制
// 结果留痕；同一输入与版本清单复算出同一合计。它不是费用、预估费用或应收应付（决定一）。
//
// 选卡用的服务产品是补记的一格（ADR-0379 决定一）：没申报即不记。它不参与合计，与价卡一样只说这份估价是怎么来的。
type PreAcceptanceEstimate struct {
	form       PreAcceptanceEstimateForm
	scope      EstimatePricingScope
	plan       string
	product    string
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

// ServiceProduct 的第二个返回值为假即没按产品选卡。
func (estimate PreAcceptanceEstimate) ServiceProduct() (string, bool) {
	return estimate.product, estimate.product != ""
}

// WithServiceProduct 交回记下选卡用服务产品的副本。空白不收：没申报就不调用，零值即没有。
func (estimate PreAcceptanceEstimate) WithServiceProduct(product string) (PreAcceptanceEstimate, error) {
	if strings.TrimSpace(product) == "" {
		return PreAcceptanceEstimate{}, fmt.Errorf("%w: pre-acceptance estimate service product", ErrBlankValue)
	}
	estimate.product = product
	return estimate, nil
}

// VersionReferenceParts 是一条版本引用在留痕里的几段：种类、标识、版本与可缺的指纹。
type VersionReferenceParts struct {
	Kind        string
	ID          string
	Version     string
	Fingerprint string
}

// VersionReferenceLiteral 是一条版本引用在留痕里的字面，`种类:标识@版本`，有指纹时再接 `#指纹`。写法与拆法
// （SplitVersionReferenceLiteral）放在一处：留痕的价卡列只存这一个字面，读口要把它拆回方案、版本与指纹（ADR-0379 决定一），两处
// 各写一份文法，改一边另一边就静默拆错。
func VersionReferenceLiteral(parts VersionReferenceParts) string {
	if parts.ID == "" {
		return ""
	}
	literal := parts.Kind + ":" + parts.ID + "@" + parts.Version
	if parts.Fingerprint != "" {
		literal += "#" + parts.Fingerprint
	}
	return literal
}

// SplitVersionReferenceLiteral 是 VersionReferenceLiteral 的逆。种类取第一个冒号之前（种类是封闭的词，不含冒号）；指纹取最后一个
// `#` 之后、版本取其前最后一个 `@` 之后——指纹是摘要、版本是短标签，两者不含这两个分隔符，标识里有也不错拆。拆不出四段里必有的三段
// 答 false，不猜。
func SplitVersionReferenceLiteral(literal string) (VersionReferenceParts, bool) {
	kind, rest, found := strings.Cut(literal, ":")
	if !found || kind == "" {
		return VersionReferenceParts{}, false
	}
	parts := VersionReferenceParts{Kind: kind}
	if at := strings.LastIndex(rest, "#"); at >= 0 {
		parts.Fingerprint = rest[at+1:]
		if parts.Fingerprint == "" {
			return VersionReferenceParts{}, false
		}
		rest = rest[:at]
	}
	at := strings.LastIndex(rest, "@")
	if at <= 0 || at == len(rest)-1 {
		return VersionReferenceParts{}, false
	}
	parts.ID, parts.Version = rest[:at], rest[at+1:]
	return parts, true
}

// FormPerParcelSellTrialSum 按「逐件 SELL 试算合计」形态把各件试算合成控制金额（ADR-0179 决定二）。停法照搬旧执行器：任一份没有
// 合计、任一份合计的币种不是控制作用域的币种、合计不是正数，都答未形成并指名停在哪一格；不拿零、部分合计或换算顶替。缺件那一格
// 在试算那一侧就答了，到不了这里。
//
// 任一份为负也停，哪怕合计仍是正数——决定二没有这一格，它是本形态的取舍：一件的 SELL 试算为负是价卡对那一件给了一笔返还，加进
// 合计会让冻结少于其余各件试算之和，等于拿这一件的负数去抵另几件该占的资金。合计加不进 int64 同样停，不绕回。两格各自指名，不并
// 进「合计不是正数」：并了，处理尝试上就分不出是价卡给了负数、合计太大，还是本就合出零。
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
		if minor < 0 {
			return PreAcceptanceEstimate{}, PreAcceptanceEstimateTrialNegative, nil
		}
		if total > maxEstimateMinor-minor {
			return PreAcceptanceEstimate{}, PreAcceptanceEstimateSumOverflow, nil
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
