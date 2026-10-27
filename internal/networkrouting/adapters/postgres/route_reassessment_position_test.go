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
// 最近一次，别的判断键不掺进来；缺触发业务时间的行写不进；「早于当前位置」的未判必带当前位置时刻且必早于它，「早于委托
// 接受」的未判必带委托接受时刻且不晚于它（ADR-0279）；取最近一次时按判断键串行到事务结束。

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

// Covers: AT-NR-087（登记册那一半）——「早于委托接受」的未判必带委托接受时刻且不晚于它（舍入到微秒后可以相等），别的原因
// 不带，也不与当前位置时刻混用——适配器与库各拒一次。
func TestATriggerBeforeTheAcceptanceCarriesTheAcceptanceItWasComparedWith(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	before := notJudged(t, key, "trigger-a-before", ports.NotJudgedTriggerPrecedesShipmentAcceptance)
	before.ShipmentAcceptedAt = notJudgedTriggerAt.Add(time.Hour)
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, before)
	})
	var reason string
	var acceptedAt time.Time
	var positionAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT reason, shipment_accepted_at, current_position_at FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = 'tenant-1' AND correlation_id = 'trigger-a-before'`).Scan(&reason, &acceptedAt, &positionAt); err != nil ||
		reason != "TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE" || !acceptedAt.Equal(before.ShipmentAcceptedAt) || positionAt != nil {
		t.Fatalf("未判行 = %q、接受 %s、当前位置 %v（err %v）, want 早于委托接受 %s、不带当前位置",
			reason, acceptedAt, positionAt, err, before.ShipmentAcceptedAt)
	}

	for name, record := range map[string]ports.NotJudgedTrigger{
		"早于委托接受却没带时刻": notJudged(t, key, "trigger-a-bare", ports.NotJudgedTriggerPrecedesShipmentAcceptance),
		"别的原因带了委托接受时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-a-extra", ports.NotJudgedNoRoutingHistory)
			record.ShipmentAcceptedAt = notJudgedTriggerAt.Add(time.Hour)
			return record
		}(),
		"早于委托接受带的是当前位置时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-a-position", ports.NotJudgedTriggerPrecedesShipmentAcceptance)
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
	// 纳秒上早于接受、舍入到微秒后落在同一刻的那一次照样写得进：编排按纳秒比，库按微秒存。
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, shipment_accepted_at, settled_at)
		 VALUES ('tenant-1', 'trigger-a-same-microsecond', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE', $1, $2, now())`,
		notJudgedTriggerAt.Add(100*time.Nanosecond), notJudgedTriggerAt.Add(400*time.Nanosecond),
	); err != nil {
		t.Fatalf("同一微秒里早于接受的那一次写不进：%v", err)
	}
	for name, insert := range map[string]string{
		"委托接受早于触发": `INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, shipment_accepted_at, settled_at)
		 VALUES ('tenant-1', 'trigger-a-raw', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE', now(), now() - interval '1 microsecond', now())`,
		"别的原因带了委托接受时刻": `INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, shipment_accepted_at, settled_at)
		 VALUES ('tenant-1', 'trigger-a-raw-extra', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'NO_ROUTING_HISTORY', now(), now() + interval '1 hour', now())`,
	} {
		if _, err := pool.Exec(ctx, insert); err == nil {
			t.Errorf("%s：绕过适配器直插进了库", name)
		}
	}
}

// Covers: AT-NR-104（登记册那一半）——「早于计划生效」的未判必带计划生效时刻且不晚于它（舍入到微秒后可以相等），别的原因不带，
// 也不与委托接受、当前位置时刻混用——适配器与库各拒一次。
func TestATriggerBeforeThePlanTakesEffectCarriesTheEffectiveTimeItWasComparedWith(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	before := notJudged(t, key, "trigger-e-before", ports.NotJudgedTriggerPrecedesPlanEffectiveFrom)
	before.PlanEffectiveAt = notJudgedTriggerAt.Add(time.Hour)
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, before)
	})
	var reason string
	var effectiveAt time.Time
	var positionAt, acceptedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT reason, plan_effective_at, current_position_at, shipment_accepted_at
		   FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = 'tenant-1' AND correlation_id = 'trigger-e-before'`).Scan(&reason, &effectiveAt, &positionAt, &acceptedAt); err != nil ||
		reason != "TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM" || !effectiveAt.Equal(before.PlanEffectiveAt) || positionAt != nil || acceptedAt != nil {
		t.Fatalf("未判行 = %q、生效 %s、当前位置 %v、接受 %v（err %v）, want 早于计划生效 %s、不带另两格时刻",
			reason, effectiveAt, positionAt, acceptedAt, err, before.PlanEffectiveAt)
	}

	for name, record := range map[string]ports.NotJudgedTrigger{
		"早于计划生效却没带时刻": notJudged(t, key, "trigger-e-bare", ports.NotJudgedTriggerPrecedesPlanEffectiveFrom),
		"别的原因带了计划生效时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-e-extra", ports.NotJudgedNoRoutingHistory)
			record.PlanEffectiveAt = notJudgedTriggerAt.Add(time.Hour)
			return record
		}(),
		"早于计划生效带的是委托接受时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-e-acceptance", ports.NotJudgedTriggerPrecedesPlanEffectiveFrom)
			record.ShipmentAcceptedAt = notJudgedTriggerAt.Add(time.Hour)
			return record
		}(),
		"早于计划生效带的是当前位置时刻": func() ports.NotJudgedTrigger {
			record := notJudged(t, key, "trigger-e-position", ports.NotJudgedTriggerPrecedesPlanEffectiveFrom)
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
	// 纳秒上早于生效、舍入到微秒后落在同一刻的那一次照样写得进：编排按纳秒比，库按微秒存。
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, plan_effective_at, settled_at)
		 VALUES ('tenant-1', 'trigger-e-same-microsecond', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM', $1, $2, now())`,
		notJudgedTriggerAt.Add(100*time.Nanosecond), notJudgedTriggerAt.Add(400*time.Nanosecond),
	); err != nil {
		t.Fatalf("同一微秒里早于生效的那一次写不进：%v", err)
	}
	for name, insert := range map[string]string{
		"计划生效早于触发": `INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, plan_effective_at, settled_at)
		 VALUES ('tenant-1', 'trigger-e-raw', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM', now(), now() - interval '1 microsecond', now())`,
		"别的原因带了计划生效时刻": `INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, plan_effective_at, settled_at)
		 VALUES ('tenant-1', 'trigger-e-raw-extra', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'NO_ROUTING_HISTORY', now(), now() + interval '1 hour', now())`,
		"早于计划生效没带时刻": `INSERT INTO network_routing.route_reassessment_not_judged
			(tenant_id, correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, reason, trigger_occurred_at, settled_at)
		 VALUES ('tenant-1', 'trigger-e-raw-bare', 'customer-a', 'request-1', 'baseline-v1',
		         'parcel-1', 'LAST_MILE_DELIVERY', 'TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM', now(), now())`,
	} {
		if _, err := pool.Exec(ctx, insert); err == nil {
			t.Errorf("%s：绕过适配器直插进了库", name)
		}
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
