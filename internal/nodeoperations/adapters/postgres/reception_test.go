package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证收寄判断库的行为：写入代数由 ON CONFLICT 承担、两格
// 在场规则由迁移 CHECK 把关、租户隔离由 SQL 条件承担、事务纪律由 RequireExecutor 拦住。
// 断言一律在事务闭包外：闭包里 Fatalf 会经 Goexit 跳过事务收尾，把连接挂死在池里。

var receivedAt = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

func newReceptions(t *testing.T) (*adapter.Receptions, bentoapp.Transactor) {
	t.Helper()
	repository, transactor, _ := newReceptionsWithPool(t)
	return repository, transactor
}

func newReceptionsWithPool(t *testing.T) (*adapter.Receptions, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func inTx(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func ref[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func formedRecord(t *testing.T, tenant, sourceID string) ports.ReceptionRecord {
	t.Helper()
	tenantID := ref(t, domain.NewTenantID, tenant)
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    tenantID,
		Unit:        ref(t, domain.NewHandlingUnitID, "unit-1"),
		Node:        ref(t, domain.NewNodeReference, "node-1"),
		DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "courier-1"),
		Evidence:    ref(t, domain.NewReceptionEvidenceReference, "evidence/receipt-1"),
		Version:     ref(t, domain.NewIntakeResultVersion, "intake/v1"),
		Association: ref(t, domain.NewParcelAssociationReference, "parcel-1/link-v1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("构造收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID:      tenantID,
		Unit:          ref(t, domain.NewHandlingUnitID, "unit-1"),
		Node:          ref(t, domain.NewNodeReference, "node-1"),
		Kind:          domain.EstablishedByNodeIntake,
		Basis:         ref(t, domain.NewControlBasisReference, "intake/v1"),
		EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	return ports.ReceptionRecord{
		Key:                ports.ReceptionKey{TenantID: tenantID, SourceID: sourceID},
		ContentDigest:      "digest-1",
		Kind:               ports.RecordIntakeFormed,
		Intake:             intake,
		Control:            control,
		ServiceMarkers:     ports.ParcelServiceResults{Cancellation: "SYN-CANCEL-1"}.Markers(),
		ServiceResultCheck: ports.ServiceResultsChecked,
		RecordedAt:         receivedAt.Add(time.Minute),
	}
}

func notFormedRecord(t *testing.T, tenant, sourceID string) ports.ReceptionRecord {
	t.Helper()
	return ports.ReceptionRecord{
		Key:           ports.ReceptionKey{TenantID: ref(t, domain.NewTenantID, tenant), SourceID: sourceID},
		ContentDigest: "digest-refused",
		Kind:          ports.RecordIntakeNotFormed,
		RefusalReason: "packaging-unacceptable",
		RecordedAt:    receivedAt.Add(time.Minute),
	}
}

func pendingRecord(t *testing.T, tenant, sourceID string) ports.ReceptionRecord {
	t.Helper()
	record := formedRecord(t, tenant, sourceID)
	// 待识别：收寄无正式关联、候选多于一并标身份冲突。
	tenantID := record.Key.TenantID
	intake, err := domain.FormNodeIntake(domain.NodeIntakeSpec{
		TenantID:    tenantID,
		Unit:        ref(t, domain.NewHandlingUnitID, "unit-2"),
		Node:        ref(t, domain.NewNodeReference, "node-1"),
		DeliveredBy: ref(t, domain.NewDeliveringPartyReference, "courier-1"),
		Evidence:    ref(t, domain.NewReceptionEvidenceReference, "evidence/receipt-2"),
		Version:     ref(t, domain.NewIntakeResultVersion, "intake/v2"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("构造待识别收寄：%v", err)
	}
	record.ContentDigest = "digest-pending"
	record.Kind = ports.RecordPendingIdentification
	record.Intake = intake
	record.Candidates = []domain.ParcelAssociationReference{
		ref(t, domain.NewParcelAssociationReference, "parcel-7/link-v1"),
		ref(t, domain.NewParcelAssociationReference, "parcel-8/link-v1"),
	}
	record.IdentityConflict = true
	record.ServiceMarkers = nil
	record.ServiceResultCheck = ports.ServiceResultCheckAbsent
	return record
}

// TestReceptionsAreReadBackUnchanged 证三格记录各自原样读回：形成格带收寄+控制+服务
// 标记，待识别格带候选与身份冲突且收寄无关联，未形成格不带收寄与控制只带拒因。
func TestReceptionsAreReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()

	formed := formedRecord(t, "tenant-a", "scan-1")
	pending := pendingRecord(t, "tenant-a", "scan-2")
	refused := notFormedRecord(t, "tenant-a", "scan-3")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{formed, pending, refused} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	foundFormed, exists, err := repository.FindByKey(ctx, formed.Key)
	if err != nil || !exists {
		t.Fatalf("取回形成格：%v exists=%v", err, exists)
	}
	if foundFormed.Kind != ports.RecordIntakeFormed ||
		foundFormed.Intake.ReceivedAt() != formed.Intake.ReceivedAt() ||
		foundFormed.Intake.Evidence() != formed.Intake.Evidence() ||
		foundFormed.Control.Basis() != formed.Control.Basis() ||
		!foundFormed.Control.Active() {
		t.Errorf("形成格读回变形：%+v", foundFormed)
	}
	if association, present := foundFormed.Intake.Association(); !present || association.String() != "parcel-1/link-v1" {
		t.Errorf("正式关联读回变形：%v present=%v", association, present)
	}
	if strings.Join(foundFormed.ServiceMarkers, ",") != strings.Join(formed.ServiceMarkers, ",") || len(formed.ServiceMarkers) != 1 {
		t.Errorf("服务标记读回变形：%v, want %v", foundFormed.ServiceMarkers, formed.ServiceMarkers)
	}

	foundPending, exists, err := repository.FindByKey(ctx, pending.Key)
	if err != nil || !exists {
		t.Fatalf("取回待识别格：%v exists=%v", err, exists)
	}
	if _, present := foundPending.Intake.Association(); present {
		t.Error("待识别收寄凭空长出了正式关联")
	}
	if len(foundPending.Candidates) != 2 || !foundPending.IdentityConflict {
		t.Errorf("候选或冲突标读回变形：%+v", foundPending)
	}

	foundRefused, exists, err := repository.FindByKey(ctx, refused.Key)
	if err != nil || !exists {
		t.Fatalf("取回未形成格：%v exists=%v", err, exists)
	}
	if foundRefused.Intake.ReceivedAt() != (time.Time{}) || foundRefused.Control.Active() {
		t.Error("未形成格凭空长出了收寄或控制")
	}
	if foundRefused.RefusalReason != "packaging-unacceptable" {
		t.Errorf("拒因 = %q", foundRefused.RefusalReason)
	}
}

// TestASecondWriterGetsAlreadyRecorded 证同键第二份写入拿到`已有记录`且事务保持可用
// ——同一事务里紧接着读回先到者作答，这正是编排的用法。
func TestASecondWriterGetsAlreadyRecorded(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()

	first := formedRecord(t, "tenant-a", "scan-1")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, first)
		return err
	})

	second := formedRecord(t, "tenant-a", "scan-1")
	second.ContentDigest = "digest-second"
	var outcome ports.ReceptionSaveOutcome
	var winner ports.ReceptionRecord
	var winnerFound bool
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = repository.FindByKey(txCtx, first.Key)
		return err
	})

	if outcome != ports.ReceptionAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %q，应为 ALREADY_RECORDED", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

// TestOtherTenantsAreInvisible 证否定结果不泄露另一个租户是否回传过同一来源标识。
func TestOtherTenantsAreInvisible(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()

	saved := formedRecord(t, "tenant-a", "scan-shared")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, saved)
		return err
	})

	elsewhere := ports.ReceptionKey{TenantID: ref(t, domain.NewTenantID, "tenant-b"), SourceID: "scan-shared"}
	_, exists, err := repository.FindByKey(ctx, elsewhere)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("另一个租户读到了不属于它的收寄记录")
	}
}

// Covers: ADR-0196 收寄库按包裹的读口——只交回同租户、收寄关联到这件包裹、作业实物不是来问那件的记录，原样读回；
// 待识别行的候选不是关联，另一个租户的同一个关联不可见。
func TestOtherUnitIntakesAreFoundByTheirParcelAssociation(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()

	formed := formedRecord(t, "SYN-TENANT-A", "SYN-SCAN-1")
	pending := pendingRecord(t, "SYN-TENANT-A", "SYN-SCAN-2")
	elsewhere := formedRecord(t, "SYN-TENANT-B", "SYN-SCAN-1")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{formed, pending, elsewhere} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	tenant := formed.Key.TenantID
	parcel, _ := formed.Intake.Association()
	asking := ref(t, domain.NewHandlingUnitID, "SYN-UNIT-9")
	found, err := repository.FindOtherUnitIntakes(ctx, tenant, parcel, asking)
	if err != nil {
		t.Fatalf("按包裹读：%v", err)
	}
	if len(found) != 1 || found[0].Key != formed.Key || found[0].Kind != formed.Kind ||
		found[0].Intake != formed.Intake || found[0].Control != formed.Control {
		t.Fatalf("按包裹读到 %+v, want 只有同租户那条关联收寄且原样读回", found)
	}

	sameUnit, err := repository.FindOtherUnitIntakes(ctx, tenant, parcel, formed.Intake.Unit())
	if err != nil || len(sameUnit) != 0 {
		t.Fatalf("同一件实物来问读到 %d 条 err = %v, want 0——自己的收寄不是别的实物", len(sameUnit), err)
	}
	candidateOnly, err := repository.FindOtherUnitIntakes(ctx, tenant, pending.Candidates[0], asking)
	if err != nil || len(candidateOnly) != 0 {
		t.Fatalf("按候选读到 %d 条 err = %v, want 0——候选不是关联", len(candidateOnly), err)
	}
}

// TestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newReceptions(t)
	ctx := t.Context()

	record := formedRecord(t, "tenant-a", "scan-1")
	if _, err := repository.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("被拒绝的写入仍然落库了")
	}
}

// TestRollbackLeavesNothingBehind 证收寄判断与它所在的事务同生共死。
func TestRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	record := formedRecord(t, "tenant-a", "scan-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后收寄记录仍在")
	}
}

// Covers: ADR-0186「收寄记录新增服务结果核对态」的存储面——已查、读不动与不带三种往返原样：读不动那条的空标记和已查
// 那条的标记都原样读回，待识别不带核对态。
func TestTheServiceResultCheckIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()

	checked := formedRecord(t, "tenant-a", "scan-1")
	unreadable := formedRecord(t, "tenant-a", "scan-2")
	unreadable.ServiceMarkers = nil
	unreadable.ServiceResultCheck = ports.ServiceResultsUnreadable
	pending := pendingRecord(t, "tenant-a", "scan-3")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ReceptionRecord{checked, unreadable, pending} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	for _, want := range []ports.ReceptionRecord{checked, unreadable, pending} {
		found, exists, err := repository.FindByKey(ctx, want.Key)
		if err != nil || !exists {
			t.Fatalf("取回 %s：%v exists=%v", want.Key.SourceID, err, exists)
		}
		if found.ServiceResultCheck != want.ServiceResultCheck || len(found.ServiceMarkers) != len(want.ServiceMarkers) {
			t.Errorf("%s 读回 核对态 %q 标记 %v, want %q %v", want.Key.SourceID,
				found.ServiceResultCheck, found.ServiceMarkers, want.ServiceResultCheck, want.ServiceMarkers)
		}
	}
}

// Covers: ADR-0186 核对态的库面形状——绕过适配器直接写库的一行也过不了：集外取值、形成格以外带核对态、读不动却带
// 标记，都被 CHECK 拒。
func TestTheReceptionTableRefusesAMisshapenServiceResultCheck(t *testing.T) {
	_, _, pool := newReceptionsWithPool(t)
	const insert = `INSERT INTO node_operations.reception
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict,
		 service_markers, service_result_check, recorded_at)
		VALUES ('SYN-TENANT-01', $1, 'syn-digest', $2,
		        CASE WHEN $2 = 'INTAKE_FORMED' THEN '{}'::jsonb END,
		        CASE WHEN $2 = 'INTAKE_FORMED' THEN '{}'::jsonb END,
		        '[]'::jsonb, false, $3::jsonb, $4, now())`
	cases := map[string]struct {
		kind, markers string
		check         *string
	}{
		"集外取值":      {kind: "INTAKE_FORMED", markers: `[]`, check: ptr("MAYBE")},
		"形成格以外带核对态": {kind: "INTAKE_NOT_FORMED", markers: `[]`, check: ptr("CHECKED")},
		"读不动却带标记":   {kind: "INTAKE_FORMED", markers: `["CANCELLED/SYN-CANCEL-1"]`, check: ptr("UNREADABLE")},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), insert, "SYN-SCAN-"+name, testCase.kind, testCase.markers, testCase.check)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("写入 err = %v, want CHECK 违例（23514）", err)
			}
		})
	}
	if _, err := pool.Exec(t.Context(), insert, "SYN-SCAN-OK", "INTAKE_FORMED", `[]`, ptr("UNREADABLE")); err != nil {
		t.Fatalf("合形的读不动行被拒：%v", err)
	}
}

func ptr(value string) *string { return &value }

// Covers: ADR-0186 迁移回填——核对态这一列加上之前已落的 INTAKE_FORMED 行回填已查，其余种类不带；推定依据见迁移头注。
// 从空库按序施加本上下文的迁移，停在核对态那一份之前落旧形状的行，再施加那一份。
func TestTheServiceResultCheckMigrationBackfillsFormedReceptionsAsChecked(t *testing.T) {
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
	const target = "0006_reception_service_result_check"
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
		(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers, recorded_at)
		VALUES ('SYN-TENANT-01', 'SYN-SCAN-FORMED', 'syn-digest-1', 'INTAKE_FORMED', '{}'::jsonb, '{}'::jsonb,
		        '[]'::jsonb, false, '["CANCELLED/SYN-CANCEL-1"]'::jsonb, now()),
		       ('SYN-TENANT-01', 'SYN-SCAN-REFUSED', 'syn-digest-2', 'INTAKE_NOT_FORMED', NULL, NULL,
		        '[]'::jsonb, false, '[]'::jsonb, now())`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, backfill.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}

	got := map[string]*string{}
	rows, err := conn.Query(ctx, `SELECT source_id, service_result_check FROM node_operations.reception`)
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
	if check := got["SYN-SCAN-FORMED"]; check == nil || *check != "CHECKED" {
		t.Fatalf("旧 INTAKE_FORMED 行回填 = %v, want CHECKED", check)
	}
	if check := got["SYN-SCAN-REFUSED"]; check != nil {
		t.Fatalf("未形成行回填了 %q, want 不带", *check)
	}
}
