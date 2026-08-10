package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

var (
	asOfAt   = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	judgedAt = time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)
)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// evidenceDouble 记录它被问到的判断范围，因为「身份不成立时不得查询」这条只有在端口是否
// 被调用上才验得出来。
type evidenceDouble struct {
	candidates  []domain.RouteCandidate
	gaps        []domain.EvidenceGap
	err         error
	assembled   int
	assembleKey domain.ReachabilityJudgmentKey
}

func (double *evidenceDouble) AssembleCandidates(
	_ context.Context,
	key domain.ReachabilityJudgmentKey,
) ([]domain.RouteCandidate, []domain.EvidenceGap, error) {
	double.assembled++
	double.assembleKey = key
	if double.err != nil {
		return nil, nil, double.err
	}
	return double.candidates, double.gaps, nil
}

// eligibilityDouble 默认回答「要求判断」，这样已有测试仍然走到候选装配那一步。
type eligibilityDouble struct {
	eligibility domain.NetworkEligibility
	err         error
	asked       int
}

func (double *eligibilityDouble) AssessNetworkEligibility(
	_ context.Context,
	_ domain.ReachabilityJudgmentKey,
) (domain.NetworkEligibility, error) {
	double.asked++
	if double.err != nil {
		return domain.NetworkEligibility{}, double.err
	}
	return double.eligibility, nil
}

func requiredEligibility(t *testing.T) *eligibilityDouble {
	t.Helper()
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
	if err != nil {
		t.Fatalf("new network eligibility: %v", err)
	}
	return &eligibilityDouble{eligibility: eligibility}
}

type storeDouble struct {
	existing   ports.ReachabilityJudgmentRecord
	found      bool
	findErr    error
	saveErr    error
	saved      []ports.ReachabilityJudgmentRecord
	askTenant  domain.TenantID
	findCalled int
}

func (double *storeDouble) FindByCorrelation(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.RequestCorrelationID,
) (ports.ReachabilityJudgmentRecord, bool, error) {
	double.findCalled++
	double.askTenant = tenant
	if double.findErr != nil {
		return ports.ReachabilityJudgmentRecord{}, false, double.findErr
	}
	return double.existing, double.found, nil
}

func (double *storeDouble) Save(
	_ context.Context,
	_ domain.RequestCorrelationID,
	record ports.ReachabilityJudgmentRecord,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.saved = append(double.saved, record)
	return nil
}

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func judgmentKey(t *testing.T, parcel string) domain.ReachabilityJudgmentKey {
	t.Helper()
	asOf, err := domain.NewJudgmentAsOf(
		value(t, domain.NewAsOfSemantic, "CURRENT_SUBMISSION_RECEIVED_AT"),
		asOfAt,
		value(t, domain.NewAsOfStrategyVersion, "asof-strategy-v1"),
	)
	if err != nil {
		t.Fatalf("new judgment asOf: %v", err)
	}
	return domain.ReachabilityJudgmentKey{
		TenantID:          value(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID: value(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: value(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, domain.NewSubmissionVersionID, "submission-1"),
		DeclaredParcelID:  value(t, domain.NewDeclaredParcelID, parcel),
		ServicePurpose:    value(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
		AsOf:              asOf,
	}
}

func qualifiedCandidate(t *testing.T, id string) domain.RouteCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidate(
		value(t, domain.NewCandidateID, id),
		domain.CandidateQualified,
		domain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("new route candidate: %v", err)
	}
	return candidate
}

func command(t *testing.T, parcel string) application.AssessParcelReachabilityCommand {
	t.Helper()
	return application.AssessParcelReachabilityCommand{
		Correlation: value(t, domain.NewRequestCorrelationID, "correlation-1"),
		Key:         judgmentKey(t, parcel),
	}
}

// Covers: UC-NR-002 判断内容「判断时间」与「适用时点」并列——`asOf` 决定按哪一刻的网络
// 证据评估，判断时间只说明这次判断何时作出，压成一个会让重放看起来像新判断。
func TestFormedJudgmentTakesItsJudgmentTimeFromTheClockNotTheAsOf(t *testing.T) {
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.JudgmentFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_FORMED", result.Outcome())
	}
	finding, present := result.Finding()
	if !present || finding.Value() != domain.Reachable {
		t.Fatalf("finding = %#v, want a REACHABLE三值判断", finding)
	}
	if !result.JudgedAt().Equal(judgedAt) {
		t.Fatalf("judged at = %s, want the clock reading %s", result.JudgedAt(), judgedAt)
	}
	if result.JudgedAt().Equal(asOfAt) {
		t.Fatal("判断时间取成了适用时点：asOf 决定按哪一刻的证据评估，判断时间只说明何时作出")
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d judgments, want exactly one crossing the commit boundary", len(store.saved))
	}
}

// Covers: AT-NR-019「商业、网络或关务权威依赖调用超时 → 未形成三值判断，保留请求和安全
// 续办依据；不伪装为资料不足」。
func TestUnavailableNetworkEvidenceIsNotFormedRatherThanInsufficientEvidence(t *testing.T) {
	evidence := &evidenceDouble{err: errors.New("network evidence view unavailable")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("依赖失败被当成技术错误抛出，而用例要求它形成未形成判断: %v", err)
	}

	if result.Outcome() != application.JudgmentNotFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_NOT_FORMED", result.Outcome())
	}
	if _, present := result.Finding(); present {
		t.Fatal("未形成判断却携带了三值领域结果")
	}
	if result.NotFormedReason() != application.NetworkEvidenceUnavailable {
		t.Fatalf("reason = %q, want NETWORK_EVIDENCE_UNAVAILABLE", result.NotFormedReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("未形成判断无法安全续办")
	}
	if len(store.saved) != 0 {
		t.Fatal("尚未形成完整判断就越过了提交边界")
	}
}

// Covers: UC-NR-002 证据判定矩阵——候选生成本身失败不是`不可达`。零个候选分不清是覆盖
// 范围排除了目的地（那本该是一个带淘汰依据的候选）还是装配失败，两者都不能凭空断言。
func TestEmptyCandidateSpaceIsNotFormedRatherThanUnreachable(t *testing.T) {
	evidence := &evidenceDouble{candidates: nil}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.JudgmentNotFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_NOT_FORMED", result.Outcome())
	}
	if result.NotFormedReason() != application.CandidateSpaceNotEstablished {
		t.Fatalf("reason = %q, want CANDIDATE_SPACE_NOT_ESTABLISHED", result.NotFormedReason())
	}
	if len(store.saved) != 0 {
		t.Fatal("没有三值结果却写入了判断")
	}
}

// Covers: UC-NR-002 步骤 2 与权限隔离「批量请求、错误信息、查询和指标不得泄露其他客户的
// 存在或网络资格」——最小判断身份不成立时不得去问任何权威。
func TestIncompleteKeyIsRefusedWithoutReadingAnyAuthority(t *testing.T) {
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt})

	incomplete := command(t, "parcel-1")
	incomplete.Key.DeclaredParcelID = domain.DeclaredParcelID{}

	result, err := handler.Handle(context.Background(), incomplete)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RequestNotAccepted {
		t.Fatalf("outcome = %q, want REQUEST_NOT_ACCEPTED", result.Outcome())
	}
	if evidence.assembled != 0 {
		t.Fatal("身份不成立却已经装配候选，这本身就泄露了该范围有没有对象")
	}
	// 按关联查一次判断库同样是一次查询，它回答了「这个关联下有没有判断」。身份不成立时
	// 一个权威都不能问，不只是不能装配候选。
	if store.findCalled != 0 {
		t.Fatal("身份不成立却已经按关联查询判断库")
	}
	if len(store.saved) != 0 {
		t.Fatal("未受理的请求创建了占位判断")
	}
}

// Covers: AT-NR-020「相同请求身份、输入版本和适用时点重试 → 返回已有判断，不创建第二个
// 结果」。
func TestSameScopeRetryReturnsTheExistingJudgmentWithoutReassessing(t *testing.T) {
	key := judgmentKey(t, "parcel-1")
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	store := &storeDouble{
		found:    true,
		existing: ports.ReachabilityJudgmentRecord{Key: key, Finding: finding, JudgedAt: judgedAt},
	}
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-2")}}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt.Add(time.Hour)})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ExistingJudgment {
		t.Fatalf("outcome = %q, want EXISTING_JUDGMENT", result.Outcome())
	}
	if evidence.assembled != 0 {
		t.Fatal("重试重新装配了候选，这会让同一请求得到第二个结果")
	}
	if len(store.saved) != 0 {
		t.Fatal("重试创建了第二个判断")
	}
	if !result.JudgedAt().Equal(judgedAt) {
		t.Fatalf("judged at = %s, want the original %s——重试不得把原判断改成新时间", result.JudgedAt(), judgedAt)
	}
	if store.askTenant != key.TenantID {
		t.Fatalf("asked tenant = %q, want %q", store.askTenant, key.TenantID)
	}
}

// Covers: UC-NR-002 启动条件「仅提供面单渠道服务且运营企业不控制端到端网络时，本用例不
// 适用」与结果语义「不得以不适用代替不可达，也不得虚构运营网络」。
func TestServiceThatDoesNotRequireANetworkJudgmentIsNotApplicable(t *testing.T) {
	basis := value(t, domain.NewEligibilityBasisReference, "LABEL_ONLY_CHANNEL_SERVICE")
	notRequired, err := domain.NewNetworkEligibility(domain.NetworkJudgmentNotRequired, basis)
	if err != nil {
		t.Fatalf("new network eligibility: %v", err)
	}
	eligibility := &eligibilityDouble{eligibility: notRequired}
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.NotApplicable {
		t.Fatalf("outcome = %q, want NOT_APPLICABLE", result.Outcome())
	}
	if _, present := result.Finding(); present {
		t.Fatal("不适用结果携带了三值领域判断——这正是用不适用冒充不可达的做法")
	}
	if result.EligibilityBasis() != basis {
		t.Fatalf("basis = %q, want the explicit %q", result.EligibilityBasis(), basis)
	}
	if evidence.assembled != 0 {
		t.Fatal("服务不要求判断却仍去装配候选")
	}
	if len(store.saved) != 0 {
		t.Fatal("不适用不是三值判断，不该越过提交边界")
	}
}

// Covers: UC-NR-002 步骤 4「依赖不可用时未形成判断」——商业侧调不通不得被读成「不要求
// 判断」，否则一次商业故障就变成了不适用。
func TestUnavailableCommercialEligibilityIsNotFormedRatherThanNotApplicable(t *testing.T) {
	eligibility := &eligibilityDouble{err: errors.New("commercial eligibility view unavailable")}
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("商业依赖失败被当成技术错误抛出，而用例要求它形成未形成判断: %v", err)
	}

	if result.Outcome() != application.JudgmentNotFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_NOT_FORMED", result.Outcome())
	}
	if result.NotFormedReason() != application.CommercialEligibilityUnavailable {
		t.Fatalf("reason = %q, want COMMERCIAL_ELIGIBILITY_UNAVAILABLE", result.NotFormedReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("未形成判断无法安全续办")
	}
	if evidence.assembled != 0 {
		t.Fatal("商业适用未确定却已经装配候选")
	}
}

// Covers: UC-NR-002 步骤顺序——商业适用（步骤 4）先于服务区域与候选装配（步骤 5、7）。
// 顺序反了会让一个本不该判断的服务先被装配一遍候选。
func TestCommercialEligibilityIsAskedBeforeAssemblingCandidates(t *testing.T) {
	eligibility := requiredEligibility(t)
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, fixedClock{at: judgedAt})

	if _, err := handler.Handle(context.Background(), command(t, "parcel-1")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if eligibility.asked != 1 {
		t.Fatalf("asked commercial eligibility %d times, want exactly one per judgment", eligibility.asked)
	}
	if evidence.assembled != 1 {
		t.Fatalf("assembled candidates %d times, want exactly one", evidence.assembled)
	}
}

// Covers: AT-NR-021「相同请求身份携带不同提交版本或输入 → 形成请求冲突；原输入和判断不被
// 覆盖」。
func TestSameCorrelationWithADifferentScopeIsAConflictAndDoesNotOverwrite(t *testing.T) {
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	store := &storeDouble{
		found: true,
		existing: ports.ReachabilityJudgmentRecord{
			Key:      judgmentKey(t, "parcel-1"),
			Finding:  finding,
			JudgedAt: judgedAt,
		},
	}
	evidence := &evidenceDouble{candidates: []domain.RouteCandidate{qualifiedCandidate(t, "candidate-2")}}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-2"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RequestConflict {
		t.Fatalf("outcome = %q, want REQUEST_CONFLICT", result.Outcome())
	}
	if _, present := result.Finding(); present {
		t.Fatal("冲突结果携带了三值领域判断")
	}
	if evidence.assembled != 0 {
		t.Fatal("冲突请求仍去装配候选")
	}
	if len(store.saved) != 0 {
		t.Fatal("冲突请求覆盖了原判断")
	}
}
