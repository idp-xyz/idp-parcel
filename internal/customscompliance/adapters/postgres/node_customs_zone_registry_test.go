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

// 「租户登没登过区域」按评估时点在真库上读（隔离合成 S，ADR-0300）：首版起点之前答没登，之后答登了，换版不改这一答；
// 别的租户登的不算。区间判法与按节点读的那一句同一，零租户与零时点是编程错误，不查成「没登」。
// Covers: AT-CC-426、AT-CC-427 的读口半边。
func TestAnyNodeCustomsZoneInForceReadsTheTenantsRegisterAtTheInstant(t *testing.T) {
	fixture := newViewFixture(t)
	registry, err := adapter.NewNodeCustomsZoneRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造区域写口：%v", err)
	}
	view, err := adapter.NewNodeCustomsZonePointView(fixture.db)
	if err != nil {
		t.Fatalf("构造区域读口：%v", err)
	}
	tenant := viewValue(t, domain.NewTenantID, "SYN-TENANT-01")
	other := viewValue(t, domain.NewTenantID, "SYN-TENANT-02")
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, from := range []time.Time{january, june} {
		if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, err := registry.RegisterNodeCustomsZone(txCtx, tenant, viewValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
				viewValue(t, domain.NewCustomsZoneReference, "SYN-ZONE-CN"), from)
			return err
		}); err != nil {
			t.Fatalf("登区域：%v", err)
		}
	}

	for _, probe := range []struct {
		tenant domain.TenantID
		at     time.Time
		want   bool
	}{
		{tenant, january.Add(-time.Microsecond), false},
		{tenant, january, true},
		{tenant, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), true},
		{tenant, june.Add(24 * time.Hour), true},
		{other, june.Add(24 * time.Hour), false},
	} {
		got, err := view.AnyNodeCustomsZoneInForce(t.Context(), probe.tenant, probe.at)
		if err != nil || got != probe.want {
			t.Fatalf("%s@%s = %v / %v，要 %v", probe.tenant, probe.at.Format(time.RFC3339Nano), got, err, probe.want)
		}
	}
	if _, err := view.AnyNodeCustomsZoneInForce(t.Context(), tenant, time.Time{}); err == nil {
		t.Fatal("零时点要报编程错误")
	}
	if _, err := view.AnyNodeCustomsZoneInForce(t.Context(), domain.TenantID{}, june); err == nil {
		t.Fatal("空租户要报编程错误")
	}
}
