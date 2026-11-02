package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: 按网点出日期的窄读口（ADR-0355 决定七；票 document-numbering/08）——选版口径与快照的节点族同一：生效当刻属新版、
// 前版按新版生效时间闭合；没登记的节点、生效之前、别的租户都答 found=false，不拿别的版本顶；两版同时适用答
// ErrAmbiguousNetworkCatalog，不挑一版。
func TestNodeBusinessTimezoneIsReadFromTheVersionEffectiveAtTheInstant(t *testing.T) {
	catalog, transactor, pool := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")
	other := scalar(t, domain.NewTenantID, "tenant-2")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogAsOf.Add(-48 * time.Hour),
		})
	})
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-hub", Version: 2, BusinessTimezone: "Europe/Berlin", EffectiveFrom: catalogAsOf,
		})
	})

	for name, probe := range map[string]struct {
		tenant domain.TenantID
		node   string
		at     time.Time
		zone   string
		found  bool
	}{
		"前版":      {tenant, "node-hub", catalogAsOf.Add(-time.Second), "Asia/Shanghai", true},
		"生效当刻属新版": {tenant, "node-hub", catalogAsOf, "Europe/Berlin", true},
		"生效之前":    {tenant, "node-hub", catalogAsOf.Add(-72 * time.Hour), "", false},
		"没登记的节点":  {tenant, "node-gate", catalogAsOf, "", false},
		"别的租户":    {other, "node-hub", catalogAsOf, "", false},
	} {
		zone, found, err := catalog.FindNodeBusinessTimezoneAt(ctx, probe.tenant, probe.node, probe.at)
		if err != nil || found != probe.found || zone != probe.zone {
			t.Fatalf("%s：%q %v %v，want %q %v", name, zone, found, err, probe.zone, probe.found)
		}
	}

	for version, from := range map[int]time.Time{3: catalogAsOf.Add(-96 * time.Hour), 4: catalogAsOf.Add(-120 * time.Hour)} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO network_routing.logistics_node_version
				(tenant_id, node_code, version, business_timezone, effective_from, effective_to)
			 VALUES ($1, 'node-overlap', $2, 'Asia/Shanghai', $3, $4)`,
			tenant.String(), version, from, catalogAsOf.Add(time.Hour),
		); err != nil {
			t.Fatalf("植入重叠版本 %d：%v", version, err)
		}
	}
	if _, _, err := catalog.FindNodeBusinessTimezoneAt(ctx, tenant, "node-overlap", catalogAsOf); !errors.Is(err, adapter.ErrAmbiguousNetworkCatalog) {
		t.Fatalf("两版同时适用应交回 ErrAmbiguousNetworkCatalog，实得：%v", err)
	}
	if _, _, err := catalog.FindNodeBusinessTimezoneAt(ctx, tenant, " ", catalogAsOf); err == nil {
		t.Fatal("空白节点被收下")
	}
}
