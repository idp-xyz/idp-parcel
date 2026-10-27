package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// JudgeETAVisibilityOutcome 是 ETA 客户可见性判断的应用处理结果。
type JudgeETAVisibilityOutcome uint8

const (
	JudgeETAVisibilityOutcomeInvalid JudgeETAVisibilityOutcome = iota
	// ETAVisibilityJudged 是这个账户这件包裹上的首个判断。
	ETAVisibilityJudged
	// ETAVisibilityRejudged 是替代了前一版的新判断（新 ETA 版本或规则版本换版）。
	ETAVisibilityRejudged
	// ETAVisibilityExistingResult 是同一候选 ETA 版本、同一规则依据的重放：答已有判断。
	ETAVisibilityExistingResult
	// ETAVisibilityNoCandidate 说明没有内部 ETA 可判断。
	ETAVisibilityNoCandidate
	// ETAVisibilityOutsideRuleTarget 说明适用规则指名的目标里程碑不是这份 ETA 的里程碑。
	ETAVisibilityOutsideRuleTarget
	ETAVisibilityUndecided
	ETAVisibilityNotAccepted
)

func (outcome JudgeETAVisibilityOutcome) String() string {
	switch outcome {
	case ETAVisibilityJudged:
		return "JUDGED"
	case ETAVisibilityRejudged:
		return "REJUDGED"
	case ETAVisibilityExistingResult:
		return "EXISTING_RESULT"
	case ETAVisibilityNoCandidate:
		return "NO_CANDIDATE_ETA"
	case ETAVisibilityOutsideRuleTarget:
		return "OUTSIDE_RULE_TARGET"
	case ETAVisibilityUndecided:
		return "UNDECIDED"
	case ETAVisibilityNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// JudgeETAVisibilityUndecidedReason 指名判断停在哪一步。各依赖的恢复动作不同，不压成一格。
type JudgeETAVisibilityUndecidedReason uint8

const (
	JudgeETAVisibilityUndecidedReasonNone JudgeETAVisibilityUndecidedReason = iota
	CandidateETAUnavailable
	ETAVisibilityRuleUnavailable
	ETAVisibilityJudgementStoreUnavailable
	ETAVisibilityIdentityUnavailable
)

func (reason JudgeETAVisibilityUndecidedReason) String() string {
	switch reason {
	case CandidateETAUnavailable:
		return "CANDIDATE_ETA_UNAVAILABLE"
	case ETAVisibilityRuleUnavailable:
		return "ETA_VISIBILITY_RULE_UNAVAILABLE"
	case ETAVisibilityJudgementStoreUnavailable:
		return "ETA_VISIBILITY_JUDGEMENT_STORE_UNAVAILABLE"
	case ETAVisibilityIdentityUnavailable:
		return "ETA_VISIBILITY_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// JudgeETAVisibilityCommand 指名要判断的那份内部 ETA：（包裹，里程碑）下各来源线的当前版里，按候选规则
// 选出的那一版（domain.CandidateETAFrom）。账户由调用方给出而不是从 ETA 推导——预测面向包裹，不认识账户；
// 租户同理（ADR-0003）。
type JudgeETAVisibilityCommand struct {
	TenantID  domain.TenantID
	Customer  domain.CustomerAccountReference
	Parcel    domain.TrackedParcelReference
	Milestone domain.MilestoneReference
}

type JudgeETAVisibilityResult struct {
	outcome   JudgeETAVisibilityOutcome
	judgement domain.ETAVisibilityJudgement
	hasResult bool
	reason    JudgeETAVisibilityUndecidedReason
}

func (result JudgeETAVisibilityResult) Outcome() JudgeETAVisibilityOutcome {
	return result.outcome
}

// Judgement 只在判断成立（本轮或此前）时给出。
func (result JudgeETAVisibilityResult) Judgement() (domain.ETAVisibilityJudgement, bool) {
	return result.judgement, result.hasResult
}

func (result JudgeETAVisibilityResult) UndecidedReason() JudgeETAVisibilityUndecidedReason {
	return result.reason
}

type JudgeETAVisibilityDeps struct {
	Predictions ports.ETACurrentRead
	Rules       ports.ETAVisibilityRuleView
	Judgements  ports.ETAVisibilityJudgementStore
	Identities  ports.ETAVisibilityJudgementIdentityFactory
	Clock       ports.Clock
}

type JudgeETAVisibilityHandler struct {
	deps JudgeETAVisibilityDeps
}

func NewJudgeETAVisibilityHandler(deps JudgeETAVisibilityDeps) *JudgeETAVisibilityHandler {
	return &JudgeETAVisibilityHandler{deps: deps}
}

// Handle 把一份内部 ETA 推进到 ETA 客户可见性判断（UC-VE-003 主流程第 3、6、8 步）：受理（租户、账户、
// 包裹、里程碑缺一即未受理，不读任何依赖）→ 一次读出各来源线的当前版并选候选（运营企业派生的优先，没有才取
// 承运商提供的；没有任何一版就不判，不用计划时间凑）→
// 固定判断时点并按它取适用规则（缺失与冲突是确定的业务答案，落「待判断」，不是错误）→ 对候选作发现
// （达标可见、未达不可见、规则缺失或冲突待判断）→ 发现与当前判断相同即重放，答已有判断，否则首版
// 判断或替代判断指回前版，原判断不覆盖。
//
// 本编排只写判断：不形成客户视图、客户可见异常或通知，也不修改内部 ETA、计划或承诺（UC-VE-003
// 主流程第 8 步）。判断换版之后谁去重派生客户视图，归触发它的消费链，不在这里。
func (handler *JudgeETAVisibilityHandler) Handle(
	ctx context.Context,
	command JudgeETAVisibilityCommand,
) (JudgeETAVisibilityResult, error) {
	if command.TenantID.String() == "" ||
		command.Customer.String() == "" ||
		command.Parcel.String() == "" ||
		command.Milestone.String() == "" {
		return JudgeETAVisibilityResult{outcome: ETAVisibilityNotAccepted}, nil
	}

	currents, err := handler.deps.Predictions.ListCurrent(ctx, command.TenantID, command.Parcel, command.Milestone)
	if err != nil {
		return undecidedJudgement(CandidateETAUnavailable), nil
	}
	eta, found := domain.CandidateETAFrom(currents)
	if !found {
		return JudgeETAVisibilityResult{outcome: ETAVisibilityNoCandidate}, nil
	}

	judgedAt := handler.deps.Clock.Now()
	basis, err := handler.deps.Rules.AssessETAVisibilityRule(ctx, command.TenantID, command.Customer, judgedAt)
	if err != nil {
		return undecidedJudgement(ETAVisibilityRuleUnavailable), nil
	}
	finding, err := domain.AssessETAVisibility(eta, basis)
	if errors.Is(err, domain.ErrETAOutsideRuleTarget) {
		return JudgeETAVisibilityResult{outcome: ETAVisibilityOutsideRuleTarget}, nil
	}
	if err != nil {
		return JudgeETAVisibilityResult{}, fmt.Errorf("assess eta visibility: %w", err)
	}

	current, hasCurrent, err := handler.deps.Judgements.FindCurrent(ctx, command.TenantID, command.Customer, command.Parcel)
	if err != nil {
		return undecidedJudgement(ETAVisibilityJudgementStoreUnavailable), nil
	}
	if hasCurrent && current.Finding() == finding {
		// 同一候选 ETA 版本、同一规则依据重放：不签新身份、不换代。
		return JudgeETAVisibilityResult{outcome: ETAVisibilityExistingResult, judgement: current, hasResult: true}, nil
	}

	version, err := handler.deps.Identities.NextETAVisibilityJudgementVersionID(ctx)
	if err != nil {
		return undecidedJudgement(ETAVisibilityIdentityUnavailable), nil
	}
	outcome := ETAVisibilityJudged
	var judgement domain.ETAVisibilityJudgement
	if hasCurrent {
		// 依据变了：新判断换版本指回前版，原判断不覆盖。
		judgement, err = current.Supersede(version, finding, judgedAt)
		outcome = ETAVisibilityRejudged
	} else {
		judgement, err = domain.FormETAVisibilityJudgement(version, command.Customer, finding, judgedAt)
	}
	if err != nil {
		return JudgeETAVisibilityResult{}, fmt.Errorf("form eta visibility judgement: %w", err)
	}
	if err := handler.deps.Judgements.Save(ctx, command.TenantID, judgement); err != nil {
		return undecidedJudgement(ETAVisibilityJudgementStoreUnavailable), nil
	}
	return JudgeETAVisibilityResult{outcome: outcome, judgement: judgement, hasResult: true}, nil
}

func undecidedJudgement(reason JudgeETAVisibilityUndecidedReason) JudgeETAVisibilityResult {
	return JudgeETAVisibilityResult{outcome: ETAVisibilityUndecided, reason: reason}
}
