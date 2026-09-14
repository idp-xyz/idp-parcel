package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var dispositionSentAt = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

type activeCaseViewDouble struct {
	active bool
	found  bool
	err    error
	calls  int
}

func (double *activeCaseViewDouble) CaseActive(
	_ context.Context,
	_ domain.CaseID,
) (bool, bool, error) {
	double.calls++
	if double.err != nil {
		return false, false, double.err
	}
	return double.active, double.found, nil
}

type dispositionStoreDouble struct {
	byID              map[domain.DispositionRequestID]*domain.DispositionRequest
	findErr           error
	saveErr           error
	saves             int
	supersessionSaves int
}

func newDispositionStore() *dispositionStoreDouble {
	return &dispositionStoreDouble{byID: map[domain.DispositionRequestID]*domain.DispositionRequest{}}
}

func (double *dispositionStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.DispositionRequestID,
) (*domain.DispositionRequest, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	request, found := double.byID[id]
	return request, found, nil
}

func (double *dispositionStoreDouble) FindCurrent(
	_ context.Context,
	_ domain.TenantID,
	caseID domain.CaseID,
	action domain.RequestedActionReference,
	scope domain.RequestScopeReference,
) (*domain.DispositionRequest, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	for _, request := range double.byID {
		if _, superseded := request.SupersededBy(); superseded {
			continue
		}
		if request.Case() == caseID && request.Action() == action && request.Scope() == scope {
			return request, true, nil
		}
	}
	return nil, false, nil
}

func (double *dispositionStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	request *domain.DispositionRequest,
) (ports.DispositionSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DispositionSaveOutcomeInvalid, double.saveErr
	}
	double.byID[request.ID()] = request
	double.saves++
	return ports.DispositionSaved, nil
}

func (double *dispositionStoreDouble) SaveSupersession(
	_ context.Context,
	_ domain.TenantID,
	prior, successor *domain.DispositionRequest,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.byID[prior.ID()] = prior
	double.byID[successor.ID()] = successor
	double.supersessionSaves++
	return nil
}

type dispositionIdentityDouble struct {
	next int
	err  error
}

func (double *dispositionIdentityDouble) NextDispositionRequestID(_ context.Context) (domain.DispositionRequestID, error) {
	if double.err != nil {
		return domain.DispositionRequestID{}, double.err
	}
	double.next++
	return domain.NewDispositionRequestID("disposition-" + string(rune('0'+double.next)))
}

type dispositionDownstreamDouble struct {
	intents []ports.DispositionHandoffIntent
	err     error
}

func (double *dispositionDownstreamDouble) HandOffDispositionRequest(
	_ context.Context,
	intent ports.DispositionHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type dispositionFixture struct {
	handler    *application.SendDispositionRequestHandler
	cases      *activeCaseViewDouble
	requests   *dispositionStoreDouble
	identities *dispositionIdentityDouble
	downstream *dispositionDownstreamDouble
}

func newDispositionFixture(t *testing.T) *dispositionFixture {
	t.Helper()
	fixture := &dispositionFixture{
		cases:      &activeCaseViewDouble{active: true, found: true},
		requests:   newDispositionStore(),
		identities: &dispositionIdentityDouble{},
		downstream: &dispositionDownstreamDouble{},
	}
	fixture.handler = application.NewSendDispositionRequestHandler(application.SendDispositionRequestDeps{
		Cases:      fixture.cases,
		Requests:   fixture.requests,
		Identities: fixture.identities,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: dispositionSentAt},
	})
	return fixture
}

func dispositionCommand(t *testing.T) application.SendDispositionRequestCommand {
	t.Helper()
	return application.SendDispositionRequestCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Case:     mustValue(t, domain.NewCaseID, "case-1"),
		Target:   domain.SourceNetworkRouting,
		Action:   mustValue(t, domain.NewRequestedActionReference, "REROUTE_REMAINING_JOURNEY"),
		Scope:    mustValue(t, domain.NewRequestScopeReference, "parcel-1/remaining"),
		Reason:   "route deviation confirmed",
		Evidence: mustValue(t, domain.NewRequestEvidenceReference, "evidence/route-deviation-1"),
	}
}

// Covers: CONTEXT「处置请求必须明确目标对象、请求动作、原因、证据、请求方、接收上下文
// 和期望时限」与生命周期「案件形成请求 → 待源上下文处理：固定目标、动作、范围、原因、
// 证据和时限」——活案件下各件齐即发送，意图由请求标识认领。点名 `AT-VE-079`「处置
// 请求发送→只形成发送结果，不声称目标动作完成」。
func TestAnActiveCaseSendsARequestWithItsEssentials(t *testing.T) {
	fixture := newDispositionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), dispositionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionRequestSent {
		t.Fatalf("outcome = %q, want SENT", result.Outcome())
	}
	request, present := result.Request()
	if !present {
		t.Fatal("a sent result carries no request")
	}
	if request.IntentVersion() != 1 {
		t.Fatalf("intent version = %d, want 1 for a first request", request.IntentVersion())
	}
	if _, judged := request.Judgment(); judged {
		t.Fatal("a freshly sent request already claims a source judgment")
	}
	if len(fixture.downstream.intents) != 1 ||
		fixture.downstream.intents[0].Request.ID() != request.ID() {
		t.Fatal("exactly one handoff intent claimed by the sent request was expected")
	}
}

// Covers: 派工受理约束「请求只能挂在活案件下」与 CONTEXT「案件范围缩小、改派、归并或
// 关闭前必须盘点全部未完成处置请求」——关闭后再挂新请求就是绕过盘点，案件不存在与已
// 关闭同答未受理。
func TestAClosedOrMissingCaseCannotHangARequest(t *testing.T) {
	for name, setup := range map[string]func(*dispositionFixture){
		"closed case":  func(fixture *dispositionFixture) { fixture.cases.active = false },
		"missing case": func(fixture *dispositionFixture) { fixture.cases.found = false },
	} {
		fixture := newDispositionFixture(t)
		setup(fixture)

		result, err := fixture.handler.Handle(context.Background(), dispositionCommand(t))
		if err != nil {
			t.Fatalf("handle %s: %v", name, err)
		}
		if result.Outcome() != application.DispositionNotAccepted {
			t.Fatalf("%s outcome = %q, want NOT_ACCEPTED", name, result.Outcome())
		}
		if fixture.requests.saves != 0 || fixture.identities.next != 0 {
			t.Fatalf("%s still wrote the store or consumed an identity", name)
		}
	}
}

// Covers: 派工幂等约束「幂等按（案件+动作+范围）——同一请求不重发」与 ADR-0043「重放
// 重发同一份意图」。
func TestTheSameCaseActionScopeDoesNotResendButResendsTheSameIntent(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstRequest, _ := first.Request()
	issuedBefore := fixture.identities.next

	replay, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.DispositionExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, _ := replay.Request()
	if replayed.ID() != firstRequest.ID() {
		t.Fatal("the replay did not return the request that already exists")
	}
	if fixture.identities.next != issuedBefore {
		t.Fatal("a replay consumed a new request identity")
	}
	if len(fixture.downstream.intents) != 2 ||
		fixture.downstream.intents[1].Request.ID() != firstRequest.ID() {
		t.Fatal("the replay must resend the same intent claimed by the existing request")
	}
}

// Covers: CONTEXT「处置请求取消或替代只改变未来意图，不撤销已经发生的源业务事实」与
// 生命周期「提出取消/替代意图……提出意图本身不表示旧请求已经停止」——后继更高意图
// 版本、前请求指向后继且原样保留、两份同一提交。
func TestSupersedingReplacesFutureIntentKeepingThePrior(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	prior, _ := first.Request()

	command := dispositionCommand(t)
	command.Scope = mustValue(t, domain.NewRequestScopeReference, "parcel-1/narrowed")
	command.Supersedes = prior.ID()
	superseded, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("supersede handle: %v", err)
	}

	if superseded.Outcome() != application.DispositionRequestSuperseded {
		t.Fatalf("outcome = %q, want SUPERSEDED", superseded.Outcome())
	}
	successor, _ := superseded.Request()
	if successor.IntentVersion() != prior.IntentVersion()+1 {
		t.Fatalf("successor version = %d, want prior+1", successor.IntentVersion())
	}
	pointed, linked := prior.SupersededBy()
	if !linked || pointed != successor.ID() {
		t.Fatal("the prior request does not point at its successor")
	}
	if fixture.requests.supersessionSaves != 1 {
		t.Fatal("the prior and its successor must cross the commit boundary together")
	}
	if fixture.downstream.intents[len(fixture.downstream.intents)-1].Request.ID() != successor.ID() {
		t.Fatal("the handoff intent must carry the successor")
	}
}

// Covers: 替代的幂等半边——已被替代的请求再被指名替代时读回赢家并重发其意图，不叠第二
// 层替代（原请求与其已有判断原样保留）。
func TestSupersedingAnAlreadySupersededRequestReturnsTheWinner(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	prior, _ := first.Request()

	supersede := dispositionCommand(t)
	supersede.Scope = mustValue(t, domain.NewRequestScopeReference, "parcel-1/narrowed")
	supersede.Supersedes = prior.ID()
	second, err := fixture.handler.Handle(ctx, supersede)
	if err != nil {
		t.Fatalf("supersede handle: %v", err)
	}
	successor, _ := second.Request()
	issuedBefore := fixture.identities.next

	again, err := fixture.handler.Handle(ctx, supersede)
	if err != nil {
		t.Fatalf("repeat supersede handle: %v", err)
	}

	if again.Outcome() != application.DispositionExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", again.Outcome())
	}
	winner, _ := again.Request()
	if winner.ID() != successor.ID() {
		t.Fatal("the repeat supersede did not read back the existing successor")
	}
	if fixture.identities.next != issuedBefore {
		t.Fatal("a repeat supersede consumed a new identity")
	}
}

// Covers: 生命周期「源上下文处理 → 接受、部分接受、拒绝或要求补充：结果只表达请求判断，
// 不证明实际执行」与派工「应答不可覆盖，重复应答拒」。
func TestASourceJudgmentIsRecordedOnce(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	sent, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("send handle: %v", err)
	}
	request, _ := sent.Request()

	recorded, err := fixture.handler.RecordSourceJudgment(ctx, application.RecordSourceJudgmentCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:  request.ID(),
		Judgment: domain.RequestAccepted,
		JudgedAt: dispositionSentAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("record judgment: %v", err)
	}
	if recorded.Outcome() != application.SourceJudgmentRecorded {
		t.Fatalf("outcome = %q, want JUDGMENT_RECORDED", recorded.Outcome())
	}

	again, err := fixture.handler.RecordSourceJudgment(ctx, application.RecordSourceJudgmentCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:  request.ID(),
		Judgment: domain.RequestRefused,
		JudgedAt: dispositionSentAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("repeat judgment: %v", err)
	}
	if again.Outcome() != application.SourceJudgmentAlreadyRecorded {
		t.Fatalf("outcome = %q, want JUDGMENT_ALREADY_RECORDED", again.Outcome())
	}
	judgment, _ := request.Judgment()
	if judgment != domain.RequestAccepted {
		t.Fatal("a repeated answer overwrote the recorded judgment")
	}
}

// Covers: 生命周期「待处理且到达受理有效期 → 已到期：该范围不得再被新接受」——届满后
// 的接受不入账是业务答案不是故障；拒绝不受该门限制，照常入账。点名 `AT-VE-092`
// 「待接受请求超过明确受理有效期→形成已到期；目标方不得再按旧请求新接受」。
func TestAnExpiredWindowRefusesALateAcceptanceButStillRecordsARefusal(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	command := dispositionCommand(t)
	command.AcceptanceWindow = dispositionSentAt.Add(time.Hour)
	sent, err := fixture.handler.Handle(ctx, command)
	if err != nil {
		t.Fatalf("send handle: %v", err)
	}
	request, _ := sent.Request()
	late := dispositionSentAt.Add(2 * time.Hour)

	expired, err := fixture.handler.RecordSourceJudgment(ctx, application.RecordSourceJudgmentCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:  request.ID(),
		Judgment: domain.RequestAccepted,
		JudgedAt: late,
	})
	if err != nil {
		t.Fatalf("late acceptance: %v", err)
	}
	if expired.Outcome() != application.AcceptanceWindowExpired {
		t.Fatalf("outcome = %q, want ACCEPTANCE_WINDOW_EXPIRED", expired.Outcome())
	}

	refused, err := fixture.handler.RecordSourceJudgment(ctx, application.RecordSourceJudgmentCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:  request.ID(),
		Judgment: domain.RequestRefused,
		JudgedAt: late,
	})
	if err != nil {
		t.Fatalf("late refusal: %v", err)
	}
	if refused.Outcome() != application.SourceJudgmentRecorded {
		t.Fatalf("outcome = %q, want JUDGMENT_RECORDED; a refusal is not gated by the window", refused.Outcome())
	}
}

// Covers: CONTEXT「目标上下文必须分别返回取消已接受、部分取消、已无法取消或拒绝取消」
// 与领域「未判断即无外部意图」——判断前谈不上取消，答过的不覆盖。
func TestACancellationAnswerNeedsAJudgmentFirstAndIsRecordedOnce(t *testing.T) {
	fixture := newDispositionFixture(t)
	ctx := context.Background()

	sent, err := fixture.handler.Handle(ctx, dispositionCommand(t))
	if err != nil {
		t.Fatalf("send handle: %v", err)
	}
	request, _ := sent.Request()
	answerAt := dispositionSentAt.Add(3 * time.Hour)

	premature, err := fixture.handler.RecordCancellationAnswer(ctx, application.RecordCancellationAnswerCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:    request.ID(),
		Answer:     domain.CancellationAcceptedByTarget,
		AnsweredAt: answerAt,
	})
	if err != nil {
		t.Fatalf("premature cancellation: %v", err)
	}
	if premature.Outcome() != application.DispositionNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED before any judgment", premature.Outcome())
	}

	if _, err := fixture.handler.RecordSourceJudgment(ctx, application.RecordSourceJudgmentCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:  request.ID(),
		Judgment: domain.RequestAccepted,
		JudgedAt: dispositionSentAt.Add(time.Hour),
	}); err != nil {
		t.Fatalf("record judgment: %v", err)
	}

	recorded, err := fixture.handler.RecordCancellationAnswer(ctx, application.RecordCancellationAnswerCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:    request.ID(),
		Answer:     domain.PartiallyCancelled,
		AnsweredAt: answerAt,
	})
	if err != nil {
		t.Fatalf("record cancellation: %v", err)
	}
	if recorded.Outcome() != application.CancellationAnswerRecorded {
		t.Fatalf("outcome = %q, want CANCELLATION_RECORDED", recorded.Outcome())
	}

	again, err := fixture.handler.RecordCancellationAnswer(ctx, application.RecordCancellationAnswerCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:    request.ID(),
		Answer:     domain.CancellationRefusedByTarget,
		AnsweredAt: answerAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("repeat cancellation: %v", err)
	}
	if again.Outcome() != application.CancellationAnswerAlreadyRecorded {
		t.Fatalf("outcome = %q, want CANCELLATION_ALREADY_RECORDED", again.Outcome())
	}
	answer, _ := request.CancellationOutcome()
	if answer != domain.PartiallyCancelled {
		t.Fatal("a repeated answer overwrote the recorded cancellation outcome")
	}
}

// Covers: 未决语义——案件视图与请求库调不通分别停在各自原因，不虚构受理也不虚构幂等。
func TestUnavailableDependenciesAreUndecidedUnderTheirOwnReasons(t *testing.T) {
	caseView := newDispositionFixture(t)
	caseView.cases.err = errors.New("case view unavailable")
	result, err := caseView.handler.Handle(context.Background(), dispositionCommand(t))
	if err != nil {
		t.Fatalf("handle with case view down: %v", err)
	}
	if result.Outcome() != application.DispositionUndecided ||
		result.UndecidedReason() != application.DispositionCaseViewUnavailable {
		t.Fatalf("result = %q/%q, want UNDECIDED/CASE_VIEW_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}

	store := newDispositionFixture(t)
	store.requests.findErr = errors.New("store unavailable")
	result, err = store.handler.Handle(context.Background(), dispositionCommand(t))
	if err != nil {
		t.Fatalf("handle with store down: %v", err)
	}
	if result.Outcome() != application.DispositionUndecided ||
		result.UndecidedReason() != application.DispositionStoreUnavailable {
		t.Fatalf("result = %q/%q, want UNDECIDED/DISPOSITION_STORE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」。
func TestAFailedDispositionHandoffLeavesAResumableReference(t *testing.T) {
	fixture := newDispositionFixture(t)
	fixture.downstream.err = errors.New("downstream unavailable")

	result, err := fixture.handler.Handle(context.Background(), dispositionCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionRequestSent {
		t.Fatalf("outcome = %q, want SENT; a failed handoff must not rewrite the request", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("a failed handoff left no resumable reference")
	}
}

// Covers: 受理半边——各件缺一即未受理，不读任何依赖。
func TestACommandMissingAnEssentialIsNotAccepted(t *testing.T) {
	fixture := newDispositionFixture(t)

	command := dispositionCommand(t)
	command.Evidence = domain.RequestEvidenceReference{}
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.DispositionNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
	if fixture.cases.calls != 0 {
		t.Fatal("an unaccepted command still asked the case view")
	}
}
