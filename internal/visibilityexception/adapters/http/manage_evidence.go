package visibilityhttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是证据的三个运营决定口（UC-VE-007 步 2）：把收到的一份材料登成证据项、经调查对它作出采信或不采信的评价、为它
// 准备一份对外披露的脱敏版本。本产品不保存材料本体，只记内容指纹：材料放在租户约定的受限存储里。

// EvidenceManager 是三口转交的编排（application.ManageEvidenceHandler）。
type EvidenceManager interface {
	SubmitEvidence(ctx context.Context, command application.SubmitEvidenceCommand) (application.ManageEvidenceResult, error)
	Appraise(ctx context.Context, command application.AppraiseEvidenceCommand) (application.ManageEvidenceResult, error)
	PrepareDisclosure(ctx context.Context, command application.PrepareEvidenceDisclosureCommand) (application.ManageEvidenceResult, error)
}

type EvidenceDecisionIntake interface {
	IntakeEvidenceSubmission(ctx context.Context, request *http.Request) (application.SubmitEvidenceCommand, error)
	IntakeEvidenceAppraisal(ctx context.Context, request *http.Request) (application.AppraiseEvidenceCommand, error)
	IntakeEvidenceDisclosurePreparation(ctx context.Context, request *http.Request) (application.PrepareEvidenceDisclosureCommand, error)
}

var _ EvidenceDecisionIntake = (*OperatorDecisionIntake)(nil)

func NewSubmitEvidenceEndpoint(intake EvidenceDecisionIntake, manager EvidenceManager) http.Handler {
	return evidenceDecisionEndpoint(intake.IntakeEvidenceSubmission, manager.SubmitEvidence)
}

func NewAppraiseEvidenceEndpoint(intake EvidenceDecisionIntake, manager EvidenceManager) http.Handler {
	return evidenceDecisionEndpoint(intake.IntakeEvidenceAppraisal, manager.Appraise)
}

func NewPrepareEvidenceDisclosureEndpoint(intake EvidenceDecisionIntake, manager EvidenceManager) http.Handler {
	return evidenceDecisionEndpoint(intake.IntakeEvidenceDisclosurePreparation, manager.PrepareDisclosure)
}

// EvidenceSubmissionPayload 是登记口的线格式。provider 是交来这份材料的一方（客户、承运商、节点……），contentDigest 是
// 材料内容的指纹。没有收到时刻与评价两格：收到时刻是服务端登记的这一刻，评价起点恒为已收到。
type EvidenceSubmissionPayload struct {
	Provider      string `json:"provider"`
	ContentDigest string `json:"contentDigest"`
}

// EvidenceAppraisalPayload 是评价口的线格式。appraisal 取封闭词 CREDITED 或 DISCREDITED；basis 是评价的依据原文。
type EvidenceAppraisalPayload struct {
	EvidenceID string `json:"evidenceId"`
	Appraisal  string `json:"appraisal"`
	Basis      string `json:"basis"`
}

// EvidenceDisclosurePayload 是披露版本准备口的线格式。redactedDigest 是脱敏后内容的指纹，scope 是这份版本给谁看。原件
// 指纹不在载荷里：它取自证据项自己。
type EvidenceDisclosurePayload struct {
	EvidenceID     string `json:"evidenceId"`
	RedactedDigest string `json:"redactedDigest"`
	Scope          string `json:"scope"`
}

// IntakeEvidenceSubmission 按证据登记这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeEvidenceSubmission(ctx context.Context, request *http.Request) (application.SubmitEvidenceCommand, error) {
	none := application.SubmitEvidenceCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionEvidenceSubmission)
	if err != nil {
		return none, err
	}
	var payload EvidenceSubmissionPayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	provider, err := domain.NewEvidenceProviderReference(payload.Provider)
	if err != nil {
		return none, fmt.Errorf("%w: provider: %v", ErrMalformedRequest, err)
	}
	digest, err := domain.NewEvidenceContentDigest(payload.ContentDigest)
	if err != nil {
		return none, fmt.Errorf("%w: contentDigest: %v", ErrMalformedRequest, err)
	}
	return application.SubmitEvidenceCommand{TenantID: operator.Tenant, Provider: provider, Digest: digest, Operator: operator.Operator}, nil
}

// IntakeEvidenceAppraisal 按证据评价这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeEvidenceAppraisal(ctx context.Context, request *http.Request) (application.AppraiseEvidenceCommand, error) {
	none := application.AppraiseEvidenceCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionEvidenceAppraisal)
	if err != nil {
		return none, err
	}
	var payload EvidenceAppraisalPayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := domain.NewEvidenceItemID(payload.EvidenceID)
	if err != nil {
		return none, fmt.Errorf("%w: evidenceId: %v", ErrMalformedRequest, err)
	}
	appraisal, err := investigatedAppraisalFromWord(payload.Appraisal)
	if err != nil {
		return none, err
	}
	return application.AppraiseEvidenceCommand{
		TenantID: operator.Tenant, Evidence: id, Appraisal: appraisal, Basis: payload.Basis, Operator: operator.Operator,
	}, nil
}

// IntakeEvidenceDisclosurePreparation 按披露版本准备这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeEvidenceDisclosurePreparation(ctx context.Context, request *http.Request) (application.PrepareEvidenceDisclosureCommand, error) {
	none := application.PrepareEvidenceDisclosureCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionEvidenceDisclosurePreparation)
	if err != nil {
		return none, err
	}
	var payload EvidenceDisclosurePayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	id, err := domain.NewEvidenceItemID(payload.EvidenceID)
	if err != nil {
		return none, fmt.Errorf("%w: evidenceId: %v", ErrMalformedRequest, err)
	}
	redacted, err := domain.NewEvidenceContentDigest(payload.RedactedDigest)
	if err != nil {
		return none, fmt.Errorf("%w: redactedDigest: %v", ErrMalformedRequest, err)
	}
	// 披露范围去掉首尾空白再用：「甲」与「 甲」是同一个相对方，不该成两份披露版本。
	return application.PrepareEvidenceDisclosureCommand{TenantID: operator.Tenant, Evidence: id, Redacted: redacted, Scope: strings.TrimSpace(payload.Scope)}, nil
}

// investigatedAppraisalFromWord 只收经调查的两种评价。`已收到`不是一次评价的结论——它是起点，写不进来。
func investigatedAppraisalFromWord(raw string) (domain.EvidenceAppraisal, error) {
	for _, appraisal := range []domain.EvidenceAppraisal{domain.EvidenceCredited, domain.EvidenceDiscredited} {
		if appraisal.String() == raw {
			return appraisal, nil
		}
	}
	return domain.EvidenceAppraisalInvalid, fmt.Errorf("%w: appraisal=%q is not CREDITED or DISCREDITED", ErrMalformedRequest, raw)
}

func evidenceDecisionEndpoint[C any](
	intake func(context.Context, *http.Request) (C, error),
	handle func(context.Context, C) (application.ManageEvidenceResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeCaseDecisionIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeEvidenceDecision(response, result)
	})
}

// evidenceDecisionResponse 是三口的封闭响应形状。证据项只在它在场时交回；披露版本只在它成立时交回。原件指纹不交回给
// 披露版本准备口之外的调用方看的地方：这里交回的是登记时请求方自己给的那一个。
type evidenceDecisionResponse struct {
	Outcome        string `json:"outcome"`
	EvidenceID     string `json:"evidenceId,omitempty"`
	Appraisal      string `json:"appraisal,omitempty"`
	AppraisalBasis string `json:"appraisalBasis,omitempty"`
	SubmittedAt    string `json:"submittedAt,omitempty"`
	RedactedDigest string `json:"redactedDigest,omitempty"`
	Scope          string `json:"scope,omitempty"`
	PreparedAt     string `json:"preparedAt,omitempty"`
}

// writeEvidenceDecision 逐名转写。登记、评价或披露版本真落了才 201；其余业务答案 200。未决只在证据册读写不动或标识签
// 不出来时出现，是没形成答案——答 5xx。
func writeEvidenceDecision(response http.ResponseWriter, result application.ManageEvidenceResult) {
	if result.Outcome() == application.ManageEvidenceUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	const instant = "2006-01-02T15:04:05Z07:00"
	body := evidenceDecisionResponse{Outcome: outcome}
	if item, present := result.Evidence(); present {
		body.EvidenceID = item.ID().String()
		body.Appraisal = item.Appraisal().String()
		body.SubmittedAt = item.SubmittedAt().UTC().Format(instant)
		if basis, appraised := item.AppraisalBasis(); appraised {
			body.AppraisalBasis = basis
		}
	}
	if version, present := result.Disclosure(); present {
		body.RedactedDigest = version.Redacted().String()
		body.Scope = version.Scope()
		body.PreparedAt = version.PreparedAt().UTC().Format(instant)
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.EvidenceSubmitted, application.EvidenceAppraised, application.EvidenceDisclosurePrepared:
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
