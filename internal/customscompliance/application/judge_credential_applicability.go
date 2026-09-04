package application

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 凭证适用性判断——UC-CC-003 步 7「核验监管凭证身份、适用性、有效期和截至当前的可用
// 依据」的判断半边（票 mechanism-executor-triage/07 CC-a 的消费侧）。它是凭证登记册的
// 第一个生产读者：登记面接好而无人读，凭证仍然只是一行数据。
//
// 本用例只判、不记：步 7 写的「记录凭证门禁」那半留给就绪判断的编排——今天就绪判断是
// 带依据引用登记进来的事实（RegisterReadiness 收 ReadinessBasisReference），UC-CC-003
// 步 3–10 没有逐门禁计算的编排，凭证门禁作为其中一格的持久化随那条编排一起落，这里先
// 把「判得出」立起来。也不占用、不释放、不核销——那三件是凭证使用的生命周期
// （UC-CC-005 步 7/9、UC-CC-006 步 7），时点由真实程序定（PAR-CUS-04）。
//
// 四格分立：适用 / 不适用（程序、持有人、时点任一维不符）/ 凭证未登记 / 未决。「未登记」
// 与「不适用」刻意分开：前者是实例半边还没到、续办是登记；后者是判断结论、续办是由凭证
// 责任流程形成有效依据后重新评估（UC-CC-003 门禁表第 4 行）。把两者压成一格，租户上线
// 前每一次判断都会读成「凭证不适用」。

// CredentialApplicabilityOutcome 是一次凭证适用性判断的应用处理结果。
type CredentialApplicabilityOutcome uint8

const (
	CredentialApplicabilityOutcomeInvalid CredentialApplicabilityOutcome = iota
	CredentialApplicable
	CredentialNotApplicable
	CredentialNotRegistered
	CredentialApplicabilityNotAccepted
	CredentialApplicabilityUndecided
)

func (outcome CredentialApplicabilityOutcome) String() string {
	switch outcome {
	case CredentialApplicable:
		return "APPLICABLE"
	case CredentialNotApplicable:
		return "NOT_APPLICABLE"
	case CredentialNotRegistered:
		return "CREDENTIAL_NOT_REGISTERED"
	case CredentialApplicabilityNotAccepted:
		return "NOT_ACCEPTED"
	case CredentialApplicabilityUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// JudgeCredentialApplicabilityCommand 携带一次判断请求：对哪张凭证、在哪个程序、由谁、
// 于何时使用。At 是拟使用时点（业务时间），不是判断发生的系统时间——凭证有效期按前者算。
type JudgeCredentialApplicabilityCommand struct {
	TenantID   domain.TenantID
	Credential domain.CredentialID
	Procedure  domain.CustomsProcedureReference
	Holder     domain.CredentialHolderReference
	At         time.Time
}

type JudgeCredentialApplicabilityDeps struct {
	View ports.CredentialView
}

type JudgeCredentialApplicabilityHandler struct {
	deps JudgeCredentialApplicabilityDeps
}

func NewJudgeCredentialApplicabilityHandler(
	deps JudgeCredentialApplicabilityDeps,
) *JudgeCredentialApplicabilityHandler {
	return &JudgeCredentialApplicabilityHandler{deps: deps}
}

func (handler *JudgeCredentialApplicabilityHandler) Handle(
	ctx context.Context,
	command JudgeCredentialApplicabilityCommand,
) (CredentialApplicabilityOutcome, error) {
	if blankTenant(command.TenantID) ||
		command.Credential.String() == "" ||
		command.Procedure.String() == "" ||
		command.Holder.String() == "" ||
		command.At.IsZero() {
		return CredentialApplicabilityNotAccepted, nil
	}

	credential, found, err := handler.deps.View.LoadCredential(ctx, command.TenantID, command.Credential)
	if err != nil {
		return CredentialApplicabilityUndecided, nil
	}
	if !found {
		return CredentialNotRegistered, nil
	}

	switch err := credential.JudgeApplicability(command.Procedure, command.Holder, command.At); {
	case err == nil:
		return CredentialApplicable, nil
	case errors.Is(err, domain.ErrCredentialNotApplicable):
		return CredentialNotApplicable, nil
	default:
		// 领域只在输入零值时答 ErrInvalidCredential，而受理门已把零值拒在前面；走到这里
		// 是编程错误，不折成任何业务格。
		return CredentialApplicabilityOutcomeInvalid, err
	}
}
