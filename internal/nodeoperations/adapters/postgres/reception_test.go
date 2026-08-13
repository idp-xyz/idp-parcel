package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证收寄判断库的行为：写入代数由 ON CONFLICT 承担、两格
// 在场规则由迁移 CHECK 把关、租户隔离由 SQL 条件承担、事务纪律由 RequireExecutor 拦住。
// 断言一律在事务闭包外：闭包里 Fatalf 会经 Goexit 跳过事务收尾，把连接挂死在池里。

var receivedAt = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

func newReceptions(t *testing.T) (*adapter.Receptions, bentoapp.Transactor) {
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
	return repository, db.Transactor()
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
		Key:            ports.ReceptionKey{TenantID: tenantID, SourceID: sourceID},
		ContentDigest:  "digest-1",
		Kind:           ports.RecordIntakeFormed,
		Intake:         intake,
		Control:        control,
		ServiceMarkers: []string{"CANCELLED_BEFORE_ARRIVAL"},
		RecordedAt:     receivedAt.Add(time.Minute),
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
	if len(foundFormed.ServiceMarkers) != 1 || foundFormed.ServiceMarkers[0] != "CANCELLED_BEFORE_ARRIVAL" {
		t.Errorf("服务标记读回变形：%v", foundFormed.ServiceMarkers)
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
