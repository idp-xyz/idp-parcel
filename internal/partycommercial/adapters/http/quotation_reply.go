package commercialhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 报价答复的在线口。写面与发出同一张登记写面。
// 载荷不收租户、代录人与登记时刻：租户与代录人取认证信封，登记时刻取服务端时钟。
// 客户答复时刻、证据索引与决定由代录人交来。

type QuotationReplyIntake interface {
	IntakeQuotationReply(ctx context.Context, request *http.Request, now time.Time) (application.RecordQuotationReplyCommand, error)
}

var _ QuotationReplyIntake = (*OperatorRegistryIntake)(nil)

type quotationReplyDocument struct {
	TenantID        json.RawMessage `json:"tenantId"`
	Recorder        json.RawMessage `json:"recorder"`
	RecordedAt      json.RawMessage `json:"recordedAt"`
	QuotationID     string          `json:"quotationId"`
	Decision        string          `json:"decision"`
	RepliedAt       string          `json:"repliedAt"`
	EvidenceIndex   string          `json:"evidenceIndex"`
	RejectionReason string          `json:"rejectionReason"`
	ContentDigest   string          `json:"contentDigest"`
}

func (intake *OperatorRegistryIntake) IntakeQuotationReply(ctx context.Context, request *http.Request, now time.Time) (application.RecordQuotationReplyCommand, error) {
	var none application.RecordQuotationReplyCommand
	operator, err := intake.authenticate(ctx, request)
	if err != nil {
		return none, err
	}
	document, err := decodeClosedDocument[quotationReplyDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return none, err
	}
	if err := refuseSelfReportedField(document.Recorder, "recorder"); err != nil {
		return none, err
	}
	if err := refuseSelfReportedField(document.RecordedAt, "recordedAt"); err != nil {
		return none, err
	}
	quotationID, err := domain.NewQuotationID(document.QuotationID)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	decision, known := domain.QuotationReplyDecisionNamed(document.Decision)
	if !known {
		return none, fmt.Errorf("%w: decision must be ACCEPTED or REJECTED", ErrMalformedRequest)
	}
	repliedAt, err := time.Parse(time.RFC3339, document.RepliedAt)
	if err != nil {
		return none, fmt.Errorf("%w: repliedAt must be RFC3339", ErrMalformedRequest)
	}
	if document.ContentDigest == "" {
		return none, fmt.Errorf("%w: contentDigest is required", ErrMalformedRequest)
	}
	if now.IsZero() {
		return none, fmt.Errorf("party commercial http: quotation reply clock is unset")
	}
	return application.RecordQuotationReplyCommand{
		Tenant: operator.Tenant, QuotationID: quotationID, Decision: decision,
		RepliedAt: repliedAt.UTC(), EvidenceIndex: document.EvidenceIndex, RejectionReason: document.RejectionReason,
		ContentDigest: document.ContentDigest, Recorder: operator.Operator.Reference(), RecordedAt: now.UTC(),
	}, nil
}

type QuotationReplyRecorder interface {
	Handle(context.Context, application.RecordQuotationReplyCommand) (application.RecordQuotationReplyResult, error)
}

var _ QuotationReplyRecorder = (*application.RecordQuotationReplyHandler)(nil)

func NewRecordQuotationReplyEndpoint(intake QuotationReplyIntake, recorder QuotationReplyRecorder, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		if now == nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		command, err := intake.IntakeQuotationReply(request.Context(), request, now())
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := recorder.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeQuotationReplyAnswer(response, result)
	})
}

func writeQuotationReplyAnswer(response http.ResponseWriter, result application.RecordQuotationReplyResult) {
	if result.Outcome.String() == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome == application.QuotationReplyRecorded {
		status = http.StatusCreated
	}
	body := quotationReplyAnswer{Outcome: result.Outcome.String(), Conclusion: result.Conclusion}
	if result.Reply.QuotationID().String() != "" {
		recorded := quotationReplyBodyOf(result.Reply)
		body.Reply = &recorded
	}
	writeJSON(response, status, body)
}

type quotationReplyAnswer struct {
	Outcome    string              `json:"outcome"`
	Conclusion string              `json:"conclusion"`
	Reply      *quotationReplyBody `json:"reply,omitempty"`
}

type quotationReplyBody struct {
	QuotationID     string `json:"quotationId"`
	Decision        string `json:"decision"`
	RepliedAt       string `json:"repliedAt"`
	EvidenceIndex   string `json:"evidenceIndex"`
	RejectionReason string `json:"rejectionReason,omitempty"`
	RecordedBy      string `json:"recordedBy"`
	RecordedAt      string `json:"recordedAt"`
}

func quotationReplyBodyOf(reply domain.QuotationReply) quotationReplyBody {
	return quotationReplyBody{
		QuotationID: reply.QuotationID().String(), Decision: reply.Decision().String(),
		RepliedAt: rfc3339(reply.RepliedAt()), EvidenceIndex: reply.EvidenceIndex().Text(),
		RejectionReason: reply.RejectionReason().Text(), RecordedBy: reply.RecordedBy().String(),
		RecordedAt: rfc3339(reply.RecordedAt()),
	}
}

// QuotationReading 是详情用的读法：状态在读的时候派生。
type QuotationReading interface {
	Get(ctx context.Context, tenant domain.TenantID, id domain.QuotationID, now time.Time) (application.QuotationReadingDetail, error)
}

var _ QuotationReading = (*application.ReadQuotationHandler)(nil)

type QuotationPublicationRequirementRecorder interface {
	Handle(context.Context, domain.TenantID, bool) (application.QuotationPublicationRequirementOutcome, error)
}

func NewQuotationPublicationRequirementEndpoint(intake *OperatorRegistryIntake, recorder QuotationPublicationRequirementRecorder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		operator, err := intake.authenticate(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		var document struct {
			Requires *bool `json:"requires"`
		}
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&document); err != nil || decoder.More() || document.Requires == nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		result, err := recorder.Handle(request.Context(), operator.Tenant, *document.Requires)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		if result == application.QuotationPublicationRequirementRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, struct {
			Outcome string `json:"outcome"`
		}{Outcome: result.String()})
	})
}
