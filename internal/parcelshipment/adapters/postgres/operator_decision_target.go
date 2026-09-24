package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var _ ports.OperatorDecisionTargets = (*ShipmentRequestViews)(nil)

// FindOperatorDecisionTarget 按（租户、委托标识）读来源身份四要素与当前提交版本，整租户可见、不按客户账户切
// （理由在 ports.OperatorDecisionTargets）。读回的值照样经领域构造门。
func (views *ShipmentRequestViews) FindOperatorDecisionTarget(
	ctx context.Context,
	tenant domain.TenantID,
	requestID domain.ShipmentRequestID,
) (ports.OperatorDecisionTarget, bool, error) {
	querier, err := views.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	var account, source, requestKey, version string
	err = querier.QueryRow(ctx,
		`SELECT customer_account_id, source, source_request_key, current_submission_version_id
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2`,
		tenant.String(), requestID.String(),
	).Scan(&account, &source, &requestKey, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OperatorDecisionTarget{}, false, nil
	}
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}

	customerAccount, err := domain.NewCustomerAccountID(account)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	sourceValue, err := domain.NewSource(source)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	key, err := domain.NewSourceRequestKey(requestKey)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	identity, err := domain.NewSourceIdentity(tenant, customerAccount, sourceValue, key)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	current, err := domain.NewSubmissionVersionID(version)
	if err != nil {
		return ports.OperatorDecisionTarget{}, false, fmt.Errorf("find operator decision target: %w", err)
	}
	return ports.OperatorDecisionTarget{Identity: identity, CurrentVersion: current}, true, nil
}
