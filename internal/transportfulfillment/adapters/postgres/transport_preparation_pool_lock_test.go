package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-331（票 demo-chain-gaps/127，ADR-0203 越权风险点 7）——准备评估在整封信封那一笔事务里读池，不持池行锁：准备那一笔
// 还没提交时，另一笔事务按池身份锁读（预占、释放的那一口）与按班次锁读（装载分配的那一口）都在锁等待时限内读完。对照：握着的
// 一方换成装载分配那一口的锁读，同一个探针等到时限、答 55P03——探针抓得到锁，上一格的「读完」才算数。
func TestPreparationHoldsNoPoolRowLockWhileItsTransactionIsOpen(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	template := syntheticTemplateCommand(tenant, "SYN-TPL-OK", "SYN-CONN-OK", "22:00")
	template.Capacities = []application.CapacityDeclarationInput{
		{Dimension: "WEIGHT", Unit: "T", Capacity: 5},
		{Dimension: "PIECES", Unit: "PCS", Capacity: 100},
	}
	wantScheduleOutcome(t, stores.registerTemplate(t, template), application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-OK", RunningDate: "2026-10-21",
	}), application.TransportScheduleRegistered)
	schedule := deliveryValue(t, domain.NewScheduleReference, "SYN-TPL-OK/2026-10-21")
	weightPool := ports.CapacityPoolKey{TenantID: tenant, Pool: deliveryValue(t, domain.NewCapacityPoolReference, "SYN-TPL-OK/2026-10-21/WEIGHT")}

	// probe 是写编排那一侧：锁等待超过 300ms 就报 55P03，不无限期等。两种锁读都问一遍。
	probe := func() error {
		return stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			executor, err := stores.db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.Exec(txCtx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
				return err
			}
			if _, found, err := stores.pools.FindByKey(txCtx, weightPool); err != nil {
				return err
			} else if !found {
				return errors.New("weight pool not found by key")
			}
			listed, err := stores.pools.ListPoolsOnSchedules(txCtx, tenant, []domain.ScheduleReference{schedule})
			if err == nil && len(listed) != 2 {
				return errors.New("the schedule should carry two pools")
			}
			return err
		})
	}

	command := application.PrepareOnPlannedLegsCommand{
		TenantID: tenant, Object: "SYN-PARCEL-01", Route: application.PlannedRouteFormed,
		Legs: []domain.PreparationLeg{
			preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-OK", departure.Add(-time.Hour), departure.Add(12*time.Hour)),
		},
	}
	var prepared application.PrepareOnPlannedLegsResult
	if err := probeWhileHolding(t, stores.transactor, func(txCtx context.Context) error {
		var err error
		prepared, err = stores.prepare.Prepare(txCtx, command)
		return err
	}, probe); err != nil {
		t.Errorf("准备那一笔未提交时写编排读不到池：%v", err)
	}
	// 准备确实读了那两个池：选中的是登记的那一班，依据里两池俱在。读不到池就没有东西可锁，上一格也就不算数。
	if prepared.Outcome() != application.PreparationRunAnswered || len(prepared.Legs()) != 1 {
		t.Fatalf("准备 = %s（%s）", prepared.Outcome(), prepared.UndecidedReason())
	}
	leg := prepared.Legs()[0].Record.Preparation
	if leg.Outcome() != domain.PreparationOpportunityPrepared || len(leg.Candidates()) != 1 ||
		!leg.Candidates()[0].Registered() || len(leg.Candidates()[0].Pools) != 2 {
		t.Fatalf("准备没按登记册上的两池判：%s %+v", leg.Outcome(), leg.Candidates())
	}

	err := probeWhileHolding(t, stores.transactor, func(txCtx context.Context) error {
		_, err := stores.pools.ListPoolsOnSchedules(txCtx, tenant, []domain.ScheduleReference{schedule})
		return err
	}, probe)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Errorf("握着装载分配那一口的锁读时探针 err = %v, want 等到时限（55P03）", err)
	}
}

// Covers: AT-TF-331「读到一笔释放的半截」那一支——不锁的读一个池分几条语句，两条之间恰有一笔释放提交时，预占上的已释放与读到的
// 释放明细对不上。这里把那个半截直接摆进库（已释放记 1、释放明细一条没有）：准备答池册读不动、零写入，不按半截的池作答。
func TestAPreparationThatReadsAPoolHalfwayThroughAReleaseAnswersUndecided(t *testing.T) {
	stores := newPreparationStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	template := syntheticTemplateCommand(tenant, "SYN-TPL-OK", "SYN-CONN-OK", "22:00")
	template.Capacities = []application.CapacityDeclarationInput{{Dimension: "WEIGHT", Unit: "T", Capacity: 5}}
	wantScheduleOutcome(t, stores.registerTemplate(t, template), application.ScheduleTemplateRegistered)
	wantScheduleOutcome(t, stores.registerSchedule(t, application.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-TPL-OK", RunningDate: "2026-10-21",
	}), application.TransportScheduleRegistered)
	var reserved application.OpportunityResult
	if err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		reserved, err = stores.opportunity.Reserve(txCtx, application.ReserveCapacityCommand{
			TenantID: tenant, Pool: "SYN-TPL-OK/2026-10-21/WEIGHT", Reservation: "SYN-RES-1", Quantity: 1, Unit: "T",
			SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-1/v1",
			ValidUntil: departure.Add(48 * time.Hour), At: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
		})
		return err
	}); err != nil || reserved.Outcome() != application.CapacityReserved {
		t.Fatalf("预占：%s %v", reserved.Outcome(), err)
	}
	if _, err := stores.pool.Exec(t.Context(),
		`UPDATE transport_fulfillment.capacity_reservation SET released = 1 WHERE reservation_id = 'SYN-RES-1'`); err != nil {
		t.Fatalf("摆出半截：%v", err)
	}

	result := stores.run(t, application.PrepareOnPlannedLegsCommand{
		TenantID: tenant, Object: "SYN-PARCEL-01", Route: application.PlannedRouteFormed,
		Legs: []domain.PreparationLeg{
			preparationLeg(t, "SYN-RPV-1#1", "SYN-CONN-OK", departure.Add(-time.Hour), departure.Add(12*time.Hour)),
		},
	})
	if result.Outcome() != application.PreparationRunUndecided || result.UndecidedReason() != application.CapacityPoolsUnavailable {
		t.Fatalf("半截的池 = %s（%s），want 未决、池册读不动", result.Outcome(), result.UndecidedReason())
	}
	if rows, err := stores.catalogue.ListTransportPreparations(t.Context(), tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("按半截的池落了结果：%d %v", len(rows), err)
	}
}

// probeWhileHolding 另起一笔事务跑 hold、不提交，趁它开着跑 probe，交回 probe 的结果；之后放 hold 那一笔提交。
func probeWhileHolding(t *testing.T, transactor bentoapp.Transactor, hold func(txCtx context.Context) error, probe func() error) error {
	t.Helper()
	holding, release, held := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		held <- transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			if err := hold(txCtx); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	var once sync.Once
	letGo := func() { once.Do(func() { close(release) }) }
	t.Cleanup(letGo)
	select {
	case <-holding:
	case err := <-held:
		t.Fatalf("握着的一方没开着就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("握着的一方没在时限内跑完")
	}
	probed := probe()
	letGo()
	if err := <-held; err != nil {
		t.Fatalf("握着的一方：%v", err)
	}
	return probed
}
