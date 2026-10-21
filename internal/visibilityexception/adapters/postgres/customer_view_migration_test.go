package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: 客户视图版本表迁移的存量半边——迁移之前落下的视图行各自成为版本表里的一版并立为当前，四维、投影锚、发布
// 时间原样；曾经是替代版的那一行前版指回原样带过来，不抹成首版（抹掉等于改写已发布的替代关系）。旧表随之退场。
// 从空库按序施加本上下文的迁移，停在版本表那一份之前落旧形状的行，再施加那一份。
func TestTheCustomerViewVersionMigrationCarriesExistingViewsAsTheirCurrentVersions(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA visibility_exception`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.VisibilityException()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0028_customer_view_versions"
	var versions *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			versions = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if versions == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.customer_view
		(tenant_id, customer_account_id, parcel_id, version, based_on_projection, prior_version, published_at,
		 milestones_state, milestones_content, eta_state, eta_content, final_state, final_content,
		 note_state, note_content)
		VALUES ('SYN-TENANT-CV28', 'SYN-ACCOUNT-CV28', 'SYN-PARCEL-FIRST', 'SYN-VIEW-FIRST', 'SYN-PROJ-1', NULL,
		        '2026-10-01T08:00:00Z', 'SHOWN', 'SYN-CONTENT-MILESTONES', 'PENDING_CONFIRMATION', NULL,
		        'NOT_DISCLOSED', NULL, 'PENDING_CONFIRMATION', NULL),
		       ('SYN-TENANT-CV28', 'SYN-ACCOUNT-CV28', 'SYN-PARCEL-SUPERSEDED', 'SYN-VIEW-SECOND', 'SYN-PROJ-2',
		        'SYN-VIEW-OVERWRITTEN', '2026-10-02T08:00:00Z', 'SHOWN', 'SYN-CONTENT-MILESTONES',
		        'PENDING_CONFIRMATION', NULL, 'SHOWN', 'SYN-CONTENT-FINAL', 'PENDING_CONFIRMATION', NULL)`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, versions.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}

	for _, want := range []struct {
		parcel, version, basedOn string
		prior                    *string
		finalState               string
	}{
		{"SYN-PARCEL-FIRST", "SYN-VIEW-FIRST", "SYN-PROJ-1", nil, "NOT_DISCLOSED"},
		{"SYN-PARCEL-SUPERSEDED", "SYN-VIEW-SECOND", "SYN-PROJ-2", stringRef("SYN-VIEW-OVERWRITTEN"), "SHOWN"},
	} {
		var currentVersion, currentBasis, formedBasis, finalState string
		var prior *string
		if err := conn.QueryRow(ctx,
			`SELECT c.version, c.based_on_projection, v.based_on_projection, v.prior_version, v.final_state
			   FROM visibility_exception.customer_view_current AS c
			   JOIN visibility_exception.customer_view_version AS v
			     ON v.tenant_id = c.tenant_id AND v.customer_account_id = c.customer_account_id
			    AND v.parcel_id = c.parcel_id AND v.version = c.version
			  WHERE c.tenant_id = 'SYN-TENANT-CV28' AND c.customer_account_id = 'SYN-ACCOUNT-CV28'
			    AND c.parcel_id = $1`, want.parcel,
		).Scan(&currentVersion, &currentBasis, &formedBasis, &prior, &finalState); err != nil {
			t.Fatalf("%s：读回迁入的当前版：%v", want.parcel, err)
		}
		if currentVersion != want.version || currentBasis != want.basedOn || formedBasis != want.basedOn {
			t.Errorf("%s：当前版 = %q 标记投影锚 = %q 版本行投影锚 = %q，应为 %q / %q",
				want.parcel, currentVersion, currentBasis, formedBasis, want.version, want.basedOn)
		}
		if (prior == nil) != (want.prior == nil) || (prior != nil && *prior != *want.prior) {
			t.Errorf("%s：前版指回 = %v，应原样为 %v", want.parcel, prior, want.prior)
		}
		if finalState != want.finalState {
			t.Errorf("%s：终局一维 = %q，应原样为 %q", want.parcel, finalState, want.finalState)
		}
	}

	var legacy *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('visibility_exception.customer_view')::text`).Scan(&legacy); err != nil {
		t.Fatalf("查旧表：%v", err)
	}
	if legacy != nil {
		t.Errorf("旧表 %s 仍在：当前版只能由一处指名", *legacy)
	}
}

func stringRef(value string) *string {
	return &value
}
