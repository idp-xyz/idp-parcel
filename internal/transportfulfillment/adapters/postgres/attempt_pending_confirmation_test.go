package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证逐对象结果「待确认」与揽收「执行方拒收」两格的库面（票 docs-consistency/code/01）：两格随
// 尝试往返、待确认的来源读得回；库面 CHECK 只收带来源与缺口的待确认、来源只属待确认、拒收必带依据；旧写入方形状的行
// 照旧合法。夹具全是合成串，证据只记 S。

// Covers: `AT-TF-018`（待确认带来源）与 `AT-TF-017`（对象级拒收带依据）的持久化半边——一次到访里待确认、拒收、揽收到手
// 三格并存落库再读回：待确认的来源与缺口、拒收的依据原样回来，揽收只有揽到手的那一件。
func TestPendingAndRefusedPickupResultsRoundTripBesideAPickup(t *testing.T) {
	store, transactor, _ := newPickupAttempts(t)
	ctx := t.Context()

	record := mixedPickupRecord(t, "tenant-a", "source-pending")
	attempt := formedPickupAttempt(t, "tenant-a", "attempt-1")
	pending, err := domain.FormPendingAttemptObjectResult(attempt,
		deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		deliveryValue(t, domain.NewAttemptResultSourceReference, "source/vehicle-arrival+driver-scan"),
		deliveryValue(t, domain.NewAttemptResultBasisReference, "gap/no-receipt"),
		pickupOccurredAt)
	if err != nil {
		t.Fatalf("构造待确认结果：%v", err)
	}
	record.Results[1] = pending
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, record)
		return err
	})

	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if len(found.Results) != 2 || len(found.Pickups) != 1 || found.Pickups[0].Object().String() != "parcel-1" {
		t.Fatalf("results=%d pickups=%d；待确认对象不该进 Pickups", len(found.Results), len(found.Pickups))
	}
	for _, result := range found.Results {
		if result.Object().String() != "parcel-2" {
			continue
		}
		if result.Outcome() != domain.PickupPendingConfirmation {
			t.Fatalf("parcel-2 outcome = %q", result.Outcome())
		}
		source, hasSource := result.Source()
		gap, hasGap := result.Basis()
		if !hasSource || source.String() != "source/vehicle-arrival+driver-scan" || !hasGap || gap.String() != "gap/no-receipt" {
			t.Fatalf("待确认往返丢了来源或缺口：source=%q gap=%q", source, gap)
		}
	}

	refusedRecord := failedPickupRecord(t, "tenant-a", "source-refused")
	refused, err := domain.FormAttemptObjectResult(refusedRecord.Attempt,
		deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"), domain.ExecutorRefused,
		deliveryValue(t, domain.NewAttemptResultBasisReference, "reason/packaging-unacceptable+photo-1"), pickupOccurredAt)
	if err != nil {
		t.Fatalf("构造拒收结果：%v", err)
	}
	refusedRecord.Results[1] = refused
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, refusedRecord)
		return err
	})
	readRefused, _, err := store.FindByKey(ctx, refusedRecord.Key)
	if err != nil {
		t.Fatalf("读回拒收：%v", err)
	}
	for _, result := range readRefused.Results {
		if result.Object().String() != "parcel-2" {
			continue
		}
		basis, present := result.Basis()
		if result.Outcome() != domain.ExecutorRefused || !present || basis.String() != "reason/packaging-unacceptable+photo-1" {
			t.Fatalf("拒收往返变形：outcome=%q basis=%q", result.Outcome(), basis)
		}
		if _, hasSource := result.Source(); hasSource {
			t.Fatal("拒收读回时长出了来源")
		}
	}
	if len(readRefused.Pickups) != 0 {
		t.Fatal("拒收的对象读回时进了揽收")
	}
}

// Covers: `AT-TF-070`（冲突的对象记待确认，带全部来源）的持久化半边——派送一侧的待确认经 FindByKey 与交付生效所读的
// LoadDeliveryResult 两条路读回同一份来源与冲突。
func TestAPendingDeliveryResultRoundTripsThroughBothReads(t *testing.T) {
	store, transactor := newDeliveryAttemptStore(t)
	ctx := t.Context()

	record := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-pending", map[string]string{"parcel-1": "", "parcel-2": "no-one-home"})
	pending, err := domain.FormPendingDeliveryAttemptResult(record.Attempt,
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		deliveryViewValue(t, domain.NewAttemptResultSourceReference, "source/signature-scan+address-geofence"),
		deliveryViewValue(t, domain.NewAttemptResultBasisReference, "conflict/signature-vs-address"),
		storedDeliveryArrivedAt.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("构造待确认：%v", err)
	}
	for index, result := range record.Results {
		if result.Object().String() == "parcel-2" {
			record.Results[index] = pending
		}
	}
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := store.Save(txCtx, record)
		return err
	})

	check := func(path string, result domain.DeliveryAttemptResult) {
		t.Helper()
		source, hasSource := result.Source()
		conflict, hasConflict := result.Basis()
		if result.Outcome() != domain.DeliveryPendingConfirmation || !hasSource || !hasConflict ||
			source.String() != "source/signature-scan+address-geofence" || conflict.String() != "conflict/signature-vs-address" {
			t.Fatalf("%s：待确认往返变形：outcome=%q source=%q conflict=%q", path, result.Outcome(), source, conflict)
		}
	}
	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("按键读回失败：err=%v exists=%v", err, exists)
	}
	for _, result := range found.Results {
		if result.Object().String() == "parcel-2" {
			check("FindByKey", result)
		} else if _, hasSource := result.Source(); hasSource {
			t.Fatal("妥投读回时长出了来源")
		}
	}
	_, viewed, present, err := store.LoadDeliveryResult(ctx, record.Key.TenantID, record.Key.Attempt,
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	if err != nil || !present {
		t.Fatalf("交付生效那条读路读不回：err=%v present=%v", err, present)
	}
	check("LoadDeliveryResult", viewed)
}

// 库面 CHECK：待确认必带来源与缺口、来源只属待确认、拒收必带依据；旧写入方不写 source_ref 的行照旧合法并读得回
// （存量行与新 CHECK 共存）。
func TestAttemptResultChecksAdmitThePendingCellOnlyWithItsSourceAndGap(t *testing.T) {
	store, _, pool := newPickupAttempts(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, objects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'src-pending', 'attempt-1', 'task-1', 'courier-1', 'place-1',
		         $1, $2, $3, 'evidence-1', '["parcel-1","parcel-2","parcel-3"]', 'd', $4)`,
		pickupPlannedFrom, pickupPlannedTo, pickupArrivedAt, pickupRecordedAt); err != nil {
		t.Fatalf("插入合法父行以便探针子表：%v", err)
	}
	refusedRows := map[string]string{
		"待确认却没有来源": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'PENDING_CONFIRMATION', 'gap', now())`,
		"待确认却没有缺口": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'PENDING_CONFIRMATION', NULL, 'source', now())`,
		"待确认的来源是空白": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'PENDING_CONFIRMATION', 'gap', '  ', now())`,
		"失败却带来源": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'CUSTOMER_ABSENT', 'absent', 'source', now())`,
		"拒收却没有依据": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'EXECUTOR_REFUSED', NULL, now())`,
		"封闭集外的词": `INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'MAYBE', 'gap', now())`,
	}
	for name, statement := range refusedRows {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("揽收结果表：一行「%s」溜了进去", name)
		}
	}

	// 旧写入方的形状：不写 source_ref。
	for _, statement := range []string{
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-1', 'PACKAGING_UNACCEPTABLE', 'packaging', $1)`,
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-2', 'PENDING_CONFIRMATION', 'gap', 'source', $1)`,
		`INSERT INTO transport_fulfillment.pickup_attempt_result
			(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'src-pending', 'parcel-3', 'EXECUTOR_REFUSED', 'refused', $1)`,
	} {
		if _, err := pool.Exec(ctx, statement, pickupOccurredAt); err != nil {
			t.Fatalf("合法行被拒：%v", err)
		}
	}
	found, exists, err := store.FindByKey(ctx, ports.PickupAttemptKey{
		TenantID: deliveryValue(t, domain.NewTenantID, "tenant-a"), SourceID: "src-pending"})
	if err != nil || !exists || len(found.Results) != 3 {
		t.Fatalf("直插的三行读不回：err=%v exists=%v results=%d", err, exists, len(found.Results))
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_attempt
			(tenant_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, objects, recorded_at)
		 VALUES ('tenant-a', 'attempt-d', 'task-1', 'courier-1', 'door-1',
		         now(), now() + interval '1 hour', now(), 'evidence-1', '["parcel-1","parcel-2"]', now())`); err != nil {
		t.Fatalf("插入合法派送父行：%v", err)
	}
	for name, statement := range map[string]string{
		"待确认却没有来源": `INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-1', 'PENDING_CONFIRMATION', 'conflict', now())`,
		"待确认却没有冲突": `INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-1', 'PENDING_CONFIRMATION', NULL, 'source', now())`,
		"妥投却带来源": `INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-1', 'DELIVERED', NULL, 'source', now())`,
		"揽收一侧的拒收词": `INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-1', 'EXECUTOR_REFUSED', 'refused', now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Fatalf("派送结果表：一行「%s」溜了进去", name)
		}
	}
	for _, statement := range []string{
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-1', 'WRONG_ADDRESS', 'wrong', now())`,
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('tenant-a', 'attempt-d', 'parcel-2', 'PENDING_CONFIRMATION', 'conflict', 'source', now())`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("合法派送行被拒：%v", err)
		}
	}
}
