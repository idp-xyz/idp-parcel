package postgres_test

import (
	"context"
	"errors"
	"sync"
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

// 本文件对真实 PostgreSQL 16 证待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果的库面（票 docs-consistency/code/90，
// 迁移 0055）：更正只追加、首次提交的结果行不改写；当前有效的结果由「首次提交，被更正则取更正」读出；回指待确认靠外键与 CHECK
// 守在库面；同一格的并发更正在行锁上排队。夹具全是合成串，证据只记 S。

type correctionRig struct {
	pickups             *adapter.PickupAttempts
	deliveries          *adapter.DeliveryAttempts
	pickupCorrections   *adapter.PickupResultCorrections
	deliveryCorrections *adapter.DeliveryResultCorrections
	catalogue           *adapter.ReviewCatalogue
	transactor          bentoapp.Transactor
	pool                *pgxpool.Pool
}

func newCorrectionRig(t *testing.T) correctionRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	rig := correctionRig{transactor: db.Transactor(), pool: pool}
	if rig.pickups, err = adapter.NewPickupAttempts(db); err != nil {
		t.Fatalf("揽收尝试库：%v", err)
	}
	if rig.deliveries, err = adapter.NewDeliveryAttempts(db); err != nil {
		t.Fatalf("派送尝试库：%v", err)
	}
	if rig.pickupCorrections, err = adapter.NewPickupResultCorrections(db); err != nil {
		t.Fatalf("揽收更正册：%v", err)
	}
	if rig.deliveryCorrections, err = adapter.NewDeliveryResultCorrections(db); err != nil {
		t.Fatalf("派送更正册：%v", err)
	}
	if rig.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatalf("查阅册：%v", err)
	}
	return rig
}

// pendingPickupRecord 造一次到访：parcel-1 揽收到手、parcel-2 待确认、parcel-3 客户不在（经 mixedPickupRecord 的两件加一件）。
func pendingPickupRecord(t *testing.T, tenant, source string) ports.PickupAttemptRecord {
	t.Helper()
	record := mixedPickupRecord(t, tenant, source)
	pending, err := domain.FormPendingAttemptObjectResult(record.Attempt,
		deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		deliveryValue(t, domain.NewAttemptResultSourceReference, "source/vehicle-arrival+driver-scan"),
		deliveryValue(t, domain.NewAttemptResultBasisReference, "gap/no-receipt"),
		pickupOccurredAt)
	if err != nil {
		t.Fatalf("构造待确认结果：%v", err)
	}
	record.Results[1] = pending
	return record
}

func (rig correctionRig) savePickupAttempt(t *testing.T, record ports.PickupAttemptRecord) {
	t.Helper()
	mustWithinDeliveryTransaction(t, rig.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := rig.pickups.Save(txCtx, record)
		return err
	})
}

func pickupCorrectionOf(
	t *testing.T,
	target ports.PickupResultCorrectionTarget,
	outcome domain.AttemptObjectOutcome,
	basis string,
	control string,
) ports.PickupResultCorrectionRecord {
	t.Helper()
	var basisRef domain.AttemptResultBasisReference
	if basis != "" {
		basisRef = deliveryValue(t, domain.NewAttemptResultBasisReference, basis)
	}
	correction, err := domain.CorrectPendingAttemptObjectResult(target.Attempt, target.Result, outcome, basisRef,
		deliveryValue(t, domain.NewAttemptResultSourceReference, "source/partner-late-receipt"),
		pickupOccurredAt.Add(-3*time.Minute), pickupOccurredAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("更正待确认：%v", err)
	}
	record := ports.PickupResultCorrectionRecord{
		Key:        ports.PickupAttemptKey{TenantID: target.Attempt.TenantID(), SourceID: "source-pending"},
		Correction: correction,
		RecordedAt: pickupRecordedAt.Add(2 * time.Hour),
	}
	if control != "" {
		record.Control = deliveryValue(t, domain.NewTransportControlReference, control)
	}
	return record
}

// Covers: TF CONTEXT「待确认那一版留在册上不改写」与「同一对象在同一尝试中只能有一个当前有效结果」的持久化半边（揽收）——更正追加
// 之后：首次提交的结果行（FindByKey）仍是待确认；当前有效结果（FindEffectiveByKey，失败尝试费读的那一条）是更正出来的确定
// 结果，未被更正的对象照旧；更正的来源、控制依据与操作证据原样读回；同一格第二条更正答已有记录。
func TestAPickupCorrectionIsAppendedAndTheCurrentEffectiveResultIsReadWithoutRewritingThePendingRow(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	record := pendingPickupRecord(t, "tenant-a", "source-pending")
	rig.savePickupAttempt(t, record)
	key := record.Key
	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	var saved ports.PickupResultCorrectionRecord
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, found, err := rig.pickupCorrections.LoadForCorrection(txCtx, key, object)
		if err != nil || !found || target.HasCorrection {
			t.Fatalf("加载被更正的那一格：found=%v has=%v err=%v", found, target.HasCorrection, err)
		}
		if !target.Result.Outcome().PendingConfirmation() {
			t.Fatalf("被更正的是 %q，不是待确认", target.Result.Outcome())
		}
		saved = pickupCorrectionOf(t, target, domain.CustomerAbsent, "reason/partner-confirms-absent", "")
		saved.DecidedBy = deliveryValue(t, domain.NewRegistrantReference, "issuer#reviewer-1")
		outcome, err := rig.pickupCorrections.Save(txCtx, saved)
		if err != nil || outcome != ports.PickupResultCorrectionSaved {
			t.Fatalf("save: outcome=%v err=%v", outcome, err)
		}
		again, err := rig.pickupCorrections.Save(txCtx, saved)
		if err != nil || again != ports.PickupResultCorrectionAlreadyRecorded {
			t.Fatalf("第二条更正：outcome=%v err=%v，want 已有记录", again, err)
		}
		return nil
	})

	original, found, err := rig.pickups.FindByKey(ctx, key)
	if err != nil || !found {
		t.Fatalf("FindByKey: found=%v err=%v", found, err)
	}
	for _, result := range original.Results {
		if result.Object() == object && !result.Outcome().PendingConfirmation() {
			t.Fatalf("首次提交的结果行被改写成 %q", result.Outcome())
		}
	}
	effective, found, err := rig.pickups.FindEffectiveByKey(ctx, key)
	if err != nil || !found {
		t.Fatalf("FindEffectiveByKey: found=%v err=%v", found, err)
	}
	outcomes := map[string]domain.AttemptObjectOutcome{}
	for _, result := range effective.Results {
		outcomes[result.Object().String()] = result.Outcome()
	}
	if outcomes["parcel-2"] != domain.CustomerAbsent || outcomes["parcel-1"] != domain.ObjectPickedUp {
		t.Fatalf("当前有效结果 = %v；parcel-2 应是更正出来的 CUSTOMER_ABSENT，parcel-1 仍是 PICKED_UP", outcomes)
	}
	if len(effective.Pickups) != 1 {
		t.Fatalf("pickups = %d；更正成失败不增减揽收", len(effective.Pickups))
	}

	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.pickupCorrections.LoadForCorrection(txCtx, key, object)
		if err != nil || !target.HasCorrection {
			t.Fatalf("更正读不回：has=%v err=%v", target.HasCorrection, err)
		}
		stored := target.Correction
		if stored.Correction.Source().String() != "source/partner-late-receipt" ||
			stored.DecidedBy.String() != "issuer#reviewer-1" || stored.Control.String() != "" ||
			!stored.Correction.CorrectedAt().Equal(saved.Correction.CorrectedAt()) ||
			!stored.Correction.Effective().OccurredAt().Equal(saved.Correction.Effective().OccurredAt()) {
			t.Fatalf("更正往返变形：%+v", stored)
		}
		if !stored.Correction.Pending().Outcome().PendingConfirmation() {
			t.Fatal("读回的更正没有回指待确认那一版")
		}
		return nil
	})
}

// Covers: 同上——更正成揽收到手的控制依据随更正留下（只为重放比对），读回时回到记录上；没有操作证据的更正（作业事实面提交）
// 读回没有操作者。
func TestAPickupCorrectionToAPickupKeepsTheControlItReportedAndNoDecider(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	record := pendingPickupRecord(t, "tenant-a", "source-pending")
	rig.savePickupAttempt(t, record)
	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.pickupCorrections.LoadForCorrection(txCtx, record.Key, object)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, err := rig.pickupCorrections.Save(txCtx, pickupCorrectionOf(t, target, domain.ObjectPickedUp, "", "TRANSPORT-CONTROL/TF-2")); err != nil {
			t.Fatalf("save: %v", err)
		}
		return nil
	})
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.pickupCorrections.LoadForCorrection(txCtx, record.Key, object)
		if err != nil || !target.HasCorrection {
			t.Fatalf("load: has=%v err=%v", target.HasCorrection, err)
		}
		if target.Correction.Control.String() != "TRANSPORT-CONTROL/TF-2" || target.Correction.DecidedBy.String() != "" ||
			target.Correction.Correction.Effective().Outcome() != domain.ObjectPickedUp {
			t.Fatalf("control=%q decidedBy=%q outcome=%q", target.Correction.Control, target.Correction.DecidedBy,
				target.Correction.Correction.Effective().Outcome())
		}
		return nil
	})
	effective, _, err := rig.pickups.FindEffectiveByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	for _, result := range effective.Results {
		if result.Object() == object && !result.Outcome().Succeeded() {
			t.Fatalf("当前有效结果 = %q，want PICKED_UP", result.Outcome())
		}
	}
}

// 库面：回指待确认靠外键与 CHECK——更正接不上一格已是确定结果的行；更正出来的不是待确认；揽收到手带依据或没有控制依据、失败
// 带控制依据都进不来；更正早于它说的那件事进不来；来源是空白进不来；一格待确认至多一条更正。
func TestCorrectionRowsOnlyAttachToAPendingResultAndKeepTheDeterminateShapes(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	rig.savePickupAttempt(t, pendingPickupRecord(t, "tenant-a", "source-pending"))
	insert := `INSERT INTO transport_fulfillment.pickup_attempt_result_correction
		(tenant_id, source_id, object_ref, corrects_outcome, outcome, basis, source_ref, control_ref,
		 occurred_at, corrected_at, decided_by, recorded_at)
		VALUES ('tenant-a', 'source-pending', $1, $2, $3, $4, $5, $6, $7, $8, $9, now())`
	at := pickupOccurredAt
	later := pickupOccurredAt.Add(time.Hour)

	rejected := map[string][]any{
		"接在一格已是揽收到手的结果上": {"parcel-1", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", nil, at, later, nil},
		"接在一格已是失败的结果上":   {"parcel-3", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", nil, at, later, nil},
		"声称更正的是一格确定结果":   {"parcel-2", "PICKED_UP", "CUSTOMER_ABSENT", "x", "src", nil, at, later, nil},
		"更正成待确认":         {"parcel-2", "PENDING_CONFIRMATION", "PENDING_CONFIRMATION", "x", "src", nil, at, later, nil},
		"封闭集外的词":         {"parcel-2", "PENDING_CONFIRMATION", "MAYBE", "x", "src", nil, at, later, nil},
		"揽收到手带依据":        {"parcel-2", "PENDING_CONFIRMATION", "PICKED_UP", "x", "src", "ctl", at, later, nil},
		"揽收到手没有控制依据":     {"parcel-2", "PENDING_CONFIRMATION", "PICKED_UP", nil, "src", nil, at, later, nil},
		"失败带控制依据":        {"parcel-2", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", "ctl", at, later, nil},
		"失败没有依据":         {"parcel-2", "PENDING_CONFIRMATION", "EXECUTOR_REFUSED", nil, "src", nil, at, later, nil},
		"更正早于它说的那件事":     {"parcel-2", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", nil, later, at, nil},
		"来源是空白":          {"parcel-2", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "  ", nil, at, later, nil},
		"操作者是空白":         {"parcel-2", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", nil, at, later, "  "},
	}
	for name, arguments := range rejected {
		if _, err := rig.pool.Exec(ctx, insert, arguments...); err == nil {
			t.Fatalf("更正册：一行「%s」溜了进去", name)
		}
	}
	if _, err := rig.pool.Exec(ctx, insert, "parcel-2", "PENDING_CONFIRMATION", "CUSTOMER_ABSENT", "x", "src", nil, at, later, nil); err != nil {
		t.Fatalf("合法更正被拒：%v", err)
	}
	if _, err := rig.pool.Exec(ctx, insert, "parcel-2", "PENDING_CONFIRMATION", "GOODS_NOT_READY", "y", "src2", nil, at, later, nil); err == nil {
		t.Fatal("同一格待确认被更正了两次")
	}
	if _, err := rig.pool.Exec(ctx,
		`DELETE FROM transport_fulfillment.pickup_attempt_result WHERE source_id = 'source-pending' AND object_ref = 'parcel-2'`); err == nil {
		t.Fatal("被更正回指的待确认行被删掉了")
	}
}

// Covers: PBC-08 负向证据——更正册的读（带行锁）与写按框架合同无事务即拒，揽收与派送各一。
func TestResultCorrectionAccessRefusesToRunOutsideATransaction(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	pickupRecord := pendingPickupRecord(t, "tenant-a", "source-pending")
	rig.savePickupAttempt(t, pickupRecord)
	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	if _, _, err := rig.pickupCorrections.LoadForCorrection(ctx, pickupRecord.Key, object); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("揽收：无事务加载应返回 ErrTransactionRequired，实得：%v", err)
	}
	var target ports.PickupResultCorrectionTarget
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		var err error
		target, _, err = rig.pickupCorrections.LoadForCorrection(txCtx, pickupRecord.Key, object)
		return err
	})
	correction := pickupCorrectionOf(t, target, domain.CustomerAbsent, "reason/x", "")
	if _, err := rig.pickupCorrections.Save(ctx, correction); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("揽收：无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	deliveryRecord := pendingDeliveryRecord(t, "tenant-a", "attempt-pending")
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		_, err := rig.deliveries.Save(txCtx, deliveryRecord)
		return err
	})
	if _, _, err := rig.deliveryCorrections.LoadForCorrection(ctx, deliveryRecord.Key, object); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("派送：无事务加载应返回 ErrTransactionRequired，实得：%v", err)
	}
	var deliveryTarget ports.DeliveryResultCorrectionTarget
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		var err error
		deliveryTarget, _, err = rig.deliveryCorrections.LoadForCorrection(txCtx, deliveryRecord.Key, object)
		return err
	})
	if _, err := rig.deliveryCorrections.Save(ctx, deliveryCorrectionOf(t, deliveryTarget, domain.ObjectDelivered, "")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("派送：无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	var count int
	if err := rig.pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.pickup_attempt_result_correction`).Scan(&count); err != nil || count != 0 {
		t.Errorf("被拒绝的写入仍然落库：count=%d err=%v", count, err)
	}
}

// Covers: PBC-08 的锁与写同一事务——同一格待确认的两笔并发更正在结果行锁上排队：先到的提交之前，后到的加载一直等着；先到的提交
// 之后，后到的读到的是先到的更正，而不是各自看见「还没更正」。（-count=5）
func TestConcurrentCorrectionsOfOnePendingResultQueueOnItsRowLock(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	record := pendingPickupRecord(t, "tenant-a", "source-pending")
	rig.savePickupAttempt(t, record)
	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	firstHolds := make(chan struct{})
	secondRequested := make(chan struct{})
	var secondSaw bool
	var secondErr error
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		<-firstHolds
		secondErr = rig.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			close(secondRequested)
			target, found, err := rig.pickupCorrections.LoadForCorrection(txCtx, record.Key, object)
			if err != nil || !found {
				return errors.Join(err, errors.New("second load found nothing"))
			}
			secondSaw = target.HasCorrection
			return nil
		})
	}()

	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.pickupCorrections.LoadForCorrection(txCtx, record.Key, object)
		if err != nil {
			t.Fatalf("first load: %v", err)
		}
		close(firstHolds)
		<-secondRequested
		time.Sleep(150 * time.Millisecond) // 第二笔此刻应当停在行锁上
		if _, err := rig.pickupCorrections.Save(txCtx, pickupCorrectionOf(t, target, domain.CustomerAbsent, "reason/x", "")); err != nil {
			t.Fatalf("first save: %v", err)
		}
		return nil
	})
	group.Wait()
	if secondErr != nil {
		t.Fatalf("second transaction: %v", secondErr)
	}
	if !secondSaw {
		t.Fatal("后到的更正加载没等先到的提交，读到的是「还没更正」——结果行没锁住")
	}
}

// --- 派送一侧 ---

func pendingDeliveryRecord(t *testing.T, tenant, attempt string) ports.DeliveryAttemptRecord {
	t.Helper()
	record := storedDeliveryAttemptRecord(t, tenant, attempt, map[string]string{"parcel-1": "", "parcel-2": "no-one-home"})
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
	return record
}

func deliveryCorrectionOf(
	t *testing.T,
	target ports.DeliveryResultCorrectionTarget,
	outcome domain.DeliveryObjectOutcome,
	basis string,
) ports.DeliveryResultCorrectionRecord {
	t.Helper()
	var basisRef domain.AttemptResultBasisReference
	if basis != "" {
		basisRef = deliveryValue(t, domain.NewAttemptResultBasisReference, basis)
	}
	correction, err := domain.CorrectPendingDeliveryAttemptResult(target.Attempt, target.Result, outcome, basisRef,
		deliveryValue(t, domain.NewAttemptResultSourceReference, "source/address-review-confirmed"),
		storedDeliveryArrivedAt.Add(4*time.Minute), storedDeliveryArrivedAt.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("更正待确认：%v", err)
	}
	return ports.DeliveryResultCorrectionRecord{
		Key:        ports.DeliveryAttemptKey{TenantID: target.Attempt.TenantID(), Attempt: target.Attempt.Attempt()},
		Correction: correction,
		RecordedAt: storedDeliveryArrivedAt.Add(3 * time.Hour),
	}
}

// Covers: `AT-TF-070` 之后的确定、UC-TF-006「尝试待确认」的持久化半边——派送结果更正追加之后：首次提交的结果行（FindByKey）仍是
// 待确认；交付登记、交付门禁读数与失败尝试费读的 LoadDeliveryResult 读到更正出来的确定结果，而且那一格能直接喂给 FormEffectiveDelivery；
// 未被更正的对象与没有更正的待确认照旧。
func TestADeliveryCorrectionMakesTheCurrentResultTheCorrectedOneWithoutRewritingThePendingRow(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	record := pendingDeliveryRecord(t, "tenant-a", "attempt-pending")
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		_, err := rig.deliveries.Save(txCtx, record)
		return err
	})
	tenant, attempt := record.Key.TenantID, record.Key.Attempt
	parcel2 := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")

	// 更正之前：当前有效结果就是待确认，登不出有效交付。
	attemptValue, before, present, err := rig.deliveries.LoadDeliveryResult(ctx, tenant, attempt, parcel2)
	if err != nil || !present || !before.Outcome().PendingConfirmation() {
		t.Fatalf("更正之前：present=%v outcome=%q err=%v", present, before.Outcome(), err)
	}
	spec := domain.EffectiveDeliverySpec{
		Method:    deliveryValue(t, domain.NewDeliveryMethodReference, "HANDED_TO_RECIPIENT"),
		Recipient: deliveryValue(t, domain.NewReceivingPartyReference, "SYN-RECIPIENT/consignee-1"),
		Proof:     deliveryValue(t, domain.NewDeliveryProofReference, "SYN-POD/signature-1"),
		Version:   deliveryValue(t, domain.NewDeliveryResultVersion, "SYN-DRV-1"),
	}
	if _, err := domain.FormEffectiveDelivery(attemptValue, before, spec); !errors.Is(err, domain.ErrDeliveryResultPendingConfirmation) {
		t.Fatalf("更正之前的交付错误 = %v，want ErrDeliveryResultPendingConfirmation", err)
	}

	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, found, err := rig.deliveryCorrections.LoadForCorrection(txCtx, record.Key, parcel2)
		if err != nil || !found || target.HasCorrection {
			t.Fatalf("load: found=%v has=%v err=%v", found, target.HasCorrection, err)
		}
		saved := deliveryCorrectionOf(t, target, domain.ObjectDelivered, "")
		saved.DecidedBy = deliveryValue(t, domain.NewRegistrantReference, "issuer#reviewer-1")
		if outcome, err := rig.deliveryCorrections.Save(txCtx, saved); err != nil || outcome != ports.DeliveryResultCorrectionSaved {
			t.Fatalf("save: outcome=%v err=%v", outcome, err)
		}
		if again, err := rig.deliveryCorrections.Save(txCtx, saved); err != nil || again != ports.DeliveryResultCorrectionAlreadyRecorded {
			t.Fatalf("第二条更正：outcome=%v err=%v", again, err)
		}
		return nil
	})

	original, _, err := rig.deliveries.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("FindByKey: %v", err)
	}
	for _, result := range original.Results {
		if result.Object() == parcel2 && !result.Outcome().PendingConfirmation() {
			t.Fatalf("首次提交的结果行被改写成 %q", result.Outcome())
		}
	}
	attemptValue, after, present, err := rig.deliveries.LoadDeliveryResult(ctx, tenant, attempt, parcel2)
	if err != nil || !present || after.Outcome() != domain.ObjectDelivered {
		t.Fatalf("更正之后：present=%v outcome=%q err=%v", present, after.Outcome(), err)
	}
	if _, hasSource := after.Source(); hasSource {
		t.Fatal("当前有效结果带着来源")
	}
	if _, err := domain.FormEffectiveDelivery(attemptValue, after, spec); err != nil {
		t.Fatalf("更正之后仍登不出有效交付：%v", err)
	}
	_, other, _, err := rig.deliveries.LoadDeliveryResult(ctx, tenant, attempt, deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if err != nil || other.Outcome() != domain.ObjectDelivered {
		t.Fatalf("未被更正的对象：outcome=%q err=%v", other.Outcome(), err)
	}

	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.deliveryCorrections.LoadForCorrection(txCtx, record.Key, parcel2)
		if err != nil || !target.HasCorrection || target.Correction.DecidedBy.String() != "issuer#reviewer-1" ||
			target.Correction.Correction.Source().String() != "source/address-review-confirmed" ||
			!target.Correction.Correction.Pending().Outcome().PendingConfirmation() {
			t.Fatalf("更正读回：has=%v err=%v record=%+v", target.HasCorrection, err, target.Correction)
		}
		return nil
	})
}

// 库面（派送）：同揽收一侧——更正接不上确定结果、不是待确认、妥投不带依据、失败必带依据、一格至多一条。
func TestDeliveryCorrectionRowsOnlyAttachToAPendingResultAndKeepTheDeterminateShapes(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	record := pendingDeliveryRecord(t, "tenant-a", "attempt-pending")
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		_, err := rig.deliveries.Save(txCtx, record)
		return err
	})
	insert := `INSERT INTO transport_fulfillment.delivery_attempt_result_correction
		(tenant_id, attempt_ref, object_ref, corrects_outcome, outcome, basis, source_ref,
		 occurred_at, corrected_at, decided_by, recorded_at)
		VALUES ('tenant-a', 'attempt-pending', $1, $2, $3, $4, $5, $6, $7, $8, now())`
	at := storedDeliveryArrivedAt.Add(time.Minute)
	later := at.Add(time.Hour)
	for name, arguments := range map[string][]any{
		"接在一格已是妥投的结果上": {"parcel-1", "PENDING_CONFIRMATION", "REFUSED", "x", "src", at, later, nil},
		"更正成待确认":       {"parcel-2", "PENDING_CONFIRMATION", "PENDING_CONFIRMATION", "x", "src", at, later, nil},
		"妥投带依据":        {"parcel-2", "PENDING_CONFIRMATION", "DELIVERED", "x", "src", at, later, nil},
		"失败没有依据":       {"parcel-2", "PENDING_CONFIRMATION", "WRONG_ADDRESS", nil, "src", at, later, nil},
		"揽收一侧的词":       {"parcel-2", "PENDING_CONFIRMATION", "EXECUTOR_REFUSED", "x", "src", at, later, nil},
		"更正早于它说的那件事":   {"parcel-2", "PENDING_CONFIRMATION", "WRONG_ADDRESS", "x", "src", later, at, nil},
		"来源是空白":        {"parcel-2", "PENDING_CONFIRMATION", "WRONG_ADDRESS", "x", " ", at, later, nil},
	} {
		if _, err := rig.pool.Exec(ctx, insert, arguments...); err == nil {
			t.Fatalf("派送更正册：一行「%s」溜了进去", name)
		}
	}
	if _, err := rig.pool.Exec(ctx, insert, "parcel-2", "PENDING_CONFIRMATION", "WRONG_ADDRESS", "x", "src", at, later, nil); err != nil {
		t.Fatalf("合法更正被拒：%v", err)
	}
	if _, err := rig.pool.Exec(ctx, insert, "parcel-2", "PENDING_CONFIRMATION", "REFUSED", "y", "src2", at, later, nil); err == nil {
		t.Fatal("同一格待确认被更正了两次")
	}
}

// Covers: 票 docs-consistency/code/90 要裁的「待确认复核清单」的持久化半边——还在待确认的那几格两侧合并、最早的在前、按租户隔离、按上限截断；
// 一格凭后补来源更正之后就不在清单上（首次提交的待确认行仍在表里，靠更正册反连接判），而没有更正的照旧在；揽收一侧带更正口要指名的来源身份。
func TestThePendingResultListHoldsOnlyWhatIsStillPendingAcrossBothSides(t *testing.T) {
	rig := newCorrectionRig(t)
	ctx := t.Context()
	tenantA := deliveryValue(t, domain.NewTenantID, "tenant-a")

	rig.savePickupAttempt(t, pendingPickupRecord(t, "tenant-a", "source-pending"))
	rig.savePickupAttempt(t, pendingPickupRecord(t, "tenant-b", "source-other-tenant"))
	deliveryRecord := pendingDeliveryRecord(t, "tenant-a", "attempt-pending")
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		_, err := rig.deliveries.Save(txCtx, deliveryRecord)
		return err
	})

	rows, err := rig.catalogue.ListPendingAttemptResults(ctx, tenantA, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v；租户 a 有一格待确认揽收与一格待确认派送，别的租户的不该在", rows)
	}
	if rows[0].Side != ports.PendingAttemptSidePickup || rows[0].SourceID != "source-pending" || rows[0].Attempt != "attempt-1" ||
		rows[0].Object != "parcel-2" || rows[0].Source != "source/vehicle-arrival+driver-scan" || rows[0].Gap != "gap/no-receipt" ||
		rows[0].Task != "pickup-task-1" || rows[0].ExecutedBy != "courier-1" || rows[0].Place != "customer-warehouse-1" {
		t.Fatalf("揽收行 = %+v", rows[0])
	}
	if rows[1].Side != ports.PendingAttemptSideDelivery || rows[1].SourceID != "" || rows[1].Attempt != "attempt-pending" ||
		rows[1].Object != "parcel-2" || rows[1].Gap != "conflict/signature-vs-address" {
		t.Fatalf("派送行 = %+v", rows[1])
	}
	if limited, err := rig.catalogue.ListPendingAttemptResults(ctx, tenantA, 1); err != nil || len(limited) != 1 || limited[0].Side != ports.PendingAttemptSidePickup {
		t.Fatalf("limit 1 = %+v err=%v；最早的先看", limited, err)
	}
	if _, err := rig.catalogue.ListPendingAttemptResults(ctx, tenantA, 0); err == nil {
		t.Fatal("上限为 0 被收下")
	}

	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2")
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.pickupCorrections.LoadForCorrection(txCtx, ports.PickupAttemptKey{TenantID: tenantA, SourceID: "source-pending"}, object)
		if err != nil {
			return err
		}
		correction := pickupCorrectionOf(t, target, domain.CustomerAbsent, "reason/x", "")
		_, err = rig.pickupCorrections.Save(txCtx, correction)
		return err
	})
	rows, err = rig.catalogue.ListPendingAttemptResults(ctx, tenantA, 10)
	if err != nil || len(rows) != 1 || rows[0].Side != ports.PendingAttemptSideDelivery {
		t.Fatalf("揽收一格更正之后的清单 = %+v err=%v；更正过的不该再列，没更正的派送一格照旧在", rows, err)
	}
	mustWithinDeliveryTransaction(t, rig.transactor, ctx, func(txCtx context.Context) error {
		target, _, err := rig.deliveryCorrections.LoadForCorrection(txCtx, deliveryRecord.Key, object)
		if err != nil {
			return err
		}
		_, err = rig.deliveryCorrections.Save(txCtx, deliveryCorrectionOf(t, target, domain.ObjectDelivered, ""))
		return err
	})
	if rows, err = rig.catalogue.ListPendingAttemptResults(ctx, tenantA, 10); err != nil || len(rows) != 0 {
		t.Fatalf("两侧都更正之后的清单 = %+v err=%v, want 空", rows, err)
	}
	other, err := rig.catalogue.ListPendingAttemptResults(ctx, deliveryValue(t, domain.NewTenantID, "tenant-b"), 10)
	if err != nil || len(other) != 1 || other[0].SourceID != "source-other-tenant" {
		t.Fatalf("他租户的清单 = %+v err=%v；租户 a 的更正动不了它", other, err)
	}
}
