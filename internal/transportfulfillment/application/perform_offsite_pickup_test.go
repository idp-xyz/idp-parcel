package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	plannedFrom = time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	plannedTo   = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	arrivedAt   = time.Date(2026, 8, 9, 8, 10, 0, 0, time.UTC)
	recordedAt  = time.Date(2026, 8, 9, 8, 30, 0, 0, time.UTC)
)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type pickupStoreDouble struct {
	records     map[string]ports.PickupAttemptRecord
	findErr     error
	saveErr     error
	saveResult  ports.PickupSaveOutcome
	forceResult bool
	missNext    bool
	saves       int
}

func newPickupStore() *pickupStoreDouble {
	return &pickupStoreDouble{records: map[string]ports.PickupAttemptRecord{}}
}

func (double *pickupStoreDouble) FindByKey(
	_ context.Context,
	key ports.PickupAttemptKey,
) (ports.PickupAttemptRecord, bool, error) {
	if double.findErr != nil {
		return ports.PickupAttemptRecord{}, false, double.findErr
	}
	if double.missNext {
		// 模拟并发窗口：赢家已提交但本方这一读还没看见。
		double.missNext = false
		return ports.PickupAttemptRecord{}, false, nil
	}
	record, found := double.records[key.SourceID]
	return record, found, nil
}

func (double *pickupStoreDouble) Save(
	_ context.Context,
	record ports.PickupAttemptRecord,
) (ports.PickupSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.PickupSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[record.Key.SourceID]; exists {
		return ports.PickupAlreadyRecorded, nil
	}
	double.records[record.Key.SourceID] = record
	return ports.PickupSaved, nil
}

type versionFactoryDouble struct {
	minted int
	err    error
}

func (double *versionFactoryDouble) NextPickupResultVersion(context.Context) (domain.PickupResultVersion, error) {
	if double.err != nil {
		return domain.PickupResultVersion{}, double.err
	}
	double.minted++
	return domain.NewPickupResultVersion(fmt.Sprintf("pickup-result/v%d", double.minted))
}

type pickupHandoffDouble struct {
	intents []ports.OffsitePickupHandoffIntent
	err     error
}

func (double *pickupHandoffDouble) HandOffOffsitePickup(
	_ context.Context,
	intent ports.OffsitePickupHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type pickupFixture struct {
	store    *pickupStoreDouble
	versions *versionFactoryDouble
	handoff  *pickupHandoffDouble
	handler  *application.PerformOffsitePickupHandler
}

func newPickupFixture(t *testing.T) *pickupFixture {
	t.Helper()
	fixture := &pickupFixture{
		store:    newPickupStore(),
		versions: &versionFactoryDouble{},
		handoff:  &pickupHandoffDouble{},
	}
	fixture.handler = application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
		Attempts:   fixture.store,
		Versions:   fixture.versions,
		Downstream: fixture.handoff,
		Clock:      fixedClock{at: recordedAt},
	})
	return fixture
}

func successSubmission(t *testing.T, object, control string) application.ObjectPickupSubmission {
	t.Helper()
	return application.ObjectPickupSubmission{
		Object:     value(t, domain.NewCarriedObjectReference, object),
		Outcome:    domain.ObjectPickedUp,
		Control:    value(t, domain.NewTransportControlReference, control),
		OccurredAt: arrivedAt.Add(5 * time.Minute),
	}
}

func failureSubmission(t *testing.T, object string, outcome domain.AttemptObjectOutcome, basis string) application.ObjectPickupSubmission {
	t.Helper()
	return application.ObjectPickupSubmission{
		Object:     value(t, domain.NewCarriedObjectReference, object),
		Outcome:    outcome,
		Basis:      value(t, domain.NewAttemptResultBasisReference, basis),
		OccurredAt: arrivedAt.Add(6 * time.Minute),
	}
}

func pickupCommand(t *testing.T, sourceID, attemptID string, objects ...application.ObjectPickupSubmission) application.PerformOffsitePickupCommand {
	t.Helper()
	return application.PerformOffsitePickupCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-1"),
		SourceID:    sourceID,
		Task:        "pickup-task-1",
		Attempt:     attemptID,
		ExecutedBy:  "courier-1",
		Place:       "customer-warehouse-1",
		PlannedFrom: plannedFrom,
		PlannedTo:   plannedTo,
		ArrivedAt:   arrivedAt,
		Evidence:    "attempt-evidence-1",
		Objects:     objects,
	}
}

// Covers: `AT-TF-013`/`AT-TF-015`——三对象两成两败逐对象并存：成功各自形成场外揽收
// （控制+逐对象版本），失败只留结果与原因；任务汇总不吞并失败对象。
func TestAMixedAttemptRecordsPerObjectResults(t *testing.T) {
	fixture := newPickupFixture(t)
	result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"),
		failureSubmission(t, "parcel-3", domain.CustomerAbsent, "reason-absent-1"),
	))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", result.Outcome())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("no record returned")
	}
	// 计数锚定本夹具：三对象来源（source-1，到场 2026-08-09T08:10Z）。
	if len(record.Results) != 3 {
		t.Fatalf("results = %d, want 3（逐对象结果并存）", len(record.Results))
	}
	if len(record.Pickups) != 2 {
		t.Fatalf("pickups = %d, want 2（只有取得控制的对象形成揽收）", len(record.Pickups))
	}
	if fixture.versions.minted != 2 {
		t.Fatalf("versions minted = %d, want 2（版本逐成功对象签发）", fixture.versions.minted)
	}
	if record.Pickups[0].Version() == record.Pickups[1].Version() {
		t.Fatal("两个对象共用了一个揽收结果版本")
	}
	for _, pickup := range record.Pickups {
		if pickup.Control().String() == "" {
			t.Fatal("揽收丢了控制依据")
		}
		if pickup.Object().String() == "parcel-3" {
			t.Fatal("失败对象混进了揽收集合")
		}
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（有成功对象的记录交一份意图）", len(fixture.handoff.intents))
	}
	if result.PickupHandoffReference() != "" {
		t.Fatal("意图已交却留了续办引用")
	}
	if !record.RecordedAt.Equal(recordedAt) {
		t.Fatalf("recorded at = %s", record.RecordedAt)
	}
}

// Covers: `AT-TF-016`「客户不在或货物未备好 → 形成失败尝试，不建立控制、实际履约段
// 或终局」——全失败记录零揽收、零版本、零意图（没有 PS 能采用的东西）。
func TestAnAllFailedAttemptBuildsNoSegmentAndHandsOffNothing(t *testing.T) {
	fixture := newPickupFixture(t)
	result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		failureSubmission(t, "parcel-1", domain.CustomerAbsent, "reason-absent-1"),
		failureSubmission(t, "parcel-2", domain.GoodsNotReady, "reason-not-ready-1"),
	))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED（失败也是要保全的结果）", result.Outcome())
	}
	record, _ := result.Record()
	if len(record.Pickups) != 0 {
		t.Fatalf("pickups = %d, want 0（失败结果不制造实际履约段）", len(record.Pickups))
	}
	if len(record.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(record.Results))
	}
	if fixture.versions.minted != 0 {
		t.Fatalf("versions minted = %d, want 0（没有揽收就没有版本可签）", fixture.versions.minted)
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatalf("intents = %d, want 0（全失败没有可交给 parcel-shipment 的东西）", len(fixture.handoff.intents))
	}
}

// Covers: `AT-TF-019`「同一尝试和内容重复回传 → 返回原结果」与 `AT-TF-024`「成功提交后
// 发布失败 → 不回退揽收，只重试原发布意图」——首投失败结果不翻，重放重发同一份。
func TestAReplayReturnsTheOriginalAndResendsTheSameIntent(t *testing.T) {
	fixture := newPickupFixture(t)
	fixture.handoff.err = errors.New("downstream unavailable")
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED（投递失败不翻结果）", first.Outcome())
	}
	if first.PickupHandoffReference() == "" {
		t.Fatal("投递失败没有留下续办引用")
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatal("失败的投递竟然登记了意图")
	}

	fixture.handoff.err = nil
	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.PickupExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	if replay.PickupHandoffReference() != "" {
		t.Fatal("意图已补交仍留续办引用")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（重放重发同一份）", len(fixture.handoff.intents))
	}
	original, _ := first.Record()
	resent := fixture.handoff.intents[0].Record
	if resent.ContentDigest != original.ContentDigest || resent.Key != original.Key {
		t.Fatal("重发的不是原来那份意图")
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want 1（重放不重复提交）", fixture.store.saves)
	}
}

// Covers: `AT-TF-020`「同一来源身份回传相反结果 → 形成冲突并查询原来源，不使用最后
// 消息覆盖」。
func TestAConflictingSourceKeepsTheOriginal(t *testing.T) {
	fixture := newPickupFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))); err != nil {
		t.Fatalf("seed handle: %v", err)
	}

	flipped, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		failureSubmission(t, "parcel-1", domain.CustomerAbsent, "reason-absent-1")))
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if flipped.Outcome() != application.PickupSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", flipped.Outcome())
	}
	kept := fixture.store.records["source-1"]
	if len(kept.Pickups) != 1 {
		t.Fatal("冲突覆盖了原结果")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（冲突不重发也不新交）", len(fixture.handoff.intents))
	}
}

// Covers: `AT-TF-021`「首次尝试失败，后续改约后第二次成功 → 两次尝试都保留，成功只在
// 第二次形成控制」。
func TestARescheduledSecondAttemptSucceedsWithoutTouchingTheFirst(t *testing.T) {
	fixture := newPickupFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		failureSubmission(t, "parcel-1", domain.GoodsNotReady, "reason-not-ready-1"))); err != nil {
		t.Fatalf("first attempt: %v", err)
	}

	retry := pickupCommand(t, "source-2", "attempt-2",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-2"))
	retry.RescheduledFrom = "attempt-1"
	retry.PlannedFrom = plannedFrom.Add(24 * time.Hour)
	retry.PlannedTo = plannedTo.Add(24 * time.Hour)
	retry.ArrivedAt = arrivedAt.Add(24 * time.Hour)
	retry.Objects[0].OccurredAt = arrivedAt.Add(24*time.Hour + 5*time.Minute)

	second, err := fixture.handler.Handle(context.Background(), retry)
	if err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if second.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", second.Outcome())
	}
	record, _ := second.Record()
	predecessor, rescheduled := record.Attempt.RescheduledFrom()
	if !rescheduled || predecessor.String() != "attempt-1" {
		t.Fatal("第二次尝试没有回指改约前身")
	}
	if len(fixture.store.records) != 2 {
		t.Fatalf("records = %d, want 2（两次尝试都保留）", len(fixture.store.records))
	}
	firstRecord := fixture.store.records["source-1"]
	if len(firstRecord.Pickups) != 0 || len(firstRecord.Results) != 1 {
		t.Fatal("改约成功改写了第一次失败尝试")
	}
	if len(record.Pickups) != 1 {
		t.Fatal("成功没有只在第二次形成")
	}
}

// 提交自身矛盾落`来源未受理`（成功缺控制、失败带控制、空对象集、尝试形状立不起来）：
// 恢复动作是改请求不是重试，与依赖故障的`未决`分格（ADR-0029）；都不落库、不签版本。
func TestAContradictorySubmissionIsNotAccepted(t *testing.T) {
	broken := map[string]func(*testing.T) application.PerformOffsitePickupCommand{
		"success without control": func(t *testing.T) application.PerformOffsitePickupCommand {
			submission := application.ObjectPickupSubmission{
				Object:     value(t, domain.NewCarriedObjectReference, "parcel-1"),
				Outcome:    domain.ObjectPickedUp,
				OccurredAt: arrivedAt.Add(5 * time.Minute),
			}
			return pickupCommand(t, "source-1", "attempt-1", submission)
		},
		"failure carrying control": func(t *testing.T) application.PerformOffsitePickupCommand {
			submission := failureSubmission(t, "parcel-1", domain.CustomerAbsent, "reason-absent-1")
			submission.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-9")
			return pickupCommand(t, "source-1", "attempt-1", submission)
		},
		"no objects": func(t *testing.T) application.PerformOffsitePickupCommand {
			return pickupCommand(t, "source-1", "attempt-1")
		},
		"failure without a reason basis": func(t *testing.T) application.PerformOffsitePickupCommand {
			submission := application.ObjectPickupSubmission{
				Object:     value(t, domain.NewCarriedObjectReference, "parcel-1"),
				Outcome:    domain.CustomerAbsent,
				OccurredAt: arrivedAt.Add(5 * time.Minute),
			}
			return pickupCommand(t, "source-1", "attempt-1", submission)
		},
		"blank attempt reference": func(t *testing.T) application.PerformOffsitePickupCommand {
			command := pickupCommand(t, "source-1", "",
				successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))
			return command
		},
	}
	for name, build := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupFixture(t)
			result, err := fixture.handler.Handle(context.Background(), build(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.PickupNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if result.UndecidedReason() != application.PickupUndecidedReasonNone {
				t.Fatalf("undecided reason = %q；未受理不指名依赖，指了调用方就会去等而不是改单", result.UndecidedReason())
			}
			if len(fixture.store.records) != 0 {
				t.Fatal("矛盾的来源仍然落了库")
			}
			if len(fixture.handoff.intents) != 0 {
				t.Fatal("矛盾的来源交出了意图")
			}
		})
	}
}

// 并发提交由写入代数裁决：落败方（先查未见、写入撞 AlreadyRecorded）读回赢家作答，
// 不覆盖、不再建第二份控制。
func TestAConcurrentLoserReadsBackTheWinner(t *testing.T) {
	fixture := newPickupFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}
	winner := fixture.store.records["source-1"]

	fixture.store.missNext = true
	fixture.store.forceResult = true
	fixture.store.saveResult = ports.PickupAlreadyRecorded
	loser, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("loser handle: %v", err)
	}
	if loser.Outcome() != application.PickupExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", loser.Outcome())
	}
	record, _ := loser.Record()
	if record.ContentDigest != winner.ContentDigest {
		t.Fatal("落败方没有读回赢家")
	}
	if fixture.versions.minted != 2 {
		// 落败方在写入前确实形成过自己的揽收（签了第二个版本），但它没有越过提交边界：
		// 库里仍只有赢家一份。
		t.Fatalf("versions minted = %d, want 2", fixture.versions.minted)
	}
	if len(fixture.store.records) != 1 {
		t.Fatalf("records = %d, want 1（并发没有第二份记录）", len(fixture.store.records))
	}
}

// 依赖故障落`未决`并以封闭原因指名等谁：揽收库读不回等库，版本厂答不上等版本厂——
// 不与`来源未受理`同格，也不用裸字符串标签区分（ADR-0029）。
func TestADependencyFailureIsUndecidedWithItsReason(t *testing.T) {
	t.Run("store unreadable", func(t *testing.T) {
		fixture := newPickupFixture(t)
		fixture.store.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
			successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.PickupUndecided {
			t.Fatalf("outcome = %q, want PICKUP_UNDECIDED", result.Outcome())
		}
		if result.UndecidedReason() != application.PickupStoreUnavailable {
			t.Fatalf("reason = %q, want PICKUP_STORE_UNAVAILABLE", result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没有留下续办引用")
		}
	})

	t.Run("identity factory unavailable", func(t *testing.T) {
		fixture := newPickupFixture(t)
		fixture.versions.err = errors.New("factory down")
		result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
			successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.PickupUndecided {
			t.Fatalf("outcome = %q, want PICKUP_UNDECIDED", result.Outcome())
		}
		if result.UndecidedReason() != application.PickupIdentityUnavailable {
			t.Fatalf("reason = %q, want PICKUP_IDENTITY_UNAVAILABLE", result.UndecidedReason())
		}
		if len(fixture.store.records) != 0 {
			t.Fatal("版本没签出来却落了库")
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.PickupUndecidedReason{
			application.PickupStoreUnavailable, application.PickupIdentityUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("reason labels collapsed into %d", len(labels))
		}
		if application.PickupUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// 写入代数之外的取值是编程错误，不是一种业务未决（同 ADR-0031 的封闭代数纪律）。
func TestAnUnexpectedSaveOutcomeIsAProgrammingError(t *testing.T) {
	fixture := newPickupFixture(t)
	fixture.store.forceResult = true
	fixture.store.saveResult = ports.PickupSaveOutcome(99)
	_, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")))
	if !errors.Is(err, application.ErrUnexpectedPickupSave) {
		t.Fatalf("error = %v, want ErrUnexpectedPickupSave", err)
	}
}
