package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证账期分支的授信额度来源（ADR-0127 决定四与五）：额度出自闭包交出的信用政策版本而不是
// 登记状况；授信依据那一口的三格各是一格待判断；任一依赖为 nil 在构造期拒（contract 段）。

// unconfigured 取反向默认，理由同 policyDouble：零值继续表示「闭包采用了信用政策」。
type creditBasisDouble struct {
	basis        domain.CreditBasis
	unconfigured bool
	err          error
	asked        int
	askedWith    domain.CommercialResolutionReference
}

func (double *creditBasisDouble) LoadCreditBasis(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
	resolution domain.CommercialResolutionReference,
) (domain.CreditBasis, bool, error) {
	double.asked++
	double.askedWith = resolution
	if double.err != nil {
		return domain.CreditBasis{}, false, double.err
	}
	if double.unconfigured {
		return domain.CreditBasis{}, false, nil
	}
	return double.basis, true, nil
}

func amountBasis(t *testing.T, policy string, amountMinor int64) domain.CreditBasis {
	t.Helper()
	basis, err := domain.NewCreditAmountBasis(value(t, domain.NewCreditPolicyReference, policy), amountMinor)
	if err != nil {
		t.Fatalf("new credit amount basis: %v", err)
	}
	return basis
}

func newCreditBasisHandler(
	t *testing.T,
	basis ports.CreditBasisView,
	credit ports.CreditStandingView,
	exposures ports.CreditExposureLedgerRepository,
) *application.ApplyPreAcceptanceControlHandler {
	t.Helper()
	return mustHandler(t, application.ApplyPreAcceptanceControlDeps{
		Policy:      &policyDouble{policy: methodPolicy(t, domain.TermsSettlement)},
		Balance:     &balanceDouble{},
		Freezes:     &ledgerDouble{ledger: domain.NewFreezeLedger()},
		Credit:      credit,
		CreditBasis: basis,
		Exposures:   exposures,
		Clock:       fixedClock{at: controlAt},
	})
}

// Covers: ADR-0127 决定四 / UC-SA-002 步 7 账期分支 / AT-SA-171「B 只形成适用信用暴露 / 限制结果……
// 分别保存政策和范围」——额度出自闭包交出的信用政策版本，不出自登记状况：登记状况只给 1_000 额度，
// 政策授权 10_000，4_000 的暴露照政策记录而不是受限；结果带回额度出自的信用政策版本；回指原样透传
// 到授信依据视图；已占用与逾期仍取本上下文自己的账。
func TestTheAuthorizedLimitComesFromTheCreditBasisNotTheRegisteredStanding(t *testing.T) {
	basis := &creditBasisDouble{basis: amountBasis(t, "credit-1/v1", 10_000)}
	credit := &creditDouble{standing: standingWith(t, 1_000, 0, false)}
	exposures := &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()}
	handler := newCreditBasisHandler(t, basis, credit, exposures)

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ControlApplied {
		t.Fatalf("outcome = %q/%q, want CONTROL_APPLIED", result.Outcome(), result.NotFormedReason())
	}
	exposure, present := result.Exposure()
	if !present || exposure.Status() != domain.ExposureRecorded || exposure.AmountMinor() != 4_000 {
		t.Fatalf("exposure = %#v, want a RECORDED 4000——额度该按政策授权的 10000 判，不按登记的 1000", exposure)
	}
	if result.CreditPolicy().String() != "credit-1/v1" {
		t.Fatalf("credit policy = %q, want credit-1/v1——暴露结果必须带回额度出自的政策版本", result.CreditPolicy())
	}
	if exposure.Policy().String() != "credit-1/v1" {
		t.Fatalf("exposure policy = %q, want credit-1/v1——入册的暴露自己要带出处，账本落库靠的是它不是结果",
			exposure.Policy())
	}
	if basis.askedWith.String() != "RES-1" {
		t.Fatalf("授信依据视图被问到的回指 = %q, want RES-1", basis.askedWith)
	}
	if credit.loaded != 1 || exposures.saved != 1 {
		t.Fatalf("standing loaded %d / ledger saved %d, want 1 / 1——已占用与逾期仍取本上下文自己的账",
			credit.loaded, exposures.saved)
	}
}

// Covers: 已占用暴露仍从本上下文的账本来——政策授 10_000、账本已占 8_000，4_000 的请求受限。只换额度
// 不换已占用，两半各归其主。
func TestTheExposedAmountStaysWithThisContextsLedger(t *testing.T) {
	handler := newCreditBasisHandler(t,
		&creditBasisDouble{basis: amountBasis(t, "credit-1/v1", 10_000)},
		&creditDouble{standing: standingWith(t, 100_000, 8_000, false)},
		&exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()})

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	exposure, present := result.Exposure()
	if !present || exposure.Status() != domain.ExposureRestricted ||
		exposure.Reason().String() != "AVAILABLE_CREDIT_INSUFFICIENT" {
		t.Fatalf("exposure = %#v, want RESTRICTED / AVAILABLE_CREDIT_INSUFFICIENT（10000 − 8000 < 4000）", exposure)
	}
	if result.CreditPolicy().String() != "credit-1/v1" {
		t.Fatalf("credit policy = %q；受限结果同样要带额度出处", result.CreditPolicy())
	}
}

// Covers: 授信依据那一口的三格（ADR-0127 决定四，照 ADR-0054）：未配置、调不通、比例基数未裁各是
// 一格`待判断`，恢复动作各不相同，不得读成「有额度」或「零额度」；且在这三格里都不去读这个客户的
// 信用状况——顺序与「策略先于余额」同一条。
func TestCreditBasisGradesEachHaltBeforeReadingTheStanding(t *testing.T) {
	ratio, err := domain.NewCreditRatioBasis(value(t, domain.NewCreditPolicyReference, "credit-r/v1"), 2_500)
	if err != nil {
		t.Fatalf("new credit ratio basis: %v", err)
	}
	cases := map[string]struct {
		view   *creditBasisDouble
		reason application.NotFormedReason
	}{
		"not configured":       {view: &creditBasisDouble{unconfigured: true}, reason: application.CreditBasisNotConfigured},
		"unavailable":          {view: &creditBasisDouble{err: errors.New("closure store down")}, reason: application.CreditBasisUnavailable},
		"ratio base undecided": {view: &creditBasisDouble{basis: ratio}, reason: application.CreditRatioBaseUndecided},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			credit := &creditDouble{standing: standingWith(t, 100_000, 0, false)}
			exposures := &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()}
			handler := newCreditBasisHandler(t, testCase.view, credit, exposures)

			result, err := handler.Handle(context.Background(), command(t, 4_000))
			if err != nil {
				t.Fatalf("授信依据那一格被当成技术错误抛出: %v", err)
			}
			if result.Outcome() != application.ControlNotFormed || result.NotFormedReason() != testCase.reason {
				t.Fatalf("outcome/reason = %q/%q, want CONTROL_NOT_FORMED/%q",
					result.Outcome(), result.NotFormedReason(), testCase.reason)
			}
			if _, exposed := result.Exposure(); exposed {
				t.Fatal("没有额度依据却形成了暴露")
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("待判断没有留下可续办引用")
			}
			if credit.loaded != 0 || exposures.saved != 0 {
				t.Fatalf("standing loaded %d / ledger saved %d, want 0 / 0——依据没到手就不该碰这个客户的信用状况",
					credit.loaded, exposures.saved)
			}
		})
	}
	distinct := map[application.NotFormedReason]struct{}{}
	for _, reason := range []application.NotFormedReason{
		application.CreditBasisUnavailable, application.CreditBasisNotConfigured,
		application.CreditRatioBaseUndecided, application.CreditStandingUnavailable,
	} {
		if reason.String() == "" {
			t.Fatalf("reason %d has no name", reason)
		}
		distinct[reason] = struct{}{}
	}
	if len(distinct) != 4 {
		t.Fatal("授信依据的三格与信用状况调不通共用了原因，运维读不出该催登记、该重试还是该等裁决")
	}
}

// Covers: ADR-0127 决定五的 contract 段——`Deps.CreditBasis` 与其它依赖一样 mandatory，nil 在构造期拒，
// 「沿旧路、额度取登记状况」那一格不再存在。expand 段留它只因 parcel-shipment 那份夹具直接构造这份
// Deps；夹具补上替身之后，nil 只可能是装配疏漏，而装配疏漏要在启动时炸出来，不能等第一笔账期委托到达
// 时静默拿登记额度当政策额度。七件依赖同一道门、同一个哨兵：缺哪一件都不该造出一个会在运行期 panic
// 的编排。
func TestAnyNilDependencyIsRefusedAtConstruction(t *testing.T) {
	complete := func() application.ApplyPreAcceptanceControlDeps {
		return application.ApplyPreAcceptanceControlDeps{
			Policy:      &policyDouble{policy: methodPolicy(t, domain.TermsSettlement)},
			Balance:     &balanceDouble{},
			Freezes:     &ledgerDouble{ledger: domain.NewFreezeLedger()},
			Credit:      &creditDouble{standing: standingWith(t, 10_000, 0, false)},
			CreditBasis: &creditBasisDouble{basis: amountBasis(t, "credit-1/v1", 10_000)},
			Exposures:   &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()},
			Clock:       fixedClock{at: controlAt},
		}
	}
	strip := map[string]func(*application.ApplyPreAcceptanceControlDeps){
		"policy":       func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Policy = nil },
		"balance":      func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Balance = nil },
		"freezes":      func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Freezes = nil },
		"credit":       func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Credit = nil },
		"credit basis": func(deps *application.ApplyPreAcceptanceControlDeps) { deps.CreditBasis = nil },
		"exposures":    func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Exposures = nil },
		"clock":        func(deps *application.ApplyPreAcceptanceControlDeps) { deps.Clock = nil },
	}
	for name, missing := range strip {
		t.Run(name, func(t *testing.T) {
			deps := complete()
			missing(&deps)
			handler, err := application.NewApplyPreAcceptanceControlHandler(deps)
			if !errors.Is(err, application.ErrNilDependency) {
				t.Fatalf("err = %v, want ErrNilDependency——缺 %s 造出了一个会在运行期 panic 的编排", err, name)
			}
			if handler != nil {
				t.Fatal("拒了还交回了编排")
			}
		})
	}

	if _, err := application.NewApplyPreAcceptanceControlHandler(complete()); err != nil {
		t.Fatalf("七件齐全仍被拒：%v", err)
	}
}
