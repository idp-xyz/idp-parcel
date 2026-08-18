package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var _ ports.CurrentAcceptedParcelTargetView = (*ShipmentRequests)(nil)

// FindCurrentAcceptedByParcel 按租户+声明包裹反查当前已接受委托。
//
// 只读投影列，不打开 snapshot：重建门仍只开到已提交，已接受行不能经仓储读回。
// 多于一行是歧义，具名上抛，不按 saved_at 或行序挑一份（ADR-0060）。
func (repository *ShipmentRequests) FindCurrentAcceptedByParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.CurrentAcceptedParcelTarget, bool, error) {
	none := domain.CurrentAcceptedParcelTarget{}
	if tenant.String() == "" || parcel.String() == "" {
		return none, false, fmt.Errorf("find current accepted parcel target: tenant and parcel identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("find current accepted parcel target: %w", err)
	}

	// LIMIT 2 只为分辨「恰一」与「多于一」，ORDER BY 故意没有——有序会引诱人把第一行
	// 当最新接受者。
	rows, err := querier.Query(ctx,
		`SELECT tenant_id, customer_account_id, source, source_request_key,
		        shipment_request_id, current_submission_version_id
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND state = $2
		    AND declared_parcel_ids @> ARRAY[$3]::text[]
		  LIMIT 2`,
		tenant.String(),
		uint8(domain.ShipmentRequestAccepted),
		parcel.String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("find current accepted parcel target: %w", err)
	}
	defer rows.Close()

	type hit struct {
		tenant, customer, source, key, requestID, versionID string
	}
	var hits []hit
	for rows.Next() {
		var row hit
		if err := rows.Scan(
			&row.tenant, &row.customer, &row.source, &row.key,
			&row.requestID, &row.versionID,
		); err != nil {
			return none, false, fmt.Errorf("find current accepted parcel target: %w", err)
		}
		hits = append(hits, row)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("find current accepted parcel target: %w", err)
	}
	if len(hits) == 0 {
		return none, false, nil
	}
	if len(hits) > 1 {
		return none, false, domain.ErrAmbiguousParcelTarget
	}

	target, err := currentAcceptedTargetFrom(hits[0].tenant, hits[0].customer, hits[0].source, hits[0].key, hits[0].requestID, hits[0].versionID)
	if err != nil {
		return none, false, fmt.Errorf("find current accepted parcel target: %w", err)
	}
	return target, true, nil
}

func currentAcceptedTargetFrom(
	tenantID, customerID, source, requestKey, requestID, versionID string,
) (domain.CurrentAcceptedParcelTarget, error) {
	tenant, err := domain.NewTenantID(tenantID)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	customer, err := domain.NewCustomerAccountID(customerID)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	channel, err := domain.NewSource(source)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	key, err := domain.NewSourceRequestKey(requestKey)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	identity, err := domain.NewSourceIdentity(tenant, customer, channel, key)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	shipmentRequestID, err := domain.NewShipmentRequestID(requestID)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	version, err := domain.NewSubmissionVersionID(versionID)
	if err != nil {
		return domain.CurrentAcceptedParcelTarget{}, err
	}
	return domain.NewCurrentAcceptedParcelTarget(identity, shipmentRequestID, version)
}
