package visibilityhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是预计到达（ETA）的手工录入口（dox/decisions.md「产品形态」）：没有预测来源对接时，操作员把承运商告知的、或
// 运营自己估的预计到达录进来。进的是 ETA 形成的同一只编排：按（包裹，里程碑，来源）成线，内容变了就换版、历史不覆盖。
// 录进来的预测与任何预测一样，还要经 ETA 可见性判断才对客展示。

// OperatorDecisionETAEntry 是人工录入预计到达。
const OperatorDecisionETAEntry OperatorDecision = "ETA_ENTRY"

// ManualETAInputsPrefix 是人工录入的预测在输入版本上的前缀：与预测来源送来的输入版本不在一个名字空间里。
const ManualETAInputsPrefix = "ETAMANUAL/"

// manualETAModel 是人工录入的预测在「模型」一格上的词：这一版不是哪个模型算的，是人照材料录的。
const manualETAModel = "MANUAL_ENTRY"

// ETAEntryPayload 是手工录入口的线格式。source 取 CARRIER_PROVIDED（承运商告知的）或 OPERATOR_DERIVED（运营自己估的），
// 两种各成一条线；rangeFrom、rangeTo 是预计到达的时间范围（RFC 3339，rangeTo 必须晚于 rangeFrom——预测是一个范围，
// 只知道一个时间点时照材料给出前后的余量）；confidence 是
// 可信度；original 是所凭原件的编号或存档引用，必填。输入版本不由调用方给：按内容派生，同一份内容重录是重放，改了
// 任何一格就是新版本。
type ETAEntryPayload struct {
	Parcel     string `json:"parcel"`
	Milestone  string `json:"milestone"`
	Source     string `json:"source"`
	RangeFrom  string `json:"rangeFrom"`
	RangeTo    string `json:"rangeTo"`
	Confidence string `json:"confidence"`
	Original   string `json:"original"`
}

// ETAEntryIntake 是手工录入口的 Intake。
type ETAEntryIntake interface {
	IntakeETAEntry(ctx context.Context, request *http.Request) (application.FormETACommand, error)
}

// ETAFormer 是本口转交的编排（包一笔事务）。
type ETAFormer interface {
	FormETA(ctx context.Context, command application.FormETACommand) (application.FormETAResult, error)
}

var _ ETAEntryIntake = (*OperatorDecisionIntake)(nil)

// IntakeETAEntry 按人工录入预计到达这一种决定认证。租户与录入人只取认证结果。
func (intake *OperatorDecisionIntake) IntakeETAEntry(ctx context.Context, request *http.Request) (application.FormETACommand, error) {
	none := application.FormETACommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionETAEntry)
	if err != nil {
		return none, err
	}
	var payload ETAEntryPayload
	if err := decodeClosedDecision(request.Body, &payload); err != nil {
		return none, err
	}
	original := strings.TrimSpace(payload.Original)
	if original == "" || len(original) > 500 || strings.IndexFunc(original, unicode.IsControl) >= 0 {
		return none, fmt.Errorf("%w: original is required for a manually entered ETA", ErrMalformedRequest)
	}
	for field, value := range map[string]string{"parcel": payload.Parcel, "milestone": payload.Milestone, "confidence": payload.Confidence} {
		if len(strings.TrimSpace(value)) > 200 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return none, fmt.Errorf("%w: %s is too long or carries control characters", ErrMalformedRequest, field)
		}
	}
	parcel, err := domain.NewTrackedParcelReference(strings.TrimSpace(payload.Parcel))
	if err != nil {
		return none, fmt.Errorf("%w: parcel: %v", ErrMalformedRequest, err)
	}
	milestone, err := domain.NewMilestoneReference(strings.TrimSpace(payload.Milestone))
	if err != nil {
		return none, fmt.Errorf("%w: milestone: %v", ErrMalformedRequest, err)
	}
	confidence, err := domain.NewConfidenceReference(strings.TrimSpace(payload.Confidence))
	if err != nil {
		return none, fmt.Errorf("%w: confidence: %v", ErrMalformedRequest, err)
	}
	var source domain.ETASourceKind
	for _, kind := range []domain.ETASourceKind{domain.CarrierProvidedETA, domain.OperatorDerivedETA} {
		if kind.String() == payload.Source {
			source = kind
		}
	}
	if source.String() == "" {
		return none, fmt.Errorf("%w: source=%q is not CARRIER_PROVIDED or OPERATOR_DERIVED", ErrMalformedRequest, payload.Source)
	}
	from, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.RangeFrom))
	if err != nil {
		return none, fmt.Errorf("%w: rangeFrom is not an RFC 3339 instant", ErrMalformedRequest)
	}
	to, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.RangeTo))
	if err != nil {
		return none, fmt.Errorf("%w: rangeTo is not an RFC 3339 instant", ErrMalformedRequest)
	}
	// 截到微秒：库里的时刻只到微秒，带着纳秒算指纹，同一个范围换个小数位重录会多出一版。
	from, to = from.UTC().Truncate(time.Microsecond), to.UTC().Truncate(time.Microsecond)
	if !to.After(from) {
		return none, fmt.Errorf("%w: rangeTo must be after rangeFrom", ErrMalformedRequest)
	}
	if from.Year() < 2000 {
		return none, fmt.Errorf("%w: rangeFrom is implausibly early", ErrMalformedRequest)
	}
	// 输入版本按内容派生，用解析后的 UTC 时刻：同一份内容换一种时间写法重录是重放。原件也进指纹——同一个时间范围凭
	// 另一份材料再录，是一次新的告知。
	digest := sha256.Sum256([]byte(strings.Join([]string{
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano), confidence.String(), original,
	}, "\x00")))
	inputs, err := domain.NewPredictionInputsReference(ManualETAInputsPrefix + hex.EncodeToString(digest[:16]))
	if err != nil {
		return none, err
	}
	model, err := domain.NewPredictionModelReference(manualETAModel)
	if err != nil {
		return none, err
	}
	return application.FormETACommand{
		TenantID: operator.Tenant, Parcel: parcel, Milestone: milestone, Source: source, Inputs: inputs, Model: model,
		RangeFrom: from.UTC(), RangeTo: to.UTC(), Confidence: confidence,
		EnteredBy: "OPERATOR/" + operator.Operator, Original: original,
	}, nil
}

type etaEntryResponse struct {
	Outcome      string `json:"outcome"`
	Version      string `json:"version,omitempty"`
	Parcel       string `json:"parcel,omitempty"`
	Milestone    string `json:"milestone,omitempty"`
	Source       string `json:"source,omitempty"`
	RangeFrom    string `json:"rangeFrom,omitempty"`
	RangeTo      string `json:"rangeTo,omitempty"`
	Confidence   string `json:"confidence,omitempty"`
	PriorVersion string `json:"priorVersion,omitempty"`
}

// NewEnterETAEndpoint 交回预计到达的手工录入口（POST /eta-prediction-manual-entries）。形成或换版答 201；同一份内容
// 重录、内容对应的是已被接替的旧版本（交回此刻的当前版）、未受理是答案（200）；预测册读写不动、意图没交出去是没形成答案。
func NewEnterETAEndpoint(intake ETAEntryIntake, former ETAFormer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeETAEntry(request.Context(), request)
		if err != nil {
			writeCaseDecisionIntakeProblem(response, err)
			return
		}
		result, err := former.FormETA(request.Context(), command)
		if err != nil || result.Outcome() == application.FormETAUndecided {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		body := etaEntryResponse{Outcome: outcome}
		if eta, has := result.ETA(); has {
			from, to := eta.Range()
			body.Version, body.Parcel, body.Milestone = eta.Version().String(), eta.Parcel().String(), eta.Milestone().String()
			body.Source, body.Confidence = eta.Source().String(), eta.Confidence().String()
			body.RangeFrom, body.RangeTo = from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)
			if prior, refreshed := eta.PriorVersion(); refreshed {
				body.PriorVersion = prior.String()
			}
		}
		status := http.StatusOK
		switch result.Outcome() {
		case application.ETAFormed, application.ETARefreshed:
			status = http.StatusCreated
		}
		writeJSON(response, status, body)
	})
}
