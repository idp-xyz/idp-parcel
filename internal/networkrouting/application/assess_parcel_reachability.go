// Package application 在 network-routing 领域内核与本上下文自有端口之上编排用例。它不
// 含持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrUnexpectedJudgmentSaveOutcome 说明判断库交回了封闭集合以外的写入结果。它上抛而不
// 形成未决：依赖答不出是业务结果，答出一个不属于这个集合的东西则是端口坏了。
var ErrUnexpectedJudgmentSaveOutcome = errors.New("network routing: unexpected judgment save outcome")

// ErrIncompleteNetworkEvidence 说明证据端口答了候选却没带视图修订标识。它上抛而不落
// `未形成判断`：那不是依赖答不出，是答复本身缺了 CONTEXT 要求判断保留的东西——记一份
// 没有比对锚的判断，提交前失效永远检测不到。
var ErrIncompleteNetworkEvidence = errors.New("network routing: network evidence carries no view revision")

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
	// NetworkEvidenceNotConfigured 是网络定义登记册对这个范围未配置（ADR-0052）。
	// 与依赖不可用分格：那一格等运维，这一格等租户登记网络定义。两者都不是`资料不足`
	// ——`资料不足`说的是这个包裹的地址等信息不全，是向客户要东西的理由。
	NetworkEvidenceNotConfigured
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
	case NetworkEvidenceNotConfigured:
		return "NETWORK_EVIDENCE_NOT_CONFIGURED"
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
	handoff          ContinuationReference
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

// JudgmentHandoffReference 只在判断已越过提交边界、而它的发布意图没能确定交出时给出。
// 它与 ContinuationReference 分开：后者续办的是尚未形成的判断，前者续办的是已提交判断
// 留下的发布——合成一个，调用方就分不清该重判还是该重放（AT-NR-030）。
func (result AssessParcelReachabilityResult) JudgmentHandoffReference() ContinuationReference {
	return result.handoff
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
	downstream  ports.ReachabilityJudgmentHandoff
	clock       ports.Clock
}

func NewAssessParcelReachabilityHandler(
	eligibility ports.CommercialEligibilityView,
	evidence ports.NetworkEvidenceView,
	store ports.ReachabilityJudgmentStore,
	downstream ports.ReachabilityJudgmentHandoff,
	clock ports.Clock,
) *AssessParcelReachabilityHandler {
	return &AssessParcelReachabilityHandler{
		eligibility: eligibility,
		evidence:    evidence,
		store:       store,
		downstream:  downstream,
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
			return handler.existingJudgment(ctx, command, existing), nil
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

	evidence, configured, err := handler.evidence.LoadNetworkEvidence(ctx, command.Key)
	if !configured && err == nil {
		// 首发唯一走得到的真实分支：没有租户就没有网络定义，如实答未配置。折成空证据
		// 会让领域评出`不可达`，那是从缺配置里编出一个业务结论。
		return handler.notFormed(command, NetworkEvidenceNotConfigured), nil
	}
	if err != nil {
		// 依赖调不通形成`未形成判断`，不向上抛技术错误也不记成证据缺口。用例明写依赖
		// 失败不得伪装为`资料不足`：混起来会让一次网络故障被下游读成这个包裹的证据不全，
		// 进而当作向客户要资料的理由。
		return handler.notFormed(command, NetworkEvidenceUnavailable), nil
	}
	if !evidence.ViewRevision.Valid() {
		return AssessParcelReachabilityResult{}, ErrIncompleteNetworkEvidence
	}

	// 评估在领域执行（ADR-0046），按 UC-NR-002 候选评估层次推进：区域事实折成候选与
	// 缺口（层次 3）→ 承诺/偏好分界 → 逻辑路径可执行性（层次 4）→ 硬约束逐项评估
	// （层次 5）。事实本身不成立（重复候选、半截解析、缺格要求、指向空间外的事实）是
	// 端口坏了，响亮上抛，不混进`未形成判断`的统计。
	candidates, gaps, err := domain.EvaluateServiceAreas(evidence.ServiceAreas)
	if err != nil {
		return AssessParcelReachabilityResult{}, fmt.Errorf("evaluate service areas: %w", err)
	}
	candidates, err = domain.EvaluateRouteRequirements(candidates, evidence.RouteRequirements)
	if err != nil {
		return AssessParcelReachabilityResult{}, fmt.Errorf("evaluate route requirements: %w", err)
	}
	candidates, err = domain.EvaluatePathExecutability(candidates, evidence.PathExecutability)
	if err != nil {
		return AssessParcelReachabilityResult{}, fmt.Errorf("evaluate path executability: %w", err)
	}
	candidates, constraintGaps, err := domain.EvaluateHardConstraints(candidates, evidence.HardConstraints)
	if err != nil {
		return AssessParcelReachabilityResult{}, fmt.Errorf("evaluate hard constraints: %w", err)
	}
	gaps = append(gaps, constraintGaps...)

	finding, err := domain.ConcludeReachability(candidates, gaps)
	if err != nil {
		// 领域拒绝空候选空间而不是给结论。分不清是覆盖范围排除了目的地——那本该是一个带
		// 淘汰依据的候选——还是候选生成失败，两者都不能凭空断言，所以停在未形成判断。
		return handler.notFormed(command, CandidateSpaceNotEstablished), nil
	}

	// 时钟在结论形成之后才读，因此判断时间落在证据装配之后而非之前。
	record := ports.ReachabilityJudgmentRecord{
		Key:          command.Key,
		Finding:      finding,
		JudgedAt:     handler.clock.Now(),
		ViewRevision: evidence.ViewRevision,
	}
	saved, err := handler.store.Save(ctx, command.Correlation, record)
	if err != nil {
		// 判断没能越过提交边界就不算形成。用例要求此时只保存请求与处理尝试，不发布
		// 可达、不可达或资料不足——交回一个未落库的三值结果，下游就会引用一个查不回来的
		// 判断。
		return handler.notFormed(command, JudgmentStoreUnavailable), nil
	}
	switch saved {
	case ports.ReachabilityJudgmentSaved:
	case ports.ReachabilityJudgmentAlreadyRecorded:
		// 有人在本轮的查与写之间先落了判断。本方这一份不落库也不覆盖（AT-NR-028），
		// 读回赢家按同一条规则分流：同范围交回它的判断，异范围是冲突。
		winner, found, err := handler.store.FindByCorrelation(ctx, command.Key.TenantID, command.Correlation)
		if err != nil || !found {
			// 写入说已有、读回却拿不到，是竞争窗口里的暂态：停在未形成，重试自然读到赢家。
			return handler.notFormed(command, JudgmentStoreUnavailable), nil
		}
		if winner.Key.SameJudgmentScope(command.Key) {
			return handler.existingJudgment(ctx, command, winner), nil
		}
		return AssessParcelReachabilityResult{outcome: RequestConflict}, nil
	default:
		// 逐取值分派，不留兜底：端口日后新增一个写入结果时这里报错，而不是静默归入某一格。
		return AssessParcelReachabilityResult{}, ErrUnexpectedJudgmentSaveOutcome
	}

	return AssessParcelReachabilityResult{
		outcome:    JudgmentFormed,
		finding:    finding,
		hasFinding: true,
		judgedAt:   record.JudgedAt,
		handoff:    handler.handOff(ctx, command, record),
	}, nil
}

// existingJudgment 交回已越过提交边界的那一份判断，并把同一份发布意图再交一次。重发是
// 因为本上下文不记意图完没完成——只答`已有结果`就收工，一份首次发布失败的判断会永远停在
// 「本上下文已提交、下游从不知道」的状态，而这条路上没有别的东西会去补发（AT-NR-030）。
func (handler *AssessParcelReachabilityHandler) existingJudgment(
	ctx context.Context,
	command AssessParcelReachabilityCommand,
	record ports.ReachabilityJudgmentRecord,
) AssessParcelReachabilityResult {
	return AssessParcelReachabilityResult{
		outcome:    ExistingJudgment,
		finding:    record.Finding,
		hasFinding: true,
		judgedAt:   record.JudgedAt,
		handoff:    handler.handOff(ctx, command, record),
	}
}

// handOff 把已提交的判断交给适用下游，交不出去时交回发布续办引用。判断与时间取自落库的
// 那一份记录而不取本轮变量，重放路径上交的就是读回来的那一份。失败不改写判断，也不算进
// `未形成判断`——判断已经形成，要续办的是发布。
func (handler *AssessParcelReachabilityHandler) handOff(
	ctx context.Context,
	command AssessParcelReachabilityCommand,
	record ports.ReachabilityJudgmentRecord,
) ContinuationReference {
	if err := handler.downstream.HandOffReachabilityJudgment(ctx, ports.ReachabilityJudgmentHandoffIntent{
		Correlation:  command.Correlation,
		Key:          record.Key,
		Finding:      record.Finding,
		JudgedAt:     record.JudgedAt,
		ViewRevision: record.ViewRevision,
	}); err != nil {
		return continuationFor(command, "JUDGMENT_NOT_HANDED_OFF")
	}
	return ContinuationReference{}
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
		continuation: continuationFor(command, reason.String()),
	}
}

// continuationFor 由请求关联、判断范围与停摆标签共同派生，因此同一请求因同一原因停滞时
// 拿到的引用始终相同——这正是发起方能按稳定关联查询原次尝试而不必靠猜的原因。标签收字符
// 串而不收 NotFormedReason：发布续办不是一种`未形成判断`，硬塞进那个枚举会把「判断已形成
// 只欠发布」计进未形成统计。
func continuationFor(command AssessParcelReachabilityCommand, label string) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		label,
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
