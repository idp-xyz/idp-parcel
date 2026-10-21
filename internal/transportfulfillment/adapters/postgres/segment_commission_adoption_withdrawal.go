package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	_ ports.SegmentCommissionAdoptionWithdrawalStore = (*SegmentCommissionAdoptions)(nil)
	_ ports.CommissionAdoptionsView                  = (*SegmentCommissionAdoptions)(nil)
)

// FindLatestWithdrawal 交这一段最近一次解除（迁移 TF 0048 的解除册），经重建门复验。
func (repository *SegmentCommissionAdoptions) FindLatestWithdrawal(
	ctx context.Context,
	key ports.SegmentCommissionAdoptionKey,
) (ports.SegmentCommissionAdoptionWithdrawalRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	var generation int
	var commissionID, withdrawnBy, beforeBasis, change string
	var afterBasis *string
	var beforeAt, recordedAt time.Time
	var afterAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT generation, commission_id, withdrawn_by, start_before_basis, start_before_at, start_change,
		        start_after_basis, start_after_at, recorded_at
		   FROM transport_fulfillment.segment_commission_adoption_withdrawal
		  WHERE tenant_id = $1
		    AND segment_ref = $2
		  ORDER BY generation DESC
		  LIMIT 1`,
		key.TenantID.String(), key.Segment.String(),
	).Scan(&generation, &commissionID, &withdrawnBy, &beforeBasis, &beforeAt, &change, &afterBasis, &afterAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, nil
	}
	if err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	spec := domain.RehydrateSegmentCommissionAdoptionWithdrawalSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		Generation:    generation,
		StartBeforeAt: beforeAt,
	}
	if spec.Commission, err = domain.NewTransportCommissionReference(commissionID); err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	if spec.WithdrawnBy, err = domain.NewRegistrantReference(withdrawnBy); err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	if spec.StartBeforeBasis, err = domain.NewParticipationBasisReference(beforeBasis); err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	if spec.StartChange, err = domain.ParseCommissionStartCorrection(change); err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	if afterBasis != nil {
		if spec.StartAfterBasis, err = domain.NewParticipationBasisReference(*afterBasis); err != nil {
			return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
		}
	}
	if afterAt != nil {
		spec.StartAfterAt = *afterAt
	}
	withdrawal, err := domain.RehydrateSegmentCommissionAdoptionWithdrawal(spec)
	if err != nil {
		return ports.SegmentCommissionAdoptionWithdrawalRecord{}, false, fmt.Errorf("find segment commission adoption withdrawal: %w", err)
	}
	return ports.SegmentCommissionAdoptionWithdrawalRecord{Key: key, Withdrawal: withdrawal, RecordedAt: recordedAt.UTC()}, true, nil
}

// SaveWithdrawal 写下一次解除。同段同代已有解除答`已登记`，不覆盖先到者；外键不满足（那一代采用不在册）是编排核过之后的违约，照实报错。
func (repository *SegmentCommissionAdoptions) SaveWithdrawal(
	ctx context.Context,
	record ports.SegmentCommissionAdoptionWithdrawalRecord,
) (ports.AdoptionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AdoptionSaveOutcomeInvalid, fmt.Errorf("save segment commission adoption withdrawal: %w", err)
	}
	withdrawal := record.Withdrawal
	if record.Key.TenantID != withdrawal.TenantID() || record.Key.Segment != withdrawal.Segment() {
		return ports.AdoptionSaveOutcomeInvalid, errors.New("save segment commission adoption withdrawal: record key disagrees with the withdrawal it carries")
	}
	beforeBasis, beforeAt := withdrawal.StartBefore()
	var afterBasis *string
	var afterAt *time.Time
	if basis, at, present := withdrawal.StartAfter(); present {
		value, moment := basis.String(), at.UTC()
		afterBasis, afterAt = &value, &moment
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.segment_commission_adoption_withdrawal
			(tenant_id, segment_ref, generation, commission_id, withdrawn_by,
			 start_before_basis, start_before_at, start_change, start_after_basis, start_after_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT ON CONSTRAINT segment_commission_adoption_withdrawal_pkey DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		withdrawal.Generation(),
		withdrawal.Commission().String(),
		withdrawal.WithdrawnBy().String(),
		beforeBasis.String(),
		beforeAt.UTC(),
		withdrawal.StartChange().String(),
		afterBasis,
		afterAt,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AdoptionSaveOutcomeInvalid, fmt.Errorf("save segment commission adoption withdrawal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AdoptionAlreadyRegistered, nil
	}
	return ports.AdoptionSaved, nil
}

// FindCurrentByCommission 列出这份委托此刻在各段上的当前采用，按段排。走迁移 0035 那条按委托的索引。
func (repository *SegmentCommissionAdoptions) FindCurrentByCommission(
	ctx context.Context,
	key ports.TransportCommissionKey,
) ([]ports.SegmentCommissionAdoptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find current adoptions of commission: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT adoption.segment_ref, adoption.generation, adoption.commission_id, adoption.provider_ref, adoption.start_basis,
		        adoption.started_at, adoption.content_digest, adoption.recorded_at
		   FROM transport_fulfillment.segment_commission_adoption AS adoption
		  WHERE adoption.tenant_id = $1
		    AND adoption.commission_id = $2
		    AND `+currentAdoptionCondition+`
		  ORDER BY adoption.segment_ref`,
		key.TenantID.String(), key.Commission.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find current adoptions of commission: %w", err)
	}
	defer rows.Close()
	var records []ports.SegmentCommissionAdoptionRecord
	for rows.Next() {
		record, err := scanAdoption(key.TenantID, rows)
		if err != nil {
			return nil, fmt.Errorf("find current adoptions of commission: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find current adoptions of commission: %w", err)
	}
	return records, nil
}
