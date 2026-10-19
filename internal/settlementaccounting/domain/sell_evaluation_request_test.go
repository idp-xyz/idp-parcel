package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉 ADR-0190 决定一、二的领域半边：包裹计费时点是封闭的两格，词表外与缺席都拒；SELL 评价请求各格
// 缺一不成，自然键只取（包裹、计费时点），换账户、换币种、换计价范围或换来源事实不改自然键。夹具全是合成串。

var sellPricingBasisAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))

func sellEvaluationRequestSpec(t *testing.T) domain.SellEvaluationRequestSpec {
	t.Helper()
	return domain.SellEvaluationRequestSpec{
		ID:                 mustRef(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-1"),
		Parcel:             mustRef(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Account:            mustRef(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		SettlementCurrency: mustRef(t, domain.NewCurrencyCode, "XTS"),
		PricingScope:       mustRef(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
		Moment:             domain.BillingMomentNetworkIntakeAdopted,
		Source:             mustRef(t, domain.NewSourceFactReference, "SYN-INTAKE-ADOPTION-1"),
		PricingBasisAt:     sellPricingBasisAt,
	}
}

func TestTheParcelBillingMomentIsAClosedPairOfBuiltInMoments(t *testing.T) {
	for name, want := range map[string]domain.ParcelBillingMoment{
		"NETWORK_INTAKE_ADOPTED":       domain.BillingMomentNetworkIntakeAdopted,
		"EFFECTIVE_DELIVERY_FINALIZED": domain.BillingMomentEffectiveDeliveryFinalized,
	} {
		got, err := domain.ParcelBillingMomentFromName(name)
		if err != nil || got != want || got.String() != name {
			t.Fatalf("%s → %v（%q）err = %v", name, got, got.String(), err)
		}
	}
	// 「委托已接受」不在内置集合里（ADR-0190 决定二），小写与空白同样不是这两个词。
	for _, name := range []string{"", "COMMISSION_ACCEPTED", "OCCURRENCE_FORMED", "network_intake_adopted"} {
		if _, err := domain.ParcelBillingMomentFromName(name); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("计费时点 %q err = %v，想要拒", name, err)
		}
	}
	if domain.ParcelBillingMomentInvalid.String() != "" {
		t.Fatal("零值计费时点有了名字")
	}
}

func TestASellEvaluationRequestCarriesEveryCellAndKeysOnParcelAndMoment(t *testing.T) {
	spec := sellEvaluationRequestSpec(t)
	request, err := domain.SubmitSellEvaluationRequest(spec)
	if err != nil {
		t.Fatalf("形成 SELL 评价请求：%v", err)
	}
	if request.ID() != spec.ID || request.Parcel() != spec.Parcel || request.Account() != spec.Account ||
		request.SettlementCurrency() != spec.SettlementCurrency || request.PricingScope() != spec.PricingScope || request.Moment() != spec.Moment || request.Source() != spec.Source {
		t.Fatalf("请求没有原样保存各格：%+v", request)
	}
	if !request.PricingBasisAt().Equal(sellPricingBasisAt) || request.PricingBasisAt().Location() != time.UTC {
		t.Fatalf("计价基准时点 = %v，想要同一时刻的 UTC 形", request.PricingBasisAt())
	}
	if key := request.NaturalKey(); key.Parcel != spec.Parcel || key.Moment != spec.Moment {
		t.Fatalf("自然键 = %+v，想要（包裹、计费时点）", key)
	}

	// 自然键只守幂等：换账户、换币种、换计价范围、换来源事实、换铸造 ID，仍是同一件包裹在同一时点的那一份。
	other := spec
	other.ID = mustRef(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2")
	other.Account = mustRef(t, domain.NewSettlementAccountID, "SYN-ACCT-2")
	other.SettlementCurrency = mustRef(t, domain.NewCurrencyCode, "XXX")
	other.PricingScope = mustRef(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-2")
	other.Source = mustRef(t, domain.NewSourceFactReference, "SYN-INTAKE-ADOPTION-2")
	otherRequest, err := domain.SubmitSellEvaluationRequest(other)
	if err != nil {
		t.Fatalf("另一份：%v", err)
	}
	if otherRequest.NaturalKey() != request.NaturalKey() {
		t.Fatalf("成分外的格改动了自然键：%+v vs %+v", otherRequest.NaturalKey(), request.NaturalKey())
	}

	// 换时点或换包裹就是另一个自然键。
	delivered := spec
	delivered.Moment = domain.BillingMomentEffectiveDeliveryFinalized
	deliveredRequest, err := domain.SubmitSellEvaluationRequest(delivered)
	if err != nil {
		t.Fatalf("终局时点那一份：%v", err)
	}
	anotherParcel := spec
	anotherParcel.Parcel = mustRef(t, domain.NewParcelReference, "SYN-PARCEL-2")
	anotherParcelRequest, err := domain.SubmitSellEvaluationRequest(anotherParcel)
	if err != nil {
		t.Fatalf("另一件包裹：%v", err)
	}
	if deliveredRequest.NaturalKey() == request.NaturalKey() || anotherParcelRequest.NaturalKey() == request.NaturalKey() {
		t.Fatal("换时点或换包裹仍算出同一个自然键")
	}
}

func TestASellEvaluationRequestMissingAnyCellIsNotFormed(t *testing.T) {
	cases := map[string]func(*domain.SellEvaluationRequestSpec){
		"缺铸造 ID":   func(spec *domain.SellEvaluationRequestSpec) { spec.ID = domain.SellEvaluationRequestID{} },
		"缺包裹":      func(spec *domain.SellEvaluationRequestSpec) { spec.Parcel = domain.ParcelReference{} },
		"缺结算账户":    func(spec *domain.SellEvaluationRequestSpec) { spec.Account = domain.SettlementAccountID{} },
		"缺结算币种":    func(spec *domain.SellEvaluationRequestSpec) { spec.SettlementCurrency = domain.CurrencyCode{} },
		"缺计价范围":    func(spec *domain.SellEvaluationRequestSpec) { spec.PricingScope = domain.EstimatePricingScope{} },
		"缺计费时点":    func(spec *domain.SellEvaluationRequestSpec) { spec.Moment = domain.ParcelBillingMomentInvalid },
		"词表外的计费时点": func(spec *domain.SellEvaluationRequestSpec) { spec.Moment = domain.ParcelBillingMoment(99) },
		"缺来源事实":    func(spec *domain.SellEvaluationRequestSpec) { spec.Source = domain.SourceFactReference{} },
		"缺计价基准时点":  func(spec *domain.SellEvaluationRequestSpec) { spec.PricingBasisAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := sellEvaluationRequestSpec(t)
			mutate(&spec)
			if _, err := domain.SubmitSellEvaluationRequest(spec); !errors.Is(err, domain.ErrInvalidSellEvaluationRequest) {
				t.Fatalf("err = %v，想要 ErrInvalidSellEvaluationRequest", err)
			}
		})
	}
	for _, blank := range []string{"", "  "} {
		if _, err := domain.NewSellEvaluationRequestID(blank); err == nil {
			t.Fatalf("空白请求 ID %q 被收下了", blank)
		}
		if _, err := domain.NewParcelReference(blank); err == nil {
			t.Fatalf("空白包裹 %q 被收下了", blank)
		}
	}
}

// Covers: ADR-0190 决定五的状态格——登记时`已提交`，其余各格由形成那一半记；只有`已形成`带费用标识；词表外的拒。
func TestTheSellEvaluationRequestStateIsAClosedSetWhereOnlyFormedCarriesACharge(t *testing.T) {
	for name, want := range map[string]domain.SellEvaluationRequestState{
		"SUBMITTED":        domain.SellRequestSubmitted,
		"FORMED":           domain.SellRequestFormed,
		"PENDING_JUDGMENT": domain.SellRequestPendingJudgment,
		"UNRATABLE":        domain.SellRequestUnratable,
		"CONFLICT":         domain.SellRequestConflict,
	} {
		got, err := domain.SellEvaluationRequestStateFromName(name)
		if err != nil || got != want || got.String() != name {
			t.Fatalf("%s → %v（%q）err = %v", name, got, got.String(), err)
		}
		if got.RecordsFormedCharge() != (want == domain.SellRequestFormed) {
			t.Fatalf("%s 带费用标识 = %v，只有已形成该带", name, got.RecordsFormedCharge())
		}
	}
	for _, name := range []string{"", "NOT_FORMED", "formed", "COMPLETED"} {
		if _, err := domain.SellEvaluationRequestStateFromName(name); !errors.Is(err, domain.ErrInvalidSellEvaluationRequest) {
			t.Fatalf("状态 %q err = %v，想要拒", name, err)
		}
	}
}
