package commercialhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 报价发出的在线口。写面与参与方登记写面同一张面。载荷不收租户、发出人、时刻或报价号：
// 租户与发出人取认证信封，时刻取服务端时钟，报价号由服务端签发。
// 读面只报发出事实，并写明送达不在这里记。再下载是读，不新增记录。

type QuotationSendIntake interface {
	IntakeQuotationSend(ctx context.Context, request *http.Request, now time.Time) (application.SendQuotationCommand, error)
}

var _ QuotationSendIntake = (*OperatorRegistryIntake)(nil)

type quotationSendDocument struct {
	TenantID            json.RawMessage `json:"tenantId"`
	Sender              json.RawMessage `json:"sender"`
	SentAt              json.RawMessage `json:"sentAt"`
	QuotationID         json.RawMessage `json:"quotationId"`
	Lane                string          `json:"lane"`
	PlanID              string          `json:"planId"`
	PlanVersion         string          `json:"planVersion"`
	ContentDigest       string          `json:"contentDigest"`
	CounterpartyPartyID string          `json:"counterpartyPartyId"`
	LegalEntityID       string          `json:"legalEntityId"`
	ValidFrom           *string         `json:"validFrom"`
	ValidUntil          *string         `json:"validUntil"`
	Terms               string          `json:"terms"`
}

func (intake *OperatorRegistryIntake) IntakeQuotationSend(ctx context.Context, request *http.Request, now time.Time) (application.SendQuotationCommand, error) {
	var none application.SendQuotationCommand
	operator, err := intake.authenticate(ctx, request)
	if err != nil {
		return none, err
	}
	document, err := decodeClosedDocument[quotationSendDocument](request)
	if err != nil {
		return none, err
	}
	if err := refuseSelfReportedTenant(document.TenantID); err != nil {
		return none, err
	}
	if err := refuseSelfReportedField(document.Sender, "sender"); err != nil {
		return none, err
	}
	if err := refuseSelfReportedField(document.SentAt, "sentAt"); err != nil {
		return none, err
	}
	if err := refuseSelfReportedField(document.QuotationID, "quotationId"); err != nil {
		return none, err
	}
	lane, known := domain.QuotationPriceLaneNamed(document.Lane)
	if !known {
		return none, fmt.Errorf("%w: lane must be APPROVED_DRAFT or REGISTERED_PRICE_CARD", ErrMalformedRequest)
	}
	counterparty, err := domain.NewPartyID(document.CounterpartyPartyID)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	legalEntity, err := domain.NewLegalEntityReference(document.LegalEntityID)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	startsAt, err := optionalClock(document.ValidFrom, "validFrom")
	if err != nil {
		return none, err
	}
	endsAt, err := optionalClock(document.ValidUntil, "validUntil")
	if err != nil {
		return none, err
	}
	if document.PlanID == "" || document.PlanVersion == "" || document.ContentDigest == "" {
		return none, fmt.Errorf("%w: planId, planVersion and contentDigest are required", ErrMalformedRequest)
	}
	if now.IsZero() {
		return none, fmt.Errorf("party commercial http: quotation send clock is unset")
	}
	return application.SendQuotationCommand{
		Tenant: operator.Tenant, Sender: operator.Operator.Reference(), SentAt: now.UTC(),
		Lane: lane, PlanID: document.PlanID, PlanVersion: document.PlanVersion, ContentDigest: document.ContentDigest,
		Counterparty: counterparty, LegalEntity: legalEntity,
		StartsAt: startsAt, EndsAt: endsAt, Terms: document.Terms,
	}, nil
}

func refuseSelfReportedField(raw json.RawMessage, name string) error {
	if len(raw) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s must not be carried in the payload", ErrMalformedRequest, name)
}

func optionalClock(raw *string, name string) (time.Time, error) {
	if raw == nil || *raw == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s must be RFC3339", ErrMalformedRequest, name)
	}
	return at.UTC(), nil
}

type QuotationSender interface {
	Handle(context.Context, application.SendQuotationCommand) (application.SendQuotationResult, error)
}

var _ QuotationSender = (*application.SendQuotationHandler)(nil)

func NewSendQuotationEndpoint(intake QuotationSendIntake, sender QuotationSender, now func() time.Time) http.Handler {
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
		command, err := intake.IntakeQuotationSend(request.Context(), request, now())
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := sender.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeQuotationSendAnswer(response, result)
	})
}

func writeQuotationSendAnswer(response http.ResponseWriter, result application.SendQuotationResult) {
	if result.Outcome.String() == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome == application.QuotationSent {
		status = http.StatusCreated
	}
	body := quotationSendAnswer{
		Outcome: result.Outcome.String(), Conclusion: result.Conclusion, DeliveryRecorded: false,
	}
	if result.Send.ID().String() != "" {
		body.QuotationID = result.Send.ID().String()
		body.SentAt = rfc3339(result.Send.SentAt())
		body.Lane = result.Send.Lane().String()
		body.PlanID = result.Send.PlanID()
		body.PlanVersion = result.Send.PlanVersion()
		body.ContentDigest = result.Send.ContentDigest()
		body.Canonicalization = result.Send.Canonicalization()
	}
	writeJSON(response, status, body)
}

type QuotationSendReader interface {
	List(ctx context.Context, tenant domain.TenantID, limit int, cursor ports.QuotationSendCursor) (application.QuotationSendList, error)
	Get(ctx context.Context, tenant domain.TenantID, id domain.QuotationID) (application.QuotationSendDetail, error)
}

var _ QuotationSendReader = (*application.QueryQuotationSendsHandler)(nil)

func NewQueryQuotationSendsEndpoint(intake CommercialCatalogueIntake, reader QuotationSendReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		limit, cursor, err := decodeQuotationListQuery(request)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		list, err := reader.List(request.Context(), query.Scope.Tenant(), limit, cursor)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]quotationSendBody, 0, len(list.Sends))
		for _, send := range list.Sends {
			items = append(items, quotationSendBodyOf(send))
		}
		answer := quotationSendListAnswer{Outcome: "QUOTATION_SENDS", DeliveryNote: list.DeliveryNote, Items: items}
		if list.HasNext {
			answer.Next = &quotationSendCursorBody{SentAt: rfc3339(list.Next.SentAt), QuotationID: list.Next.ID.String()}
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

func NewQueryQuotationSendEndpoint(intake CommercialCatalogueIntake, reader QuotationSendReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		id, err := domain.NewQuotationID(request.PathValue("quotationId"))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		detail, err := reader.Get(request.Context(), query.Scope.Tenant(), id)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !detail.Found {
			writeJSON(response, http.StatusOK, quotationSendDetailAnswer{
				Outcome: "NOT_FOUND", DeliveryNote: detail.DeliveryNote, DeliveryRecorded: false,
			})
			return
		}
		body := quotationSendBodyOf(detail.Send)
		writeJSON(response, http.StatusOK, quotationSendDetailAnswer{
			Outcome: "QUOTATION_SEND", DeliveryNote: detail.DeliveryNote, DeliveryRecorded: false, Send: &body,
		})
	})
}

type QuotationExporter interface {
	Handle(ctx context.Context, tenant domain.TenantID, id domain.QuotationID) (application.ExportQuotationResult, error)
}

var _ QuotationExporter = (*application.ExportQuotationHandler)(nil)

func NewExportQuotationEndpoint(intake CommercialCatalogueIntake, exporter QuotationExporter) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		id, err := domain.NewQuotationID(request.PathValue("quotationId"))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		result, err := exporter.Handle(request.Context(), query.Scope.Tenant(), id)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if result.Outcome == application.QuotationExportReady {
			if len(result.Body) == 0 || result.FileName == "" {
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			response.Header().Set("Content-Type", result.MediaType)
			response.Header().Set("Content-Disposition", `attachment; filename="`+result.FileName+`"`)
			response.WriteHeader(http.StatusOK)
			_, _ = response.Write(result.Body)
			return
		}
		if result.Outcome.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		writeJSON(response, http.StatusOK, struct {
			Outcome    string `json:"outcome"`
			Conclusion string `json:"conclusion"`
		}{Outcome: result.Outcome.String(), Conclusion: result.Conclusion})
	})
}

func decodeQuotationListQuery(request *http.Request) (int, ports.QuotationSendCursor, error) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return 0, ports.QuotationSendCursor{}, err
	}
	for key := range values {
		switch key {
		case "limit", "sentAt", "quotationId":
		default:
			return 0, ports.QuotationSendCursor{}, fmt.Errorf("unknown query parameter %s", key)
		}
	}
	limit := 50
	if raw, present := values["limit"]; present {
		if len(raw) != 1 {
			return 0, ports.QuotationSendCursor{}, fmt.Errorf("limit")
		}
		parsed, convErr := strconv.Atoi(raw[0])
		if convErr != nil || parsed < 1 || parsed > 100 {
			return 0, ports.QuotationSendCursor{}, fmt.Errorf("limit")
		}
		limit = parsed
	}
	_, hasSent := values["sentAt"]
	_, hasID := values["quotationId"]
	if hasSent != hasID {
		return 0, ports.QuotationSendCursor{}, fmt.Errorf("cursor")
	}
	if !hasSent {
		return limit, ports.QuotationSendCursor{}, nil
	}
	if len(values["sentAt"]) != 1 || len(values["quotationId"]) != 1 {
		return 0, ports.QuotationSendCursor{}, fmt.Errorf("cursor")
	}
	sentAt, err := time.Parse(time.RFC3339, values["sentAt"][0])
	if err != nil {
		return 0, ports.QuotationSendCursor{}, err
	}
	id, err := domain.NewQuotationID(values["quotationId"][0])
	if err != nil {
		return 0, ports.QuotationSendCursor{}, err
	}
	return limit, ports.QuotationSendCursor{SentAt: sentAt.UTC(), ID: id}, nil
}

type quotationSendAnswer struct {
	Outcome          string `json:"outcome"`
	Conclusion       string `json:"conclusion,omitempty"`
	QuotationID      string `json:"quotationId,omitempty"`
	SentAt           string `json:"sentAt,omitempty"`
	Lane             string `json:"lane,omitempty"`
	PlanID           string `json:"planId,omitempty"`
	PlanVersion      string `json:"planVersion,omitempty"`
	ContentDigest    string `json:"contentDigest,omitempty"`
	Canonicalization string `json:"canonicalization,omitempty"`
	DeliveryRecorded bool   `json:"deliveryRecorded"`
}

type quotationSendListAnswer struct {
	Outcome      string                   `json:"outcome"`
	DeliveryNote string                   `json:"deliveryNote"`
	Items        []quotationSendBody      `json:"items"`
	Next         *quotationSendCursorBody `json:"next,omitempty"`
}

type quotationSendDetailAnswer struct {
	Outcome          string             `json:"outcome"`
	DeliveryNote     string             `json:"deliveryNote"`
	DeliveryRecorded bool               `json:"deliveryRecorded"`
	Send             *quotationSendBody `json:"send,omitempty"`
}

type quotationSendCursorBody struct {
	SentAt      string `json:"sentAt"`
	QuotationID string `json:"quotationId"`
}

type quotationSendBody struct {
	QuotationID      string `json:"quotationId"`
	Lane             string `json:"lane"`
	PlanID           string `json:"planId"`
	PlanVersion      string `json:"planVersion"`
	Canonicalization string `json:"canonicalization"`
	ContentDigest    string `json:"contentDigest"`
	CounterpartyID   string `json:"counterpartyPartyId"`
	CounterpartyName string `json:"counterpartyName"`
	LegalEntityID    string `json:"legalEntityId"`
	LegalEntityName  string `json:"legalEntityName"`
	ValidFrom        string `json:"validFrom"`
	ValidUntil       string `json:"validUntil"`
	Terms            string `json:"terms"`
	Sender           string `json:"sender"`
	SentAt           string `json:"sentAt"`
	Method           string `json:"method"`
	DeliveryRecorded bool   `json:"deliveryRecorded"`
}

func quotationSendBodyOf(send domain.QuotationSend) quotationSendBody {
	return quotationSendBody{
		QuotationID: send.ID().String(), Lane: send.Lane().String(),
		PlanID: send.PlanID(), PlanVersion: send.PlanVersion(),
		Canonicalization: send.Canonicalization(), ContentDigest: send.ContentDigest(),
		CounterpartyID: send.Counterparty().String(), CounterpartyName: send.CounterpartyName().String(),
		LegalEntityID: send.LegalEntity().String(), LegalEntityName: send.LegalEntityName().String(),
		ValidFrom: rfc3339(send.Validity().StartsAt()), ValidUntil: rfc3339(send.Validity().EndsAt()),
		Terms: send.Terms().Text(), Sender: send.Sender().String(), SentAt: rfc3339(send.SentAt()),
		Method: send.Method().String(), DeliveryRecorded: false,
	}
}
