package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉 ADR-0229 的领域半边：停因按段与方向分格，一类参数登记只续办它解得开的那几种停因；续办依据只有两种来源；
// 代次链首代无前一代、后续各代指名前一代并带依据；由一份停下的请求起下一代时问题原样不动、本份不被改写。

// Covers: AT-SA-188 停因的段与方向——发起之前那几格各属一路，计价那三格两路都有，换算步骤缺席只在 BUY、映射不全只在
// SELL；词表往返一致。
func TestEvaluationStopCausesSitOnTheirStageAndDirection(t *testing.T) {
	cases := []struct {
		cause domain.EvaluationStopCause
		stage domain.EvaluationStopStage
		buy   bool
		sell  bool
	}{
		{domain.StopSettlementPolicyNotAdopted, domain.StopBeforeRequest, false, true},
		{domain.StopSettlementAccountNotRegistered, domain.StopBeforeRequest, false, true},
		{domain.StopSellBillingMomentNotAdopted, domain.StopBeforeRequest, false, true},
		{domain.StopParcelCommercialResolutionAbsent, domain.StopBeforeRequest, false, true},
		{domain.StopOccurrenceReasonNotRegistered, domain.StopBeforeRequest, true, false},
		{domain.StopTriggerFeeItemNotRegistered, domain.StopBeforeRequest, true, false},
		{domain.StopPriceCardNotConfigured, domain.StopInPricing, true, true},
		{domain.StopPriceCardApplicabilityConflict, domain.StopInPricing, true, true},
		{domain.StopPricingInputUnavailable, domain.StopInPricing, true, true},
		{domain.StopEvaluationPendingJudgment, domain.StopAtAdoption, true, true},
		{domain.StopEvaluationUnratable, domain.StopAtAdoption, true, true},
		{domain.StopEvaluationConflict, domain.StopAtAdoption, true, true},
		{domain.StopEvaluationFailed, domain.StopAtAdoption, true, true},
		{domain.StopAmountRoundingUndeclared, domain.StopAtAdoption, true, true},
		{domain.StopConversionStepMissing, domain.StopAtAdoption, true, false},
		{domain.StopFeeItemMappingIncomplete, domain.StopAtAdoption, false, true},
		{domain.StopPendingJudgmentCauseUnrecorded, domain.StopAtAdoption, false, true},
	}
	if len(cases) != len(domain.AllEvaluationStopCauses()) {
		t.Fatalf("用例 %d 格，停因全集 %d 格", len(cases), len(domain.AllEvaluationStopCauses()))
	}
	for _, test := range cases {
		if got := test.cause.Stage(); got != test.stage {
			t.Errorf("%s 的段 = %s，want %s", test.cause, got, test.stage)
		}
		if got := test.cause.OccursIn(domain.BuyEvaluationDirection); got != test.buy {
			t.Errorf("%s 在 BUY = %v，want %v", test.cause, got, test.buy)
		}
		if got := test.cause.OccursIn(domain.SellEvaluationDirection); got != test.sell {
			t.Errorf("%s 在 SELL = %v，want %v", test.cause, got, test.sell)
		}
		parsed, err := domain.EvaluationStopCauseFromName(test.cause.String())
		if err != nil || parsed != test.cause {
			t.Errorf("%s 往返 = %s %v", test.cause, parsed, err)
		}
	}
	if _, err := domain.EvaluationStopCauseFromName("SOMETHING_ELSE"); !errors.Is(err, domain.ErrInvalidEvaluationResumption) {
		t.Fatalf("词表外的停因 err = %v，想要拒", err)
	}
}

// Covers: AT-SA-189 一类参数登记只续办它解得开的停因——判据是登了之后同一个问题再问一次可能不同；结算政策没采用、
// 包裹没有商业解析与计价输入不可得哪一类登记都解不开，只留给运营续办口。
func TestAParameterRegistrationResumesOnlyTheCausesItCanUnblock(t *testing.T) {
	want := map[domain.EvaluationParameterKind][]domain.EvaluationStopCause{
		domain.ParameterSettlementAccount:         {domain.StopSettlementAccountNotRegistered},
		domain.ParameterSellBillingMomentAdoption: {domain.StopSellBillingMomentNotAdopted},
		domain.ParameterBuyEvaluationTrigger:      {domain.StopOccurrenceReasonNotRegistered, domain.StopTriggerFeeItemNotRegistered},
		domain.ParameterFeeItemMapping:            {domain.StopFeeItemMappingIncomplete, domain.StopPendingJudgmentCauseUnrecorded},
		domain.ParameterPriceCard: {
			domain.StopPriceCardNotConfigured, domain.StopPriceCardApplicabilityConflict,
			domain.StopEvaluationPendingJudgment, domain.StopEvaluationUnratable, domain.StopEvaluationConflict,
			domain.StopEvaluationFailed, domain.StopAmountRoundingUndeclared, domain.StopConversionStepMissing,
			domain.StopPendingJudgmentCauseUnrecorded,
		},
		domain.ParameterReferenceSeries: {
			domain.StopEvaluationPendingJudgment, domain.StopEvaluationFailed, domain.StopConversionStepMissing,
			domain.StopPendingJudgmentCauseUnrecorded,
		},
		domain.ParameterReferenceCatalogue: {
			domain.StopEvaluationPendingJudgment, domain.StopEvaluationFailed, domain.StopPendingJudgmentCauseUnrecorded,
		},
	}
	for kind, causes := range want {
		got := domain.EvaluationStopCausesResumedBy(kind)
		if len(got) != len(causes) {
			t.Errorf("%s 解得开 %v，want %v", kind, got, causes)
			continue
		}
		for index := range got {
			if got[index] != causes[index] {
				t.Errorf("%s 解得开 %v，want %v", kind, got, causes)
				break
			}
		}
		parsed, err := domain.EvaluationParameterKindFromName(kind.String())
		if err != nil || parsed != kind {
			t.Errorf("%s 往返 = %s %v", kind, parsed, err)
		}
	}
	for _, cause := range []domain.EvaluationStopCause{
		domain.StopSettlementPolicyNotAdopted, domain.StopParcelCommercialResolutionAbsent, domain.StopPricingInputUnavailable,
	} {
		for kind := range want {
			if cause.ResumedBy(kind) {
				t.Errorf("%s 被 %s 续办，它只该留给运营续办口", cause, kind)
			}
		}
	}
	if got := domain.EvaluationStopCausesResumedBy(domain.EvaluationParameterKindInvalid); len(got) != 0 {
		t.Fatalf("零值种类解得开 %v", got)
	}
}

// Covers: 续办依据只有两种来源，同一次登记、同一个运营引用造出同一个依据；空引用与零值种类拒；册上的串按来源段重建。
func TestAResumeBasisNamesEitherAParameterRegistrationOrAnOperatorReference(t *testing.T) {
	parameter, err := domain.NewParameterResumeBasis(domain.ParameterPriceCard, "pricing-parameter/abc")
	if err != nil {
		t.Fatalf("参数依据：%v", err)
	}
	again, _ := domain.NewParameterResumeBasis(domain.ParameterPriceCard, "pricing-parameter/abc")
	if parameter != again || !parameter.Present() {
		t.Fatal("同一次登记造出了两个依据，或依据不在场")
	}
	operator, err := domain.NewOperatorResumeBasis("SYN-TICKET-1")
	if err != nil || operator == parameter {
		t.Fatalf("运营依据：%v", err)
	}
	for _, value := range []string{parameter.String(), operator.String()} {
		rebuilt, err := domain.ResumeBasisFromName(value)
		if err != nil || rebuilt.String() != value {
			t.Fatalf("重建 %q = %q %v", value, rebuilt, err)
		}
	}
	if _, err := domain.ResumeBasisFromName("SOMEWHERE/x"); !errors.Is(err, domain.ErrInvalidEvaluationResumption) {
		t.Fatalf("来源段外的依据 err = %v，想要拒", err)
	}
	if _, err := domain.NewParameterResumeBasis(domain.EvaluationParameterKindInvalid, "x"); err == nil {
		t.Fatal("零值种类造出了依据")
	}
	if _, err := domain.NewOperatorResumeBasis(" "); err == nil {
		t.Fatal("空运营引用造出了依据")
	}
	if (domain.ResumeBasis{}).Present() {
		t.Fatal("零值依据在场")
	}
}

// Covers: AT-SA-190 代次链——首代无前一代、可带依据（发起停办续办后的第一份）；续办出来的下一代问题原样、指名前一代、
// 带依据；没有依据或没有前一代的第二代立不住；本份不被改写。
func TestASellRequestResumesIntoTheNextGenerationAskingTheSameQuestion(t *testing.T) {
	spec := sellEvaluationRequestSpec(t)
	first, err := domain.SubmitSellEvaluationRequest(spec)
	if err != nil {
		t.Fatalf("首代：%v", err)
	}
	if first.Generation() != 1 {
		t.Fatalf("零值代次当首代读，得 %d", first.Generation())
	}
	if _, has := first.Previous(); has || first.ResumeBasis().Present() {
		t.Fatal("原始首代带了前一代或依据")
	}
	basis, _ := domain.NewParameterResumeBasis(domain.ParameterPriceCard, "pricing-parameter/1")
	nextID := mustRef(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2")
	next, err := first.Resume(nextID, basis)
	if err != nil {
		t.Fatalf("续办：%v", err)
	}
	if next.ID() != nextID || next.Generation() != 2 || next.ResumeBasis() != basis {
		t.Fatalf("下一代 = %s 第 %d 代 依据 %s", next.ID(), next.Generation(), next.ResumeBasis())
	}
	if previous, has := next.Previous(); !has || previous != first.ID() {
		t.Fatalf("下一代没指名前一代：%s %v", previous, has)
	}
	if next.NaturalKey() != first.NaturalKey() || next.Account() != first.Account() ||
		next.SettlementCurrency() != first.SettlementCurrency() || next.PricingScope() != first.PricingScope() ||
		next.Source() != first.Source() || !next.PricingBasisAt().Equal(first.PricingBasisAt()) {
		t.Fatal("下一代换了问题")
	}
	if first.Generation() != 1 || first.ID() != spec.ID {
		t.Fatal("续办改写了前一代")
	}
	if next.EstimatedCustomerChargeID() == first.EstimatedCustomerChargeID() {
		t.Fatal("两代派生出同一个费用标识")
	}

	resumedFirst := spec
	resumedFirst.ResumeBasis = basis
	if _, err := domain.SubmitSellEvaluationRequest(resumedFirst); err != nil {
		t.Fatalf("发起停办续办之后的首代带依据：%v", err)
	}
	for name, mutate := range map[string]func(*domain.SellEvaluationRequestSpec){
		"第二代没有前一代": func(s *domain.SellEvaluationRequestSpec) { s.Generation = 2; s.ResumeBasis = basis },
		"第二代没有依据":  func(s *domain.SellEvaluationRequestSpec) { s.Generation = 2; s.Previous = first.ID() },
		"首代带前一代":   func(s *domain.SellEvaluationRequestSpec) { s.Previous = nextID },
		"负代次":      func(s *domain.SellEvaluationRequestSpec) { s.Generation = -1 },
		"自己接自己": func(s *domain.SellEvaluationRequestSpec) {
			s.Generation = 2
			s.Previous = s.ID
			s.ResumeBasis = basis
		},
	} {
		broken := spec
		mutate(&broken)
		if _, err := domain.SubmitSellEvaluationRequest(broken); !errors.Is(err, domain.ErrInvalidSellEvaluationRequest) {
			t.Errorf("%s err = %v，想要拒", name, err)
		}
	}
}

// Covers: AT-SA-190 的 BUY 半边——下一代的主要范围、计算目的与三件来源引用原样，自然键因此相同；派生的版本标识随请求
// 标识换。
func TestABuyRequestResumesIntoTheNextGenerationWithTheSameSources(t *testing.T) {
	sources := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	first, err := domain.SubmitEvaluationRequest(evaluationRequestSpec(t, "EVALREQ-SYN-1", sources))
	if err != nil {
		t.Fatalf("首代：%v", err)
	}
	basis, _ := domain.NewOperatorResumeBasis("SYN-TICKET-1")
	at := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	by := settlementValue(t, domain.NewRequesterReference, "settlement-accounting/evaluation-resumption")
	next, err := first.Resume(settlementValue(t, domain.NewEvaluationRequestID, "EVALREQ-SYN-2"), basis, at, by)
	if err != nil {
		t.Fatalf("续办：%v", err)
	}
	if next.NaturalKey() != first.NaturalKey() || next.Generation() != 2 || !next.RequestedAt().Equal(at) || next.RequestedBy() != by {
		t.Fatalf("下一代 = %+v 第 %d 代", next.NaturalKey(), next.Generation())
	}
	if previous, has := next.Previous(); !has || previous != first.ID() {
		t.Fatal("下一代没指名前一代")
	}
	if next.SupplierExpectedCostVersionID() == first.SupplierExpectedCostVersionID() {
		t.Fatal("两代派生出同一个版本标识")
	}
	if _, err := first.Resume(settlementValue(t, domain.NewEvaluationRequestID, "EVALREQ-SYN-3"), domain.ResumeBasis{}, at, by); err == nil {
		t.Fatal("没有依据的下一代立住了")
	}
}

// Covers: 发起停办只收发起之前、本路的停因；缺来源事实或计价基准时点立不住。
func TestAnInitiationStallTakesOnlyItsOwnBeforeRequestCauses(t *testing.T) {
	stoppedAt := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	sell := domain.SellEvaluationInitiationStallSpec{
		Parcel:         mustRef(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Moment:         domain.BillingMomentNetworkIntakeAdopted,
		Source:         mustRef(t, domain.NewSourceFactReference, "SYN-INTAKE-ADOPTION-1"),
		PricingBasisAt: sellPricingBasisAt,
		Cause:          domain.StopSettlementAccountNotRegistered,
		StoppedAt:      stoppedAt,
	}
	stall, err := domain.NewSellEvaluationInitiationStall(sell)
	if err != nil {
		t.Fatalf("SELL 发起停办：%v", err)
	}
	if stall.NaturalKey().Parcel != sell.Parcel || stall.Cause() != sell.Cause || !stall.StoppedAt().Equal(stoppedAt) {
		t.Fatal("SELL 发起停办没按输入记")
	}
	for _, cause := range []domain.EvaluationStopCause{
		domain.StopPriceCardNotConfigured, domain.StopOccurrenceReasonNotRegistered, domain.EvaluationStopCauseInvalid,
	} {
		broken := sell
		broken.Cause = cause
		if _, err := domain.NewSellEvaluationInitiationStall(broken); !errors.Is(err, domain.ErrInvalidEvaluationResumption) {
			t.Errorf("SELL 发起停办收了 %q", cause.String())
		}
	}
	noSource := sell
	noSource.Source = domain.SourceFactReference{}
	if _, err := domain.NewSellEvaluationInitiationStall(noSource); err == nil {
		t.Fatal("缺来源事实的发起停办立住了")
	}

	buy := domain.BuyEvaluationInitiationStallSpec{
		Occurrence: occurrence(t),
		Agreement:  mustValue(t, domain.NewSupplierAgreementReference, "SYN-AGREEMENT/v1"),
		Cause:      domain.StopTriggerFeeItemNotRegistered,
		StoppedAt:  stoppedAt,
	}
	if _, err := domain.NewBuyEvaluationInitiationStall(buy); err != nil {
		t.Fatalf("BUY 发起停办：%v", err)
	}
	buy.Cause = domain.StopSettlementAccountNotRegistered
	if _, err := domain.NewBuyEvaluationInitiationStall(buy); !errors.Is(err, domain.ErrInvalidEvaluationResumption) {
		t.Fatal("BUY 发起停办收了 SELL 的停因")
	}
}
