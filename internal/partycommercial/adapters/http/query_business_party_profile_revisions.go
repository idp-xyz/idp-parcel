package commercialhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// BusinessPartyProfileHistoryReader 是参与方资料修订历史端点消费的读口。
type BusinessPartyProfileHistoryReader interface {
	ListBusinessPartyProfileRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) ([]ports.RecordedBusinessPartyProfileRevision, error)
}

var _ BusinessPartyProfileHistoryReader = ports.BusinessPartyProfileHistoryRead(nil)

const outcomeBusinessPartyProfileRevisionsListed = "BUSINESS_PARTY_PROFILE_REVISIONS_LISTED"

// NewQueryBusinessPartyProfileRevisionsEndpoint 交回参与方资料修订历史查阅的 HTTP 入口
// （GET /commercial-business-parties/{partyId}/profile-revisions，ADR-0369）。
//
// 判据照法人资料修订历史口：往来单位标识从路径取、在 Intake 之后读；不在册与跨租户都答 200 + 空数组；读失败答没形成答案。多交
// 一格 effectiveRevision：此刻有效的是哪一笔，由 domain.EffectiveBusinessPartyProfileRevision 判——页面不另抄一份判法。
// 没有一笔此刻有效（都未到生效时点或从未登记）时这一格缺席。
func NewQueryBusinessPartyProfileRevisionsEndpoint(
	intake CommercialCatalogueIntake,
	reader BusinessPartyProfileHistoryReader,
	now func() time.Time,
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
		party, err := domain.NewPartyID(request.PathValue(partyIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		recorded, err := reader.ListBusinessPartyProfileRevisions(request.Context(), query.Scope.Tenant(), party)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		chain := make([]domain.BusinessPartyProfileRevision, 0, len(recorded))
		bodies := make([]businessPartyProfileRevisionBody, 0, len(recorded))
		for _, entry := range recorded {
			chain = append(chain, entry.Revision)
			bodies = append(bodies, businessPartyProfileRevisionBodyOf(entry))
		}
		answer := businessPartyProfileRevisionListResponse{
			Outcome:   outcomeBusinessPartyProfileRevisionsListed,
			PartyID:   party.String(),
			Revisions: bodies,
		}
		effective, found, err := domain.EffectiveBusinessPartyProfileRevision(chain, now())
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if found {
			answer.EffectiveRevision = &effective
		}
		writeJSON(response, http.StatusOK, answer)
	})
}

type businessPartyProfileRevisionListResponse struct {
	Outcome           string                             `json:"outcome"`
	PartyID           string                             `json:"partyId"`
	EffectiveRevision *int                               `json:"effectiveRevision,omitempty"`
	Revisions         []businessPartyProfileRevisionBody `json:"revisions"`
}

// businessPartyProfileRevisionBody 逐字段透出一笔参与方资料修订。子格与法人资料修订的答复同形；各项都可缺，缺即键缺席，
// 开票资料在不在用显式布尔 invoicingRegistered 说，判据同法人资料。
type businessPartyProfileRevisionBody struct {
	TenantID               string                          `json:"tenantId"`
	PartyID                string                          `json:"partyId"`
	Revision               int                             `json:"revision"`
	Basis                  string                          `json:"basis"`
	EffectiveFrom          string                          `json:"effectiveFrom"`
	RegisteredAt           string                          `json:"registeredAt"`
	Names                  []domain.LocalizedPartyName     `json:"names,omitempty"`
	RegisteredAddress      *registeredAddressBody          `json:"registeredAddress,omitempty"`
	OfficeAddress          *registeredAddressBody          `json:"officeAddress,omitempty"`
	CorrespondenceAddress  *registeredAddressBody          `json:"correspondenceAddress,omitempty"`
	TaxRegistrationNumbers []taxRegistrationNumberBody     `json:"taxRegistrationNumbers"`
	InvoicingRegistered    bool                            `json:"invoicingRegistered"`
	InvoiceTitle           string                          `json:"invoiceTitle,omitempty"`
	InvoiceTitleLanguage   string                          `json:"invoiceTitleLanguage,omitempty"`
	InvoiceTitles          []domain.LocalizedInvoiceTitle  `json:"invoiceTitles,omitempty"`
	Contacts               []legalEntityContactBody        `json:"contacts"`
	BankAccounts           []domain.LegalEntityBankAccount `json:"bankAccounts,omitempty"`
}

func addressBodyOf(address *domain.RegisteredAddress) *registeredAddressBody {
	if address == nil {
		return nil
	}
	return &registeredAddressBody{Country: address.Country().String(), Lines: address.Lines()}
}

func businessPartyProfileRevisionBodyOf(entry ports.RecordedBusinessPartyProfileRevision) businessPartyProfileRevisionBody {
	revision := entry.Revision
	parts := revision.Content().Parts()
	body := businessPartyProfileRevisionBody{
		TenantID:               revision.Tenant().String(),
		PartyID:                revision.Party().String(),
		Revision:               revision.Revision(),
		Basis:                  revision.Basis().String(),
		EffectiveFrom:          rfc3339(revision.EffectiveFrom()),
		RegisteredAt:           rfc3339(entry.RecordedAt),
		Names:                  parts.Names,
		RegisteredAddress:      addressBodyOf(parts.RegisteredAddress),
		OfficeAddress:          addressBodyOf(parts.OfficeAddress),
		CorrespondenceAddress:  addressBodyOf(parts.CorrespondenceAddress),
		TaxRegistrationNumbers: make([]taxRegistrationNumberBody, 0, len(parts.TaxNumbers)),
		Contacts:               make([]legalEntityContactBody, 0, len(parts.Contacts)),
		BankAccounts:           parts.BankAccounts,
	}
	for _, number := range parts.TaxNumbers {
		body.TaxRegistrationNumbers = append(body.TaxRegistrationNumbers, taxRegistrationNumberBody{
			TypeCode: number.TypeCode().String(), Number: number.Number().String(),
		})
	}
	if details := parts.Invoicing; details != nil {
		body.InvoicingRegistered = true
		body.InvoiceTitle = details.Title().String()
		body.InvoiceTitleLanguage = details.TitleLanguage()
		body.InvoiceTitles = details.LocalizedTitles()
	}
	for _, contact := range parts.Contacts {
		details := contact.Details()
		body.Contacts = append(body.Contacts, legalEntityContactBody{
			Name: contact.Name(), Email: contact.Email(), Phone: contact.Phone(), Department: details.Department,
			Position: details.Position, Purposes: details.Purposes, PrimaryFor: details.PrimaryFor, SocialAccounts: details.SocialAccounts,
		})
	}
	return body
}
