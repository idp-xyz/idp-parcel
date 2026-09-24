package commercialhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// LegalEntityProfileResolver 是法人资料按时点解析端点消费的用例口，只声明端点用到的那一个方法。
type LegalEntityProfileResolver interface {
	Resolve(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
		at time.Time,
	) (domain.LegalEntityProfileResolution, error)
}

// 编译期锁缝：本端点不新造解析语义，只消费 application 里那一个解析用例。
var _ LegalEntityProfileResolver = (*application.ResolveLegalEntityProfileHandler)(nil)

// NewQueryLegalEntityProfileResolutionEndpoint 交回法人资料按时点解析的 HTTP 入口
// （GET /commercial-group-legal-entities/{legalEntityId}/profile-resolution?at=…，票 legal-entity-profile/05）。
//
// 形同法人资料修订历史口（NewQueryLegalEntityProfileRevisionsEndpoint）：法人标识从路径取、在 Intake 之后读；读失败答没形成
// 答案；Intake 与其余目录查阅口同一个。`at` 由调用方给（RFC 3339），缺或形状不对答 400，不拿服务端时钟代填——同一问两次要
// 得同一答。答复把解析用例的各格原样交出，不在这里另算：只有 `RESOLVED` 是成功，其余各格都是明确非成功（ADR-0145 决定五、六）。
func NewQueryLegalEntityProfileResolutionEndpoint(
	intake CommercialCatalogueIntake,
	resolver LegalEntityProfileResolver,
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
		at, err := time.Parse(time.RFC3339, request.URL.Query().Get("at"))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		resolution, err := resolver.Resolve(request.Context(), query.Scope.Tenant(), entity, at)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body, named := legalEntityProfileResolutionBodyOf(entity, at, resolution)
		if !named {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		writeJSON(response, http.StatusOK, body)
	})
}

// legalEntityProfileResolutionResponse 是一次解析的答复。effectiveRevision 是解析时点有效的那一笔：`RESOLVED` 时就是可以固定的
// 那一笔；`资料不全`因缺开票资料时也带上，读的人要知道该给哪一笔之后补登——能不能开立只看 outcome，不看它在不在。
type legalEntityProfileResolutionResponse struct {
	Outcome           string                                   `json:"outcome"`
	LegalEntityID     string                                   `json:"legalEntityId"`
	At                string                                   `json:"at"`
	IncompleteCause   string                                   `json:"incompleteCause,omitempty"`
	EffectiveRevision *legalEntityProfileEffectiveRevisionBody `json:"effectiveRevision,omitempty"`
}

// legalEntityProfileEffectiveRevisionBody 逐字段透出那一笔资料修订，子格与修订历史口的行体同形（集合格恒为数组、开票资料
// 在不在用显式布尔说）。修订引用即租户 + 法人 + 修订号：租户是作用域，法人在顶层，修订号在这里。
type legalEntityProfileEffectiveRevisionBody struct {
	Revision               int                         `json:"revision"`
	Basis                  string                      `json:"basis"`
	EffectiveFrom          string                      `json:"effectiveFrom"`
	RegisteredAddress      registeredAddressBody       `json:"registeredAddress"`
	TaxRegistrationNumbers []taxRegistrationNumberBody `json:"taxRegistrationNumbers"`
	InvoicingRegistered    bool                        `json:"invoicingRegistered"`
	InvoiceTitle           string                      `json:"invoiceTitle,omitempty"`
	Contacts               []legalEntityContactBody    `json:"contacts"`
}

// legalEntityProfileResolutionBodyOf 把解析答案译成答复体；领域答出没有名字的格时交回 false，由端点答没命名的结果。
func legalEntityProfileResolutionBodyOf(
	entity domain.LegalEntityReference,
	at time.Time,
	resolution domain.LegalEntityProfileResolution,
) (legalEntityProfileResolutionResponse, bool) {
	outcome := resolution.Outcome().String()
	if outcome == "" {
		return legalEntityProfileResolutionResponse{}, false
	}
	body := legalEntityProfileResolutionResponse{
		Outcome:       outcome,
		LegalEntityID: entity.String(),
		At:            rfc3339(at),
	}
	if resolution.Outcome() == domain.LegalEntityProfileIncomplete {
		body.IncompleteCause = resolution.IncompleteCause().String()
	}
	if revision, has := resolution.EffectiveRevision(); has {
		effective := legalEntityProfileEffectiveRevisionBodyOf(revision)
		body.EffectiveRevision = &effective
	}
	return body, true
}

func legalEntityProfileEffectiveRevisionBodyOf(revision domain.LegalEntityProfileRevision) legalEntityProfileEffectiveRevisionBody {
	content := revision.Content()
	address := content.Address()
	body := legalEntityProfileEffectiveRevisionBody{
		Revision:               revision.Revision(),
		Basis:                  revision.Basis().String(),
		EffectiveFrom:          rfc3339(revision.EffectiveFrom()),
		RegisteredAddress:      registeredAddressBody{Country: address.Country().String(), Lines: address.Lines()},
		TaxRegistrationNumbers: make([]taxRegistrationNumberBody, 0, len(content.TaxNumbers())),
		Contacts:               make([]legalEntityContactBody, 0, len(content.Contacts())),
	}
	for _, number := range content.TaxNumbers() {
		body.TaxRegistrationNumbers = append(body.TaxRegistrationNumbers,
			taxRegistrationNumberBody{TypeCode: number.TypeCode().String(), Number: number.Number().String()})
	}
	if details, has := content.Invoicing(); has {
		body.InvoicingRegistered = true
		body.InvoiceTitle = details.Title().String()
	}
	for _, contact := range content.Contacts() {
		body.Contacts = append(body.Contacts,
			legalEntityContactBody{Name: contact.Name(), Email: contact.Email(), Phone: contact.Phone()})
	}
	return body
}
