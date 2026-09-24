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

// LegalEntityProfiles 实现法人资料登记册的写口、修订链读口与修订历史读口（0036 迁移）。撞键不覆盖：同内容
// 重放，异内容冲突（ADR-0031，判据与 RegistrationNumberTypes 同款）。
type LegalEntityProfiles struct {
	db *bentopg.DB
}

func NewLegalEntityProfiles(db *bentopg.DB) (*LegalEntityProfiles, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &LegalEntityProfiles{db: db}, nil
}

var (
	_ ports.LegalEntityProfileRegistry            = (*LegalEntityProfiles)(nil)
	_ ports.LegalEntityProfileChainRead           = (*LegalEntityProfiles)(nil)
	_ ports.LegalEntityProfileRevisionHistoryRead = (*LegalEntityProfiles)(nil)
)

// legalEntityProfileDocument 是登记快照的形状。集合格一律写成数组（空即 []），不写 null：同一份内容只有一种
// 字节，摘要才判得出重放。
type legalEntityProfileDocument struct {
	Tenant        string                           `json:"tenant"`
	LegalEntity   string                           `json:"legalEntity"`
	Revision      int                              `json:"revision"`
	Basis         string                           `json:"basis"`
	EffectiveFrom time.Time                        `json:"effectiveFrom"`
	Address       legalEntityProfileAddressDoc     `json:"address"`
	TaxNumbers    []legalEntityProfileTaxNumberDoc `json:"taxRegistrationNumbers"`
	InvoiceTitle  *string                          `json:"invoiceTitle,omitempty"`
	Contacts      []legalEntityProfileContactDoc   `json:"contacts"`
}

type legalEntityProfileAddressDoc struct {
	Country string   `json:"country"`
	Lines   []string `json:"lines"`
}

type legalEntityProfileTaxNumberDoc struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

type legalEntityProfileContactDoc struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

func documentOfLegalEntityProfile(revision domain.LegalEntityProfileRevision) legalEntityProfileDocument {
	content := revision.Content()
	address := content.Address()
	document := legalEntityProfileDocument{
		Tenant:        revision.Tenant().String(),
		LegalEntity:   revision.LegalEntity().String(),
		Revision:      revision.Revision(),
		Basis:         revision.Basis().String(),
		EffectiveFrom: revision.EffectiveFrom().UTC(),
		Address:       legalEntityProfileAddressDoc{Country: address.Country().String(), Lines: address.Lines()},
		TaxNumbers:    make([]legalEntityProfileTaxNumberDoc, 0, len(content.TaxNumbers())),
		Contacts:      make([]legalEntityProfileContactDoc, 0, len(content.Contacts())),
	}
	for _, number := range content.TaxNumbers() {
		document.TaxNumbers = append(document.TaxNumbers, legalEntityProfileTaxNumberDoc{
			TypeCode: number.TypeCode().String(),
			Number:   number.Number().String(),
		})
	}
	if details, has := content.Invoicing(); has {
		title := details.Title().String()
		document.InvoiceTitle = &title
	}
	for _, contact := range content.Contacts() {
		document.Contacts = append(document.Contacts, legalEntityProfileContactDoc{
			Name:  contact.Name(),
			Email: contact.Email(),
			Phone: contact.Phone(),
		})
	}
	return document
}

func (repository *LegalEntityProfiles) SaveLegalEntityProfile(
	ctx context.Context,
	revision domain.LegalEntityProfileRevision,
) (ports.LegalEntityProfileSaveOutcome, error) {
	const operation = "save legal entity profile"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	document := documentOfLegalEntityProfile(revision)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])
	lines, err := json.Marshal(document.Address.Lines)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	taxNumbers, err := json.Marshal(document.TaxNumbers)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	contacts, err := json.Marshal(document.Contacts)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.legal_entity_profile_revision
			(tenant_id, legal_entity_id, revision, basis_ref, effective_from,
			 address_country, address_lines, tax_registration_numbers, invoice_title, contacts,
			 content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.LegalEntity, document.Revision, document.Basis, document.EffectiveFrom,
		document.Address.Country, string(lines), string(taxNumbers), document.InvoiceTitle, string(contacts),
		contentDigest, raw,
	)
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.LegalEntityProfileRegistrySaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = $1 AND legal_entity_id = $2 AND revision = $3`,
		document.Tenant, document.LegalEntity, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.LegalEntityProfileSaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.LegalEntityProfileRegistryAlreadyRegistered, nil
	}
	return ports.LegalEntityProfileRegistryContentConflict, nil
}

func (repository *LegalEntityProfiles) LoadLatestLegalEntityProfile(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) (domain.LegalEntityProfileRevision, bool, error) {
	const operation = "load latest legal entity profile"
	if tenant.String() == "" || entity.String() == "" {
		return domain.LegalEntityProfileRevision{}, false, fmt.Errorf("%s: tenant and legal entity are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		`SELECT snapshot
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = $1 AND legal_entity_id = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), entity.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LegalEntityProfileRevision{}, false, nil
	}
	if err != nil {
		return domain.LegalEntityProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	revision, err := legalEntityProfileFromSnapshot(raw)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return revision, true, nil
}

func (repository *LegalEntityProfiles) LoadLegalEntityProfileChain(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) ([]domain.LegalEntityProfileRevision, error) {
	const operation = "load legal entity profile chain"
	documents, _, err := repository.listDocuments(ctx, operation, tenant, entity)
	if err != nil {
		return nil, err
	}
	chain := make([]domain.LegalEntityProfileRevision, 0, len(documents))
	for _, raw := range documents {
		revision, err := legalEntityProfileFromSnapshot(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		chain = append(chain, revision)
	}
	return chain, nil
}

// ListLegalEntityProfileRevisions 按修订号升序展开一个法人资料的全部修订。行从快照转写，与重建领域对象读同一份
// 字节；不在册与跨租户都是零行、无错。
func (repository *LegalEntityProfiles) ListLegalEntityProfileRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) ([]ports.LegalEntityProfileRevisionRow, error) {
	const operation = "list legal entity profile revisions"
	documents, recordedAt, err := repository.listDocuments(ctx, operation, tenant, entity)
	if err != nil {
		return nil, err
	}
	rows := make([]ports.LegalEntityProfileRevisionRow, 0, len(documents))
	for index, raw := range documents {
		var document legalEntityProfileDocument
		if err := json.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("%s: 快照不是本适配器写下的形状：%w", operation, err)
		}
		row := ports.LegalEntityProfileRevisionRow{
			TenantID:       document.Tenant,
			LegalEntityID:  document.LegalEntity,
			Revision:       document.Revision,
			Basis:          document.Basis,
			EffectiveFrom:  document.EffectiveFrom,
			AddressCountry: document.Address.Country,
			AddressLines:   document.Address.Lines,
			TaxNumbers:     make([]ports.TaxRegistrationNumberRow, 0, len(document.TaxNumbers)),
			Contacts:       make([]ports.LegalEntityContactRow, 0, len(document.Contacts)),
			RegisteredAt:   recordedAt[index],
		}
		for _, number := range document.TaxNumbers {
			row.TaxNumbers = append(row.TaxNumbers, ports.TaxRegistrationNumberRow{TypeCode: number.TypeCode, Number: number.Number})
		}
		if document.InvoiceTitle != nil {
			row.HasInvoicing, row.InvoiceTitle = true, *document.InvoiceTitle
		}
		for _, contact := range document.Contacts {
			row.Contacts = append(row.Contacts, ports.LegalEntityContactRow{Name: contact.Name, Email: contact.Email, Phone: contact.Phone})
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (repository *LegalEntityProfiles) listDocuments(
	ctx context.Context,
	operation string,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) ([][]byte, []time.Time, error) {
	if tenant.String() == "" || entity.String() == "" {
		return nil, nil, fmt.Errorf("%s: tenant and legal entity are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT snapshot, recorded_at
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = $1 AND legal_entity_id = $2
		  ORDER BY revision`,
		tenant.String(), entity.String(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	var documents [][]byte
	var recordedAt []time.Time
	for rows.Next() {
		var raw []byte
		var at time.Time
		if err := rows.Scan(&raw, &at); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", operation, err)
		}
		documents = append(documents, raw)
		recordedAt = append(recordedAt, at)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", operation, err)
	}
	return documents, recordedAt, nil
}

// legalEntityProfileFromSnapshot 经真构造门重建一笔修订；任何一格过不了门即坏数据，上抛不吞。
func legalEntityProfileFromSnapshot(raw []byte) (domain.LegalEntityProfileRevision, error) {
	var document legalEntityProfileDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.LegalEntityProfileRevision{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	entity, err := domain.NewLegalEntityReference(document.LegalEntity)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	basis, err := domain.NewLegalEntityProfileBasisReference(document.Basis)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	country, err := domain.NewRegistrationCountryCode(document.Address.Country)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	address, err := domain.NewRegisteredAddress(country, document.Address.Lines)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	taxNumbers := make([]domain.TaxRegistrationNumber, 0, len(document.TaxNumbers))
	for _, raw := range document.TaxNumbers {
		typeCode, err := domain.NewRegistrationNumberTypeCode(raw.TypeCode)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		number, err := domain.NewRegistrationNumber(raw.Number)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		tax, err := domain.NewTaxRegistrationNumber(typeCode, number)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		taxNumbers = append(taxNumbers, tax)
	}
	var invoicing *domain.InvoicingDetails
	if document.InvoiceTitle != nil {
		title, err := domain.NewInvoiceTitle(*document.InvoiceTitle)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		details, err := domain.NewInvoicingDetails(title)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		invoicing = &details
	}
	contacts := make([]domain.LegalEntityContact, 0, len(document.Contacts))
	for _, raw := range document.Contacts {
		contact, err := domain.NewLegalEntityContact(raw.Name, raw.Email, raw.Phone)
		if err != nil {
			return domain.LegalEntityProfileRevision{}, err
		}
		contacts = append(contacts, contact)
	}
	content, err := domain.NewLegalEntityProfileContent(address, taxNumbers, invoicing, contacts)
	if err != nil {
		return domain.LegalEntityProfileRevision{}, err
	}
	return domain.NewLegalEntityProfileRevision(tenant, entity, document.Revision, basis, document.EffectiveFrom, content)
}
