package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var chargeOccurredAt = time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func occurrence(t *testing.T) domain.TransportChargeOccurrence {
	t.Helper()
	built, err := domain.NewTransportChargeOccurrence(
		mustValue(t, domain.NewChargeOccurrenceID, "charge-1"),
		mustValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		mustValue(t, domain.NewOccurrenceVersion, "charge-1/v1"),
		chargeOccurredAt,
	)
	if err != nil {
		t.Fatalf("new transport charge occurrence: %v", err)
	}
	return built
}

func crossCurrencySpec(t *testing.T) domain.SupplierExpectedCostSpec {
	t.Helper()
	return domain.SupplierExpectedCostSpec{
		Version:            mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v1"),
		Occurrence:         occurrence(t),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "LINEHAUL_BASE"),
		RuleVersion:        mustValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rules/v1"),
		Agreement:          mustValue(t, domain.NewSupplierAgreementReference, "agreement-1/v3"),
		Evaluation:         mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-1"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      12550,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    89980,
		Conversion:         mustValue(t, domain.NewConversionStepReference, "fx-series/2026-32/step-1"),
	}
}

// Covers: `AT-SA-176`「BUY 价卡为美元、合同结算币为人民币，评价已在内部完成换算——
// 采用评价内的原币金额、汇率序列版本和换算步骤作为换算依据，不重算也不改用其他
// 汇率」——原币与结算币两个金额各归各位、换算步骤随成本保存；类型上没有账单主张或
// 审核应付字段（不冒充，结构性）。
func TestACrossCurrencyCostAdoptsTheEvaluationsConversion(t *testing.T) {
	cost, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatalf("form supplier expected cost: %v", err)
	}

	originalCurrency, originalMinor := cost.OriginalAmount()
	if originalCurrency.String() != "USD" || originalMinor != 12550 {
		t.Fatalf("original = %s %d", originalCurrency, originalMinor)
	}
	settlementCurrency, settlementMinor := cost.SettlementAmount()
	if settlementCurrency.String() != "CNY" || settlementMinor != 89980 {
		t.Fatalf("settlement = %s %d", settlementCurrency, settlementMinor)
	}
	conversion, present := cost.Conversion()
	if !present || conversion.String() != "fx-series/2026-32/step-1" {
		t.Fatalf("conversion = %v present = %v; 换算依据必须随成本保存", conversion, present)
	}
}

// Covers: `AT-SA-177`「原币与合同结算币不同，但评价未携带换算步骤——保持待判断并记录
// 缺口，不自行取汇率补算，也不以原币金额直接充当结算币金额」的构造面——缺换算步骤
// 的跨币种成本立不起来（独立哨兵供编排落待判断）；同币种两金额不一致是矛盾输入。
func TestAMissingConversionStepStopsTheCost(t *testing.T) {
	missing := crossCurrencySpec(t)
	missing.Conversion = domain.ConversionStepReference{}
	if _, err := domain.FormSupplierExpectedCost(missing); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("err = %v, want ErrConversionStepMissing", err)
	}

	contradictory := crossCurrencySpec(t)
	contradictory.SettlementCurrency = contradictory.OriginalCurrency
	contradictory.SettlementMinor = contradictory.OriginalMinor + 1
	contradictory.Conversion = domain.ConversionStepReference{}
	if _, err := domain.FormSupplierExpectedCost(contradictory); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("err = %v; 同币种造出了第二个数", err)
	}

	same := crossCurrencySpec(t)
	same.SettlementCurrency = same.OriginalCurrency
	same.SettlementMinor = same.OriginalMinor
	same.Conversion = domain.ConversionStepReference{}
	if _, err := domain.FormSupplierExpectedCost(same); err != nil {
		t.Fatalf("form same-currency cost: %v", err)
	}
}

// Covers: `AT-SA-054`「供应商计费规则更正但客户规则未变——只追加供应商预期成本计价
// 纠错，不形成供应商账单贷项」与 `AT-SA-178`「汇率序列期次事后被更正——据新评价追加
// 计价纠错；原费用与原换算依据保留」——纠错换版本带原因换新评价指回原版，金额与
// 换算步骤整组取自新评价（ADR-0067），原成本不可变；重号纠错拒；跨币种纠错仍要
// 换算步骤。
func TestACorrectionAppendsWithoutRewritingTheOriginal(t *testing.T) {
	cost, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatalf("form supplier expected cost: %v", err)
	}

	corrected, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12600,
		SettlementMinor:  90110,
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/2026-32-corrected/step-1"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "FX_SERIES_CORRECTED/2026-32"),
	})
	if err != nil {
		t.Fatalf("append correction: %v", err)
	}
	prior, present := corrected.PriorVersion()
	if !present || prior.String() != "cost-1/v1" {
		t.Fatalf("prior = %s present = %v; 纠错必须指回原版", prior, present)
	}
	_, originalSettlement := cost.SettlementAmount()
	if originalSettlement != 89980 {
		t.Fatal("原费用被改写了")
	}
	if conversion, _ := cost.Conversion(); conversion.String() != "fx-series/2026-32/step-1" {
		t.Fatal("原换算依据被改写了")
	}
	if currency, minor := corrected.OriginalAmount(); currency.String() != "USD" || minor != 12600 {
		t.Fatalf("corrected original = %s %d; 原币金额必须随新评价重述", currency, minor)
	}
	_, correctedSettlement := corrected.SettlementAmount()
	if correctedSettlement != 90110 {
		t.Fatalf("corrected settlement = %d", correctedSettlement)
	}

	if _, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          cost.Version(),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-3"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12600,
		SettlementMinor:  90110,
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/step-2"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	}); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("err = %v; 重号的纠错分不出两版", err)
	}
	if _, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v3"),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-3"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12600,
		SettlementMinor:  90110,
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	}); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("err = %v; 跨币种纠错缺换算步骤被收下了", err)
	}
}

// Covers: ADR-0067 决定四与 `AT-SA-054`——同币种计价纠错的原币金额是新评价给出的
// 原币金额，与新结算金额相等，产物能原样喂回形成门（两扇门同一口径）；两额不等是
// 矛盾输入，与形成门同一理由：没有换算却造出了第二个数。
func TestASameCurrencyCorrectionRestatesBothAmounts(t *testing.T) {
	same := crossCurrencySpec(t)
	same.SettlementCurrency = same.OriginalCurrency
	same.SettlementMinor = same.OriginalMinor
	same.Conversion = domain.ConversionStepReference{}
	cost, err := domain.FormSupplierExpectedCost(same)
	if err != nil {
		t.Fatalf("form same-currency cost: %v", err)
	}

	corrected, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    11800,
		SettlementMinor:  11800,
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	})
	if err != nil {
		t.Fatalf("append correction: %v", err)
	}
	originalCurrency, originalMinor := corrected.OriginalAmount()
	settlementCurrency, settlementMinor := corrected.SettlementAmount()
	if originalCurrency.String() != "USD" || originalMinor != 11800 || settlementMinor != 11800 {
		t.Fatalf("corrected = %s %d / %d; 同币种纠错必须整组重述且两额相等",
			originalCurrency, originalMinor, settlementMinor)
	}

	conversion, _ := corrected.Conversion()
	if _, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            corrected.Version(),
		Occurrence:         corrected.Occurrence(),
		FeeItem:            corrected.FeeItem(),
		RuleVersion:        corrected.RuleVersion(),
		Agreement:          corrected.Agreement(),
		Evaluation:         corrected.Evaluation(),
		OriginalCurrency:   originalCurrency,
		OriginalMinor:      originalMinor,
		SettlementCurrency: settlementCurrency,
		SettlementMinor:    settlementMinor,
		Conversion:         conversion,
	}); err != nil {
		t.Fatalf("纠错产物原样喂回形成门被拒：%v", err)
	}

	if _, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v3"),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-3"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    11800,
		SettlementMinor:  11900,
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	}); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("err = %v; 同币种两额不等被收下了", err)
	}
}

// Covers: ADR-0067 决定二/三与 `AT-SA-164`——纠错换评价，采购规则版本、协议引用与
// 发生项版本、业务时点随评价整组重述（`OccurrenceVersion` 自注「发生项有效性更正换
// 版本，预期成本据以追加计价纠错」正是这条路径）；发生项 ID 是身份，与被纠正版本
// 不一致时拒——换 ID 就是另一份成本，只能另行形成。
func TestACorrectionRestatesTheEvaluationsReferenceGroup(t *testing.T) {
	cost, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatalf("form supplier expected cost: %v", err)
	}

	revisedOccurrence, err := domain.NewTransportChargeOccurrence(
		mustValue(t, domain.NewChargeOccurrenceID, "charge-1"),
		mustValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		mustValue(t, domain.NewOccurrenceVersion, "charge-1/v2"),
		chargeOccurredAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("new transport charge occurrence: %v", err)
	}
	corrected, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       revisedOccurrence,
		RuleVersion:      mustValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rules/v2"),
		Agreement:        mustValue(t, domain.NewSupplierAgreementReference, "agreement-1/v4"),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12600,
		SettlementMinor:  90110,
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/2026-32-corrected/step-1"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "OCCURRENCE_CORRECTED"),
	})
	if err != nil {
		t.Fatalf("append correction: %v", err)
	}
	if corrected.RuleVersion().String() != "purchase-rules/v2" ||
		corrected.Agreement().String() != "agreement-1/v4" {
		t.Fatalf("rule = %s agreement = %s; 规则版本与协议必须随评价重述",
			corrected.RuleVersion(), corrected.Agreement())
	}
	occurrence := corrected.Occurrence()
	if occurrence.ID().String() != "charge-1" ||
		occurrence.Version().String() != "charge-1/v2" ||
		!occurrence.OccurredAt().Equal(chargeOccurredAt.Add(2*time.Hour).UTC()) {
		t.Fatalf("occurrence = %s/%s @ %s; 版本与业务时点随评价走，ID 不动",
			occurrence.ID(), occurrence.Version(), occurrence.OccurredAt())
	}

	alien, err := domain.NewTransportChargeOccurrence(
		mustValue(t, domain.NewChargeOccurrenceID, "charge-9"),
		mustValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		mustValue(t, domain.NewOccurrenceVersion, "charge-9/v1"),
		chargeOccurredAt,
	)
	if err != nil {
		t.Fatalf("new transport charge occurrence: %v", err)
	}
	if _, err := cost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v3"),
		Occurrence:       alien,
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-3"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12600,
		SettlementMinor:  90110,
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/2026-32-corrected/step-1"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "OCCURRENCE_CORRECTED"),
	}); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("err = %v; 换了发生项 ID 的纠错被收下了", err)
	}
}

// Covers: ADR-0067 决定五——换算步骤必备按该版本自己的币种对判断，不按被它纠正的
// 那一版：原币币种随评价重述，跨币种首版纠错成同币种、同币种首版纠错成跨币种都是
// 合法形状。
func TestConversionNecessityFollowsTheCorrectionsOwnCurrencyPair(t *testing.T) {
	cross, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatalf("form cross-currency cost: %v", err)
	}
	toSame, err := cross.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       cross.Occurrence(),
		RuleVersion:      cross.RuleVersion(),
		Agreement:        cross.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:    90110,
		SettlementMinor:  90110,
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "OCCURRENCE_CORRECTED"),
	})
	if err != nil {
		t.Fatalf("跨币种首版纠错成同币种：%v", err)
	}
	if conversion, present := toSame.Conversion(); present {
		t.Fatalf("同币种纠错版本带着换算步骤：%s", conversion)
	}

	same := crossCurrencySpec(t)
	same.SettlementCurrency = same.OriginalCurrency
	same.SettlementMinor = same.OriginalMinor
	same.Conversion = domain.ConversionStepReference{}
	sameCost, err := domain.FormSupplierExpectedCost(same)
	if err != nil {
		t.Fatalf("form same-currency cost: %v", err)
	}
	if _, err := sameCost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       sameCost.Occurrence(),
		RuleVersion:      sameCost.RuleVersion(),
		Agreement:        sameCost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "EUR"),
		OriginalMinor:    9800,
		SettlementMinor:  11800,
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	}); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("err = %v; 同币种首版纠错成跨币种缺换算步骤被收下了", err)
	}
	crossed, err := sameCost.AppendCorrection(domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       sameCost.Occurrence(),
		RuleVersion:      sameCost.RuleVersion(),
		Agreement:        sameCost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "EUR"),
		OriginalMinor:    9800,
		SettlementMinor:  11800,
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/2026-33/step-1"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	})
	if err != nil {
		t.Fatalf("同币种首版纠错成跨币种：%v", err)
	}
	if currency, minor := crossed.OriginalAmount(); currency.String() != "EUR" || minor != 9800 {
		t.Fatalf("crossed original = %s %d", currency, minor)
	}
}
