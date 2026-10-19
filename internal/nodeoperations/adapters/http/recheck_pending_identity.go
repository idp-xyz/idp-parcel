package nodeopshttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是人工重核命令口（ADR-0199 决定三第二个触发）：复核页上由人指名一件待识别实物，按存下的号再问一次 PS。它是管理台
// 上作判断的口，归操作者渠道的「运营决定」能力面（ADR-0151 决定一），决定种类`待识别重核`。

// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）——阶段治理去登记区间。重核可能形成一版关联收寄并
// 交 PS 采用，是生产事实，运营决定口照 ADR-0151 决定三判准入。
var ErrOutsideAdmissionScope = errors.New("node operations http: production write is outside the admission scope")

const (
	codeOutsideAdmissionScope = "OUTSIDE_ADMISSION_SCOPE"
	codeJudgmentNotVisible    = "JUDGMENT_NOT_VISIBLE"
)

// IdentityRecheckIntake 把一次已认证的人工重核请求译成命令。租户只取认证结果（ADR-0151 决定二），请求体只指名判断版本。
type IdentityRecheckIntake interface {
	IntakeIdentityRecheck(ctx context.Context, request *http.Request) (application.RecheckPendingIdentityCommand, error)
}

// IdentityRecheckHandler 是本口转交的重核编排，装配负责把它包进一笔事务。
type IdentityRecheckHandler interface {
	Recheck(ctx context.Context, command application.RecheckPendingIdentityCommand) (application.RecheckPendingIdentityResult, error)
}

// NewRecheckPendingIdentityEndpoint 交回人工重核的 HTTP 入口。按 ADR-0022 映射：追加了一版（关联收寄、冲突版，或未核对那一件
// 问到了零候选而追加的「待识别 · 已核对」）答 201；判断不变、已有结果与不重核的各格都是形成了的答案，答 200 带 outcome——服务端
// 知道为什么不重核，这不是调用方的错（4xx），也不是
// 没形成答案（5xx）。指名本租户册上没有的版本答 404，与越权探针同答。身份核对读不动、落册或意图入队失败答 5xx：重核没形成
// 答案，册上什么也没追加。
func NewRecheckPendingIdentityEndpoint(intake IdentityRecheckIntake, handler IdentityRecheckHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeIdentityRecheck(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Recheck(request.Context(), command)
		switch {
		case errors.Is(err, application.ErrRecheckSubjectNotFound):
			writeProblem(response, http.StatusNotFound, codeJudgmentNotVisible)
			return
		case errors.Is(err, application.ErrRecheckCommandRejected):
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		case err != nil:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRecheckOutcome(response, result)
	})
}

// writeIntakeProblem 逐格映射 Intake 的拒答：每一格对一种恢复动作（ADR-0029），不合并。
func writeIntakeProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrOutsideAdmissionScope):
		writeProblem(response, http.StatusForbidden, codeOutsideAdmissionScope)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	case errors.Is(err, ErrMalformedRequest):
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
	default:
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
	}
}

// identityRecheckResponse 是本口的封闭响应形状。outcome 取重核结果枚举的原名。recheckedVersion 恒在场，是被重核的那一版；
// judgmentVersion 只随追加了一版的各格出现，是追加的那一版；currentVersion 与 recordedOutcome 只随已有结果出现，是当前有效
// 那一版与它的判断种类。unit 以下几格说的是答复所指的那一版（追加的、当前有效的，或被重核的那一版本身）；identityCheck 只在
// 那一版是待识别时出现（ADR-0199 决定五）；identityConflictCause 只在那一版记身份冲突、且记了来由时出现（ADR-0211 决定一），
// 取 ports.IdentityConflictCause 的原名，复核页据它分得开「这一件可再重核」（只因歧义信号）与「只经处置解开」。
type identityRecheckResponse struct {
	Outcome               string   `json:"outcome"`
	RecheckedVersion      string   `json:"recheckedVersion"`
	JudgmentVersion       string   `json:"judgmentVersion,omitempty"`
	CurrentVersion        string   `json:"currentVersion,omitempty"`
	RecordedOutcome       string   `json:"recordedOutcome,omitempty"`
	Unit                  string   `json:"unit,omitempty"`
	Node                  string   `json:"node,omitempty"`
	Association           string   `json:"association,omitempty"`
	Candidates            []string `json:"candidates,omitempty"`
	IdentityConflict      bool     `json:"identityConflict,omitempty"`
	IdentityConflictCause string   `json:"identityConflictCause,omitempty"`
	ServiceMarkers        []string `json:"serviceMarkers,omitempty"`
	ServiceResultCheck    string   `json:"serviceResultCheck,omitempty"`
	IdentityCheck         string   `json:"identityCheck,omitempty"`
}

func writeRecheckOutcome(response http.ResponseWriter, result application.RecheckPendingIdentityResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := identityRecheckResponse{Outcome: outcome, RecheckedVersion: result.Rechecked().String()}
	status := http.StatusOK
	if record, present := result.Record(); present {
		switch result.Outcome() {
		case application.RecheckIntakeFormed, application.RecheckIdentityConflict, application.RecheckIdentityNowChecked:
			body.JudgmentVersion = record.JudgmentVersion.String()
			status = http.StatusCreated
		case application.RecheckExistingResult:
			body.CurrentVersion = record.JudgmentVersion.String()
			body.RecordedOutcome = record.Kind.String()
		}
		if record.Intake.Version().String() != "" {
			body.Unit = record.Intake.Unit().String()
			body.Node = record.Intake.Node().String()
			if association, associated := record.Intake.Association(); associated {
				body.Association = association.String()
			}
		}
		for _, candidate := range record.Candidates {
			body.Candidates = append(body.Candidates, candidate.String())
		}
		body.IdentityConflict = record.IdentityConflict
		body.IdentityConflictCause = record.IdentityConflictCause.String()
		body.ServiceMarkers = append(body.ServiceMarkers, record.ServiceMarkers...)
		body.ServiceResultCheck = record.ServiceResultCheck.String()
		body.IdentityCheck = record.IdentityCheck.String()
	}
	writeJSON(response, status, body)
}

// OperatorDecision 是本包各口对应的运营决定种类（ADR-0151 决定一），字面与操作者册的决定种类相同。
type OperatorDecision string

// OperatorDecisionPendingIdentityRecheck 是复核页人工重核一件待识别实物。
const OperatorDecisionPendingIdentityRecheck OperatorDecision = "PENDING_IDENTITY_RECHECK"

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与提交操作者的引用（`<发行方>#<sub>`）。身份冲突处置把操作者写进处置
// 文档（ADR-0210 决定二），重核只用租户。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorDecisionAuthenticator 认证一次运营决定出示，交回出示者的租户与操作者。失败只交回本包的格：ErrAccessChannelNotConfigured、
// ErrOperatorCredentialRejected、ErrOperatorNotGranted、ErrOutsideAdmissionScope、ErrIdentityDependencyUnavailable；从共享
// 接入身份能力译过来的那一层在 adapters/accessidentity。
type OperatorDecisionAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在本包的 Intake（ADR-0151 决定一）。先认证、后解载荷：授予不过的调用方
// 看不到载荷校验的结果。载荷封闭：重核只有判断版本，身份冲突处置的各格见 identityDispositionPayload。
type OperatorDecisionIntake struct {
	authenticator OperatorDecisionAuthenticator
}

var _ IdentityRecheckIntake = (*OperatorDecisionIntake)(nil)

func NewOperatorDecisionIntake(authenticator OperatorDecisionAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("node operations http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

// identityRecheckPayload 是人工重核的线格式：指名一版判断。
type identityRecheckPayload struct {
	JudgmentVersion string `json:"judgmentVersion"`
}

// IntakeIdentityRecheck 译人工重核（`/node-operations/identity-rechecks`）。
func (intake *OperatorDecisionIntake) IntakeIdentityRecheck(ctx context.Context, request *http.Request) (application.RecheckPendingIdentityCommand, error) {
	none := application.RecheckPendingIdentityCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionPendingIdentityRecheck)
	if err != nil {
		return none, err
	}
	var payload identityRecheckPayload
	if err := decodeClosedDocument(request, &payload); err != nil {
		return none, err
	}
	if strings.TrimSpace(payload.JudgmentVersion) == "" {
		return none, fmt.Errorf("%w: judgmentVersion is required", ErrMalformedRequest)
	}
	version, err := domain.NewIntakeResultVersion(payload.JudgmentVersion)
	if err != nil {
		return none, fmt.Errorf("%w: judgmentVersion: %v", ErrMalformedRequest, err)
	}
	return application.RecheckPendingIdentityCommand{TenantID: operator.Tenant, Version: version}, nil
}

var _ IdentityRecheckIntake = UnconfiguredIntake{}

// IntakeIdentityRecheck 同 IntakeReception：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeIdentityRecheck(context.Context, *http.Request) (application.RecheckPendingIdentityCommand, error) {
	return application.RecheckPendingIdentityCommand{}, ErrAccessChannelNotConfigured
}
