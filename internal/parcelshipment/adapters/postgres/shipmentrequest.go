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

func zeroTime() time.Time { return time.Time{} }

// ShipmentRequestRepository persists the shipment request aggregate under its
// strongly typed composite key.
type ShipmentRequestRepository struct {
	db *bentopostgres.DB
}

// NewShipmentRequestRepository builds the adapter.
func NewShipmentRequestRepository(db *bentopostgres.DB) (*ShipmentRequestRepository, error) {
	if db == nil {
		return nil, errors.New("parcelshipment/postgres: shipment request repository requires a database")
	}
	return &ShipmentRequestRepository{db: db}, nil
}

// Load reads the aggregate and its current revision.
func (r *ShipmentRequestRepository) Load(
	ctx context.Context,
	key domain.ShipmentRequestKey,
) (*domain.ShipmentRequest, int64, error) {
	if err := key.Validate(); err != nil {
		return nil, 0, err
	}
	querier, err := r.db.ReadExecutor(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("parcelshipment/postgres: load shipment request: %w", err)
	}

	var (
		batchID          string
		source           string
		sourceRequestKey string
		state            string
		version          int32
		eventID          string
		sourceOccurredAt time.Time
		submittedAt      time.Time
		revision         int64
	)

	err = querier.QueryRow(ctx, `
		SELECT submission_batch_id, source, source_request_key, lifecycle_state,
		       submission_version, event_id, source_occurred_at, submitted_at, revision
		FROM parcel_shipment.shipment_request
		WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.ShipmentRequestID,
	).Scan(&batchID, &source, &sourceRequestKey, &state,
		&version, &eventID, &sourceOccurredAt, &submittedAt, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, ports.ErrNotFound
	}
	if err != nil {
		return nil, 0, fmt.Errorf("parcelshipment/postgres: load shipment request: %w", err)
	}

	parcels, err := r.declaredParcels(ctx, key)
	if err != nil {
		return nil, 0, err
	}

	request := domain.RestoreShipmentRequest(
		key,
		domain.SubmissionBatchID(batchID),
		domain.SourceKey{
			Scope:            key.Scope,
			Source:           domain.Source(source),
			SourceRequestKey: domain.SourceRequestKey(sourceRequestKey),
		},
		domain.LifecycleState(state),
		domain.SubmissionVersion(version),
		parcels,
		eventID,
		sourceOccurredAt,
		submittedAt,
		revision,
	)
	return request, revision, nil
}

func (r *ShipmentRequestRepository) declaredParcels(
	ctx context.Context,
	key domain.ShipmentRequestKey,
) ([]domain.DeclaredParcel, error) {
	querier, err := r.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read declared parcels: %w", err)
	}

	rows, err := querier.Query(ctx, `
		SELECT declared_parcel_id, customer_reference
		FROM parcel_shipment.declared_parcel
		WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3
		ORDER BY declared_position`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.ShipmentRequestID)
	if err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read declared parcels: %w", err)
	}
	defer rows.Close()

	var parcels []domain.DeclaredParcel
	for rows.Next() {
		var (
			id        string
			reference *string
		)
		if err := rows.Scan(&id, &reference); err != nil {
			return nil, fmt.Errorf("parcelshipment/postgres: scan declared parcel: %w", err)
		}
		parcels = append(parcels, domain.DeclaredParcel{
			ID:                domain.DeclaredParcelID(id),
			CustomerReference: deref(reference),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("parcelshipment/postgres: read declared parcels: %w", err)
	}
	return parcels, nil
}

// Insert writes the aggregate and its declared parcels inside the caller's
// transaction.
func (r *ShipmentRequestRepository) Insert(ctx context.Context, request *domain.ShipmentRequest) error {
	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: insert shipment request: %w", err)
	}

	key := request.Key()
	sourceKey := request.SourceKey()

	_, err = executor.Exec(ctx, `
		INSERT INTO parcel_shipment.shipment_request
			(tenant_id, customer_account_id, shipment_request_id, submission_batch_id,
			 source, source_request_key, lifecycle_state, submission_version, event_id,
			 source_occurred_at, submitted_at, revision)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.ShipmentRequestID,
		request.SubmissionBatchID(), sourceKey.Source, sourceKey.SourceRequestKey,
		string(request.State()), int32(request.SubmissionVersion()), request.EventID(),
		request.SourceOccurredAt(), request.SubmittedAt(), request.Revision())
	if isUniqueViolation(err) {
		return ports.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: insert shipment request: %w", err)
	}

	for position, parcel := range request.DeclaredParcels() {
		_, err := executor.Exec(ctx, `
			INSERT INTO parcel_shipment.declared_parcel
				(tenant_id, customer_account_id, shipment_request_id,
				 declared_parcel_id, customer_reference, declared_position)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			key.Scope.TenantID, key.Scope.CustomerAccountID, key.ShipmentRequestID,
			parcel.ID, nullable(parcel.CustomerReference), position+1)
		if isUniqueViolation(err) {
			return ports.ErrAlreadyExists
		}
		if err != nil {
			return fmt.Errorf("parcelshipment/postgres: insert declared parcel: %w", err)
		}
	}
	return nil
}

// Update applies a conditional write against the expected revision. A revision
// mismatch is reported rather than overwritten.
func (r *ShipmentRequestRepository) Update(
	ctx context.Context,
	key domain.ShipmentRequestKey,
	request *domain.ShipmentRequest,
	expected int64,
) error {
	executor, err := r.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: update shipment request: %w", err)
	}

	tag, err := executor.Exec(ctx, `
		UPDATE parcel_shipment.shipment_request
		SET lifecycle_state = $4, submission_version = $5, revision = revision + 1
		WHERE tenant_id = $1 AND customer_account_id = $2 AND shipment_request_id = $3
		  AND revision = $6`,
		key.Scope.TenantID, key.Scope.CustomerAccountID, key.ShipmentRequestID,
		string(request.State()), int32(request.SubmissionVersion()), expected)
	if err != nil {
		return fmt.Errorf("parcelshipment/postgres: update shipment request: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ErrRevisionStale
	}
	return nil
}
