package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

var applicabilityAt = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

func TestPlanApplicabilityRoundTripsAndSaveRewritesStateNotIdentity(t *testing.T) {
	store, transactor, _ := newApplicabilityStore(t)
	ctx := t.Context()

	current, err := domain.EstablishPlanApplicability(
		scalar(t, domain.NewRoutePlanVersionID, "plan-1/v1"), applicabilityAt)
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return store.Save(txCtx, current)
	})

	found, exists, err := store.FindByPlan(ctx, current.Plan())
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.State() != domain.PlanCurrentlyEffective {
		t.Fatalf("state = %q", found.State())
	}

	lapsed, err := found.Lapse(
		scalar(t, domain.NewApplicabilityBasisReference, "LINE-CLOSED/NET-ADJ-7"),
		applicabilityAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("lapse: %v", err)
	}
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return store.Save(txCtx, lapsed)
	})

	after, _, err := store.FindByPlan(ctx, current.Plan())
	if err != nil {
		t.Fatalf("换态后读回：%v", err)
	}
	if after.State() != domain.PlanLapsed || after.Plan().String() != "plan-1/v1" {
		t.Fatal("换态改写了计划身份或没落失效")
	}
	basis, ok := after.Basis()
	if !ok || basis.String() != "LINE-CLOSED/NET-ADJ-7" {
		t.Fatal("失效依据没有落库")
	}
}

func TestPlanApplicabilityWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _, _ := newApplicabilityStore(t)
	current, err := domain.EstablishPlanApplicability(
		scalar(t, domain.NewRoutePlanVersionID, "plan-ntx"), applicabilityAt)
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if err := store.Save(t.Context(), current); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存适用性应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestPlanApplicabilityCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newApplicabilityStore(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis)
		 VALUES ('plan-bad-1', 'CURRENTLY_EFFECTIVE', now(), 'should-not')`); err == nil {
		t.Fatal("一行「当前有效却带着依据」溜进了适用性库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis)
		 VALUES ('plan-bad-2', 'SUPERSEDED', now(), 'basis-1')`); err == nil {
		t.Fatal("一行「已被替代却没有接班」溜进了适用性库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis, successor)
		 VALUES ('plan-bad-3', 'LAPSED', now(), 'basis-1', 'plan-2')`); err == nil {
		t.Fatal("一行「已失效却带着接班」溜进了适用性库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis, successor)
		 VALUES ('plan-self', 'SUPERSEDED', now(), 'basis-1', 'plan-self')`); err == nil {
		t.Fatal("一行「自代」溜进了适用性库")
	}
}

func newApplicabilityStore(t *testing.T) (*adapter.PlanApplicabilities, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewPlanApplicabilities(db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	return store, db.Transactor(), pool
}
