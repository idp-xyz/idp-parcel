package domain

// RehydrateSupplierExpectedCostSpec 是预期成本行在库里的样子。首版与纠错版本同表
// 同形，靠回指与原因是否在场分辨——纠错换版本、原版本保留，因此一份成本的历史是
// 多行，不是一行被改写。
type RehydrateSupplierExpectedCostSpec struct {
	Version            SupplierCostVersionID
	Occurrence         TransportChargeOccurrence
	FeeItem            FeeItemReference
	RuleVersion        PurchaseRuleVersionReference
	Agreement          SupplierAgreementReference
	Evaluation         BuyEvaluationReference
	OriginalCurrency   CurrencyCode
	OriginalMinor      int64
	SettlementCurrency CurrencyCode
	SettlementMinor    int64
	Conversion         ConversionStepReference
	PriorVersion       SupplierCostVersionID
	CorrectionReason   CostCorrectionReason
}

// RehydrateSupplierExpectedCost 读回一份预期成本：复验行自身形状，不重算金额
// （ADR-0028 的重建门）。
//
// 它不走 FormSupplierExpectedCost，因为回指与原因这两个字段形成门根本不接：纠错
// 版本要求两件成对且不自指，只带一件的行说不清它纠正的是哪一版，而 AppendCorrection
// 从不产生那种形状。金额一侧两扇门自 ADR-0067 起同一口径：同币种两额必须相等对
// 所有版本成立，纠错版本不再例外。
func RehydrateSupplierExpectedCost(spec RehydrateSupplierExpectedCostSpec) (SupplierExpectedCost, error) {
	if !spec.Version.valid() ||
		!spec.Occurrence.id.valid() ||
		!spec.FeeItem.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.Agreement.valid() ||
		!spec.Evaluation.valid() ||
		!spec.OriginalCurrency.valid() ||
		spec.OriginalMinor <= 0 ||
		!spec.SettlementCurrency.valid() ||
		spec.SettlementMinor <= 0 {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	if spec.OriginalCurrency != spec.SettlementCurrency && !spec.Conversion.valid() {
		return SupplierExpectedCost{}, ErrConversionStepMissing
	}
	if spec.OriginalCurrency == spec.SettlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}

	if spec.PriorVersion.valid() || spec.CorrectionReason.valid() {
		if !spec.PriorVersion.valid() || !spec.CorrectionReason.valid() ||
			spec.PriorVersion == spec.Version {
			return SupplierExpectedCost{}, ErrInvalidSupplierCost
		}
	}

	return SupplierExpectedCost{
		version:            spec.Version,
		occurrence:         spec.Occurrence,
		feeItem:            spec.FeeItem,
		ruleVersion:        spec.RuleVersion,
		agreement:          spec.Agreement,
		evaluation:         spec.Evaluation,
		originalCurrency:   spec.OriginalCurrency,
		originalMinor:      spec.OriginalMinor,
		settlementCurrency: spec.SettlementCurrency,
		settlementMinor:    spec.SettlementMinor,
		conversion:         spec.Conversion,
		priorVersion:       spec.PriorVersion,
		correctionReason:   spec.CorrectionReason,
	}, nil
}
