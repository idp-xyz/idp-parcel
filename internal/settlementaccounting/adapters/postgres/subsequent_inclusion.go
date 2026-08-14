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

// SubsequentInclusions 实现 ports.SubsequentInclusionStore（写入代数同 ADR-0031）。
// 同一纳入标识只登一次。
type SubsequentInclusions struct {
	db *bentopg.DB
}

func NewSubsequentInclusions(db *bentopg.DB) (*SubsequentInclusions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SubsequentInclusions{db: db}, nil
}

// FindByKey 按（租户+纳入）取回。否定结果只回 false。读回经重建门复验种类形状，
// 不重审对账单是否仍发布、费用是否已确认。
func (repository *SubsequentInclusions) FindByKey(
	ctx context.Context,
	key ports.InclusionKey,
) (ports.InclusionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.InclusionRecord{}, false, fmt.Errorf("find subsequent inclusion: %w", err)
	}

	var kindName, statement, original, subsequent, charge, digest string
	var adjustment *string
	var includedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT kind, statement_number, original_period, subsequent_period, charge_id,
		        adjustment_id, included_at, content_digest, recorded_at
		   FROM settlement_accounting.subsequent_inclusion
		  WHERE tenant_id = $1
		    AND inclusion_id = $2`,
		key.TenantID.String(),
		key.Inclusion.String(),
	).Scan(&kindName, &statement, &original, &subsequent, &charge,
		&adjustment, &includedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.InclusionRecord{}, false, nil
	}
	if err != nil {
		return ports.InclusionRecord{}, false, fmt.Errorf("find subsequent inclusion: %w", err)
	}

	inclusion, err := rebuildSubsequentInclusion(key.Inclusion, kindName, statement, original, subsequent, charge, adjustment, includedAt)
	if err != nil {
		return ports.InclusionRecord{}, false, fmt.Errorf("find subsequent inclusion: %w", err)
	}
	return ports.InclusionRecord{
		Key:           key,
		ContentDigest: digest,
		Inclusion:     inclusion,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次纳入。同标识已有记录时答`已登记`，不覆盖先到者。
func (repository *SubsequentInclusions) Save(
	ctx context.Context,
	record ports.InclusionRecord,
) (ports.InclusionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.InclusionSaveOutcomeInvalid, fmt.Errorf("save subsequent inclusion: %w", err)
	}

	adjustment, hasAdjustment := record.Inclusion.Adjustment()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.subsequent_inclusion
			(tenant_id, inclusion_id, kind, statement_number, original_period,
			 subsequent_period, charge_id, adjustment_id, included_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Inclusion.String(),
		record.Inclusion.Kind().String(),
		record.Inclusion.Statement().String(),
		record.Inclusion.OriginalPeriod().String(),
		record.Inclusion.SubsequentPeriod().String(),
		record.Inclusion.Charge().String(),
		optionalRef(adjustment, hasAdjustment),
		record.Inclusion.IncludedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.InclusionSaveOutcomeInvalid, fmt.Errorf("save subsequent inclusion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.InclusionAlreadyRecorded, nil
	}
	return ports.InclusionSaved, nil
}

func rebuildSubsequentInclusion(
	id domain.InclusionReference,
	kindName, statement, original, subsequent, charge string,
	adjustment *string,
	includedAt time.Time,
) (domain.SubsequentInclusion, error) {
	kind, err := inclusionKindFrom(kindName)
	if err != nil {
		return domain.SubsequentInclusion{}, err
	}
	statementNumber, err := domain.NewStatementNumber(statement)
	if err != nil {
		return domain.SubsequentInclusion{}, err
	}
	originalPeriod, err := domain.NewBillingPeriodReference(original)
	if err != nil {
		return domain.SubsequentInclusion{}, err
	}
	subsequentPeriod, err := domain.NewBillingPeriodReference(subsequent)
	if err != nil {
		return domain.SubsequentInclusion{}, err
	}
	chargeID, err := domain.NewCustomerChargeID(charge)
	if err != nil {
		return domain.SubsequentInclusion{}, err
	}
	spec := domain.RehydrateSubsequentInclusionSpec{
		Inclusion:        id,
		Kind:             kind,
		Statement:        statementNumber,
		OriginalPeriod:   originalPeriod,
		SubsequentPeriod: subsequentPeriod,
		Charge:           chargeID,
		IncludedAt:       includedAt,
	}
	if adjustment != nil {
		spec.Adjustment, err = domain.NewChargeAdjustmentID(*adjustment)
		if err != nil {
			return domain.SubsequentInclusion{}, err
		}
	}
	return domain.RehydrateSubsequentInclusion(spec)
}

func inclusionKindFrom(raw string) (domain.InclusionKind, error) {
	switch raw {
	case domain.IncludedAdjustment.String():
		return domain.IncludedAdjustment, nil
	case domain.IncludedLateCharge.String():
		return domain.IncludedLateCharge, nil
	default:
		return 0, fmt.Errorf("unknown inclusion kind %q", raw)
	}
}
