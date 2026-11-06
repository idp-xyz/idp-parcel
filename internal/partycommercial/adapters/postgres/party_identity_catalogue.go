package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件把参与方身份目录读端口（ADR-0077）挂在 OperationsCatalogue 上：集团与法人、
// 业务参与方两页的供数面。上列对象是各身份/关系的**最新修订**；修订史是登记册的
// 证据面，不是目录的行。
//
// 翻页、排序与筛选照 ADR-0144（票 catalogue-read-pagination-rollout/11）：先把每个身份折叠到最新修订、导出状态、左连接
// 名称（各册的 listed），再在它上面按筛选与 q 数总数、按「排序维值 + 标识」取游标之后的行——状态与名称都是折叠之后才有
// 的列，条件只能落在那之后。各册的声明在 ports.BusinessPartyPages 等处。
var _ ports.PartyIdentityCatalogueRead = (*OperationsCatalogue)(nil)

// identityNumbersText 把 jsonb 号数组里的号抽成一段文本供 q 覆盖：整列转文本会让检索词命中键名（typeCode、number）与引号，
// 所以只抽值。各号之间用 chr(31) 隔开——检索词里打不出这个字符，带空格的检索词就不会把相邻两个号拼成一次命中。alias 是带
// lifetime_registration_numbers 列的那一行。
func identityNumbersText(alias string) string {
	return `(SELECT string_agg(entry.value ->> 'number', chr(31))
		          FROM jsonb_array_elements(` + alias + `.lifetime_registration_numbers) AS entry(value))`
}

// businessPartyListed 是业务参与方总数与取页共用的前半句。它不左连接任何东西：名称就在本册行上。status 的导出与法人册、
// 客户账户册逐字相同（停用判断在先，其次生效时点），因为它们用的是同一个 domain.IdentityLifecycle。外部编码照管理台的
// 检索口径每项抽成「系统 编码」一段，q 写系统名加编码也命中；项与项之间用 chr(31) 隔开（理由同 identityNumbersText）。
var businessPartyListed = `WITH party AS (
        SELECT DISTINCT ON (party_id) *
          FROM party_commercial.business_party_registration
         WHERE tenant_id = $1
         ORDER BY party_id, revision DESC
     ), listed AS (
        SELECT party.tenant_id, party.party_id, party.party_name,
               party.revision, party.basis_ref, party.effective_from,
               party.deactivated_at, party.deactivation_basis, party.recorded_at,
               party.registration_country, party.lifetime_registration_numbers,
               party.identity_correction_basis, party.external_references,
               CASE
                   WHEN party.deactivated_at IS NOT NULL AND party.deactivated_at <= now()
                       THEN 'DEACTIVATED'
                   WHEN party.effective_from <= now() THEN 'EFFECTIVE'
                   ELSE 'REGISTERED'
               END AS status,
               ` + identityNumbersText("party") + ` AS registration_numbers,
               (SELECT string_agg(concat_ws(' ', entry.value ->> 'system', entry.value ->> 'code'), chr(31))
                  FROM jsonb_array_elements(party.external_references) AS entry(value)) AS external_codes
          FROM party
     )`

// businessPartyRead：q 覆盖参与方号、名称与行上的识别信息（终身注册号、外部编码），即管理台检索的那几格。
var businessPartyRead = catalogueRead[ports.BusinessPartyRow]{
	binding: ports.BusinessPartyPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"registeredAt": "recorded_at", "effectiveFrom": "effective_from", "partyId": "party_id"},
		Identity: []string{"party_id"},
		Filters:  map[string]string{"status": "status"},
		Keyword:  []string{"party_id", "party_name", "registration_numbers", "external_codes"},
	}),
	listed: businessPartyListed,
	selection: `tenant_id, party_id, party_name,
		        revision, basis_ref, effective_from,
		        deactivated_at, deactivation_basis, recorded_at,
		        registration_country, lifetime_registration_numbers,
		        identity_correction_basis, external_references, status`,
	scan:     scanBusinessParty,
	position: businessPartyPosition,
}

// ListBusinessParties 上列业务参与方身份本体的最新修订。
func (catalogue *OperationsCatalogue) ListBusinessParties(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.BusinessPartyRow], error) {
	return businessPartyRead.page(ctx, catalogue.db, "list business parties", limit, query, tenant.String())
}

func scanBusinessParty(rows pgx.Rows) (ports.BusinessPartyRow, error) {
	var row ports.BusinessPartyRow
	var deactivatedAt *time.Time
	var deactivationBasis *string
	var identification businessPartyIdentificationColumns
	if err := rows.Scan(
		&row.TenantID, &row.PartyID, &row.PartyName,
		&row.Revision, &row.Basis, &row.EffectiveFrom,
		&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
		&identification.country, &identification.numbers,
		&identification.correction, &identification.references,
		&row.Status,
	); err != nil {
		return row, err
	}
	if err := identification.apply(&row.BusinessPartyIdentificationRow); err != nil {
		return row, err
	}
	if deactivatedAt != nil {
		row.DeactivatedAt = *deactivatedAt
		row.HasDeactivation = true
		if deactivationBasis != nil {
			row.DeactivationBasis = *deactivationBasis
		}
	}
	return row, nil
}

func businessPartyPosition(row ports.BusinessPartyRow, sortField string) cataloguepage.Position {
	value := row.PartyID
	switch sortField {
	case "registeredAt":
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveFrom":
		value = cataloguepage.FormatInstant(row.EffectiveFrom)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.PartyID}}
}

// groupLegalEntityListed 是法人总数与取页共用的前半句：法人折叠到最新修订，左连接参与方册的最新修订取名称。
//
// status 在 SQL 里按 now() 导出，是 domain.IdentityLifecycle.StatusAt 的逐字镜像：
// 停用判断在先（生效前撤下的登记自撤下时点起即已停用），其次生效时点。目录答的是
// 「装载时点的状态」，行上同时携带全部生命周期事实，消费方要别的时点自己判。
var groupLegalEntityListed = `WITH entity AS (
        SELECT DISTINCT ON (legal_entity_id) *
          FROM party_commercial.legal_entity_registration
         WHERE tenant_id = $1
         ORDER BY legal_entity_id, revision DESC
     ), party AS (
        SELECT DISTINCT ON (party_id) party_id, party_name
          FROM party_commercial.business_party_registration
         WHERE tenant_id = $1
         ORDER BY party_id, revision DESC
     ), listed AS (
        SELECT entity.tenant_id, entity.legal_entity_id, entity.party_id,
               party.party_name,
               entity.revision, entity.basis_ref, entity.effective_from,
               entity.deactivated_at, entity.deactivation_basis, entity.recorded_at,
               entity.registration_country, entity.lifetime_registration_numbers,
               entity.identity_correction_basis,
               CASE
                   WHEN entity.deactivated_at IS NOT NULL AND entity.deactivated_at <= now()
                       THEN 'DEACTIVATED'
                   WHEN entity.effective_from <= now() THEN 'EFFECTIVE'
                   ELSE 'REGISTERED'
               END AS status,
               ` + identityNumbersText("entity") + ` AS registration_numbers
          FROM entity
          LEFT JOIN party ON party.party_id = entity.party_id
     )`

// groupLegalEntityRead：q 覆盖法人号、所钉参与方的标识与名称、注册国家与终身注册号，即管理台检索的那几格。
var groupLegalEntityRead = catalogueRead[ports.GroupLegalEntityRow]{
	binding: ports.GroupLegalEntityPages.MustBind(cataloguepage.Columns{
		Sorts: map[string]string{
			"registeredAt": "recorded_at", "effectiveFrom": "effective_from", "legalEntityId": "legal_entity_id",
		},
		Identity: []string{"legal_entity_id"},
		Filters:  map[string]string{"status": "status", "registrationCountry": "registration_country"},
		Keyword: []string{
			"legal_entity_id", "party_id", "party_name", "registration_country", "registration_numbers",
		},
	}),
	listed: groupLegalEntityListed,
	selection: `tenant_id, legal_entity_id, party_id,
		        party_name,
		        revision, basis_ref, effective_from,
		        deactivated_at, deactivation_basis, recorded_at,
		        registration_country, lifetime_registration_numbers,
		        identity_correction_basis, status`,
	scan:     scanGroupLegalEntity,
	position: groupLegalEntityPosition,
}

// ListGroupLegalEntities 上列责任法人的最新修订，名称从参与方册转写。
func (catalogue *OperationsCatalogue) ListGroupLegalEntities(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
	return groupLegalEntityRead.page(ctx, catalogue.db, "list group legal entities", limit, query, tenant.String())
}

func scanGroupLegalEntity(rows pgx.Rows) (ports.GroupLegalEntityRow, error) {
	var row ports.GroupLegalEntityRow
	var partyName *string
	var deactivatedAt *time.Time
	var deactivationBasis *string
	var identity identityLayerColumns
	if err := rows.Scan(
		&row.TenantID, &row.LegalEntityID, &row.PartyID,
		&partyName,
		&row.Revision, &row.Basis, &row.EffectiveFrom,
		&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
		&identity.country, &identity.numbers, &identity.correction,
		&row.Status,
	); err != nil {
		return row, err
	}
	if err := identity.apply(
		&row.HasIdentityLayer, &row.RegistrationCountry, &row.LifetimeNumbers,
		&row.HasIdentityCorrection, &row.IdentityCorrectionBasis,
	); err != nil {
		return row, err
	}
	if partyName != nil {
		row.PartyName = *partyName
		row.HasPartyName = true
	}
	if deactivatedAt != nil {
		row.DeactivatedAt = *deactivatedAt
		row.HasDeactivation = true
		if deactivationBasis != nil {
			row.DeactivationBasis = *deactivationBasis
		}
	}
	return row, nil
}

func groupLegalEntityPosition(row ports.GroupLegalEntityRow, sortField string) cataloguepage.Position {
	value := row.LegalEntityID
	switch sortField {
	case "registeredAt":
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveFrom":
		value = cataloguepage.FormatInstant(row.EffectiveFrom)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.LegalEntityID}}
}

// partyRelationshipListed 是关系总数与取页共用的前半句：关系折叠到最新修订，双方名称从参与方册左连接转写。
//
// status 照存的关系状态事实转写（CANDIDATE/EFFECTIVE/EXPIRED/REVOKED/SUPERSEDED），
// 不随装载时钟走：关系是否仍在有效区间内由消费方对区间判断，目录不代答——与身份
// 三册的时点导出状态刻意不同，两种状态的来源不同（关系状态经登记的真转换到达，
// 身份状态是生效/停用时点的推导）。
var partyRelationshipListed = `WITH relation AS (
        SELECT DISTINCT ON (relationship_id) *
          FROM party_commercial.party_relationship_registration
         WHERE tenant_id = $1
         ORDER BY relationship_id, revision DESC
     ), party AS (
        SELECT DISTINCT ON (party_id) party_id, party_name
          FROM party_commercial.business_party_registration
         WHERE tenant_id = $1
         ORDER BY party_id, revision DESC
     ), listed AS (
        SELECT relation.tenant_id, relation.relationship_id, relation.revision,
               relation.holder_party_id, holder.party_name AS holder_name,
               relation.counterparty_party_id, counterparty.party_name AS counterparty_name,
               relation.role, relation.scope_ref, relation.basis_ref, relation.status,
               relation.effective_starts_at, relation.effective_ends_at,
               relation.ended_at, relation.end_basis, relation.successor_party_id,
               relation.recorded_at
          FROM relation
          LEFT JOIN party AS holder ON holder.party_id = relation.holder_party_id
          LEFT JOIN party AS counterparty ON counterparty.party_id = relation.counterparty_party_id
     )`

// partyRelationshipRead：q 覆盖关系号与双方的标识、名称，即管理台检索的那几格（角色与状态是筛选维）。
var partyRelationshipRead = catalogueRead[ports.PartyRelationshipRow]{
	binding: ports.PartyRelationshipPages.MustBind(cataloguepage.Columns{
		Sorts: map[string]string{
			"registeredAt": "recorded_at", "effectiveStartsAt": "effective_starts_at", "relationshipId": "relationship_id",
		},
		Identity: []string{"relationship_id"},
		Filters:  map[string]string{"status": "status", "role": "role"},
		Keyword: []string{
			"relationship_id", "holder_party_id", "holder_name", "counterparty_party_id", "counterparty_name",
		},
	}),
	listed: partyRelationshipListed,
	selection: `tenant_id, relationship_id, revision,
		        holder_party_id, holder_name,
		        counterparty_party_id, counterparty_name,
		        role, scope_ref, basis_ref, status,
		        effective_starts_at, effective_ends_at,
		        ended_at, end_basis, successor_party_id,
		        recorded_at`,
	scan:     scanPartyRelationship,
	position: partyRelationshipPosition,
}

// ListPartyRelationships 上列参与方关系的最新修订。
func (catalogue *OperationsCatalogue) ListPartyRelationships(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
	return partyRelationshipRead.page(ctx, catalogue.db, "list party relationships", limit, query, tenant.String())
}

func scanPartyRelationship(rows pgx.Rows) (ports.PartyRelationshipRow, error) {
	var row ports.PartyRelationshipRow
	var holderName, counterpartyName *string
	var endsAt, endedAt *time.Time
	var endBasis, successor *string
	if err := rows.Scan(
		&row.TenantID, &row.RelationshipID, &row.Revision,
		&row.HolderID, &holderName,
		&row.CounterpartyID, &counterpartyName,
		&row.Role, &row.Scope, &row.Basis, &row.Status,
		&row.EffectiveStartsAt, &endsAt,
		&endedAt, &endBasis, &successor,
		&row.RegisteredAt,
	); err != nil {
		return row, err
	}
	if holderName != nil {
		row.HolderName = *holderName
		row.HasHolderName = true
	}
	if counterpartyName != nil {
		row.CounterpartyName = *counterpartyName
		row.HasCounterpartyName = true
	}
	if endsAt != nil {
		row.EffectiveEndsAt = *endsAt
		row.HasEffectiveEnd = true
	}
	if endedAt != nil {
		row.EndedAt = *endedAt
		row.HasEnd = true
	}
	if endBasis != nil {
		row.EndBasis = *endBasis
	}
	if successor != nil {
		row.SuccessorID = *successor
	}
	return row, nil
}

func partyRelationshipPosition(row ports.PartyRelationshipRow, sortField string) cataloguepage.Position {
	value := row.RelationshipID
	switch sortField {
	case "registeredAt":
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveStartsAt":
		value = cataloguepage.FormatInstant(row.EffectiveStartsAt)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.RelationshipID}}
}
