// Package postgres holds the parcel shipment persistence adapter. Row models,
// SQLSTATE translation and scanning stay here; they never reach the domain.
// Every write takes its executor from the framework transaction: a repository
// never falls back to the connection pool.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	bentopostgres "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// uniqueViolation is the SQLSTATE the adapter translates into
// ports.ErrAlreadyExists.
const uniqueViolation = "23505"

// SourceSubmissionRepository preserves and reads immutable source records.
type SourceSubmissionRepository struct {
	db *bentopostgres.DB
}

// NewSourceSubmissionRepository builds the adapter.
func NewSourceSubmissionRepository(db *bentopostgres.DB) (*SourceSubmissionRepository, error) {
	if db == nil {
		return nil, errors.New("parcelshipment/postgres: source repository requires a database")
	}
	return &SourceSubmissionRepository{db: db}, nil
}

// Preserve writes the immutable record. A repeated source key surfaces as
// ports.ErrAlreadyExists so the caller reads the original instead of
// overwriting it.
func (r *SourceSubmissionRepository) Preserve(ctx context.Context, submission *domain.SourceSubmission) error {
	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: preserve source: %w", err)
	}

	key := submission.Key()
	_, err = executor.Exec(ctx, `
		INSERT INTO parcel_shipment.source_submission
			(tenant_id, customer_account_id, source, source_request_key,
			 raw_content_ref, payload_digest, source_occurred_at, system_received_at, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.Source, key.SourceRequestKey,
		submission.RawContentRef(), submission.PayloadDigest(),
		submission.OccurredAt(), submission.ReceivedAt(), nullable(submission.CorrelationID()))
	if isUniqueViolation(err) {
		return ports.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: preserve source: %w", err)
	}
	return nil
}

// Find reads the preserved record. The scope is always part of the predicate,
// so a not found result never reveals another scope.
func (r *SourceSubmissionRepository) Find(
	ctx context.Context,
	key domain.SourceKey,
) (*domain.SourceSubmission, error) {
	querier, err := r.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read source: %w", err)
	}

	var (
		rawContentRef    string
		digest           string
		occurredAt       = zeroTime()
		receivedAt       = zeroTime()
		correlationID    *string
		outcome          *string
		notAdmitReason   *string
		parseRuleVersion *string
		batchID          *string
	)

	err = querier.QueryRow(ctx, `
		SELECT raw_content_ref, payload_digest, source_occurred_at, system_received_at,
		       correlation_id, intake_outcome, not_admitted_reason, parse_rule_version,
		       submission_batch_id
		FROM parcel_shipment.source_submission
		WHERE tenant_id = $1 AND customer_account_id = $2
		  AND source = $3 AND source_request_key = $4`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.Source, key.SourceRequestKey,
	).Scan(&rawContentRef, &digest, &occurredAt, &receivedAt,
		&correlationID, &outcome, &notAdmitReason, &parseRuleVersion, &batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ports.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read source: %w", err)
	}

	requestIDs, err := r.linkedRequestIDs(ctx, key)
	if err != nil {
		return nil, err
	}

	return domain.RestoreSource(
		key, rawContentRef, domain.PayloadDigest(digest), occurredAt, receivedAt,
		deref(correlationID), domain.IntakeOutcome(deref(outcome)),
		deref(notAdmitReason), deref(parseRuleVersion),
		domain.SubmissionBatchID(deref(batchID)), requestIDs,
	), nil
}

func (r *SourceSubmissionRepository) linkedRequestIDs(
	ctx context.Context,
	key domain.SourceKey,
) ([]domain.ShipmentRequestID, error) {
	querier, err := r.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read linked requests: %w", err)
	}

	rows, err := querier.Query(ctx, `
		SELECT shipment_request_id
		FROM parcel_shipment.shipment_request
		WHERE tenant_id = $1 AND customer_account_id = $2
		  AND source = $3 AND source_request_key = $4
		ORDER BY shipment_request_id`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.Source, key.SourceRequestKey)
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read linked requests: %w", err)
	}
	defer rows.Close()

	var ids []domain.ShipmentRequestID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("parcelshipment/postgres: scan linked request: %w", err)
		}
		ids = append(ids, domain.ShipmentRequestID(id))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read linked requests: %w", err)
	}
	return ids, nil
}

// RecordOutcome stores the intake processing result. The raw content reference
// and the digest are deliberately absent from the update: a preserved fact is
// never rewritten.
func (r *SourceSubmissionRepository) RecordOutcome(
	ctx context.Context,
	submission *domain.SourceSubmission,
) error {
	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: record intake outcome: %w", err)
	}

	key := submission.Key()
	tag, err := executor.Exec(ctx, `
		UPDATE parcel_shipment.source_submission
		SET intake_outcome = $5,
		    not_admitted_reason = $6,
		    parse_rule_version = $7,
		    submission_batch_id = $8
		WHERE tenant_id = $1 AND customer_account_id = $2
		  AND source = $3 AND source_request_key = $4`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.Source, key.SourceRequestKey,
		string(submission.Outcome()), nullable(submission.NotAdmittedReason()),
		nullable(submission.ParseRuleVersion()), nullable(string(submission.SubmissionBatchID())))
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: record intake outcome: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
