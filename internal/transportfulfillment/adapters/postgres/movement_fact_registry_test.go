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

// 本文件对真实 PostgreSQL 16 证实际移动事实登记：版本链上多版共存、门禁放行依据只挂得上出发、
// 撞键译`已登记`且不顶替、无事务拒，以及库内 CHECK 挡住领域造不出的行。夹具全为合成登记（S 级）。

var movedAtDBFixture = time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)

// TestAMovementFactRoundTrips 证一条事实原样往返，且到达不带门禁依据。
func TestAMovementFactRoundTrips(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0001", "v1", domain.ArrivalFact, ""))

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0001", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	if found.Fact.Kind() != domain.ArrivalFact {
		t.Fatalf("种类没有原样带回：%q", found.Fact.Kind())
	}
	if found.Fact.Location().String() != "hub-1" || found.Fact.Source().String() != "carrier-scan/v1" {
		t.Fatalf("地点或来源没有原样带回：%q / %q", found.Fact.Location(), found.Fact.Source())
	}
	if !found.Fact.OccurredAt().Equal(movedAtDBFixture) {
		t.Fatalf("发生时刻没有原样带回：%s", found.Fact.OccurredAt())
	}
	if _, gated := found.Fact.GateClearance(); gated {
		t.Fatal("到达读回来带了门禁放行依据")
	}
	if _, has := found.Fact.Corrects(); has {
		t.Fatal("首版读回来带了前身")
	}
}

// TestAClearedDepartureKeepsItsClearance 证受管出发的放行依据随行保全——本上下文只认它的
// 引用、不重建门禁机制，所以那一格必须原样存回。
func TestAClearedDepartureKeepsItsClearance(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	mustSaveMovementFact(t, transactor, ctx, repository,
		movementFactRecord(t, "MF-0002", "v1", domain.DepartureFact, "cc-clearance/v1"))

	found, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0002", "v1"))
	if err != nil || !exists {
		t.Fatalf("取回事实：%v exists=%v", err, exists)
	}
	clearance, gated := found.Fact.GateClearance()
	if !gated || clearance.String() != "cc-clearance/v1" {
		t.Fatalf("放行依据没有原样带回：%q gated=%v", clearance, gated)
	}
}

// TestASecondSaveOfTheSameFactVersionKeepsTheOriginal 证撞键译`已登记`且不顶替：更正走新
// 版本，原记录与其派生历史不被改写。
func TestASecondSaveOfTheSameFactVersionKeepsTheOriginal(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()

	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0003", "v1", domain.InTransitFact, ""))

	other := movementFactRecord(t, "MF-0003", "v1", domain.InTransitFact, "")
	var outcome ports.MovementFactSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, other)
		return err
	})
	if outcome != ports.MovementFactVersionAlreadyRegistered {
		t.Fatalf("撞键 outcome = %d, want MovementFactVersionAlreadyRegistered", outcome)
	}
}

// TestMovementFactWritesRefuseToRunOutsideATransaction 证写口无环境事务即拒。断言指名
// ErrTransactionRequired 而不是 err != nil。
func TestMovementFactWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newMovementFacts(t)

	_, err := repository.Save(t.Context(), movementFactRecord(t, "MF-0004", "v1", domain.ArrivalFact, ""))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestMovementFactRollbackLeavesNothingBehind 证首登与它所在的事务同生共死。
func TestMovementFactRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newMovementFacts(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, movementFactRecord(t, "MF-0005", "v1", domain.ArrivalFact, "")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := repository.FindByKey(ctx, movementFactKeyFixture(t, "tenant-1", "MF-0005", "v1")); err != nil || exists {
		t.Errorf("回滚后事实仍在：exists=%v err=%v", exists, err)
	}
}

// TestMovementFactCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门。
//
// 「到达挂了放行依据」那一格尤其要有：领域构造门已拒，但**绕过构造门的写入路径同样不该落进
// 一个不存在的门**。
func TestMovementFactCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newMovementFacts(t)
	ctx := t.Context()
	mustSaveMovementFact(t, transactor, ctx, repository, movementFactRecord(t, "MF-0006", "v1", domain.ArrivalFact, ""))

	base := `INSERT INTO transport_fulfillment.transport_movement_fact
	    (tenant_id, fact_ref, version, schedule_ref, kind, location_ref, source_ref,
	     occurred_at, gate_clearance, corrects_version, corrected_at, recorded_at) VALUES `
	for name, values := range map[string]string{
		"集外事实种类":  `('tenant-1','BAD-1','v1','s','DIVERTED','l','src',now(),NULL,NULL,NULL,now())`,
		"到达挂放行依据": `('tenant-1','BAD-2','v1','s','ARRIVAL','l','src',now(),'cc/v1',NULL,NULL,now())`,
		"移动挂放行依据": `('tenant-1','BAD-3','v1','s','IN_TRANSIT','l','src',now(),'cc/v1',NULL,NULL,now())`,
		"更正只有前身":  `('tenant-1','BAD-4','v2','s','ARRIVAL','l','src',now(),NULL,'v1',NULL,now())`,
		"更正只有时刻":  `('tenant-1','BAD-5','v2','s','ARRIVAL','l','src',now(),NULL,NULL,now(),now())`,
		"前身指向自己":  `('tenant-1','BAD-6','v2','s','ARRIVAL','l','src',now(),NULL,'v2',now(),now())`,
		"更正早于发生":  `('tenant-1','BAD-7','v2','s','ARRIVAL','l','src',now(),NULL,'v1',now()-interval '1 day',now())`,
		"缺来源":     `('tenant-1','BAD-8','v1','s','ARRIVAL','l','',now(),NULL,NULL,NULL,now())`,
		"缺地点":     `('tenant-1','BAD-9','v1','s','ARRIVAL','','src',now(),NULL,NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}

func newMovementFacts(t *testing.T) (*adapter.MovementFacts, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewMovementFacts(db)
	if err != nil {
		t.Fatalf("构造移动事实登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustSaveMovementFact(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.MovementFacts,
	record ports.MovementFactRecord,
) {
	t.Helper()
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, record)
		return err
	})
}

func movementFactKeyFixture(t *testing.T, tenant, fact, version string) ports.MovementFactKey {
	t.Helper()
	return ports.MovementFactKey{
		TenantID: segmentRef(t, domain.NewTenantID, tenant),
		Fact:     segmentRef(t, domain.NewMovementFactReference, fact),
		Version:  segmentRef(t, domain.NewMovementFactVersion, version),
	}
}

func movementFactRecord(
	t *testing.T,
	fact, version string,
	kind domain.MovementFactKind,
	clearance string,
) ports.MovementFactRecord {
	t.Helper()
	spec := domain.MovementFactSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Fact:       segmentRef(t, domain.NewMovementFactReference, fact),
		Schedule:   segmentRef(t, domain.NewScheduleReference, "schedule-1"),
		Kind:       kind,
		Location:   segmentRef(t, domain.NewMovementLocationReference, "hub-1"),
		Source:     segmentRef(t, domain.NewMovementSourceReference, "carrier-scan/v1"),
		Version:    segmentRef(t, domain.NewMovementFactVersion, version),
		OccurredAt: movedAtDBFixture,
	}
	if clearance != "" {
		spec.GateRequired = true
		spec.GateClearance = segmentRef(t, domain.NewGateClearanceReference, clearance)
	}
	recorded, err := domain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("形成事实夹具：%v", err)
	}
	return ports.MovementFactRecord{
		Key:        movementFactKeyFixture(t, "tenant-1", fact, version),
		Fact:       recorded,
		RecordedAt: movedAtDBFixture.Add(time.Minute),
	}
}
