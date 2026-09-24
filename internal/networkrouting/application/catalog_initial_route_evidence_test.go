package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
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

// Covers: ADR-0148 决定六的初始路由半边——`未配置`与可达性一侧同一条规则（目录修订锚、适用于服务目的的路由策略
// 版本），不再读定义登记册（0007）；判断时点是本上下文的路由判断时点（取时钟）。目录已配置而初始路由事实族的
// 折叠还没落（时间投影、段链、成本归 routing-first-cut/09、10），照旧响亮上抛，不退成`未配置`也不退成空证据。
func TestTheCatalogInitialRouteEvidenceDecidesUnconfiguredAndOtherwiseStaysLoud(t *testing.T) {
	noStrategy := syntheticCatalog(t)
	noStrategy.snapshot.Strategies = nil
	for name, catalog := range map[string]*catalogReadDouble{"目录修订锚不存在": {}, "没有适用策略": noStrategy} {
		t.Run(name, func(t *testing.T) {
			view, err := application.NewCatalogInitialRouteEvidence(catalog, fixedClock{at: judgedAt})
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			_, configured, err := view.LoadInitialRouteEvidence(t.Context(), catalogInitialRouteKey(t))
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
	_, configured, err := view.LoadInitialRouteEvidence(t.Context(), catalogInitialRouteKey(t))
	if !errors.Is(err, application.ErrInitialRouteEvidenceUnresolvable) || configured {
		t.Fatalf("configured=%v err=%v，想要 ErrInitialRouteEvidenceUnresolvable", configured, err)
	}

	if _, err := application.NewCatalogInitialRouteEvidence(nil, fixedClock{at: judgedAt}); err == nil {
		t.Fatal("nil 目录读口应在构造期被拒")
	}
	if _, err := application.NewCatalogInitialRouteEvidence(syntheticCatalog(t), nil); err == nil {
		t.Fatal("nil 时钟应在构造期被拒")
	}
}
