package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: 客户视图版本表迁移的存量半边——迁移之前落下的视图行各自成为版本表里的一版并立为当前，四维、投影锚、发布
// 时间原样；曾经是替代版的那一行前版指回原样带过来，不抹成首版（抹掉等于改写已发布的替代关系）。旧表随之退场，
// 当前标记只能指存下来的版本。
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

	type dims struct {
		milestonesState, etaState, finalState, noteState         string
		milestonesContent, etaContent, finalContent, noteContent *string
	}
	for _, want := range []struct {
		parcel, version, basedOn string
		prior                    *string
		publishedAt              string
		dims                     dims
	}{
		{"SYN-PARCEL-FIRST", "SYN-VIEW-FIRST", "SYN-PROJ-1", nil, "2026-10-01T08:00:00Z", dims{
			"SHOWN", "PENDING_CONFIRMATION", "NOT_DISCLOSED", "PENDING_CONFIRMATION",
			stringRef("SYN-CONTENT-MILESTONES"), nil, nil, nil}},
		{"SYN-PARCEL-SUPERSEDED", "SYN-VIEW-SECOND", "SYN-PROJ-2", stringRef("SYN-VIEW-OVERWRITTEN"),
			"2026-10-02T08:00:00Z", dims{
				"SHOWN", "PENDING_CONFIRMATION", "SHOWN", "PENDING_CONFIRMATION",
				stringRef("SYN-CONTENT-MILESTONES"), nil, stringRef("SYN-CONTENT-FINAL"), nil}},
	} {
		var currentVersion, currentBasis, formedBasis, publishedAt string
		var prior *string
		var got dims
		if err := conn.QueryRow(ctx,
			`SELECT c.version, c.based_on_projection, v.based_on_projection, v.prior_version,
			        to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			        v.milestones_state, v.eta_state, v.final_state, v.note_state,
			        v.milestones_content, v.eta_content, v.final_content, v.note_content
			   FROM visibility_exception.customer_view_current AS c
			   JOIN visibility_exception.customer_view_version AS v
			     ON v.tenant_id = c.tenant_id AND v.customer_account_id = c.customer_account_id
			    AND v.parcel_id = c.parcel_id AND v.version = c.version
			  WHERE c.tenant_id = 'SYN-TENANT-CV28' AND c.customer_account_id = 'SYN-ACCOUNT-CV28'
			    AND c.parcel_id = $1`, want.parcel,
		).Scan(&currentVersion, &currentBasis, &formedBasis, &prior, &publishedAt,
			&got.milestonesState, &got.etaState, &got.finalState, &got.noteState,
			&got.milestonesContent, &got.etaContent, &got.finalContent, &got.noteContent); err != nil {
			t.Fatalf("%s：读回迁入的当前版：%v", want.parcel, err)
		}
		if currentVersion != want.version || currentBasis != want.basedOn || formedBasis != want.basedOn {
			t.Errorf("%s：当前版 = %q 标记依据 = %q 版本行依据 = %q，应为 %q / %q",
				want.parcel, currentVersion, currentBasis, formedBasis, want.version, want.basedOn)
		}
		if !sameText(prior, want.prior) {
			t.Errorf("%s：前版指回 = %v，应原样为 %v", want.parcel, prior, want.prior)
		}
		if publishedAt != want.publishedAt {
			t.Errorf("%s：发布时间 = %q，应原样为 %q", want.parcel, publishedAt, want.publishedAt)
		}
		if got.milestonesState != want.dims.milestonesState || got.etaState != want.dims.etaState ||
			got.finalState != want.dims.finalState || got.noteState != want.dims.noteState ||
			!sameText(got.milestonesContent, want.dims.milestonesContent) ||
			!sameText(got.etaContent, want.dims.etaContent) ||
			!sameText(got.finalContent, want.dims.finalContent) ||
			!sameText(got.noteContent, want.dims.noteContent) {
			t.Errorf("%s：四维迁移变形：%+v，应原样为 %+v", want.parcel, got, want.dims)
		}
	}

	// 标记只能指存下来的版本：指一个不存在的版本被外键拒绝，当前版因此不会指着读不回的行。
	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.customer_view_current
		(tenant_id, customer_account_id, parcel_id, version, based_on_projection)
		VALUES ('SYN-TENANT-CV28', 'SYN-ACCOUNT-CV28', 'SYN-PARCEL-DANGLING', 'SYN-VIEW-MISSING', 'SYN-PROJ-1')`); err == nil {
		t.Error("当前标记指了一个不存在的版本，外键却没拒")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" ||
			pgErr.ConstraintName != "customer_view_current_names_stored_version" {
			t.Errorf("悬空标记应被版本外键拒绝，实得：%v", err)
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

func sameText(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}
