package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

type measurementClock time.Time

func (clock measurementClock) Now() time.Time { return time.Time(clock) }

type quietMeasurementChange struct{}

func (quietMeasurementChange) NoticeMeasurementChanged(context.Context, ports.MeasurementChanged) error {
	return nil
}

func newMeasurementHandler(t *testing.T, store ports.ActualMeasurementStore, at time.Time) *application.RecordActualMeasurementHandler {
	t.Helper()
	handler, err := application.NewRecordActualMeasurementHandler(store, measurementClock(at), quietMeasurementChange{})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

type memoryMeasurements struct {
	measurements  map[string]ports.ActualMeasurementRecord
	invalidations map[string]ports.MeasurementInvalidationRecord
	voided        map[string]string
}

func newMemoryMeasurements() *memoryMeasurements {
	return &memoryMeasurements{
		measurements:  map[string]ports.ActualMeasurementRecord{},
		invalidations: map[string]ports.MeasurementInvalidationRecord{},
		voided:        map[string]string{},
	}
}

func measurementKey(key ports.ActualMeasurementKey) string {
	return key.TenantID.String() + "\x00" + key.SourceID
}

func (store *memoryMeasurements) FindMeasurement(
	_ context.Context,
	key ports.ActualMeasurementKey,
) (ports.ActualMeasurementRecord, bool, error) {
	record, found := store.measurements[measurementKey(key)]
	return record, found, nil
}

func (store *memoryMeasurements) SaveMeasurement(
	_ context.Context,
	record ports.ActualMeasurementRecord,
) (ports.ActualMeasurementSaveOutcome, error) {
	key := measurementKey(record.Key)
	if _, found := store.measurements[key]; found {
		return ports.ActualMeasurementAlreadyRecorded, nil
	}
	store.measurements[key] = record
	return ports.ActualMeasurementSaved, nil
}

func (store *memoryMeasurements) ListStillValid(
	_ context.Context,
	tenant domain.TenantID,
	objectKind domain.MeasurementObjectKind,
	objectID string,
) ([]ports.ActualMeasurementRecord, error) {
	var listed []ports.ActualMeasurementRecord
	for _, record := range store.measurements {
		if record.Key.TenantID != tenant || record.Measurement.ObjectKind() != objectKind || record.Measurement.ObjectID() != objectID {
			continue
		}
		if _, voided := store.voided[tenant.String()+"\x00"+record.Key.SourceID]; voided {
			continue
		}
		listed = append(listed, record)
	}
	return listed, nil
}

func (store *memoryMeasurements) FindInvalidation(
	_ context.Context,
	key ports.ActualMeasurementKey,
) (ports.MeasurementInvalidationRecord, bool, error) {
	record, found := store.invalidations[measurementKey(key)]
	return record, found, nil
}

func (store *memoryMeasurements) SaveInvalidation(
	_ context.Context,
	record ports.MeasurementInvalidationRecord,
) (ports.MeasurementInvalidationSaveOutcome, error) {
	target := record.Key.TenantID.String() + "\x00" + record.Invalidation.TargetSourceID()
	if _, found := store.measurements[target]; !found {
		return ports.MeasurementInvalidationTargetMissing, nil
	}
	if existing, found := store.voided[target]; found {
		if existing == record.Key.SourceID {
			return ports.MeasurementInvalidationAlreadyRecorded, nil
		}
		return ports.MeasurementInvalidationAlreadyVoid, nil
	}
	if _, found := store.invalidations[measurementKey(record.Key)]; found {
		return ports.MeasurementInvalidationAlreadyRecorded, nil
	}
	store.invalidations[measurementKey(record.Key)] = record
	store.voided[target] = record.Key.SourceID
	return ports.MeasurementInvalidationSaved, nil
}

func weightCommand() application.RecordActualMeasurementCommand {
	return application.RecordActualMeasurementCommand{
		TenantID: "tenant-1", SourceID: "measure-1", PerformedBy: "scale-7", Evidence: "WEIGH/7",
		OccurredAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
		ObjectKind: "HANDLING_UNIT", ObjectID: "unit-1", Node: "node-origin",
		Location: "bench-2", Basis: "intake-1",
		WeightValue: "2.10", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	}
}

func TestAReportedWeightIsKeptAndARepeatReturnsIt(t *testing.T) {
	store := newMemoryMeasurements()
	handler := newMeasurementHandler(t, store, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	recorded, err := handler.Handle(context.Background(), weightCommand())
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if recorded.Outcome() != application.ActualMeasurementRecorded {
		t.Fatalf("结果 %s", recorded.Outcome())
	}
	measurement, present := recorded.Measurement()
	if !present {
		t.Fatal("记下了却没有测量")
	}
	value, unit := measurement.Weight()
	if value != "2.10" || unit != "KG" {
		t.Fatalf("重量 = %s %s", value, unit)
	}
	if length, width, height, lengthUnit := measurement.Dimensions(); length != "20" || width != "15" || height != "10" || lengthUnit != "CM" {
		t.Fatalf("尺寸 = %s×%s×%s %s", length, width, height, lengthUnit)
	}

	again, err := handler.Handle(context.Background(), weightCommand())
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if again.Outcome() != application.ActualMeasurementExisting {
		t.Fatalf("重放结果 %s", again.Outcome())
	}

	changed := weightCommand()
	changed.WeightValue = "2.40"
	conflict, err := handler.Handle(context.Background(), changed)
	if err != nil {
		t.Fatalf("冲突：%v", err)
	}
	if conflict.Outcome() != application.ActualMeasurementSourceConflict {
		t.Fatalf("冲突结果 %s", conflict.Outcome())
	}
	kept, _ := again.Measurement()
	keptValue, _ := kept.Weight()
	if keptValue != "2.10" {
		t.Fatalf("冲突之后重量变成了 %s", keptValue)
	}
}

// Covers: 重量与尺寸都是必填，单位都要在产品内置目录上。只称重不量方、只量方不称重、任一单位不认识，都不收。
func TestAMeasurementWithoutBothItemsOrWithAnUnknownUnitIsNotAccepted(t *testing.T) {
	handler := newMeasurementHandler(t, newMemoryMeasurements(), time.Now())
	for name, change := range map[string]func(*application.RecordActualMeasurementCommand){
		"重量单位不在目录上": func(command *application.RecordActualMeasurementCommand) { command.WeightUnit = "斤" },
		"尺寸单位不在目录上": func(command *application.RecordActualMeasurementCommand) { command.DimensionUnit = "尺" },
		"只称重不量方": func(command *application.RecordActualMeasurementCommand) {
			command.Length, command.Width, command.Height, command.DimensionUnit = "", "", "", ""
		},
		"只量方不称重": func(command *application.RecordActualMeasurementCommand) {
			command.WeightValue, command.WeightUnit = "", ""
		},
		"尺寸缺一边": func(command *application.RecordActualMeasurementCommand) { command.Width = "" },
	} {
		command := weightCommand()
		change(&command)
		result, err := handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if result.Outcome() != application.ActualMeasurementNotAccepted {
			t.Fatalf("%s：结果 %s，want 不收", name, result.Outcome())
		}
	}
}

func TestVoidingDropsTheMeasurementFromTheStillValidList(t *testing.T) {
	store := newMemoryMeasurements()
	handler := newMeasurementHandler(t, store, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	if _, err := handler.Handle(context.Background(), weightCommand()); err != nil {
		t.Fatalf("登记：%v", err)
	}
	voided, err := handler.Invalidate(context.Background(), application.InvalidateActualMeasurementCommand{
		TenantID: "tenant-1", SourceID: "void-1", TargetSourceID: "measure-1", Reason: "称错件",
		PerformedBy: "scale-7", Evidence: "WEIGH/8",
		OccurredAt: time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if voided.Outcome() != application.MeasurementInvalidationRecorded {
		t.Fatalf("作废结果 %s", voided.Outcome())
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	still, err := store.ListStillValid(context.Background(), tenant, domain.HandlingUnitObject, "unit-1")
	if err != nil {
		t.Fatalf("仍有效：%v", err)
	}
	if len(still) != 0 {
		t.Fatalf("作废之后仍有效 %d 条", len(still))
	}

	second, err := handler.Invalidate(context.Background(), application.InvalidateActualMeasurementCommand{
		TenantID: "tenant-1", SourceID: "void-2", TargetSourceID: "measure-1", Reason: "再作废一次",
		PerformedBy: "scale-7", Evidence: "WEIGH/9",
		OccurredAt: time.Date(2026, 11, 17, 5, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("第二次作废：%v", err)
	}
	if second.Outcome() != application.MeasurementInvalidationAlreadyVoid {
		t.Fatalf("第二次作废结果 %s", second.Outcome())
	}
}

type recordedMeasurementChanges struct{ changes []ports.MeasurementChanged }

func (recorded *recordedMeasurementChanges) NoticeMeasurementChanged(_ context.Context, change ports.MeasurementChanged) error {
	recorded.changes = append(recorded.changes, change)
	return nil
}

// Covers: 带纳秒的发生时间登记成功，原样重放答已有结果；交给下游的变化通知（事件信封的发生时间由它来）、册上的测量用的是
// 同一个截到微秒的时刻，作废同理。
func TestANanosecondOccurrenceReachesTheChangeNoticeAtTheRegisterInstant(t *testing.T) {
	nanos := time.Date(2026, 11, 17, 2, 0, 0, 123456789, time.UTC)
	micros := time.Date(2026, 11, 17, 2, 0, 0, 123456000, time.UTC)
	notices := &recordedMeasurementChanges{}
	handler, err := application.NewRecordActualMeasurementHandler(
		newMemoryMeasurements(), measurementClock(time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)), notices)
	if err != nil {
		t.Fatal(err)
	}
	command := weightCommand()
	command.OccurredAt = nanos
	recorded, err := handler.Handle(context.Background(), command)
	if err != nil || recorded.Outcome() != application.ActualMeasurementRecorded {
		t.Fatalf("登记 = %v %v", recorded.Outcome(), err)
	}
	if measurement, _ := recorded.Measurement(); !measurement.Source().OccurredAt().Equal(micros) {
		t.Fatalf("册上的发生时间 = %s", measurement.Source().OccurredAt().Format(time.RFC3339Nano))
	}
	again, err := handler.Handle(context.Background(), command)
	if err != nil || again.Outcome() != application.ActualMeasurementExisting {
		t.Fatalf("原样重放 = %v %v", again.Outcome(), err)
	}

	voided, err := handler.Invalidate(context.Background(), application.InvalidateActualMeasurementCommand{
		TenantID: "tenant-1", SourceID: "void-1", TargetSourceID: "measure-1", Reason: "称错件",
		PerformedBy: "scale-7", Evidence: "WEIGH/8", OccurredAt: nanos.Add(time.Hour),
	})
	if err != nil || voided.Outcome() != application.MeasurementInvalidationRecorded {
		t.Fatalf("作废 = %v %v", voided.Outcome(), err)
	}

	want := []time.Time{micros, micros, micros.Add(time.Hour)}
	if len(notices.changes) != len(want) {
		t.Fatalf("通知 %d 封，want %d", len(notices.changes), len(want))
	}
	for index, change := range notices.changes {
		if !change.OccurredAt.Equal(want[index]) || change.OccurredAt.Nanosecond()%1000 != 0 {
			t.Fatalf("第 %d 封通知的发生时间 = %s，want %s", index+1,
				change.OccurredAt.Format(time.RFC3339Nano), want[index].Format(time.RFC3339Nano))
		}
	}
}
