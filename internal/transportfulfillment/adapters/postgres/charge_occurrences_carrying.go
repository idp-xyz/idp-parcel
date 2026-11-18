package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.ChargeOccurrencesCarryingParcel = (*ChargeOccurrences)(nil)

// ListCurrentCarrying 列出仍有效、成员里有这一件对象的发生项版本。
func (repository *ChargeOccurrences) ListCurrentCarrying(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.ChargeOccurrenceKey, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT occurrence.occurrence_ref, occurrence.validity_version
		   FROM transport_fulfillment.transport_charge_occurrence AS occurrence
		   JOIN transport_fulfillment.transport_charge_occurrence_member AS member
		     ON member.tenant_id = occurrence.tenant_id
		    AND member.occurrence_ref = occurrence.occurrence_ref
		    AND member.validity_version = occurrence.validity_version
		  WHERE occurrence.tenant_id = $1
		    AND member.object_ref = $2
		    AND occurrence.revision_kind IS DISTINCT FROM 'INVALIDATED'
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence AS later
		         WHERE later.tenant_id = occurrence.tenant_id
		           AND later.occurrence_ref = occurrence.occurrence_ref
		           AND later.corrects_version = occurrence.validity_version
		    )
		  ORDER BY occurrence.occurrence_ref, occurrence.validity_version`,
		tenant.String(), object.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
	}
	defer rows.Close()
	var listed []ports.ChargeOccurrenceKey
	for rows.Next() {
		var occurrence, validity string
		if err := rows.Scan(&occurrence, &validity); err != nil {
			return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
		}
		reference, err := domain.NewChargeOccurrenceReference(occurrence)
		if err != nil {
			return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
		}
		version, err := domain.NewOccurrenceValidityVersion(validity)
		if err != nil {
			return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
		}
		listed = append(listed, ports.ChargeOccurrenceKey{
			TenantID: tenant, Occurrence: reference, Validity: version,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list charge occurrences carrying: %w", err)
	}
	return listed, nil
}
