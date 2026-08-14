package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证治理意图：与业务行同一提交、回滚一并消失、重发同一份、
// 无事务拒、缺席/混装响亮报错。信封 ID 由暂停标识 / 被恢复的暂停 / 接管区间四维认领。
// 入队走 EnqueueOnce。

type governanceHandoffClock struct{ at time.Time }

func (clock governanceHandoffClock) Now() time.Time { return clock.at }

type governanceHandoffFixture struct {
	suspensions *adapter.Suspensions
	resumptions *adapter.Resumptions
	takeovers   *adapter.Takeovers
	handoff     *adapter.OutboxGovernanceHandoff
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newGovernanceHandoffFixture(t *testing.T) *governanceHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	suspensions, err := adapter.NewSuspensions(db)
	if err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	resumptions, err := adapter.NewResumptions(db)
	if err != nil {
		t.Fatalf("构造恢复库：%v", err)
	}
	takeovers, err := adapter.NewTakeovers(db)
	if err != nil {
		t.Fatalf("构造接管库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxGovernanceHandoff(db, store, governanceHandoffClock{
		at: time.Date(2026, 8, 14, 21, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &governanceHandoffFixture{
		suspensions: suspensions,
		resumptions: resumptions,
		takeovers:   takeovers,
		handoff:     handoff,
		transactor:  db.Transactor(),
		pool:        pool,
	}
}

func (fixture *governanceHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func TestGovernanceSuspensionIntentCommitsAtomically(t *testing.T) {
	fixture := newGovernanceHandoffFixture(t)
	ctx := t.Context()
	decision := incidentSuspension(t, "suspension-1")
	intent := ports.GovernanceHandoffIntent{Suspension: &decision}
	eventID := "suspension/suspension-1"

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.suspensions.Save(txCtx, decision); err != nil {
			return err
		}
		return fixture.handoff.HandOffGovernance(txCtx, intent)
	})

	if _, exists, err := fixture.suspensions.FindByID(ctx, decision.ID()); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countGovernanceIntents(t, fixture.pool, eventID, "pilot-governance.suspension.recorded"); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
}

func TestGovernanceSuspensionIntentRollbackDropsBoth(t *testing.T) {
	fixture := newGovernanceHandoffFixture(t)
	ctx := t.Context()
	decision := incidentSuspension(t, "suspension-1")
	intent := ports.GovernanceHandoffIntent{Suspension: &decision}
	eventID := "suspension/suspension-1"
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.suspensions.Save(txCtx, decision); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffGovernance(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.suspensions.FindByID(ctx, decision.ID()); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countGovernanceIntents(t, fixture.pool, eventID, "pilot-governance.suspension.recorded"); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

func TestResendingTheSameGovernanceIntentIsIdempotent(t *testing.T) {
	fixture := newGovernanceHandoffFixture(t)
	ctx := t.Context()

	suspension := incidentSuspension(t, "suspension-1")
	resumption := incidentResumption(t, "suspension-1")
	takeover := incidentTakeover(t, incidentInterval(true))

	cases := []struct {
		name     string
		intent   ports.GovernanceHandoffIntent
		eventID  string
		eventTyp string
	}{
		{
			name:     "suspension",
			intent:   ports.GovernanceHandoffIntent{Suspension: &suspension},
			eventID:  "suspension/suspension-1",
			eventTyp: "pilot-governance.suspension.recorded",
		},
		{
			name:     "resumption",
			intent:   ports.GovernanceHandoffIntent{Resumption: &resumption},
			eventID:  "resumption/suspension-1",
			eventTyp: "pilot-governance.resumption.recorded",
		},
		{
			name:     "takeover",
			intent:   ports.GovernanceHandoffIntent{Takeover: &takeover},
			eventID:  "takeover/lane-1-parcels/shipment-intake/acceptance-decision",
			eventTyp: "pilot-governance.takeover.recorded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture.within(t, ctx, func(txCtx context.Context) error {
				return fixture.handoff.HandOffGovernance(txCtx, tc.intent)
			})
			fixture.within(t, ctx, func(txCtx context.Context) error {
				return fixture.handoff.HandOffGovernance(txCtx, tc.intent)
			})
			if count := countGovernanceIntents(t, fixture.pool, tc.eventID, tc.eventTyp); count != 1 {
				t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
			}
		})
	}
}

func TestGovernanceIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newGovernanceHandoffFixture(t)
	decision := incidentSuspension(t, "suspension-1")
	if err := fixture.handoff.HandOffGovernance(t.Context(), ports.GovernanceHandoffIntent{Suspension: &decision}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countGovernanceIntents(t, fixture.pool, "suspension/suspension-1", "pilot-governance.suspension.recorded"); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignGovernanceIntentIsLoud(t *testing.T) {
	fixture := newGovernanceHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGovernance(txCtx, ports.GovernanceHandoffIntent{})
	}); err == nil {
		t.Fatal("空意图必须响亮报错")
	}

	suspension := incidentSuspension(t, "suspension-1")
	resumption := incidentResumption(t, "suspension-1")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGovernance(txCtx, ports.GovernanceHandoffIntent{
			Suspension: &suspension,
			Resumption: &resumption,
		})
	}); err == nil {
		t.Fatal("混装意图必须响亮报错")
	}

	blank := domain.TakeoverRecord{}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffGovernance(txCtx, ports.GovernanceHandoffIntent{Takeover: &blank})
	}); err == nil {
		t.Fatal("缺区间身份的接管必须响亮报错")
	}
}

func countGovernanceIntents(t *testing.T, pool *pgxpool.Pool, eventID, eventType string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, eventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}
