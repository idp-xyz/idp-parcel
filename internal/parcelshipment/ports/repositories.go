// Package ports declares the parcel shipment owned interfaces. They speak
// parcel shipment language; none of them exposes arbitrary table CRUD, an
// implicit tenant or a cross aggregate save.
package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Errors every adapter maps its driver failures onto. ErrNotFound never reveals
// whether an object exists in another scope.
var (
	ErrNotFound       = errors.New("parcelshipment: not found")
	ErrAlreadyExists  = errors.New("parcelshipment: already exists")
	ErrRevisionStale  = errors.New("parcelshipment: revision conflict")
	ErrIntakeConflict = errors.New("parcelshipment: same source key with different payload digest")
)

// SourceSubmissionRepository stores and reads the immutable source records. All
// SQL carries the tenant and the customer account explicitly.
type SourceSubmissionRepository interface {
	// Preserve writes the immutable record. It returns ErrAlreadyExists when
	// the source key is already preserved so the caller can read the original
	// rather than overwrite it.
	Preserve(ctx context.Context, submission *domain.SourceSubmission) error

	// Find reads the preserved record for one source key.
	Find(ctx context.Context, key domain.SourceKey) (*domain.SourceSubmission, error)

	// RecordOutcome stores the intake processing result of an already
	// preserved record. It never rewrites the raw content or the digest.
	RecordOutcome(ctx context.Context, submission *domain.SourceSubmission) error
}

// ShipmentRequestRepository loads, inserts and versioned-updates the shipment
// request aggregate under its strongly typed composite key.
type ShipmentRequestRepository interface {
	Load(ctx context.Context, key domain.ShipmentRequestKey) (*domain.ShipmentRequest, int64, error)
	Insert(ctx context.Context, request *domain.ShipmentRequest) error
	Update(ctx context.Context, key domain.ShipmentRequestKey, request *domain.ShipmentRequest, expected int64) error
}

// SubmissionBatchRepository owns batch grouping and the per request result
// references only. It never takes over service responsibility for a request.
type SubmissionBatchRepository interface {
	// EnsureBatch creates the grouping row. It runs before the first shipment
	// request so a request can reference its batch, and it is idempotent so a
	// retried intake call does not fail on the grouping alone.
	EnsureBatch(
		ctx context.Context,
		scope domain.Scope,
		batchID domain.SubmissionBatchID,
		sourceKey domain.SourceKey,
		recordedAt time.Time,
	) error

	Record(ctx context.Context, batch domain.BatchResult) error
	Find(ctx context.Context, scope domain.Scope, batchID domain.SubmissionBatchID) (domain.BatchResult, error)
}
