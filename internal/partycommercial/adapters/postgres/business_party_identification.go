package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是业务参与方识别信息（ADR-0367，迁移 0045）在本适配器里的两半：快照与镜像列的形状，以及按号、按外部编码
// 找在用持有方的查重读口。

// externalReferenceDocument 是外部系统编码在快照与 external_references 列里的同一个形状（0045）。
type externalReferenceDocument struct {
	System  string `json:"system"`
	Purpose string `json:"purpose"`
	Code    string `json:"code"`
}

func externalReferenceDocumentsOf(references []domain.ExternalReference) []externalReferenceDocument {
	if len(references) == 0 {
		return nil
	}
	documents := make([]externalReferenceDocument, len(references))
	for index, reference := range references {
		documents[index] = externalReferenceDocument{
			System:  reference.System().String(),
			Purpose: reference.Purpose().String(),
			Code:    reference.Code(),
		}
	}
	return documents
}

func externalReferencesFromDocuments(documents []externalReferenceDocument) ([]domain.ExternalReference, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	references := make([]domain.ExternalReference, len(documents))
	for index, document := range documents {
		system, err := domain.NewExternalSystemCode(document.System)
		if err != nil {
			return nil, err
		}
		purpose, err := domain.ParseExternalReferencePurpose(document.Purpose)
		if err != nil {
			return nil, err
		}
		if references[index], err = domain.NewExternalReference(system, purpose, document.Code); err != nil {
			return nil, err
		}
	}
	return references, nil
}

// businessPartyIdentificationColumns 是识别信息四列从库里读出来的原样：国家与更正依据可空，两个数组是 jsonb 原文。
type businessPartyIdentificationColumns struct {
	country    *string
	numbers    []byte
	correction *string
	references []byte
}

// apply 把四列转写进上列行的识别信息；国家为空即这笔修订没带号。
func (columns businessPartyIdentificationColumns) apply(row *ports.BusinessPartyIdentificationRow) error {
	if columns.country != nil {
		var documents []lifetimeNumberDocument
		if err := json.Unmarshal(columns.numbers, &documents); err != nil {
			return fmt.Errorf("lifetime_registration_numbers 不是本适配器写下的形状：%w", err)
		}
		row.HasIdentityLayer = true
		row.RegistrationCountry = *columns.country
		row.LifetimeNumbers = make([]ports.LifetimeRegistrationNumberRow, len(documents))
		for index, document := range documents {
			row.LifetimeNumbers[index] = ports.LifetimeRegistrationNumberRow{TypeCode: document.TypeCode, Number: document.Number}
		}
	}
	if columns.correction != nil {
		row.HasIdentityCorrection = true
		row.IdentityCorrectionBasis = *columns.correction
	}
	if columns.references != nil {
		var documents []externalReferenceDocument
		if err := json.Unmarshal(columns.references, &documents); err != nil {
			return fmt.Errorf("external_references 不是本适配器写下的形状：%w", err)
		}
		row.ExternalReferences = make([]ports.ExternalReferenceRow, len(documents))
		for index, document := range documents {
			row.ExternalReferences[index] = ports.ExternalReferenceRow{
				System: document.System, Purpose: document.Purpose, Code: document.Code,
			}
		}
	}
	return nil
}

// identificationLockKey 是查重咨询锁的键。按租户分键：两个租户登同一个号互不相干，不该彼此排队。
func identificationLockKey(parts ...string) string {
	key := "party-commercial/identification"
	for _, part := range parts {
		key += "\x1f" + part
	}
	return key
}

// liveCondition 是「在用」在 SQL 上的写法：这一笔没停用，或停用时点还没到。与目录 status 导出的停用判断同一条。
const liveCondition = `NOT (revision.deactivated_at IS NOT NULL AND revision.deactivated_at <= now())`

// 查重查询的形状：先按 jsonb 包含在任意一笔修订上命中（走 0045 的 GIN 索引），再用 NOT EXISTS 留下各身份的最新那一笔。
// 反过来先 DISTINCT ON 取最新再过滤，索引用不上，每次登记都要读遍租户的全部修订。

// FindRegistrationNumberHolders 交回身份层上每个号此刻的在用持有方，往来单位与责任法人一并查（ADR-0367 决定二）。
// 先按号取事务级咨询锁、再查：理由在端口注释。号已按类型代码排好，各笔登记取锁的次序一致，不会互相等成死锁。
func (repository *PartyIdentityRegistrations) FindRegistrationNumberHolders(
	ctx context.Context,
	tenant domain.TenantID,
	identity domain.LegalEntityIdentityLayer,
) ([]ports.RegistrationNumberHolder, error) {
	const operation = "find registration number holders"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	country := identity.Country().String()
	holders := make([]ports.RegistrationNumberHolder, 0)
	for _, number := range identity.Numbers() {
		typeCode, value := number.TypeCode().String(), number.Number().String()
		if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			identificationLockKey(tenant.String(), "number", country, typeCode, value)); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		probe, err := json.Marshal([]lifetimeNumberDocument{{TypeCode: typeCode, Number: value}})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		rows, err := executor.Query(ctx,
			`SELECT 'BUSINESS_PARTY', revision.party_id, revision.party_id
			   FROM party_commercial.business_party_registration AS revision
			  WHERE revision.tenant_id = $1 AND revision.registration_country = $2
			    AND revision.lifetime_registration_numbers @> $3::jsonb AND `+liveCondition+`
			    AND NOT EXISTS (
			        SELECT 1 FROM party_commercial.business_party_registration AS newer
			         WHERE newer.tenant_id = revision.tenant_id AND newer.party_id = revision.party_id
			           AND newer.revision > revision.revision)
			 UNION ALL
			 SELECT 'LEGAL_ENTITY', revision.legal_entity_id, revision.party_id
			   FROM party_commercial.legal_entity_registration AS revision
			  WHERE revision.tenant_id = $1 AND revision.registration_country = $2
			    AND revision.lifetime_registration_numbers @> $3::jsonb AND `+liveCondition+`
			    AND NOT EXISTS (
			        SELECT 1 FROM party_commercial.legal_entity_registration AS newer
			         WHERE newer.tenant_id = revision.tenant_id AND newer.legal_entity_id = revision.legal_entity_id
			           AND newer.revision > revision.revision)
			  ORDER BY 1, 2`,
			tenant.String(), country, string(probe),
		)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		for rows.Next() {
			var kind string
			holder := ports.RegistrationNumberHolder{TypeCode: typeCode, Number: value}
			if err := rows.Scan(&kind, &holder.ID, &holder.Party); err != nil {
				rows.Close()
				return nil, fmt.Errorf("%s: %w", operation, err)
			}
			holder.Kind = ports.IdentificationHeldByBusinessParty
			if kind == "LEGAL_ENTITY" {
				holder.Kind = ports.IdentificationHeldByLegalEntity
			}
			holders = append(holders, holder)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
	}
	return holders, nil
}

// FindExternalReferenceHolders 交回每个外部编码此刻的在用持有方（ADR-0367 决定三），取锁判据同上；编码已按（系统，用途）
// 排好。
func (repository *PartyIdentityRegistrations) FindExternalReferenceHolders(
	ctx context.Context,
	tenant domain.TenantID,
	references []domain.ExternalReference,
) ([]ports.ExternalReferenceHolder, error) {
	const operation = "find external reference holders"
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	holders := make([]ports.ExternalReferenceHolder, 0)
	for _, document := range externalReferenceDocumentsOf(references) {
		if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			identificationLockKey(tenant.String(), "external", document.System, document.Purpose, document.Code)); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		probe, err := json.Marshal([]externalReferenceDocument{document})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		rows, err := executor.Query(ctx,
			`SELECT revision.party_id
			   FROM party_commercial.business_party_registration AS revision
			  WHERE revision.tenant_id = $1 AND revision.external_references @> $2::jsonb AND `+liveCondition+`
			    AND NOT EXISTS (
			        SELECT 1 FROM party_commercial.business_party_registration AS newer
			         WHERE newer.tenant_id = revision.tenant_id AND newer.party_id = revision.party_id
			           AND newer.revision > revision.revision)
			  ORDER BY 1`,
			tenant.String(), string(probe),
		)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		for rows.Next() {
			holder := ports.ExternalReferenceHolder{System: document.System, Purpose: document.Purpose, Code: document.Code}
			if err := rows.Scan(&holder.Party); err != nil {
				rows.Close()
				return nil, fmt.Errorf("%s: %w", operation, err)
			}
			holders = append(holders, holder)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
	}
	return holders, nil
}

// FindLegalEntitiesPinning 交回最新修订钉着这个参与方的责任法人，已停用的也算：停用了的公司主体仍是我方公司，它的号
// 仍只登在公司主体上。先按（租户，参与方）取事务级咨询锁再查：「给参与方登号」与「拿公司主体钉这个参与方」各查对方
// 一眼再落册，不锁的话两笔并发时都看不见对方，参与方就既被钉着又带着号。两条路径都在读对方之前取这把锁。
func (repository *PartyIdentityRegistrations) FindLegalEntitiesPinning(
	ctx context.Context,
	tenant domain.TenantID,
	party domain.PartyID,
) ([]string, error) {
	const operation = "find legal entities pinning a party"
	querier, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := querier.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		identificationLockKey(tenant.String(), "pin", party.String())); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT latest.legal_entity_id
		   FROM (
		        SELECT DISTINCT ON (legal_entity_id) legal_entity_id, party_id
		          FROM party_commercial.legal_entity_registration
		         WHERE tenant_id = $1
		         ORDER BY legal_entity_id, revision DESC
		   ) AS latest
		  WHERE latest.party_id = $2
		  ORDER BY 1`,
		tenant.String(), party.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()
	entities := make([]string, 0)
	for rows.Next() {
		var entity string
		if err := rows.Scan(&entity); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		entities = append(entities, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return entities, nil
}
