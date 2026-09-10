package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CredentialCatalogue 实现 ports.CredentialCatalogueRead：监管凭证登记册的列表读面
// （ADR-0077，票 sa-cc/10）。点读 CredentialView 按凭证身份单点作答伺候适用性判断，
// 这里按租户上列伺候查阅——两种读法各答各的问题，谁也不为对方改形状（判据同
// GateConditionCatalogue）。
//
// 读回经领域构造重建（rebuildCredential，与点读同一条路）：库里一行若立不起
// RegulatoryCredential，说明有人绕过写口改了它，作错误抛出而不是交回一个半成品对象。
// uses 为零由领域译成「来源未提供」——本读口不在 SQL 里替它折成任何别的东西。
//
// 排序按凭证身份升序，保证分页可重复。limit 非正是调用方编程错误：静默答一页会把
// 「忘了传」变成一个没人决定过的页大小。
type CredentialCatalogue struct {
	db *bentopg.DB
}

func NewCredentialCatalogue(db *bentopg.DB) (*CredentialCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CredentialCatalogue{db: db}, nil
}

var _ ports.CredentialCatalogueRead = (*CredentialCatalogue)(nil)

func (catalogue *CredentialCatalogue) ListCredentials(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CredentialCatalogueEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list credentials: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT credential_id, issuer_ref, holder_ref, procedure_ref,
		        valid_from, valid_to, uses, registered_at
		   FROM customs_compliance.regulatory_credential
		  WHERE tenant_id = $1
		  ORDER BY credential_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CredentialCatalogueEntry, 0, limit)
	for rows.Next() {
		var (
			idRaw, issuer, holder, procedure string
			validFrom, validTo, registeredAt time.Time
			uses                             int
		)
		if err := rows.Scan(&idRaw, &issuer, &holder, &procedure,
			&validFrom, &validTo, &uses, &registeredAt); err != nil {
			return nil, fmt.Errorf("list credentials: %w", err)
		}
		id, err := domain.NewCredentialID(idRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild credential catalogue: %w", err)
		}
		credential, err := rebuildCredential(id, issuer, holder, procedure, validFrom, validTo, uses)
		if err != nil {
			return nil, fmt.Errorf("rebuild credential catalogue: %w", err)
		}
		entries = append(entries, ports.CredentialCatalogueEntry{
			Credential:   credential,
			RegisteredAt: registeredAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	return entries, nil
}
