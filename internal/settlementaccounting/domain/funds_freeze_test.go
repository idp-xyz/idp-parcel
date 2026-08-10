package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var frozenAt = time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

func settlementScope(t *testing.T, legalEntity, account, currency string) domain.SettlementScope {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		settlementValue(t, domain.NewLegalEntityReference, legalEntity),
		settlementValue(t, domain.NewSettlementAccountID, account),
		settlementValue(t, domain.NewCurrencyCode, currency),
	)
	if err != nil {
		t.Fatalf("new settlement scope: %v", err)
	}
	return scope
}

func balance(t *testing.T, posted, creditLimit, frozen, unsettled int64) domain.OperationalBalance {
	t.Helper()
	built, err := domain.NewOperationalBalance(
		settlementScope(t, "legal-1", "account-1", "SYN"),
		posted, creditLimit, frozen, unsettled,
	)
	if err != nil {
		t.Fatalf("new operational balance: %v", err)
	}
	return built
}

func freezeRequest(t *testing.T, requestID string, amount int64) domain.FreezeRequest {
	t.Helper()
	request, err := domain.NewFreezeRequest(
		settlementValue(t, domain.NewControlRequestID, requestID),
		settlementScope(t, "legal-1", "account-1", "SYN"),
		amount,
		settlementValue(t, domain.NewBusinessAssociationReference, "association-1"),
		frozenAt,
	)
	if err != nil {
		t.Fatalf("new freeze request: %v", err)
	}
	return request
}

// Covers: settlement-accounting CONTEXT 运营结算余额 — 可用余额等于入账余额加当前有效
// 授信额度，再扣除冻结金额与已确认未结应收。
func TestAvailableBalanceFollowsTheDeclaredFormula(t *testing.T) {
	value := balance(t, 1000, 500, 200, 100)
	if got, want := value.Available(), int64(1200); got != want {
		t.Fatalf("available = %d, want %d", got, want)
	}
}

// Covers: CONTEXT「不同责任法人、结算账户和币种默认不能共用余额」— 作用域不同即不同余额，
// 冻结不得跨作用域取用。
func TestFreezeRefusesABalanceFromAnotherScope(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	other := balance(t, 100000, 0, 0, 0)

	differing := map[string]domain.SettlementScope{
		"other legal entity": settlementScope(t, "legal-2", "account-1", "SYN"),
		"other account":      settlementScope(t, "legal-1", "account-2", "SYN"),
		"other currency":     settlementScope(t, "legal-1", "account-1", "SYN2"),
	}
	for name, scope := range differing {
		t.Run(name, func(t *testing.T) {
			request, err := domain.NewFreezeRequest(
				settlementValue(t, domain.NewControlRequestID, "request-x"),
				scope,
				100,
				settlementValue(t, domain.NewBusinessAssociationReference, "association-1"),
				frozenAt,
			)
			if err != nil {
				t.Fatalf("new freeze request: %v", err)
			}
			if _, err := ledger.Freeze(request, other); !errors.Is(err, domain.ErrSettlementScopeMismatch) {
				t.Fatalf("error = %v, want ErrSettlementScopeMismatch", err)
			}
		})
	}
}

// Covers: CONTEXT 可用余额 → 已冻结 — 通过余额校验才占用；余额不足只形成业务限制结果，
// 不是拒绝，也不是错误。
func TestFreezeSucceedsWithinAvailableBalanceAndRestrictsBeyondIt(t *testing.T) {
	t.Run("within available balance", func(t *testing.T) {
		ledger := domain.NewFreezeLedger()
		result, err := ledger.Freeze(freezeRequest(t, "request-1", 1200), balance(t, 1000, 500, 200, 100))
		if err != nil {
			t.Fatalf("freeze: %v", err)
		}
		if result.Status() != domain.FreezeHeld {
			t.Fatalf("status = %q, want HELD", result.Status())
		}
		if result.AmountMinor() != 1200 {
			t.Fatalf("amount = %d, want 1200", result.AmountMinor())
		}
	})

	t.Run("beyond available balance is a business restriction", func(t *testing.T) {
		ledger := domain.NewFreezeLedger()
		result, err := ledger.Freeze(freezeRequest(t, "request-2", 1201), balance(t, 1000, 500, 200, 100))
		if err != nil {
			t.Fatalf("insufficient balance surfaced as an error rather than a restriction: %v", err)
		}
		if result.Status() != domain.FreezeRestricted {
			t.Fatalf("status = %q, want RESTRICTED", result.Status())
		}
		if result.Reason().String() == "" {
			t.Fatal("a restriction carries no reason")
		}
		if ledger.HeldCount() != 0 {
			t.Fatal("a restricted request still occupied funds")
		}
	})
}

// Covers: CONTEXT「重复请求、查询和补偿不得形成重复冻结或重复释放」。
func TestRepeatedFreezeAndReleaseAreIdempotent(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	available := balance(t, 10000, 0, 0, 0)
	request := freezeRequest(t, "request-1", 1200)

	first, err := ledger.Freeze(request, available)
	if err != nil {
		t.Fatalf("first freeze: %v", err)
	}
	replay, err := ledger.Freeze(request, available)
	if err != nil {
		t.Fatalf("replay freeze: %v", err)
	}
	if replay.FreezeID() != first.FreezeID() || ledger.HeldCount() != 1 {
		t.Fatalf("a replay created a second freeze: held = %d", ledger.HeldCount())
	}

	released, err := ledger.Release(first.FreezeID(), frozenAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Status() != domain.FreezeReleased {
		t.Fatalf("status = %q, want RELEASED", released.Status())
	}
	replayReleased, err := ledger.Release(first.FreezeID(), frozenAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("replay release: %v", err)
	}
	if !replayReleased.ReleasedAt().Equal(released.ReleasedAt()) {
		t.Fatal("a replayed release moved the original release time")
	}
	if ledger.ReleaseCount() != 1 {
		t.Fatalf("release count = %d, want 1", ledger.ReleaseCount())
	}
}

// Covers: CONTEXT「释放不删除原冻结历史」。
func TestReleasePreservesTheOriginalFreezeHistory(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	held, err := ledger.Freeze(freezeRequest(t, "request-1", 1200), balance(t, 10000, 0, 0, 0))
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if _, err := ledger.Release(held.FreezeID(), frozenAt.Add(time.Hour)); err != nil {
		t.Fatalf("release: %v", err)
	}

	stored, found := ledger.Lookup(held.FreezeID())
	if !found {
		t.Fatal("release deleted the freeze record")
	}
	if stored.AmountMinor() != held.AmountMinor() || !stored.FrozenAt().Equal(held.FrozenAt()) {
		t.Fatal("release rewrote the original freeze amount or time")
	}
}

// Covers: CONTEXT「同一请求身份携带不同金额或范围」— 冲突不得静默复用原冻结，也不得
// 再冻一次。
func TestConflictingRequestIdentityNeitherReusesNorRefreezes(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	available := balance(t, 10000, 0, 0, 0)
	if _, err := ledger.Freeze(freezeRequest(t, "request-1", 1200), available); err != nil {
		t.Fatalf("first freeze: %v", err)
	}

	changed := freezeRequest(t, "request-1", 1300)
	result, err := ledger.Freeze(changed, available)
	if !errors.Is(err, domain.ErrControlRequestConflict) {
		t.Fatalf("error = %v, want ErrControlRequestConflict", err)
	}
	if result.Status() == domain.FreezeHeld {
		t.Fatal("a conflicting request reported a held freeze")
	}
	if ledger.HeldCount() != 1 {
		t.Fatalf("held = %d; a conflicting request changed the ledger", ledger.HeldCount())
	}
}

// Covers: CONTEXT「不得用一次虚假零金额冻结冒充无控制」— 零或负金额的冻结请求构造不出来。
func TestZeroAmountFreezeCannotStandInForNoControl(t *testing.T) {
	for _, amount := range []int64{0, -1} {
		if _, err := domain.NewFreezeRequest(
			settlementValue(t, domain.NewControlRequestID, "request-x"),
			settlementScope(t, "legal-1", "account-1", "SYN"),
			amount,
			settlementValue(t, domain.NewBusinessAssociationReference, "association-1"),
			frozenAt,
		); !errors.Is(err, domain.ErrInvalidFreezeRequest) {
			t.Fatalf("amount %d constructed a freeze request: err = %v", amount, err)
		}
	}
}

// Covers: CONTEXT「本上下文不得把多个结果汇总成委托接受或拒绝决定」— 冻结结果集合里
// 没有任何表示放行或拒绝的取值。
func TestFreezeStatusSetCarriesNoAcceptanceVerdict(t *testing.T) {
	labels := map[string]struct{}{}
	for _, status := range []domain.FreezeStatus{domain.FreezeHeld, domain.FreezeReleased, domain.FreezeRestricted} {
		label := status.String()
		if label == "" {
			t.Fatalf("status %d has no label", status)
		}
		labels[label] = struct{}{}
	}
	if len(labels) != 3 {
		t.Fatalf("freeze status collapsed into %d labels", len(labels))
	}
	if domain.FreezeStatus(len(labels)+1).String() != "" {
		t.Fatal("a fourth freeze status exists, which invites an acceptance verdict into this context")
	}
}
