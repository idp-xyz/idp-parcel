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

// PartyIdentityRegistrations 实现 ports.PartyIdentityRegistry：参与方身份三册与
// 关系册的只增登记与最新修订装载（0015 迁移）。撞键不覆盖：同内容重放，异内容冲突
// （ADR-0031，判据与 AuthorityGrants 同款）。
type PartyIdentityRegistrations struct {
	db *bentopg.DB
}

func NewPartyIdentityRegistrations(db *bentopg.DB) (*PartyIdentityRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &PartyIdentityRegistrations{db: db}, nil
}

var _ ports.PartyIdentityRegistry = (*PartyIdentityRegistrations)(nil)

var _ ports.LegalEntityRegistrationLookup = (*PartyIdentityRegistrations)(nil)

// identityRegistrationRow 收拢三本身份册共同的行内容：Save 的列值与快照由它折出，
// 三册不各抄一份 INSERT 骨架。
type identityRegistrationRow struct {
	table     string
	idColumn  string
	tenant    string
	id        string
	revision  int
	extra     map[string]any
	basis     string
	lifecycle domain.IdentityLifecycle
	document  any
}

func (repository *PartyIdentityRegistrations) saveIdentityRow(
	ctx context.Context,
	row identityRegistrationRow,
) (ports.PartyRegistrySaveOutcome, error) {
	operation := "save " + row.table
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	raw, err := json.Marshal(row.document)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	var deactivatedAt *time.Time
	var deactivationBasis *string
	if basis, at, has := row.lifecycle.Deactivation(); has {
		utc := at.UTC()
		deactivatedAt = &utc
		text := basis.String()
		deactivationBasis = &text
	}

	columns := []string{"tenant_id", row.idColumn, "revision"}
	values := []any{row.tenant, row.id, row.revision}
	for column, value := range row.extra {
		columns = append(columns, column)
		values = append(values, value)
	}
	columns = append(columns,
		"basis_ref", "effective_from", "deactivated_at", "deactivation_basis",
		"content_digest", "snapshot")
	values = append(values,
		row.basis, row.lifecycle.EffectiveFrom().UTC(), deactivatedAt, deactivationBasis,
		contentDigest, raw)

	statement := fmt.Sprintf(
		"INSERT INTO party_commercial.%s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		row.table, joinColumns(columns), placeholders(len(values)),
	)
	tag, err := executor.Exec(ctx, statement, values...)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PartyRegistrySaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		fmt.Sprintf(
			"SELECT content_digest FROM party_commercial.%s WHERE tenant_id = $1 AND %s = $2 AND revision = $3",
			row.table, row.idColumn,
		),
		row.tenant, row.id, row.revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.PartyRegistryAlreadyRegistered, nil
	}
	return ports.PartyRegistryContentConflict, nil
}

func joinColumns(columns []string) string {
	joined := ""
	for index, column := range columns {
		if index > 0 {
			joined += ", "
		}
		joined += column
	}
	return joined
}

func placeholders(count int) string {
	joined := ""
	for index := 1; index <= count; index++ {
		if index > 1 {
			joined += ", "
		}
		joined += fmt.Sprintf("$%d", index)
	}
	return joined
}

// identityLifecycleDocument 是三本身份册快照共用的生命周期段。
type identityLifecycleDocument struct {
	Basis             string     `json:"basis"`
	EffectiveFrom     time.Time  `json:"effectiveFrom"`
	DeactivatedAt     *time.Time `json:"deactivatedAt,omitempty"`
	DeactivationBasis string     `json:"deactivationBasis,omitempty"`
}

func lifecycleDocumentOf(basis domain.IdentityBasisReference, lifecycle domain.IdentityLifecycle) identityLifecycleDocument {
	document := identityLifecycleDocument{
		Basis:         basis.String(),
		EffectiveFrom: lifecycle.EffectiveFrom().UTC(),
	}
	if deactivationBasis, at, has := lifecycle.Deactivation(); has {
		utc := at.UTC()
		document.DeactivatedAt = &utc
		document.DeactivationBasis = deactivationBasis.String()
	}
	return document
}

func (document identityLifecycleDocument) rebuild() (domain.IdentityBasisReference, domain.IdentityLifecycle, error) {
	basis, err := domain.NewIdentityBasisReference(document.Basis)
	if err != nil {
		return domain.IdentityBasisReference{}, domain.IdentityLifecycle{}, err
	}
	lifecycle, err := domain.NewIdentityLifecycle(document.EffectiveFrom)
	if err != nil {
		return domain.IdentityBasisReference{}, domain.IdentityLifecycle{}, err
	}
	if document.DeactivatedAt != nil {
		deactivationBasis, err := domain.NewIdentityBasisReference(document.DeactivationBasis)
		if err != nil {
			return domain.IdentityBasisReference{}, domain.IdentityLifecycle{}, err
		}
		lifecycle, err = lifecycle.Deactivate(deactivationBasis, *document.DeactivatedAt)
		if err != nil {
			return domain.IdentityBasisReference{}, domain.IdentityLifecycle{}, err
		}
	}
	return basis, lifecycle, nil
}

type businessPartyDocument struct {
	Tenant    string `json:"tenant"`
	PartyID   string `json:"partyId"`
	PartyName string `json:"partyName"`
	Revision  int    `json:"revision"`
	identityLifecycleDocument
}

func (repository *PartyIdentityRegistrations) SaveBusinessParty(
	ctx context.Context,
	registration domain.BusinessPartyRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	party := registration.Party()
	document := businessPartyDocument{
		Tenant:                    party.Tenant().String(),
		PartyID:                   party.ID().String(),
		PartyName:                 party.Name().String(),
		Revision:                  registration.Revision(),
		identityLifecycleDocument: lifecycleDocumentOf(registration.Basis(), registration.Lifecycle()),
	}
	return repository.saveIdentityRow(ctx, identityRegistrationRow{
		table:     "business_party_registration",
		idColumn:  "party_id",
		tenant:    party.Tenant().String(),
		id:        party.ID().String(),
		revision:  registration.Revision(),
		extra:     map[string]any{"party_name": party.Name().String()},
		basis:     registration.Basis().String(),
		lifecycle: registration.Lifecycle(),
		document:  document,
	})
}

// legalEntityDocument 的身份层三格一律 omitempty 且排在末尾：本格落地之前写下的快照没有它们，照旧编出同一串
// 字节、同一个内容摘要——历史修订的重放判定不因加格而变成冲突。
type legalEntityDocument struct {
	Tenant        string `json:"tenant"`
	LegalEntityID string `json:"legalEntityId"`
	PartyID       string `json:"partyId"`
	Revision      int    `json:"revision"`
	identityLifecycleDocument
	RegistrationCountry     string                   `json:"registrationCountry,omitempty"`
	LifetimeNumbers         []lifetimeNumberDocument `json:"lifetimeRegistrationNumbers,omitempty"`
	IdentityCorrectionBasis string                   `json:"identityCorrectionBasis,omitempty"`
}

func (repository *PartyIdentityRegistrations) SaveLegalEntity(
	ctx context.Context,
	registration domain.LegalEntityRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	entity := registration.Entity()
	document := legalEntityDocument{
		Tenant:                    entity.Tenant().String(),
		LegalEntityID:             entity.ID().String(),
		PartyID:                   entity.Party().String(),
		Revision:                  registration.Revision(),
		identityLifecycleDocument: lifecycleDocumentOf(registration.Basis(), registration.Lifecycle()),
	}
	// 身份层三列与快照同源；历史形状的修订三列落 NULL（0034 的同空同有约束）。
	extra := map[string]any{
		"party_id":                      entity.Party().String(),
		"registration_country":          nil,
		"lifetime_registration_numbers": nil,
		"identity_correction_basis":     nil,
	}
	if layer, has := registration.IdentityLayer(); has {
		numbers := lifetimeNumberDocumentsOf(layer)
		raw, err := json.Marshal(numbers)
		if err != nil {
			return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("save legal_entity_registration: %w", err)
		}
		document.RegistrationCountry = layer.Country().String()
		document.LifetimeNumbers = numbers
		extra["registration_country"] = layer.Country().String()
		extra["lifetime_registration_numbers"] = string(raw)
	}
	if basis, has := registration.IdentityCorrectionBasis(); has {
		document.IdentityCorrectionBasis = basis.String()
		extra["identity_correction_basis"] = basis.String()
	}
	return repository.saveIdentityRow(ctx, identityRegistrationRow{
		table:     "legal_entity_registration",
		idColumn:  "legal_entity_id",
		tenant:    entity.Tenant().String(),
		id:        entity.ID().String(),
		revision:  registration.Revision(),
		extra:     extra,
		basis:     registration.Basis().String(),
		lifecycle: registration.Lifecycle(),
		document:  document,
	})
}

type customerAccountDocument struct {
	Tenant        string `json:"tenant"`
	AccountID     string `json:"accountId"`
	CustomerParty string `json:"customerParty"`
	Revision      int    `json:"revision"`
	identityLifecycleDocument
}

func (repository *PartyIdentityRegistrations) SaveCustomerAccount(
	ctx context.Context,
	registration domain.CustomerAccountRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	account := registration.Account()
	document := customerAccountDocument{
		Tenant:                    account.Tenant().String(),
		AccountID:                 account.ID().String(),
		CustomerParty:             account.CustomerParty().String(),
		Revision:                  registration.Revision(),
		identityLifecycleDocument: lifecycleDocumentOf(registration.Basis(), registration.Lifecycle()),
	}
	return repository.saveIdentityRow(ctx, identityRegistrationRow{
		table:     "customer_account_registration",
		idColumn:  "account_id",
		tenant:    account.Tenant().String(),
		id:        account.ID().String(),
		revision:  registration.Revision(),
		extra:     map[string]any{"customer_party_id": account.CustomerParty().String()},
		basis:     registration.Basis().String(),
		lifecycle: registration.Lifecycle(),
		document:  document,
	})
}

type relationshipDocument struct {
	Tenant         string     `json:"tenant"`
	RelationshipID string     `json:"relationshipId"`
	Revision       int        `json:"revision"`
	Holder         string     `json:"holder"`
	Counterparty   string     `json:"counterparty"`
	Role           string     `json:"role"`
	Scope          string     `json:"scope"`
	Basis          string     `json:"basis"`
	Status         string     `json:"status"`
	StartsAt       time.Time  `json:"startsAt"`
	EndsAt         *time.Time `json:"endsAt,omitempty"`
	ApprovalRef    string     `json:"approvalRef,omitempty"`
	ApprovedAt     *time.Time `json:"approvedAt,omitempty"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
	EndBasis       string     `json:"endBasis,omitempty"`
	Successor      string     `json:"successor,omitempty"`
}

func documentOfRelationship(registration domain.PartyRelationshipRegistration) relationshipDocument {
	relationship := registration.Relationship()
	document := relationshipDocument{
		Tenant:         registration.Tenant().String(),
		RelationshipID: registration.ID().String(),
		Revision:       registration.Revision(),
		Holder:         relationship.Holder().String(),
		Counterparty:   relationship.Counterparty().String(),
		Role:           relationship.Role().String(),
		Scope:          relationship.Scope().String(),
		Basis:          relationship.Basis().String(),
		Status:         relationship.Status().String(),
		StartsAt:       relationship.Effective().StartsAt().UTC(),
	}
	if endsAt, bounded := relationship.Effective().EndsAt(); bounded {
		utc := endsAt.UTC()
		document.EndsAt = &utc
	}
	if approval, approvedAt, has := relationship.Approval(); has {
		document.ApprovalRef = approval.String()
		utc := approvedAt.UTC()
		document.ApprovedAt = &utc
	}
	if endedAt, has := relationship.EndedAt(); has {
		utc := endedAt.UTC()
		document.EndedAt = &utc
	}
	if endBasis, has := relationship.EndBasis(); has {
		document.EndBasis = endBasis.String()
	}
	if successor, has := relationship.Successor(); has {
		document.Successor = successor.String()
	}
	return document
}

func (repository *PartyIdentityRegistrations) SaveRelationship(
	ctx context.Context,
	registration domain.PartyRelationshipRegistration,
) (ports.PartyRegistrySaveOutcome, error) {
	const operation = "save party relationship"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}

	document := documentOfRelationship(registration)
	raw, err := json.Marshal(document)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	digest := sha256.Sum256(raw)
	contentDigest := hex.EncodeToString(digest[:])

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.party_relationship_registration
			(tenant_id, relationship_id, revision,
			 holder_party_id, counterparty_party_id, role, scope_ref, basis_ref, status,
			 effective_starts_at, effective_ends_at, approval_ref, approved_at,
			 ended_at, end_basis, successor_party_id, content_digest, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		document.Tenant, document.RelationshipID, document.Revision,
		document.Holder, document.Counterparty, document.Role, document.Scope,
		document.Basis, document.Status,
		document.StartsAt, document.EndsAt,
		nullableText(document.ApprovalRef), document.ApprovedAt,
		document.EndedAt, nullableText(document.EndBasis), nullableText(document.Successor),
		contentDigest, raw,
	)
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PartyRegistrySaved, nil
	}

	var existingDigest string
	err = executor.QueryRow(ctx,
		`SELECT content_digest
		   FROM party_commercial.party_relationship_registration
		  WHERE tenant_id = $1 AND relationship_id = $2 AND revision = $3`,
		document.Tenant, document.RelationshipID, document.Revision,
	).Scan(&existingDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return ports.PartyRegistrySaveOutcomeInvalid, fmt.Errorf("%s: %w", operation, err)
	}
	if existingDigest == contentDigest {
		return ports.PartyRegistryAlreadyRegistered, nil
	}
	return ports.PartyRegistryContentConflict, nil
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (repository *PartyIdentityRegistrations) LoadLatestBusinessParty(
	ctx context.Context,
	tenant domain.TenantID,
	party domain.PartyID,
) (domain.BusinessPartyRegistration, bool, error) {
	raw, found, err := repository.latestSnapshot(ctx,
		"business_party_registration", "party_id", tenant.String(), party.String())
	if err != nil || !found {
		return domain.BusinessPartyRegistration{}, found, err
	}
	registration, err := businessPartyFromSnapshot(raw)
	if err != nil {
		return domain.BusinessPartyRegistration{}, false, fmt.Errorf("load latest business party: %w", err)
	}
	return registration, true, nil
}

func (repository *PartyIdentityRegistrations) LoadLatestLegalEntity(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) (domain.LegalEntityRegistration, bool, error) {
	raw, found, err := repository.latestSnapshot(ctx,
		"legal_entity_registration", "legal_entity_id", tenant.String(), entity.String())
	if err != nil || !found {
		return domain.LegalEntityRegistration{}, found, err
	}
	registration, err := legalEntityFromSnapshot(raw)
	if err != nil {
		return domain.LegalEntityRegistration{}, false, fmt.Errorf("load latest legal entity: %w", err)
	}
	return registration, true, nil
}

func (repository *PartyIdentityRegistrations) LoadLatestCustomerAccount(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.CustomerAccountID,
) (domain.CustomerAccountRegistration, bool, error) {
	raw, found, err := repository.latestSnapshot(ctx,
		"customer_account_registration", "account_id", tenant.String(), account.String())
	if err != nil || !found {
		return domain.CustomerAccountRegistration{}, found, err
	}
	registration, err := customerAccountFromSnapshot(raw)
	if err != nil {
		return domain.CustomerAccountRegistration{}, false, fmt.Errorf("load latest customer account: %w", err)
	}
	return registration, true, nil
}

func (repository *PartyIdentityRegistrations) LoadLatestRelationship(
	ctx context.Context,
	tenant domain.TenantID,
	relationship domain.RelationshipID,
) (domain.PartyRelationshipRegistration, bool, error) {
	raw, found, err := repository.latestSnapshot(ctx,
		"party_relationship_registration", "relationship_id", tenant.String(), relationship.String())
	if err != nil || !found {
		return domain.PartyRelationshipRegistration{}, found, err
	}
	registration, err := relationshipFromSnapshot(raw)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, false, fmt.Errorf("load latest relationship: %w", err)
	}
	return registration, true, nil
}

func (repository *PartyIdentityRegistrations) latestSnapshot(
	ctx context.Context,
	table, idColumn, tenant, id string,
) ([]byte, bool, error) {
	operation := "load latest " + table
	if tenant == "" || id == "" {
		return nil, false, fmt.Errorf("%s: tenant and identifier are required", operation)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", operation, err)
	}
	var raw []byte
	err = querier.QueryRow(ctx,
		fmt.Sprintf(
			"SELECT snapshot FROM party_commercial.%s WHERE tenant_id = $1 AND %s = $2 ORDER BY revision DESC LIMIT 1",
			table, idColumn,
		),
		tenant, id,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", operation, err)
	}
	return raw, true, nil
}

func businessPartyFromSnapshot(raw []byte) (domain.BusinessPartyRegistration, error) {
	var document businessPartyDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.BusinessPartyRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.BusinessPartyRegistration{}, err
	}
	partyID, err := domain.NewPartyID(document.PartyID)
	if err != nil {
		return domain.BusinessPartyRegistration{}, err
	}
	name, err := domain.NewPartyName(document.PartyName)
	if err != nil {
		return domain.BusinessPartyRegistration{}, err
	}
	party, err := domain.NewBusinessParty(tenant, partyID, name)
	if err != nil {
		return domain.BusinessPartyRegistration{}, err
	}
	basis, lifecycle, err := document.identityLifecycleDocument.rebuild()
	if err != nil {
		return domain.BusinessPartyRegistration{}, err
	}
	return domain.NewBusinessPartyRegistration(party, document.Revision, basis, lifecycle)
}

func legalEntityFromSnapshot(raw []byte) (domain.LegalEntityRegistration, error) {
	var document legalEntityDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.LegalEntityRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	entityID, err := domain.NewLegalEntityReference(document.LegalEntityID)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	partyID, err := domain.NewPartyID(document.PartyID)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	entity, err := domain.RehydrateResponsibleLegalEntity(tenant, entityID, partyID)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	basis, lifecycle, err := document.identityLifecycleDocument.rebuild()
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	registration, err := domain.NewLegalEntityRegistration(entity, document.Revision, basis, lifecycle)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	layer, hasIdentity, err := identityLayerFromDocuments(document.RegistrationCountry, document.LifetimeNumbers)
	if err != nil {
		return domain.LegalEntityRegistration{}, err
	}
	if !hasIdentity {
		return registration, nil
	}
	var correction *domain.IdentityBasisReference
	if document.IdentityCorrectionBasis != "" {
		value, err := domain.NewIdentityBasisReference(document.IdentityCorrectionBasis)
		if err != nil {
			return domain.LegalEntityRegistration{}, err
		}
		correction = &value
	}
	return registration.WithIdentityLayer(layer, correction)
}

func customerAccountFromSnapshot(raw []byte) (domain.CustomerAccountRegistration, error) {
	var document customerAccountDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CustomerAccountRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.CustomerAccountRegistration{}, err
	}
	accountID, err := domain.NewCustomerAccountID(document.AccountID)
	if err != nil {
		return domain.CustomerAccountRegistration{}, err
	}
	partyID, err := domain.NewPartyID(document.CustomerParty)
	if err != nil {
		return domain.CustomerAccountRegistration{}, err
	}
	account, err := domain.RehydrateCustomerAccount(tenant, accountID, partyID)
	if err != nil {
		return domain.CustomerAccountRegistration{}, err
	}
	basis, lifecycle, err := document.identityLifecycleDocument.rebuild()
	if err != nil {
		return domain.CustomerAccountRegistration{}, err
	}
	return domain.NewCustomerAccountRegistration(account, document.Revision, basis, lifecycle)
}

func relationshipFromSnapshot(raw []byte) (domain.PartyRelationshipRegistration, error) {
	var document relationshipDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.PartyRelationshipRegistration{}, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	id, err := domain.NewRelationshipID(document.RelationshipID)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	holder, err := domain.NewPartyID(document.Holder)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	counterparty, err := domain.NewPartyID(document.Counterparty)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	role, err := partyRoleFrom(document.Role)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	scope, err := domain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	basis, err := domain.NewRelationshipBasisReference(document.Basis)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	endsAt := time.Time{}
	if document.EndsAt != nil {
		endsAt = *document.EndsAt
	}
	interval, err := domain.NewEffectiveInterval(document.StartsAt, endsAt)
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}

	// 经真转换重放到快照状态：候选 →（批准）→（终止）。写入走的就是这些门，读回
	// 走不通即坏数据，上抛不折成空。
	relationship, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       holder,
		Counterparty: counterparty,
		Role:         role,
		Scope:        scope,
		Basis:        basis,
		Effective:    interval,
	})
	if err != nil {
		return domain.PartyRelationshipRegistration{}, err
	}
	if document.Status != domain.RelationshipCandidate.String() {
		if document.ApprovedAt == nil {
			return domain.PartyRelationshipRegistration{}, fmt.Errorf("快照状态 %s 缺批准事实", document.Status)
		}
		approval, err := domain.NewApprovalReference(document.ApprovalRef)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
		relationship, err = relationship.Approve(approval, *document.ApprovedAt)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
	}
	switch document.Status {
	case domain.RelationshipCandidate.String(), domain.RelationshipEffective.String():
	case domain.RelationshipExpired.String():
		if document.EndedAt == nil {
			return domain.PartyRelationshipRegistration{}, fmt.Errorf("快照状态 EXPIRED 缺终止时点")
		}
		relationship, err = relationship.Expire(*document.EndedAt)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
	case domain.RelationshipRevoked.String():
		if document.EndedAt == nil {
			return domain.PartyRelationshipRegistration{}, fmt.Errorf("快照状态 REVOKED 缺终止时点")
		}
		endBasis, err := domain.NewRelationshipBasisReference(document.EndBasis)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
		relationship, err = relationship.Revoke(endBasis, *document.EndedAt)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
	case domain.RelationshipSuperseded.String():
		if document.EndedAt == nil {
			return domain.PartyRelationshipRegistration{}, fmt.Errorf("快照状态 SUPERSEDED 缺终止时点")
		}
		successor, err := domain.NewPartyID(document.Successor)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
		relationship, err = relationship.SupersededBy(successor, *document.EndedAt)
		if err != nil {
			return domain.PartyRelationshipRegistration{}, err
		}
	default:
		return domain.PartyRelationshipRegistration{}, fmt.Errorf("unknown relationship status %q", document.Status)
	}

	return domain.NewPartyRelationshipRegistration(tenant, id, document.Revision, relationship)
}

func partyRoleFrom(raw string) (domain.PartyRole, error) {
	for _, role := range []domain.PartyRole{
		domain.CustomerRole, domain.SupplierRole, domain.CarrierAgentRole,
		domain.ResellerRole, domain.AccountHolderRole,
	} {
		if role.String() == raw {
			return role, nil
		}
	}
	return domain.PartyRoleInvalid, fmt.Errorf("unknown party role %q", raw)
}
