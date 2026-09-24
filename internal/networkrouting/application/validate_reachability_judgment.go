package application

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ValidationOutcome 是提交前重校的封闭结果集合（`AT-PS-037` 的 NR 半边；分工出自 NR
// CONTEXT：消费方发现关键依据失效时**请求新的判断版本**，本用例只回答「失效没失效」，
// 不代替消费方重判）。
//
// `已换代`与`重校未形成`必须分开：前者重试一万次也还是换代了，消费方要以新关联请求新
// 判断；后者等依赖恢复重试同一次重校即可。`判断未找回`合并「查无此关联」与「范围不符」
// ——两者的恢复动作同为回去重新请求判断，分开就等于回答了调用方无权知道的「这个关联下
// 有没有判断」（同 ADR-0029 的合并理由）。
type ValidationOutcome uint8

const (
	ValidationOutcomeInvalid ValidationOutcome = iota
	JudgmentStillCurrent
	JudgmentSuperseded
	JudgmentNotFound
	ValidationNotFormed
	ValidationNotAccepted
)

func (outcome ValidationOutcome) String() string {
	switch outcome {
	case JudgmentStillCurrent:
		return "JUDGMENT_STILL_CURRENT"
	case JudgmentSuperseded:
		return "JUDGMENT_SUPERSEDED"
	case JudgmentNotFound:
		return "JUDGMENT_NOT_FOUND"
	case ValidationNotFormed:
		return "VALIDATION_NOT_FORMED"
	case ValidationNotAccepted:
		return "VALIDATION_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ValidateReachabilityJudgmentCommand 只回指请求关联，不收调用方带回来的判断本体：中间
// 状态由本上下文按关联保留（ADR-0027），调用方另给一份判断就能拿别的内容冒充原判断。
// Key 随行用于归属与范围核对——关联不是一张能力凭证。
type ValidateReachabilityJudgmentCommand struct {
	Correlation domain.RequestCorrelationID
	Key         domain.ReachabilityJudgmentKey
}

type ValidateReachabilityJudgmentResult struct {
	outcome      ValidationOutcome
	finding      domain.ReachabilityFinding
	hasFinding   bool
	judgedAt     time.Time
	reason       NotFormedReason
	continuation ContinuationReference
}

func (result ValidateReachabilityJudgmentResult) Outcome() ValidationOutcome {
	return result.outcome
}

// Finding 只在`仍然当前`时给出。`已换代`不带：交回一份，调用方会以为可以继续用它，而
// 它要做的是以新关联请求新判断。
func (result ValidateReachabilityJudgmentResult) Finding() (domain.ReachabilityFinding, bool) {
	return result.finding, result.hasFinding
}

func (result ValidateReachabilityJudgmentResult) JudgedAt() time.Time {
	return result.judgedAt
}

func (result ValidateReachabilityJudgmentResult) NotFormedReason() NotFormedReason {
	return result.reason
}

func (result ValidateReachabilityJudgmentResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type ValidateReachabilityJudgmentHandler struct {
	evidence ports.NetworkEvidenceView
	store    ports.ReachabilityJudgmentStore
}

func NewValidateReachabilityJudgmentHandler(
	evidence ports.NetworkEvidenceView,
	store ports.ReachabilityJudgmentStore,
) *ValidateReachabilityJudgmentHandler {
	return &ValidateReachabilityJudgmentHandler{evidence: evidence, store: store}
}

// Handle 在消费方提交决定之前核对一次判断是否仍基于当前网络证据视图。它不重判——重判是
// 消费方以新关联发起的新判断（NR CONTEXT 的分工句），这里只比对修订标识。
func (handler *ValidateReachabilityJudgmentHandler) Handle(
	ctx context.Context,
	command ValidateReachabilityJudgmentCommand,
) (ValidateReachabilityJudgmentResult, error) {
	// 先判身份再读任何权威，与形成判断同一条受理前提。
	if !command.Key.MinimumIdentityEstablished() || !command.Correlation.Valid() {
		return ValidateReachabilityJudgmentResult{outcome: ValidationNotAccepted}, nil
	}

	record, found, err := handler.store.FindByCorrelation(ctx, command.Key.TenantID, command.Correlation)
	if err != nil {
		return handler.notFormed(command, JudgmentStoreUnavailable), nil
	}
	// 查无此关联与范围不符同一个答案：可区分即可枚举同租户下别人的判断（ADR-0029 的
	// 合并理由在这里一字不差地成立）。
	if !found || !record.Key.SameJudgmentScope(command.Key) {
		return ValidateReachabilityJudgmentResult{outcome: JudgmentNotFound}, nil
	}
	if !record.ViewRevision.Valid() {
		// 库里躺着一份没有比对锚的判断：修订标识落库之前的旧记录，或适配器丢了字段。
		// 既不能确认也不能断言换代，只能未决——判成`仍然当前`等于免检放行。
		return handler.notFormed(command, JudgmentStoreUnavailable), nil
	}

	// 重校只比视图修订，而修订是目录修订锚，不随所携内容变；原判断所携的投影本上下文不留
	// （ADR-0075 决定三），这里也无从再带，所以不携带任何内容。
	evidence, configured, err := handler.evidence.LoadNetworkEvidence(ctx, command.Key, ports.RequestCarriedContent{})
	if !configured && err == nil {
		// 登记册未配置时同样既不能确认也不能断言换代：没有当前修订可比，原判断的
		// 有效性无从判断（ADR-0052）。
		return handler.notFormed(command, NetworkEvidenceNotConfigured), nil
	}
	if err != nil {
		// 权威读不到时原判断既不能被确认也不能被断言换代，保持可续办的未决——判成
		// 换代会让调用方去重判一份其实还好好的判断，判成仍然当前则是免检放行。
		return handler.notFormed(command, NetworkEvidenceUnavailable), nil
	}
	if !evidence.ViewRevision.Valid() {
		return ValidateReachabilityJudgmentResult{}, ErrIncompleteNetworkEvidence
	}

	if evidence.ViewRevision != record.ViewRevision {
		return ValidateReachabilityJudgmentResult{outcome: JudgmentSuperseded}, nil
	}
	return ValidateReachabilityJudgmentResult{
		outcome:    JudgmentStillCurrent,
		finding:    record.Finding,
		hasFinding: true,
		judgedAt:   record.JudgedAt,
	}, nil
}

func (handler *ValidateReachabilityJudgmentHandler) notFormed(
	command ValidateReachabilityJudgmentCommand,
	reason NotFormedReason,
) ValidateReachabilityJudgmentResult {
	return ValidateReachabilityJudgmentResult{
		outcome: ValidationNotFormed,
		reason:  reason,
		continuation: continuationFor(AssessParcelReachabilityCommand{
			Correlation: command.Correlation,
			Key:         command.Key,
		}, "VALIDATION_"+reason.String()),
	}
}
