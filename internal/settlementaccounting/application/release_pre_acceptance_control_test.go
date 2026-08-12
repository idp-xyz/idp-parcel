package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var releasedAtClock = time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)

func releaseScope(t *testing.T) domain.SettlementScope {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		value(t, domain.NewLegalEntityReference, freezeScope.legalEntity),
		value(t, domain.NewSettlementAccountID, freezeScope.account),
		value(t, domain.NewCurrencyCode, freezeScope.currency),
	)
	if err != nil {
		t.Fatalf("new settlement scope: %v", err)
	}
	return scope
}

// heldLedger 造一本已经真经领域 Freeze 过一笔的账本。假状态挡不住 Release 的查找与幂等，
// 也证明不了编排走对了路。
func heldLedger(t *testing.T) (*domain.FreezeLedger, domain.FundsFreeze) {
	t.Helper()
	scope := releaseScope(t)
	balance, err := domain.NewOperationalBalance(scope, 10_000, 0, 0, 0)
	if err != nil {
		t.Fatalf("new operational balance: %v", err)
	}
	request, err := domain.NewFreezeRequest(
		value(t, domain.NewControlRequestID, "ctrl-req-1"),
		scope,
		4_000,
		value(t, domain.NewBusinessAssociationReference, "request-1/version-1"),
		controlAt,
	)
	if err != nil {
		t.Fatalf("new freeze request: %v", err)
	}
	ledger := domain.NewFreezeLedger()
	freeze, err := ledger.Freeze(request, balance)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if freeze.Status() != domain.FreezeHeld {
		t.Fatalf("fixture freeze status = %q, want HELD", freeze.Status())
	}
	return ledger, freeze
}

func releaseCommand(t *testing.T) application.ReleasePreAcceptanceControlCommand {
	t.Helper()
	return application.ReleasePreAcceptanceControlCommand{
		TenantID:  value(t, domain.NewTenantID, "tenant-1"),
		RequestID: value(t, domain.NewControlRequestID, "ctrl-req-1"),
		Scope:     releaseScope(t),
	}
}

// Covers: UC-SA-002 `AT-SA-046`「接受失败且冻结已形成 → 按原关联显式释放，留审计」的释放
// 编排半边，与 CONTEXT「已经形成的接受前资金冻结必须通过原业务关联请求显式释放」——调用方
// 手里只有当初的控制请求身份，FreezeID 是本账本签发的内部编号，不随控制结果离开本上下文
// 重建，所以释放按请求身份认领。原金额与冻结时间留在记录上（审计半边由领域测试承重）。
func TestAHeldFreezeIsReleasedByItsOriginalAssociation(t *testing.T) {
	ledger, frozen := heldLedger(t)
	repo := &ledgerDouble{ledger: ledger}
	handler := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock})

	result, err := handler.Handle(context.Background(), releaseCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlReleased {
		t.Fatalf("outcome = %q, want CONTROL_RELEASED", result.Outcome())
	}
	released, present := result.Freeze()
	if !present || released.Status() != domain.FreezeReleased {
		t.Fatalf("freeze = %#v present = %v, want RELEASED", released, present)
	}
	if released.FreezeID() != frozen.FreezeID() {
		t.Fatal("释放的不是原来那一笔冻结")
	}
	if !released.ReleasedAt().Equal(releasedAtClock) {
		t.Fatalf("released at = %s, want the clock reading %s", released.ReleasedAt(), releasedAtClock)
	}
	if repo.saved != 1 {
		t.Fatalf("saved %d times; 释放没落库就不算释放", repo.saved)
	}
}

// Covers: UC-SA-002 `AT-SA-045` 幂等在释放一侧——重复释放返回与首次相同的答案，包括原释放
// 时间：响应丢失后的重试不得看起来像发生了第二次释放（领域已守，此处钉编排不换答案）。
func TestARepeatedReleaseReturnsTheOriginalAnswerWithItsOriginalTime(t *testing.T) {
	ledger, _ := heldLedger(t)
	repo := &ledgerDouble{ledger: ledger}

	first, err := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock}).
		Handle(context.Background(), releaseCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	second, err := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock.Add(time.Hour)}).
		Handle(context.Background(), releaseCommand(t))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	firstFreeze, _ := first.Freeze()
	secondFreeze, present := second.Freeze()
	if second.Outcome() != application.ControlReleased || !present {
		t.Fatalf("second outcome = %q present = %v", second.Outcome(), present)
	}
	if !secondFreeze.ReleasedAt().Equal(firstFreeze.ReleasedAt()) {
		t.Fatalf("repeat released at = %s, want the original %s——重试看起来像第二次释放",
			secondFreeze.ReleasedAt(), firstFreeze.ReleasedAt())
	}
}

// Covers: UC-PS-005 `AT-PS-074` 的 SA 半边「未形成过资金冻结 → 保存财务补偿不适用依据」——
// 这个关联下从未占用过资金（冻结从未形成，或只形成过`业务限制`而限制不入账本），释放交回
// `无可释放`这个业务答案，对账不必去追一笔不存在的释放。它不是错误也不是未决：重试一万次
// 也不会长出一笔冻结来。
func TestAReleaseForARequestThatNeverFrozeIsNothingToRelease(t *testing.T) {
	repo := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock})

	result, err := handler.Handle(context.Background(), releaseCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NothingToRelease {
		t.Fatalf("outcome = %q, want NOTHING_TO_RELEASE", result.Outcome())
	}
	if _, present := result.Freeze(); present {
		t.Fatal("没有冻结却交回了一笔")
	}
	if repo.saved != 0 {
		t.Fatal("无可释放还写了账本")
	}
}

// Covers: UC-SA-002 结果语义——依赖读不回是待判断不是业务答案；释放侧同一条：读不回与
// 存不回都停在未形成，带封闭原因与续办引用，绝不冒充`已释放`或`无可释放`。
func TestAnUnreachableLedgerKeepsTheReleaseUnformed(t *testing.T) {
	cases := map[string]func(*ledgerDouble){
		"load fails": func(repo *ledgerDouble) { repo.loadErr = errors.New("ledger unavailable") },
		"save fails": func(repo *ledgerDouble) { repo.saveErr = errors.New("ledger unavailable") },
	}

	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			ledger, _ := heldLedger(t)
			repo := &ledgerDouble{ledger: ledger}
			arrange(repo)
			handler := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock})

			result, err := handler.Handle(context.Background(), releaseCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.ReleaseNotFormed {
				t.Fatalf("outcome = %q, want RELEASE_NOT_FORMED", result.Outcome())
			}
			if result.NotFormedReason() != application.FreezeLedgerUnavailable {
				t.Fatalf("reason = %q, want FREEZE_LEDGER_UNAVAILABLE", result.NotFormedReason())
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("未形成的释放无法安全续办")
			}
		})
	}
}

// Covers: UC-SA-002 步骤 2 同一条受理闸——最小身份不成立时不读任何权威，一次已经发出的
// 读取本身就回答了这个租户、这个作用域存不存在。
func TestAnIncompleteReleaseRequestIsNotAccepted(t *testing.T) {
	ledger, _ := heldLedger(t)
	repo := &ledgerDouble{ledger: ledger}
	handler := application.NewReleasePreAcceptanceControlHandler(repo, fixedClock{at: releasedAtClock})

	command := releaseCommand(t)
	command.RequestID = domain.ControlRequestID{}

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReleaseRequestNotAccepted {
		t.Fatalf("outcome = %q, want RELEASE_REQUEST_NOT_ACCEPTED", result.Outcome())
	}
	if repo.loaded != 0 {
		t.Fatal("身份不成立却读了账本")
	}
}
