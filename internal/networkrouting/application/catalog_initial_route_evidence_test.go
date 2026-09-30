package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

func catalogInitialRouteKey(t *testing.T) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           value(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:  value(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, domain.NewAcceptanceBaselineReference, "SYN-BASELINE-1"),
		DeclaredParcelID:   value(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		ServicePurpose:     value(t, domain.NewServicePurpose, catalogPurpose),
	}
}

// Covers: ADR-0148 决定六的初始路由半边——`未配置`与可达性一侧同一条规则。目录已配置时折出除成本外的事实；
// 没登记节点日历不补默认投影。判断时点取时钟。
func TestTheCatalogInitialRouteEvidenceDecidesUnconfiguredAndOtherwiseStaysLoud(t *testing.T) {
	noStrategy := syntheticCatalog(t)
	noStrategy.snapshot.Strategies = nil
	for name, catalog := range map[string]*catalogReadDouble{"目录修订锚不存在": {}, "没有适用策略": noStrategy} {
		t.Run(name, func(t *testing.T) {
			view, err := application.NewCatalogInitialRouteEvidence(catalog, fixedClock{at: judgedAt})
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			_, configured, err := view.LoadInitialRouteEvidence(t.Context(), catalogInitialRouteKey(t), ports.RequestCarriedContent{})
			if err != nil || configured {
				t.Fatalf("configured=%v err=%v，想要未配置", configured, err)
			}
			if catalog.reads != 1 || !catalog.gotAsOf.Equal(judgedAt) {
				t.Fatalf("选版时点 = %s（读 %d 次），想要时钟给的路由判断时点", catalog.gotAsOf, catalog.reads)
			}
		})
	}

	view, err := application.NewCatalogInitialRouteEvidence(syntheticCatalog(t), fixedClock{at: judgedAt})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(t.Context(), catalogInitialRouteKey(t), ports.RequestCarriedContent{})
	if err != nil || !configured || !evidence.Strategy.Valid() {
		t.Fatalf("configured=%v err=%v strategy=%s，目录已配置时应折出除成本外的事实", configured, err, evidence.Strategy)
	}
	if len(evidence.Projections) != 0 {
		t.Fatal("没登记节点日历却折出了时间投影")
	}

	if _, err := application.NewCatalogInitialRouteEvidence(nil, fixedClock{at: judgedAt}); err == nil {
		t.Fatal("nil 目录读口应在构造期被拒")
	}
	if _, err := application.NewCatalogInitialRouteEvidence(syntheticCatalog(t), nil); err == nil {
		t.Fatal("nil 时钟应在构造期被拒")
	}
}
