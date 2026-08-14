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

// RecoveryAcknowledgements 实现 ports.RecoveryAcknowledgementStore（写入代数同
// ADR-0031）。同一认可标识只登记一次。
type RecoveryAcknowledgements struct {
	db *bentopg.DB
}

func NewRecoveryAcknowledgements(db *bentopg.DB) (*RecoveryAcknowledgements, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &RecoveryAcknowledgements{db: db}, nil
}

// FindByKey 按（租户+认可）取回。否定结果只回 false。读回经重建门复验立场与金额
// 矩阵，不重审应追偿是否仍在场。
func (repository *RecoveryAcknowledgements) FindByKey(
	ctx context.Context,
	key ports.AcknowledgementKey,
) (ports.AcknowledgementRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AcknowledgementRecord{}, false, fmt.Errorf("find recovery acknowledgement: %w", err)
	}

	var receivableID, response, standingName, currency, digest string
	var acknowledged, receivableMinor int64
	var acknowledgedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT receivable_id, response_ref, standing, currency, acknowledged_minor,
		        receivable_minor, acknowledged_at, content_digest, recorded_at
		   FROM settlement_accounting.recovery_acknowledgement
		  WHERE tenant_id = $1
		    AND acknowledgement_id = $2`,
		key.TenantID.String(),
		key.Acknowledgement.String(),
	).Scan(&receivableID, &response, &standingName, &currency, &acknowledged,
		&receivableMinor, &acknowledgedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AcknowledgementRecord{}, false, nil
	}
	if err != nil {
		return ports.AcknowledgementRecord{}, false, fmt.Errorf("find recovery acknowledgement: %w", err)
	}

	acknowledgement, err := rebuildRecoveryAcknowledgement(
		key.Acknowledgement, receivableID, response, standingName, currency,
		acknowledged, receivableMinor, acknowledgedAt)
	if err != nil {
		return ports.AcknowledgementRecord{}, false, fmt.Errorf("find recovery acknowledgement: %w", err)
	}
	return ports.AcknowledgementRecord{
		Key:             key,
		ContentDigest:   digest,
		Acknowledgement: acknowledgement,
		RecordedAt:      recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次认可。同标识已有记录时答`已登记`，不覆盖先到者。
func (repository *RecoveryAcknowledgements) Save(
	ctx context.Context,
	record ports.AcknowledgementRecord,
) (ports.AcknowledgementSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AcknowledgementSaveOutcomeInvalid, fmt.Errorf("save recovery acknowledgement: %w", err)
	}

	currency, acknowledged := record.Acknowledgement.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_acknowledgement
			(tenant_id, acknowledgement_id, receivable_id, response_ref, standing,
			 currency, acknowledged_minor, receivable_minor, acknowledged_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Acknowledgement.String(),
		record.Acknowledgement.Receivable().String(),
		record.Acknowledgement.Response().String(),
		record.Acknowledgement.Standing().String(),
		currency.String(),
		acknowledged,
		acknowledged+record.Acknowledgement.UnacknowledgedMinor(),
		record.Acknowledgement.AcknowledgedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AcknowledgementSaveOutcomeInvalid, fmt.Errorf("save recovery acknowledgement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AcknowledgementAlreadyRecorded, nil
	}
	return ports.AcknowledgementSaved, nil
}

func rebuildRecoveryAcknowledgement(
	id domain.AcknowledgementID,
	receivableID, response, standingName, currency string,
	acknowledged, receivableMinor int64,
	acknowledgedAt time.Time,
) (domain.RecoveryAcknowledgement, error) {
	receivable, err := domain.NewRecoveryReceivableID(receivableID)
	if err != nil {
		return domain.RecoveryAcknowledgement{}, err
	}
	responseRef, err := domain.NewCounterpartyResponseReference(response)
	if err != nil {
		return domain.RecoveryAcknowledgement{}, err
	}
	standing, err := responseStandingFrom(standingName)
	if err != nil {
		return domain.RecoveryAcknowledgement{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.RecoveryAcknowledgement{}, err
	}
	return domain.RehydrateRecoveryAcknowledgement(domain.RehydrateRecoveryAcknowledgementSpec{
		ID:                id,
		Receivable:        receivable,
		Response:          responseRef,
		Standing:          standing,
		Currency:          currencyCode,
		AcknowledgedMinor: acknowledged,
		ReceivableMinor:   receivableMinor,
		AcknowledgedAt:    acknowledgedAt,
	})
}

func responseStandingFrom(raw string) (domain.ResponseStanding, error) {
	switch raw {
	case domain.ResponseAccepted.String():
		return domain.ResponseAccepted, nil
	case domain.ResponsePartiallyAccepted.String():
		return domain.ResponsePartiallyAccepted, nil
	default:
		return 0, fmt.Errorf("unknown response standing %q", raw)
	}
}
