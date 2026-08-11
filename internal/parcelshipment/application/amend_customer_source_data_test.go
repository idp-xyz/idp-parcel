package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-002 `AT-PS-014`「接受后补充缺失的客户申报原始资料 → 形成新客户原始资料版本，
// 接受基线不变」与步骤 7-8「形成不可覆盖版本」「派生当前资料版本采用判断」。
func TestAnAuthorizedAmendmentFormsAVersionAndDerivesItsAdoption(t *testing.T) {
	fixture := newAmendmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentRecorded {
		t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
	}
	version, present := result.Version()
	if !present {
		t.Fatal("an authorized amendment formed no customer source data version")
	}
	if version.Authority().String() != "PC-AMEND-GRANT-1" {
		t.Fatalf("authority = %q; the adopted authorization must be the one party-commercial issued", version.Authority())
	}
	// 请求方与实际决定方分立：UC-PS-002 要求「登录操作人不能替代实际决定方」。
	if version.Requester().String() != "CUSTOMER-CONTACT-1" || version.Decider().String() != "OPERATOR-1" {
		t.Fatalf("requester = %q decider = %q; the two must be recorded apart", version.Requester(), version.Decider())
	}
	adoption, derived := result.Adoption()
	if !derived || adoption.Outcome() != domain.SourceDataAdopted {
		t.Fatalf("adoption = %q derived = %v; a recorded version must leave a current adoption for downstream", adoption.Outcome(), derived)
	}
	if fixture.requests.saved == nil {
		t.Fatal("a version was formed but never saved")
	}
}

// Covers: UC-PS-002「具体字段、字段组、阶段和允许动作由 `PAR-COM-13` 登记……未登记时只能形成
// 未决或业务拒绝，不能以系统便利推断允许」，以及结果语义`待补充/待复核`。
//
// 停在`待复核`而不是业务拒绝：未登记是「还没人说这处资料能不能改」，不是「客户违规」。判成
// 拒绝会让客户以为自己请求有错，而错的是我们还没登记规则，等矩阵登记后要回头翻案。
//
// 不形成版本：结果语义表里只有`已记录并采用`那一行写着「新版本已形成」，`待补充/待复核`那一行
// 要保留的是「缺失/冲突范围、**当前版本**、待补足原因和续办引用」——当前版本指既有的那一份，
// 不是新造一份。请求本身的留痕由步骤 1 的来源保全承担，与版本是两回事。
func TestAnUndeclaredFieldStageRuleStopsAtAwaitingReviewRatherThanAllowing(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.allowance = ports.SourceDataAmendmentNotDeclared

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentAwaitingReview {
		t.Fatalf("outcome = %q, want AWAITING_REVIEW; an unregistered matrix must never be read as permission", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("an undeclared rule formed a version anyway; downstream would then adopt data no rule cleared")
	}
	if fixture.requests.saved != nil {
		t.Fatal("an undeclared rule wrote a version onto the accepted request")
	}
	// 来源已保全：请求到达过这件事必须留下，否则事后连客户提没提过都无从追溯。
	if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
		t.Fatal("an undeclared rule dropped the source fact too; the request still arrived")
	}
}

// Covers: UC-PS-002 步骤 6「不允许的阶段或字段形成业务拒绝」与`业务拒绝`结果语义。
//
// 与上一条的分界：已登记规则说「这处资料在这个阶段不能这样改」是确定答案，复核也翻不了案；
// 未登记等的是有人去登记矩阵。两者合成一格，客户就分不清该改请求还是该等我们登记规则。
func TestADisallowedFieldStageRuleFormsABusinessRejectionRatherThanAwaitingReview(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.allowance = ports.SourceDataAmendmentDisallowed

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentDisallowed {
		t.Fatalf("outcome = %q, want DISALLOWED; a registered rule forbidding the change is a business answer, not a pending review", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("a disallowed amendment formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; a disallowed attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("a disallowed amendment wrote a version onto the accepted request")
	}
}

// Covers: UC-PS-002 步骤 4「授权不足形成业务拒绝」与`业务拒绝`结果语义 — 未获授权是确定的
// 业务答案，不是未决，续办也补不出授权来；它与「授权服务答不出」分属两回事。
func TestAnUnauthorizedAmendmentFormsNoVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.granted = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("an unauthorized request formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an unauthorized attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("an unauthorized attempt saved the request")
	}
}

// Covers: UC-PS-002「同一请求身份、相同规范化内容摘要、相同目标范围和基础版本的重试返回已有
// 结果；不能创建第二个资料版本」与 `AT-PS-016`。
//
// 重放不再走一遍授权与规则：授权在两次之间可能已经失效，同一份已经形成的版本会因此读出两种
// 回执。网络重试是常态，这条路一旦缺席，一次重发就在已接受委托上多挂一份版本——而下游按范围
// 取当前采用判断，多出来的那一份会直接改掉它消费的内容。
func TestARepeatedAmendmentReturnsTheOriginalVersionWithoutFormingASecond(t *testing.T) {
	fixture := newAmendmentFixture(t)
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, formed := first.Version()
	if !formed {
		t.Fatal("the first amendment formed no version")
	}

	repeated, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle repeat: %v", err)
	}

	if repeated.Outcome() != application.AmendmentAlreadyHandled {
		t.Fatalf("outcome = %q, want ALREADY_HANDLED", repeated.Outcome())
	}
	version, present := repeated.Version()
	if !present || version.VersionID() != original.VersionID() {
		t.Fatalf(
			"version = %q present = %v; a retry must read back the version it already formed (%q)",
			version.VersionID(), present, original.VersionID(),
		)
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs; the retry formed a second version", fixture.identities.issued)
	}
	if fixture.authorizer.calls != 1 {
		t.Fatalf("authorized %d times; the retry re-ran authorization and may read a different answer", fixture.authorizer.calls)
	}
	if kept := fixture.requests.saved.CustomerSourceDataVersions(); len(kept) != 1 {
		t.Fatalf("versions on the request = %d, want the one original", len(kept))
	}
}

// Covers: UC-PS-002「同一请求身份携带不同内容、范围、基准或 `requestEffectiveAt` 形成请求冲突」
// 与 `AT-PS-017`「原请求和原版本不被覆盖」。
//
// 与重放分成两个结果，因为客户要做的事不同：重放是「这次请求已经办过了」，冲突是「同一个请求
// 身份底下压着两份不同内容，先纠正是哪一份」。按最后到达覆盖是用例明禁的。
func TestAnAmendmentSourceConflictNeitherOverwritesNorFormsASecondVersion(t *testing.T) {
	fixture := newAmendmentFixture(t)
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, formed := first.Version()
	if !formed {
		t.Fatal("the first amendment formed no version")
	}

	conflicting := fixture.command(t)
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "amend-digest-2")
	result, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("handle conflicting: %v", err)
	}

	if result.Outcome() != application.AmendmentSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
	}
	if _, present := result.Version(); present {
		t.Fatal("a conflicting request formed a version anyway")
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d version IDs; a conflicting request formed a second version", fixture.identities.issued)
	}
	// 原来源不被后到的内容覆盖：留痕一旦被改写，事后就分不出客户先说了什么、后说了什么。
	preserved, kept := fixture.sources.stored(t, amendmentSourceIdentity(t))
	if !kept || preserved.Digest().String() != "amend-digest-1" {
		t.Fatalf("preserved digest = %q kept = %v, want the original amend-digest-1", preserved.Digest(), kept)
	}
	versions := fixture.requests.saved.CustomerSourceDataVersions()
	if len(versions) != 1 || versions[0].VersionID() != original.VersionID() {
		t.Fatalf("versions on the request = %d; the conflicting request disturbed the original", len(versions))
	}
}

// Covers: UC-PS-002 步骤 4「依赖无法确定形成待复核」与业务未决结束「规则或授权无法确定」。
//
// 授权服务答不出与「这个人不能改这处资料」是两回事：后者续办也补不出授权来，前者重试就好。
// 判成未获授权会让客户以为自己越权，而真相是我们没问到；判成技术错误则连未决原因与续办引用
// 都给不出，而用例两样都要。
func TestAnUnavailableAmendmentAuthorizerIsUndecidedRatherThanUnauthorized(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.err = errors.New("party-commercial unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED; an unreachable authorizer is not an answer about authority", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataAmendmentAuthorityUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undecided round left no continuation reference; the caller cannot resume what it cannot name")
	}
	if _, present := result.Version(); present {
		t.Fatal("an undecided round formed a version anyway")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an undecided round consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: 同一条业务未决语义的规则一侧 — 「规则……无法确定」。
//
// 与未登记分开：未登记是矩阵答了「没有这条」，等的是有人去 `PAR-COM-13` 登记；读不回是矩阵没
// 答上话，等的是依赖恢复。合成一格，续办方就不知道该催人还是该重试。
func TestAnUnreadableSourceDataRuleIsUndecidedRatherThanAwaitingReview(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.rules.err = errors.New("rule declaration unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED; an unreadable matrix is not the same as an unregistered one", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataRuleUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_RULE_UNAVAILABLE", result.PendingReason())
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d version IDs; an undecided round consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: UC-PS-002`技术未形成`「原始请求已保全，但版本提交或发布意图未完成」与「处理阶段、
// 失败位置和安全续办引用」— 签发不出版本标识时停在未决，来源已保全的事实不因此丢失。
func TestAnUnavailableVersionIdentityLeavesTheRequestUndecidedAndResumable(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.identities.err = errors.New("identity factory unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.SourceDataVersionIdentityUnavailable {
		t.Fatalf("reason = %q, want SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE", result.PendingReason())
	}
	if fixture.requests.saved != nil {
		t.Fatal("a round that formed no version saved the request anyway")
	}
	// 来源仍在：`技术未形成`的前提就是「原始请求已保全」，丢了它这一轮连续办都无从认领。
	if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
		t.Fatal("an undecided round dropped the preserved source")
	}
}

// Covers: 同一条`技术未形成`的另一处失败位置 — 版本已形成但没落库。
//
// 不交回`已记录并采用`：用例明禁「不得返回已采用或业务拒绝」。版本没存住，下游按它办事就会
// 扑空，而本轮交回的续办引用正是让它重放同一次修订的凭据。
func TestAnUnsavedAmendmentIsUndecidedRatherThanRecorded(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.saveErr = errors.New("storage unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.AmendedRequestNotSaved {
		t.Fatalf("reason = %q, want AMENDED_REQUEST_NOT_SAVED", result.PendingReason())
	}
	if _, present := result.Version(); present {
		t.Fatal("a version that never reached storage was reported as formed")
	}
}

// Covers: 同一条业务未决语义在委托读取一侧 — 依赖答不出与「指名了一份不存在的委托」分开。
//
// 后者仍上抛（调用方对世界的判断就是错的），前者是依赖抖动，重试就好。两者都写成错误的话，
// 一次存储抖动会被记成调用方的编程错误。
func TestAnUnreadableShipmentRequestIsUndecidedRatherThanAnError(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.requests.err = errors.New("storage unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ShipmentRequestUnavailable {
		t.Fatalf("reason = %q, want SHIPMENT_REQUEST_UNAVAILABLE", result.PendingReason())
	}
	if fixture.authorizer.calls != 0 {
		t.Fatalf("authorized %d times; a round that never read the request went on to ask about authority", fixture.authorizer.calls)
	}
}

// Covers: `judgment_continuation.go`「原因参与派生也意味着停在不同阶段的两次未决给出不同引用」。
//
// 两轮的范围完全一致，唯一的变量是原因；引用一旦相同就说明原因根本没进派生，那样所有按原因
// 区分的续办断言都是假的。这一条钉的是派生本身，不是某一个调用点。
func TestTwoDifferentAmendmentStoppingCausesNeverShareOneContinuation(t *testing.T) {
	unavailableAuthorizer := newAmendmentFixture(t)
	unavailableAuthorizer.authorizer.err = errors.New("party-commercial unreachable")
	authorityStop, err := unavailableAuthorizer.handler.Handle(context.Background(), unavailableAuthorizer.command(t))
	if err != nil {
		t.Fatalf("handle with unavailable authorizer: %v", err)
	}

	unreadableRules := newAmendmentFixture(t)
	unreadableRules.rules.err = errors.New("rule declaration unreachable")
	ruleStop, err := unreadableRules.handler.Handle(context.Background(), unreadableRules.command(t))
	if err != nil {
		t.Fatalf("handle with unreadable rules: %v", err)
	}

	if authorityStop.ContinuationReference() == ruleStop.ContinuationReference() {
		t.Fatalf(
			"both stopping causes derived %q; the reason does not participate in the derivation",
			authorityStop.ContinuationReference(),
		)
	}
}

type amendmentFixture struct {
	handler    *application.AmendCustomerSourceDataHandler
	sources    *sourceRepositoryDouble
	requests   *amendableRequestStore
	authorizer *amendmentAuthorizerDouble
	rules      *sourceDataRuleDouble
	identities *sourceDataIdentityFactory
	steps      []string
}

func newAmendmentFixture(t *testing.T) *amendmentFixture {
	t.Helper()
	value := &amendmentFixture{
		requests:   &amendableRequestStore{t: t},
		authorizer: &amendmentAuthorizerDouble{t: t, granted: true},
		rules:      &sourceDataRuleDouble{allowance: ports.SourceDataAmendmentAllowed},
		identities: &sourceDataIdentityFactory{t: t},
	}
	value.sources = &sourceRepositoryDouble{
		records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{},
		record:  func(step string) { value.steps = append(value.steps, step) },
	}
	value.authorizer.record = func(step string) { value.steps = append(value.steps, step) }
	value.handler = application.NewAmendCustomerSourceDataHandler(application.AmendCustomerSourceDataDeps{
		Sources:    value.sources,
		Requests:   value.requests,
		Authorizer: value.authorizer,
		Rules:      value.rules,
		Identities: value.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return value
}

// amendmentSourceIdentity 是修订请求自己的来源身份，与产生委托的那一次提交分开：同一个客户
// 就同一份委托先提交后修订，是两次来源请求，各自有各自的请求标识与内容摘要。
func amendmentSourceIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	return sourceIdentity(t, "tenant-1", "customer-1", "source-a", "amend-key-1")
}

// consigneeDataScope 是一处委托级资料范围。委托级而非逐包裹：寄收件一类资料本就作用于整份
// 委托，逐包裹填会把一处更正复制成成员份数。
func consigneeDataScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewSourceDataGroupReference, "CONSIGNEE_ADDRESS"),
	)
	if err != nil {
		t.Fatalf("new shipment scoped source data: %v", err)
	}
	return scope
}

func (value *amendmentFixture) command(t *testing.T) application.AmendCustomerSourceDataCommand {
	t.Helper()
	return application.AmendCustomerSourceDataCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		AmendmentIdentity: amendmentSourceIdentity(t),
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "amend-digest-1"),
		OccurredAt:        handlerClockAt,
		ReceivedAt:        handlerClockAt,
		Scope:             consigneeDataScope(t),
		Basis:             domain.NewSupplementOnAcceptanceBaseline(),
		Reason:            mustValue(t, domain.NewAmendmentReasonReference, "CONSIGNEE_ADDRESS_CORRECTION"),
		Requester:         mustValue(t, domain.NewRequesterReference, "CUSTOMER-CONTACT-1"),
	}
}

// amendableRequestStore 交回一份真正经领域接受过的委托。造一个「看起来已接受」的假状态挡不住
// AmendCustomerSourceData 的状态闸门，也就证明不了编排走对了路。
//
// 保存过就交回保存的那一份，而不是每次都重新造：重放与冲突都要跨两次调用才谈得上，每次交回
// 一份干净委托等于让第二次调用看不见第一次的版本，「不能创建第二个资料版本」也就无从断言。
type amendableRequestStore struct {
	t       *testing.T
	err     error
	saveErr error
	saved   *domain.ShipmentRequest
}

func (store *amendableRequestStore) FindBySourceIdentity(
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
	return acceptedRequest(store.t), true, nil
}

func (store *amendableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) error {
	return nil
}

func (store *amendableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	if store.saveErr != nil {
		return store.saveErr
	}
	store.saved = &request
	return nil
}

// acceptedRequest 把 submittedRequest 经领域推到`已接受`。资料修订只对`已接受`开放，所以
// 这一层是被测行为的前提而不是它的一部分。
func acceptedRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	applicable, err := domain.NewApplicableCheckGroups(domain.NetworkReachabilityCheck)
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

	checks := make([]domain.AcceptanceCheck, 0, 2)
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

	accepted, err := submittedRequest(t).Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     checks,
		Basis:      snapshot,
		DecidedAt:  handlerClockAt,
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if accepted.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", accepted.State())
	}
	return accepted
}

type amendmentAuthorizerDouble struct {
	t       *testing.T
	granted bool
	err     error
	calls   int
	record  func(string)
}

func (double *amendmentAuthorizerDouble) AuthorizeSourceDataAmendment(
	_ context.Context,
	_ ports.SourceDataAmendmentAuthorizationQuery,
) (ports.SourceDataAmendmentAuthorization, error) {
	double.t.Helper()
	double.calls++
	if double.record != nil {
		double.record("authorize-amendment")
	}
	if double.err != nil {
		return ports.SourceDataAmendmentAuthorization{}, double.err
	}
	if !double.granted {
		return ports.SourceDataAmendmentAuthorization{}, nil
	}
	return ports.SourceDataAmendmentAuthorization{
		Authority: mustValue(double.t, domain.NewAmendmentAuthoritySnapshot, "PC-AMEND-GRANT-1"),
		Decider:   mustValue(double.t, domain.NewDeciderReference, "OPERATOR-1"),
	}, nil
}

type sourceDataRuleDouble struct {
	allowance ports.SourceDataAmendmentAllowance
	err       error
}

func (double *sourceDataRuleDouble) DeclareSourceDataAmendment(
	_ context.Context,
	_ ports.SourceDataAmendmentQuery,
) (ports.SourceDataAmendmentAllowance, error) {
	if double.err != nil {
		return ports.SourceDataAmendmentNotDeclared, double.err
	}
	return double.allowance, nil
}

type sourceDataIdentityFactory struct {
	t      *testing.T
	err    error
	issued int
}

func (factory *sourceDataIdentityFactory) NextSourceDataVersionID(
	_ context.Context,
) (domain.SourceDataVersionID, error) {
	factory.t.Helper()
	if factory.err != nil {
		return domain.SourceDataVersionID{}, factory.err
	}
	factory.issued++
	return mustValue(factory.t, domain.NewSourceDataVersionID, "data-version-1"), nil
}

var (
	_ ports.ShipmentRequestRepository     = (*amendableRequestStore)(nil)
	_ ports.SourceDataAmendmentAuthorizer = (*amendmentAuthorizerDouble)(nil)
	_ ports.SourceDataRuleDeclaration     = (*sourceDataRuleDouble)(nil)
	_ ports.SourceDataVersionIdentity     = (*sourceDataIdentityFactory)(nil)
)
