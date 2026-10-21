package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 委托协议换版口（ADR-0231 决定三）。运营人员指名一份运输委托自哪一业务时刻起改按同一协议对象的后继版本采购，是管理台上作出的生产
// 决定，挂操作者渠道的「运营决定」能力面、按委托协议换版这一种认证：能提交、能取消委托的不因此能改一份在途委托的协议版本。路径取读面
// 册名前缀 `transport-fulfillment-`，理由同委托提交口。

// OperatorDecisionTransportCommissionAgreementRevision 是委托协议换版（ADR-0231 决定三）。
const OperatorDecisionTransportCommissionAgreementRevision OperatorDecision = "TRANSPORT_COMMISSION_AGREEMENT_REVISION"

var _ CommissionAgreementRevisionIntake = (*OperatorDecisionIntake)(nil)

// CommissionAgreementRevisionIntake 把已认证的运营写请求翻译成换版命令。
type CommissionAgreementRevisionIntake interface {
	IntakeCommissionAgreementRevision(ctx context.Context, request *http.Request) (application.ReviseCommissionAgreementCommand, error)
}

// CommissionAgreementReviser 是本适配器转交的应用编排。
type CommissionAgreementReviser interface {
	Revise(ctx context.Context, command application.ReviseCommissionAgreementCommand) (application.CommissionAgreementRevisionResult, error)
}

// CommissionAgreementRevisionPayload 是换版口的线格式。载荷只有内容、没有身份（封闭解码）：租户与决定人取认证结果。起用时刻是业务时间，
// 由操作者给；依据是换版所凭的那一份运营决定，同一份依据只换一次版。
type CommissionAgreementRevisionPayload struct {
	Commission                string `json:"commission"`
	SuccessorAgreementObject  string `json:"successorAgreementObject"`
	SuccessorAgreementVersion string `json:"successorAgreementVersion"`
	AppliesFrom               string `json:"appliesFrom"`
	Basis                     string `json:"basis"`
}

// Command 把载荷连同认证出的身份翻成换版命令。时刻解不出是坏报文（400）；各格成不成立由编排答。
func (payload CommissionAgreementRevisionPayload) Command(operator OperatorIdentity) (application.ReviseCommissionAgreementCommand, error) {
	if operator.Tenant.String() == "" {
		return application.ReviseCommissionAgreementCommand{}, ErrOperatorIdentityMissing
	}
	appliesFrom, err := parseOptionalInstant("appliesFrom", payload.AppliesFrom)
	if err != nil {
		return application.ReviseCommissionAgreementCommand{}, err
	}
	return application.ReviseCommissionAgreementCommand{
		TenantID:         operator.Tenant,
		DecidedBy:        operator.Operator,
		Commission:       payload.Commission,
		SuccessorObject:  payload.SuccessorAgreementObject,
		SuccessorVersion: payload.SuccessorAgreementVersion,
		AppliesFrom:      appliesFrom,
		Basis:            payload.Basis,
	}, nil
}

// IntakeCommissionAgreementRevision 译委托协议换版（`/transport-fulfillment-transport-commission-agreement-revisions`）。决定人取认证出的
// 操作者：换版记下的决定人就是做出这一次决定的人，载荷里不收。
func (intake *OperatorDecisionIntake) IntakeCommissionAgreementRevision(ctx context.Context, request *http.Request) (application.ReviseCommissionAgreementCommand, error) {
	operator, err := intake.identityFor(ctx, request, OperatorDecisionTransportCommissionAgreementRevision)
	if err != nil {
		return application.ReviseCommissionAgreementCommand{}, err
	}
	var payload CommissionAgreementRevisionPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return application.ReviseCommissionAgreementCommand{}, err
	}
	return payload.Command(operator)
}

// NewReviseCommissionAgreementEndpoint 交回换版口的 HTTP 入口。
func NewReviseCommissionAgreementEndpoint(intake CommissionAgreementRevisionIntake, reviser CommissionAgreementReviser) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionAgreementRevisionResult, error, bool) {
		command, err := intake.IntakeCommissionAgreementRevision(request.Context(), request)
		if err != nil {
			return application.CommissionAgreementRevisionResult{}, err, false
		}
		result, err := reviser.Revise(request.Context(), command)
		return result, err, true
	}, writeCommissionAgreementRevisionOutcome)
}

// commissionAgreementRevisionResponse 是换版口的封闭响应形状。`revision` 随换版成立、已有与冲突出现，后两格是同一份依据在册的那一版；
// `agreedSupplier` 只随 `PROVIDER_MISMATCH` 出现，是后继那一版约定的供应商。
type commissionAgreementRevisionResponse struct {
	Outcome               string                           `json:"outcome"`
	UndecidedReason       string                           `json:"undecidedReason,omitempty"`
	ContinuationReference string                           `json:"continuationReference,omitempty"`
	AgreedSupplier        string                           `json:"agreedSupplier,omitempty"`
	Revision              *commissionAgreementRevisionBody `json:"revision,omitempty"`
}

// commissionAgreementRevisionBody 逐字段透出一次换版：`revision` 是它在委托上的号（1 是提交那一版），`agreement` 与 `legalEntity` 是核过的
// 后继那一版的快照与采购责任法人，`appliesFrom` 是业务时间，`registeredAt` 是登记时刻。时间一律 RFC 3339 UTC。
type commissionAgreementRevisionBody struct {
	Revision         int    `json:"revision"`
	AgreementObject  string `json:"agreementObject"`
	AgreementVersion string `json:"agreementVersion"`
	Agreement        string `json:"agreement"`
	LegalEntity      string `json:"legalEntity"`
	AppliesFrom      string `json:"appliesFrom"`
	Basis            string `json:"basis"`
	DecidedBy        string `json:"decidedBy"`
	RegisteredAt     string `json:"registeredAt"`
}

func commissionAgreementRevisionBodyOf(revision domain.CommissionAgreementRevision) *commissionAgreementRevisionBody {
	return &commissionAgreementRevisionBody{
		Revision:         revision.Number(),
		AgreementObject:  revision.Successor().Object(),
		AgreementVersion: revision.Successor().Version(),
		Agreement:        revision.Snapshot().String(),
		LegalEntity:      revision.LegalEntity().String(),
		AppliesFrom:      rfc3339(revision.AppliesFrom()),
		Basis:            revision.Basis().String(),
		DecidedBy:        revision.DecidedBy().String(),
		RegisteredAt:     rfc3339(revision.RegisteredAt()),
	}
}

func writeCommissionAgreementRevisionOutcome(response http.ResponseWriter, result application.CommissionAgreementRevisionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := commissionAgreementRevisionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		AgreedSupplier:        result.AgreedSupplier(),
	}
	if revision, present := result.Revision(); present {
		body.Revision = commissionAgreementRevisionBodyOf(revision)
	}
	// ADR-0022：只有这次真落下一版用 201；已有、冲突、前置条件不满足的各格、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.CommissionAgreementRevised {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
