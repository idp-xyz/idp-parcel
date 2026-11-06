package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// BusinessPartyProfiles 是参与方资料登记册（ADR-0369，迁移 0046）：写入面与修订历史读面在同一只适配器上，读写同一张表。
type BusinessPartyProfiles struct {
	db *bentopg.DB
}

func NewBusinessPartyProfiles(db *bentopg.DB) (*BusinessPartyProfiles, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &BusinessPartyProfiles{db: db}, nil
}

var (
	_ ports.BusinessPartyProfileRegistry    = (*BusinessPartyProfiles)(nil)
	_ ports.BusinessPartyProfileHistoryRead = (*BusinessPartyProfiles)(nil)
)

// businessPartyProfileDocument 是一笔修订的快照。子格的形状借法人资料快照的那几种（地址、监管登记号、联系人）：同一种资料
// 在两本册上写成同一个样子。各项都可缺，缺席的格不落字节。
type businessPartyProfileDocument struct {
	Tenant                string                           `json:"tenant"`
	Party                 string                           `json:"party"`
	Revision              int                              `json:"revision"`
	Basis                 string                           `json:"basis"`
	EffectiveFrom         time.Time                        `json:"effectiveFrom"`
	Names                 []domain.LocalizedPartyName      `json:"names,omitempty"`
	RegisteredAddress     *legalEntityProfileAddressDoc    `json:"registeredAddress,omitempty"`
	OfficeAddress         *legalEntityProfileAddressDoc    `json:"officeAddress,omitempty"`
	CorrespondenceAddress *legalEntityProfileAddressDoc    `json:"correspondenceAddress,omitempty"`
	TaxNumbers            []legalEntityProfileTaxNumberDoc `json:"taxRegistrationNumbers,omitempty"`
	InvoiceTitle          *string                          `json:"invoiceTitle,omitempty"`
	InvoiceTitleLanguage  string                           `json:"invoiceTitleLanguage,omitempty"`
	InvoiceTitles         []domain.LocalizedInvoiceTitle   `json:"invoiceTitles,omitempty"`
	Contacts              []legalEntityProfileContactDoc   `json:"contacts,omitempty"`
	BankAccounts          []domain.LegalEntityBankAccount  `json:"bankAccounts,omitempty"`
}

func addressDocOf(address *domain.RegisteredAddress) *legalEntityProfileAddressDoc {
	if address == nil {
		return nil
	}
	return &legalEntityProfileAddressDoc{Country: address.Country().String(), Lines: address.Lines()}
}

func addressOfDoc(document *legalEntityProfileAddressDoc) (*domain.RegisteredAddress, error) {
	if document == nil {
		return nil, nil
	}
	country, err := domain.NewRegistrationCountryCode(document.Country)
	if err != nil {
		return nil, err
	}
	address, err := domain.NewRegisteredAddress(country, document.Lines)
	if err != nil {
		return nil, err
	}
	return &address, nil
}

func documentOfBusinessPartyProfile(revision domain.BusinessPartyProfileRevision) businessPartyProfileDocument {
	parts := revision.Content().Parts()
	document := businessPartyProfileDocument{
		Tenant:                revision.Tenant().String(),
		Party:                 revision.Party().String(),
		Revision:              revision.Revision(),
		Basis:                 revision.Basis().String(),
		EffectiveFrom:         revision.EffectiveFrom().UTC(),
		Names:                 parts.Names,
		RegisteredAddress:     addressDocOf(parts.RegisteredAddress),
		OfficeAddress:         addressDocOf(parts.OfficeAddress),
		CorrespondenceAddress: addressDocOf(parts.CorrespondenceAddress),
		BankAccounts:          parts.BankAccounts,
	}
	for _, number := range parts.TaxNumbers {
		document.TaxNumbers = append(document.TaxNumbers, legalEntityProfileTaxNumberDoc{
			TypeCode: number.TypeCode().String(), Number: number.Number().String(),
		})
	}
	if details := parts.Invoicing; details != nil {
		title := details.Title().String()
		document.InvoiceTitle = &title
		document.InvoiceTitleLanguage = details.TitleLanguage()
		document.InvoiceTitles = details.LocalizedTitles()
	}
	for _, contact := range parts.Contacts {
		details := contact.Details()
		document.Contacts = append(document.Contacts, legalEntityProfileContactDoc{
			Name: contact.Name(), Email: contact.Email(), Phone: contact.Phone(),
			Department: details.Department, Position: details.Position, Purposes: details.Purposes,
			PrimaryFor: details.PrimaryFor, SocialAccounts: details.SocialAccounts,
		})
	}
	return document
}

func businessPartyProfileFromSnapshot(raw []byte) (domain.BusinessPartyProfileRevision, error) {
	none := domain.BusinessPartyProfileRevision{}
	var document businessPartyProfileDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return none, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return none, err
	}
	party, err := domain.NewPartyID(document.Party)
	if err != nil {
		return none, err
	}
	basis, err := domain.NewBusinessPartyProfileBasisReference(document.Basis)
	if err != nil {
		return none, err
	}
	parts := domain.BusinessPartyProfileParts{Names: document.Names, BankAccounts: document.BankAccounts}
	if parts.RegisteredAddress, err = addressOfDoc(document.RegisteredAddress); err != nil {
		return none, err
	}
	if parts.OfficeAddress, err = addressOfDoc(document.OfficeAddress); err != nil {
		return none, err
	}
	if parts.CorrespondenceAddress, err = addressOfDoc(document.CorrespondenceAddress); err != nil {
		return none, err
	}
	for _, raw := range document.TaxNumbers {
		typeCode, err := domain.NewRegistrationNumberTypeCode(raw.TypeCode)
		if err != nil {
			return none, err
		}
		number, err := domain.NewRegistrationNumber(raw.Number)
		if err != nil {
			return none, err
		}
		tax, err := domain.NewTaxRegistrationNumber(typeCode, number)
		if err != nil {
			return none, err
		}
		parts.TaxNumbers = append(parts.TaxNumbers, tax)
	}
	if document.InvoiceTitle != nil {
		title, err := domain.NewInvoiceTitle(*document.InvoiceTitle)
		if err != nil {
			return none, err
		}
		details, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, document.InvoiceTitleLanguage, document.InvoiceTitles)
		if err != nil {
			return none, err
		}
		parts.Invoicing = &details
	}
	for _, raw := range document.Contacts {
		contact, err := domain.NewLegalEntityContactWithDetails(raw.Name, raw.Email, raw.Phone, domain.LegalEntityContactDetails{
			Department: raw.Department, Position: raw.Position, Purposes: raw.Purposes, PrimaryFor: raw.PrimaryFor, SocialAccounts: raw.SocialAccounts,
		})
		if err != nil {
			return none, err
		}
		parts.Contacts = append(parts.Contacts, contact)
	}
	content, err := domain.NewBusinessPartyProfileContent(parts)
	if err != nil {
		return none, err
	}
	return domain.NewBusinessPartyProfileRevision(tenant, party, document.Revision, basis, document.EffectiveFrom, content)
}

func (repository *BusinessPartyProfiles) SaveBusinessPartyProfile(
	ctx context.Context,
	revision domain.BusinessPartyProfileRevision,
) (ports.BusinessPartyProfileSaveOutcome, error) {
	const operation = "save business party profile"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.BusinessPartyProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	document := documentOfBusinessPartyProfile(revision)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.BusinessPartyProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])
	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.business_party_profile_revision
			(tenant_id, party_id, revision, basis_ref, effective_from, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.Party, document.Revision, document.Basis, document.EffectiveFrom, contentDigest, raw,
	)
	if err != nil {
		return ports.BusinessPartyProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.BusinessPartyProfileSaved, nil
	}
	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest FROM party_commercial.business_party_profile_revision
		  WHERE tenant_id = $1 AND party_id = $2 AND revision = $3`,
		document.Tenant, document.Party, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.BusinessPartyProfileSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.BusinessPartyProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.BusinessPartyProfileAlreadyRegistered, nil
	}
	return ports.BusinessPartyProfileContentConflict, nil
}

func (repository *BusinessPartyProfiles) LoadLatestBusinessPartyProfile(
	ctx context.Context,
	tenant domain.TenantID,
	party domain.PartyID,
) (domain.BusinessPartyProfileRevision, bool, error) {
	const operation = "load latest business party profile"
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.BusinessPartyProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot FROM party_commercial.business_party_profile_revision
		  WHERE tenant_id = $1 AND party_id = $2 ORDER BY revision DESC LIMIT 1`,
		tenant.String(), party.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BusinessPartyProfileRevision{}, false, nil
	}
	if err != nil {
		return domain.BusinessPartyProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := businessPartyProfileFromSnapshot(raw)
	if err != nil {
		return domain.BusinessPartyProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

// ListBusinessPartyProfileRevisions 交回一家往来单位的全部修订，按修订号升序，不带 LIMIT（理由在端口注释）。不在册也交回
// 空切片而不是 nil：传输层据此编成 []。
func (repository *BusinessPartyProfiles) ListBusinessPartyProfileRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	party domain.PartyID,
) ([]ports.RecordedBusinessPartyProfileRevision, error) {
	const operation = "list business party profile revisions"
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT snapshot, recorded_at FROM party_commercial.business_party_profile_revision
		  WHERE tenant_id = $1 AND party_id = $2 ORDER BY revision ASC`,
		tenant.String(), party.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	revisions := make([]ports.RecordedBusinessPartyProfileRevision, 0)
	for rows.Next() {
		var raw []byte
		var recordedAt time.Time
		if err := rows.Scan(&raw, &recordedAt); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		revision, err := businessPartyProfileFromSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		revisions = append(revisions, ports.RecordedBusinessPartyProfileRevision{Revision: revision, RecordedAt: recordedAt})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return revisions, nil
}
