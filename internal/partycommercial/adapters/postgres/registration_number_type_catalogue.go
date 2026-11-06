package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件把注册号类型目录读端口（ADR-0077）挂在 OperationsCatalogue 上。上列对象是各类型的
// **最新登记修订**；修订史是登记册的证据面，不是目录的行（判据同参与方身份目录）。翻页、排序与筛选照 ADR-0144，
// 折叠与状态导出都在 listed 里、条件落在它之后，理由同身份三册；声明在 ports.RegistrationNumberTypePages。
var _ ports.RegistrationNumberTypeCatalogueRead = (*OperationsCatalogue)(nil)

// registrationNumberTypeListed 是总数与取页共用的前半句。status 的导出与参与方身份册逐字相同（停用判断在先，其次生效
// 时点）：两册的生命周期是同样三格。
var registrationNumberTypeListed = `WITH entry AS (
        SELECT DISTINCT ON (country_code, type_code) *
          FROM party_commercial.registration_number_type_registration
         WHERE tenant_id = $1
         ORDER BY country_code, type_code, revision DESC
     ), listed AS (
        SELECT entry.tenant_id, entry.country_code, entry.type_code, entry.revision,
               entry.type_name, entry.layer, entry.format_pattern, entry.basis_ref,
               entry.effective_from, entry.deactivated_at, entry.deactivation_basis, entry.recorded_at,
               CASE
                   WHEN entry.deactivated_at IS NOT NULL AND entry.deactivated_at <= now()
                       THEN 'DEACTIVATED'
                   WHEN entry.effective_from <= now() THEN 'EFFECTIVE'
                   ELSE 'REGISTERED'
               END AS status
          FROM entry
     )`

// registrationNumberTypeRead：q 覆盖国家 / 地区、类型代码与类型名称——登记表单挑候选时认的就是这三格。
var registrationNumberTypeRead = catalogueRead[ports.RegistrationNumberTypeRow]{
	binding: ports.RegistrationNumberTypePages.MustBind(cataloguepage.Columns{
		Sorts: map[string]string{
			"registeredAt": "recorded_at", "effectiveFrom": "effective_from", "countryCode": "country_code",
		},
		Identity: []string{"country_code", "type_code"},
		Filters:  map[string]string{"status": "status", "layer": "layer", "countryCode": "country_code"},
		Keyword:  []string{"country_code", "type_code", "type_name"},
	}),
	listed: registrationNumberTypeListed,
	selection: `tenant_id, country_code, type_code, revision,
		        type_name, layer, format_pattern, basis_ref,
		        effective_from, deactivated_at, deactivation_basis, recorded_at, status`,
	scan:     scanRegistrationNumberType,
	position: registrationNumberTypePosition,
}

// ListRegistrationNumberTypes 上列注册号类型的最新修订。
func (catalogue *OperationsCatalogue) ListRegistrationNumberTypes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error) {
	return registrationNumberTypeRead.page(ctx, catalogue.db, "list registration number types", limit, query, tenant.String())
}

func scanRegistrationNumberType(rows pgx.Rows) (ports.RegistrationNumberTypeRow, error) {
	var row ports.RegistrationNumberTypeRow
	var deactivatedAt *time.Time
	var deactivationBasis *string
	if err := rows.Scan(
		&row.TenantID, &row.CountryCode, &row.TypeCode, &row.Revision,
		&row.TypeName, &row.Layer, &row.FormatPattern, &row.Basis,
		&row.EffectiveFrom, &deactivatedAt, &deactivationBasis, &row.RegisteredAt,
		&row.Status,
	); err != nil {
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

func registrationNumberTypePosition(row ports.RegistrationNumberTypeRow, sortField string) cataloguepage.Position {
	value := row.CountryCode
	switch sortField {
	case "registeredAt":
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveFrom":
		value = cataloguepage.FormatInstant(row.EffectiveFrom)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.CountryCode, row.TypeCode}}
}
