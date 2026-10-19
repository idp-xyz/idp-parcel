package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证预报有效时窗的登记编排（ADR-0212 决定二末项；PAR-NET-18；ADR-0085 登记写面与受控批量口同一用例）：首登、重放答已有
// 结果、区间重叠答重叠并回显撞上的那一行；形不成一行的命令被拒，册不被问到；登记时刻取时钟。夹具全为合成（S 级）。

// windowRegisterDouble 是时窗册替身，照真库的代数答：与在册行重叠就不落、回显撞上的那一行。
type windowRegisterDouble struct {
	rows  []domain.PreAdviceValidityWindow
	asked int
	err   error
}

func (register *windowRegisterDouble) RegisterPreAdviceWindow(_ context.Context, window domain.PreAdviceValidityWindow) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	register.asked++
	if register.err != nil {
		return domain.PreAdviceValidityWindow{}, ports.DeliveryScopeAppendInvalid, register.err
	}
	for _, row := range register.rows {
		if row.TenantID() == window.TenantID() && row.Overlaps(window) {
			return row, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	register.rows = append(register.rows, window)
	return window, ports.DeliveryScopeAppended, nil
}

func (register *windowRegisterDouble) FindPreAdviceWindowAt(context.Context, domain.TenantID, time.Time) (domain.PreAdviceValidityWindow, bool, error) {
	panic("the registration does not read windows by instant")
}

var windowRegisteredAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)

func windowCommand(t *testing.T, from time.Time, grace time.Duration) application.RegisterPreAdviceWindowCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return application.RegisterPreAdviceWindowCommand{
		TenantID:      tenant,
		Lead:          24 * time.Hour,
		Grace:         grace,
		EffectiveFrom: from,
		Basis:         "SYN-BASIS-PSXM08-WINDOW",
	}
}

func windowHandler(t *testing.T, register ports.PreAdviceWindowRegister) *application.RegisterPreAdviceWindowHandler {
	t.Helper()
	handler, err := application.NewRegisterPreAdviceWindowHandler(application.RegisterPreAdviceWindowDeps{
		Windows: register,
		Clock:   fixedClock{at: windowRegisteredAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: 时窗登记的三格答案——首登答已登记、登记时刻取时钟；同一行原样再登答已有结果，回显的是第一次的那一行；与在册行区间
// 重叠答重叠，回显撞上的那一行、册上不多一行。
func TestRegisteringAValidityWindowAnswersRegisteredExistingOrOverlapping(t *testing.T) {
	register := &windowRegisterDouble{}
	handler := windowHandler(t, register)
	ctx := context.Background()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	first, err := handler.Handle(ctx, windowCommand(t, jan, 6*time.Hour))
	if err != nil || first.Outcome() != application.PreAdviceWindowRegistered {
		t.Fatalf("first = %s %v", first.Outcome(), err)
	}
	if !first.Window().RegisteredAt().Equal(windowRegisteredAt) {
		t.Fatalf("registeredAt = %v, want the clock's", first.Window().RegisteredAt())
	}

	replay, err := handler.Handle(ctx, windowCommand(t, jan, 6*time.Hour))
	if err != nil || replay.Outcome() != application.PreAdviceWindowExisting || !replay.Window().SameContent(first.Window()) {
		t.Fatalf("replay = %s %v", replay.Outcome(), err)
	}

	overlapping, err := handler.Handle(ctx, windowCommand(t, jan.AddDate(0, 6, 0), 8*time.Hour))
	if err != nil || overlapping.Outcome() != application.PreAdviceWindowOverlapping {
		t.Fatalf("overlapping = %s %v", overlapping.Outcome(), err)
	}
	if overlapping.Window().Grace() != 6*time.Hour {
		t.Fatalf("echo = %v, want the registered row", overlapping.Window().Grace())
	}
	if len(register.rows) != 1 {
		t.Fatalf("rows = %d, want only the first", len(register.rows))
	}
	for _, outcome := range []application.PreAdviceWindowRegistrationOutcome{
		application.PreAdviceWindowRegistered, application.PreAdviceWindowExisting, application.PreAdviceWindowOverlapping,
	} {
		if outcome.String() == "" {
			t.Fatalf("outcome %d has no wire name", outcome)
		}
	}
}

// Covers: 租户取值不推定——形不成一行的命令（宽限量为负、终点不晚于起点、缺依据）答被拒，册一次也不被问到；册读写不动原样交回，
// 不形成答案。
func TestARegistrationThatFormsNoWindowNeverReachesTheRegister(t *testing.T) {
	register := &windowRegisterDouble{}
	handler := windowHandler(t, register)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*application.RegisterPreAdviceWindowCommand){
		"宽限量为负": func(command *application.RegisterPreAdviceWindowCommand) { command.Grace = -time.Hour },
		"终点不晚于起点": func(command *application.RegisterPreAdviceWindowCommand) {
			command.EffectiveUntil = command.EffectiveFrom
		},
		"缺依据": func(command *application.RegisterPreAdviceWindowCommand) { command.Basis = "" },
	} {
		command := windowCommand(t, jan, time.Hour)
		mutate(&command)
		if _, err := handler.Handle(context.Background(), command); !errors.Is(err, application.ErrPreAdviceWindowRejected) {
			t.Fatalf("%s: err = %v, want ErrPreAdviceWindowRejected", name, err)
		}
	}
	if register.asked != 0 {
		t.Fatalf("register asked %d times", register.asked)
	}

	failing := &windowRegisterDouble{err: errors.New("register unreachable")}
	if _, err := windowHandler(t, failing).Handle(context.Background(), windowCommand(t, jan, time.Hour)); err == nil ||
		errors.Is(err, application.ErrPreAdviceWindowRejected) {
		t.Fatalf("unreachable register: err = %v, want a dependency failure", err)
	}
	if _, err := application.NewRegisterPreAdviceWindowHandler(application.RegisterPreAdviceWindowDeps{Clock: fixedClock{}}); err == nil {
		t.Fatal("缺册的编排构造得出来")
	}
}
