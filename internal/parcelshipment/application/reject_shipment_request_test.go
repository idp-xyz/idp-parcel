package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 8「授权角色可以依据结构化原因主动拒绝」与 AT-PS-034 后半 —— 获授权
// 时形成拒绝，且所采用的授权引用来自 party-commercial 而不是命令自带。
func TestAnAuthorizedOperatorFormsAnActiveRejection(t *testing.T) {
	fixture := newRejectionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ActiveRejectionFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present {
		t.Fatal("a formed rejection carried no decision")
	}
	active, isActive := decision.ActiveRejection()
	if !isActive || active.Authority().String() != "PC-REJECT-ROLE-1" {
		t.Fatalf("authority = %#v; the adopted authorization must be the one party-commercial issued", active)
	}
	if fixture.requests.saved == nil {
		t.Fatal("a rejection was formed but never saved")
	}
}

// Covers: CONTEXT「普通备注、口头意见或未经授权的操作不能形成拒绝事实」— 未获授权时不形成
// 决定，也不消耗一个决定标识。未获授权与未决分开：前者续办也补不出授权。
func TestAnUnauthorizedOperatorFormsNothing(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.authorizer.granted = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ActiveRejectionNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	if result.State() == domain.ShipmentRequestRejected {
		t.Fatal("an unauthorized operator rejected the request")
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("an unauthorized attempt recorded a decision")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision IDs; an unauthorized attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("an unauthorized attempt saved the request")
	}
}

// Covers: UC-PS-001 AT-PS-035「接受提交未成立时按原关联释放冻结」— 主动拒绝同样是接受确定
// 未成立，冻结不能因为拒绝出自运营之手就留在原处。
func TestAnActiveRejectionReleasesTheFreeze(t *testing.T) {
	fixture := newRejectionFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want exactly 1", fixture.release.calls)
	}
	if fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("released %q, want the original control association SAC-1", fixture.release.controlResultID)
	}
}

// Covers: CONTEXT「接受或拒绝提交后，另一方只能读取既有结果，不能追加相反决定」— 撞上一个
// 已成立的接受时交回那一个，不报错也不覆盖。
func TestAnActiveRejectionAgainstADecidedVersionReadsTheExistingDecision(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.requests.decided = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	decision, present := result.AcceptanceDecision()
	if !present || decision.DecisionID().String() != "decision-0" {
		t.Fatalf("decision = %#v; the later attempt must read the earlier decision, not form its own", decision)
	}
	if fixture.requests.saved != nil {
		t.Fatal("a late rejection saved over an already decided version")
	}
	if fixture.release.calls != 0 {
		t.Fatal("a late rejection ran the compensation of a decision it did not form")
	}
}

// Covers: UC-PS-001 结果语义`尚未决定` — 授权服务答不出是依赖故障，与「不授权」用不同结果和
// 不同原因：前者要重试，后者要去补授权。
func TestAnUnavailableAuthorizerIsUndecidedRatherThanUnauthorized(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.authorizer.err = errors.New("authorization service unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ActiveRejectionUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.RejectionAuthorityUnavailable {
		t.Fatalf("pending reason = %q, want REJECTION_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undecided round left no continuation")
	}
}

// Covers: UC-PS-001 AT-PS-035「释放失败…保持补偿未决」与 CONTEXT「撤回提交和释放属于可补偿
// 编排」— 主动拒绝这一侧同样不因释放失败回滚已经越过提交边界的决定。
func TestAFailedReleaseKeepsTheActiveRejectionAndLeavesCompensationPending(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.release.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a failed release rolled back a rejection that already crossed the boundary", result.State())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("a failed release left no continuation to resume the compensation")
	}
}

// Covers: UC-PS-001 步骤 7 与 AT-PS-035 — 读不回已记录的判断就不发释放：不知道原关联就发，
// settlement-accounting 无从认领是哪一笔冻结。本轮把补偿留成可续办。
func TestAnActiveRejectionWithUnreadableJudgmentsSendsNoRelease(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.judgments.err = errors.New("recorded judgments unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; an unreadable judgment store rolled back a formed rejection", result.State())
	}
	if fixture.release.calls != 0 {
		t.Fatal("a release was sent without knowing which control association it would settle")
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("an unresolved compensation left no continuation")
	}
}

// Covers: judgmentContinuation 的派生约定「同一范围因同一原因停滞时拿到的引用始终相同」—
// 同一笔冻结释放失败，拒绝出自规则还是出自授权角色都必须给出同一个引用，否则续办方得先知道
// 是哪条路径形成的拒绝才查得回原次尝试，而它没有理由知道。原因参与派生则反过来验证：换一个
// 原因必须换一个引用，否则两类补偿会挤在同一个引用上。
func TestBothRejectionPathsDeriveTheSameCompensationReference(t *testing.T) {
	automatic := newDecisionFixture(t)
	automatic.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	automatic.release.err = errors.New("settlement authority unavailable")

	byRule, err := automatic.handler.Handle(context.Background(), automatic.command(t))
	if err != nil {
		t.Fatalf("handle the rule-formed rejection: %v", err)
	}

	active := newRejectionFixture(t)
	active.release.err = errors.New("settlement authority unavailable")

	byAuthority, err := active.handler.Handle(context.Background(), active.command(t))
	if err != nil {
		t.Fatalf("handle the active rejection: %v", err)
	}

	if byRule.CompensationReference().String() != byAuthority.CompensationReference().String() {
		t.Fatalf(
			"rule-formed %q, authority-formed %q; one pending compensation must carry one continuation whichever path formed the rejection",
			byRule.CompensationReference(), byAuthority.CompensationReference(),
		)
	}

	unreadable := newRejectionFixture(t)
	unreadable.judgments.err = errors.New("recorded judgments unavailable")

	byMissingJudgments, err := unreadable.handler.Handle(context.Background(), unreadable.command(t))
	if err != nil {
		t.Fatalf("handle the rejection with unreadable judgments: %v", err)
	}

	if byMissingJudgments.CompensationReference().String() == byAuthority.CompensationReference().String() {
		t.Fatalf(
			"both stalls derived %q; a continuation that ignores the reason cannot tell an unresolved association from a failed release",
			byAuthority.CompensationReference(),
		)
	}
}

type rejectionFixture struct {
	handler    *application.RejectShipmentRequestHandler
	requests   *rejectableRequestStore
	authorizer *rejectionAuthorizerDouble
	judgments  *recordedJudgmentsDouble
	release    *controlReleaseDouble
	identities *countingIdentityFactory
}

func newRejectionFixture(t *testing.T) *rejectionFixture {
	t.Helper()
	value := &rejectionFixture{
		requests:   &rejectableRequestStore{t: t},
		authorizer: &rejectionAuthorizerDouble{t: t, granted: true},
		judgments:  &recordedJudgmentsDouble{t: t, controlOutcome: domain.FinancialControlHeld},
		release:    &controlReleaseDouble{},
		identities: &countingIdentityFactory{t: t},
	}
	value.handler = application.NewRejectShipmentRequestHandler(application.RejectShipmentRequestDeps{
		Requests:   value.requests,
		Authorizer: value.authorizer,
		Judgments:  value.judgments,
		Recorder:   &judgmentRequestStore{},
		Release:    value.release,
		Identities: value.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return value
}

func (value *rejectionFixture) command(t *testing.T) application.RejectShipmentRequestCommand {
	t.Helper()
	return application.RejectShipmentRequestCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Decider:           mustValue(t, domain.NewDeciderReference, "OPERATOR-1"),
		Reason:            mustValue(t, domain.NewRejectionReasonReference, "OPERATOR_DECLINED_SERVICE"),
		Evidence:          mustValue(t, domain.NewRejectionEvidenceReference, "EVID-1"),
	}
}

// rejectionAuthorizerDouble 用零值引用表示未授权，与端口约定一致：未授权是业务答案，不是错误。
type rejectionAuthorizerDouble struct {
	t       *testing.T
	granted bool
	err     error
}

func (double *rejectionAuthorizerDouble) AuthorizeActiveRejection(
	_ context.Context,
	_ ports.ActiveRejectionAuthorizationQuery,
) (domain.RejectionAuthorityReference, error) {
	double.t.Helper()
	if double.err != nil {
		return domain.RejectionAuthorityReference{}, double.err
	}
	if !double.granted {
		return domain.RejectionAuthorityReference{}, nil
	}
	return mustValue(double.t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-1"), nil
}

// rejectableRequestStore 按 decided/withdrawn 交回一份`已提交`、一份已经决定或一份已经撤回的
// 委托，用来验证后到的请求只能读取既有结果。
type rejectableRequestStore struct {
	t         *testing.T
	decided   bool
	withdrawn bool
	err       error
	saved     *domain.ShipmentRequest
}

func (store *rejectableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	if store.err != nil {
		return domain.ShipmentRequest{}, false, store.err
	}
	request := submittedRequest(store.t)
	if store.withdrawn {
		// 同样让它经领域真的撤一次：假状态挡不住 WithdrawByCustomer，也说明不了问题。
		gone, err := request.WithdrawByCustomer(domain.WithdrawalSpec{
			DecisionID: mustValue(store.t, domain.NewAcceptanceDecisionID, "decision-0"),
			Authority:  mustValue(store.t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-0"),
			Requester:  mustValue(store.t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-0"),
			Reason:     mustValue(store.t, domain.NewWithdrawalReasonReference, "EARLIER_WITHDRAWAL"),
			DecidedAt:  handlerClockAt,
		})
		if err != nil {
			store.t.Fatalf("form the earlier withdrawal: %v", err)
		}
		return gone, true, nil
	}
	if !store.decided {
		return request, true, nil
	}
	// 主动拒绝要撞的是一个真正越过提交边界的决定，所以这里让委托先经领域形成一次接受，
	// 而不是造一个「看起来已接受」的假状态——假状态挡不住 RejectByAuthority 也说明不了问题。
	accepted, err := request.RejectByAuthority(domain.ActiveRejectionSpec{
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
	return accepted, true, nil
}

func (store *rejectableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) error {
	return nil
}

func (store *rejectableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	store.saved = &request
	return nil
}

type countingIdentityFactory struct {
	t      *testing.T
	issued int
}

func (factory *countingIdentityFactory) NextAcceptanceDecisionID(
	_ context.Context,
) (domain.AcceptanceDecisionID, error) {
	factory.t.Helper()
	factory.issued++
	return mustValue(factory.t, domain.NewAcceptanceDecisionID, "decision-1"), nil
}

var (
	_ ports.ShipmentRequestRepository  = (*rejectableRequestStore)(nil)
	_ ports.ActiveRejectionAuthorizer  = (*rejectionAuthorizerDouble)(nil)
	_ ports.AcceptanceDecisionIdentity = (*countingIdentityFactory)(nil)
)
