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
// 它不走 FormSupplierExpectedCost，因为形成门的「同币种两额必须相等」只对首版成立。
// AppendCorrection 重述的是结算金额、原币金额原样留着，于是一份同币种的纠错版本
// 两额本就可以不等——拿形成门去验它，读回的会是一份写得好好的成本被判为不成立。
// 反过来，首版仍要过那一条：没有换算却出现第二个数，只可能是自行取汇率补算出来的。
//
// 纠错版本另要求回指与原因成对且不自指：只带一件的行说不清它纠正的是哪一版，而
// AppendCorrection 从不产生那种形状。
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

	isCorrection := spec.PriorVersion.valid() || spec.CorrectionReason.valid()
	if isCorrection {
		if !spec.PriorVersion.valid() || !spec.CorrectionReason.valid() ||
			spec.PriorVersion == spec.Version {
			return SupplierExpectedCost{}, ErrInvalidSupplierCost
		}
	} else if spec.OriginalCurrency == spec.SettlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
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
