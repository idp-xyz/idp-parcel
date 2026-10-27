package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// closureCatalogueEcho 是五本目录行上与收尾有关的那几格，抹平五种行类型之后逐本同口径断言。
type closureCatalogueEcho struct {
	label     string
	status    string
	endsAt    time.Time
	hasEnd    bool
	cutAt     time.Time
	hasCut    bool
	retiredAt time.Time
	successor string
}

// Covers: `AT-PC-065` 在真库上的那一半——服务产品、客户合同、授权规则、接受前财务控制策略、客户服务规则五本目录
// 与供应商协议同口径回显收尾（票 demo-chain-gaps/184，ADR-0233 决定五）：按后继截止后旧版仍已生效、回显截止点；同起点退役后
// 回显后继与退役时刻、状态为已退役；两种情形原区间都不被覆盖，没收尾的后继不带收尾。
func TestFiveCommercialCataloguesEchoTheVersionClosure(t *testing.T) {
	list := map[domain.CommercialObjectKind]func(context.Context, *adapter.OperationsCatalogue, domain.TenantID) ([]closureCatalogueEcho, error){
		domain.ServiceProductObject: func(ctx context.Context, c *adapter.OperationsCatalogue, tenant domain.TenantID) ([]closureCatalogueEcho, error) {
			rows, err := c.ListServiceProducts(ctx, tenant, 20)
			echoes := make([]closureCatalogueEcho, 0, len(rows))
			for _, r := range rows {
				echoes = append(echoes, closureCatalogueEcho{r.ObjectID + "/" + r.VersionLabel, r.Status, r.EffectiveEndsAt, r.HasEffectiveEnd,
					r.SelectionEndsAt, r.HasSelectionCut, r.RetiredAt, r.Successor})
			}
			return echoes, err
		},
		domain.CustomerContractObject: func(ctx context.Context, c *adapter.OperationsCatalogue, tenant domain.TenantID) ([]closureCatalogueEcho, error) {
			rows, err := c.ListCustomerContracts(ctx, tenant, 20)
			echoes := make([]closureCatalogueEcho, 0, len(rows))
			for _, r := range rows {
				echoes = append(echoes, closureCatalogueEcho{r.ObjectID + "/" + r.VersionLabel, r.Status, r.EffectiveEndsAt, r.HasEffectiveEnd,
					r.SelectionEndsAt, r.HasSelectionCut, r.RetiredAt, r.Successor})
			}
			return echoes, err
		},
		domain.AuthorizationRuleObject: func(ctx context.Context, c *adapter.OperationsCatalogue, tenant domain.TenantID) ([]closureCatalogueEcho, error) {
			rows, err := c.ListAuthorizationRules(ctx, tenant, 20)
			echoes := make([]closureCatalogueEcho, 0, len(rows))
			for _, r := range rows {
				echoes = append(echoes, closureCatalogueEcho{r.ObjectID + "/" + r.VersionLabel, r.Status, r.EffectiveEndsAt, r.HasEffectiveEnd,
					r.SelectionEndsAt, r.HasSelectionCut, r.RetiredAt, r.Successor})
			}
			return echoes, err
		},
		domain.PreAcceptanceFinancialControlPolicyObject: func(ctx context.Context, c *adapter.OperationsCatalogue, tenant domain.TenantID) ([]closureCatalogueEcho, error) {
			rows, err := c.ListPreAcceptanceFinancialControlPolicies(ctx, tenant, 20)
			echoes := make([]closureCatalogueEcho, 0, len(rows))
			for _, r := range rows {
				echoes = append(echoes, closureCatalogueEcho{r.ObjectID + "/" + r.VersionLabel, r.Status, r.EffectiveEndsAt, r.HasEffectiveEnd,
					r.SelectionEndsAt, r.HasSelectionCut, r.RetiredAt, r.Successor})
			}
			return echoes, err
		},
		domain.CustomerServiceRuleObject: func(ctx context.Context, c *adapter.OperationsCatalogue, tenant domain.TenantID) ([]closureCatalogueEcho, error) {
			rows, err := c.ListCustomerServiceRules(ctx, tenant, 20)
			echoes := make([]closureCatalogueEcho, 0, len(rows))
			for _, r := range rows {
				echoes = append(echoes, closureCatalogueEcho{r.ObjectID + "/" + r.VersionLabel, r.Status, r.EffectiveEndsAt, r.HasEffectiveEnd,
					r.SelectionEndsAt, r.HasSelectionCut, r.RetiredAt, r.Successor})
			}
			return echoes, err
		},
	}

	for kind, listKind := range list {
		t.Run(kind.String(), func(t *testing.T) {
			fixture := newCutFixture(t)
			ctx := t.Context()
			// 对象 cut-1：v1 被 v2 按后继截止；对象 retire-1：v1 与 v2 同起点，v1 让位给 v2 退役。
			cutOld := cutVersion(t, kind, "cut-1", "v1", cutRowPredecessorStart)
			retireOld := cutVersion(t, kind, "retire-1", "v1", cutRowPredecessorStart)
			for _, version := range []domain.CommercialVersion{
				cutOld,
				cutVersion(t, kind, "cut-1", "v2", cutRowSuccessorStart),
				retireOld,
				cutVersion(t, kind, "retire-1", "v2", cutRowPredecessorStart),
			} {
				fixture.save(t, version)
			}
			if result := fixture.cut(t, cutCommand(t, cutOld, "v2", "SYN-CUT-CATALOGUE")); result.Outcome() != application.VersionClosureRegistered {
				t.Fatalf("cut outcome = %q (cause %v)", result.Outcome(), result.Cause())
			}
			if result := fixture.retire(t, retireCommand(t, retireOld, "v2", "SYN-RETIRE-CATALOGUE")); result.Outcome() != application.VersionClosureRegistered {
				t.Fatalf("retire outcome = %q (cause %v)", result.Outcome(), result.Cause())
			}

			catalogue, err := adapter.NewOperationsCatalogue(fixture.db)
			if err != nil {
				t.Fatal(err)
			}
			echoes, err := listKind(ctx, catalogue, cutOld.Tenant())
			if err != nil {
				t.Fatalf("目录上列：%v", err)
			}
			byLabel := map[string]closureCatalogueEcho{}
			for _, echo := range echoes {
				byLabel[echo.label] = echo
			}
			effective, retired := domain.CommercialVersionEffective.String(), domain.CommercialVersionRetired.String()

			cut := byLabel["cut-1/v1"]
			if cut.status != effective || !cut.hasCut || !cut.cutAt.Equal(cutRowSuccessorStart) || cut.successor != "" || !cut.retiredAt.IsZero() {
				t.Fatalf("目录 cut-1/v1 = %+v, want 已生效且截止于 v2 的生效时点", cut)
			}
			gone := byLabel["retire-1/v1"]
			if gone.status != retired || gone.successor != "v2" || !gone.retiredAt.Equal(cutRowRegisteredAt) || gone.hasCut {
				t.Fatalf("目录 retire-1/v1 = %+v, want 已退役、让位给 v2", gone)
			}
			for _, echo := range []closureCatalogueEcho{cut, gone} {
				if !echo.hasEnd || !echo.endsAt.Equal(cutRowEnd) {
					t.Fatalf("目录 %s 的原区间终点被收尾覆盖：%+v", echo.label, echo)
				}
			}
			for _, label := range []string{"cut-1/v2", "retire-1/v2"} {
				if echo := byLabel[label]; echo.status != effective || echo.hasCut || echo.successor != "" {
					t.Fatalf("目录 %s = %+v, want 已生效、不带收尾", label, echo)
				}
			}
		})
	}
}
