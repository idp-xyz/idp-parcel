package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证装载分配登记：版本链上多版共存、变化与撤回各自往返、撞键译
// `已登记`且不顶替、无事务拒，以及库内 CHECK 挡住领域造不出的行。夹具全为合成登记（S 级）。

var assignedAtDBFixture = time.Date(2026, 9, 13, 6, 0, 0, 0, time.UTC)

// TestALoadAssignmentVersionRoundTripsWithItsMembers 证首版与它的对象范围同笔落、整图读回，
// 且读回来既没有前身也没有撤回——首版就是首版。
func TestALoadAssignmentVersionRoundTripsWithItsMembers(t *testing.T) {
	repository, transactor, _ := newLoadAssignments(t)
	ctx := t.Context()

	record := loadAssignmentRecord(t, "LA-0001", "v1", "parcel-1", "parcel-2")
	mustSaveLoadAssignment(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0001", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回分配：%v exists=%v", err, exists)
	}
	if found.Assignment.Schedule().String() != "schedule-1" {
		t.Fatalf("班次没有原样带回：%q", found.Assignment.Schedule())
	}
	members := found.Assignment.Members()
	if len(members) != 2 || members[0].String() != "parcel-1" || members[1].String() != "parcel-2" {
		t.Fatalf("对象范围没有原样带回：%v", members)
	}
	if _, has := found.Assignment.Corrects(); has {
		t.Fatal("首版读回来带了前身")
	}
	if _, withdrawn := found.Assignment.Withdrawn(); withdrawn {
		t.Fatal("首版读回来是已撤回的")
	}
}

// TestARevisedVersionCoexistsWithItsPredecessor 证 CONTEXT「装载分配形成、变化或撤回时保存
// 版本和对象范围」——变化形成新版本，**原版本原样留在库里**，两版各自读得回来且成员集不同。
//
// 这一条是本表存在版本维的全部理由：若变化是 UPDATE，前一版连同它的对象范围就没了。
func TestARevisedVersionCoexistsWithItsPredecessor(t *testing.T) {
	repository, transactor, _ := newLoadAssignments(t)
	ctx := t.Context()

	first := loadAssignmentRecord(t, "LA-0002", "v1", "parcel-1", "parcel-2")
	mustSaveLoadAssignment(t, transactor, ctx, repository, first)

	revised, err := first.Assignment.ReviseMembers(
		[]domain.CarriedObjectReference{segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")},
		segmentRef(t, domain.NewLoadAssignmentVersion, "v2"),
		assignedAtDBFixture.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("变化：%v", err)
	}
	mustSaveLoadAssignment(t, transactor, ctx, repository, ports.LoadAssignmentRecord{
		Key:        loadAssignmentKeyFixture(t, "tenant-1", "LA-0002", "v2"),
		Assignment: revised,
		RecordedAt: assignedAtDBFixture.Add(2*time.Hour + time.Minute),
	})

	original, exists, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0002", "v1"))
	if err != nil || !exists {
		t.Fatalf("原版本不见了：%v exists=%v", err, exists)
	}
	if len(original.Assignment.Members()) != 2 {
		t.Fatalf("原版本的对象范围被改了：%v", original.Assignment.Members())
	}

	next, exists, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0002", "v2"))
	if err != nil || !exists {
		t.Fatalf("取回新版本：%v exists=%v", err, exists)
	}
	corrects, has := next.Assignment.Corrects()
	if !has || corrects.String() != "v1" {
		t.Fatalf("新版本没有回指前身：%q has=%v", corrects, has)
	}
	revisedAt, wasRevised := next.Assignment.RevisedAt()
	if !wasRevised || !revisedAt.Equal(assignedAtDBFixture.Add(2*time.Hour)) {
		t.Fatalf("变化时刻没有原样带回：%v revised=%v", revisedAt, wasRevised)
	}
	if len(next.Assignment.Members()) != 1 {
		t.Fatalf("新版本的对象范围不对：%v", next.Assignment.Members())
	}
	// 分配时刻在整条链上不变——变化不是一次新的分配。
	if !next.Assignment.AssignedAt().Equal(assignedAtDBFixture) {
		t.Fatalf("新版本的分配时刻被改了：%s", next.Assignment.AssignedAt())
	}
}

// TestAWithdrawnVersionRoundTripsStillWithdrawn 证撤回两件同笔往返，且读回来仍然是撤回的
// ——装回不是重开：已撤回的分配不再变化。
func TestAWithdrawnVersionRoundTripsStillWithdrawn(t *testing.T) {
	repository, transactor, _ := newLoadAssignments(t)
	ctx := t.Context()

	first := loadAssignmentRecord(t, "LA-0003", "v1", "parcel-1")
	mustSaveLoadAssignment(t, transactor, ctx, repository, first)

	withdrawnAt := assignedAtDBFixture.Add(3 * time.Hour)
	withdrawn, err := first.Assignment.Withdraw(
		segmentRef(t, domain.NewLoadAssignmentVersion, "v2"), withdrawnAt)
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	mustSaveLoadAssignment(t, transactor, ctx, repository, ports.LoadAssignmentRecord{
		Key:        loadAssignmentKeyFixture(t, "tenant-1", "LA-0003", "v2"),
		Assignment: withdrawn,
		RecordedAt: withdrawnAt.Add(time.Minute),
	})

	found, exists, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0003", "v2"))
	if err != nil || !exists {
		t.Fatalf("取回撤回版本：%v exists=%v", err, exists)
	}
	gotAt, isWithdrawn := found.Assignment.Withdrawn()
	if !isWithdrawn || !gotAt.Equal(withdrawnAt) {
		t.Fatalf("撤回两件没有原样带回：at=%v withdrawn=%v", gotAt, isWithdrawn)
	}
	// 撤回形成的版本不带变化时刻——两条路互斥，合用一个时刻会让两种版本读起来一样。
	if _, wasRevised := found.Assignment.RevisedAt(); wasRevised {
		t.Fatal("撤回形成的版本带了变化时刻")
	}
	if _, err := found.Assignment.Withdraw(
		segmentRef(t, domain.NewLoadAssignmentVersion, "v3"), withdrawnAt.Add(time.Hour)); err == nil {
		t.Fatal("读回来的已撤回分配还能再撤一次——往返把它重开了")
	}
}

// TestASecondSaveOfTheSameVersionKeepsTheOriginal 证撞键译`已登记`而不是报错，也不顶替：
// 换对象范围要换版本号，那是变化不是重投。
func TestASecondSaveOfTheSameVersionKeepsTheOriginal(t *testing.T) {
	repository, transactor, _ := newLoadAssignments(t)
	ctx := t.Context()

	mustSaveLoadAssignment(t, transactor, ctx, repository, loadAssignmentRecord(t, "LA-0004", "v1", "parcel-1"))

	var outcome ports.LoadAssignmentSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, loadAssignmentRecord(t, "LA-0004", "v1", "parcel-9"))
		return err
	})
	if outcome != ports.LoadAssignmentVersionAlreadyRegistered {
		t.Fatalf("撞键 outcome = %d, want LoadAssignmentVersionAlreadyRegistered", outcome)
	}

	found, _, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0004", "v1"))
	if err != nil {
		t.Fatalf("取回分配：%v", err)
	}
	members := found.Assignment.Members()
	if len(members) != 1 || members[0].String() != "parcel-1" {
		t.Fatalf("重投顶替了原对象范围：%v", members)
	}
}

// TestLoadAssignmentWritesRefuseToRunOutsideATransaction 证写口无环境事务即拒——版本行与
// 成员行必须同笔落。断言指名 ErrTransactionRequired 而不是 err != nil。
func TestLoadAssignmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newLoadAssignments(t)

	_, err := repository.Save(t.Context(), loadAssignmentRecord(t, "LA-0005", "v1", "parcel-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestLoadAssignmentRollbackLeavesNothingBehind 证首登与它所在的事务同生共死。
func TestLoadAssignmentRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newLoadAssignments(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, loadAssignmentRecord(t, "LA-0006", "v1", "parcel-1")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := repository.FindByKey(ctx, loadAssignmentKeyFixture(t, "tenant-1", "LA-0006", "v1")); err != nil || exists {
		t.Errorf("回滚后分配仍在：exists=%v err=%v", exists, err)
	}
}

// TestLoadAssignmentCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
func TestLoadAssignmentCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newLoadAssignments(t)
	ctx := t.Context()
	mustSaveLoadAssignment(t, transactor, ctx, repository, loadAssignmentRecord(t, "LA-0007", "v1", "parcel-1"))

	base := `INSERT INTO transport_fulfillment.load_assignment
	    (tenant_id, assignment_ref, version, schedule_ref, assigned_at,
	     corrects_version, revised_at, withdrawn, withdrawn_at, recorded_at) VALUES `
	for name, values := range map[string]string{
		"撤了却没有时刻":  `('tenant-1','BAD-1','v1','s',now(),'v0',NULL,true,NULL,now())`,
		"有时刻却没撤":   `('tenant-1','BAD-2','v1','s',now(),'v0',NULL,false,now(),now())`,
		"同时是变化与撤回": `('tenant-1','BAD-3','v2','s',now(),'v1',now(),true,now(),now())`,
		"后继版本没有前身": `('tenant-1','BAD-4','v2','s',now(),NULL,now(),false,NULL,now())`,
		"首版却回指前身":  `('tenant-1','BAD-5','v1','s',now(),'v0',NULL,false,NULL,now())`,
		"前版引用指向自己": `('tenant-1','BAD-6','v2','s',now(),'v2',now(),false,NULL,now())`,
		"变化早于分配":   `('tenant-1','BAD-7','v2','s',now(),'v1',now()-interval '1 day',false,NULL,now())`,
		"撤回早于分配":   `('tenant-1','BAD-8','v2','s',now(),'v1',NULL,true,now()-interval '1 day',now())`,
		"缺班次":      `('tenant-1','BAD-9','v1','',now(),NULL,NULL,false,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}

	// 成员行挂在一个不存在的版本上要撞外键——对象范围不能脱离版本独立存在。
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.load_assignment_member
		     (tenant_id, assignment_ref, version, object_ref, recorded_at)
		 VALUES ('tenant-1','LA-0007','v-not-there','parcel-1',now())`); err == nil {
		t.Fatal("无主的成员行落进去了")
	}
}

func newLoadAssignments(t *testing.T) (*adapter.LoadAssignments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewLoadAssignments(db)
	if err != nil {
		t.Fatalf("构造分配登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustSaveLoadAssignment(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.LoadAssignments,
	record ports.LoadAssignmentRecord,
) {
	t.Helper()
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, record)
		return err
	})
}

func loadAssignmentKeyFixture(t *testing.T, tenant, assignment, version string) ports.LoadAssignmentKey {
	t.Helper()
	return ports.LoadAssignmentKey{
		TenantID:   segmentRef(t, domain.NewTenantID, tenant),
		Assignment: segmentRef(t, domain.NewLoadAssignmentReference, assignment),
		Version:    segmentRef(t, domain.NewLoadAssignmentVersion, version),
	}
}

func loadAssignmentRecord(t *testing.T, assignment, version string, members ...string) ports.LoadAssignmentRecord {
	t.Helper()
	objects := make([]domain.CarriedObjectReference, 0, len(members))
	for _, member := range members {
		objects = append(objects, segmentRef(t, domain.NewCarriedObjectReference, member))
	}
	formed, err := domain.FormLoadAssignment(domain.LoadAssignmentSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Assignment: segmentRef(t, domain.NewLoadAssignmentReference, assignment),
		Schedule:   segmentRef(t, domain.NewScheduleReference, "schedule-1"),
		Members:    objects,
		Version:    segmentRef(t, domain.NewLoadAssignmentVersion, version),
		AssignedAt: assignedAtDBFixture,
	})
	if err != nil {
		t.Fatalf("形成分配夹具：%v", err)
	}
	return ports.LoadAssignmentRecord{
		Key:        loadAssignmentKeyFixture(t, "tenant-1", assignment, version),
		Assignment: formed,
		RecordedAt: assignedAtDBFixture.Add(time.Minute),
	}
}
