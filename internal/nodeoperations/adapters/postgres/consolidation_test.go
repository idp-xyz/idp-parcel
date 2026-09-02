package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var consolidationAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

func TestAnOpenedUnitRoundTripsAndDuplicateSaveKeepsTheFirst(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	opened := openConsolidation(t, "bag-1", "asset-7")
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := store.Save(txCtx, tenant, opened)
		if err != nil {
			return err
		}
		if outcome != ports.ConsolidationSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := store.FindByID(ctx, tenant, opened.ID())
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Sealed() || found.Closed() || len(found.Members()) != 0 || found.Asset() != opened.Asset() {
		t.Fatalf("开启往返变形：sealed=%v closed=%v members=%d", found.Sealed(), found.Closed(), len(found.Members()))
	}

	duplicate := openConsolidation(t, "bag-1", "asset-other")
	var outcome ports.ConsolidationSaveOutcome
	var winner *domain.ConsolidationUnit
	var winnerFound bool
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := store.Save(txCtx, tenant, duplicate)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = store.FindByID(txCtx, tenant, opened.ID())
		return err
	})
	if outcome != ports.ConsolidationAlreadyRecorded {
		t.Fatalf("重复开启结果 = %d", outcome)
	}
	if !winnerFound || winner.Asset().String() != "asset-7" {
		t.Fatalf("同事务读回赢家失败：found=%v asset=%q", winnerFound, winner.Asset())
	}
}

func TestMembershipSealUnsealAndCloseRoundTripThroughUpdate(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, store, tenant, unit)

	member1 := ref(t, domain.NewHandlingUnitID, "unit-1")
	member2 := ref(t, domain.NewHandlingUnitID, "unit-2")
	if err := unit.AddMember(member1); err != nil {
		t.Fatalf("add member-1：%v", err)
	}
	if err := unit.AddMember(member2); err != nil {
		t.Fatalf("add member-2：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, unit)

	parent, contained, err := store.CurrentParent(ctx, tenant, member1)
	if err != nil || !contained || parent != unit.ID() {
		t.Fatalf("加入后找不到父级：contained=%v parent=%q err=%v", contained, parent, err)
	}

	if err := unit.Seal(
		ref(t, domain.NewSealReference, "seal-1"),
		ref(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-1", consolidationAt),
	); err != nil {
		t.Fatalf("seal：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, unit)

	sealed, exists, err := store.FindByID(ctx, tenant, unit.ID())
	if err != nil || !exists || !sealed.Sealed() || len(sealed.Snapshots()) != 1 {
		t.Fatalf("封装往返失败：exists=%v sealed=%v snapshots=%d err=%v",
			exists, sealed != nil && sealed.Sealed(), len(sealed.Snapshots()), err)
	}
	if sealed.Snapshots()[0].Seal().String() != "seal-1" ||
		len(sealed.Snapshots()[0].Members()) != 2 {
		t.Fatal("封装快照往返变形")
	}

	if err := sealed.Unseal(ref(t, domain.NewWorkBasisReference, "UNPACK/2"), consolidationAt.Add(time.Hour)); err != nil {
		t.Fatalf("unseal：%v", err)
	}
	if err := sealed.RemoveMember(member2); err != nil {
		t.Fatalf("remove：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, sealed)

	_, contained, err = store.CurrentParent(ctx, tenant, member2)
	if err != nil || contained {
		t.Fatalf("移出后仍能找到父级：contained=%v err=%v", contained, err)
	}

	if err := sealed.RemoveMember(member1); err != nil {
		t.Fatalf("remove last：%v", err)
	}
	if err := sealed.Close(domain.WorkBasisReference{}, consolidationAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("close emptied：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, sealed)

	closed, exists, err := store.FindByID(ctx, tenant, unit.ID())
	if err != nil || !exists || !closed.Closed() || closed.ClosedAt().IsZero() {
		t.Fatalf("关闭往返失败：exists=%v closed=%v err=%v", exists, closed != nil && closed.Closed(), err)
	}
	if len(closed.Members()) != 0 || len(closed.Snapshots()) != 1 {
		t.Fatalf("清空关闭后 members=%d snapshots=%d", len(closed.Members()), len(closed.Snapshots()))
	}
	_, contained, err = store.CurrentParent(ctx, tenant, member1)
	if err != nil || contained {
		t.Fatalf("关闭后成员仍占据当前父级：contained=%v err=%v", contained, err)
	}
}

func TestClosingWithRemainingMembersReleasesTheCurrentParent(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")
	saveConsolidation(t, transactor, ctx, store, tenant, unit)

	member := ref(t, domain.NewHandlingUnitID, "unit-1")
	if err := unit.AddMember(member); err != nil {
		t.Fatalf("add：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, unit)

	if err := unit.Close(ref(t, domain.NewWorkBasisReference, "DISPOSITION/3"), consolidationAt); err != nil {
		t.Fatalf("close with disposition：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, unit)

	found, exists, err := store.FindByID(ctx, tenant, unit.ID())
	if err != nil || !exists || !found.Closed() || len(found.Members()) != 1 {
		t.Fatalf("带成员关闭应保留成员：exists=%v closed=%v members=%d err=%v",
			exists, found != nil && found.Closed(), len(found.Members()), err)
	}
	_, contained, err := store.CurrentParent(ctx, tenant, member)
	if err != nil || contained {
		t.Fatalf("关闭后仍占据当前父级：contained=%v err=%v", contained, err)
	}

	next := openConsolidation(t, "bag-2", "asset-8")
	saveConsolidation(t, transactor, ctx, store, tenant, next)
	if err := next.AddMember(member); err != nil {
		t.Fatalf("add to next：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, next)
	parent, contained, err := store.CurrentParent(ctx, tenant, member)
	if err != nil || !contained || parent != next.ID() {
		t.Fatalf("关闭后未能进入下一单元：contained=%v parent=%q err=%v", contained, parent, err)
	}
}

func TestAMemberCannotHaveTwoOpenParents(t *testing.T) {
	store, transactor, pool := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	first := openConsolidation(t, "bag-1", "asset-7")
	second := openConsolidation(t, "bag-2", "asset-8")
	saveConsolidation(t, transactor, ctx, store, tenant, first)
	saveConsolidation(t, transactor, ctx, store, tenant, second)

	member := ref(t, domain.NewHandlingUnitID, "unit-1")
	if err := first.AddMember(member); err != nil {
		t.Fatalf("add to first：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenant, first)

	parent, contained, err := store.CurrentParent(ctx, tenant, member)
	if err != nil || !contained || parent != first.ID() {
		t.Fatalf("第一父级：contained=%v parent=%q err=%v", contained, parent, err)
	}

	if err := second.AddMember(member); err != nil {
		t.Fatalf("add to second in memory：%v", err)
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return store.Update(txCtx, tenant, second)
	}); err == nil {
		t.Fatal("同一实物写进了第二个未关闭单元")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
		 VALUES ('tenant-a', 'unit-1', 'bag-2')`); err == nil {
		t.Fatal("容纳主键没有拦住跨单元第二父级")
	}
}

func TestConsolidationIsInvisibleAcrossTenants(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenantA := ref(t, domain.NewTenantID, "tenant-a")
	tenantB := ref(t, domain.NewTenantID, "tenant-b")

	unit := openConsolidation(t, "bag-shared", "asset-7")
	saveConsolidation(t, transactor, ctx, store, tenantA, unit)
	if err := unit.AddMember(ref(t, domain.NewHandlingUnitID, "unit-1")); err != nil {
		t.Fatalf("add：%v", err)
	}
	updateConsolidation(t, transactor, ctx, store, tenantA, unit)

	_, exists, err := store.FindByID(ctx, tenantB, unit.ID())
	if err != nil || exists {
		t.Errorf("另一个租户读到了集运单元：exists=%v err=%v", exists, err)
	}
	_, contained, err := store.CurrentParent(ctx, tenantB, ref(t, domain.NewHandlingUnitID, "unit-1"))
	if err != nil || contained {
		t.Errorf("另一个租户读到了容纳：contained=%v err=%v", contained, err)
	}
}

func TestConsolidationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _, _ := newConsolidationStore(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")

	if _, err := store.Save(ctx, tenant, unit); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := store.Update(ctx, tenant, unit); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Update 应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, exists, err := store.FindByID(ctx, tenant, unit.ID()); err != nil || exists {
		t.Errorf("被拒绝的写入仍然落库：exists=%v err=%v", exists, err)
	}
}

func TestConsolidationRollbackLeavesNothingBehind(t *testing.T) {
	store, transactor, _ := newConsolidationStore(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	unit := openConsolidation(t, "bag-1", "asset-7")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, tenant, unit); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := store.FindByID(ctx, tenant, unit.ID()); err != nil || exists {
		t.Errorf("回滚后集运单元仍在：exists=%v err=%v", exists, err)
	}
}

func TestConsolidationCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newConsolidationStore(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, closed_at)
		 VALUES ('tenant-a', 'bag-bad-1', 'asset-7', 'SEALED', '["unit-1"]', '[]', NULL)`); err == nil {
		t.Fatal("一行「封装却没有快照」溜进了合箱库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, closed_at)
		 VALUES ('tenant-a', 'bag-bad-2', 'asset-7', 'CLOSED', '[]', '[]', NULL)`); err == nil {
		t.Fatal("一行「关闭却没有时刻」溜进了合箱库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, closed_at)
		 VALUES ('tenant-a', 'bag-bad-3', 'asset-7', 'OPEN', '[]', '[]', now())`); err == nil {
		t.Fatal("一行「开放却带着关闭时刻」溜进了合箱库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, members, snapshots, closed_at)
		 VALUES ('tenant-a', 'bag-bad-4', 'asset-7', 'OPEN', NULL, '[]', NULL)`); err == nil {
		t.Fatal("一行「成员列为 NULL」按 jsonb 三值缝溜进了合箱库")
	}
}

func newConsolidationStore(t *testing.T) (*adapter.ConsolidationUnits, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	return store, db.Transactor(), pool
}

// workSource 造一份完整的来源表达。业务时间由调用方给——每一步作业各带各的现场时刻，
// 这正是 Clock.Now() 让位之后落库该有的样子（ADR-0023）。
func workSource(t *testing.T, sourceID string, at time.Time) domain.WorkFactSource {
	t.Helper()
	return workSourceBy(t, sourceID, "packer-1", at)
}

// workSourceBy 与 workSource 同形，另指定执行方。同一行里要同时看两处来源时用它——
// 两处若共用默认执行方，读面把它们取反了也验不出来。
func workSourceBy(t *testing.T, sourceID, performedBy string, at time.Time) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		ref(t, domain.NewPerformingPartyReference, performedBy),
		ref(t, domain.NewExecutionEvidenceReference, "WORK-EVIDENCE/"+sourceID),
		at,
	)
	if err != nil {
		t.Fatalf("来源表达 %q：%v", sourceID, err)
	}
	return source
}

func openConsolidation(t *testing.T, id, asset string) *domain.ConsolidationUnit {
	t.Helper()
	unit, err := domain.OpenConsolidationUnit(
		ref(t, domain.NewConsolidationUnitID, id),
		ref(t, domain.NewCarrierAssetReference, asset),
		workSource(t, "src-open-"+id, consolidationAt),
	)
	if err != nil {
		t.Fatalf("开启集运单元：%v", err)
	}
	return unit
}

func saveConsolidation(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	store *adapter.ConsolidationUnits,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) {
	t.Helper()
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := store.Save(txCtx, tenant, unit)
		if err != nil {
			return err
		}
		if outcome != ports.ConsolidationSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func updateConsolidation(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	store *adapter.ConsolidationUnits,
	tenant domain.TenantID,
	unit *domain.ConsolidationUnit,
) {
	t.Helper()
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		return store.Update(txCtx, tenant, unit)
	})
}
