package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.ChargeOccurrencesCarryingObject = (*ChargeOccurrences)(nil)

// ListCurrentCarrying 列出仍有效、成员里有这一件对象的发生项版本。「仍有效」的定义在 currentChargeOccurrenceHead，
// 发生项在本查询里因此取它要求的别名 head。
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
		`SELECT head.occurrence_ref, head.validity_version
		   FROM transport_fulfillment.transport_charge_occurrence AS head
		   JOIN transport_fulfillment.transport_charge_occurrence_member AS member
		     ON member.tenant_id = head.tenant_id
		    AND member.occurrence_ref = head.occurrence_ref
		    AND member.validity_version = head.validity_version
		  WHERE head.tenant_id = $1
		    AND member.object_ref = $2
		    AND `+currentChargeOccurrenceHead(3)+`
		  ORDER BY head.occurrence_ref, head.validity_version`,
		tenant.String(), object.String(), domain.OccurrenceInvalidated.String(),
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
