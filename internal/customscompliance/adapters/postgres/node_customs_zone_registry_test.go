package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 节点关务区域册的版本代数在真库上走一遍（隔离合成 S）：登记、重放、同起点换区域冲突、换版落终点、错序撞排他
// 约束——与口岸目录同款，但本册的约束与 SQL 是新写的，先例的绿证不了它。穿登记用例走，冲突判定的读回也在内。
// Covers: AT-CC-425。
func TestNodeCustomsZoneRegistryVersionsOnRealPostgres(t *testing.T) {
	fixture := newViewFixture(t)
	registry, err := adapter.NewNodeCustomsZoneRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造区域写口：%v", err)
	}
	view, err := adapter.NewNodeCustomsZonePointView(fixture.db)
	if err != nil {
		t.Fatalf("构造区域读口：%v", err)
	}
	handler := application.NewRegisterNodeCustomsZoneHandler(application.RegisterNodeCustomsZoneDeps{Registry: registry, View: view})
	tenant := viewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	node := viewValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE")
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	registerZone := func(zone string, from time.Time) application.CaseConfigurationOutcome {
		t.Helper()
		var outcome application.CaseConfigurationOutcome
		err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var handleErr error
			outcome, handleErr = handler.Handle(txCtx, application.RegisterNodeCustomsZoneCommand{
				TenantID: tenant, Node: node, Zone: viewValue(t, domain.NewCustomsZoneReference, zone), AppliesFrom: from,
			})
			return handleErr
		})
		if err != nil {
			t.Fatalf("登区域：%v", err)
		}
		return outcome
	}

	for _, step := range []struct {
		zone string
		from time.Time
		want application.CaseConfigurationOutcome
	}{
		{"SYN-ZONE-CN", january, application.ConfigurationRegistered},
		{"SYN-ZONE-CN", january, application.ConfigurationExisting},
		{"SYN-ZONE-SG", january, application.ConfigurationContentConflict},
		{"SYN-ZONE-BONDED", june, application.ConfigurationRegistered},
		// 起点落在已闭合区间里：撞排他约束，读回是起点不同的那一版——冲突，不是重放。
		{"SYN-ZONE-CN", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), application.ConfigurationContentConflict},
	} {
		if got := registerZone(step.zone, step.from); got != step.want {
			t.Fatalf("登 %s@%s = %s，要 %s", step.zone, step.from.Format(time.DateOnly), got, step.want)
		}
	}

	march, found, err := view.LoadNodeCustomsZone(t.Context(), tenant, node, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !found || march.Zone.String() != "SYN-ZONE-CN" || !march.AppliesUntil.Equal(june) {
		t.Fatalf("三月读回 = %+v / %v / %v", march, found, err)
	}
	july, found, err := view.LoadNodeCustomsZone(t.Context(), tenant, node, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !found || july.Zone.String() != "SYN-ZONE-BONDED" || !july.AppliesUntil.IsZero() {
		t.Fatalf("七月读回 = %+v / %v / %v", july, found, err)
	}
	if _, found, err := view.LoadNodeCustomsZone(t.Context(), tenant, node, time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)); err != nil || found {
		t.Fatalf("首版之前该是未登记：%v / %v", found, err)
	}
}
