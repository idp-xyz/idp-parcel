package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证收寄记录的身份核对态（ADR-0199 决定五，票 no-reception-source/05）：往返原样、库面形状、迁移回填。

// uncheckedPending 是一条未核对的待识别：零候选、不记冲突、来源已保全。
func uncheckedPending(t *testing.T, tenant, sourceID, version string) ports.ReceptionRecord {
	t.Helper()
	record := recheckablePending(t, tenant, sourceID, version, "SYN-MARK-NRS05")
	record.IdentityCheck = ports.IdentityUnchecked
	return record
}

// Covers: ADR-0199 决定五「身份核对态只在待识别记录上，封闭两值」的存储面——未核对、已核对与不带三种往返原样：未核对与
// 零候选的已核对两行候选同为空，只有这一格把「没问到」与「查过了，查无此号」分开。
func TestTheIdentityCheckIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	unchecked := uncheckedPending(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-UNCHECKED", "SYN-INTAKEV-NRS05-1")
	checked := recheckablePending(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-CHECKED", "SYN-INTAKEV-NRS05-2", "SYN-MARK-NRS05")
	checked.IdentityCheck = ports.IdentityChecked
	formed := formedRecord(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-FORMED")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{unchecked, checked, formed} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	for want, record := range map[ports.IdentityCheck]ports.ReceptionRecord{
		ports.IdentityUnchecked:   unchecked,
		ports.IdentityChecked:     checked,
		ports.IdentityCheckAbsent: formed,
	} {
		found, exists, err := repository.FindByKey(ctx, record.Key)
		if err != nil || !exists {
			t.Fatalf("取回 %s：exists=%v err=%v", record.Key.SourceID, exists, err)
		}
		if found.IdentityCheck != want {
			t.Errorf("%s 核对态读回 %q, want %q", record.Key.SourceID, found.IdentityCheck, want)
		}
	}
}

// Covers: ADR-0199 决定五身份核对态的库面形状——绕过适配器直接写库的一行也过不了：集外取值、待识别以外带核对态、未核对却带
// 候选、未核对却记身份冲突、未核对却没有来源文档，都被 CHECK 拒。合形的未核对与已核对照常可写。
func TestTheReceptionTableRefusesAMisshapenIdentityCheck(t *testing.T) {
	_, _, pool := newReceptionsWithPool(t)
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, service_result_check, identity_check, source_document, recorded_at)
		VALUES ('SYN-TENANT-NRS05', $1, 'syn-digest', $2,
		        CASE WHEN $2 IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION') THEN '{}'::jsonb END,
		        CASE WHEN $2 IN ('INTAKE_FORMED', 'PENDING_IDENTIFICATION') THEN '{}'::jsonb END,
		        $3::jsonb, $4, '[]'::jsonb,
		        CASE WHEN $2 = 'INTAKE_FORMED' THEN 'CHECKED' END,
		        $5, $6::jsonb, now())`
	document := ptr(`{"shape":"NRS-1"}`)
	cases := map[string]struct {
		kind, candidates string
		conflict         bool
		check, source    *string
	}{
		"集外取值":      {kind: "PENDING_IDENTIFICATION", candidates: `[]`, check: ptr("MAYBE"), source: document},
		"形成格带核对态":   {kind: "INTAKE_FORMED", candidates: `[]`, check: ptr("CHECKED"), source: document},
		"拒收带核对态":    {kind: "INTAKE_NOT_FORMED", candidates: `[]`, check: ptr("UNCHECKED"), source: document},
		"未核对却带候选":   {kind: "PENDING_IDENTIFICATION", candidates: `["SYN-PARCEL-1"]`, check: ptr("UNCHECKED"), source: document},
		"未核对却记身份冲突": {kind: "PENDING_IDENTIFICATION", candidates: `[]`, conflict: true, check: ptr("UNCHECKED"), source: document},
		"未核对却没有来源":  {kind: "PENDING_IDENTIFICATION", candidates: `[]`, check: ptr("UNCHECKED")},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), insert, "SYN-SCAN-"+name, testCase.kind, testCase.candidates, testCase.conflict,
				testCase.check, testCase.source)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("写入 err = %v, want CHECK 违例（23514）", err)
			}
		})
	}
	if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-OK-UNCHECKED", "PENDING_IDENTIFICATION", `[]`, false, ptr("UNCHECKED"), document); err != nil {
		t.Fatalf("合形的未核对行被拒：%v", err)
	}
	if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-OK-CHECKED", "PENDING_IDENTIFICATION", `["SYN-PARCEL-1","SYN-PARCEL-2"]`, true, ptr("CHECKED"), nil); err != nil {
		t.Fatalf("合形的已核对冲突行被拒：%v", err)
	}
}

// Covers: ADR-0199 决定五「翻转之前落下的待识别行回填『已核对』」的迁移——核对态这一列加上之前已落的待识别行（零候选与记冲突
// 的都算）回填已核对，其余种类不带；推定依据见迁移头注。从空库按序施加本上下文的迁移，停在核对态那一份之前落旧形状的行，
// 再施加那一份。
func TestTheIdentityCheckMigrationBackfillsPendingUnitsAsChecked(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA node_operations`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NodeOperations()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0011_reception_identity_check"
	var backfill *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			backfill = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if backfill == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
		 service_result_check, judgment_version, recorded_at)
		VALUES ('SYN-TENANT-NRS05', 'SYN-SCAN-ZERO', 'syn-digest-1', 'PENDING_IDENTIFICATION',
		        '{"version":"SYN-INTAKEV-NRS05-ZERO"}'::jsonb, '{}'::jsonb, '[]'::jsonb, false, '[]'::jsonb, NULL,
		        'SYN-INTAKEV-NRS05-ZERO', now()),
		       ('SYN-TENANT-NRS05', 'SYN-SCAN-CONFLICT', 'syn-digest-2', 'PENDING_IDENTIFICATION',
		        '{"version":"SYN-INTAKEV-NRS05-CONFLICT"}'::jsonb, '{}'::jsonb, '["SYN-PARCEL-1","SYN-PARCEL-2"]'::jsonb, true,
		        '[]'::jsonb, NULL, 'SYN-INTAKEV-NRS05-CONFLICT', now()),
		       ('SYN-TENANT-NRS05', 'SYN-SCAN-FORMED', 'syn-digest-3', 'INTAKE_FORMED',
		        '{"version":"SYN-INTAKEV-NRS05-FORMED"}'::jsonb, '{}'::jsonb, '[]'::jsonb, false, '[]'::jsonb, 'CHECKED',
		        'SYN-INTAKEV-NRS05-FORMED', now()),
		       ('SYN-TENANT-NRS05', 'SYN-SCAN-REFUSED', 'syn-digest-4', 'INTAKE_NOT_FORMED', NULL, NULL,
		        '[]'::jsonb, false, '[]'::jsonb, NULL, NULL, now())`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, backfill.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}

	got := map[string]*string{}
	rows, err := conn.Query(ctx, `SELECT source_id, identity_check FROM node_operations.reception`)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	for rows.Next() {
		var source string
		var check *string
		if err := rows.Scan(&source, &check); err != nil {
			t.Fatalf("扫描：%v", err)
		}
		got[source] = check
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读回：%v", err)
	}
	for _, source := range []string{"SYN-SCAN-ZERO", "SYN-SCAN-CONFLICT"} {
		if check := got[source]; check == nil || *check != "CHECKED" {
			t.Fatalf("旧待识别行 %s 回填 = %v, want CHECKED", source, check)
		}
	}
	for _, source := range []string{"SYN-SCAN-FORMED", "SYN-SCAN-REFUSED"} {
		if check := got[source]; check != nil {
			t.Fatalf("%s 回填了 %q, want 不带", source, *check)
		}
	}
}
