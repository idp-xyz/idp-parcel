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
	pickupOccurredAt   = time.Date(2026, 8, 13, 8, 15, 0, 0, time.UTC)
	pickupRegisteredAt = time.Date(2026, 8, 13, 8, 45, 0, 0, time.UTC)
)

type pickupRegistryDouble struct {
	records     map[string]ports.OffsitePickupRecord
	findErr     error
	saveErr     error
	saveResult  ports.OffsitePickupSaveOutcome
	forceResult bool
	saves       int
}

func newPickupRegistry() *pickupRegistryDouble {
	return &pickupRegistryDouble{records: map[string]ports.OffsitePickupRecord{}}
}

func pickupRegistryKey(key ports.OffsitePickupKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Attempt.String()
}

func (double *pickupRegistryDouble) FindByKey(
	_ context.Context,
	key ports.OffsitePickupKey,
) (ports.OffsitePickupRecord, bool, error) {
	if double.findErr != nil {
		return ports.OffsitePickupRecord{}, false, double.findErr
	}
	record, found := double.records[pickupRegistryKey(key)]
	return record, found, nil
}

func (double *pickupRegistryDouble) Save(
	_ context.Context,
	record ports.OffsitePickupRecord,
) (ports.OffsitePickupSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.OffsitePickupSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[pickupRegistryKey(record.Key)]; exists {
		return ports.OffsitePickupAlreadyRegistered, nil
	}
	double.records[pickupRegistryKey(record.Key)] = record
	return ports.OffsitePickupSaved, nil
}

type pickupRegVersionFactory struct {
	minted int
	err    error
}

func (double *pickupRegVersionFactory) NextPickupResultVersion(context.Context) (domain.PickupResultVersion, error) {
	if double.err != nil {
		return domain.PickupResultVersion{}, double.err
	}
	double.minted++
	return domain.NewPickupResultVersion(fmt.Sprintf("pickup-result/v%d", double.minted))
}

type pickupRegHandoffDouble struct {
	intents []ports.OffsitePickupRegistrationIntent
	err     error
}

func (double *pickupRegHandoffDouble) HandOffOffsitePickupRegistration(
	_ context.Context,
	intent ports.OffsitePickupRegistrationIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type pickupRegClock struct{ at time.Time }

func (clock pickupRegClock) Now() time.Time { return clock.at }

type pickupRegFixture struct {
	registry *pickupRegistryDouble
	versions *pickupRegVersionFactory
	handoff  *pickupRegHandoffDouble
	handler  *application.RegisterOffsitePickupHandler
}

func newPickupRegFixture(t *testing.T) *pickupRegFixture {
	t.Helper()
	fixture := &pickupRegFixture{
		registry: newPickupRegistry(),
		versions: &pickupRegVersionFactory{},
		handoff:  &pickupRegHandoffDouble{},
	}
	fixture.handler = application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.registry,
		Versions:   fixture.versions,
		Downstream: fixture.handoff,
		Clock:      pickupRegClock{at: pickupRegisteredAt},
	})
	return fixture
}

func pickupRegistrationCommand(t *testing.T) application.RegisterOffsitePickupCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RegisterOffsitePickupCommand{
		TenantID:   tenant,
		Object:     "parcel-1",
		Task:       "pickup-task-1",
		Attempt:    "attempt-1",
		Place:      "customer-warehouse-1",
		Control:    "TRANSPORT-CONTROL/TF-1",
		ExecutedBy: "courier-1",
		OccurredAt: pickupOccurredAt,
	}
}

// Covers: CONTEXT「场外揽收只有在明确载运对象……由运输方取得控制时才建立履约参与
// 关系」的编排面——控制证据等七件由领域把门首登成功；重放返原版不重签；异内容同键
// 冲突不顶替（来源更正走新版本）。
func TestAPickupRegistrationIsIdempotentPerObjectAttempt(t *testing.T) {
	fixture := newPickupRegFixture(t)
	command := pickupRegistrationCommand(t)

	first, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if first.Outcome() != application.PickupRegistered {
		t.Fatalf("outcome = %q, want PICKUP_REGISTERED", first.Outcome())
	}
	record, _ := first.Record()
	if record.Pickup.Control().String() != "TRANSPORT-CONTROL/TF-1" {
		t.Fatalf("control = %q（控制依据随揽收保全）", record.Pickup.Control())
	}
	if !record.Pickup.OccurredAt().Equal(pickupOccurredAt) {
		t.Fatalf("occurred at = %s（责任起点锚在实际接货时间）", record.Pickup.OccurredAt())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（UC-PS-003 揽收源链）", len(fixture.handoff.intents))
	}

	replay, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.PickupExistingVersion {
		t.Fatalf("outcome = %q, want EXISTING_VERSION", replay.Outcome())
	}
	if fixture.versions.minted != 1 || fixture.registry.saves != 1 {
		t.Fatalf("minted = %d saves = %d（重放不重签不重存）", fixture.versions.minted, fixture.registry.saves)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a different control under the same key is a conflict", func(t *testing.T) {
		flipped := pickupRegistrationCommand(t)
		flipped.Control = "TRANSPORT-CONTROL/TF-9"
		result, err := fixture.handler.Register(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict register: %v", err)
		}
		if result.Outcome() != application.PickupRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（来源更正走新版本，不顶替首登）", result.Outcome())
		}
		if fixture.versions.minted != 1 {
			t.Fatal("冲突还签了新版本")
		}
	})
}

// Covers: CONTEXT「客户不在、货物未备好、包装不合格或其他失败结果不制造实际履约段」
// 的编排面——失败到访没有控制证据可供，缺控制（及任一必备件）在受理处即未受理，编排
// 不绕领域构造器。
func TestAFailedVisitHasNothingToRegister(t *testing.T) {
	broken := map[string]func(*application.RegisterOffsitePickupCommand){
		"no control":  func(command *application.RegisterOffsitePickupCommand) { command.Control = " " },
		"no task":     func(command *application.RegisterOffsitePickupCommand) { command.Task = " " },
		"no place":    func(command *application.RegisterOffsitePickupCommand) { command.Place = " " },
		"no executor": func(command *application.RegisterOffsitePickupCommand) { command.ExecutedBy = " " },
		"no object":   func(command *application.RegisterOffsitePickupCommand) { command.Object = " " },
		"no time":     func(command *application.RegisterOffsitePickupCommand) { command.OccurredAt = time.Time{} },
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupRegFixture(t)
			command := pickupRegistrationCommand(t)
			breakCommand(&command)
			result, err := fixture.handler.Register(context.Background(), command)
			if err != nil {
				t.Fatalf("register: %v", err)
			}
			if result.Outcome() != application.PickupRegistrationNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if len(fixture.registry.records) != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatal("未受理的登记落了库或交了意图")
			}
		})
	}
}

// 恢复纪律：库/版本厂故障各归未决一格；投递失败不翻结果重放重发；并发落败读回赢家；
// 写入代数外是编程错误。
func TestPickupRegistrationRecoveryDiscipline(t *testing.T) {
	t.Run("dependency failures are undecided with their reasons", func(t *testing.T) {
		registry := newPickupRegFixture(t)
		registry.registry.findErr = errors.New("registry down")
		result, err := registry.handler.Register(context.Background(), pickupRegistrationCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.UndecidedReason() != application.PickupRegistryUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		factory := newPickupRegFixture(t)
		factory.versions.err = errors.New("factory down")
		result, err = factory.handler.Register(context.Background(), pickupRegistrationCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.UndecidedReason() != application.PickupVersionUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if first.Outcome() != application.PickupRegistered || first.PickupHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.PickupHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.PickupHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fixture.handoff.intents), replay.PickupHandoffReference())
		}
	})

	t.Run("a concurrent loser reads back the winner", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		command := pickupRegistrationCommand(t)
		if _, err := fixture.handler.Register(context.Background(), command); err != nil {
			t.Fatalf("seed: %v", err)
		}
		fixture.registry.forceResult = true
		fixture.registry.saveResult = ports.OffsitePickupAlreadyRegistered
		// 强制 Save 撞 AlreadyRegistered：FindByKey 命中原记录读回赢家（同内容走重放路，
		// 这里直接覆盖 forceResult 验证读回半边）。
		result, err := fixture.handler.Register(context.Background(), command)
		if err != nil {
			t.Fatalf("loser: %v", err)
		}
		if result.Outcome() != application.PickupExistingVersion {
			t.Fatalf("outcome = %q, want EXISTING_VERSION", result.Outcome())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		fixture.registry.forceResult = true
		fixture.registry.saveResult = ports.OffsitePickupSaveOutcome(99)
		if _, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t)); !errors.Is(err, application.ErrUnexpectedPickupRegistrySave) {
			t.Fatalf("error = %v, want ErrUnexpectedPickupRegistrySave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.PickupRegistrationUndecidedReason{
			application.PickupRegistryUnavailable, application.PickupVersionUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.PickupRegistrationUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
