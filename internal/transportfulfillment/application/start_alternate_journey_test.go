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

var (
	journeyStartAt    = time.Date(2026, 8, 13, 17, 0, 0, 0, time.UTC)
	journeyRecordedAt = time.Date(2026, 8, 13, 17, 30, 0, 0, time.UTC)
)

type journeyStoreDouble struct {
	records     map[string]ports.AlternateJourneyRecord
	findErr     error
	saveResult  ports.AlternateJourneySaveOutcome
	forceResult bool
	saves       int
}

func newJourneyStore() *journeyStoreDouble {
	return &journeyStoreDouble{records: map[string]ports.AlternateJourneyRecord{}}
}

func journeyStoreKey(key ports.AlternateJourneyKey) string {
	return key.TenantID.String() + "|" + key.Original.String() + "|" + key.Purpose.String() + "|" + key.Basis.String()
}

func (double *journeyStoreDouble) FindByKey(
	_ context.Context,
	key ports.AlternateJourneyKey,
) (ports.AlternateJourneyRecord, bool, error) {
	if double.findErr != nil {
		return ports.AlternateJourneyRecord{}, false, double.findErr
	}
	record, found := double.records[journeyStoreKey(key)]
	return record, found, nil
}

func (double *journeyStoreDouble) Save(
	_ context.Context,
	record ports.AlternateJourneyRecord,
) (ports.AlternateJourneySaveOutcome, error) {
	double.saves++
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[journeyStoreKey(record.Key)]; exists {
		return ports.AlternateJourneyAlreadyStarted, nil
	}
	double.records[journeyStoreKey(record.Key)] = record
	return ports.AlternateJourneySaved, nil
}

type dispositionHandoffDouble struct {
	intents []ports.AlternateJourneyIntent
	err     error
}

func (double *dispositionHandoffDouble) HandOffDispositionExecution(
	_ context.Context,
	intent ports.AlternateJourneyIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type exceptionHandoffDouble struct {
	intents []ports.AlternateJourneyIntent
	err     error
}

func (double *exceptionHandoffDouble) HandOffExceptionJourney(
	_ context.Context,
	intent ports.AlternateJourneyIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type journeyClock struct{ at time.Time }

func (clock journeyClock) Now() time.Time { return clock.at }

type journeyFixture struct {
	store       *journeyStoreDouble
	disposition *dispositionHandoffDouble
	exception   *exceptionHandoffDouble
	handler     *application.StartAlternateJourneyHandler
}

func newJourneyFixture(t *testing.T) *journeyFixture {
	t.Helper()
	fixture := &journeyFixture{
		store:       newJourneyStore(),
		disposition: &dispositionHandoffDouble{},
		exception:   &exceptionHandoffDouble{},
	}
	fixture.handler = application.NewStartAlternateJourneyHandler(application.StartAlternateJourneyDeps{
		Journeys:    fixture.store,
		Disposition: fixture.disposition,
		Exception:   fixture.exception,
		Clock:       journeyClock{at: journeyRecordedAt},
	})
	return fixture
}

func startJourneyCommand(t *testing.T, kind domain.DispositionBasisKind) application.StartAlternateJourneyCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.StartAlternateJourneyCommand{
		TenantID:  tenant,
		Journey:   "journey-return-1",
		Purpose:   domain.ReturnJourneyPurpose,
		Original:  "journey-original-1",
		BasisKind: kind,
		Basis:     "disposition-decision-1",
		Members:   []string{"parcel-1"},
		StartedAt: journeyStartAt,
	}
}

// Covers: `AT-TF-080` 的编排面——监管来路（UC-TF-001 处置承接）的旅程双链分流：CC 处置
// 执行事实源+VE 异常链；同一处置不开两条旅程（幂等键含处置依据）；异内容同键冲突。
func TestARegulatoryJourneyFeedsBothChains(t *testing.T) {
	fixture := newJourneyFixture(t)
	command := startJourneyCommand(t, domain.RegulatoryDispositionDecision)

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if first.Outcome() != application.JourneyStarted {
		t.Fatalf("outcome = %q, want JOURNEY_STARTED", first.Outcome())
	}
	record, _ := first.Record()
	if !record.Journey.RegulatoryOrigin() {
		t.Fatal("监管来路没有被显式标记")
	}
	if len(fixture.disposition.intents) != 1 || len(fixture.exception.intents) != 1 {
		t.Fatalf("cc = %d ve = %d, want 1/1（监管旅程双链）", len(fixture.disposition.intents), len(fixture.exception.intents))
	}

	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.JourneyExistingResult || fixture.store.saves != 1 {
		t.Fatalf("outcome = %q saves = %d（同一处置不开两条旅程）", replay.Outcome(), fixture.store.saves)
	}
	if len(fixture.disposition.intents) != 2 || len(fixture.exception.intents) != 2 {
		t.Fatalf("cc = %d ve = %d（重放重发同一份）", len(fixture.disposition.intents), len(fixture.exception.intents))
	}

	t.Run("a different journey under the same disposition is a conflict", func(t *testing.T) {
		flipped := startJourneyCommand(t, domain.RegulatoryDispositionDecision)
		flipped.Journey = "journey-return-2"
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict start: %v", err)
		}
		if result.Outcome() != application.JourneyStartConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（同一处置决定不开两条替代旅程）", result.Outcome())
		}
	})
}

// Covers: 领域硬句「监管退运走 UC-TF-001 另标意图，不得用普通退运冒充」的编排面——
// 非监管来路只交 VE 异常链，不进 CC 处置执行链。
func TestAServiceJourneyFeedsOnlyTheExceptionChain(t *testing.T) {
	fixture := newJourneyFixture(t)
	command := startJourneyCommand(t, domain.ServiceDispositionDecision)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if result.Outcome() != application.JourneyStarted {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if len(fixture.disposition.intents) != 0 {
		t.Fatalf("cc intents = %d, want 0（普通退运不得混进监管链）", len(fixture.disposition.intents))
	}
	if len(fixture.exception.intents) != 1 {
		t.Fatalf("ve intents = %d, want 1", len(fixture.exception.intents))
	}
}

// 未受理与恢复纪律：缺依据/同旅程自指/空成员未受理；库故障未决；CC 链投递失败不翻
// 结果重放补发；写入代数外是编程错误。
func TestJourneyRecoveryDiscipline(t *testing.T) {
	broken := map[string]func(*application.StartAlternateJourneyCommand){
		"no basis":       func(command *application.StartAlternateJourneyCommand) { command.Basis = " " },
		"self reference": func(command *application.StartAlternateJourneyCommand) { command.Original = command.Journey },
		"no members":     func(command *application.StartAlternateJourneyCommand) { command.Members = nil },
		"invalid basiskind": func(command *application.StartAlternateJourneyCommand) {
			command.BasisKind = domain.DispositionBasisKindInvalid
		},
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newJourneyFixture(t)
			command := startJourneyCommand(t, domain.RegulatoryDispositionDecision)
			breakCommand(&command)
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if result.Outcome() != application.JourneyNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if len(fixture.store.records) != 0 {
				t.Fatal("未受理的旅程落了库")
			}
		})
	}

	t.Run("a store failure is undecided", func(t *testing.T) {
		fixture := newJourneyFixture(t)
		fixture.store.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), startJourneyCommand(t, domain.RegulatoryDispositionDecision))
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		if result.Outcome() != application.JourneyUndecided ||
			result.UndecidedReason() != application.JourneyStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a CC handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newJourneyFixture(t)
		fixture.disposition.err = errors.New("cc downstream unavailable")
		command := startJourneyCommand(t, domain.RegulatoryDispositionDecision)
		first, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		if first.Outcome() != application.JourneyStarted || first.DispositionHandoffReference() == "" {
			t.Fatalf("outcome = %q cc = %q（投递失败不翻结果）", first.Outcome(), first.DispositionHandoffReference())
		}
		if first.ExceptionHandoffReference() != "" || len(fixture.exception.intents) != 1 {
			t.Fatal("CC 失败殃及了 VE 链")
		}
		fixture.disposition.err = nil
		replay, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.DispositionHandoffReference() != "" || len(fixture.disposition.intents) != 1 {
			t.Fatalf("cc intents = %d handoff = %q（重放补发监管链）", len(fixture.disposition.intents), replay.DispositionHandoffReference())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newJourneyFixture(t)
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.AlternateJourneySaveOutcome(99)
		if _, err := fixture.handler.Handle(context.Background(), startJourneyCommand(t, domain.RegulatoryDispositionDecision)); !errors.Is(err, application.ErrUnexpectedJourneySave) {
			t.Fatalf("error = %v, want ErrUnexpectedJourneySave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		if application.JourneyStoreUnavailable.String() == "" {
			t.Fatal("reason has no label")
		}
		if application.JourneyUndecidedReason(2).String() != "" {
			t.Fatal("第二个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
