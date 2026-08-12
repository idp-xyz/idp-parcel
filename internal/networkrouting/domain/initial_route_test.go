package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

var handoffAcceptedAt = time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)

func routeHandoffSpec(t *testing.T, parcels ...string) domain.RouteHandoffSpec {
	t.Helper()
	declared := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, mustValue(t, domain.NewDeclaredParcelID, parcel))
	}
	return domain.RouteHandoffSpec{
		Correlation:        mustValue(t, domain.NewRequestCorrelationID, "handoff-1"),
		TenantID:           mustValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:  mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  mustValue(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceDecision: mustValue(t, domain.NewAcceptanceDecisionReference, "decision-1"),
		AcceptanceBaseline: mustValue(t, domain.NewAcceptanceBaselineReference, "baseline-1/v1"),
		Parcels:            declared,
		AcceptedAt:         handoffAcceptedAt,
	}
}

// Covers: UC-NR-001 步骤 4「按接受基线逐包裹建立独立判断范围」与一致性硬句「同一接受
// 基线、同一包裹和同一初始路由目的只能形成一个当前有效初始路由结果」的身份半边——键含
// 基线与目的、不含判断时点，同键即同一判断、异包裹即两个独立范围。
func TestARouteHandoffDerivesOneJudgmentKeyPerParcel(t *testing.T) {
	handoff, err := domain.NewRouteHandoff(routeHandoffSpec(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("new route handoff: %v", err)
	}

	keys, err := handoff.JudgmentKeys(mustValue(t, domain.NewServicePurpose, "NETWORK_SERVICE"))
	if err != nil {
		t.Fatalf("judgment keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %d, want one per parcel", len(keys))
	}
	for _, key := range keys {
		if !key.MinimumIdentityEstablished() {
			t.Fatalf("key %#v 身份不成立", key)
		}
		if key.AcceptanceBaseline.String() != "baseline-1/v1" {
			t.Fatalf("baseline = %s; 判断身份必须锚在基线版本上", key.AcceptanceBaseline)
		}
	}
	if keys[0].SameJudgmentScope(keys[1]) {
		t.Fatal("两个包裹的判断范围混成了一个——委托级统一路线正是被禁的")
	}
	if !keys[0].SameJudgmentScope(keys[0]) {
		t.Fatal("同一范围不自等")
	}

	if _, err := handoff.JudgmentKeys(domain.ServicePurpose{}); !errors.Is(err, domain.ErrInvalidRouteHandoff) {
		t.Fatalf("err = %v; 服务目的未配置不得替产品挑一个", err)
	}
}

// Covers: UC-NR-001 启动条件「路由交接能够关联接受决定、接受基线版本以及其中的明确包裹
// 身份；只有状态字符串而没有基线引用时不得继续」——缺任何一件在构造期就死；成员空或
// 重复是装配错误。
func TestARouteHandoffRefusesToProceedWithoutItsAnchors(t *testing.T) {
	cases := map[string]func(domain.RouteHandoffSpec) domain.RouteHandoffSpec{
		"no acceptance baseline": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.AcceptanceBaseline = domain.AcceptanceBaselineReference{}
			return spec
		},
		"no acceptance decision": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.AcceptanceDecision = domain.AcceptanceDecisionReference{}
			return spec
		},
		"no correlation": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.Correlation = domain.RequestCorrelationID{}
			return spec
		},
		"no parcels": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.Parcels = nil
			return spec
		},
		"duplicated parcel": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.Parcels = append(spec.Parcels, spec.Parcels[0])
			return spec
		},
		"no accepted time": func(spec domain.RouteHandoffSpec) domain.RouteHandoffSpec {
			spec.AcceptedAt = time.Time{}
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewRouteHandoff(mutate(routeHandoffSpec(t, "parcel-1"))); !errors.Is(err, domain.ErrInvalidRouteHandoff) {
				t.Fatalf("err = %v, want ErrInvalidRouteHandoff", err)
			}
		})
	}
}
