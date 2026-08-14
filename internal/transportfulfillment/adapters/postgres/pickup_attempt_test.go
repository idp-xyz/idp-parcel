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

var (
	pickupPlannedFrom = time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	pickupPlannedTo   = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	pickupArrivedAt   = time.Date(2026, 8, 9, 8, 10, 0, 0, time.UTC)
	pickupOccurredAt  = time.Date(2026, 8, 9, 8, 15, 0, 0, time.UTC)
	pickupRecordedAt  = time.Date(2026, 8, 9, 8, 16, 0, 0, time.UTC)
)

func TestAMixedPickupAttemptRoundTripsAndKeepsFailuresOutOfPickups(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()

	record := mixedPickupRecord(t, "tenant-a", "source-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := store.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.PickupSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.ContentDigest != record.ContentDigest {
		t.Errorf("digest = %q", found.ContentDigest)
	}
	if len(found.Results) != 2 || len(found.Pickups) != 1 {
		t.Fatalf("results=%d pickups=%d；失败对象不该进 Pickups", len(found.Results), len(found.Pickups))
	}
	if found.Pickups[0].Object().String() != "parcel-1" ||
		found.Pickups[0].Control().String() == "" {
		t.Fatal("成功对象的揽收或控制依据往返丢失")
	}
	if found.Results[0].Outcome() != domain.ObjectPickedUp && found.Results[1].Outcome() != domain.ObjectPickedUp {
		t.Fatal("成功结果没有随尝试往返")
	}
	failed := found.Results[0]
	if failed.Outcome().Succeeded() {
		failed = found.Results[1]
	}
	if !failed.Outcome().Failed() {
		t.Fatal("失败结果没有随尝试往返")
	}
	if _, present := failed.Basis(); !present {
		t.Fatal("失败依据往返丢失")
	}
}

func TestAnAllFailedAttemptRoundTripsWithEmptyPickups(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()

	record := failedPickupRecord(t, "tenant-a", "source-fail")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, record)
		return err
	})

	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if len(found.Pickups) != 0 || len(found.Results) != 2 {
		t.Fatalf("全失败应无揽收：pickups=%d results=%d", len(found.Pickups), len(found.Results))
	}
}

func TestASecondPickupWriterGetsAlreadyRecorded(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()

	first := mixedPickupRecord(t, "tenant-a", "source-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, first)
		return err
	})

	second := mixedPickupRecord(t, "tenant-a", "source-1")
	second.ContentDigest = "digest-other"
	var outcome ports.PickupSaveOutcome
	var winner ports.PickupAttemptRecord
	var winnerFound bool
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := store.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = store.FindByKey(txCtx, first.Key)
		return err
	})
	if outcome != ports.PickupAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

func TestPickupAttemptsAreInvisibleAcrossTenants(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()

	saved := mixedPickupRecord(t, "tenant-a", "source-shared")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, saved)
		return err
	})

	_, exists, err := store.FindByKey(ctx, ports.PickupAttemptKey{
		TenantID: deliveryValue(t, domain.NewTenantID, "tenant-b"),
		SourceID: "source-shared",
	})
	if err != nil || exists {
		t.Errorf("另一个租户读到了揽收尝试：exists=%v err=%v", exists, err)
	}
}

func TestPickupWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _, _ := newPickupAttempts(t)
	ctx := t.Context()

	record := mixedPickupRecord(t, "tenant-a", "source-1")
	if _, err := store.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, exists, err := store.FindByKey(ctx, record.Key); err != nil || exists {
		t.Errorf("被拒绝的写入仍然落库：exists=%v err=%v", exists, err)
	}
}

func TestPickupRollbackLeavesNothingBehind(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	record := mixedPickupRecord(t, "tenant-a", "source-1")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := store.FindByKey(ctx, record.Key); err != nil || exists {
		t.Errorf("回滚后揽收尝试仍在：exists=%v err=%v", exists, err)
	}
}

func TestPickupCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, pool := newPickupAttempts(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, objects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'src-bad-1', 'attempt-1', 'task-1', 'courier-1', 'place-1',
		         now(), now() - interval '1 hour', now(), 'evidence-1', '["parcel-1"]', 'd', now())`); err == nil {
		t.Fatal("一行「计划窗口颠倒」溜进了揽收库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, rescheduled_from,
			 objects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'src-bad-2', 'attempt-1', 'task-1', 'courier-1', 'place-1',
		         now(), now() + interval '1 hour', now(), 'evidence-1', 'attempt-1',
		         '["parcel-1"]', 'd', now())`); err == nil {
		t.Fatal("一行「改约自指」溜进了揽收库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, objects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'src-bad-3', 'attempt-1', 'task-1', 'courier-1', 'place-1',
		         now(), now() + interval '1 hour', now(), 'evidence-1', NULL, 'd', now())`); err == nil {
		t.Fatal("一行「对象范围为 NULL」按 jsonb 三值缝溜进了揽收库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, objects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'src-shape', 'attempt-1', 'task-1', 'courier-1', 'place-1',
		         now(), now() + interval '1 hour', now(), 'evidence-1', '["parcel-1"]', 'd', now())`); err != nil {
		t.Fatalf("插入合法父行以便探针子表：%v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-shape', 'parcel-1', 'PICKED_UP', 'should-not', now())`); err == nil {
		t.Fatal("一行「成功却带着失败依据」溜进了结果表")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-shape', 'parcel-1', 'CUSTOMER_ABSENT', NULL, now())`); err == nil {
		t.Fatal("一行「失败却没有依据」溜进了结果表")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-shape', 'parcel-fail', 'CUSTOMER_ABSENT', 'absent', now())`); err != nil {
		t.Fatalf("插入失败结果以便探针揽收外键：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt_pickup
			(tenant_id, source_id, object_ref, outcome, task_ref, attempt_ref,
			 place_ref, control_ref, executed_by, version, occurred_at)
		 VALUES ('tenant-a', 'src-shape', 'parcel-fail', 'PICKED_UP', 'task-1', 'attempt-1',
		         'place-1', 'control-1', 'courier-1', 'pickup/v1', now())`); err == nil {
		t.Fatal("失败对象的揽收行接到了结果表——外键没有拦住")
	}
}

func newPickupAttempts(t *testing.T) (*adapter.PickupAttempts, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewPickupAttempts(db)
	if err != nil {
		t.Fatalf("构造揽收尝试库：%v", err)
	}
	return store, db.Transactor(), pool
}

func mixedPickupRecord(t *testing.T, tenant, source string) ports.PickupAttemptRecord {
	t.Helper()
	attempt := formedPickupAttempt(t, tenant, "attempt-1")
	picked := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1")
	absent := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	success, err := domain.FormAttemptObjectResult(attempt, picked, domain.ObjectPickedUp, domain.AttemptResultBasisReference{}, pickupOccurredAt)
	if err != nil {
		t.Fatalf("构造成功结果：%v", err)
	}
	failure, err := domain.FormAttemptObjectResult(
		attempt, absent, domain.CustomerAbsent,
		deliveryValue(t, domain.NewAttemptResultBasisReference, "customer-absent"),
		pickupOccurredAt,
	)
	if err != nil {
		t.Fatalf("构造失败结果：%v", err)
	}
	pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
		TenantID:   attempt.TenantID(),
		Object:     picked,
		Task:       deliveryValue(t, domain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    attempt.Attempt(),
		Place:      deliveryValue(t, domain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    deliveryValue(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-3"),
		ExecutedBy: attempt.ExecutedBy(),
		Version:    deliveryValue(t, domain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: pickupOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造场外揽收：%v", err)
	}
	return ports.PickupAttemptRecord{
		Key:           ports.PickupAttemptKey{TenantID: attempt.TenantID(), SourceID: source},
		ContentDigest: "digest-" + source + "-mixed",
		Attempt:       attempt,
		Results:       []domain.AttemptObjectResult{success, failure},
		Pickups:       []domain.OffsitePickup{pickup},
		RecordedAt:    pickupRecordedAt,
	}
}

func failedPickupRecord(t *testing.T, tenant, source string) ports.PickupAttemptRecord {
	t.Helper()
	attempt := formedPickupAttempt(t, tenant, "attempt-fail")
	first, err := domain.FormAttemptObjectResult(
		attempt, deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.GoodsNotReady, deliveryValue(t, domain.NewAttemptResultBasisReference, "not-ready"),
		pickupOccurredAt,
	)
	if err != nil {
		t.Fatalf("构造失败结果 1：%v", err)
	}
	second, err := domain.FormAttemptObjectResult(
		attempt, deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		domain.PackagingUnacceptable, deliveryValue(t, domain.NewAttemptResultBasisReference, "packaging"),
		pickupOccurredAt,
	)
	if err != nil {
		t.Fatalf("构造失败结果 2：%v", err)
	}
	return ports.PickupAttemptRecord{
		Key:           ports.PickupAttemptKey{TenantID: attempt.TenantID(), SourceID: source},
		ContentDigest: "digest-" + source + "-failed",
		Attempt:       attempt,
		Results:       []domain.AttemptObjectResult{first, second},
		RecordedAt:    pickupRecordedAt,
	}
}

func formedPickupAttempt(t *testing.T, tenant, attemptID string) domain.FulfillmentAttempt {
	t.Helper()
	attempt, err := domain.FormFulfillmentAttempt(domain.FulfillmentAttemptSpec{
		TenantID:    deliveryValue(t, domain.NewTenantID, tenant),
		Attempt:     deliveryValue(t, domain.NewAttemptReference, attemptID),
		Task:        deliveryValue(t, domain.NewDispatchTaskReference, "pickup-task-1"),
		ExecutedBy:  deliveryValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Place:       deliveryValue(t, domain.NewAttemptPlaceReference, "customer-warehouse-1"),
		PlannedFrom: pickupPlannedFrom,
		PlannedTo:   pickupPlannedTo,
		ArrivedAt:   pickupArrivedAt,
		Objects: []domain.CarriedObjectReference{
			deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		},
		Evidence: deliveryValue(t, domain.NewAttemptEvidenceReference, "attempt-evidence-1"),
	})
	if err != nil {
		t.Fatalf("构造履约尝试：%v", err)
	}
	return attempt
}
