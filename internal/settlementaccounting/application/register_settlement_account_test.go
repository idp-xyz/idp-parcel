package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉结算账户登记编排交信封的判据（票 routing-first-cut/19）：按落点交，不按「这次调用有没有成功」交。
// 真库那一层（cmd/parcel-settlement-register）钉不住「冲突不交」——冲突那一步若用同一户，信封 ID 与首登同一
// 指纹，交了也被 EnqueueOnce 吞掉，看不出来；替身数调用次数才看得出。

type accountRegisterDouble struct {
	effect ports.SettlementAccountRegistrationEffect
	err    error
}

func (double accountRegisterDouble) Save(
	context.Context,
	domain.TenantID,
	domain.SettlementAccount,
	time.Time,
) (ports.SettlementAccountRegistrationEffect, error) {
	return double.effect, double.err
}

type accountHandoffDouble struct {
	intents []ports.SettlementAccountRegisteredIntent
	err     error
}

func (double *accountHandoffDouble) HandOffSettlementAccountRegistered(
	_ context.Context,
	intent ports.SettlementAccountRegisteredIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

type accountClock struct{ at time.Time }

func (clock accountClock) Now() time.Time { return clock.at }

var accountRegisteredAt = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)

func mustAccountValue[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func accountCommand(t *testing.T) application.RegisterSettlementAccountCommand {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(
		mustAccountValue(t, domain.NewLegalEntityReference, "LE-1"),
		mustAccountValue(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		domain.ChargeReceivable,
		mustAccountValue(t, domain.NewCurrencyCode, "XTS"),
		mustAccountValue(t, domain.NewSettlementPolicyReference, "POL-1"),
	)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	account, err := domain.NewSettlementAccount(
		mustAccountValue(t, domain.NewSettlementAccountID, "ACCT-1"),
		key,
		domain.SettlementCounterpartyReference{},
		false,
		mustAccountValue(t, domain.NewResponsibilityBasis, "CONTRACT-1"),
	)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	return application.RegisterSettlementAccountCommand{
		Tenant:  mustAccountValue(t, domain.NewTenantID, "tenant-a"),
		Account: account,
	}
}

func newAccountHandler(
	t *testing.T,
	register accountRegisterDouble,
	handoff *accountHandoffDouble,
) *application.RegisterSettlementAccountHandler {
	t.Helper()
	handler, err := application.NewRegisterSettlementAccountHandler(register, handoff, accountClock{at: accountRegisteredAt})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

// Covers: `已登记`与`已存在`各交恰好一份意图，带租户、账户与登记时刻；`冲突`一份都不交——冲突那一行没写进册，
// 交出去下游只会再撞见同一个缺口。
func TestARegistrationHandsOffByItsEffectAndAConflictHandsOffNothing(t *testing.T) {
	cases := map[string]struct {
		effect    ports.SettlementAccountRegistrationEffect
		wantHands int
	}{
		"已登记": {ports.SettlementAccountRegistered, 1},
		"已存在": {ports.SettlementAccountReplay, 1},
		"冲突":  {ports.SettlementAccountConflict, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handoff := &accountHandoffDouble{}
			handler := newAccountHandler(t, accountRegisterDouble{effect: tc.effect}, handoff)

			effect, err := handler.Register(t.Context(), accountCommand(t))
			if err != nil {
				t.Fatalf("register: %v", err)
			}
			if effect != tc.effect {
				t.Fatalf("effect = %d, want %d", effect, tc.effect)
			}
			if len(handoff.intents) != tc.wantHands {
				t.Fatalf("交接口被调 %d 次，want %d", len(handoff.intents), tc.wantHands)
			}
			if tc.wantHands == 0 {
				return
			}
			intent := handoff.intents[0]
			if intent.Tenant.String() != "tenant-a" || intent.Account.String() != "ACCT-1" ||
				!intent.RegisteredAt.Equal(accountRegisteredAt) {
				t.Fatalf("意图 = %+v，want tenant-a / ACCT-1 / 登记时刻", intent)
			}
		})
	}
}

// Covers: 交接失败整项出错——信封与登记行同生共死，事务壳据此回滚；登记册的失败同样上抛且不交信封。
func TestAFailedHandoffOrSaveFailsTheRegistration(t *testing.T) {
	refused := errors.New("outbox unavailable")
	handoff := &accountHandoffDouble{err: refused}
	handler := newAccountHandler(t, accountRegisterDouble{effect: ports.SettlementAccountRegistered}, handoff)
	if _, err := handler.Register(t.Context(), accountCommand(t)); !errors.Is(err, refused) {
		t.Fatalf("交接失败应上抛，实得：%v", err)
	}

	broken := errors.New("register unavailable")
	untouched := &accountHandoffDouble{}
	failing := newAccountHandler(t, accountRegisterDouble{err: broken}, untouched)
	if _, err := failing.Register(t.Context(), accountCommand(t)); !errors.Is(err, broken) {
		t.Fatalf("登记册失败应上抛，实得：%v", err)
	}
	if len(untouched.intents) != 0 {
		t.Fatalf("登记册失败仍交了 %d 份意图", len(untouched.intents))
	}
}

// Covers: 交接口不许为 nil——缺了它登记照样成功，停在作用域那一格的委托却再也等不来重驱。
func TestARegistrationHandlerRefusesAMissingHandoff(t *testing.T) {
	if _, err := application.NewRegisterSettlementAccountHandler(
		accountRegisterDouble{}, nil, accountClock{at: accountRegisteredAt}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺交接口应在构造期被拒，实得：%v", err)
	}
}
