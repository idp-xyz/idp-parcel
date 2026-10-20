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

// 本文件证预报有效时窗的收口编排（票 ps-external-mark-relations/14 裁定）：收口落下答已收口、收口时刻取命令、登记时刻取时钟；原样
// 重复收口答已有结果并回显第一份；收口时刻不晚于起点、对已有终点的行收口、换内容再收、本租户没有那一行都答被拒；册读写不动原样
// 交回。收口之后，同一份登记编排登得进起点不早于收口时刻的后继。夹具全为合成（S 级）。

// ClosePreAdviceWindow 照真库的代数答：本租户没有那一行答查无此行；那一行登记时带终点或已收过口，什么也不落、回显那一行；否则
// 挂上收口、回显收口之后的那一行。
func (register *windowRegisterDouble) ClosePreAdviceWindow(_ context.Context, closure domain.PreAdviceWindowClosure) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	register.asked++
	if register.err != nil {
		return domain.PreAdviceValidityWindow{}, ports.DeliveryScopeAppendInvalid, register.err
	}
	for index, row := range register.rows {
		if row.TenantID() != closure.TenantID() || !row.EffectiveFrom().Equal(closure.WindowFrom()) {
			continue
		}
		closed, err := row.Close(closure)
		if errors.Is(err, domain.ErrPreAdviceWindowNotOpen) {
			return row, ports.DeliveryScopeAlreadyRecorded, nil
		}
		if err != nil {
			return domain.PreAdviceValidityWindow{}, ports.DeliveryScopeAppendInvalid, err
		}
		register.rows[index] = closed
		return closed, ports.DeliveryScopeAppended, nil
	}
	return domain.PreAdviceValidityWindow{}, ports.DeliveryScopeTargetNotFound, nil
}

var closureRegisteredAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)

func closureCommand(t *testing.T, windowFrom, closedAt time.Time) application.ClosePreAdviceWindowCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return application.ClosePreAdviceWindowCommand{
		TenantID:   tenant,
		WindowFrom: windowFrom,
		ClosedAt:   closedAt,
		Basis:      "SYN-BASIS-PSXM14-CLOSURE",
	}
}

func closureHandler(t *testing.T, register ports.PreAdviceWindowRegister) *application.ClosePreAdviceWindowHandler {
	t.Helper()
	handler, err := application.NewClosePreAdviceWindowHandler(application.ClosePreAdviceWindowDeps{
		Windows: register,
		Clock:   fixedClock{at: closureRegisteredAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: 裁定「对当前终点为空的那一行追加一条收口……收口之后，起点不早于收口时刻的新行照常登记」——收口前后继答重叠；收口答已
// 收口，回显带着收口（收口时刻取命令、登记时刻取时钟）；原样重复收口答已有结果、回显第一份；收口之后同一份登记编排登得进后继。
func TestClosingAWindowAnswersClosedOrExisting(t *testing.T) {
	register := &windowRegisterDouble{}
	ctx := context.Background()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	registration := windowHandler(t, register)
	if result, err := registration.Handle(ctx, windowCommand(t, jan, 6*time.Hour)); err != nil || result.Outcome() != application.PreAdviceWindowRegistered {
		t.Fatalf("open = %s %v", result.Outcome(), err)
	}
	if result, err := registration.Handle(ctx, windowCommand(t, jul, 8*time.Hour)); err != nil || result.Outcome() != application.PreAdviceWindowOverlapping {
		t.Fatalf("收口之前的后继 = %s %v", result.Outcome(), err)
	}

	closing := closureHandler(t, register)
	closed, err := closing.Handle(ctx, closureCommand(t, jan, jul))
	if err != nil || closed.Outcome() != application.PreAdviceWindowClosed {
		t.Fatalf("close = %s %v", closed.Outcome(), err)
	}
	closure, isClosed := closed.Window().Closure()
	if !isClosed || !closure.ClosedAt().Equal(jul) || !closure.RegisteredAt().Equal(closureRegisteredAt) || closure.Basis() != "SYN-BASIS-PSXM14-CLOSURE" {
		t.Fatalf("closure echo = %+v %v", closure, isClosed)
	}

	replay, err := closing.Handle(ctx, closureCommand(t, jan, jul))
	if err != nil || replay.Outcome() != application.PreAdviceWindowClosureExisting {
		t.Fatalf("replay = %s %v", replay.Outcome(), err)
	}
	if echoed, _ := replay.Window().Closure(); !echoed.RegisteredAt().Equal(closureRegisteredAt) {
		t.Fatalf("replay echo = %+v, want the first closure", echoed)
	}

	if result, err := registration.Handle(ctx, windowCommand(t, jul, 8*time.Hour)); err != nil || result.Outcome() != application.PreAdviceWindowRegistered {
		t.Fatalf("收口之后的后继 = %s %v", result.Outcome(), err)
	}
	for _, outcome := range []application.PreAdviceWindowClosureOutcome{application.PreAdviceWindowClosed, application.PreAdviceWindowClosureExisting} {
		if outcome.String() == "" {
			t.Fatalf("outcome %d has no wire name", outcome)
		}
	}
}

// Covers: 裁定「收口时刻早于前一行起点、对已有终点的行收口、重复收口都答坏输入或已有结果」——收口时刻不晚于起点、缺依据形不成
// 收口，册一次也不被问到；登记时带终点的行、已按别的时刻收过口的行、本租户没有的那一行都答被拒，册上不多一份收口；册读写不动
// 原样交回，不形成答案。
func TestAClosureThatCannotEndItsRowIsRejected(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	register := &windowRegisterDouble{}
	closing := closureHandler(t, register)
	for name, command := range map[string]application.ClosePreAdviceWindowCommand{
		"收口时刻早于起点": closureCommand(t, jan, jan.Add(-time.Minute)),
		"收口时刻恰在起点": closureCommand(t, jan, jan),
		"缺依据": func() application.ClosePreAdviceWindowCommand {
			command := closureCommand(t, jan, jul)
			command.Basis = " "
			return command
		}(),
	} {
		if _, err := closing.Handle(context.Background(), command); !errors.Is(err, application.ErrPreAdviceWindowClosureRejected) {
			t.Fatalf("%s: err = %v, want ErrPreAdviceWindowClosureRejected", name, err)
		}
	}
	if register.asked != 0 {
		t.Fatalf("register asked %d times", register.asked)
	}

	registration := windowHandler(t, register)
	bounded := windowCommand(t, jan, 6*time.Hour)
	bounded.EffectiveUntil = jul
	if _, err := registration.Handle(context.Background(), bounded); err != nil {
		t.Fatal(err)
	}
	if _, err := registration.Handle(context.Background(), windowCommand(t, jul, 8*time.Hour)); err != nil {
		t.Fatal(err)
	}
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if result, err := closing.Handle(context.Background(), closureCommand(t, jul, dec)); err != nil || result.Outcome() != application.PreAdviceWindowClosed {
		t.Fatalf("close = %s %v", result.Outcome(), err)
	}
	for name, command := range map[string]application.ClosePreAdviceWindowCommand{
		"登记时带终点的行":  closureCommand(t, jan, jan.AddDate(0, 3, 0)),
		"已按别的时刻收过口": closureCommand(t, jul, dec.AddDate(0, 1, 0)),
		"本租户没有那一行":  closureCommand(t, jul.Add(time.Hour), dec),
	} {
		if _, err := closing.Handle(context.Background(), command); !errors.Is(err, application.ErrPreAdviceWindowClosureRejected) {
			t.Fatalf("%s: err = %v, want ErrPreAdviceWindowClosureRejected", name, err)
		}
	}
	if closure, _ := register.rows[1].Closure(); !closure.ClosedAt().Equal(dec) {
		t.Fatalf("换时刻再收改到了第一份收口: %v", closure.ClosedAt())
	}

	failing := &windowRegisterDouble{err: errors.New("register unreachable")}
	if _, err := closureHandler(t, failing).Handle(context.Background(), closureCommand(t, jan, jul)); err == nil ||
		errors.Is(err, application.ErrPreAdviceWindowClosureRejected) {
		t.Fatalf("unreachable register: err = %v, want a dependency failure", err)
	}
	if _, err := application.NewClosePreAdviceWindowHandler(application.ClosePreAdviceWindowDeps{Clock: fixedClock{}}); err == nil {
		t.Fatal("缺册的编排构造得出来")
	}
}
