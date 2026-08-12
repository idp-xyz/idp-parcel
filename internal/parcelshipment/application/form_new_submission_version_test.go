package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// supplementableRequestStore 交回一份`已提交`委托并把保存黏住：重放要跨两次调用才谈得上，
// 每次交回干净的一份，第二次调用就看不见第一次的换代。
type supplementableRequestStore struct {
	t            *testing.T
	decided      bool
	err          error
	saveErr      error
	saveConflict bool
	saved        *domain.ShipmentRequest
}

func (store *supplementableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	if store.err != nil {
		return domain.ShipmentRequest{}, false, store.err
	}
	if store.saved != nil {
		return *store.saved, true, nil
	}
	request := submittedRequest(store.t)
	if store.decided {
		// 让委托真经领域越过决定边界（先例：rejectableRequestStore）：假状态挡不住
		// FormNewSubmissionVersion 的短路，也证明不了编排读回的是那份决定。
		decided, err := request.RejectByAuthority(domain.ActiveRejectionSpec{
			DecisionID: mustValue(store.t, domain.NewAcceptanceDecisionID, "decision-0"),
			Authority:  mustValue(store.t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-0"),
			Decider:    mustValue(store.t, domain.NewDeciderReference, "OPERATOR-0"),
			Reason:     mustValue(store.t, domain.NewRejectionReasonReference, "EARLIER_DECISION"),
			Evidence:   mustValue(store.t, domain.NewRejectionEvidenceReference, "EVID-0"),
			DecidedAt:  handlerClockAt,
		})
		if err != nil {
			store.t.Fatalf("decide fixture: %v", err)
		}
		return decided, true, nil
	}
	return request, true, nil
}

func (store *supplementableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *supplementableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.saveErr != nil {
		return 0, store.saveErr
	}
	if store.saveConflict {
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

// supplementIdentityFactory 从 2 号起签发：夹具里的委托已经用掉 version-1/task-1，工厂
// 重号会被领域的重号闸拦下，那不是本编排要测的东西。
type supplementIdentityFactory struct {
	t              *testing.T
	err            error
	versionsIssued int
	tasksIssued    int
}

func (factory *supplementIdentityFactory) NextSubmissionVersionID(
	_ context.Context,
) (domain.SubmissionVersionID, error) {
	factory.t.Helper()
	if factory.err != nil {
		return domain.SubmissionVersionID{}, factory.err
	}
	factory.versionsIssued++
	return mustValue(factory.t, domain.NewSubmissionVersionID, "version-2"), nil
}

func (factory *supplementIdentityFactory) NextAcceptanceDecisionTaskID(
	_ context.Context,
) (domain.AcceptanceDecisionTaskID, error) {
	factory.t.Helper()
	if factory.err != nil {
		return domain.AcceptanceDecisionTaskID{}, factory.err
	}
	factory.tasksIssued++
	return mustValue(factory.t, domain.NewAcceptanceDecisionTaskID, "task-2"), nil
}

var (
	_ ports.ShipmentRequestRepository = (*supplementableRequestStore)(nil)
	_ ports.SubmissionIdentityFactory = (*supplementIdentityFactory)(nil)
)

type supplementFixture struct {
	handler    *application.FormNewSubmissionVersionHandler
	sources    *sourceRepositoryDouble
	requests   *supplementableRequestStore
	identities *supplementIdentityFactory
}

func newSupplementFixture(t *testing.T) *supplementFixture {
	t.Helper()
	fixture := &supplementFixture{
		sources: &sourceRepositoryDouble{
			records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{},
			record:  func(string) {},
		},
		requests:   &supplementableRequestStore{t: t},
		identities: &supplementIdentityFactory{t: t},
	}
	fixture.handler = application.NewFormNewSubmissionVersionHandler(application.FormNewSubmissionVersionDeps{
		Sources:    fixture.sources,
		Requests:   fixture.requests,
		Identities: fixture.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return fixture
}

func (fixture *supplementFixture) command(t *testing.T) application.FormNewSubmissionVersionCommand {
	t.Helper()
	return application.FormNewSubmissionVersionCommand{
		Identity:           sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		SupplementIdentity: sourceIdentity(t, "tenant-1", "customer-1", "source-a", "supplement-key-1"),
		PayloadDigest:      mustValue(t, domain.NewPayloadDigest, "supplement-digest-1"),
		OccurredAt:         handlerClockAt,
		ReceivedAt:         handlerClockAt,
		ShipmentRequestID:  mustValue(t, domain.NewShipmentRequestID, "request-1"),
		BasisVersion:       mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	}
}

// Covers: `AT-PS-036` 第一支的编排半边（域层半边见 ADR-0045 与领域用例）——受控补充落库：
// 新版本成为当前版本、旧版本与停止的旧任务留在历史上、新任务随新版本运行。
func TestASupplementOnASubmittedRequestFormsANewVersion(t *testing.T) {
	fixture := newSupplementFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SupplementRecorded {
		t.Fatalf("outcome = %q reason = %q, want SUPPLEMENT_RECORDED", result.Outcome(), result.PendingReason())
	}
	version, present := result.Version()
	if !present || version.VersionID().String() != "version-2" {
		t.Fatalf("version = %#v present = %v, want the newly issued version-2", version, present)
	}
	if fixture.requests.saved == nil {
		t.Fatal("换代没有落库")
	}
	saved := *fixture.requests.saved
	if saved.CurrentSubmissionVersion().VersionID().String() != "version-2" {
		t.Fatalf("current version = %q, want version-2", saved.CurrentSubmissionVersion().VersionID())
	}
	priors := saved.PriorSubmissionVersions()
	if len(priors) != 1 || priors[0].VersionID().String() != "version-1" {
		t.Fatalf("prior versions = %#v; 旧版本必须保留", priors)
	}
	task := saved.AcceptanceDecisionTask()
	if task.SubmissionVersionID().String() != "version-2" || task.IsComplete() || task.IsStopped() {
		t.Fatalf("task = %#v; 新任务必须钉住新版本且在运行", task)
	}
	if fixture.identities.versionsIssued != 1 || fixture.identities.tasksIssued != 1 {
		t.Fatalf("issued versions=%d tasks=%d, want one each（实测于本轮）",
			fixture.identities.versionsIssued, fixture.identities.tasksIssued)
	}
}

// Covers: CONTEXT 指纹句「同一完整来源请求身份与相同规范化内容摘要返回已有结果」在补充
// 一侧——重放交回原版本，不再签发身份、不再换代；携带不同内容则是请求冲突。
func TestASupplementReplayReturnsTheOriginalVersionWithoutReforming(t *testing.T) {
	fixture := newSupplementFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	issuedBefore := fixture.identities.versionsIssued

	replay, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.SupplementAlreadyHandled {
		t.Fatalf("outcome = %q, want SUPPLEMENT_ALREADY_HANDLED", replay.Outcome())
	}
	version, present := replay.Version()
	if !present || version.VersionID().String() != "version-2" {
		t.Fatalf("version = %#v present = %v, want the original version-2", version, present)
	}
	if fixture.identities.versionsIssued != issuedBefore {
		t.Fatal("重放又签发了版本身份——那是第二次换代，不是同一次的重试")
	}

	t.Run("a different payload under the same identity is a conflict", func(t *testing.T) {
		conflicting := fixture.command(t)
		conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "supplement-digest-2")

		result, err := fixture.handler.Handle(context.Background(), conflicting)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.SupplementSourceConflict {
			t.Fatalf("outcome = %q, want SUPPLEMENT_SOURCE_CONFLICT", result.Outcome())
		}
	})
}

// Covers: 「资料版本提交顺序由明确的领域顺序、基础版本关系裁决」在提交版本一层——基准
// 过期是明确答案：不静默换代、不消耗身份。
func TestAStaleBasisIsAnsweredWithoutForming(t *testing.T) {
	fixture := newSupplementFixture(t)
	command := fixture.command(t)
	command.BasisVersion = mustValue(t, domain.NewSubmissionVersionID, "version-0")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SupplementBasisStale {
		t.Fatalf("outcome = %q, want SUPPLEMENT_BASIS_STALE", result.Outcome())
	}
	if fixture.identities.versionsIssued != 0 {
		t.Fatal("基准过期仍消耗了版本身份")
	}
	if fixture.requests.saved != nil {
		t.Fatal("基准过期仍换了代")
	}
}

// Covers: UC-PS-001「删除、拆分或重组成员必须建立关联新委托」——成员集合变更是确定的
// 业务拒绝，不形成版本也不落库。
func TestAChangedMemberSetIsRejectedTowardALinkedNewRequest(t *testing.T) {
	fixture := newSupplementFixture(t)
	command := fixture.command(t)
	command.DeclaredParcelIDs = append(command.DeclaredParcelIDs,
		mustValue(t, domain.NewDeclaredParcelID, "parcel-3"))

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SupplementBoundaryChanged {
		t.Fatalf("outcome = %q, want SUPPLEMENT_BOUNDARY_CHANGED", result.Outcome())
	}
	if fixture.requests.saved != nil {
		t.Fatal("成员重组仍换了代")
	}
}

// Covers: CONTEXT「已提交期间形成新提交版本」的决定边界——补充来晚了要读的是那份决定，
// 不再消耗版本与任务身份（先例：撤回的同一短路）。
func TestASupplementAfterTheDecisionReadsItBack(t *testing.T) {
	fixture := newSupplementFixture(t)
	fixture.requests.decided = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.SupplementDecisionAlreadyFormed {
		t.Fatalf("outcome = %q, want SUPPLEMENT_DECISION_ALREADY_FORMED", result.Outcome())
	}
	if _, present := result.AcceptanceDecision(); !present {
		t.Fatal("决定已越过边界却没交回那份决定")
	}
	if fixture.identities.versionsIssued != 0 {
		t.Fatal("已决定的委托仍消耗了版本身份")
	}
}

// Covers: UC-PS-002 结果语义`技术未形成`在补充一侧——委托读不回、身份签发不出、换代没
// 落库都停在未决，各带封闭原因与续办引用。
func TestUnreadableDependenciesKeepTheSupplementUndecided(t *testing.T) {
	cases := map[string]struct {
		arrange func(*supplementFixture)
		want    application.JudgmentPendingReason
	}{
		"request unreadable": {
			arrange: func(fixture *supplementFixture) { fixture.requests.err = errors.New("store down") },
			want:    application.ShipmentRequestUnavailable,
		},
		"identity factory unavailable": {
			arrange: func(fixture *supplementFixture) { fixture.identities.err = errors.New("factory down") },
			want:    application.SubmissionIdentityUnavailable,
		},
		"save fails": {
			arrange: func(fixture *supplementFixture) { fixture.requests.saveErr = errors.New("save down") },
			want:    application.SupplementedRequestNotSaved,
		},
		"save lost to a concurrent writer": {
			arrange: func(fixture *supplementFixture) { fixture.requests.saveConflict = true },
			want:    application.StaleShipmentRequestRevision,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newSupplementFixture(t)
			testCase.arrange(fixture)

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.SupplementUndecided {
				t.Fatalf("outcome = %q, want SUPPLEMENT_UNDECIDED", result.Outcome())
			}
			if result.PendingReason() != testCase.want {
				t.Fatalf("reason = %q, want %q", result.PendingReason(), testCase.want)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("未决没留续办引用")
			}
		})
	}
}
