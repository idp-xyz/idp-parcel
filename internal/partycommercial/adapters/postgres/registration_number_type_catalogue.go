package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把注册号类型目录读端口（ADR-0077）挂在 OperationsCatalogue 上。上列对象是各类型的
// **最新登记修订**；修订史是登记册的证据面，不是目录的行（判据同参与方身份目录）。
var _ ports.RegistrationNumberTypeCatalogueRead = (*OperationsCatalogue)(nil)

// ListRegistrationNumberTypes 上列注册号类型的最新修订。status 的导出与参与方身份册逐字相同
// （停用判断在先，其次生效时点）：两册的生命周期是同样三格。
func (catalogue *OperationsCatalogue) ListRegistrationNumberTypes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.RegistrationNumberTypeRow, error) {
	if err := requirePositiveLimit("list registration number types", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list registration number types: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT entry.tenant_id, entry.country_code, entry.type_code, entry.revision,
		        entry.type_name, entry.layer, entry.format_pattern, entry.basis_ref,
		        entry.effective_from, entry.deactivated_at, entry.deactivation_basis, entry.recorded_at,
		        CASE
		            WHEN entry.deactivated_at IS NOT NULL AND entry.deactivated_at <= now()
		                THEN 'DEACTIVATED'
		            WHEN entry.effective_from <= now() THEN 'EFFECTIVE'
		            ELSE 'REGISTERED'
		        END AS status
		   FROM (
		        SELECT DISTINCT ON (country_code, type_code) *
		          FROM party_commercial.registration_number_type_registration
		         WHERE tenant_id = $1
		         ORDER BY country_code, type_code, revision DESC
		   ) AS entry
		  ORDER BY entry.recorded_at DESC, entry.country_code, entry.type_code
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list registration number types: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.RegistrationNumberTypeRow, 0, limit)
	for rows.Next() {
		var row ports.RegistrationNumberTypeRow
		var deactivatedAt *time.Time
		var deactivationBasis *string
		if err := rows.Scan(
			&row.TenantID, &row.CountryCode, &row.TypeCode, &row.Revision,
			&row.TypeName, &row.Layer, &row.FormatPattern, &row.Basis,
			&row.EffectiveFrom, &deactivatedAt, &deactivationBasis, &row.RegisteredAt,
			&row.Status,
		); err != nil {
			return nil, fmt.Errorf("list registration number types: %w", err)
		}
		if deactivatedAt != nil {
			row.DeactivatedAt = *deactivatedAt
			row.HasDeactivation = true
			if deactivationBasis != nil {
				row.DeactivationBasis = *deactivationBasis
			}
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list registration number types: %w", err)
	}
	return catalogueRows, nil
}
