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
	if completion.Authority().String() != "PC-REVIEW-ROLE-1" ||
		completion.Reviewer().String() != "OPERATOR-1" ||
		completion.Evidence().String() != "EVID-R1" {
		t.Fatalf("completion = %#v; the three references must be the adopted ones", completion)
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

type reviewCompletionFixture struct {
	handler  *application.CompleteManualReviewHandler
	requests *reviewableRequestStore
}

func newReviewCompletionFixture(t *testing.T) *reviewCompletionFixture {
	t.Helper()
	fixture := &reviewCompletionFixture{
		requests: &reviewableRequestStore{t: t},
	}
	fixture.handler = application.NewCompleteManualReviewHandler(application.CompleteManualReviewDeps{
		Requests: fixture.requests,
		Clock:    fixedClock{at: handlerClockAt},
	})
	return fixture
}

func (fixture *reviewCompletionFixture) command(t *testing.T) application.CompleteManualReviewCommand {
	t.Helper()
	return application.CompleteManualReviewCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Authority:         mustValue(t, domain.NewReviewAuthorityReference, "PC-REVIEW-ROLE-1"),
		Reviewer:          mustValue(t, domain.NewReviewerReference, "OPERATOR-1"),
		Evidence:          mustValue(t, domain.NewReviewEvidenceReference, "EVID-R1"),
	}
}

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
