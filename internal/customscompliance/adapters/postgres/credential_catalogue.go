package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
// 按 ports.CredentialPages 取页，缺省序是凭证身份升序。limit 非正是调用方编程错误：
// 静默答一页会把「忘了传」变成一个没人决定过的页大小。
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

// credentialPages 把 ports.CredentialPages 落到列上。q 在凭证身份、签发方、持有方与所涉程序上做字面包含。
var credentialPages = ports.CredentialPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"credential": "credential_id", "registeredAt": "registered_at"},
	Identity: []string{"credential_id"},
	Keyword:  []string{"credential_id", "issuer_ref", "holder_ref", "procedure_ref"},
})

// scannedCredential 留着凭证身份与登记时刻两列的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象。
type scannedCredential struct {
	entry        ports.CredentialCatalogueEntry
	id           string
	registeredAt time.Time
}

func (catalogue *CredentialCatalogue) ListCredentials(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CredentialCatalogueEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.CredentialCatalogueEntry], error) {
		return cataloguepage.Result[ports.CredentialCatalogueEntry]{}, fmt.Errorf("list credentials: %w", err)
	}
	const source = "customs_compliance.regulatory_credential"
	page, err := openPage(ctx, catalogue.db, credentialPages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`credential_id, issuer_ref, holder_ref, procedure_ref,
		        valid_from, valid_to, uses, registered_at, operation_evidence`,
		source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedCredential, 0, limit+1)
	for rows.Next() {
		var (
			row                       scannedCredential
			issuer, holder, procedure string
			validFrom, validTo        time.Time
			uses                      int
			evidence                  *string
		)
		if err := rows.Scan(&row.id, &issuer, &holder, &procedure,
			&validFrom, &validTo, &uses, &row.registeredAt, &evidence); err != nil {
			return fail(err)
		}
		id, err := domain.NewCredentialID(row.id)
		if err != nil {
			return fail(fmt.Errorf("rebuild credential catalogue: %w", err))
		}
		credential, err := rebuildCredential(id, issuer, holder, procedure, validFrom, validTo, uses)
		if err != nil {
			return fail(fmt.Errorf("rebuild credential catalogue: %w", err))
		}
		row.entry = ports.CredentialCatalogueEntry{
			Credential:   credential,
			RegisteredAt: row.registeredAt.UTC(),
		}
		if evidence != nil {
			row.entry.OperationEvidence = *evidence
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedCredential, sortField string) cataloguepage.Position {
			value := row.id
			if sortField == "registeredAt" {
				value = cataloguepage.FormatInstant(row.registeredAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.id}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedCredential) ports.CredentialCatalogueEntry { return row.entry }), nil
}
