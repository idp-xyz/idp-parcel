package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// LegalEntityProfileRevisionHistoryReader 是法人资料修订历史端点消费的读口，只声明端点用到的那一个方法。
type LegalEntityProfileRevisionHistoryReader interface {
	ListLegalEntityProfileRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) ([]ports.LegalEntityProfileRevisionRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ LegalEntityProfileRevisionHistoryReader = ports.LegalEntityProfileRevisionHistoryRead(nil)

const outcomeLegalEntityProfileRevisionsListed = "LEGAL_ENTITY_PROFILE_REVISIONS_LISTED"

// NewQueryLegalEntityProfileRevisionsEndpoint 交回法人资料修订历史查阅的 HTTP 入口
// （GET /commercial-group-legal-entities/{legalEntityId}/profile-revisions）。
//
// 形同法人修订历史口（NewQueryLegalEntityRevisionsEndpoint），判据逐条照搬：法人标识从路径取、在 Intake 之后读；
// 不在册与跨租户都答 200 + 空数组；读失败答没形成答案；Intake 与其余目录查阅口同一个。这一口只交修订事实，
// 不答「此刻有效的是哪一笔」——那是按时点解析的事，追溯修订会让它随时点变。
func NewQueryLegalEntityProfileRevisionsEndpoint(
	intake CommercialCatalogueIntake,
	reader LegalEntityProfileRevisionHistoryReader,
) http.Handler {
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

		entity, err := domain.NewLegalEntityReference(request.PathValue(legalEntityIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		rows, err := reader.ListLegalEntityProfileRevisions(request.Context(), query.Scope.Tenant(), entity)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]legalEntityProfileRevisionBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, legalEntityProfileRevisionBodyOf(row))
		}
		writeJSON(response, http.StatusOK, legalEntityProfileRevisionListResponse{
			Outcome:       outcomeLegalEntityProfileRevisionsListed,
			LegalEntityID: entity.String(),
			Revisions:     bodies,
		})
	})
}

// legalEntityProfileRevisionListResponse 顶层回显法人标识，理由同 legalEntityRevisionListResponse。
type legalEntityProfileRevisionListResponse struct {
	Outcome       string                           `json:"outcome"`
	LegalEntityID string                           `json:"legalEntityId"`
	Revisions     []legalEntityProfileRevisionBody `json:"revisions"`
}

// legalEntityProfileRevisionBody 逐字段透出一笔资料修订。集合格总是数组（空即 []）；开票资料在不在用显式布尔
// invoicingRegistered 说，不拿空抬头去推——缺开票资料是开立时答`资料不全`的成因之一，页面要能如实标出来。
type legalEntityProfileRevisionBody struct {
	TenantID               string                      `json:"tenantId"`
	LegalEntityID          string                      `json:"legalEntityId"`
	Revision               int                         `json:"revision"`
	Basis                  string                      `json:"basis"`
	EffectiveFrom          string                      `json:"effectiveFrom"`
	RegisteredAddress      registeredAddressBody       `json:"registeredAddress"`
	TaxRegistrationNumbers []taxRegistrationNumberBody `json:"taxRegistrationNumbers"`
	InvoicingRegistered    bool                        `json:"invoicingRegistered"`
	InvoiceTitle           string                      `json:"invoiceTitle,omitempty"`
	Contacts               []legalEntityContactBody    `json:"contacts"`
	RegisteredAt           string                      `json:"registeredAt"`
}

type registeredAddressBody struct {
	Country string   `json:"country"`
	Lines   []string `json:"lines"`
}

type taxRegistrationNumberBody struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

type legalEntityContactBody struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

func legalEntityProfileRevisionBodyOf(row ports.LegalEntityProfileRevisionRow) legalEntityProfileRevisionBody {
	lines := row.AddressLines
	if lines == nil {
		lines = []string{}
	}
	body := legalEntityProfileRevisionBody{
		TenantID:               row.TenantID,
		LegalEntityID:          row.LegalEntityID,
		Revision:               row.Revision,
		Basis:                  row.Basis,
		EffectiveFrom:          rfc3339(row.EffectiveFrom),
		RegisteredAddress:      registeredAddressBody{Country: row.AddressCountry, Lines: lines},
		TaxRegistrationNumbers: make([]taxRegistrationNumberBody, 0, len(row.TaxNumbers)),
		InvoicingRegistered:    row.HasInvoicing,
		Contacts:               make([]legalEntityContactBody, 0, len(row.Contacts)),
		RegisteredAt:           rfc3339(row.RegisteredAt),
	}
	for _, number := range row.TaxNumbers {
		body.TaxRegistrationNumbers = append(body.TaxRegistrationNumbers, taxRegistrationNumberBody{TypeCode: number.TypeCode, Number: number.Number})
	}
	if row.HasInvoicing {
		body.InvoiceTitle = row.InvoiceTitle
	}
	for _, contact := range row.Contacts {
		body.Contacts = append(body.Contacts, legalEntityContactBody{Name: contact.Name, Email: contact.Email, Phone: contact.Phone})
	}
	return body
}
