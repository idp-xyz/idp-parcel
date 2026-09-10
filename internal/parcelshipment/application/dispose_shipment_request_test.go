package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件钉 ADR-0132 在应用层的两处落点：形成决定那一步对`等待授权处置`的保存护栏（形照 ADR-0086 决定一
// 那三条），以及处置命令——授权先于一切写、结果代数照复核完成分格、两去向都在命令事务里按 OccupationFormed
// 释放本版本占用、不发信封。

// awaitingDispositionControl 是 SA 交回的「预付冻结成立、信用受限」结果，受限项已采用正文登记的
// `进入授权处置`——形成控制判断那一步落库的正是这份。
func awaitingDispositionControl(t *testing.T) domain.FinancialControlResult {
	t.Helper()
	adopted, err := heldThenRestrictedControl(t).AdoptControlDispositions(
		map[domain.ControlItemKind]domain.AdoptedControlDisposition{
			domain.CreditCheckControlItem: adoptedDispositionFor(t, domain.AuthorizedDispositionOnControlFailure),
		})
	if err != nil {
		t.Fatalf("adopt control dispositions: %v", err)
	}
	return adopted
}

// Covers: ADR-0132 决定二 + ADR-0086 决定一前半——`等待授权处置`在交回之前先把带等待态的聚合落库，
// 委托仍是`已提交`、不形成决定；处理记录的续办路径是「授权处置」，不是复核也不是内部重试。
func TestAPendingAuthorizedDispositionPersistsThePauseItReports(t *testing.T) {
	fixture := newDecisionFixture(t)
	control := awaitingDispositionControl(t)
	fixture.judgments.control = &control

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——正文登了授权处置的受限控制被自动决定了", result.Outcome())
	}
	if result.PendingReason() != application.AuthorizedDispositionPending {
		t.Fatalf("pending reason = %q, want AUTHORIZED_DISPOSITION_PENDING", result.PendingReason())
	}
	if _, formed := result.AcceptanceDecision(); formed {
		t.Fatal("等处置形成了一份决定")
	}
	if fixture.requests.saved == nil {
		t.Fatal("暂停没落库就交回了`等待授权处置`——消费门会据此提交入账，队列从此列不出这份委托")
	}
	waiting, present := fixture.requests.saved.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("saved waitingOn = (%v, %v), want AUTHORIZED_DISPOSITION——落库的聚合没带等待态，投影列会写 0", waiting, present)
	}
	if fixture.requests.saved.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("saved state = %q, want SUBMITTED——暂停不是决定，生命周期不得离开已提交", fixture.requests.saved.State())
	}
	if fixture.release.calls != 0 {
		t.Fatal("等处置那一轮释放了占用——去向还没选，接受尚未确定未成立")
	}
	if len(fixture.recorder.recordedAttempts) != 1 ||
		fixture.recorder.recordedAttempts[0].ResumePath() != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("attempts = %v, want one attempt on AUTHORIZED_DISPOSITION", fixture.recorder.recordedAttempts)
	}
}

// Covers: ADR-0086 决定一后半（ADR-0132 决定二原样扩用）——暂停没落库时**不得**交回`等待授权处置`，
// 改交保存那一格自己的原因，消费门照旧回滚重投。
func TestAPauseThatFailedToPersistDoesNotReportAuthorizedDispositionPending(t *testing.T) {
	unreachable := newDecisionFixture(t)
	control := awaitingDispositionControl(t)
	unreachable.judgments.control = &control
	unreachable.requests.err = errors.New("storage unreachable")

	result, err := unreachable.handler.Handle(context.Background(), unreachable.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() == application.AuthorizedDispositionPending {
		t.Fatal("保存失败仍交回`等待授权处置`——消费门会提交一份没落库的暂停")
	}
	if result.PendingReason() != application.DecisionNotRecorded {
		t.Fatalf("pending reason = %q, want DECISION_NOT_RECORDED", result.PendingReason())
	}

	outraced := newDecisionFixture(t)
	outraced.judgments.control = &control
	outraced.requests.conflict = true

	lost, err := outraced.handler.Handle(context.Background(), outraced.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if lost.PendingReason() == application.AuthorizedDispositionPending {
		t.Fatal("版本冲突仍交回`等待授权处置`——抢先那一方写了什么本方并不知道")
	}
	if lost.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", lost.PendingReason())
	}
}

// Covers: ADR-0132 决定一末段——正文登 REJECT 的受限控制照今天拒绝，不进等处置。
func TestARejectDispositionStillRejectsWithoutPausing(t *testing.T) {
	fixture := newDecisionFixture(t)
	control, err := heldThenRestrictedControl(t).AdoptControlDispositions(
		map[domain.ControlItemKind]domain.AdoptedControlDisposition{
			domain.CreditCheckControlItem: adoptedDispositionFor(t, domain.RejectOnControlFailure),
		})
	if err != nil {
		t.Fatalf("adopt control dispositions: %v", err)
	}
	fixture.judgments.control = &control

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceDecided || result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("outcome = %q state = %q, want DECIDED / REJECTED", result.Outcome(), result.State())
	}
	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want 1——第一项占下的资金随拒绝释放", fixture.release.calls)
	}
}

// Covers: ADR-0132 决定一`拒绝`去向——处置记下即形成授权角色拒绝决定（实际决定方 = 处置方、授权引用来自
// PC 的答复、时间取编排时钟），委托转`已拒绝`并落库，本版本已成立项的占用按原关联释放，不发信封。
func TestDisposingByRejectionFormsTheDecisionAndReleasesTheOccupation(t *testing.T) {
	fixture := newDispositionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AuthorizedDispositionRecorded {
		t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	decision, formed := result.AcceptanceDecision()
	if !formed || decision.Accepted() {
		t.Fatalf("decision formed=%v accepted=%v; want a rejection", formed, decision.Accepted())
	}
	rejection, active := decision.ActiveRejection()
	if !active || rejection.Decider().String() != "CREDIT-OFFICER-1" ||
		rejection.Authority().String() != "PC-DISPOSE-RULE-3/v1" ||
		rejection.Reason().String() != "CREDIT_LIMIT_NOT_EXTENDED" ||
		rejection.Evidence().String() != "EVID-D1" {
		t.Fatalf("rejection trail = %+v (active=%v)", rejection, active)
	}
	if !decision.DecidedAt().Equal(handlerClockAt) {
		t.Fatalf("decidedAt = %v, want the orchestration clock", decision.DecidedAt())
	}
	disposition, present := result.Disposition()
	if !present || disposition.Choice() != domain.DisposeByRejection {
		t.Fatalf("disposition = %+v (present=%v)", disposition, present)
	}
	if fixture.requests.saved == nil || fixture.requests.saved.State() != domain.ShipmentRequestRejected {
		t.Fatal("拒绝去向没有落库")
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d decision identities, want 1", fixture.identities.issued)
	}
	if fixture.release.calls != 1 || fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("release calls = %d id = %q; 第一项占下的资金随处置成了孤儿", fixture.release.calls, fixture.release.controlResultID)
	}
	if result.CompensationReference().String() != "" {
		t.Fatal("释放成功却交回了补偿续办引用")
	}
}

// Covers: ADR-0132 决定一`交客户补充`去向——不形成决定、委托仍`已提交`，等待态转到`等待受控补充`并落库，
// 处置记录留在任务上；本版本的占用同样释放（这一版不会再被判）；不领决定标识。
func TestDisposingToCustomerSupplementMovesTheWaitAndReleasesTheOccupation(t *testing.T) {
	fixture := newDispositionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByCustomerSupplement))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AuthorizedDispositionRecorded {
		t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED——交客户补充不是决定", result.State())
	}
	if _, formed := result.AcceptanceDecision(); formed {
		t.Fatal("交客户补充形成了一份决定")
	}
	if fixture.requests.saved == nil {
		t.Fatal("处置没有落库")
	}
	waiting, present := fixture.requests.saved.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByCustomerSupplement {
		t.Fatalf("saved waitingOn = (%v, %v), want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	recorded, present := fixture.requests.saved.AcceptanceDecisionTask().AuthorizedDisposition()
	if !present || recorded.Choice() != domain.DisposeByCustomerSupplement ||
		recorded.Authority().String() != "PC-DISPOSE-RULE-3/v1" {
		t.Fatalf("saved disposition = %+v (present=%v)", recorded, present)
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision identities, want 0——交客户补充不形成决定", fixture.identities.issued)
	}
	if fixture.release.calls != 1 || fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("release calls = %d id = %q; 这一版不会再被判，占用没有理由留着", fixture.release.calls, fixture.release.controlResultID)
	}
}

// Covers: 释放失败不回滚处置——处置已越过提交边界，补偿按原关联另行续办；引用与主动拒绝、自动拒绝派生的
// 逐字一致（同一笔占用因同一原因停下，续办方不必先知道是谁停的）。
func TestAFailedReleaseAfterDispositionYieldsTheSharedCompensationReference(t *testing.T) {
	fixture := newDispositionFixture(t)
	fixture.release.err = errors.New("settlement unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AuthorizedDispositionRecorded {
		t.Fatalf("outcome = %q, want RECORDED——释放失败不改写处置结果", result.Outcome())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("释放失败却没有交回补偿续办引用")
	}
	if fixture.requests.saved == nil || fixture.requests.saved.State() != domain.ShipmentRequestRejected {
		t.Fatal("释放失败回滚了已落库的拒绝")
	}
}

// Covers: 授权先于一切写动作（顺序同 RejectShipmentRequestHandler / CompleteManualReviewHandler）——
// `不允许`与`授权规则未配置`各占一格、都什么也不落库、不领决定标识、不释放；权威答不出上抛；
// 未获授权的处置人对着已换代的版本提交，得到的是`不允许`而不是`版本已换代`。
func TestDispositionAuthorizationPrecedesEveryWrite(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.authorizer.outcome = ports.AuthorizationRefused
		command := fixture.command(t, domain.DisposeByRejection)
		command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-0")

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionNotAuthorized {
			t.Fatalf("outcome = %q, want NOT_AUTHORIZED before any version answer", result.Outcome())
		}
		fixture.assertNothingWritten(t)
		fixture.assertNoIdentityIssued(t)
	})

	t.Run("rules not configured", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.authorizer.outcome = ports.AuthorizationRulesNotConfigured

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionAuthorityRulesNotConfigured {
			t.Fatalf("outcome = %q, want AUTHORITY_RULES_NOT_CONFIGURED——对一个尚未配置的产品不得说「你无权处置」", result.Outcome())
		}
		fixture.assertNothingWritten(t)
		fixture.assertNoIdentityIssued(t)
	})

	t.Run("authority unavailable", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.authorizer.err = errors.New("权威不可读")

		_, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if !errors.Is(err, fixture.authorizer.err) {
			t.Fatalf("err = %v, want the wrapped authority failure", err)
		}
		fixture.assertNothingWritten(t)
		fixture.assertNoIdentityIssued(t)
	})

	t.Run("query carries the disposer, reason and evidence but no authority", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection)); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.authorizer.asked) != 1 {
			t.Fatalf("asked %d times, want 1", len(fixture.authorizer.asked))
		}
		asked := fixture.authorizer.asked[0]
		if asked.Disposer.String() != "CREDIT-OFFICER-1" || asked.Reason.String() != "CREDIT_LIMIT_NOT_EXTENDED" ||
			asked.Evidence.String() != "EVID-D1" || asked.SubmissionVersion.String() != "version-1" {
			t.Fatalf("query = %+v", asked)
		}
	})
}

// Covers: 结果代数照 ManualReviewCompletionOutcome 分格——`已处置`读回既有处置；`任务已完结`交回既有决定；
// `没停在等处置`是本命令自己的一格；`版本已换代`报出当前版本；版本冲突是业务答案且不释放。任一格都不落库。
func TestEveryDispositionRefusalLandsOnItsOwnOutcome(t *testing.T) {
	t.Run("already disposed", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		handed, err := fixture.requests.request.DisposeUnderAuthority(domain.DisposeUnderAuthoritySpec{
			Disposition: earlierDisposition(t),
		})
		if err != nil {
			t.Fatalf("dispose earlier: %v", err)
		}
		fixture.requests.request = handed

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionAlreadyRecorded {
			t.Fatalf("outcome = %q, want ALREADY_DISPOSED", result.Outcome())
		}
		existing, present := result.Disposition()
		if !present || existing.Disposer().String() != "CREDIT-OFFICER-0" {
			t.Fatalf("disposition = %+v (present=%v); the second disposition must read the first", existing, present)
		}
		fixture.assertNothingWritten(t)
	})

	t.Run("task already closed", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		rejected, err := fixture.requests.request.RejectByAuthority(domain.ActiveRejectionSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-0"),
			Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-0"),
			Decider:    mustValue(t, domain.NewDeciderReference, "OPERATOR-0"),
			Reason:     mustValue(t, domain.NewRejectionReasonReference, "EARLIER_DECISION"),
			Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "EVID-0"),
			DecidedAt:  handlerClockAt,
		})
		if err != nil {
			t.Fatalf("form the earlier decision: %v", err)
		}
		fixture.requests.request = rejected

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionTaskAlreadyClosed {
			t.Fatalf("outcome = %q, want TASK_ALREADY_CLOSED", result.Outcome())
		}
		if _, present := result.AcceptanceDecision(); !present {
			t.Fatal("a closed task carried no decision to read")
		}
		fixture.assertNothingWritten(t)
	})

	t.Run("not waiting on disposition", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.requests.request = submittedRequest(t)

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionNotWaiting {
			t.Fatalf("outcome = %q, want NOT_WAITING_ON_DISPOSITION", result.Outcome())
		}
		fixture.assertNothingWritten(t)
	})

	t.Run("version superseded", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		command := fixture.command(t, domain.DisposeByRejection)
		command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-0")

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionVersionSuperseded {
			t.Fatalf("outcome = %q, want VERSION_SUPERSEDED", result.Outcome())
		}
		if result.CurrentVersion().String() != "version-1" {
			t.Fatalf("current version = %q, want version-1", result.CurrentVersion())
		}
		fixture.assertNothingWritten(t)
	})

	t.Run("revision conflict", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.requests.conflict = true

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AuthorizedDispositionConflict {
			t.Fatalf("outcome = %q, want REVISION_CONFLICT", result.Outcome())
		}
		if fixture.release.calls != 0 {
			t.Fatal("本方这次处置确定没落库，却发了释放——那笔占用归抢先那一方处置")
		}
	})
}

type dispositionFixture struct {
	handler    *application.DisposeShipmentRequestHandler
	requests   *disposableRequestStore
	authorizer *dispositionAuthorizerDouble
	judgments  *recordedJudgmentsDouble
	release    *controlReleaseDouble
	identities *decisionIdentityFactory
}

func newDispositionFixture(t *testing.T) *dispositionFixture {
	t.Helper()
	control := heldThenRestrictedControl(t)
	fixture := &dispositionFixture{
		requests: &disposableRequestStore{t: t, request: awaitingDispositionRequest(t)},
		authorizer: &dispositionAuthorizerDouble{
			outcome:   ports.AuthorizationGranted,
			authority: mustValue(t, domain.NewDispositionAuthorityReference, "PC-DISPOSE-RULE-3/v1"),
		},
		judgments:  &recordedJudgmentsDouble{t: t, control: &control},
		release:    &controlReleaseDouble{},
		identities: &decisionIdentityFactory{t: t},
	}
	fixture.handler = application.NewDisposeShipmentRequestHandler(application.DisposeShipmentRequestDeps{
		Requests:   fixture.requests,
		Authorizer: fixture.authorizer,
		Judgments:  fixture.judgments,
		Release:    fixture.release,
		Identities: fixture.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return fixture
}

func (fixture *dispositionFixture) command(
	t *testing.T,
	choice domain.AuthorizedDispositionChoice,
) application.DisposeShipmentRequestCommand {
	t.Helper()
	return application.DisposeShipmentRequestCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Disposer:          mustValue(t, domain.NewDisposerReference, "CREDIT-OFFICER-1"),
		Choice:            choice,
		Reason:            mustValue(t, domain.NewDispositionReasonReference, "CREDIT_LIMIT_NOT_EXTENDED"),
		Evidence:          mustValue(t, domain.NewDispositionEvidenceReference, "EVID-D1"),
	}
}

// assertNothingWritten 钉「未获授权 / 撞门的那几格什么也不落库」：不保存、不释放。决定标识不在这里断言：
// 授权那三格在签发之前就答了（各自另断），撞领域门的几格与主动拒绝同形——标识在授权之后、转移之前签发，
// 撞上已成立的决定时已经领过一个。
func (fixture *dispositionFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	if fixture.requests.saved != nil {
		t.Fatal("这一格不该碰聚合，却保存了")
	}
	if fixture.release.calls != 0 {
		t.Fatal("这一格不该释放占用，却发了释放")
	}
}

// assertNoIdentityIssued 钉「授权先于签发」：一次未获授权的尝试不该消耗一个本上下文签发的稀缺身份。
func (fixture *dispositionFixture) assertNoIdentityIssued(t *testing.T) {
	t.Helper()
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision identities, want 0——授权之前不得签发决定标识", fixture.identities.issued)
	}
}

// awaitingDispositionRequest 把 submittedRequest 经领域推到`等待授权处置`：其余组全部通过，接受前财务控制
// 译成`无法判定`+ 续办路径「授权处置」。这一层是被测行为的前提而不是它的一部分。
func awaitingDispositionRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	groups := []domain.AcceptanceCheckGroup{
		domain.CustomerRelationshipCheck,
		domain.LegalEntityAndContractCheck,
		domain.ProductAndServiceCheck,
		domain.MemberBaselineCheck,
		domain.RequiredDocumentCheck,
		domain.PreAcceptanceFinancialControlCheck,
		domain.NetworkReachabilityCheck,
	}
	applicable, err := domain.NewApplicableCheckGroups(groups...)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, domain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  mustValue(t, domain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: mustValue(t, domain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: domain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}

	checks := make([]domain.AcceptanceCheck, 0, len(groups)+2)
	for _, group := range groups {
		if group == domain.PreAcceptanceFinancialControlCheck || group == domain.NetworkReachabilityCheck {
			continue
		}
		check, err := domain.NewAcceptanceCheck(group, domain.DeclaredParcelID{}, domain.CheckPassed, domain.CheckReason{})
		if err != nil {
			t.Fatalf("new acceptance check: %v", err)
		}
		checks = append(checks, check)
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		check, err := domain.NewAcceptanceCheck(
			domain.NetworkReachabilityCheck,
			mustValue(t, domain.NewDeclaredParcelID, parcel),
			domain.CheckPassed,
			domain.CheckReason{},
		)
		if err != nil {
			t.Fatalf("new acceptance check: %v", err)
		}
		checks = append(checks, check)
	}
	control, err := domain.FinancialControlCheckFor(awaitingDispositionControl(t))
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}
	checks = append(checks, control)

	waiting, err := submittedRequest(t).Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     checks,
		Basis:      snapshot,
		DecidedAt:  handlerClockAt,
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	path, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByAuthorizedDisposition {
		t.Fatalf("fixture waitingOn = %q (present=%v), want AUTHORIZED_DISPOSITION", path, present)
	}
	return waiting
}

func earlierDisposition(t *testing.T) domain.AuthorizedDisposition {
	t.Helper()
	disposition, err := domain.NewAuthorizedDisposition(domain.AuthorizedDispositionSpec{
		Choice:     domain.DisposeByCustomerSupplement,
		Authority:  mustValue(t, domain.NewDispositionAuthorityReference, "PC-DISPOSE-RULE-0"),
		Disposer:   mustValue(t, domain.NewDisposerReference, "CREDIT-OFFICER-0"),
		Reason:     mustValue(t, domain.NewDispositionReasonReference, "EARLIER_DISPOSITION"),
		Evidence:   mustValue(t, domain.NewDispositionEvidenceReference, "EVID-0"),
		DisposedAt: handlerClockAt,
	})
	if err != nil {
		t.Fatalf("new authorized disposition: %v", err)
	}
	return disposition
}

// dispositionAuthorizerDouble 按开关交回三值之一或错误，并记下被问了什么。授权引用只在`已授权`时带。
type dispositionAuthorizerDouble struct {
	outcome   ports.AuthorizationOutcome
	authority domain.DispositionAuthorityReference
	err       error
	asked     []ports.AuthorizedDispositionAuthorizationQuery
}

func (double *dispositionAuthorizerDouble) AuthorizeDisposition(
	_ context.Context,
	query ports.AuthorizedDispositionAuthorizationQuery,
) (ports.AuthorizedDispositionAuthorization, error) {
	double.asked = append(double.asked, query)
	if double.err != nil {
		return ports.AuthorizedDispositionAuthorization{}, double.err
	}
	answer := ports.AuthorizedDispositionAuthorization{Outcome: double.outcome}
	if double.outcome == ports.AuthorizationGranted {
		answer.Authority = double.authority
	}
	return answer, nil
}

// disposableRequestStore 交回夹具摆好的那一份委托，并留住被处置后保存的那一份。
type disposableRequestStore struct {
	t        *testing.T
	request  domain.ShipmentRequest
	saved    *domain.ShipmentRequest
	conflict bool
}

func (store *disposableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	return store.request, true, nil
}

func (store *disposableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *disposableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.conflict {
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

var (
	_ ports.AuthorizedDispositionAuthorizer = (*dispositionAuthorizerDouble)(nil)
	_ ports.ShipmentRequestRepository       = (*disposableRequestStore)(nil)
)
