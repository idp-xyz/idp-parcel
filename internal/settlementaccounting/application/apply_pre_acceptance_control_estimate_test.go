package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证施加编排的估价那一口（ADR-0179 决定一、二、五、六）：金额按本户的采用行经试算口形成、本次各控制项共用一份、随控制
// 结果留痕；采用册上没有这一户、试算答不出、合计停下与留痕写不进，各停一格，都在碰两本账之前。

const estimatePriceCard = "PRICE_CARD:SYN-PLAN-SELL@v1"

// adoptionViewDouble 替采用册读口：交回 adoption；missing 答册上没有这一户，err 答读不回。
type adoptionViewDouble struct {
	adoption domain.PreAcceptanceEstimateAdoption
	missing  bool
	err      error
	asked    int
	askedFor domain.SettlementAccountID
}

func (double *adoptionViewDouble) FindAdoption(
	_ context.Context,
	_ domain.TenantID,
	account domain.SettlementAccountID,
) (domain.PreAcceptanceEstimateAdoption, bool, error) {
	double.asked++
	double.askedFor = account
	if double.err != nil {
		return domain.PreAcceptanceEstimateAdoption{}, false, double.err
	}
	if double.missing {
		return domain.PreAcceptanceEstimateAdoption{}, false, nil
	}
	return double.adoption, true, nil
}

// adopted 造夹具作用域那一户的采用行：逐件 SELL 试算合计，计价范围 SCOPE-SELL-1。
func adopted(t *testing.T) *adoptionViewDouble {
	t.Helper()
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(
		value(t, domain.NewSettlementAccountID, freezeScope.account),
		domain.PerParcelSellTrialSum,
		value(t, domain.NewEstimatePricingScope, "SCOPE-SELL-1"),
	)
	if err != nil {
		t.Fatalf("new pre-acceptance estimate adoption: %v", err)
	}
	return &adoptionViewDouble{adoption: adoption}
}

// trialSourceDouble 替试算口：交回 answer 或 err，记下被问的次数与最后一次的问法。
type trialSourceDouble struct {
	answer    ports.PreAcceptanceTrialAnswer
	err       error
	asked     int
	askedWith ports.PreAcceptanceTrialQuery
}

func (double *trialSourceDouble) FormPreAcceptanceTrials(
	_ context.Context,
	query ports.PreAcceptanceTrialQuery,
) (ports.PreAcceptanceTrialAnswer, error) {
	double.asked++
	double.askedWith = query
	if double.err != nil {
		return ports.PreAcceptanceTrialAnswer{}, double.err
	}
	return double.answer, nil
}

// estimating 造一只每件一份试算、合计按给定最小币单位、币种取夹具作用域的试算口。控制金额就是这几件之和。
func estimating(t *testing.T, minors ...int64) *trialSourceDouble {
	t.Helper()
	double := &trialSourceDouble{}
	double.set(t, minors...)
	return double
}

// set 换掉这只试算口之后各件的合计：同一请求身份第二次来要的是另一份估价。
func (double *trialSourceDouble) set(t *testing.T, minors ...int64) {
	t.Helper()
	trials := make([]domain.PreAcceptanceTrial, 0, len(minors))
	for index, minor := range minors {
		parcel := "P-" + string(rune('1'+index))
		trial, err := domain.NewPreAcceptanceTrial(parcel, "EST-"+parcel,
			[]string{estimatePriceCard, "REFERENCE_SERIES:SYN-FUEL@v3"}, minor,
			value(t, domain.NewCurrencyCode, freezeScope.currency))
		if err != nil {
			t.Fatalf("new pre-acceptance trial: %v", err)
		}
		trials = append(trials, trial)
	}
	double.answer = ports.PreAcceptanceTrialAnswer{
		Outcome:   ports.PreAcceptanceTrialsFormed,
		PriceCard: estimatePriceCard,
		Trials:    trials,
	}
}

// traceDouble 替留痕写口，判据照真册：同（租户、作用域、请求）首写记下，再写同一份是重放、另一份是冲突，比内容不比落库时刻。
type traceDouble struct {
	recorded map[string]ports.PreAcceptanceEstimateTrace
	err      error
	writes   int
}

func (double *traceDouble) RecordEstimate(
	_ context.Context,
	trace ports.PreAcceptanceEstimateTrace,
) (ports.PreAcceptanceEstimateTraceEffect, error) {
	if double.err != nil {
		return 0, double.err
	}
	if double.recorded == nil {
		double.recorded = map[string]ports.PreAcceptanceEstimateTrace{}
	}
	key := strings.Join([]string{trace.Tenant.String(), trace.Scope.LegalEntity().String(),
		trace.Scope.Account().String(), trace.Scope.Currency().String(), trace.Request.String()}, "\x00")
	stored, found := double.recorded[key]
	if !found {
		double.recorded[key] = trace
		double.writes++
		return ports.PreAcceptanceEstimateTraceRecorded, nil
	}
	stored.RecordedAt, trace.RecordedAt = time.Time{}, time.Time{}
	if reflect.DeepEqual(stored, trace) {
		return ports.PreAcceptanceEstimateTraceReplay, nil
	}
	return ports.PreAcceptanceEstimateTraceConflict, nil
}

// only 交回唯一那一份留痕；不是恰好一份时让用例失败。
func (double *traceDouble) only(t *testing.T) ports.PreAcceptanceEstimateTrace {
	t.Helper()
	if len(double.recorded) != 1 {
		t.Fatalf("留痕 %d 份, want 恰好 1", len(double.recorded))
	}
	for _, trace := range double.recorded {
		return trace
	}
	return ports.PreAcceptanceEstimateTrace{}
}

// estimateDeps 是只做预付冻结那条路的整套依赖，估价三口由用例给。
func estimateDeps(
	t *testing.T,
	policy *policyDouble,
	adoptions *adoptionViewDouble,
	trials *trialSourceDouble,
	traces *traceDouble,
	balance *balanceDouble,
	ledger *ledgerDouble,
) application.ApplyPreAcceptanceControlDeps {
	t.Helper()
	return application.ApplyPreAcceptanceControlDeps{
		Policy:      policy,
		Adoptions:   adoptions,
		Trials:      trials,
		Traces:      traces,
		Balance:     balance,
		Freezes:     ledger,
		Credit:      &creditDouble{},
		CreditBasis: &creditBasisDouble{},
		RatioBases:  &ratioBaseDouble{},
		Exposures:   &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()},
		Clock:       fixedClock{at: controlAt},
	}
}

// Covers: ADR-0179 决定一、二、五、六 / UC-SA-002 接受前财务控制「形成估价、冻结」——冻结金额等于按采用行要来的逐件试算合计；
// 试算按采用行的计价范围、作用域的币种、控制判断时点与命令带来的委托和提交版本原样去问；估价连同版本清单随控制结果留痕，
// 结果交回的估价与冻结同一个金额。
func TestTheFrozenAmountIsTheEstimateFormedFromTheAdoptedRow(t *testing.T) {
	adoptions := adopted(t)
	trials := estimating(t, 1_250, 800)
	traces := &traceDouble{}
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := mustHandler(t, estimateDeps(t, requiredPolicy(t), adoptions, trials, traces,
		&balanceDouble{balance: balanceWith(t, 10_000)}, ledger))

	result, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ControlApplied {
		t.Fatalf("outcome = %q/%q, want CONTROL_APPLIED", result.Outcome(), result.NotFormedReason())
	}
	freeze, present := result.Freeze()
	if !present || freeze.Status() != domain.FreezeHeld || freeze.AmountMinor() != 2_050 {
		t.Fatalf("freeze = %#v, want a HELD 2050——1250 + 800", freeze)
	}
	estimate, formed := result.Estimate()
	if !formed || estimate.TotalMinor() != freeze.AmountMinor() || estimate.PriceCard() != estimatePriceCard {
		t.Fatalf("estimate = %#v formed = %t, want 冻结金额的那一份估价", estimate, formed)
	}

	if adoptions.askedFor.String() != freezeScope.account {
		t.Fatalf("采用册被问的账户 = %q, want 作用域那一户 %s", adoptions.askedFor, freezeScope.account)
	}
	query := trials.askedWith
	if trials.asked != 1 || query.PricingScope.String() != "SCOPE-SELL-1" || query.Currency.String() != freezeScope.currency ||
		!query.At.Equal(asOfAt) || query.ShipmentRequest != "SHR-1" || query.SubmissionVersion != "SV-1" ||
		query.Tenant.String() != "tenant-1" {
		t.Fatalf("试算被问 %d 次、问法 %+v；计价范围取采用行、币种取作用域、时点取控制判断时点、委托与版本原样", trials.asked, query)
	}

	trace := traces.only(t)
	if trace.Request.String() != "control-request-1" || trace.ShipmentRequest != "SHR-1" || trace.SubmissionVersion != "SV-1" ||
		!trace.At.Equal(asOfAt) || !trace.RecordedAt.Equal(controlAt) || trace.Scope != settlementScope(t) {
		t.Fatalf("留痕 = %+v；请求身份、委托与版本、判断时点、作用域都要在，落库时刻取控制时刻", trace)
	}
	kept := trace.Estimate.Trials()
	if trace.Estimate.TotalMinor() != 2_050 || len(kept) != 2 || len(kept[0].Versions()) != 2 || kept[1].Evaluation() != "EST-P-2" {
		t.Fatalf("留痕的估价 = %#v；逐件评价标识与版本清单原样，合计等于冻结金额", trace.Estimate)
	}
}

// Covers: ADR-0179 决定一「本次各控制项用同一份估价金额」——组合策略两项各占各的账，金额是同一个，试算只问一次、留痕只写一份。
func TestACombinedPolicyOccupiesOneEstimateOnBothLedgers(t *testing.T) {
	trials := estimating(t, 3_000, 1_000)
	traces := &traceDouble{}
	handler := mustHandler(t, application.ApplyPreAcceptanceControlDeps{
		Policy: &policyDouble{policy: itemsPolicy(t, domain.TermsSettlement,
			controlItem(t, domain.PrepaidFreezeControl, 1),
			controlItem(t, domain.CreditCheckControl, 2))},
		Adoptions:   adopted(t),
		Trials:      trials,
		Traces:      traces,
		Balance:     &balanceDouble{balance: balanceWith(t, 10_000)},
		Freezes:     &ledgerDouble{ledger: domain.NewFreezeLedger()},
		Credit:      &creditDouble{standing: standingWith(t, 10_000, 0, false)},
		CreditBasis: authorized(t, 10_000),
		RatioBases:  &ratioBaseDouble{},
		Exposures:   &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()},
		Clock:       fixedClock{at: controlAt},
	})

	result, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	freeze, held := result.Freeze()
	exposure, exposed := result.Exposure()
	if !held || !exposed || freeze.AmountMinor() != 4_000 || exposure.AmountMinor() != 4_000 {
		t.Fatalf("freeze/exposure = %#v / %#v, want 两项都占 4000", freeze, exposure)
	}
	if trials.asked != 1 || traces.writes != 1 {
		t.Fatalf("试算被问 %d 次、留痕写 %d 份, want 1 / 1——一次控制一份估价", trials.asked, traces.writes)
	}
}

// Covers: ADR-0179 决定五 / 票 product-strategy-boundary/25 完成判据一的 SA 半边——采用册上没有这一户时停在金额未配置：不默认形态、
// 不拿零顶，带续办引用；试算不问、余额不读、两本账与留痕都不碰。它与采用册读不回分两格：一个等登记，一个等重试。
func TestAnUnadoptedAccountStopsAtAmountNotConfiguredBeforeAnyLedger(t *testing.T) {
	trials := estimating(t, 4_000)
	traces := &traceDouble{}
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := mustHandler(t, estimateDeps(t, requiredPolicy(t), &adoptionViewDouble{missing: true}, trials, traces, balance, ledger))

	result, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ControlNotFormed || result.NotFormedReason() != application.ControlAmountNotConfigured {
		t.Fatalf("outcome/reason = %q/%q, want CONTROL_NOT_FORMED/CONTROL_AMOUNT_NOT_CONFIGURED", result.Outcome(), result.NotFormedReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("金额未配置没有留下续办引用——它等的是登记，不是重试到死")
	}
	if _, formed := result.Estimate(); formed {
		t.Fatal("没有采用行却交回了估价")
	}
	if trials.asked != 0 || balance.loaded != 0 || ledger.loaded != 0 || traces.writes != 0 {
		t.Fatalf("trials %d / balance %d / ledger %d / traces %d, want 全 0——没有采用行就没有可问的计价范围",
			trials.asked, balance.loaded, ledger.loaded, traces.writes)
	}
	if application.ControlAmountNotConfigured == application.EstimateAdoptionUnavailable {
		t.Fatal("没登记与读不回共用一个原因，运维读不出该催登记还是该重试")
	}
}

// Covers: ADR-0179 决定二、三的停法与 UC-SA-002「非正数金额」——估价那一口的每一格各落一个原因、都答未形成：采用册与试算读不回、
// 选卡未配置与适用冲突、提交版本不可见、缺件、一份试算都没有、任一份没有合计、币种不是作用域的币种、合计不是正数、一份为负、合计加
// 不进 int64、留痕写不进。
// 每一格都在碰两本账之前停下，不拿零、部分合计或换算顶替；原因两两不同。
func TestEveryEstimateCellStopsBeforeTouchingEitherLedger(t *testing.T) {
	answerOf := func(outcome ports.PreAcceptanceTrialOutcome) *trialSourceDouble {
		return &trialSourceDouble{answer: ports.PreAcceptanceTrialAnswer{Outcome: outcome, PriceCard: estimatePriceCard}}
	}
	untotaled := func(t *testing.T) *trialSourceDouble {
		trials := estimating(t, 1_000)
		trial, err := domain.NewUntotaledPreAcceptanceTrial("P-2", "EST-P-2", []string{estimatePriceCard})
		if err != nil {
			t.Fatalf("new untotaled trial: %v", err)
		}
		trials.answer.Trials = append(trials.answer.Trials, trial)
		return trials
	}
	foreign := func(t *testing.T) *trialSourceDouble {
		trials := estimating(t, 1_000)
		trial, err := domain.NewPreAcceptanceTrial("P-2", "EST-P-2", []string{estimatePriceCard}, 100,
			value(t, domain.NewCurrencyCode, "USD"))
		if err != nil {
			t.Fatalf("new foreign trial: %v", err)
		}
		trials.answer.Trials = append(trials.answer.Trials, trial)
		return trials
	}
	cases := map[string]struct {
		adoptions func(t *testing.T) *adoptionViewDouble
		trials    func(t *testing.T) *trialSourceDouble
		traces    *traceDouble
		reason    application.NotFormedReason
	}{
		"采用册读不回": {adoptions: func(*testing.T) *adoptionViewDouble {
			return &adoptionViewDouble{err: errors.New("adoption register down")}
		},
			reason: application.EstimateAdoptionUnavailable},
		"试算读不回": {trials: func(*testing.T) *trialSourceDouble {
			return &trialSourceDouble{err: errors.Join(ports.ErrPreAcceptanceTrialsUnavailable, errors.New("series register down"))}
		}, reason: application.EstimateTrialsUnavailable},
		"选卡未配置":       {trials: func(*testing.T) *trialSourceDouble { return answerOf(ports.PreAcceptanceTrialPriceCardNotConfigured) }, reason: application.EstimatePriceCardNotConfigured},
		"选卡适用冲突":      {trials: func(*testing.T) *trialSourceDouble { return answerOf(ports.PreAcceptanceTrialPriceCardConflict) }, reason: application.EstimatePriceCardConflict},
		"提交版本不可见":     {trials: func(*testing.T) *trialSourceDouble { return answerOf(ports.PreAcceptanceTrialSubmissionNotVisible) }, reason: application.EstimateSubmissionNotVisible},
		"缺件":          {trials: func(*testing.T) *trialSourceDouble { return answerOf(ports.PreAcceptanceTrialsIncomplete) }, reason: application.EstimateDeclarationIncomplete},
		"一份试算都没有":     {trials: func(*testing.T) *trialSourceDouble { return answerOf(ports.PreAcceptanceTrialsFormed) }, reason: application.EstimateHasNoTrial},
		"一份没有合计":      {trials: untotaled, reason: application.EstimateTrialWithoutTotal},
		"币种不是作用域的":    {trials: foreign, reason: application.EstimateCurrencyMismatch},
		"合计不是正数":      {trials: func(t *testing.T) *trialSourceDouble { return estimating(t, 0) }, reason: application.EstimateNotPositive},
		"一份为负":        {trials: func(t *testing.T) *trialSourceDouble { return estimating(t, 500, -1) }, reason: application.EstimateTrialNegative},
		"合计加不进 int64": {trials: func(t *testing.T) *trialSourceDouble { return estimating(t, 1<<62, 1<<62) }, reason: application.EstimateSumOverflow},
		"留痕写不进":       {traces: &traceDouble{err: errors.New("trace register down")}, reason: application.EstimateTraceUnavailable},
	}
	seen := map[application.NotFormedReason]string{}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			adoptions, trials, traces := adopted(t), estimating(t, 4_000), &traceDouble{}
			if tc.adoptions != nil {
				adoptions = tc.adoptions(t)
			}
			if tc.trials != nil {
				trials = tc.trials(t)
			}
			if tc.traces != nil {
				traces = tc.traces
			}
			balance := &balanceDouble{balance: balanceWith(t, 10_000)}
			ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
			handler := mustHandler(t, estimateDeps(t, requiredPolicy(t), adoptions, trials, traces, balance, ledger))

			result, err := handler.Handle(context.Background(), command(t))
			if err != nil {
				t.Fatalf("估价那一格被当成技术错误抛出: %v", err)
			}
			if result.Outcome() != application.ControlNotFormed || result.NotFormedReason() != tc.reason {
				t.Fatalf("outcome/reason = %q/%q, want CONTROL_NOT_FORMED/%q", result.Outcome(), result.NotFormedReason(), tc.reason)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("待判断没有留下可续办引用")
			}
			if balance.loaded != 0 || ledger.loaded != 0 || ledger.saved != 0 {
				t.Fatalf("balance %d / ledger loaded %d saved %d, want 0——估价没形成就不该碰这个客户的资金",
					balance.loaded, ledger.loaded, ledger.saved)
			}
			if tc.reason != application.EstimateTraceUnavailable && traces.writes != 0 {
				t.Fatalf("估价没形成却写了 %d 份留痕", traces.writes)
			}
		})
		if other, clash := seen[tc.reason]; clash {
			t.Fatalf("%s 与 %s 共用原因 %s——停在哪一格分不出来", name, other, tc.reason)
		}
		if tc.reason.String() == "" {
			t.Fatalf("%s 的原因 %d 没有名字", name, tc.reason)
		}
		seen[tc.reason] = name
	}
}

// Covers: ADR-0179 决定六与两本账「同一请求身份携带不同内容形成冲突」同一条判据——同一次控制请求第二次来时估价换了一份（这里
// 是换了一件的合计），留痕答冲突，编排停在`请求冲突`、不去碰两本账；同一份估价再来是重放，原冻结原样交回、不二次占用。
func TestTheSameRequestWithAnotherEstimateIsAConflictBeforeEitherLedger(t *testing.T) {
	trials := estimating(t, 4_000)
	traces := &traceDouble{}
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := mustHandler(t, estimateDeps(t, requiredPolicy(t), adopted(t), trials, traces,
		&balanceDouble{balance: balanceWith(t, 10_000)}, ledger))

	first, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, _ := first.Freeze()
	replay, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("handle replay: %v", err)
	}
	if repeated, present := replay.Freeze(); !present || repeated.FreezeID() != original.FreezeID() || traces.writes != 1 {
		t.Fatalf("replay freeze = %#v, traces written %d; want 原冻结、留痕仍一份", repeated, traces.writes)
	}

	trials.set(t, 3_000, 1_000)
	loadedBefore := ledger.loaded
	conflicting, err := handler.Handle(context.Background(), command(t))
	if err != nil {
		t.Fatalf("请求冲突被当成技术错误抛出: %v", err)
	}
	if conflicting.Outcome() != application.ControlRequestConflict {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_CONFLICT——同一次请求的金额已经有了出处", conflicting.Outcome())
	}
	if ledger.loaded != loadedBefore {
		t.Fatal("留痕已答冲突，编排仍去读了冻结账本")
	}
	if held := ledger.ledger.HeldMinor(); held != 4_000 || traces.only(t).Estimate.TotalMinor() != 4_000 {
		t.Fatalf("held = %d、留痕合计 = %d, want 4000 / 4000——冲突不覆盖原冻结与原留痕", held, traces.only(t).Estimate.TotalMinor())
	}
}

// Covers: ADR-0025「翻译必须是全函数」在估价这一口——试算口交出端口约定之外的错误、或交出端口词表外的格，是消费侧适配器译坏了，
// 上抛为错误，不吸收成任何一格待判断。
func TestAnUntranslatableTrialAnswerIsAProgrammingError(t *testing.T) {
	for name, trials := range map[string]*trialSourceDouble{
		"端口约定之外的错误": {err: errors.New("adapter translated nothing")},
		"词表外的格":     {answer: ports.PreAcceptanceTrialAnswer{Outcome: ports.PreAcceptanceTrialOutcomeInvalid}},
	} {
		t.Run(name, func(t *testing.T) {
			handler := mustHandler(t, estimateDeps(t, requiredPolicy(t), adopted(t), trials, &traceDouble{},
				&balanceDouble{balance: balanceWith(t, 10_000)}, &ledgerDouble{ledger: domain.NewFreezeLedger()}))
			if _, err := handler.Handle(context.Background(), command(t)); err == nil {
				t.Fatal("适配器译坏的答复被吸收成了一格业务答案")
			}
		})
	}
}
