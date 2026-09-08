package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 AT-PS-034「按证据完成人工复核」与 CONTEXT 等待人工复核态——完成落库，
// 留痕携带授权、实际复核方与证据三引用，时间取编排时钟而不是调用方自报。
//
// 授权引用来自 party-commercial 的答复（所采用的授权规则版本），不来自命令：命令根本没有那
// 一格可填。此前它由 Intake 整组注入，没人问过 PC——票 wiring-baseline-remainder/04 接的就是这条。
func TestACompletedManualReviewIsRecordedOnTheCurrentTask(t *testing.T) {
	fixture := newReviewCompletionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewCompletionRecorded {
		t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
	}
	completion, present := result.Completion()
	if !present {
		t.Fatal("a recorded completion carried no completion")
	}
	if completion.Authority().String() != "PC-REVIEW-RULE-7/v2" ||
		completion.Reviewer().String() != "OPERATOR-1" ||
		completion.Evidence().String() != "EVID-R1" {
		t.Fatalf("completion = %#v; authority must be the grant version PC adopted, reviewer and evidence the command's", completion)
	}
	if !completion.CompletedAt().Equal(handlerClockAt) {
		t.Fatalf("completedAt = %v, want the orchestration clock %v", completion.CompletedAt(), handlerClockAt)
	}
	if fixture.requests.saved == nil {
		t.Fatal("a recorded completion never reached the repository")
	}
	saved, done := fixture.requests.saved.AcceptanceDecisionTask().ManualReviewCompletion()
	if !done || saved.Reviewer().String() != "OPERATOR-1" {
		t.Fatalf("saved completion = %#v; the aggregate that was saved must carry the completion", saved)
	}
}

// Covers: CONTEXT「复核完成本身不形成决定，决定仍由判断任务按适用规则形成」——完成落库后
// 委托仍是`已提交`，本编排不越权替判断任务作决定。
func TestACompletedReviewFormsNoDecision(t *testing.T) {
	fixture := newReviewCompletionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; recording a review completion must not decide the request", result.State())
	}
	if _, formed := fixture.requests.saved.AcceptanceDecision(); formed {
		t.Fatal("recording a review completion formed an acceptance decision")
	}
}

// Covers: 域规则「同一提交版本只接受一次完成」在应用层的落点——重复提交读回既有留痕，
// 不覆盖也不报错重试。
func TestASecondCompletionReadsTheExistingOne(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.requests.reviewed = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewCompletionAlreadyDone {
		t.Fatalf("outcome = %q, want ALREADY_COMPLETED", result.Outcome())
	}
	existing, present := result.Completion()
	if !present || existing.Reviewer().String() != "OPERATOR-0" {
		t.Fatalf("completion = %#v; the second submission must read the first completion", existing)
	}
	if fixture.requests.saved != nil {
		t.Fatal("a duplicate completion overwrote the first one")
	}
}

// Covers: CONTEXT「判断任务 → 已完成：只有接受或拒绝决定已经越过提交边界时完成」+
// 域规则「决定越界后拒绝补录」——已决委托上补一次完成要交回既有决定，不是安静吞掉。
func TestACompletionAgainstADecidedRequestReadsTheDecision(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.requests.decided = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewTaskAlreadyClosed {
		t.Fatalf("outcome = %q, want TASK_ALREADY_CLOSED", result.Outcome())
	}
	if _, present := result.AcceptanceDecision(); !present {
		t.Fatal("a closed task carried no decision to read")
	}
	if fixture.requests.saved != nil {
		t.Fatal("a completion against a decided request saved something")
	}
}

// Covers: 复核对象是明确提交版本——新版本换代后，旧版本的复核不能签到新任务头上；答案是
// `版本已换代`并报出当前版本，不是把完成记下也不是报错。
func TestACompletionForASupersededVersionIsRefused(t *testing.T) {
	fixture := newReviewCompletionFixture(t)

	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-0")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewVersionSuperseded {
		t.Fatalf("outcome = %q, want VERSION_SUPERSEDED", result.Outcome())
	}
	if result.CurrentVersion().String() != "version-1" {
		t.Fatalf("current version = %q, want version-1 so the operator can re-read the queue", result.CurrentVersion())
	}
	if fixture.requests.saved != nil {
		t.Fatal("a superseded completion was recorded onto the new task")
	}
}

// Covers: ADR-0031 写入代数——版本冲突是业务答案不是故障，调用方重读再重放。
func TestAConflictedSaveReportsTheConflict(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.requests.conflicted = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewCompletionConflict {
		t.Fatalf("outcome = %q, want REVISION_CONFLICT", result.Outcome())
	}
}

// Covers: 与形成决定那一步同一判断——指名一份查不到的委托是调用方的错，上抛而不是造一个
// 业务取值；HTTP 侧与其余未形成答案一并 5xx，维持统一不可见结果。
func TestACompletionForAMissingRequestIsACallerError(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.requests.missing = true

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, domain.ErrInvalidShipmentRequest) {
		t.Fatalf("err = %v, want ErrInvalidShipmentRequest", err)
	}
}

// Covers: UC-PS-001 AT-PS-034「只由规则授权的角色按证据完成复核」——编排问的是这位复核人、
// 凭这份证据、为这份提交版本，且不自带授权引用（自带一个等于自己给自己签字）。
func TestTheReviewAuthorityIsAskedAboutTheReviewerAndTheVersion(t *testing.T) {
	fixture := newReviewCompletionFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	asked := fixture.authorizer.asked
	if len(asked) != 1 {
		t.Fatalf("authorizer asked %d times, want exactly once", len(asked))
	}
	query := asked[0]
	if query.Reviewer.String() != "OPERATOR-1" || query.Evidence.String() != "EVID-R1" {
		t.Fatalf("query = %+v; the authority must be asked about the command's reviewer and evidence", query)
	}
	if query.SubmissionVersion.String() != "version-1" || query.ShipmentRequestID.String() != "request-1" {
		t.Fatalf("query = %+v; the authority must be asked about the version being reviewed", query)
	}
}

// Covers: UC-PC-003 结果表`不允许`→「停止该动作，按业务否定处置」——权威已就该范围表过态，
// 本复核人不在其内。它是确定的业务答案不是未决：什么都不落库，也不报错让人重试。
func TestAnUnauthorizedReviewerRecordsNothing(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.authorizer.outcome = ports.AuthorizationRefused

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	if _, present := result.Completion(); present {
		t.Fatal("an unauthorized completion still carried a completion")
	}
	if fixture.requests.saved != nil {
		t.Fatal("an unauthorized reviewer's completion reached the repository")
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; refusing the reviewer must not touch the request", result.State())
	}
}

// Covers: UC-PC-003 结果表`授权规则未配置`→「保持未决，等租户登记规则」，并明禁压进`不允许`
// ——首发期没有租户，每一次询问都落在这一格；把它说成「你无权复核」是对一个尚未配置的产品
// 说假话（红线：实例半边留空并拒绝默认值）。
func TestAReviewWithoutAuthorityRulesStaysUndecided(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.authorizer.outcome = ports.AuthorizationRulesNotConfigured

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ManualReviewAuthorityRulesNotConfigured {
		t.Fatalf("outcome = %q, want AUTHORITY_RULES_NOT_CONFIGURED", result.Outcome())
	}
	if result.Outcome() == application.ManualReviewNotAuthorized {
		t.Fatal("未配置被压成了不允许")
	}
	if fixture.requests.saved != nil {
		t.Fatal("a completion without any authority rule reached the repository")
	}
}

// Covers: UC-PC-003 结果表`未形成`→「保持未决，重试或续办」，不冒充不允许或未配置——权威
// 答不出是错误，与本编排其余未形成答案一并上抛（HTTP 5xx NO_ANSWER_FORMED）。
func TestAnUnavailableReviewAuthorityFormsNoAnswer(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.authorizer.err = errors.New("权威不可读")

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, fixture.authorizer.err) {
		t.Fatalf("err = %v, want the wrapped authority failure", err)
	}
	if fixture.requests.saved != nil {
		t.Fatal("a completion whose authority could not be read reached the repository")
	}
}

// Covers: 授权先于一切写动作，也先于任何领域判断——同 RejectShipmentRequestHandler 的顺序。
// 一个未获授权的复核人对着已换代的版本提交，得到的是`不允许`而不是`版本已换代`：后者是给
// 有权复核的人重读队列用的续办提示，不该先于授权答给任何人。
func TestAuthorizationPrecedesTheVersionCheck(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.authorizer.outcome = ports.AuthorizationRefused

	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-0")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ManualReviewNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED before any version answer", result.Outcome())
	}
}

type reviewCompletionFixture struct {
	handler    *application.CompleteManualReviewHandler
	requests   *reviewableRequestStore
	authorizer *reviewAuthorizerDouble
}

func newReviewCompletionFixture(t *testing.T) *reviewCompletionFixture {
	t.Helper()
	fixture := &reviewCompletionFixture{
		requests: &reviewableRequestStore{t: t},
		authorizer: &reviewAuthorizerDouble{
			outcome:   ports.AuthorizationGranted,
			authority: mustValue(t, domain.NewReviewAuthorityReference, "PC-REVIEW-RULE-7/v2"),
		},
	}
	fixture.handler = application.NewCompleteManualReviewHandler(application.CompleteManualReviewDeps{
		Requests:   fixture.requests,
		Authorizer: fixture.authorizer,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return fixture
}

func (fixture *reviewCompletionFixture) command(t *testing.T) application.CompleteManualReviewCommand {
	t.Helper()
	return application.CompleteManualReviewCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Reviewer:          mustValue(t, domain.NewReviewerReference, "OPERATOR-1"),
		Evidence:          mustValue(t, domain.NewReviewEvidenceReference, "EVID-R1"),
	}
}

// reviewAuthorizerDouble 按开关交回三值之一或错误，并记下被问了什么。授权引用只在`已授权`
// 时带——与端口契约同形，否则测不出编排有没有把一个没拿到的引用签进留痕。
type reviewAuthorizerDouble struct {
	outcome   ports.AuthorizationOutcome
	authority domain.ReviewAuthorityReference
	err       error
	asked     []ports.ManualReviewAuthorizationQuery
}

func (double *reviewAuthorizerDouble) AuthorizeManualReview(
	_ context.Context,
	query ports.ManualReviewAuthorizationQuery,
) (ports.ManualReviewAuthorization, error) {
	double.asked = append(double.asked, query)
	if double.err != nil {
		return ports.ManualReviewAuthorization{}, double.err
	}
	answer := ports.ManualReviewAuthorization{Outcome: double.outcome}
	if double.outcome == ports.AuthorizationGranted {
		answer.Authority = double.authority
	}
	return answer, nil
}

var _ ports.ManualReviewAuthorizer = (*reviewAuthorizerDouble)(nil)

// reviewableRequestStore 按开关交回一份`已提交`、已完成过复核、已决或查不到的委托。已决
// 与已完成都真经领域形成，理由同 rejectableRequestStore：假状态挡不住领域门，也说明不了问题。
type reviewableRequestStore struct {
	t          *testing.T
	reviewed   bool
	decided    bool
	missing    bool
	conflicted bool
	saved      *domain.ShipmentRequest
}

func (store *reviewableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
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
	request := submittedRequest(store.t)
	if store.reviewed {
		earlier, err := domain.NewManualReviewCompletion(domain.ManualReviewCompletionSpec{
			Authority:   mustValue(store.t, domain.NewReviewAuthorityReference, "PC-REVIEW-ROLE-0"),
			Reviewer:    mustValue(store.t, domain.NewReviewerReference, "OPERATOR-0"),
			Evidence:    mustValue(store.t, domain.NewReviewEvidenceReference, "EVID-0"),
			CompletedAt: handlerClockAt,
		})
		if err != nil {
			store.t.Fatalf("new earlier completion: %v", err)
		}
		completed, err := request.CompleteManualReview(earlier)
		if err != nil {
			store.t.Fatalf("complete the earlier review: %v", err)
		}
		return completed, true, nil
	}
	return request, true, nil
}

func (store *reviewableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *reviewableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.conflicted {
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

var _ ports.ShipmentRequestRepository = (*reviewableRequestStore)(nil)
