package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证派送尝试的写入一半（票 product-strategy-boundary/19）：登记下的尝试与逐对象结果
// 经 FindByKey 与交付生效所读的 LoadDeliveryResult 两条路读回同一份；同一尝试再存答已有记录、不覆盖先到者。

var (
	storedDeliveryPlannedFrom = time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	storedDeliveryPlannedTo   = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	storedDeliveryArrivedAt   = time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	storedDeliveryRecordedAt  = time.Date(2026, 9, 24, 9, 30, 0, 0, time.UTC)
)

func TestASavedDeliveryAttemptReadsBackThroughBothReads(t *testing.T) {
	store, transactor := newDeliveryAttemptStore(t)
	ctx := t.Context()

	record := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-1", map[string]string{
		"parcel-1": "",
		"parcel-2": "no-one-home",
	})
	var saved ports.DeliveryAttemptSaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := store.Save(txCtx, record)
		saved = outcome
		return err
	})
	if saved != ports.DeliveryAttemptSaved {
		t.Fatalf("save outcome = %d, want saved", saved)
	}

	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("按键读回失败：err=%v exists=%v", err, exists)
	}
	if found.Attempt.Task().String() != "task-d1" ||
		found.Attempt.ExecutedBy().String() != "courier-1" ||
		!found.Attempt.ArrivedAt().Equal(storedDeliveryArrivedAt) ||
		!found.RecordedAt.Equal(storedDeliveryRecordedAt) ||
		len(found.Results) != 2 {
		t.Fatalf("尝试往返变形：attempt=%+v results=%d recordedAt=%s", found.Attempt, len(found.Results), found.RecordedAt)
	}

	for object, want := range map[string]domain.DeliveryObjectOutcome{
		"parcel-1": domain.ObjectDelivered,
		"parcel-2": domain.NoOneToReceive,
	} {
		_, result, present, err := store.LoadDeliveryResult(ctx,
			record.Key.TenantID, record.Key.Attempt, deliveryViewValue(t, domain.NewCarriedObjectReference, object))
		if err != nil || !present {
			t.Fatalf("%s：交付生效那条读路读不回刚登记的结果：err=%v present=%v", object, err, present)
		}
		if result.Outcome() != want {
			t.Fatalf("%s：outcome = %s, want %s", object, result.Outcome(), want)
		}
	}
}

func TestSavingTheSameAttemptAgainAnswersAlreadyRecordedAndKeepsTheFirst(t *testing.T) {
	store, transactor := newDeliveryAttemptStore(t)
	ctx := t.Context()

	first := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-1", map[string]string{"parcel-1": "no-one-home"})
	second := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-1", map[string]string{"parcel-1": ""})
	var outcomes []ports.DeliveryAttemptSaveOutcome
	for _, record := range []ports.DeliveryAttemptRecord{first, second} {
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			outcome, err := store.Save(txCtx, record)
			outcomes = append(outcomes, outcome)
			return err
		})
	}
	if outcomes[0] != ports.DeliveryAttemptSaved || outcomes[1] != ports.DeliveryAttemptAlreadyRecorded {
		t.Fatalf("outcomes = %v, want saved then already recorded", outcomes)
	}

	found, _, err := store.FindByKey(ctx, first.Key)
	if err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if len(found.Results) != 1 || found.Results[0].Outcome() != domain.NoOneToReceive {
		t.Fatalf("results = %+v, want the first report untouched", found.Results)
	}
}

// TestDeliveryAttemptWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池：父行与子行要一起成立。
func TestDeliveryAttemptWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _ := newDeliveryAttemptStore(t)

	record := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-1", map[string]string{"parcel-1": ""})
	if _, err := store.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestADeliveryAttemptRollbackLeavesNothingBehind 证尝试与它的对象结果随所在事务同生共死：回滚之后父行子行都不在。
func TestADeliveryAttemptRollbackLeavesNothingBehind(t *testing.T) {
	store, transactor := newDeliveryAttemptStore(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	record := storedDeliveryAttemptRecord(t, "tenant-1", "attempt-1", map[string]string{"parcel-1": "", "parcel-2": "no-one-home"})
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := store.Save(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := store.FindByKey(ctx, record.Key); err != nil || exists {
		t.Fatalf("回滚后仍读得到尝试：err=%v exists=%v", err, exists)
	}
	if _, _, present, err := store.LoadDeliveryResult(ctx, record.Key.TenantID, record.Key.Attempt,
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-2")); err != nil || present {
		t.Fatalf("回滚后仍读得到对象结果：err=%v present=%v", err, present)
	}
}

func TestAnUnknownDeliveryAttemptIsNotFoundRatherThanAnError(t *testing.T) {
	store, _ := newDeliveryAttemptStore(t)

	_, exists, err := store.FindByKey(t.Context(), ports.DeliveryAttemptKey{
		TenantID: deliveryViewValue(t, domain.NewTenantID, "tenant-1"),
		Attempt:  deliveryViewValue(t, domain.NewAttemptReference, "attempt-unknown"),
	})
	if err != nil || exists {
		t.Fatalf("err=%v exists=%v, want not found without error", err, exists)
	}
}

func newDeliveryAttemptStore(t *testing.T) (*adapter.DeliveryAttempts, bentoapp.Transactor) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewDeliveryAttempts(db)
	if err != nil {
		t.Fatalf("构造派送尝试库：%v", err)
	}
	return store, db.Transactor()
}

// storedDeliveryAttemptRecord 造一份尝试记录：outcomes 的键是对象，值为空表示妥投，非空是无人签收及其原因依据。
func storedDeliveryAttemptRecord(t *testing.T, tenant, attempt string, outcomes map[string]string) ports.DeliveryAttemptRecord {
	t.Helper()
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    deliveryViewValue(t, domain.NewTenantID, tenant),
		Attempt:     deliveryViewValue(t, domain.NewAttemptReference, attempt),
		Task:        deliveryViewValue(t, domain.NewDispatchTaskReference, "task-d1"),
		ExecutedBy:  deliveryViewValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Place:       deliveryViewValue(t, domain.NewAttemptPlaceReference, "door-1"),
		PlannedFrom: storedDeliveryPlannedFrom,
		PlannedTo:   storedDeliveryPlannedTo,
		ArrivedAt:   storedDeliveryArrivedAt,
		Evidence:    deliveryViewValue(t, domain.NewAttemptEvidenceReference, "evidence-1"),
	}
	for object := range outcomes {
		spec.Objects = append(spec.Objects, deliveryViewValue(t, domain.NewCarriedObjectReference, object))
	}
	formed, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("构造派送尝试：%v", err)
	}
	record := ports.DeliveryAttemptRecord{
		Key:        ports.DeliveryAttemptKey{TenantID: spec.TenantID, Attempt: spec.Attempt},
		Attempt:    formed,
		RecordedAt: storedDeliveryRecordedAt,
	}
	for object, basis := range outcomes {
		outcome := domain.ObjectDelivered
		var basisRef domain.AttemptResultBasisReference
		if basis != "" {
			outcome = domain.NoOneToReceive
			basisRef = deliveryViewValue(t, domain.NewAttemptResultBasisReference, basis)
		}
		result, err := domain.FormDeliveryAttemptResult(formed,
			deliveryViewValue(t, domain.NewCarriedObjectReference, object), outcome, basisRef,
			storedDeliveryArrivedAt.Add(5*time.Minute))
		if err != nil {
			t.Fatalf("构造对象结果：%v", err)
		}
		record.Results = append(record.Results, result)
	}
	return record
}
