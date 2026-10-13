package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: ADR-0094 Decision 五 —「落此格前先把带等待态的聚合 Save 落库」。`判断时点未配置`停在
// 校验之前，Decide 走不到，等待态由 AwaitOperatorRegistration 在决定之前写下并经仓储落库；本轮
// 交回的原因仍是那一格自己的（续办路径`等待运营登记`），处理记录照追加。
func TestAsOfNotConfiguredParksTheRequestOnOperatorRegistrationBeforeAnsweringUndecided(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfNotConfigured

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ReachabilityAsOfNotConfigured {
		t.Fatalf("pending reason = %q, want REACHABILITY_AS_OF_NOT_CONFIGURED", result.PendingReason())
	}

	saved := fixture.repository.saved
	if saved == nil {
		t.Fatal("等待态没有落库——ADR-0094 Decision 五要求交回该原因之前先 Save")
	}
	waiting, present := saved.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByOperatorRegistration {
		t.Fatalf("saved waitingOn = %v/%v, want OPERATOR_REGISTRATION", waiting, present)
	}
	if saved.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("saved state = %q, want SUBMITTED——停等不是决定", saved.State())
	}
	if _, formed := saved.AcceptanceDecision(); formed {
		t.Fatal("停等的那一份聚合带着一个决定")
	}
	if len(fixture.requests.recordedAttempts) != 1 {
		t.Fatalf("processing attempts = %d, want 1——停等之外处理记录照追加", len(fixture.requests.recordedAttempts))
	}
	if path := fixture.requests.recordedAttempts[0].ResumePath(); path != domain.ResumeByOperatorRegistration {
		t.Fatalf("attempt resume path = %q, want OPERATOR_REGISTRATION", path)
	}
}

// Covers: 同一 Decision 五的反面 — 等待态**没**落库时不得交回`判断时点未配置`：那个原因是消费门
// （D4 之后）提交暂停的凭据，暂停没落库就交它，等待态随本轮回滚蒸发而投递已被记为完毕，队列从此
// 列不出这份委托。三种没落库的样子各交回自己那一格，全部归内部重试，照旧回滚重投。
func TestAWaitThatDidNotLandDoesNotClaimOperatorRegistration(t *testing.T) {
	cases := map[string]struct {
		arrange func(*awaitingRequestStore)
		want    application.JudgmentPendingReason
	}{
		"委托查不到": {
			arrange: func(store *awaitingRequestStore) { store.missing = true },
			want:    application.ShipmentRequestUnavailable,
		},
		"保存没落库": {
			arrange: func(store *awaitingRequestStore) { store.saveErr = errors.New("库不可达") },
			want:    application.OperatorRegistrationWaitNotSaved,
		},
		"版本被抢先": {
			arrange: func(store *awaitingRequestStore) { store.conflicted = true },
			want:    application.StaleShipmentRequestRevision,
		},
	}
	seen := make(map[string]string, len(cases))
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.commercial.asOfOutcome = ports.JudgmentAsOfNotConfigured
			tc.arrange(fixture.repository)

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceJudgmentUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if result.PendingReason() != tc.want {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), tc.want)
			}
			if result.PendingReason() == application.ReachabilityAsOfNotConfigured {
				t.Fatal("等待态没落库却交回了`判断时点未配置`")
			}
			if len(fixture.requests.recordedAttempts) != 1 {
				t.Fatalf("processing attempts = %d, want 1", len(fixture.requests.recordedAttempts))
			}
			if path := fixture.requests.recordedAttempts[0].ResumePath(); path != domain.ResumeByInternalRetry {
				t.Fatalf("attempt resume path = %q, want INTERNAL_RETRY——没落库的停等只有本方推得动", path)
			}
			reference := result.ContinuationReference().String()
			if clash, exists := seen[reference]; exists {
				t.Fatalf("%q 与 %q 共用续办引用 %q", name, clash, reference)
			}
			seen[reference] = name
		})
	}
}

// Covers: AwaitOperatorRegistration 的两道门在编排里的落法 — 委托已越过决定边界时没有等待态要落，
// 也没有队列条目要保；原因照交不改（本轮真实停在哪一步仍由它说出来），聚合不写。
func TestADecidedRequestIsNotParkedAgain(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfNotConfigured
	fixture.repository.decided = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.ReachabilityAsOfNotConfigured {
		t.Fatalf("pending reason = %q, want REACHABILITY_AS_OF_NOT_CONFIGURED", result.PendingReason())
	}
	if fixture.repository.saved != nil {
		t.Fatal("一份已决委托被重新写成停等")
	}
}

// Covers: 只有续办路径为`等待运营登记`的停顿才碰聚合。其余未决由信封重投自然再驱，等待态不必落库；
// 多读一次聚合不只是浪费——它把「谁该落等待态」这份判断从 ResumePath 挪到了编排里。
func TestOtherStallsLeaveTheAggregateAlone(t *testing.T) {
	cases := map[string]ports.JudgmentAsOfOutcome{
		"依据未解析": ports.JudgmentAsOfBasisNotResolved,
		"权威未决":  ports.JudgmentAsOfPending,
		"值被拒":   ports.JudgmentAsOfValueRejected,
		"入参未受理": ports.JudgmentAsOfInputNotAccepted,
	}
	for name, outcome := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.commercial.asOfOutcome = outcome

			if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
				t.Fatalf("handle: %v", err)
			}
			if fixture.repository.finds != 0 || fixture.repository.saved != nil {
				t.Fatalf("finds = %d, saved = %v——不是`等待运营登记`的停顿碰了聚合",
					fixture.repository.finds, fixture.repository.saved != nil)
			}
		})
	}
}

// Covers: 财务控制那条腿与可达性同一机制 — `FinancialControlAsOfNotConfigured` 同样在决定之前
// 落等待态；两条腿各自的原因不同，续办引用因此分得开。
func TestFinancialControlAsOfNotConfiguredParksTheRequestToo(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfNotConfigured

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.FinancialControlAsOfNotConfigured {
		t.Fatalf("pending reason = %q, want FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED", result.PendingReason())
	}
	saved := fixture.repository.saved
	if saved == nil {
		t.Fatal("等待态没有落库")
	}
	if waiting, present := saved.AcceptanceDecisionTask().WaitingOn(); !present || waiting != domain.ResumeByOperatorRegistration {
		t.Fatalf("saved waitingOn = %v/%v, want OPERATOR_REGISTRATION", waiting, present)
	}

	fixture = newFinancialControlFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfNotConfigured
	fixture.repository.conflicted = true
	result, err = fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", result.PendingReason())
	}
}

// Covers: 票 routing-first-cut/19 拆格那一笔、ADR-0094 Decision 五 — 控制端口答`未配置`与时点那一支同一
// 机制：等待态先落库，本轮交回 FinancialControlNotConfigured，处理尝试带`等待运营登记`入账；控制结果一份
// 不记。ADR-0095 Decision 一要的就是这条留痕：原因、续办路径与续办引用都在任务上，不再随回滚蒸发。
func TestAControlNotConfiguredParksTheRequestOnOperatorRegistration(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.controller.portOutcome = ports.PreAcceptanceControlNotConfigured
	fixture.controller.reason = "SA-CONTROL_POLICY_NOT_CONFIGURED"

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.FinancialControlNotConfigured {
		t.Fatalf("pending reason = %q, want FINANCIAL_CONTROL_NOT_CONFIGURED", result.PendingReason())
	}
	saved := fixture.repository.saved
	if saved == nil {
		t.Fatal("等待态没有落库——交回`等待运营登记`那一格的原因之前要先 Save")
	}
	if waiting, present := saved.AcceptanceDecisionTask().WaitingOn(); !present || waiting != domain.ResumeByOperatorRegistration {
		t.Fatalf("saved waitingOn = %v/%v, want OPERATOR_REGISTRATION", waiting, present)
	}
	if len(fixture.requests.recordedAttempts) != 1 {
		t.Fatalf("processing attempts = %d, want 1", len(fixture.requests.recordedAttempts))
	}
	attempt := fixture.requests.recordedAttempts[0]
	if attempt.ResumePath() != domain.ResumeByOperatorRegistration {
		t.Fatalf("attempt resume path = %q, want OPERATOR_REGISTRATION", attempt.ResumePath())
	}
	if attempt.Reason().String() != "FINANCIAL_CONTROL_NOT_CONFIGURED" {
		t.Fatalf("attempt reason = %q, want FINANCIAL_CONTROL_NOT_CONFIGURED", attempt.Reason())
	}
	if attempt.ContinuationReference() != result.ContinuationReference() {
		t.Fatalf("attempt continuation = %q, result continuation = %q——任务上记的不是交回的那一条",
			attempt.ContinuationReference(), result.ContinuationReference())
	}
	if len(fixture.requests.recordedControl) != 0 {
		t.Fatalf("recorded %d control results for a control that never formed", len(fixture.requests.recordedControl))
	}
}

// Covers: 票 routing-first-cut/19 完成判据「作用域停、金额停、策略停三者的续办引用两两不同」—— 提供方原因
// 进续办派生。作用域、策略与金额（票 product-strategy-boundary/25 起）同落`未配置`格、同一个未决原因，只靠原因
// 引用分开；停在`未形成`的那一格（这里取试算读不回）照旧不碰聚合。
func TestEachControlStallCarriesItsOwnContinuation(t *testing.T) {
	cases := map[string]struct {
		outcome   ports.PreAcceptanceControlOutcome
		reason    string
		want      application.JudgmentPendingReason
		wantsWait bool
	}{
		"作用域":   {ports.PreAcceptanceControlNotConfigured, "CONTROL_SCOPE_NOT_CONFIGURED", application.FinancialControlNotConfigured, true},
		"策略":    {ports.PreAcceptanceControlNotConfigured, "SA-CONTROL_POLICY_NOT_CONFIGURED", application.FinancialControlNotConfigured, true},
		"金额":    {ports.PreAcceptanceControlNotConfigured, "SA-CONTROL_AMOUNT_NOT_CONFIGURED", application.FinancialControlNotConfigured, true},
		"试算读不回": {ports.PreAcceptanceControlNotFormed, "SA-ESTIMATE_TRIALS_UNAVAILABLE", application.FinancialControlNotFormed, false},
	}
	seen := make(map[string]string, len(cases))
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFinancialControlFixture(t)
			fixture.controller.portOutcome = tc.outcome
			fixture.controller.reason = tc.reason

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.PendingReason() != tc.want {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), tc.want)
			}
			if parked := fixture.repository.saved != nil; parked != tc.wantsWait {
				t.Fatalf("parked = %v, want %v——只有`等待运营登记`那一格落等待态", parked, tc.wantsWait)
			}
			reference := result.ContinuationReference().String()
			if clash, exists := seen[reference]; exists {
				t.Fatalf("%q 与 %q 共用续办引用 %q", name, clash, reference)
			}
			seen[reference] = name
		})
	}
}

// Covers: 控制那一支的保存护栏 — 等待态没落库时不交回`未配置`那一格，改交保存那一格自己的原因（内部重试，
// 照旧回滚重投），范围随原因一起放下：提供方原因说的是控制为何没形成，不是保存为何没落。
func TestAControlWaitThatDidNotLandFallsBackToInternalRetry(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.controller.portOutcome = ports.PreAcceptanceControlNotConfigured
	fixture.controller.reason = "CONTROL_SCOPE_NOT_CONFIGURED"
	fixture.repository.saveErr = errors.New("库不可达")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.OperatorRegistrationWaitNotSaved {
		t.Fatalf("pending reason = %q, want OPERATOR_REGISTRATION_WAIT_NOT_SAVED", result.PendingReason())
	}
	if len(fixture.requests.recordedAttempts) != 1 {
		t.Fatalf("processing attempts = %d, want 1", len(fixture.requests.recordedAttempts))
	}
	if path := fixture.requests.recordedAttempts[0].ResumePath(); path != domain.ResumeByInternalRetry {
		t.Fatalf("attempt resume path = %q, want INTERNAL_RETRY", path)
	}

	other := newFinancialControlFixture(t)
	other.controller.portOutcome = ports.PreAcceptanceControlNotConfigured
	other.controller.reason = "SA-CONTROL_POLICY_NOT_CONFIGURED"
	other.repository.saveErr = errors.New("库不可达")
	second, err := other.handler.Handle(context.Background(), other.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if second.ContinuationReference() != result.ContinuationReference() {
		t.Fatalf("保存没落库的两次停顿续办引用不同（%q / %q）——提供方原因没有随原因一起放下",
			result.ContinuationReference(), second.ContinuationReference())
	}
}

// awaitingRequestStore 是两条 as-of 编排的委托仓储替身：默认交回一份`已提交`委托并记下保存的那一份；
// 三个开关各造一种「没落库」，decided 造一份真经领域形成的已决委托（假状态挡不住领域门）。
type awaitingRequestStore struct {
	t          *testing.T
	missing    bool
	decided    bool
	conflicted bool
	saveErr    error
	finds      int
	saved      *domain.ShipmentRequest
}

func (store *awaitingRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	store.finds++
	if store.missing {
		return domain.ShipmentRequest{}, false, nil
	}
	if store.decided {
		rejected, err := submittedRequest(store.t).RejectByAuthority(domain.ActiveRejectionSpec{
			DecisionID: mustValue(store.t, domain.NewAcceptanceDecisionID, "decision-0"),
			Authority:  mustValue(store.t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-0"),
			Decider:    mustValue(store.t, domain.NewDeciderReference, "OPERATOR-0"),
			Reason:     mustValue(store.t, domain.NewRejectionReasonReference, "EARLIER_DECISION"),
			Evidence:   mustValue(store.t, domain.NewRejectionEvidenceReference, "EVID-0"),
			DecidedAt:  handlerClockAt,
		})
		if err != nil {
			store.t.Fatalf("form the earlier decision: %v", err)
		}
		return rejected, true, nil
	}
	return submittedRequest(store.t), true, nil
}

func (store *awaitingRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *awaitingRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.saveErr != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, store.saveErr
	}
	if store.conflicted {
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

var _ ports.ShipmentRequestRepository = (*awaitingRequestStore)(nil)
