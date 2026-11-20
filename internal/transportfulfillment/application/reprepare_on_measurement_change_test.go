package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证实测变化入口与初始准备入口共用的「按实测补一份」：不覆盖申报，也不覆盖按别的实测判过的那一份。夹具全为合成值（S 级）。

type measuredDemandDouble struct {
	reading ports.MeasuredDemandReading
	err     error
	before  func()
}

func (double measuredDemandDouble) LoadMeasuredDemand(context.Context, domain.TenantID, domain.CarriedObjectReference) (ports.MeasuredDemandReading, error) {
	if double.before != nil {
		double.before()
	}
	if double.err != nil {
		return ports.MeasuredDemandReading{}, double.err
	}
	return double.reading, nil
}

func noneMeasured() measuredDemandDouble {
	return measuredDemandDouble{reading: ports.MeasuredDemandReading{Answer: ports.MeasuredDemandNone}}
}

func reprepareHandler(store *preparationStoreDouble, measured ports.MeasuredDemandSource) *application.PrepareOnPlannedLegsHandler {
	return application.NewPrepareOnPlannedLegsHandler(application.PrepareOnPlannedLegsDeps{
		Demand: demandDouble{}, Schedules: candidateSourceDouble{}, Pools: poolsOnSchedulesDouble{},
		Preparations: store, Clock: clockDouble{}, Measured: measured,
	})
}

func measuredReading(t *testing.T, anchor string, occurred time.Time) ports.MeasuredDemandReading {
	t.Helper()
	snapshot, err := domain.MeasuredDemand(domain.MeasuredDemandSpec{
		Anchor: anchor, WeightValue: "2.00", WeightUnit: "KG",
		Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatalf("实测快照：%v", err)
	}
	return ports.MeasuredDemandReading{Answer: ports.MeasuredDemandOne, Snapshot: snapshot, OccurredAt: occurred}
}

func seedDeclaration(t *testing.T, store *preparationStoreDouble, leg domain.PreparationLeg) {
	t.Helper()
	demand, err := domain.DeclaredDemand(domain.DeclaredDemandSpec{Anchor: "baseline", WeightValue: "1", WeightUnit: "KG"})
	if err != nil {
		t.Fatalf("申报：%v", err)
	}
	preparation, err := domain.PrepareTransportOpportunity(domain.PrepareTransportOpportunitySpec{
		TenantID: opportunityTenant(t), Leg: leg, Object: mustCarriedObject(t, "SYN-PARCEL-01"),
		Rule: domain.BuiltInPreparationRuleVersion(), Demand: demand, JudgedAt: preparedAt,
	})
	if err != nil {
		t.Fatalf("准备：%v", err)
	}
	digest, err := domain.CanonicalizePreparationInput(leg, demand)
	if err != nil {
		t.Fatalf("摘要：%v", err)
	}
	record := ports.TransportPreparationRecord{
		Key: ports.TransportPreparationKey{
			TenantID: opportunityTenant(t), Leg: leg.Reference(), Object: mustCarriedObject(t, "SYN-PARCEL-01"),
			Rule: domain.BuiltInPreparationRuleVersion(), Source: domain.DemandFromDeclaration,
		},
		InputDigest: digest, Preparation: preparation, RecordedAt: preparedAt,
	}
	store.records[record.Key] = record
}

func reprepare(t *testing.T, handler *application.PrepareOnPlannedLegsHandler) application.RePrepareOnMeasurementChangeResult {
	t.Helper()
	result, err := handler.RePrepareOnMeasurementChange(context.Background(), application.RePrepareOnMeasurementChangeCommand{
		TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01",
	})
	if err != nil {
		t.Fatalf("再准备：%v", err)
	}
	return result
}

func countMeasured(store *preparationStoreDouble) int {
	count := 0
	for key := range store.records {
		if key.Source == domain.DemandFromActualMeasurement {
			count++
		}
	}
	return count
}

// Covers: 实测变化入口的零写入各格——没有实测、定不下、没有申报行、窗口都已过完，以及命令本身立不成。
func TestRePrepareOnMeasurementChangeWritesNothingWhenThereIsNothingToJudge(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	cases := []struct {
		name    string
		seed    bool
		reading ports.MeasuredDemandReading
		command application.RePrepareOnMeasurementChangeCommand
		want    application.RePrepareRunOutcome
	}{
		{
			name: "没有实测", seed: true, reading: ports.MeasuredDemandReading{Answer: ports.MeasuredDemandNone},
			want: application.RePrepareRunNoMeasurement,
		},
		{
			name: "实测定不下", seed: true, reading: ports.MeasuredDemandReading{Answer: ports.MeasuredDemandUndetermined},
			want: application.RePrepareRunMeasurementUndetermined,
		},
		{
			name: "还没有准备结果", reading: measuredReading(t, "SYN-MEASURE-1", preparedAt),
			want: application.RePrepareRunNoDeclaration,
		},
		{
			name: "窗口早于实测", seed: true, reading: measuredReading(t, "SYN-MEASURE-1", preparedAt.Add(time.Hour+time.Second)),
			want: application.RePrepareRunWindowClosed,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
			if test.seed {
				seedDeclaration(t, store, leg)
			}
			before := len(store.records)
			handler := reprepareHandler(store, measuredDemandDouble{reading: test.reading})
			command := test.command
			if command.Object == "" && command.TenantID.String() == "" {
				command = application.RePrepareOnMeasurementChangeCommand{TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01"}
			}
			result, err := handler.RePrepareOnMeasurementChange(context.Background(), command)
			if err != nil || result.Outcome() != test.want || len(result.Legs()) != 0 || len(store.records) != before {
				t.Fatalf("%s = %s %v 段 %d 行 %d→%d", test.name, result.Outcome(), err, len(result.Legs()), before, len(store.records))
			}
		})
	}

	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	blank, err := reprepareHandler(store, noneMeasured()).RePrepareOnMeasurementChange(context.Background(), application.RePrepareOnMeasurementChangeCommand{})
	if err != nil || blank.Outcome() != application.RePrepareRunNotAccepted || len(store.records) != 0 {
		t.Fatalf("空命令 = %s %v", blank.Outcome(), err)
	}
}

// Covers: 窗口结束时刻早于实测发生时刻的段跳过，等于该时刻的不跳；新形成的那一份带来源与实测引用。
func TestRePrepareSkipsLegsWhoseWindowEndedBeforeTheMeasurement(t *testing.T) {
	occurred := preparedAt.Add(2 * time.Hour)
	closed := plannedLegBetween(t, "RPV-1#1", preparedAt, preparedAt.Add(time.Hour))
	open := plannedLegBetween(t, "RPV-1#2", preparedAt, occurred)
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	seedDeclaration(t, store, closed)
	seedDeclaration(t, store, open)
	result := reprepare(t, reprepareHandler(store, measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-1", occurred)}))
	if result.Outcome() != application.RePrepareRunAnswered || len(result.Legs()) != 1 {
		t.Fatalf("再准备 = %s 段 %d", result.Outcome(), len(result.Legs()))
	}
	answered := result.Legs()[0]
	if answered.Answer != application.LegPreparationFormed || answered.Record.Key.Leg.String() != open.Reference().String() ||
		answered.Record.Key.Source != domain.DemandFromActualMeasurement || answered.Record.Key.MeasurementRef != "SYN-MEASURE-1" {
		t.Fatalf("作答 = %s %s %s %q", answered.Answer, answered.Record.Key.Leg, answered.Record.Key.Source, answered.Record.Key.MeasurementRef)
	}
	if countMeasured(store) != 1 {
		t.Fatalf("实测行 %d", countMeasured(store))
	}
}

// Covers: 同一实测重放答已有结果；换一条实测另落一份，前一份与申报那一份都不动。
func TestRePrepareReplaysTheSameMeasurementAndKeepsADifferentOne(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	seedDeclaration(t, store, leg)
	handler := reprepareHandler(store, &measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-1", preparedAt)})
	first := reprepare(t, handler)
	if first.Outcome() != application.RePrepareRunAnswered || first.Legs()[0].Answer != application.LegPreparationFormed {
		t.Fatalf("首判 = %s %s", first.Outcome(), first.Legs()[0].Answer)
	}
	again := reprepare(t, handler)
	if again.Outcome() != application.RePrepareRunAnswered || again.Legs()[0].Answer != application.LegPreparationExisting || countMeasured(store) != 1 {
		t.Fatalf("重放 = %s %s 实测行 %d", again.Outcome(), again.Legs()[0].Answer, countMeasured(store))
	}
	handler = reprepareHandler(store, &measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-2", preparedAt)})
	second := reprepare(t, handler)
	if second.Outcome() != application.RePrepareRunAnswered || second.Legs()[0].Answer != application.LegPreparationFormed ||
		second.Legs()[0].Record.Key.MeasurementRef != "SYN-MEASURE-2" || countMeasured(store) != 2 {
		t.Fatalf("另一条 = %s %s %q 实测行 %d", second.Outcome(), second.Legs()[0].Answer, second.Legs()[0].Record.Key.MeasurementRef, countMeasured(store))
	}
	if _, found := store.records[first.Legs()[0].Record.Key]; !found {
		t.Fatal("前一条实测被盖掉了")
	}
}

// Covers: 读不动答未决且点名实测需求量；译不成原样上抛；一段未决则整次未决。
func TestRePrepareSeparatesADeadMeasurementReadFromAnUndecidedLeg(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	seedDeclaration(t, store, leg)
	down := errors.New("measurement down")
	result, err := reprepareHandler(store, measuredDemandDouble{err: down}).RePrepareOnMeasurementChange(context.Background(), application.RePrepareOnMeasurementChangeCommand{
		TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01",
	})
	if err != nil || result.Outcome() != application.RePrepareRunUndecided || result.UndecidedReason() != application.MeasuredDemandUnavailable ||
		result.ContinuationReference() == "" || countMeasured(store) != 0 {
		t.Fatalf("读不动 = %s %s %q %v", result.Outcome(), result.UndecidedReason(), result.ContinuationReference(), err)
	}

	untranslatable := reprepareHandler(store, measuredDemandDouble{err: errors.Join(ports.ErrMeasuredDemandUntranslatable, errors.New("edge"))})
	if _, err := untranslatable.RePrepareOnMeasurementChange(context.Background(), application.RePrepareOnMeasurementChangeCommand{
		TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01",
	}); !errors.Is(err, ports.ErrMeasuredDemandUntranslatable) || countMeasured(store) != 0 {
		t.Fatalf("译不成 = %v", err)
	}

	later := plannedLeg(t, "RPV-1#2")
	seedDeclaration(t, store, later)
	store.failFind = later.Reference().String()
	undecided := reprepare(t, reprepareHandler(store, measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-1", preparedAt)}))
	if undecided.Outcome() != application.RePrepareRunUndecided || undecided.UndecidedReason() != application.TransportPreparationStoreUnavailable {
		t.Fatalf("一段未决 = %s %s", undecided.Outcome(), undecided.UndecidedReason())
	}
}

// Covers: 初始准备在申报那一份之外，有恰一条实测时每段再补一份；没有或定不下时申报那一份与改动前一样；读口没接上要响亮失败。
func TestInitialPreparationAddsAMeasuredCopyBesideTheDeclaration(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	withMeasurement := reprepareHandler(store, measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-1", preparedAt)})
	answered, err := withMeasurement.Prepare(context.Background(), formedCommand(t, leg))
	if err != nil || answered.Outcome() != application.PreparationRunAnswered || len(answered.Legs()) != 2 {
		t.Fatalf("有实测 = %s %d %v", answered.Outcome(), len(answered.Legs()), err)
	}
	if answered.Legs()[0].Record.Key.Source != domain.DemandFromDeclaration || answered.Legs()[0].Record.Key.MeasurementRef != "" ||
		answered.Legs()[1].Record.Key.Source != domain.DemandFromActualMeasurement || answered.Legs()[1].Record.Key.MeasurementRef != "SYN-MEASURE-1" ||
		answered.Legs()[0].Record.Key.Leg != answered.Legs()[1].Record.Key.Leg {
		t.Fatalf("两份 = %+v / %+v", answered.Legs()[0].Record.Key, answered.Legs()[1].Record.Key)
	}

	bare := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	without, err := reprepareHandler(bare, noneMeasured()).Prepare(context.Background(), formedCommand(t, leg))
	if err != nil || without.Outcome() != application.PreparationRunAnswered || len(without.Legs()) != 1 ||
		without.Legs()[0].Record.Key.Source != domain.DemandFromDeclaration || countMeasured(bare) != 0 {
		t.Fatalf("没有实测 = %s %d %v", without.Outcome(), len(without.Legs()), err)
	}

	undeterminedStore := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	undetermined, err := reprepareHandler(undeterminedStore, measuredDemandDouble{reading: ports.MeasuredDemandReading{Answer: ports.MeasuredDemandUndetermined}}).
		Prepare(context.Background(), formedCommand(t, leg))
	if err != nil || undetermined.Outcome() != application.PreparationRunAnswered || len(undetermined.Legs()) != 1 || countMeasured(undeterminedStore) != 0 {
		t.Fatalf("定不下 = %s %d %v", undetermined.Outcome(), len(undetermined.Legs()), err)
	}

	downStore := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	down, err := reprepareHandler(downStore, measuredDemandDouble{err: errors.New("measurement down")}).Prepare(context.Background(), formedCommand(t, leg))
	if err != nil || down.Outcome() != application.PreparationRunUndecided || down.UndecidedReason() != application.MeasuredDemandUnavailable {
		t.Fatalf("读不动 = %s %s %v", down.Outcome(), down.UndecidedReason(), err)
	}

	if _, err := reprepareHandler(&preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}, nil).
		Prepare(context.Background(), formedCommand(t, leg)); !errors.Is(err, application.ErrMeasuredDemandSourceUnwired) {
		t.Fatalf("读口没接上 = %v", err)
	}
}

// Covers: 初始准备交来两段时，申报两段都判；窗口结束早于实测发生时刻的那段不补实测，结束时刻不早于发生时刻的才补。
func TestInitialPreparationSupplementsOnlyLegsOpenAtTheMeasurement(t *testing.T) {
	occurred := preparedAt.Add(2 * time.Hour)
	closed := plannedLegBetween(t, "RPV-1#1", preparedAt, preparedAt.Add(time.Hour))
	open := plannedLegBetween(t, "RPV-1#2", preparedAt, occurred)
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	answered, err := reprepareHandler(store, measuredDemandDouble{reading: measuredReading(t, "SYN-MEASURE-1", occurred)}).
		Prepare(context.Background(), formedCommand(t, closed, open))
	if err != nil || answered.Outcome() != application.PreparationRunAnswered {
		t.Fatalf("初始准备 = %s %v", answered.Outcome(), err)
	}
	var declared, measured []string
	for _, leg := range answered.Legs() {
		if leg.Answer != application.LegPreparationFormed {
			t.Fatalf("作答 %s %s", leg.Answer, leg.Record.Key.Leg)
		}
		switch leg.Record.Key.Source {
		case domain.DemandFromDeclaration:
			if leg.Record.Key.MeasurementRef != "" {
				t.Fatalf("申报引用 %q", leg.Record.Key.MeasurementRef)
			}
			declared = append(declared, leg.Record.Key.Leg.String())
		case domain.DemandFromActualMeasurement:
			if leg.Record.Key.MeasurementRef != "SYN-MEASURE-1" {
				t.Fatalf("实测引用 %q", leg.Record.Key.MeasurementRef)
			}
			measured = append(measured, leg.Record.Key.Leg.String())
		default:
			t.Fatalf("来源 %s", leg.Record.Key.Source)
		}
	}
	if len(declared) != 2 || declared[0] != closed.Reference().String() || declared[1] != open.Reference().String() ||
		len(measured) != 1 || measured[0] != open.Reference().String() || countMeasured(store) != 1 {
		t.Fatalf("申报 %v 实测 %v 实测行 %d", declared, measured, countMeasured(store))
	}
}

// Covers: 列申报段时读不懂的行原样上抛、不答未决；别的错误才答未决并点名准备结果库。
func TestRePrepareRaisesAnUnreadableDeclaredListAndWaitsWhenTheStoreIsDown(t *testing.T) {
	reading := measuredReading(t, "SYN-MEASURE-1", preparedAt)
	command := application.RePrepareOnMeasurementChangeCommand{TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01"}
	ask := func(listErr error) (application.RePrepareOnMeasurementChangeResult, error) {
		t.Helper()
		store := &preparationStoreDouble{
			records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{},
			listErr: listErr,
		}
		result, err := reprepareHandler(store, measuredDemandDouble{reading: reading}).
			RePrepareOnMeasurementChange(context.Background(), command)
		if len(store.records) != 0 {
			t.Fatalf("写了 %d 行", len(store.records))
		}
		return result, err
	}
	unreadable, err := ask(errors.Join(ports.ErrTransportPreparationRecordUnreadable, errors.New("bad row")))
	if !errors.Is(err, ports.ErrTransportPreparationRecordUnreadable) || unreadable.Outcome() == application.RePrepareRunUndecided {
		t.Fatalf("读不懂 = %s %v", unreadable.Outcome(), err)
	}
	down, err := ask(errors.New("preparation store down"))
	if err != nil || down.Outcome() != application.RePrepareRunUndecided ||
		down.UndecidedReason() != application.TransportPreparationStoreUnavailable ||
		down.UndecidedReason().String() != "TRANSPORT_PREPARATION_STORE_UNAVAILABLE" ||
		down.ContinuationReference() == "" {
		t.Fatalf("读不动 = %s %s %q %v", down.Outcome(), down.UndecidedReason(), down.ContinuationReference(), err)
	}
}

type countingDeclaredDemand struct{ calls int }

func (demand *countingDeclaredDemand) LoadDeclaredDemand(context.Context, domain.TenantID, domain.CarriedObjectReference) (domain.DemandSnapshot, error) {
	demand.calls++
	return domain.DeclaredDemand(domain.DeclaredDemandSpec{WeightValue: "1", WeightUnit: "KG"})
}

// Covers: 取锁失败按准备结果库未决、零写入。锁在读之前，读口不该被叫到。
func TestAFailedPreparationLockLeavesBothEntriesUndecided(t *testing.T) {
	down := errors.New("preparation lock down")
	leg := plannedLeg(t, "RPV-1#1")
	declared := &countingDeclaredDemand{}
	store := &preparationStoreDouble{
		records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{},
		holdErr: down,
	}
	handler := application.NewPrepareOnPlannedLegsHandler(application.PrepareOnPlannedLegsDeps{
		Demand: declared, Schedules: candidateSourceDouble{}, Pools: poolsOnSchedulesDouble{},
		Preparations: store, Clock: clockDouble{}, Measured: noneMeasured(),
	})
	prepared, err := handler.Prepare(context.Background(), formedCommand(t, leg))
	if err != nil || prepared.Outcome() != application.PreparationRunUndecided ||
		prepared.UndecidedReason() != application.TransportPreparationStoreUnavailable ||
		prepared.UndecidedReason().String() != "TRANSPORT_PREPARATION_STORE_UNAVAILABLE" ||
		prepared.ContinuationReference() == "" || len(store.records) != 0 || declared.calls != 0 {
		t.Fatalf("初始准备 = %s %s %q 行 %d 申报读 %d %v",
			prepared.Outcome(), prepared.UndecidedReason(), prepared.ContinuationReference(), len(store.records), declared.calls, err)
	}

	seeded := &preparationStoreDouble{
		records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{},
		holdErr: down,
	}
	seedDeclaration(t, seeded, leg)
	var loaded int
	measured := measuredDemandDouble{
		reading: measuredReading(t, "SYN-MEASURE-1", preparedAt),
		before:  func() { loaded++ },
	}
	again, err := reprepareHandler(seeded, measured).RePrepareOnMeasurementChange(context.Background(), application.RePrepareOnMeasurementChangeCommand{
		TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01",
	})
	if err != nil || again.Outcome() != application.RePrepareRunUndecided ||
		again.UndecidedReason() != application.TransportPreparationStoreUnavailable ||
		again.UndecidedReason().String() != "TRANSPORT_PREPARATION_STORE_UNAVAILABLE" ||
		again.ContinuationReference() == "" || countMeasured(seeded) != 0 || loaded != 0 || len(seeded.records) != 1 {
		t.Fatalf("再准备 = %s %s %q 实测行 %d 实测读 %d 行 %d %v",
			again.Outcome(), again.UndecidedReason(), again.ContinuationReference(), countMeasured(seeded), loaded, len(seeded.records), err)
	}
}

// Covers: 实测再准备先取对象锁，再问实测、再列申报段。顺序倒了，称重已提交而申报行还没提交时会把空名单入账。
func TestRePrepareHoldsTheObjectBeforeReadingDemandOrDeclaredLegs(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	var steps []string
	store := &preparationStoreDouble{
		records:    map[ports.TransportPreparationKey]ports.TransportPreparationRecord{},
		beforeHold: func() { steps = append(steps, "hold") },
		beforeList: func() { steps = append(steps, "list") },
	}
	seedDeclaration(t, store, leg)
	measured := measuredDemandDouble{
		reading: measuredReading(t, "SYN-MEASURE-1", preparedAt),
		before:  func() { steps = append(steps, "load") },
	}
	result := reprepare(t, reprepareHandler(store, measured))
	if result.Outcome() != application.RePrepareRunAnswered {
		t.Fatalf("再准备 = %s", result.Outcome())
	}
	holdAt, loadAt, listAt := -1, -1, -1
	for index, step := range steps {
		switch step {
		case "hold":
			if holdAt < 0 {
				holdAt = index
			}
		case "load":
			if loadAt < 0 {
				loadAt = index
			}
		case "list":
			if listAt < 0 {
				listAt = index
			}
		}
	}
	if holdAt < 0 || loadAt < 0 || listAt < 0 || holdAt > loadAt || holdAt > listAt {
		t.Fatalf("调用序 = %v，want 先 hold 再 load 与 list", steps)
	}
}

func plannedLegBetween(t *testing.T, reference string, start, end time.Time) domain.PreparationLeg {
	t.Helper()
	direction, err := domain.NewScheduleDirection("SYN-CONN-1", "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	segment, err := domain.NewPlannedSegmentReference(reference)
	if err != nil {
		t.Fatalf("段引用：%v", err)
	}
	leg, err := domain.NewPreparationLeg(segment, direction, start, end)
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	return leg
}
