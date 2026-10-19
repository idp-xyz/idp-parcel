package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证收寄记录的冲突来由（ADR-0211 决定一、二，票 no-reception-identity/07）：往返原样、库面形状、
// 迁移不回填，以及按号找待重核时放进来由为只因歧义信号的冲突行。夹具全为合成引用（S 级）。

// contestedPending 是一条记身份冲突的待识别：候选只有一件、来源已保全（号码 mark），来由由调用方给。
func contestedPending(t *testing.T, tenant, sourceID, version, mark string, cause ports.IdentityConflictCause) ports.ReceptionRecord {
	t.Helper()
	record := recheckablePending(t, tenant, sourceID, version, mark)
	record.Candidates = []domain.ParcelAssociationReference{ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI07")}
	record.IdentityConflict = true
	record.IdentityConflictCause = cause
	record.IdentityCheck = ports.IdentityChecked
	return record
}

// Covers: ADR-0211 决定一的存储面——三种来由与不带来由各自往返原样；记冲突却不带来由的行（本迁移之前落下的形状）也读得回。
func TestTheConflictCauseIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-NRI07", "SYN-MARK-NRI07"
	records := map[ports.IdentityConflictCause]ports.ReceptionRecord{
		ports.ConflictByAmbiguityOnly:      contestedPending(t, tenant, "SYN-SCAN-NRI07-AMB", "SYN-INTAKEV-NRI07-AMB", mark, ports.ConflictByAmbiguityOnly),
		ports.ConflictByMultipleCandidates: contestedPending(t, tenant, "SYN-SCAN-NRI07-MANY", "SYN-INTAKEV-NRI07-MANY", mark, ports.ConflictByMultipleCandidates),
		ports.ConflictByOneMarkOnTwoUnits:  contestedPending(t, tenant, "SYN-SCAN-NRI07-TWO", "SYN-INTAKEV-NRI07-TWO", mark, ports.ConflictByOneMarkOnTwoUnits),
		ports.IdentityConflictCauseAbsent:  contestedPending(t, tenant, "SYN-SCAN-NRI07-OLD", "SYN-INTAKEV-NRI07-OLD", mark, ports.IdentityConflictCauseAbsent),
	}
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
	for want, record := range records {
		found, exists, err := repository.FindByKey(ctx, record.Key)
		if err != nil || !exists {
			t.Fatalf("取回 %s：exists=%v err=%v", record.Key.SourceID, exists, err)
		}
		if found.IdentityConflictCause != want || !found.IdentityConflict {
			t.Errorf("%s 读回冲突 %v 来由 %q, want 记冲突、来由 %q", record.Key.SourceID, found.IdentityConflict, found.IdentityConflictCause, want)
		}
	}
}

// Covers: ADR-0211 决定一来由的库面形状——绕过适配器直接写库的一行也过不了：集外取值、不记冲突却带来由，都被 CHECK 拒。记冲突
// 不带来由照常可写：本迁移之前的冲突行就是这个形状，不回填。
func TestTheReceptionTableRefusesAMisshapenConflictCause(t *testing.T) {
	_, _, pool := newReceptionsWithPool(t)
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, identity_check, identity_conflict_cause, recorded_at)
		VALUES ('SYN-TENANT-NRI07', $1, 'syn-digest', 'PENDING_IDENTIFICATION', '{}'::jsonb, '{}'::jsonb,
		        '["SYN-PARCEL-NRI07"]'::jsonb, $2, '[]'::jsonb, 'CHECKED', $3, now())`
	for name, testCase := range map[string]struct {
		conflict bool
		cause    *string
	}{
		"集外取值":      {conflict: true, cause: ptr("SIGNAL_GONE")},
		"不记冲突却带来由":  {conflict: false, cause: ptr("AMBIGUITY_ONLY")},
		"不记冲突却带多候选": {conflict: false, cause: ptr("MULTIPLE_CANDIDATES")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), insert, "SYN-SCAN-NRI07-"+name, testCase.conflict, testCase.cause)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("写入 err = %v, want CHECK 违例（23514）", err)
			}
		})
	}
	for name, cause := range map[string]*string{
		"只因歧义信号": ptr("AMBIGUITY_ONLY"), "多候选": ptr("MULTIPLE_CANDIDATES"), "一码两物": ptr("ONE_MARK_ON_TWO_UNITS"), "来由缺席": nil,
	} {
		if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-NRI07-OK-"+name, true, cause); err != nil {
			t.Fatalf("合形的冲突行（%s）被拒：%v", name, err)
		}
	}
}

// Covers: ADR-0211 决定二重核对象的存储面——按号找待重核时，来由为只因歧义信号、仍是链头的冲突行与零候选的待识别一起找回；
// 来由是多候选、一码两物或缺席的冲突行照旧不在其中，已被接续的只因歧义信号冲突行也不在。
func TestAnAmbiguityOnlyConflictIsFoundByThePreservedMark(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-NRI07", "SYN-MARK-NRI07"
	pending := recheckablePending(t, tenant, "SYN-SCAN-NRI07-A", "SYN-INTAKEV-NRI07-A", mark)
	contested := contestedPending(t, tenant, "SYN-SCAN-NRI07-B", "SYN-INTAKEV-NRI07-B", mark, ports.ConflictByAmbiguityOnly)
	released := contestedPending(t, tenant, "SYN-SCAN-NRI07-C", "SYN-INTAKEV-NRI07-C", mark, ports.ConflictByAmbiguityOnly)
	successor := recheckablePending(t, tenant, "SYN-SCAN-NRI07-C-NEXT", "SYN-INTAKEV-NRI07-C-NEXT", mark)
	successor.Predecessor = released.JudgmentVersion
	successor.IdentityConflict, successor.Candidates = true, released.Candidates
	successor.IdentityConflictCause = ports.ConflictByMultipleCandidates
	many := contestedPending(t, tenant, "SYN-SCAN-NRI07-D", "SYN-INTAKEV-NRI07-D", mark, ports.ConflictByMultipleCandidates)
	two := contestedPending(t, tenant, "SYN-SCAN-NRI07-E", "SYN-INTAKEV-NRI07-E", mark, ports.ConflictByOneMarkOnTwoUnits)
	old := contestedPending(t, tenant, "SYN-SCAN-NRI07-F", "SYN-INTAKEV-NRI07-F", mark, ports.IdentityConflictCauseAbsent)
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{pending, contested, released, successor, many, two, old} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	found, err := repository.FindRecheckableByMark(ctx, ref(t, domain.NewTenantID, tenant), mark)
	if err != nil {
		t.Fatalf("按号找：%v", err)
	}
	if got := strings.Join(sourceIDsOf(found), ","); got != "SYN-SCAN-NRI07-A,SYN-SCAN-NRI07-B" {
		t.Fatalf("按号找到 %s, want 零候选的待识别与仍是链头的只因歧义信号冲突行", got)
	}
	if found[1].IdentityConflictCause != ports.ConflictByAmbiguityOnly || !found[1].IdentityConflict {
		t.Fatalf("找回的冲突行没有原样读回：%+v", found[1])
	}
}

// Covers: ADR-0211 决定一「既有冲突行来由缺席、不回填」的迁移——来由这一列加上之前已落的冲突行（多候选与带一个候选的都算）
// 读回不带来由，迁移不推定任何一行的来由。从空库按序施加本上下文的迁移，停在来由那一份之前落旧形状的行，再施加那一份。
func TestTheConflictCauseMigrationLeavesExistingConflictsWithoutACause(t *testing.T) {
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
	const target = "0012_reception_identity_conflict_cause"
	var cause *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			cause = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if cause == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
		 identity_check, judgment_version, recorded_at)
		VALUES ('SYN-TENANT-NRI07', 'SYN-SCAN-NRI07-ONE', 'syn-digest-1', 'PENDING_IDENTIFICATION',
		        '{"version":"SYN-INTAKEV-NRI07-ONE"}'::jsonb, '{}'::jsonb, '["SYN-PARCEL-1"]'::jsonb, true, '[]'::jsonb,
		        'CHECKED', 'SYN-INTAKEV-NRI07-ONE', now()),
		       ('SYN-TENANT-NRI07', 'SYN-SCAN-NRI07-MANY', 'syn-digest-2', 'PENDING_IDENTIFICATION',
		        '{"version":"SYN-INTAKEV-NRI07-MANY"}'::jsonb, '{}'::jsonb, '["SYN-PARCEL-1","SYN-PARCEL-2"]'::jsonb, true,
		        '[]'::jsonb, 'CHECKED', 'SYN-INTAKEV-NRI07-MANY', now())`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, cause.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}
	var withCause int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM node_operations.reception WHERE identity_conflict_cause IS NOT NULL`).Scan(&withCause); err != nil {
		t.Fatalf("读回：%v", err)
	}
	if withCause != 0 {
		t.Fatalf("迁移给 %d 行既有冲突推定了来由, want 0——不回填", withCause)
	}
}
