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
// 被调用上才验得出来。revision 缺省给合法值：修订标识是答复的必备件，想演练缺失要显式
// 置空（noRevision）。端口按 ADR-0046 只回事实，评估在领域执行。
type evidenceDouble struct {
	areas         []domain.ServiceAreaResolution
	requirements  []domain.RouteRequirement
	executability []domain.PathExecutability
	constraints   []domain.HardConstraintFinding
	err           error
	notConfigured bool
	noRevision    bool
	assembled     int
	assembleKey   domain.ReachabilityJudgmentKey
}

func (double *evidenceDouble) LoadNetworkEvidence(
	_ context.Context,
	key domain.ReachabilityJudgmentKey,
) (ports.NetworkEvidence, bool, error) {
	double.assembled++
	double.assembleKey = key
	if double.err != nil {
		return ports.NetworkEvidence{}, false, double.err
	}
	if double.notConfigured {
		return ports.NetworkEvidence{}, false, nil
	}
	evidence := ports.NetworkEvidence{
		ServiceAreas:      double.areas,
		RouteRequirements: double.requirements,
		PathExecutability: double.executability,
		HardConstraints:   double.constraints,
	}
	if !double.noRevision {
		revision, err := domain.NewNetworkViewRevision("net-view-rev-1")
		if err != nil {
			return ports.NetworkEvidence{}, false, err
		}
		evidence.ViewRevision = revision
	}
	return evidence, true, nil
}

// coveringAreas 造覆盖目的地的区域解析事实——经领域评估折成合格候选，替代旧夹具直接
// 喂候选的做法。
func coveringAreas(t *testing.T, candidates ...string) []domain.ServiceAreaResolution {
	t.Helper()
	areas := make([]domain.ServiceAreaResolution, 0, len(candidates))
	for _, candidate := range candidates {
		resolution, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
			Candidate:   value(t, domain.NewCandidateID, candidate),
			Outcome:     domain.AreaCoversDestination,
			AreaVersion: value(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
		})
		if err != nil {
			t.Fatalf("new covering resolution: %v", err)
		}
		areas = append(areas, resolution)
	}
	return areas
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
	existing ports.ReachabilityJudgmentRecord
	found    bool
	findErr  error
	saveErr  error
	// alreadyRecorded 让 Save 答「已有记录」，并让随后的 Find 读得到 existing——并发赢家
	// 对迟到写入方的可见性正是 AT-NR-028 要演练的东西。
	alreadyRecorded bool
	saved           []ports.ReachabilityJudgmentRecord
	askTenant       domain.TenantID
	findCalled      int
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
	if double.found || (double.alreadyRecorded && double.findCalled > 1) {
		return double.existing, true, nil
	}
	return ports.ReachabilityJudgmentRecord{}, false, nil
}

func (double *storeDouble) Save(
	_ context.Context,
	_ domain.RequestCorrelationID,
	record ports.ReachabilityJudgmentRecord,
) (ports.ReachabilityJudgmentSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ReachabilityJudgmentSaveOutcomeInvalid, double.saveErr
	}
	if double.alreadyRecorded {
		return ports.ReachabilityJudgmentAlreadyRecorded, nil
	}
	double.saved = append(double.saved, record)
	return ports.ReachabilityJudgmentSaved, nil
}

// handoffDouble 留住每一份交出的判断意图。计数与关联都要：意图由请求关联认领，「重试同一
// 份」与「第二份」只有关联分得开（AT-NR-030）。
type handoffDouble struct {
	err     error
	intents []ports.ReachabilityJudgmentHandoffIntent
}

func (double *handoffDouble) HandOffReachabilityJudgment(
	_ context.Context,
	intent ports.ReachabilityJudgmentHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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

// Covers: ADR-0052 的「未配置」格——网络定义登记册对这个范围未配置时如实答未形成判断并
// 占**自己**的原因格，不与依赖不可用共用，更不评成`不可达`。首发无租户时这是唯一走得到
// 的真实分支：折成空证据会让领域照常评估、得出一个业务结论，而实际情况是还没人说过网络
// 长什么样。两格的恢复动作相反——这一格等租户去登记，那一格等运维去救依赖。
func TestAnUnconfiguredNetworkCatalogueIsItsOwnNotFormedReason(t *testing.T) {
	evidence := &evidenceDouble{notConfigured: true}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(
		requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("未配置被当成技术错误抛出：%v", err)
	}
	if result.Outcome() != application.JudgmentNotFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_NOT_FORMED", result.Outcome())
	}
	if _, present := result.Finding(); present {
		t.Fatal("未配置却给出了三值领域结果——那是从缺配置里编出的业务结论")
	}
	if result.NotFormedReason() != application.NetworkEvidenceNotConfigured {
		t.Fatalf("reason = %q, want NETWORK_EVIDENCE_NOT_CONFIGURED", result.NotFormedReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("未配置无法安全续办")
	}
	if len(store.saved) != 0 {
		t.Fatal("未配置越过了提交边界")
	}
}

// Covers: UC-NR-002 证据判定矩阵——候选生成本身失败不是`不可达`。零个候选分不清是覆盖
// 范围排除了目的地（那本该是一个带淘汰依据的候选）还是装配失败，两者都不能凭空断言。
func TestEmptyCandidateSpaceIsNotFormedRatherThanUnreachable(t *testing.T) {
	evidence := &evidenceDouble{areas: nil}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-2")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt.Add(time.Hour)})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(eligibility, evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-2")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

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

// Covers: `AT-NR-028`「同一判断请求版本并发处理 → 只允许一个结果版本越过提交边界，第二个
// 返回已有结果或明确冲突」——保存被抢先不是故障，Save 的写入结果因此是封闭代数而不是
// error（ADR-0031 同一裁决）。迟到方读回赢家：同范围交回它的判断，自己那一份不落库。
func TestAConcurrentWinnerIsReadBackRatherThanOverwritten(t *testing.T) {
	winner, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-0")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	store := &storeDouble{
		alreadyRecorded: true,
		existing: ports.ReachabilityJudgmentRecord{
			Key:      judgmentKey(t, "parcel-1"),
			Finding:  winner,
			JudgedAt: judgedAt,
		},
	}
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt.Add(time.Hour)})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ExistingJudgment {
		t.Fatalf("outcome = %q, want EXISTING_JUDGMENT——迟到方要读回赢家而不是报故障", result.Outcome())
	}
	if !result.JudgedAt().Equal(judgedAt) {
		t.Fatalf("judged at = %s, want the winner's %s——迟到结果按到达顺序覆盖了原判断时间", result.JudgedAt(), judgedAt)
	}
	if len(store.saved) != 0 {
		t.Fatal("第二个结果版本越过了提交边界")
	}
}

// 同一并发窗口的另一半：赢家的判断范围与本次不同，是冲突而不是本范围的结果。把它当成
// 已有判断交回去，调用方会拿另一个范围的结论继续往下走。
func TestAConcurrentWinnerWithADifferentScopeIsAConflict(t *testing.T) {
	winner, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-0")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	store := &storeDouble{
		alreadyRecorded: true,
		existing: ports.ReachabilityJudgmentRecord{
			Key:      judgmentKey(t, "parcel-2"),
			Finding:  winner,
			JudgedAt: judgedAt,
		},
	}
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RequestConflict {
		t.Fatalf("outcome = %q, want REQUEST_CONFLICT", result.Outcome())
	}
	if _, present := result.Finding(); present {
		t.Fatal("冲突结果携带了另一个范围的三值判断")
	}
}

// Covers: `AT-NR-030` 的意图半边「判断结果已提交，但事件发布失败 → 保留判断结果并只重试
// 同一发布意图，不重复评估」——已提交的判断交出恰好一份由请求关联认领的发布意图。投递与
// Outbox 半边仍在 Bento 闸门后（ADR-0017），本上下文不记意图完没完成（ADR-0043）。
func TestAFormedJudgmentHandsOffOneIntentClaimedByItsCorrelation(t *testing.T) {
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	downstream := &handoffDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, downstream, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(downstream.intents) != 1 {
		t.Fatalf("handed off %d intents, want exactly one", len(downstream.intents))
	}
	intent := downstream.intents[0]
	if intent.Correlation.String() != "correlation-1" {
		t.Fatalf("intent claims %q, want the request correlation", intent.Correlation)
	}
	finding, present := result.Finding()
	if !present || intent.Finding.Value() != finding.Value() {
		t.Fatalf("intent finding = %#v, want the formed judgment", intent.Finding)
	}
	if !intent.JudgedAt.Equal(result.JudgedAt()) {
		t.Fatalf("intent judged at = %s, want %s", intent.JudgedAt, result.JudgedAt())
	}
	if result.JudgmentHandoffReference().String() != "" {
		t.Fatal("交付成功仍留下了发布续办引用——调用方会去重放一件已经办完的事")
	}
}

// 同 AT 的失败方向：首次发布失败不改写判断——三值结果与判断时间原样交回、判断仍在库里，
// 发布续办引用单独留出，与`未形成判断`的续办分开。
func TestAnUndeliveredJudgmentHandoffKeepsTheJudgmentWithAResumableIntent(t *testing.T) {
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	downstream := &handoffDouble{err: errors.New("downstream unreachable")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, downstream, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.JudgmentFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_FORMED——发布失败改写了判断结果", result.Outcome())
	}
	if _, present := result.Finding(); !present {
		t.Fatal("发布失败弄丢了三值判断")
	}
	if len(store.saved) != 1 {
		t.Fatalf("saved %d judgments; 发布失败不得回退已提交的判断", len(store.saved))
	}
	if result.JudgmentHandoffReference().String() == "" {
		t.Fatal("首次发布失败没留下发布续办引用；没有引用，这份意图不会有人再交一次")
	}
	if result.ContinuationReference().String() != "" {
		t.Fatal("发布失败混进了未形成判断的续办——判断已经形成，没有什么要重判")
	}
}

// Covers: NR CONTEXT「每次可达性或路由判断必须保留……关键输入的有效区间和**当前修订
// 标识**」——修订标识随判断落库并随发布意图带给消费方，它是 `AT-PS-037` 提交前失效重判
// 的比对锚：没有它，消费方永远发现不了「判断形成后视图换过代」。
func TestAFormedJudgmentRetainsTheEvidenceViewRevision(t *testing.T) {
	evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-1")}
	store := &storeDouble{}
	downstream := &handoffDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, downstream, fixedClock{at: judgedAt})

	if _, err := handler.Handle(context.Background(), command(t, "parcel-1")); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(store.saved) != 1 || store.saved[0].ViewRevision.String() != "net-view-rev-1" {
		t.Fatalf("saved = %#v; 判断没有留下证据视图修订标识", store.saved)
	}
	if len(downstream.intents) != 1 || downstream.intents[0].ViewRevision.String() != "net-view-rev-1" {
		t.Fatalf("intents = %#v; 发布意图没有带上修订标识，消费方无从比对", downstream.intents)
	}
}

// 证据答复缺修订标识是端口坏了，不是一种未决：记一份没有比对锚的判断，提交前失效永远
// 检测不到——响亮报错，判断不落库。
func TestEvidenceWithoutAViewRevisionIsALoudErrorNotAJudgment(t *testing.T) {
	evidence := &evidenceDouble{
		areas:      coveringAreas(t, "candidate-1"),
		noRevision: true,
	}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

	_, err := handler.Handle(context.Background(), command(t, "parcel-1"))

	if !errors.Is(err, application.ErrIncompleteNetworkEvidence) {
		t.Fatalf("error = %v, want ErrIncompleteNetworkEvidence", err)
	}
	if len(store.saved) != 0 {
		t.Fatal("缺比对锚的判断越过了提交边界")
	}
}

// 重放走已有判断路径时重发同一份意图：本上下文不记意图完没完成，只答`已有结果`就收工，
// 一份首次发布失败的判断会永远停在「本上下文已提交、下游从不知道」的状态。
func TestAReplayResendsTheSameJudgmentIntentWithoutReassessing(t *testing.T) {
	key := judgmentKey(t, "parcel-1")
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	store := &storeDouble{
		found:    true,
		existing: ports.ReachabilityJudgmentRecord{Key: key, Finding: finding, JudgedAt: judgedAt},
	}
	evidence := &evidenceDouble{}
	downstream := &handoffDouble{err: errors.New("downstream unreachable")}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, downstream, fixedClock{at: judgedAt.Add(time.Hour)})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ExistingJudgment {
		t.Fatalf("outcome = %q, want EXISTING_JUDGMENT", result.Outcome())
	}
	if len(downstream.intents) != 1 || downstream.intents[0].Correlation.String() != "correlation-1" {
		t.Fatalf("intents = %#v; 重放必须把同一份意图再交一次", downstream.intents)
	}
	if !downstream.intents[0].JudgedAt.Equal(judgedAt) {
		t.Fatal("重发的意图不是原判断那一份——那是第二份意图，不是同一份的重试")
	}
	if evidence.assembled != 0 {
		t.Fatal("重放重新评估了一遍")
	}
	if result.JudgmentHandoffReference().String() == "" {
		t.Fatal("重试仍未交出，发布续办引用不该消失")
	}
}

// Covers: `AT-NR-026` 在评估流水线里的接线——承诺声明经证据答复进入领域评估：唯一覆盖
// 目的地的候选不满足合同承诺时，判断是带承诺依据的`不可达`，不是把承诺当偏好放行。
// 分界规则本体的两半（偏好不淘汰、承诺不复活）钉在领域测试里。
func TestACommittedRequirementFlowsThroughTheAssessment(t *testing.T) {
	basis, err := domain.NewCommitmentBasisReference("CONTRACT-V7/ROUTE-X")
	if err != nil {
		t.Fatalf("new commitment basis: %v", err)
	}
	requirement, err := domain.NewRouteRequirement(domain.RouteRequirementSpec{
		Requirement: value(t, domain.NewRouteRequirementReference, "REQUIRE_ROUTE_X"),
		Binding:     domain.CommittedRequirement,
		Basis:       basis,
		Satisfies:   nil,
	})
	if err != nil {
		t.Fatalf("new route requirement: %v", err)
	}
	evidence := &evidenceDouble{
		areas:        coveringAreas(t, "candidate-1"),
		requirements: []domain.RouteRequirement{requirement},
	}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.JudgmentFormed {
		t.Fatalf("outcome = %q, want JUDGMENT_FORMED", result.Outcome())
	}
	finding, present := result.Finding()
	if !present || finding.Value() != domain.Unreachable {
		t.Fatalf("finding = %#v present = %v, want UNREACHABLE——承诺是硬约束", finding, present)
	}
	eliminated := finding.EliminatedCandidates()
	if len(eliminated) != 1 || eliminated[0].Reason().String() != "ROUTE_COMMITMENT_NOT_SATISFIED/CONTRACT-V7/ROUTE-X" {
		t.Fatalf("eliminated = %#v; 承诺淘汰必须携带承诺依据", eliminated)
	}
}

// Covers: 层次 4/5 在评估流水线里的接线——不可执行事实淘汰一条候选、硬约束状态未知把
// 另一条降为证据未知，缺口并入判断，结论`资料不足`。各层规则本体钉在领域测试里。
func TestPathAndConstraintFactsFlowThroughTheAssessment(t *testing.T) {
	notExecutable, err := domain.NewPathExecutability(domain.PathExecutabilitySpec{
		Candidate: value(t, domain.NewCandidateID, "candidate-1"),
		Outcome:   domain.PathNotExecutable,
		Schedule:  value(t, domain.NewScheduleVersionReference, "SCHED-V3"),
	})
	if err != nil {
		t.Fatalf("new path executability: %v", err)
	}
	unknownConstraint, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: value(t, domain.NewCandidateID, "candidate-2"),
		Outcome:   domain.ConstraintStatusUnknown,
		Missing:   value(t, domain.NewEvidenceGapReference, "DANGEROUS_GOODS_CLASSIFICATION"),
		Reassess:  value(t, domain.NewReassessmentCondition, "WHEN_GOODS_CLASSIFICATION_CONFIRMED"),
	})
	if err != nil {
		t.Fatalf("new hard constraint finding: %v", err)
	}
	evidence := &evidenceDouble{
		areas:         coveringAreas(t, "candidate-1", "candidate-2"),
		executability: []domain.PathExecutability{notExecutable},
		constraints:   []domain.HardConstraintFinding{unknownConstraint},
	}
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(requiredEligibility(t), evidence, store, &handoffDouble{}, fixedClock{at: judgedAt})

	result, err := handler.Handle(context.Background(), command(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	finding, present := result.Finding()
	if !present || finding.Value() != domain.InsufficientEvidence {
		t.Fatalf("finding = %#v present = %v, want INSUFFICIENT_EVIDENCE", finding, present)
	}
	if len(finding.EliminatedCandidates()) != 1 || len(finding.UnknownCandidates()) != 1 {
		t.Fatalf("candidates = %#v; 两层评估各应落下一格", finding.Candidates())
	}
	gaps := finding.EvidenceGaps()
	if len(gaps) != 1 || gaps[0].Reference().String() != "DANGEROUS_GOODS_CLASSIFICATION" {
		t.Fatalf("gaps = %#v; 硬约束缺口没有并入判断", gaps)
	}
}
