package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证测量与作废共用一个来源身份空间。编排用内存登记册与记下变化通知的替身；夹具全为合成（S 级）。

func newSharedIdentityHandler(t *testing.T) (*application.RecordActualMeasurementHandler, *memoryMeasurements, *recordedMeasurementChanges) {
	t.Helper()
	store := newMemoryMeasurements()
	notices := &recordedMeasurementChanges{}
	handler, err := application.NewRecordActualMeasurementHandler(
		store, measurementClock(time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)), notices)
	if err != nil {
		t.Fatal(err)
	}
	return handler, store, notices
}

func measurementWithSource(sourceID string) application.RecordActualMeasurementCommand {
	command := weightCommand()
	command.SourceID = sourceID
	return command
}

func voidCommand(sourceID, target, reason string) application.InvalidateActualMeasurementCommand {
	return application.InvalidateActualMeasurementCommand{
		TenantID: "tenant-1", SourceID: sourceID, TargetSourceID: target, Reason: reason,
		PerformedBy: "scale-7", Evidence: "WEIGH/8",
		OccurredAt: time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC),
		Scope:      application.InvalidationAcrossTenantNodes(),
	}
}

func mustRecordMeasurement(t *testing.T, handler *application.RecordActualMeasurementHandler, sourceID string) {
	t.Helper()
	result, err := handler.Handle(context.Background(), measurementWithSource(sourceID))
	if err != nil || result.Outcome() != application.ActualMeasurementRecorded {
		t.Fatalf("登记 %s = %s %v", sourceID, result.Outcome(), err)
	}
}

func mustRecordInvalidation(t *testing.T, handler *application.RecordActualMeasurementHandler, sourceID, target string) {
	t.Helper()
	result, err := handler.Invalidate(context.Background(), voidCommand(sourceID, target, "称错件"))
	if err != nil || result.Outcome() != application.MeasurementInvalidationRecorded {
		t.Fatalf("作废 %s = %s %v", sourceID, result.Outcome(), err)
	}
}

func stillValidSourceIDs(t *testing.T, store *memoryMeasurements) map[string]bool {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListStillValid(context.Background(), tenant, domain.HandlingUnitObject, "unit-1")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, record := range listed {
		ids[record.Key.SourceID] = true
	}
	return ids
}

// Covers: 作废的来源身份已经是另一条测量的来源身份，答来源冲突：不存、不发变化事件，被它指着的那条测量仍有效。
// 变化事件的标识与结算续办的依据都由来源身份定，撞号会让作废登记成功而重评被静默丢掉。
func TestAnInvalidationCannotTakeTheSourceIdentityOfAnotherMeasurement(t *testing.T) {
	handler, store, notices := newSharedIdentityHandler(t)
	mustRecordMeasurement(t, handler, "measure-1")
	mustRecordMeasurement(t, handler, "measure-2")

	result, err := handler.Invalidate(context.Background(), voidCommand("measure-1", "measure-2", "称错件"))
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if result.Outcome() != application.MeasurementInvalidationSourceConflict {
		t.Fatalf("作废结果 %s，want SOURCE_CONFLICT", result.Outcome())
	}
	if _, present := result.Invalidation(); present {
		t.Fatal("冲突的作废不该带出作废事实")
	}
	if len(store.invalidations) != 0 || len(store.voided) != 0 {
		t.Fatalf("冲突的作废落了册：%d 条作废，%d 条被指", len(store.invalidations), len(store.voided))
	}
	if len(notices.changes) != 2 {
		t.Fatalf("变化通知 %d 封，want 2（两条测量各一封，冲突的作废不发）", len(notices.changes))
	}
	if ids := stillValidSourceIDs(t, store); !ids["measure-1"] || !ids["measure-2"] {
		t.Fatalf("仍有效 = %v，want 两条都在", ids)
	}
}

// Covers: 测量的来源身份已经是某条作废的来源身份，答来源冲突：不存、不发变化事件。
func TestAMeasurementCannotTakeTheSourceIdentityOfAnInvalidation(t *testing.T) {
	handler, store, notices := newSharedIdentityHandler(t)
	mustRecordMeasurement(t, handler, "measure-1")
	mustRecordInvalidation(t, handler, "void-1", "measure-1")

	result, err := handler.Handle(context.Background(), measurementWithSource("void-1"))
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.ActualMeasurementSourceConflict {
		t.Fatalf("登记结果 %s，want SOURCE_CONFLICT", result.Outcome())
	}
	if _, present := result.Measurement(); present {
		t.Fatal("冲突的登记不该带出测量")
	}
	if _, found := store.measurements["tenant-1\x00void-1"]; found {
		t.Fatal("冲突的测量落了册")
	}
	if len(notices.changes) != 2 {
		t.Fatalf("变化通知 %d 封，want 2（登记与作废各一封，冲突的登记不发）", len(notices.changes))
	}
	if ids := stillValidSourceIDs(t, store); len(ids) != 0 {
		t.Fatalf("仍有效 = %v，want 空（那条测量已作废，冲突的那条没记）", ids)
	}
}

// Covers: 同类之内的判重不变：同来源身份同内容重报答原结果，内容不同答冲突——测量与作废各自如此，
// 另一类在册也不改这两个答案。
func TestTheSameKindKeepsItsReplayAndConflictAnswersBesideTheOtherKind(t *testing.T) {
	handler, _, _ := newSharedIdentityHandler(t)
	mustRecordMeasurement(t, handler, "measure-1")
	mustRecordInvalidation(t, handler, "void-1", "measure-1")

	again, err := handler.Handle(context.Background(), measurementWithSource("measure-1"))
	if err != nil || again.Outcome() != application.ActualMeasurementExisting {
		t.Fatalf("测量原样重报 = %s %v，want EXISTING_RESULT", again.Outcome(), err)
	}
	changed := measurementWithSource("measure-1")
	changed.WeightValue = "2.40"
	conflict, err := handler.Handle(context.Background(), changed)
	if err != nil || conflict.Outcome() != application.ActualMeasurementSourceConflict {
		t.Fatalf("测量改内容重报 = %s %v，want SOURCE_CONFLICT", conflict.Outcome(), err)
	}

	voidAgain, err := handler.Invalidate(context.Background(), voidCommand("void-1", "measure-1", "称错件"))
	if err != nil || voidAgain.Outcome() != application.MeasurementInvalidationExisting {
		t.Fatalf("作废原样重报 = %s %v，want EXISTING_RESULT", voidAgain.Outcome(), err)
	}
	voidChanged, err := handler.Invalidate(context.Background(), voidCommand("void-1", "measure-1", "换个原因"))
	if err != nil || voidChanged.Outcome() != application.MeasurementInvalidationSourceConflict {
		t.Fatalf("作废改内容重报 = %s %v，want SOURCE_CONFLICT", voidChanged.Outcome(), err)
	}
}

// Covers: 并发的两条路只能成一个，靠的是先取（租户，来源身份）的锁、再读另一类、最后才写：读在锁前，两条路都会读到
// 「没有」而各写各的。形成不了的输入不取锁——它到不了册。
func TestTheSourceIdentityIsLockedBeforeTheOtherKindIsRead(t *testing.T) {
	handler, store, _ := newSharedIdentityHandler(t)

	if _, err := handler.Handle(context.Background(), measurementWithSource("measure-1")); err != nil {
		t.Fatal(err)
	}
	wantMeasure := []string{"lock tenant-1/measure-1", "find-invalidation", "save-measurement"}
	if got := store.calls; len(got) < len(wantMeasure) || got[0] != wantMeasure[0] || got[1] != wantMeasure[1] || got[2] != wantMeasure[2] {
		t.Fatalf("测量登记对册的调用 = %v，want 以 %v 开头", got, wantMeasure)
	}

	store.calls = nil
	if _, err := handler.Invalidate(context.Background(), voidCommand("void-1", "measure-1", "称错件")); err != nil {
		t.Fatal(err)
	}
	wantVoid := []string{"lock tenant-1/void-1", "find-measurement", "save-invalidation"}
	if got := store.calls; len(got) < len(wantVoid) || got[0] != wantVoid[0] || got[1] != wantVoid[1] || got[2] != wantVoid[2] {
		t.Fatalf("作废对册的调用 = %v，want 以 %v 开头", got, wantVoid)
	}

	store.calls = nil
	bad := measurementWithSource("measure-2")
	bad.WeightUnit = "斤"
	result, err := handler.Handle(context.Background(), bad)
	if err != nil || result.Outcome() != application.ActualMeasurementNotAccepted {
		t.Fatalf("形成不了的测量 = %s %v，want NOT_ACCEPTED", result.Outcome(), err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("形成不了的测量碰了册：%v", store.calls)
	}
}
