package networkhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 授权改路决定口（ADR-0284 决定一）：授权角色据一份改路建议择一候选、形成改路决定。挂操作者渠道「运营决定」能力面，按「授权改路」
// 这一种认证；铸信封前判准入范围——决定形成生产事实（新计划进路由历史），与登记写面不同。

// OperatorDecision 是本包各口对应的运营决定种类，字面与操作者册的决定种类相同。
type OperatorDecision string

// OperatorDecisionAuthorizedReroute 是授权改路（ADR-0284 决定一）。
const OperatorDecisionAuthorizedReroute OperatorDecision = "AUTHORIZED_REROUTE"

// ErrOutsideAdmissionScope：这笔生产写不落在治理登记的生产权威区间内（403）——管治理的人去登准入对照与区间。登记写面与查阅面
// 不判准入，只有运营决定口答这一格。
var ErrOutsideAdmissionScope = errors.New("network http: production write is outside the admission scope")

const codeOutsideAdmissionScope = "OUTSIDE_ADMISSION_SCOPE"

// OperatorIdentity 是一次运营决定出示认证出的身份：租户与做出决定的操作者（「发行方#sub」）。
type OperatorIdentity struct {
	Tenant   domain.TenantID
	Operator string
}

// OperatorDecisionAuthenticator 认证一次运营决定出示，交回出示者的身份。失败只交回本包的格；从共享接入身份能力译过来的那一层在
// adapters/accessidentity。
type OperatorDecisionAuthenticator interface {
	AuthenticateOperatorDecision(ctx context.Context, bearerToken string, decision OperatorDecision) (OperatorIdentity, error)
}

// maxDecisionBytes 给一份决定请求设读取上限：四个短字符串，一兆字节已宽出几个量级，设限只为不让坏掉的调用方把请求读进内存。
const maxDecisionBytes = 1 << 20

// OperatorDecisionIntake 是操作者渠道「运营决定」能力面在本包的 Intake。先认证、后读载荷：令牌不过的调用方看不到载荷校验的结果。
type OperatorDecisionIntake struct {
	authenticator OperatorDecisionAuthenticator
}

var _ RerouteDecisionIntake = (*OperatorDecisionIntake)(nil)

func NewOperatorDecisionIntake(authenticator OperatorDecisionAuthenticator) (*OperatorDecisionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("network http: operator decision intake needs an authenticator")
	}
	return &OperatorDecisionIntake{authenticator: authenticator}, nil
}

// RerouteDecisionIntake 把已认证的决定请求翻译成命令。
type RerouteDecisionIntake interface {
	IntakeRerouteDecision(ctx context.Context, request *http.Request) (application.DecideRerouteCommand, error)
}

// RerouteDecider 是本口转交的决定编排；事务边界在它那一侧（判断键锁随事务释放）。
type RerouteDecider interface {
	Decide(ctx context.Context, command application.DecideRerouteCommand) (application.DecideRerouteResult, error)
}

// RerouteDecisionPayload 是决定口的线格式。载荷只有内容、没有身份（封闭解码）：租户与决定方取认证结果。包裹与建议关联一起指名
// 那份建议，所择候选取建议列出的候选标识，依据是决定方所凭的审批单一类引用。
type RerouteDecisionPayload struct {
	DeclaredParcelID string `json:"declaredParcelId"`
	Suggestion       string `json:"suggestion"`
	Candidate        string `json:"candidate"`
	Basis            string `json:"basis"`
}

// IntakeRerouteDecision 译授权改路决定（`/route-reroute-decisions`）。决定方取认证出的操作者，带渠道前缀记成
// `OPERATOR/<发行方>#<sub>`（与 PS、SA 的操作证据同一写法）：决定上记下的就是做出这一次决定的人，载荷里不收。
func (intake *OperatorDecisionIntake) IntakeRerouteDecision(
	ctx context.Context,
	request *http.Request,
) (application.DecideRerouteCommand, error) {
	none := application.DecideRerouteCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionAuthorizedReroute)
	if err != nil {
		return none, err
	}
	var payload RerouteDecisionPayload
	if err := decodeClosed(request.Body, &payload); err != nil {
		return none, err
	}
	command := application.DecideRerouteCommand{Tenant: operator.Tenant}
	if command.Parcel, err = domain.NewDeclaredParcelID(payload.DeclaredParcelID); err != nil {
		return none, fmt.Errorf("%w: declaredParcelId: %v", ErrMalformedRequest, err)
	}
	if command.Suggestion, err = domain.NewRequestCorrelationID(payload.Suggestion); err != nil {
		return none, fmt.Errorf("%w: suggestion: %v", ErrMalformedRequest, err)
	}
	if command.Candidate, err = domain.NewCandidateID(payload.Candidate); err != nil {
		return none, fmt.Errorf("%w: candidate: %v", ErrMalformedRequest, err)
	}
	if command.Basis, err = domain.NewRerouteDecisionBasisReference(payload.Basis); err != nil {
		return none, fmt.Errorf("%w: basis: %v", ErrMalformedRequest, err)
	}
	if command.DecidedBy, err = domain.NewAuthorizedRoleReference("OPERATOR/" + operator.Operator); err != nil {
		return none, fmt.Errorf("network http: decider: %w", err)
	}
	return command, nil
}

// decodeClosed 读一份封闭载荷：读不出、超限、不是 JSON、带未知键或尾随内容都答坏报文。
func decodeClosed(body io.Reader, target any) error {
	if body == nil {
		return fmt.Errorf("%w: empty body", ErrMalformedRequest)
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxDecisionBytes+1))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if len(raw) > maxDecisionBytes {
		return fmt.Errorf("%w: body exceeds %d bytes", ErrMalformedRequest, maxDecisionBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	if decoder.More() {
		return fmt.Errorf("%w: trailing content", ErrMalformedRequest)
	}
	return nil
}

// NewDecideRerouteEndpoint 交回授权改路决定口的 HTTP 入口：方法门 → Intake → 编排 → 逐名转写。
func NewDecideRerouteEndpoint(intake RerouteDecisionIntake, decider RerouteDecider) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeRerouteDecision(request.Context(), request)
		if err != nil {
			writeDecisionIntakeProblem(response, err)
			return
		}
		result, err := decider.Decide(request.Context(), command)
		if err != nil {
			// 决定成没成未知，或撞上不变量（建议与基线对不上之类）：状态码只报「没形成答案」（ADR-0022），成因去查服务端记录。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeRerouteDecisionAnswer(response, result)
	})
}

// rerouteDecisionResponse 是决定口的封闭响应形状。`decision` 随成立与已决定出现，后者是先成的那一份；`candidateReason` 只随此刻
// 重判不成立的三格出现；`undecidedReason` 与 `continuationReference` 只随未决出现。
type rerouteDecisionResponse struct {
	Outcome               string               `json:"outcome"`
	CandidateReason       string               `json:"candidateReason,omitempty"`
	UndecidedReason       string               `json:"undecidedReason,omitempty"`
	ContinuationReference string               `json:"continuationReference,omitempty"`
	Decision              *rerouteDecisionBody `json:"decision,omitempty"`
}

// rerouteDecisionBody 逐字段透出一份决定：新计划从 `effectiveFrom` 起、自 `effectiveFromNode` 生效（ADR-0284 决定四），时间一律
// RFC 3339 UTC。
type rerouteDecisionBody struct {
	Suggestion        string `json:"suggestion"`
	DeclaredParcelID  string `json:"declaredParcelId"`
	Candidate         string `json:"candidate"`
	OriginalPlan      string `json:"originalPlan"`
	NewPlanVersion    string `json:"newPlanVersion"`
	EffectiveFromNode string `json:"effectiveFromNode"`
	EffectiveFrom     string `json:"effectiveFrom"`
	Mode              string `json:"mode"`
	DecidedBy         string `json:"decidedBy"`
	Basis             string `json:"basis"`
	DecidedAt         string `json:"decidedAt"`
}

func writeRerouteDecisionAnswer(response http.ResponseWriter, result application.DecideRerouteResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := rerouteDecisionResponse{
		Outcome:               outcome,
		CandidateReason:       result.CandidateReason(),
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference().String(),
	}
	if recorded, present := result.Decision(); present {
		decision := recorded.Decision
		decidedBy, _ := decision.DecidedBy()
		body.Decision = &rerouteDecisionBody{
			Suggestion:        recorded.Suggestion.String(),
			DeclaredParcelID:  recorded.Key.DeclaredParcelID.String(),
			Candidate:         recorded.Candidate.String(),
			OriginalPlan:      decision.OriginalPlan().String(),
			NewPlanVersion:    decision.NewPlan().Version().String(),
			EffectiveFromNode: decision.EffectiveFromNode().String(),
			EffectiveFrom:     utcText(decision.NewPlan().EffectiveFrom()),
			Mode:              decision.Mode().String(),
			DecidedBy:         decidedBy.String(),
			Basis:             recorded.Basis.String(),
			DecidedAt:         utcText(decision.DecidedAt()),
		}
	}
	// ADR-0022：只有这一次真形成了决定用 201；已决定、过时、拒与未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.RerouteDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// writeDecisionIntakeProblem 比查阅与登记写面多出准入范围那一格：运营决定形成生产事实，铸信封前判准入。
func writeDecisionIntakeProblem(response http.ResponseWriter, err error) {
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
