package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉 ADR-0306 决定一的形成门半边（票 demo-chain-gaps/133）：评价在合同结算币上合计为零，客户费用、供应商预期成本首版与
// 纠错版本的形成门都答 ErrZeroEvaluationTotal、不成行，不并进未受理；负数、原币为零而结算币不为零照旧是未受理；币种与
// 换算核对先说话，合计为零不吞掉它们。

func zeroCustomerChargeSpec(t *testing.T) domain.CustomerChargeSpec {
	t.Helper()
	cny := mustValue(t, domain.NewCurrencyCode, "CNY")
	return domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-zero"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-zero"),
		OriginalCurrency:   cny,
		SettlementCurrency: cny,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	}
}

// Covers: AT-SA-215 的客户费用形成门——合计为零答零额、不成行；跨币种换算后结算币为零同样是零额；负数与原币为零而结算币
// 不为零是未受理；同币种两额不等、跨币种缺换算步骤先于零额作答。
func TestACustomerChargeTotallingZeroIsAZeroTotalNotAnInvalidCharge(t *testing.T) {
	zero := zeroCustomerChargeSpec(t)
	if _, err := domain.FormCustomerCharge(zero); !errors.Is(err, domain.ErrZeroEvaluationTotal) {
		t.Fatalf("合计为零 err = %v，want ErrZeroEvaluationTotal", err)
	}

	convertedToZero := zeroCustomerChargeSpec(t)
	convertedToZero.OriginalCurrency = mustValue(t, domain.NewCurrencyCode, "KRW")
	convertedToZero.OriginalMinor = 1
	convertedToZero.Conversion = mustValue(t, domain.NewConversionStepReference, "fx-series/syn/step-1")
	if _, err := domain.FormCustomerCharge(convertedToZero); !errors.Is(err, domain.ErrZeroEvaluationTotal) {
		t.Fatalf("换算后结算币为零 err = %v，want ErrZeroEvaluationTotal", err)
	}

	for name, mutate := range map[string]func(*domain.CustomerChargeSpec){
		"结算币为负": func(spec *domain.CustomerChargeSpec) { spec.OriginalMinor, spec.SettlementMinor = -1, -1 },
		"原币为零而结算币不为零": func(spec *domain.CustomerChargeSpec) {
			spec.OriginalCurrency = mustValue(t, domain.NewCurrencyCode, "USD")
			spec.SettlementMinor = 100
			spec.Conversion = mustValue(t, domain.NewConversionStepReference, "fx-series/syn/step-1")
		},
		"同币种两额不等": func(spec *domain.CustomerChargeSpec) { spec.SettlementMinor = 100 },
	} {
		spec := zeroCustomerChargeSpec(t)
		mutate(&spec)
		if _, err := domain.FormCustomerCharge(spec); !errors.Is(err, domain.ErrInvalidCustomerCharge) {
			t.Fatalf("%s err = %v，want ErrInvalidCustomerCharge", name, err)
		}
	}

	crossWithoutStep := zeroCustomerChargeSpec(t)
	crossWithoutStep.OriginalCurrency = mustValue(t, domain.NewCurrencyCode, "USD")
	if _, err := domain.FormCustomerCharge(crossWithoutStep); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("跨币种缺换算步骤而合计为零 err = %v，want ErrConversionStepMissing", err)
	}
}

// Covers: AT-SA-215 的供应商预期成本形成门——首版与纠错版本合计为零都答零额、不成行，原成本不动；负数是未受理。
func TestASupplierCostTotallingZeroIsAZeroTotalForTheFirstVersionAndACorrection(t *testing.T) {
	zero := crossCurrencySpec(t)
	zero.OriginalMinor, zero.SettlementMinor = 0, 0
	if _, err := domain.FormSupplierExpectedCost(zero); !errors.Is(err, domain.ErrZeroEvaluationTotal) {
		t.Fatalf("首版合计为零 err = %v，want ErrZeroEvaluationTotal", err)
	}
	negative := crossCurrencySpec(t)
	negative.SettlementMinor = -1
	if _, err := domain.FormSupplierExpectedCost(negative); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("首版结算币为负 err = %v，want ErrInvalidSupplierCost", err)
	}

	cost, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatalf("首版：%v", err)
	}
	correction := domain.CostCorrectionSpec{
		Version:          mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:       cost.Occurrence(),
		RuleVersion:      cost.RuleVersion(),
		Agreement:        cost.Agreement(),
		Evaluation:       mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-zero"),
		OriginalCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		Conversion:       mustValue(t, domain.NewConversionStepReference, "fx-series/syn/step-2"),
		Reason:           mustValue(t, domain.NewCostCorrectionReason, "OCCURRENCE_SUPERSEDED"),
	}
	if _, err := cost.AppendCorrection(correction); !errors.Is(err, domain.ErrZeroEvaluationTotal) {
		t.Fatalf("纠错合计为零 err = %v，want ErrZeroEvaluationTotal", err)
	}
	if _, minor := cost.SettlementAmount(); minor != 89980 {
		t.Fatalf("零额纠错改了原成本：%d", minor)
	}
	correction.SettlementMinor = -1
	if _, err := cost.AppendCorrection(correction); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("纠错结算币为负 err = %v，want ErrInvalidSupplierCost", err)
	}
}
