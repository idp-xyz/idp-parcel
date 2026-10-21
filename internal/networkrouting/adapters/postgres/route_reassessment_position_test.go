package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 16 证复核按业务时间排的库面（ADR-0235）：复核行带所判触发的业务时间并原样读回；按判断键取
// 最近一次，别的判断键不掺进来；缺触发业务时间的行写不进；「早于当前位置」的未判必带当前位置时刻且必早于它；取最近
// 一次时按判断键串行到事务结束。

func stillApplicableAt(t *testing.T, key domain.InitialRouteJudgmentKey, correlation string, at time.Time) ports.ReassessmentRecord {
	t.Helper()
	return ports.ReassessmentRecord{
		Correlation:       scalar(t, domain.NewRequestCorrelationID, correlation),
		Key:               key,
		Conclusion:        ports.ReassessmentStillApplicable,
		ReviewedPlan:      scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		TriggerOccurredAt: at,
		ReassessedAt:      reassessedAtTS,
	}
}

func latestAt(t *testing.T, ctx context.Context, store interface {
	LatestTriggerAt(context.Context, domain.InitialRouteJudgmentKey) (time.Time, bool, error)
}, key domain.InitialRouteJudgmentKey) (time.Time, bool) {
	t.Helper()
	at, found, err := store.LatestTriggerAt(ctx, key)
	if err != nil {
		t.Fatalf("取最近一次控制触发：%v", err)
	}
	return at, found
}

// Covers: AT-NR-079（登记册那一半）——复核行带所判触发的业务时间、原样读回；同一判断键取最晚那一次，别的包裹的复核不
// 掺进来；没有复核的键答没有当前位置。新行缺触发业务时间，适配器拒、库也拒。
func TestTheLatestTriggerIsReadPerJudgmentKey(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	other := routeKey(t, "tenant-1", "parcel-2")
	early, late := notJudgedTriggerAt.Add(-2*time.Hour), notJudgedTriggerAt

	saveReassessment(t, transactor, ctx, reassessments, scalar(t, domain.NewRequestCorrelationID, "trigger-p-late"),
		stillApplicableAt(t, key, "trigger-p-late", late))
	saveReassessment(t, transactor, ctx, reassessments, scalar(t, domain.NewRequestCorrelationID, "trigger-p-early"),
		stillApplicableAt(t, key, "trigger-p-early", early))
	saveReassessment(t, transactor, ctx, reassessments, scalar(t, domain.NewRequestCorrelationID, "trigger-p-other"),
		stillApplicableAt(t, other, "trigger-p-other", late.Add(24*time.Hour)))

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if at, found := latestAt(t, txCtx, reassessments, key); !found || !at.Equal(late) {
			t.Fatalf("最近一次 = %s（found %v）, want %s", at, found, late)
		}
		if _, found := latestAt(t, txCtx, reassessments, routeKey(t, "tenant-1", "parcel-none")); found {
			t.Fatal("没有复核的键答出了当前位置")
		}
		return nil
	})
	readBack, present, err := reassessments.FindByCorrelation(ctx, key.TenantID, scalar(t, domain.NewRequestCorrelationID, "trigger-p-early"))
	if err != nil || !present || !readBack.TriggerOccurredAt.Equal(early) {
		t.Fatalf("读回的触发业务时间 = %s（present %v err %v）, want %s", readBack.TriggerOccurredAt, present, err, early)
	}

	missing := stillApplicableAt(t, key, "trigger-p-missing", time.Time{})
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := reassessments.Save(txCtx, missing.Correlation, missing)
		return err
	}); err == nil {
		t.Fatal("缺触发业务时间的复核写进了库")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, reassessed_at)
		 VALUES ('tenant-1', 'trigger-p-raw', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-1', 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001', now())`); err == nil {
		t.Fatal("绕过适配器直插的新行缺触发业务时间也进了库")
	}
	if _, _, err := reassessments.LatestTriggerAt(ctx, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务取最近一次应返回 ErrTransactionRequired（锁要持到事务结束），实得：%v", err)
	}
}

// Covers: 「早于当前位置」的未判必带当前位置时刻且必早于它，别的原因不带——适配器与库各拒一次。
func TestABehindTriggerCarriesTheCurrentPositionItWasComparedWith(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	behind := notJudged(t, key, "trigger-p-behind", ports.NotJudgedTriggerPrecedesCurrentPosition)
	behind.CurrentPositionAt = notJudgedTriggerAt.Add(time.Hour)
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, behind)
	})
	var positionAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT current_position_at FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = 'tenant-1' AND correlation_id = 'trigger-p-behind'`).Scan(&positionAt); err != nil ||
		!positionAt.Equal(behind.CurrentPositionAt) {
		t.Fatalf("当前位置时刻 = %s（err %v）, want %s", positionAt, err, behind.CurrentPositionAt)
	}

	for name, record := range map[string]ports.NotJudgedTrigger{
		"早于当前位置却没带时刻": notJudged(t, key, "trigger-p-bare", ports.NotJudgedTriggerPrecedesCurrentPosition),
		"别的原因带了时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-p-extra", ports.NotJudgedNoRoutingHistory)
			record.CurrentPositionAt = notJudgedTriggerAt.Add(time.Hour)
			return record
		}(),
	} {
		if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return reassessments.RecordNotJudged(txCtx, record)
		}); err == nil {
			t.Errorf("%s：写进了未判登记", name)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, current_position_at, settled_at)
		 VALUES ('tenant-1', 'trigger-p-raw', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_CURRENT_POSITION', now(), now() - interval '1 hour', now())`,
	); err == nil {
		t.Fatal("当前位置不晚于触发的「早于当前位置」进了库")
	}
}

// Covers: AT-NR-083——取最近一次控制触发时按判断键串行到事务结束：一笔事务取了之后、提交之前，同一判断键的另一笔等在
// 锁上；前一笔落了更晚的复核并提交，后一笔读到的正是它。两路触发在两笔并发事务里到达，较早的那次因此不会与较新的
// 那次双双判过。
func TestReadingTheLatestTriggerSerializesPerJudgmentKey(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	held, release, firstDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	record := stillApplicableAt(t, key, "trigger-p-first", notJudgedTriggerAt)

	go func() {
		firstDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, _, err := reassessments.LatestTriggerAt(txCtx, key); err != nil {
				return err
			}
			close(held)
			<-release
			_, err := reassessments.Save(txCtx, record.Correlation, record)
			return err
		})
	}()
	<-held

	secondSaw := make(chan time.Time, 1)
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			at, _, err := reassessments.LatestTriggerAt(txCtx, key)
			secondSaw <- at
			return err
		})
	}()
	select {
	case at := <-secondSaw:
		t.Fatalf("前一笔还没提交，同一判断键的后一笔就读到了 %s——没等锁", at)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("前一笔：%v", err)
	}
	if at := <-secondSaw; !at.Equal(notJudgedTriggerAt) {
		t.Fatalf("后一笔读到 %s, want 前一笔刚提交的 %s", at, notJudgedTriggerAt)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("后一笔：%v", err)
	}
}
