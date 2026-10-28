package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func dispositionSpec(t *testing.T, direction domain.PostIntakeDispositionDirection) domain.PostIntakeDispositionSpec {
	t.Helper()
	return domain.PostIntakeDispositionSpec{
		Tenant:              mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		Parcel:              mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		Version:             1,
		Direction:           direction,
		Source:              domain.PostIntakeOperatorInitiated,
		ResponsibilityStart: mustValue(t, domain.NewSourceResultVersion, "intake-result/v1"),
		Decider:             mustValue(t, domain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#alice"),
		Basis:               mustValue(t, domain.NewPostIntakeDispositionBasis, "SYN-BASIS"),
		DecidedAt:           time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC),
	}
}

// Covers: AT-PS-176；ADR-0332 决定三——去向是封闭集合，每个去向对各所有者的范围化请求是封闭表：继续不请求任何人；拦截请求节点与运输；改送请求路由与运输；
// 退运请求路由、节点、运输；服务终止请求路由释放、节点与运输扣住；任何一项里都没有结算，也没有直接改写所有者对象的动作。
func TestEachDirectionAsksOnlyItsOwnersForScopedActions(t *testing.T) {
	want := map[domain.PostIntakeDispositionDirection][]string{
		domain.PostIntakeContinue:         {},
		domain.PostIntakeIntercept:        {"NODE_OPERATIONS/HOLD_AT_CONTROL", "TRANSPORT_FULFILLMENT/HOLD_ONWARD_MOVEMENT"},
		domain.PostIntakeRedirect:         {"NETWORK_ROUTING/REPLAN_TO_ALTERNATE_DESTINATION", "TRANSPORT_FULFILLMENT/START_ALTERNATE_JOURNEY"},
		domain.PostIntakeReturn:           {"NETWORK_ROUTING/PLAN_RETURN_ROUTE", "NODE_OPERATIONS/LOAD_FOR_RETURN", "TRANSPORT_FULFILLMENT/START_RETURN_JOURNEY"},
		domain.PostIntakeTerminateService: {"NETWORK_ROUTING/RELEASE_UNCONSUMED_PLAN", "NODE_OPERATIONS/HOLD_AT_CONTROL", "TRANSPORT_FULFILLMENT/HOLD_ONWARD_MOVEMENT"},
	}
	for direction, expected := range want {
		var got []string
		for _, request := range direction.OwnerRequests() {
			got = append(got, request.Owner.String()+"/"+request.Action.String())
		}
		if len(got) != len(expected) {
			t.Fatalf("%s 的所有者请求 = %v, want %v", direction, got, expected)
		}
		for index := range expected {
			if got[index] != expected[index] {
				t.Fatalf("%s 的所有者请求 = %v, want %v", direction, got, expected)
			}
		}
	}
	if _, err := domain.ParsePostIntakeDispositionDirection("DESTROY"); err == nil {
		t.Fatal("封闭集合之外的去向被认下")
	}
	if domain.PostIntakeRedirect.OriginalJourneyRelation() != domain.OriginalJourneyAssociatedIndependent ||
		domain.PostIntakeReturn.OriginalJourneyRelation() != domain.OriginalJourneyAssociatedIndependent ||
		domain.PostIntakeIntercept.OriginalJourneyRelation() != domain.OriginalJourneyUnchanged {
		t.Fatal("只有改送与退运另起关联但独立的旅程，其余去向原旅程不变")
	}
}

// Covers: AT-PS-176；ADR-0332 决定一——决定的立得住条件：改送必带目的地、别的去向不带；客户请求来源必须指名请求、运营发起必须不指名；决定人与依据缺一不立；
// 版本接得上前一版（首份取代 0，其余恰好前一版）。
func TestADispositionNeedsItsShapeSourceAndChain(t *testing.T) {
	good := dispositionSpec(t, domain.PostIntakeIntercept)
	if _, err := domain.DecidePostIntakeDisposition(good); err != nil {
		t.Fatalf("合格的决定立不起来：%v", err)
	}
	for name, mutate := range map[string]func(*domain.PostIntakeDispositionSpec){
		"改送缺目的地": func(s *domain.PostIntakeDispositionSpec) { s.Direction = domain.PostIntakeRedirect },
		"拦截带目的地": func(s *domain.PostIntakeDispositionSpec) {
			s.Destination = mustValue(t, domain.NewPostIntakeDispositionDestination, "SYN-PLACE")
		},
		"客户请求不指名请求": func(s *domain.PostIntakeDispositionSpec) { s.Source = domain.PostIntakeCustomerRequest },
		"运营发起指名请求": func(s *domain.PostIntakeDispositionSpec) {
			s.RequestKey = mustValue(t, domain.NewSourceRequestKey, "k")
		},
		"没有决定人": func(s *domain.PostIntakeDispositionSpec) { s.Decider = domain.PostIntakeDispositionDeciderReference{} },
		"没有依据":  func(s *domain.PostIntakeDispositionSpec) { s.Basis = domain.PostIntakeDispositionBasis{} },
		"版本接不上": func(s *domain.PostIntakeDispositionSpec) { s.Version, s.Supersedes = 3, 1 },
		"没有去向":  func(s *domain.PostIntakeDispositionSpec) { s.Direction = domain.PostIntakeDispositionDirectionInvalid },
	} {
		spec := good
		mutate(&spec)
		if _, err := domain.DecidePostIntakeDisposition(spec); !errors.Is(err, domain.ErrInvalidPostIntakeDisposition) {
			t.Errorf("%s：err = %v, want ErrInvalidPostIntakeDisposition", name, err)
		}
	}
}
