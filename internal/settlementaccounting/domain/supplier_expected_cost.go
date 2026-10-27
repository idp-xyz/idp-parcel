package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidChargeOccurrence = errors.New("settlement accounting: invalid transport charge occurrence")
	ErrInvalidSupplierCost     = errors.New("settlement accounting: invalid supplier expected cost")
	ErrConversionStepMissing   = errors.New("settlement accounting: the evaluation carries no conversion step")
	// ErrZeroEvaluationTotal 说明采用的评价在合同结算币上合计为零（ADR-0306 决定一）。零是这张卡给出的价格，既不是缺口
	// 也不是坏输入：形成门不为它建费用行或成本行，另答这一格、不并入未受理，形成编排据此把请求记成负向结束（零额）。
	ErrZeroEvaluationTotal = errors.New("settlement accounting: the evaluation totals zero in the settlement currency")
)

// zeroTotalGate 判一组已过币种与换算核对的金额。结算币合计为零答 ErrZeroEvaluationTotal；原币为零而结算币不为零答
// invalid——换算造不出这种数。调用方把它排在币种与换算核对之后：跨币种缺换算步骤、同币种两额不等都比「合计为零」先说话，
// 否则一份本该作矛盾上抛的评价会被零额那一格静默吞下。
func zeroTotalGate(originalMinor, settlementMinor int64, invalid error) error {
	if settlementMinor == 0 {
		return ErrZeroEvaluationTotal
	}
	if originalMinor == 0 {
		return invalid
	}
	return nil
}

// ChargeOccurrenceID 指名 transport-fulfillment 拥有的运输收费发生项。发生项不是
// 金额、账单主张或审核应付——这里只引用。
type ChargeOccurrenceID struct{ requiredValue }

func NewChargeOccurrenceID(value string) (ChargeOccurrenceID, error) {
	required, err := newRequiredValue("charge occurrence ID", value)
	return ChargeOccurrenceID{required}, err
}

// OccurrenceReasonReference 指名发生项的原因（订舱、取消、失败尝试、实际履约——
// 语汇属 TF）。原/替代旅程分别计价，原因随引用带出供审计分辨。
type OccurrenceReasonReference struct{ requiredValue }

func NewOccurrenceReasonReference(value string) (OccurrenceReasonReference, error) {
	required, err := newRequiredValue("occurrence reason reference", value)
	return OccurrenceReasonReference{required}, err
}

// OccurrenceVersion 是发生项的有效性版本。发生项有效性更正换版本，预期成本据以追加
// 计价纠错。
type OccurrenceVersion struct{ requiredValue }

func NewOccurrenceVersion(value string) (OccurrenceVersion, error) {
	required, err := newRequiredValue("occurrence version", value)
	return OccurrenceVersion{required}, err
}

// TransportChargeOccurrence 是对一份运输收费发生项的只读引用。
type TransportChargeOccurrence struct {
	id         ChargeOccurrenceID
	reason     OccurrenceReasonReference
	version    OccurrenceVersion
	occurredAt time.Time
}

func NewTransportChargeOccurrence(
	id ChargeOccurrenceID,
	reason OccurrenceReasonReference,
	version OccurrenceVersion,
	occurredAt time.Time,
) (TransportChargeOccurrence, error) {
	if !id.valid() || !reason.valid() || !version.valid() || occurredAt.IsZero() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	return TransportChargeOccurrence{
		id:         id,
		reason:     reason,
		version:    version,
		occurredAt: occurredAt.UTC(),
	}, nil
}

func (occurrence TransportChargeOccurrence) ID() ChargeOccurrenceID {
	return occurrence.id
}

func (occurrence TransportChargeOccurrence) Reason() OccurrenceReasonReference {
	return occurrence.reason
}

func (occurrence TransportChargeOccurrence) Version() OccurrenceVersion {
	return occurrence.version
}

func (occurrence TransportChargeOccurrence) OccurredAt() time.Time {
	return occurrence.occurredAt
}

// FeeItemReference 指名费用项目。一个发生项可按规则形成零个或多个费用项目。
type FeeItemReference struct{ requiredValue }

func NewFeeItemReference(value string) (FeeItemReference, error) {
	required, err := newRequiredValue("fee item reference", value)
	return FeeItemReference{required}, err
}

// PurchaseRuleVersionReference 指名评价命中的可执行价卡版本（GLOSSARY「可执行价卡版本」）——
// 存的是 parcel-pricing 的定价方案版本，不是 party-commercial 的价格规则版本。它是幂等三维
// 之一：UC-SA-002 的「同一发生项、费用项目和规则版本不得重复形成预期成本」里的规则版本就是这一维。
//
// 类型名里的「采购规则版本」与 GLOSSARY 不符。它是导出名，与 HTTP 字段 purchaseRuleVersion、
// 库列 purchase_rule_version 同名，三处要一起改才不留错位，所以名字没有单独动。
type PurchaseRuleVersionReference struct{ requiredValue }

func NewPurchaseRuleVersionReference(value string) (PurchaseRuleVersionReference, error) {
	required, err := newRequiredValue("purchase rule version reference", value)
	return PurchaseRuleVersionReference{required}, err
}

// SupplierAgreementReference 指名供应商协议快照（PC/TF 拥有）。
type SupplierAgreementReference struct{ requiredValue }

func NewSupplierAgreementReference(value string) (SupplierAgreementReference, error) {
	required, err := newRequiredValue("supplier agreement reference", value)
	return SupplierAgreementReference{required}, err
}

// BuyEvaluationReference 指名 parcel-pricing 的 BUY 方向 PricingEvaluation。结算只
// 消费并保存采用关系，不重算。
type BuyEvaluationReference struct{ requiredValue }

func NewBuyEvaluationReference(value string) (BuyEvaluationReference, error) {
	required, err := newRequiredValue("buy evaluation reference", value)
	return BuyEvaluationReference{required}, err
}

// ConversionStepReference 指名评价内完成的原币→合同结算币换算步骤（含汇率序列版本）。
type ConversionStepReference struct{ requiredValue }

func NewConversionStepReference(value string) (ConversionStepReference, error) {
	required, err := newRequiredValue("conversion step reference", value)
	return ConversionStepReference{required}, err
}

// CostCorrectionReason 指名一次预期成本计价纠错的依据。
type CostCorrectionReason struct{ requiredValue }

func NewCostCorrectionReason(value string) (CostCorrectionReason, error) {
	required, err := newRequiredValue("cost correction reason", value)
	return CostCorrectionReason{required}, err
}

// SupplierCostVersionID 是预期成本的版本标识。计价纠错换版本，原版本保留。
type SupplierCostVersionID struct{ requiredValue }

func NewSupplierCostVersionID(value string) (SupplierCostVersionID, error) {
	required, err := newRequiredValue("supplier cost version ID", value)
	return SupplierCostVersionID{required}, err
}

// SupplierExpectedCostSpec 是形成一份供应商预期成本所需的全部输入。
type SupplierExpectedCostSpec struct {
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
}

// SupplierExpectedCost 是内部预期金额：发生项、采购商业依据与 BUY 评价共同形成
// （UC-SA-002 结果契约）。类型上没有账单主张、审核应付或付款字段——「不冒充」是
// 结构性的；供应商账单与贷项归 UC-SA-004。
type SupplierExpectedCost struct {
	version            SupplierCostVersionID
	occurrence         TransportChargeOccurrence
	feeItem            FeeItemReference
	priceCardVersion   PurchaseRuleVersionReference
	agreement          SupplierAgreementReference
	evaluation         BuyEvaluationReference
	originalCurrency   CurrencyCode
	originalMinor      int64
	settlementCurrency CurrencyCode
	settlementMinor    int64
	conversion         ConversionStepReference
	priorVersion       SupplierCostVersionID
	correctionReason   CostCorrectionReason
	// withdrawn 说这一版是失效纠错版本：它依的发生项已失效，这份成本不再成立（ADR-0232 决定七）。
	withdrawn bool
}

// FormSupplierExpectedCost 形成首个预期成本版本。原币与合同结算币不同时换算步骤
// 必备（AT-SA-177 的构造面）：评价未携带换算步骤就保持待判断，不自行取汇率补算，
// 也不以原币金额直接充当结算币金额——那两条路在这里都走不通，只能停。评价合计为零
// 答 ErrZeroEvaluationTotal、不成行（ADR-0306 决定一）。
func FormSupplierExpectedCost(spec SupplierExpectedCostSpec) (SupplierExpectedCost, error) {
	if !spec.Version.valid() ||
		!spec.Occurrence.id.valid() ||
		!spec.FeeItem.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.Agreement.valid() ||
		!spec.Evaluation.valid() ||
		!spec.OriginalCurrency.valid() ||
		spec.OriginalMinor < 0 ||
		!spec.SettlementCurrency.valid() ||
		spec.SettlementMinor < 0 {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	if spec.OriginalCurrency != spec.SettlementCurrency && !spec.Conversion.valid() {
		return SupplierExpectedCost{}, ErrConversionStepMissing
	}
	if spec.OriginalCurrency == spec.SettlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		// 同币种两个金额不一致：没有换算却造出了第二个数。
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	if err := zeroTotalGate(spec.OriginalMinor, spec.SettlementMinor, ErrInvalidSupplierCost); err != nil {
		return SupplierExpectedCost{}, err
	}
	return SupplierExpectedCost{
		version:            spec.Version,
		occurrence:         spec.Occurrence,
		feeItem:            spec.FeeItem,
		priceCardVersion:   spec.RuleVersion,
		agreement:          spec.Agreement,
		evaluation:         spec.Evaluation,
		originalCurrency:   spec.OriginalCurrency,
		originalMinor:      spec.OriginalMinor,
		settlementCurrency: spec.SettlementCurrency,
		settlementMinor:    spec.SettlementMinor,
		conversion:         spec.Conversion,
	}, nil
}

func (cost SupplierExpectedCost) Version() SupplierCostVersionID {
	return cost.version
}

func (cost SupplierExpectedCost) Occurrence() TransportChargeOccurrence {
	return cost.occurrence
}

func (cost SupplierExpectedCost) FeeItem() FeeItemReference {
	return cost.feeItem
}

func (cost SupplierExpectedCost) RuleVersion() PurchaseRuleVersionReference {
	return cost.priceCardVersion
}

func (cost SupplierExpectedCost) Agreement() SupplierAgreementReference {
	return cost.agreement
}

func (cost SupplierExpectedCost) Evaluation() BuyEvaluationReference {
	return cost.evaluation
}

func (cost SupplierExpectedCost) OriginalAmount() (CurrencyCode, int64) {
	return cost.originalCurrency, cost.originalMinor
}

func (cost SupplierExpectedCost) SettlementAmount() (CurrencyCode, int64) {
	return cost.settlementCurrency, cost.settlementMinor
}

// Conversion 只在跨币种时给出（AT-SA-176 采用评价内换算步骤）。
func (cost SupplierExpectedCost) Conversion() (ConversionStepReference, bool) {
	return cost.conversion, cost.conversion.valid()
}

// PriorVersion 只在纠错版本上给出，指回被纠正的那一版。
func (cost SupplierExpectedCost) PriorVersion() (SupplierCostVersionID, bool) {
	return cost.priorVersion, cost.priorVersion.valid()
}

// CorrectionReason 只在纠错版本上给出。
func (cost SupplierExpectedCost) CorrectionReason() (CostCorrectionReason, bool) {
	return cost.correctionReason, cost.correctionReason.valid()
}

// CostCorrectionSpec 是追加一次计价纠错所需的全部输入：评价引用、评价命中的价卡版本、
// 协议引用、发生项版本与业务时点、金额币种与换算步骤整组取自新评价（ADR-0067
// 决定二），不从被纠正版本继承。合同结算币不在此列——它是合同交给评价的输入而
// 不是评价的产物，计价纠错不改合同。
type CostCorrectionSpec struct {
	Version          SupplierCostVersionID
	Occurrence       TransportChargeOccurrence
	RuleVersion      PurchaseRuleVersionReference
	Agreement        SupplierAgreementReference
	Evaluation       BuyEvaluationReference
	OriginalCurrency CurrencyCode
	OriginalMinor    int64
	SettlementMinor  int64
	Conversion       ConversionStepReference
	Reason           CostCorrectionReason
}

// AppendCorrection 依据新评价（规则更正、汇率序列更正或发生项有效性更正）追加计价
// 纠错版本（AT-SA-054/164/178）：换版本、带原因、指回原版，计价结果整组取自新评价；
// 原版本一字不动，也不形成供应商账单贷项（那归 UC-SA-004）。
//
// 纠错不动的只有身份（ADR-0067 决定三）：发生项 ID 与被纠正版本不一致时拒——换 ID
// 就是另一份成本，只能另行形成；发生项版本与业务时点随评价走。换算步骤必备按本版
// 自己的币种对判断（决定五）：原币币种随评价重述，首版跨币种而纠错版本同币种、或
// 反过来，都是合法形状。同币种两额必须相等对纠错版本同样成立（决定四），理由与
// 形成门那条一字不差：没有换算却造出了第二个数。
func (cost SupplierExpectedCost) AppendCorrection(spec CostCorrectionSpec) (SupplierExpectedCost, error) {
	if !spec.Version.valid() || spec.Version == cost.version ||
		!spec.Occurrence.id.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.Agreement.valid() ||
		!spec.Evaluation.valid() ||
		!spec.OriginalCurrency.valid() || spec.OriginalMinor < 0 ||
		spec.SettlementMinor < 0 || !spec.Reason.valid() {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	if spec.Occurrence.id != cost.occurrence.id {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	if spec.OriginalCurrency != cost.settlementCurrency && !spec.Conversion.valid() {
		return SupplierExpectedCost{}, ErrConversionStepMissing
	}
	if spec.OriginalCurrency == cost.settlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	// 新评价合计为零时不追加纠错版本：零额不成行，链头原样（ADR-0306 决定三）。
	if err := zeroTotalGate(spec.OriginalMinor, spec.SettlementMinor, ErrInvalidSupplierCost); err != nil {
		return SupplierExpectedCost{}, err
	}
	corrected := cost
	corrected.version = spec.Version
	corrected.occurrence = spec.Occurrence
	corrected.priceCardVersion = spec.RuleVersion
	corrected.agreement = spec.Agreement
	corrected.evaluation = spec.Evaluation
	corrected.originalCurrency = spec.OriginalCurrency
	corrected.originalMinor = spec.OriginalMinor
	corrected.settlementMinor = spec.SettlementMinor
	corrected.conversion = spec.Conversion
	corrected.priorVersion = cost.version
	corrected.correctionReason = spec.Reason
	corrected.withdrawn = false
	return corrected, nil
}
