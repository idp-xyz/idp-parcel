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
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 证复核所据的控制依据与位置：两列往返、同有同无与封闭集、新的未判原因写得进，
// 以及「最近控制位置」「等位置的实测变化」两种读法。

func positionOf(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	store *adapter.RouteReassessments,
	key domain.InitialRouteJudgmentKey,
) ports.ControlPosition {
	t.Helper()
	var got ports.ControlPosition
	within(t, transactor, ctx, func(txCtx context.Context) error {
		position, err := store.LatestControlPosition(txCtx, key)
		if err != nil {
			return err
		}
		got = position
		return nil
	})
	return got
}

func anchored(
	t *testing.T,
	key domain.InitialRouteJudgmentKey,
	correlation string,
	control domain.ControlEvidenceKind,
	location string,
	at time.Time,
) ports.ReassessmentRecord {
	t.Helper()
	record := stillApplicableAt(t, key, correlation, at)
	record.TriggerControl = control
	record.TriggerLocation = scalar(t, domain.NewActualLocationReference, location)
	return record
}

// Covers: 控制依据与位置随复核往返；两列同有同无，有值时控制依据在封闭集内、位置非空白。新行两列都空写不进。
func TestTriggerControlAndLocationRoundTripAndStayPaired(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-cp", "parcel-cp")
	at := notJudgedTriggerAt

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-handover"),
		anchored(t, key, "trigger-cp-handover", domain.TransportHandoverControl, "node-hub", at))
	found, present, err := reassessments.FindByCorrelation(ctx, key.TenantID,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-handover"))
	if err != nil || !present || found.TriggerControl != domain.TransportHandoverControl ||
		found.TriggerLocation.String() != "node-hub" || !found.TriggerOccurredAt.Equal(at) {
		t.Fatalf("读回 = %s %s %s（present %v err %v），want 权威交接 node-hub %s",
			found.TriggerControl, found.TriggerLocation, found.TriggerOccurredAt, present, err, at)
	}

	for name, statement := range map[string]string{
		"两列都空": `INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, trigger_occurred_at, reassessed_at)
		 VALUES ('tenant-cp', 'trigger-cp-bare', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-cp', 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001',
		         '2026-08-21T14:30:00Z', '2026-08-21T15:00:00Z')`,
		"只写了控制依据": `INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, trigger_occurred_at, trigger_control, reassessed_at)
		 VALUES ('tenant-cp', 'trigger-cp-half', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-cp', 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001',
		         '2026-08-21T14:30:00Z', 'NODE_INTAKE_CONTROL', '2026-08-21T15:00:00Z')`,
		"线索级控制依据": `INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, trigger_occurred_at, trigger_control, trigger_location, reassessed_at)
		 VALUES ('tenant-cp', 'trigger-cp-hint', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-cp', 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001',
		         '2026-08-21T14:30:00Z', 'SOURCE_HINT', 'node-origin', '2026-08-21T15:00:00Z')`,
		"位置是空白": `INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, trigger_occurred_at, trigger_control, trigger_location, reassessed_at)
		 VALUES ('tenant-cp', 'trigger-cp-blank', 'customer-a', 'request-1',
		         'baseline-v1', 'parcel-cp', 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001',
		         '2026-08-21T14:30:00Z', 'NODE_INTAKE_CONTROL', '   ', '2026-08-21T15:00:00Z')`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Errorf("%s：写进了复核册", name)
		}
	}
}

// Covers: 未判原因加上「控制位置未定」之后写得进，且不带比较时刻；旧原因照旧写得进。
func TestControlPositionUnavailableIsAClosedNotJudgedReason(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-cp", "parcel-cp")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, notJudged(t, key, "trigger-cp-wait", ports.NotJudgedControlPositionUnavailable))
	})
	var reason string
	var positionAt, acceptedAt, planAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT reason, current_position_at, shipment_accepted_at, plan_effective_at
		   FROM network_routing.route_reassessment_not_judged
		  WHERE correlation_id = 'trigger-cp-wait'`).Scan(&reason, &positionAt, &acceptedAt, &planAt); err != nil {
		t.Fatalf("读未判行：%v", err)
	}
	if reason != "CONTROL_POSITION_UNAVAILABLE" || positionAt != nil || acceptedAt != nil || planAt != nil {
		t.Fatalf("未判行 = %s、位置 %v、接受 %v、计划 %v，want 控制位置未定且比较时刻都空", reason, positionAt, acceptedAt, planAt)
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return reassessments.RecordNotJudged(txCtx, notJudged(t, key, "trigger-cp-old", ports.NotJudgedNoRoutingHistory))
	})
	if err := pool.QueryRow(ctx,
		`SELECT reason FROM network_routing.route_reassessment_not_judged WHERE correlation_id = 'trigger-cp-old'`).Scan(&reason); err != nil || reason != "NO_ROUTING_HISTORY" {
		t.Fatalf("旧原因 = %s（err %v），want NO_ROUTING_HISTORY", reason, err)
	}
}

// Covers: 最近控制位置。没有已判复核 → 没有；最大时刻只有一组 → 交回那一组；
// 最大时刻上有旧行两列为空，或同一时刻两组位置 → 定不下。重判行抄了同一组锚点之后答案不变。
func TestTheLatestControlPositionIsKnownOnlyWhenTheLatestInstantAgrees(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-cp", "parcel-cp")
	at := notJudgedTriggerAt

	if got := positionOf(t, transactor, ctx, reassessments, key); got.Status != ports.ControlPositionAbsent {
		t.Fatalf("空册 = %v，want 没有", got.Status)
	}

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-early"),
		anchored(t, key, "trigger-cp-early", domain.NodeIntakeControl, "node-early", at.Add(-time.Hour)))
	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-a"),
		anchored(t, key, "trigger-cp-a", domain.TransportHandoverControl, "node-hub", at))
	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-a2"),
		anchored(t, key, "trigger-cp-a2", domain.TransportHandoverControl, "node-hub", at))
	got := positionOf(t, transactor, ctx, reassessments, key)
	if got.Status != ports.ControlPositionKnown || got.Control != domain.TransportHandoverControl ||
		got.Location.String() != "node-hub" || !got.OccurredAt.Equal(at) {
		t.Fatalf("一组 = %v %s %s %s，want 已知、权威交接、node-hub、%s",
			got.Status, got.Control, got.Location, got.OccurredAt, at)
	}

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-copy"),
		anchored(t, key, "trigger-cp-copy", domain.TransportHandoverControl, "node-hub", at))
	again := positionOf(t, transactor, ctx, reassessments, key)
	if again.Status != ports.ControlPositionKnown || again.Control != got.Control ||
		again.Location.String() != got.Location.String() || !again.OccurredAt.Equal(got.OccurredAt) {
		t.Fatalf("抄了锚点之后 = %v %s %s %s，want 与抄之前相同", again.Status, again.Control, again.Location, again.OccurredAt)
	}

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-split"),
		anchored(t, key, "trigger-cp-split", domain.NodeIntakeControl, "node-other", at))
	if split := positionOf(t, transactor, ctx, reassessments, key); split.Status != ports.ControlPositionUndetermined {
		t.Fatalf("同一时刻两处位置 = %v，want 定不下", split.Status)
	}

	legacyKey := routeKey(t, "tenant-cp", "parcel-legacy")
	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "trigger-cp-known-before"),
		anchored(t, legacyKey, "trigger-cp-known-before", domain.NodeIntakeControl, "node-origin", at.Add(-time.Hour)))
	dropPresentAndInsertBare(t, pool, ctx, "trigger-cp-legacy", "parcel-legacy", at)
	if legacy := positionOf(t, transactor, ctx, reassessments, legacyKey); legacy.Status != ports.ControlPositionUndetermined {
		t.Fatalf("最大时刻上的旧行两列为空 = %v，want 定不下", legacy.Status)
	}
}

// dropPresentAndInsertBare 在这一个测试库里摘掉「新行必带」再插入两列为空的行，模拟迁移前的旧行。
// 那条约束是 NOT VALID，存量空列本就合法；测试库是克隆出来的，摘掉它不会碰到别的用例。
func dropPresentAndInsertBare(t *testing.T, pool *pgxpool.Pool, ctx context.Context, correlation, parcel string, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`ALTER TABLE network_routing.route_reassessment
		     DROP CONSTRAINT route_reassessment_trigger_control_position_present`); err != nil {
		t.Fatalf("摘掉新行必带：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, trigger_occurred_at, reassessed_at)
		 VALUES ('tenant-cp', $1, 'customer-a', 'request-1',
		         'baseline-v1', $2, 'LAST_MILE_DELIVERY', 'STILL_APPLICABLE', 'RPV-0001',
		         $3, $3)`, correlation, parcel, at); err != nil {
		t.Fatalf("插入旧行：%v", err)
	}
}

// Covers: 等位置的实测变化只交回复核册还没有对应行的那些，按触发业务时间再按关联排序。已经落了复核的、别的原因、别的判断键都不交。
func TestAwaitingMeasurementChangesSkipsOnesAlreadyReassessed(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-cp", "parcel-cp")
	other := routeKey(t, "tenant-cp", "parcel-other")
	at := notJudgedTriggerAt

	within(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.NotJudgedTrigger{
			waitingAt(t, key, "m-late", at.Add(time.Hour)),
			waitingAt(t, key, "m-early", at),
			waitingAt(t, key, "m-done", at.Add(-time.Hour)),
			waitingAt(t, other, "m-foreign", at),
		} {
			if err := reassessments.RecordNotJudged(txCtx, record); err != nil {
				return err
			}
		}
		history := notJudged(t, key, "m-history", ports.NotJudgedNoRoutingHistory)
		return reassessments.RecordNotJudged(txCtx, history)
	})
	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "m-done"),
		anchored(t, key, "m-done", domain.NodeIntakeControl, "node-origin", at))

	var got []domain.RequestCorrelationID
	within(t, transactor, ctx, func(txCtx context.Context) error {
		ids, err := reassessments.AwaitingMeasurementChanges(txCtx, key)
		got = ids
		return err
	})
	want := []string{"m-early", "m-late"}
	if len(got) != len(want) {
		t.Fatalf("等位置 = %v，want %v", got, want)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("等位置[%d] = %s，want %s（全部 %v）", i, got[i], want[i], got)
		}
	}
}

// Covers: 已判时刻之后还有一次更晚的控制触发入账未判、关联还没有复核行，最近控制位置定不下。
// 那条未判后来落了复核，位置改答那次的；更早、恰好同一时刻、等位置的实测变化、别的判断键都不把已定的位置搅掉。
func TestALaterUnjudgedControlTriggerLeavesThePositionUndetermined(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-cp", "parcel-late")
	t1 := notJudgedTriggerAt
	t2 := t1.Add(time.Hour)

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "late-t1"),
		anchored(t, key, "late-t1", domain.NodeIntakeControl, "node-origin", t1))
	within(t, transactor, ctx, func(txCtx context.Context) error {
		record := notJudged(t, key, "late-freeze", ports.NotJudgedFreezeFormUnconfigured)
		record.TriggerOccurredAt = t2
		return reassessments.RecordNotJudged(txCtx, record)
	})
	if got := positionOf(t, transactor, ctx, reassessments, key); got.Status != ports.ControlPositionUndetermined {
		t.Fatalf("更晚的未判控制触发 = %v，want 定不下", got.Status)
	}

	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "late-freeze"),
		anchored(t, key, "late-freeze", domain.TransportHandoverControl, "node-later", t2))
	got := positionOf(t, transactor, ctx, reassessments, key)
	if got.Status != ports.ControlPositionKnown || got.Control != domain.TransportHandoverControl ||
		got.Location.String() != "node-later" || !got.OccurredAt.Equal(t2) {
		t.Fatalf("未判落成复核之后 = %v %s %s %s，want 已知、权威交接、node-later、%s",
			got.Status, got.Control, got.Location, got.OccurredAt, t2)
	}

	quiet := routeKey(t, "tenant-cp", "parcel-quiet")
	other := routeKey(t, "tenant-cp", "parcel-quiet-other")
	saveReassessment(t, transactor, ctx, reassessments,
		scalar(t, domain.NewRequestCorrelationID, "quiet-t1"),
		anchored(t, quiet, "quiet-t1", domain.NodeIntakeControl, "node-origin", t1))
	within(t, transactor, ctx, func(txCtx context.Context) error {
		earlier := notJudged(t, quiet, "quiet-earlier", ports.NotJudgedFreezeFormUnconfigured)
		earlier.TriggerOccurredAt = t1.Add(-time.Minute)
		same := notJudged(t, quiet, "quiet-same", ports.NotJudgedEvidenceNotConfigured)
		same.TriggerOccurredAt = t1
		waiting := notJudged(t, quiet, "quiet-waiting", ports.NotJudgedControlPositionUnavailable)
		waiting.TriggerOccurredAt = t2
		foreign := notJudged(t, other, "quiet-foreign", ports.NotJudgedFreezeFormUnconfigured)
		foreign.TriggerOccurredAt = t2
		for _, record := range []ports.NotJudgedTrigger{earlier, same, waiting, foreign} {
			if err := reassessments.RecordNotJudged(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
	kept := positionOf(t, transactor, ctx, reassessments, quiet)
	if kept.Status != ports.ControlPositionKnown || kept.Control != domain.NodeIntakeControl ||
		kept.Location.String() != "node-origin" || !kept.OccurredAt.Equal(t1) {
		t.Fatalf("不该搅动的未判之后 = %v %s %s %s，want 仍是 T1 的节点收寄 node-origin",
			kept.Status, kept.Control, kept.Location, kept.OccurredAt)
	}
}

func waitingAt(t *testing.T, key domain.InitialRouteJudgmentKey, correlation string, at time.Time) ports.NotJudgedTrigger {
	t.Helper()
	record := notJudged(t, key, correlation, ports.NotJudgedControlPositionUnavailable)
	record.TriggerOccurredAt = at
	return record
}

// 两口都取判断键锁。锁随事务释放；没有事务时锁在语句结束就放掉，静默放过会让并发的两笔各自读不到对方尚未提交的复核。
func TestControlPositionReadsRefuseToRunOutsideATransaction(t *testing.T) {
	_, reassessments, _, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	if _, err := reassessments.LatestControlPosition(ctx, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务取最近控制位置应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := reassessments.AwaitingMeasurementChanges(ctx, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务读等位置的实测变化应返回 ErrTransactionRequired，实得：%v", err)
	}
}
