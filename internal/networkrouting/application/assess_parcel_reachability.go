// Package application 在 network-routing 领域内核与本上下文自有端口之上编排用例。它不
// 含持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// AssessmentOutcome 是本用例的应用处理结果，与三值领域判断分属两层。用例把这条写死：
// `可达`、`不可达`、`资料不足`是 network-routing 拥有的领域判断，其余结果不得进入三值
// 统计，也不得被 parcel-shipment 当作可达性事实。两者压进一个枚举就再也分不开了。
type AssessmentOutcome uint8

const (
	AssessmentOutcomeInvalid AssessmentOutcome = iota
	JudgmentFormed
	ExistingJudgment
	JudgmentNotFormed
	RequestConflict
	RequestNotAccepted
	NotApplicable
)

func (outcome AssessmentOutcome) String() string {
	switch outcome {
	case JudgmentFormed:
		return "JUDGMENT_FORMED"
	case ExistingJudgment:
		return "EXISTING_JUDGMENT"
	case JudgmentNotFormed:
		return "JUDGMENT_NOT_FORMED"
	case RequestConflict:
		return "REQUEST_CONFLICT"
	case RequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	case NotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// NotFormedReason 指名本次为何没有形成三值判断。用例要求未形成判断按技术或依赖阶段分类
// 统计，所以它是封闭集合而非自由文本；取值与产生它的那条路径同时出现。
//
// 判断已形成时没有原因，所以零值表示「缺席」而不是「未知」。
type NotFormedReason uint8

const (
	NotFormedReasonNone NotFormedReason = iota
	JudgmentStoreUnavailable
	CommercialEligibilityUnavailable
	NetworkEvidenceUnavailable
	CandidateSpaceNotEstablished
)

func (reason NotFormedReason) String() string {
	switch reason {
	case JudgmentStoreUnavailable:
		return "JUDGMENT_STORE_UNAVAILABLE"
	case CommercialEligibilityUnavailable:
		return "COMMERCIAL_ELIGIBILITY_UNAVAILABLE"
	case NetworkEvidenceUnavailable:
		return "NETWORK_EVIDENCE_UNAVAILABLE"
	case CandidateSpaceNotEstablished:
		return "CANDIDATE_SPACE_NOT_ESTABLISHED"
	default:
		return ""
	}
}

// ContinuationReference 让发起方把停下的请求重新接上。它属应用层而不属领域，因为
// `未形成判断`本身就是应用处理结果——领域只拥有三值判断，不认识依赖失败这回事。
type ContinuationReference struct{ value string }

func (reference ContinuationReference) String() string {
	return reference.value
}

type AssessParcelReachabilityCommand struct {
	Correlation domain.RequestCorrelationID
	Key         domain.ReachabilityJudgmentKey
}

type AssessParcelReachabilityResult struct {
	outcome          AssessmentOutcome
	finding          domain.ReachabilityFinding
	hasFinding       bool
	judgedAt         time.Time
	reason           NotFormedReason
	continuation     ContinuationReference
	eligibilityBasis domain.EligibilityBasisReference
}

func (result AssessParcelReachabilityResult) Outcome() AssessmentOutcome {
	return result.outcome
}

// Finding 只在形成或找回了三值判断时给出。未形成判断、冲突与未受理一律没有——把一个零值
// 结论交回去，等于让调用方在本上下文根本没作出的判断上继续往下走。
func (result AssessParcelReachabilityResult) Finding() (domain.ReachabilityFinding, bool) {
	return result.finding, result.hasFinding
}

func (result AssessParcelReachabilityResult) JudgedAt() time.Time {
	return result.judgedAt
}

func (result AssessParcelReachabilityResult) NotFormedReason() NotFormedReason {
	return result.reason
}

func (result AssessParcelReachabilityResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

// EligibilityBasis 只在`不适用`时给出。用例要求这个结果携带明确不适用依据——没有依据的
// `不适用`看起来像一个结论，实际是一次没作出的判断。
func (result AssessParcelReachabilityResult) EligibilityBasis() domain.EligibilityBasisReference {
	return result.eligibilityBasis
}

type AssessParcelReachabilityHandler struct {
	eligibility ports.CommercialEligibilityView
	evidence    ports.NetworkEvidenceView
	store       ports.ReachabilityJudgmentStore
	clock       ports.Clock
}

func NewAssessParcelReachabilityHandler(
	eligibility ports.CommercialEligibilityView,
	evidence ports.NetworkEvidenceView,
	store ports.ReachabilityJudgmentStore,
	clock ports.Clock,
) *AssessParcelReachabilityHandler {
	return &AssessParcelReachabilityHandler{
		eligibility: eligibility,
		evidence:    evidence,
		store:       store,
		clock:       clock,
	}
}

// Handle 形成一个包裹的接受前可达性判断。它不形成委托接受、不选择当前有效路由、也不预占
// 任何履约资源——三值结果交回 parcel-shipment，由后者决定委托是否接受。
func (handler *AssessParcelReachabilityHandler) Handle(
	ctx context.Context,
	command AssessParcelReachabilityCommand,
) (AssessParcelReachabilityResult, error) {
	// 先判身份再读权威。顺序不能反：最小判断身份不成立时用例要求请求未受理，而一次已经
	// 发出的查询收不回来，它本身就回答了这个客户、这个委托存不存在。
	if !command.Key.MinimumIdentityEstablished() || !command.Correlation.Valid() {
		return AssessParcelReachabilityResult{outcome: RequestNotAccepted}, nil
	}

	existing, found, err := handler.store.FindByCorrelation(ctx, command.Key.TenantID, command.Correlation)
	if err != nil {
		return handler.notFormed(command, JudgmentStoreUnavailable), nil
	}
	if found {
		// 同范围重试返回原判断而不重新评估：重新评估会在同一请求下产生第二个结果，而
		// 用例要求返回原判断。范围不同却共用一个请求关联是冲突，同样不重新评估——一次
		// 新的评估正是「静默覆盖原请求」的做法。
		if existing.Key.SameJudgmentScope(command.Key) {
			return AssessParcelReachabilityResult{
				outcome:    ExistingJudgment,
				finding:    existing.Finding,
				hasFinding: true,
				judgedAt:   existing.JudgedAt,
			}, nil
		}
		return AssessParcelReachabilityResult{outcome: RequestConflict}, nil
	}

	// 商业适用先于候选装配。顺序不能反：一个本就不要求判断的服务，没有理由先被装配一遍
	// 候选——那次装配既是白做的，也已经读了这个客户的网络资格。
	eligibility, err := handler.eligibility.AssessNetworkEligibility(ctx, command.Key)
	if err != nil {
		// 商业侧调不通形成`未形成判断`，不读成「不要求判断」。混起来会让一次商业故障
		// 变成`不适用`，而`不适用`说的是这个问题不该问，与问过了没答案是两回事。
		return handler.notFormed(command, CommercialEligibilityUnavailable), nil
	}
	if !eligibility.JudgmentRequired() {
		return AssessParcelReachabilityResult{
			outcome:          NotApplicable,
			eligibilityBasis: eligibility.Basis(),
		}, nil
	}

	candidates, gaps, err := handler.evidence.AssembleCandidates(ctx, command.Key)
	if err != nil {
		// 依赖调不通形成`未形成判断`，不向上抛技术错误也不记成证据缺口。用例明写依赖
		// 失败不得伪装为`资料不足`：混起来会让一次网络故障被下游读成这个包裹的证据不全，
		// 进而当作向客户要资料的理由。
		return handler.notFormed(command, NetworkEvidenceUnavailable), nil
	}

	finding, err := domain.ConcludeReachability(candidates, gaps)
	if err != nil {
		// 领域拒绝空候选空间而不是给结论。分不清是覆盖范围排除了目的地——那本该是一个带
		// 淘汰依据的候选——还是候选生成失败，两者都不能凭空断言，所以停在未形成判断。
		return handler.notFormed(command, CandidateSpaceNotEstablished), nil
	}

	// 时钟在结论形成之后才读，因此判断时间落在证据装配之后而非之前。
	record := ports.ReachabilityJudgmentRecord{
		Key:      command.Key,
		Finding:  finding,
		JudgedAt: handler.clock.Now(),
	}
	if err := handler.store.Save(ctx, command.Correlation, record); err != nil {
		// 判断没能越过提交边界就不算形成。用例要求此时只保存请求与处理尝试，不发布
		// 可达、不可达或资料不足——交回一个未落库的三值结果，下游就会引用一个查不回来的
		// 判断。
		return handler.notFormed(command, JudgmentStoreUnavailable), nil
	}

	return AssessParcelReachabilityResult{
		outcome:    JudgmentFormed,
		finding:    finding,
		hasFinding: true,
		judgedAt:   record.JudgedAt,
	}, nil
}

// notFormed 构造所有未形成判断共用的那一种形状，让它们全部带上原因与续办引用：一次停下
// 的请求能不能接回去，不该取决于它停在哪一步。
func (handler *AssessParcelReachabilityHandler) notFormed(
	command AssessParcelReachabilityCommand,
	reason NotFormedReason,
) AssessParcelReachabilityResult {
	return AssessParcelReachabilityResult{
		outcome:      JudgmentNotFormed,
		reason:       reason,
		continuation: continuationFor(command, reason),
	}
}

// continuationFor 由请求关联、判断范围与原因共同派生，因此同一请求因同一原因停滞时拿到的
// 引用始终相同——这正是发起方能按稳定关联查询原次尝试而不必靠猜的原因。
func continuationFor(command AssessParcelReachabilityCommand, reason NotFormedReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.Correlation.String(),
		command.Key.TenantID.String(),
		command.Key.CustomerAccountID.String(),
		command.Key.ShipmentRequestID.String(),
		command.Key.SubmissionVersion.String(),
		command.Key.DeclaredParcelID.String(),
		command.Key.ServicePurpose.String(),
		command.Key.AsOf.Semantic().String(),
		command.Key.AsOf.StrategyVersion().String(),
		command.Key.AsOf.At().Format(time.RFC3339Nano),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}
