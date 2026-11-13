package customshttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 本文件是申报的线下递交标记口（dox/decisions.md「产品形态」）：没有对接报关行或单一窗口时，关务人员在线下递交一版
// 申报之后回来记「已递交」。

// OperatorDecisionCustomsManualLodgement 是线下递交之后的「已递交」标记。
const OperatorDecisionCustomsManualLodgement OperatorDecision = "CUSTOMS_MANUAL_LODGEMENT"

// ManualLodgementPayload 是标记口的线格式。version 是提交版本；lodgedAt 是实际递交的时刻（RFC 3339，可不填，不填取
// 现在，不得晚于现在）；reference 是对方给的受理号，可不填；original 是递交凭据的编号或存档引用，必填。
type ManualLodgementPayload struct {
	Version   string `json:"version"`
	LodgedAt  string `json:"lodgedAt"`
	Reference string `json:"reference"`
	Original  string `json:"original"`
}

// ManualLodgementIntake 是标记口的 Intake。
type ManualLodgementIntake interface {
	IntakeManualLodgement(ctx context.Context, request *http.Request) (application.RecordManualLodgementCommand, error)
}

// ManualLodgementRecorder 是本口转交的编排（包一笔事务）。
type ManualLodgementRecorder interface {
	Record(ctx context.Context, command application.RecordManualLodgementCommand) (application.RecordManualLodgementResult, error)
}

var _ ManualLodgementIntake = (*OperatorDecisionIntake)(nil)

// IntakeManualLodgement 按线下递交标记这一种决定认证。递交人只取认证结果。
func (intake *OperatorDecisionIntake) IntakeManualLodgement(ctx context.Context, request *http.Request) (application.RecordManualLodgementCommand, error) {
	none := application.RecordManualLodgementCommand{}
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionCustomsManualLodgement)
	if err != nil {
		return none, err
	}
	var payload ManualLodgementPayload
	if err := decodeClosedPayload(request.Body, &payload); err != nil {
		return none, err
	}
	original, reference := strings.TrimSpace(payload.Original), strings.TrimSpace(payload.Reference)
	hasControl := func(text string) bool { return strings.IndexFunc(text, unicode.IsControl) >= 0 }
	if original == "" || len(original) > 500 || hasControl(original) || hasControl(reference) {
		return none, fmt.Errorf("%w: original is required for a manual lodgement record", ErrMalformedRequest)
	}
	command := application.RecordManualLodgementCommand{
		TenantID: operator.Tenant, Version: strings.TrimSpace(payload.Version), Reference: reference, Original: original,
		LodgedBy: "OPERATOR/" + operator.Operator,
	}
	if raw := strings.TrimSpace(payload.LodgedAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return none, fmt.Errorf("%w: lodgedAt is not an RFC 3339 instant", ErrMalformedRequest)
		}
		command.LodgedAt = at.UTC()
	}
	return command, nil
}

type manualLodgementResponse struct {
	Outcome   string `json:"outcome"`
	Version   string `json:"version,omitempty"`
	Sequence  int    `json:"sequence,omitempty"`
	Result    string `json:"result,omitempty"`
	SentAt    string `json:"sentAt,omitempty"`
	Reference string `json:"reference,omitempty"`
}

// NewRecordManualLodgementEndpoint 交回线下递交标记口（POST /customs-declaration-manual-lodgements）。这次记下了答
// 201；已发过、不再就绪、未受理、受理号已被占都是答案（200）；册读写不动是没形成答案。
func NewRecordManualLodgementEndpoint(intake ManualLodgementIntake, recorder ManualLodgementRecorder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeManualLodgement(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := recorder.Record(request.Context(), command)
		if err != nil || result.Outcome() == application.ManualLodgementUndecided {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome().String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		body := manualLodgementResponse{Outcome: outcome}
		if fact, has := result.Fact(); has {
			body.Version, body.Sequence = fact.Version().String(), fact.Sequence()
			body.Result = fact.Outcome().String()
			body.SentAt = fact.SentAt().UTC().Format(time.RFC3339Nano)
			body.Reference = fact.ChannelMessage()
		}
		status := http.StatusOK
		if result.Outcome() == application.ManualLodgementRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, body)
	})
}
