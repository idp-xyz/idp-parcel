package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 16 证未判触发登记（ADR-0235）：一行留下判断键、原因与触发业务时间；同一关联再记只留先到
// 那一行、不报错；原因在库里封闭；无事务拒写，回滚无痕。

var notJudgedTriggerAt = time.Date(2026, 8, 21, 14, 30, 0, 0, time.UTC)

type notJudgedRow struct {
	customer, request, baseline, parcel, purpose, reason string
	triggerAt, settledAt                                 time.Time
}

func notJudgedRowOf(t *testing.T, pool *pgxpool.Pool, tenant, correlation string) (notJudgedRow, bool) {
	t.Helper()
	var row notJudgedRow
	err := pool.QueryRow(t.Context(),
		`SELECT customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
		        reason, trigger_occurred_at, settled_at
		   FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = $1 AND correlation_id = $2`, tenant, correlation,
	).Scan(&row.customer, &row.request, &row.baseline, &row.parcel, &row.purpose, &row.reason, &row.triggerAt, &row.settledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return notJudgedRow{}, false
	}
	if err != nil {
		t.Fatalf("读未判行：%v", err)
	}
	return row, true
}

func notJudged(t *testing.T, key domain.InitialRouteJudgmentKey, correlation string, reason ports.NotJudgedReason) ports.NotJudgedTrigger {
	t.Helper()
	return ports.NotJudgedTrigger{
		Correlation:       scalar(t, domain.NewRequestCorrelationID, correlation),
		Key:               key,
		Reason:            reason,
		TriggerOccurredAt: notJudgedTriggerAt,
		SettledAt:         reassessedAtTS,
	}
}

// Covers: AT-NR-074（登记册那一半）——一行留下判断键各维、原因与触发业务时间；同一关联再记一次（另一格原因）撞键不写、
// 不报错，先到那一行不被改写，事务照常可用。
func TestANotJudgedTriggerKeepsItsFirstReason(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, notJudged(t, key, "trigger-nj-1", ports.NotJudgedNoRoutingHistory))
	})
	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := reassessments.RecordNotJudged(txCtx,
			notJudged(t, key, "trigger-nj-1", ports.NotJudgedEvidenceNotConfigured)); err != nil {
			return err
		}
		_, _, err := reassessments.FindByCorrelation(txCtx, key.TenantID, scalar(t, domain.NewRequestCorrelationID, "trigger-nj-1"))
		return err
	})

	row, present := notJudgedRowOf(t, pool, "tenant-1", "trigger-nj-1")
	if !present {
		t.Fatal("未判行没落库")
	}
	if row.reason != "NO_ROUTING_HISTORY" || row.customer != "customer-a" || row.request != "request-1" ||
		row.baseline != "baseline-v1" || row.parcel != "parcel-1" || row.purpose != "LAST_MILE_DELIVERY" ||
		!row.triggerAt.Equal(notJudgedTriggerAt) || !row.settledAt.Equal(reassessedAtTS) {
		t.Fatalf("未判行 = %+v, want 先到那一格原因、判断键与两个时刻原样", row)
	}
	if _, present, err := reassessments.FindByCorrelation(ctx, key.TenantID,
		scalar(t, domain.NewRequestCorrelationID, "trigger-nj-1")); err != nil || present {
		t.Fatalf("未判不是复核结论，复核册里却有：present=%v err=%v", present, err)
	}
}

// Covers: 未判原因在库里封闭（ports.NotJudgedReason）：绕过适配器直插的词汇表外原因被 CHECK 拒；适配器拒交零值原因。
func TestTheNotJudgedReasonIsClosedInTheDatabase(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, settled_at)
		 VALUES ('tenant-1', 'trigger-nj-2', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'SOMETHING_ELSE', now(), now())`,
	); err == nil {
		t.Fatal("词汇表外的原因进了库")
	}
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx,
			notJudged(t, routeKey(t, "tenant-1", "parcel-1"), "trigger-nj-3", ports.NotJudgedReasonInvalid))
	})
	if err == nil {
		t.Fatal("零值原因写进了未判登记")
	}
}

// Covers: 未判原因与指纹、inbox 入账同笔提交：无事务拒写，回滚无痕。
func TestANotJudgedTriggerIsWrittenOnlyInsideATransaction(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	if err := reassessments.RecordNotJudged(ctx, notJudged(t, key, "trigger-nj-4", ports.NotJudgedNoRoutingHistory)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记未判应返回 ErrTransactionRequired，实得：%v", err)
	}
	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := reassessments.RecordNotJudged(txCtx, notJudged(t, key, "trigger-nj-4", ports.NotJudgedNoRoutingHistory)); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, present := notJudgedRowOf(t, pool, "tenant-1", "trigger-nj-4"); present {
		t.Error("回滚后未判行仍在")
	}
}
