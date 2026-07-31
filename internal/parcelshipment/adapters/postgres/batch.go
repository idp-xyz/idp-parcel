package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	bentopostgres "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmissionBatchRepository owns batch grouping and per request result
// references only.
type SubmissionBatchRepository struct {
	db *bentopostgres.DB
}

// NewSubmissionBatchRepository builds the adapter.
func NewSubmissionBatchRepository(db *bentopostgres.DB) (*SubmissionBatchRepository, error) {
	if db == nil {
		return nil, errors.New("parcelshipment/postgres: batch repository requires a database")
	}
	return &SubmissionBatchRepository{db: db}, nil
}

// EnsureBatch writes the batch grouping row. It is idempotent so a retried
// intake call does not fail on the grouping alone.
func (r *SubmissionBatchRepository) EnsureBatch(
	ctx context.Context,
	scope domain.Scope,
	batchID domain.SubmissionBatchID,
	sourceKey domain.SourceKey,
	recordedAt time.Time,
) error {
	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: ensure batch: %w", err)
	}

	_, err = executor.Exec(ctx, `
		INSERT INTO parcel_shipment.submission_batch
			(tenant_id, customer_account_id, submission_batch_id,
			 source, source_request_key, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, customer_account_id, submission_batch_id) DO NOTHING`,
		scope.TenantID, scope.CustomerAccountID, batchID,
		sourceKey.Source, sourceKey.SourceRequestKey, recordedAt)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: ensure batch: %w", err)
	}
	return nil
}

// Record stores the per request results of one batch.
func (r *SubmissionBatchRepository) Record(ctx context.Context, batch domain.BatchResult) error {
	if err := batch.Validate(); err != nil {
		return err
	}
	if err := r.EnsureBatch(ctx, batch.Scope, batch.BatchID, batch.SourceKey, batch.RecordedAt); err != nil {
		return err
	}

	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: record batch: %w", err)
	}

	for _, request := range batch.Requests {
		_, err := executor.Exec(ctx, `
			INSERT INTO parcel_shipment.submission_batch_request
				(tenant_id, customer_account_id, submission_batch_id,
				 shipment_request_id, outcome, reason)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tenant_id, customer_account_id, submission_batch_id, shipment_request_id)
			DO NOTHING`,
			batch.Scope.TenantID, batch.Scope.CustomerAccountID, batch.BatchID,
			request.ShipmentRequestID, string(request.Outcome), nullable(request.Reason))
		if err != nil {
			return fmt.Errorf("parcelshipment/postgres: record batch request: %w", err)
		}
	}
	return nil
}

// Find reads one batch and its per request results.
func (r *SubmissionBatchRepository) Find(
	ctx context.Context,
	scope domain.Scope,
	batchID domain.SubmissionBatchID,
) (domain.BatchResult, error) {
	querier, err := r.db.ReadExecutor(ctx)
	if err != nil {
		return domain.BatchResult{}, fmt.Errorf("parcelshipment/postgres: read batch: %w", err)
	}

	var (
		source           string
		sourceRequestKey string
		recordedAt       time.Time
	)
	err = querier.QueryRow(ctx, `
		SELECT source, source_request_key, recorded_at
		FROM parcel_shipment.submission_batch
		WHERE tenant_id = $1 AND customer_account_id = $2 AND submission_batch_id = $3`,
		scope.TenantID, scope.CustomerAccountID, batchID,
	).Scan(&source, &sourceRequestKey, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BatchResult{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.BatchResult{}, fmt.Errorf("parcelshipment/postgres: read batch: %w", err)
	}

	rows, err := querier.Query(ctx, `
		SELECT shipment_request_id, outcome, reason
		FROM parcel_shipment.submission_batch_request
		WHERE tenant_id = $1 AND customer_account_id = $2 AND submission_batch_id = $3
		ORDER BY shipment_request_id`,
		scope.TenantID, scope.CustomerAccountID, batchID)
	if err != nil {
		return domain.BatchResult{}, fmt.Errorf("parcelshipment/postgres: read batch requests: %w", err)
	}
	defer rows.Close()

	result := domain.BatchResult{
		Scope:   scope,
		BatchID: batchID,
		SourceKey: domain.SourceKey{
			Scope:            scope,
			Source:           domain.Source(source),
			SourceRequestKey: domain.SourceRequestKey(sourceRequestKey),
		},
		RecordedAt: recordedAt,
	}
	for rows.Next() {
		var (
			requestID string
			outcome   string
			reason    *string
		)
		if err := rows.Scan(&requestID, &outcome, &reason); err != nil {
			return domain.BatchResult{}, fmt.Errorf("parcelshipment/postgres: scan batch request: %w", err)
		}
		result.Requests = append(result.Requests, domain.BatchRequestRef{
			ShipmentRequestID: domain.ShipmentRequestID(requestID),
			Outcome:           domain.RequestOutcome(outcome),
			Reason:            deref(reason),
		})
	}
	if err := rows.Err(); err != nil {
		return domain.BatchResult{}, fmt.Errorf("parcelshipment/postgres: read batch requests: %w", err)
	}
	return result, nil
}
