package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// PreAcceptanceConversions 实现 ports.PreAcceptanceConversionBook：读受理前估价留痕与 SELL 评价请求册，写转出册。
type PreAcceptanceConversions struct {
	db *bentopg.DB
}

func NewPreAcceptanceConversions(db *bentopg.DB) (*PreAcceptanceConversions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &PreAcceptanceConversions{db: db}, nil
}

var _ ports.PreAcceptanceConversionBook = (*PreAcceptanceConversions)(nil)

// ParcelOfCharge 经 SELL 评价请求册上的费用标识回找包裹：只有形成了费用的请求带费用标识（表上 CHECK）。
func (book *PreAcceptanceConversions) ParcelOfCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) (string, bool, error) {
	querier, err := book.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, fmt.Errorf("parcel of charge: %w", err)
	}
	var parcel string
	err = querier.QueryRow(ctx,
		`SELECT parcel_id
		   FROM settlement_accounting.sell_evaluation_request
		  WHERE tenant_id = $1
		    AND customer_charge_id = $2`,
		tenant.String(),
		charge.String(),
	).Scan(&parcel)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("parcel of charge: %w", err)
	}
	return parcel, true, nil
}

// ParcelShares 把留痕里逐件试算平铺成（控制请求，该件合计）。一份留痕里同一件包裹出现多次只会是申报重复，合计相加——
// 一次控制占用的金额本就是各行合计之和，份额之和因此仍等于原金额。
func (book *PreAcceptanceConversions) ParcelShares(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	parcel string,
) ([]ports.ParcelShare, error) {
	querier, err := book.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcel shares: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT trace.control_request_id, SUM((trial->>'totalMinor')::bigint)::bigint
		   FROM settlement_accounting.pre_acceptance_estimate_trace AS trace
		  CROSS JOIN LATERAL jsonb_array_elements(trace.trials) AS trial
		  WHERE trace.tenant_id = $1
		    AND trace.legal_entity = $2
		    AND trace.account_id = $3
		    AND trace.currency = $4
		    AND trial->>'parcel' = $5
		  GROUP BY trace.control_request_id
		  ORDER BY trace.control_request_id`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		parcel,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel shares: %w", err)
	}
	defer rows.Close()

	var shares []ports.ParcelShare
	for rows.Next() {
		var requestID string
		var share int64
		if err := rows.Scan(&requestID, &share); err != nil {
			return nil, fmt.Errorf("parcel shares: %w", err)
		}
		id, err := domain.NewControlRequestID(requestID)
		if err != nil {
			return nil, fmt.Errorf("parcel shares: %w", err)
		}
		shares = append(shares, ports.ParcelShare{RequestID: id, ShareMinor: share})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("parcel shares: %w", err)
	}
	return shares, nil
}

// RecordConversion 插一行转出；键撞上即已转过，不覆盖先到者的金额与触发费用。
func (book *PreAcceptanceConversions) RecordConversion(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	requestID domain.ControlRequestID,
	parcel string,
	kind ports.OccupationKind,
	minor int64,
	charge domain.CustomerChargeID,
	convertedAt time.Time,
) (bool, error) {
	if kind.String() == "" {
		return false, fmt.Errorf("record conversion: occupation kind is required")
	}
	executor, err := book.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("record conversion: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.pre_acceptance_control_conversion
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 parcel_id, occupation_kind, converted_minor, charge_id, converted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT ON CONSTRAINT pre_acceptance_control_conversion_pkey DO NOTHING`,
		tenant.String(),
		scope.LegalEntity().String(),
		scope.Account().String(),
		scope.Currency().String(),
		requestID.String(),
		parcel,
		kind.String(),
		minor,
		charge.String(),
		convertedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("record conversion: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
