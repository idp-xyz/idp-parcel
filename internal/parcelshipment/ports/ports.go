// Package ports declares the semantic boundaries parcel-shipment owns. These
// are interfaces only: their PostgreSQL adapters stay blocked behind the Bento
// persistence gate (ADR-0017), so the sole implementations today are the
// deterministic doubles used by tests.
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// SourceSubmissionRepository preserves immutable source facts under an explicit
// source scope. Lookups are keyed by the full SourceIdentity, so a negative
// answer never distinguishes absence from an object owned by another tenant or
// customer account.
type SourceSubmissionRepository interface {
	FindPreserved(ctx context.Context, identity domain.SourceIdentity) (domain.SourceSubmissionFingerprint, bool, error)
	Preserve(ctx context.Context, submission domain.SourceSubmissionFingerprint) error
	// AppendObservation records that the same logical request was seen again.
	// It appends beside the preserved fact and never replaces it, which is what
	// keeps differing occurredAt/receivedAt from rewriting history.
	AppendObservation(ctx context.Context, observed domain.SourceSubmissionFingerprint) error
}

// ShipmentRequestRepository stores request aggregates against the source
// identity that produced them, which is what lets a replay return the original
// request instead of building a second one.
type ShipmentRequestRepository interface {
	FindBySourceIdentity(ctx context.Context, identity domain.SourceIdentity) (domain.ShipmentRequest, bool, error)
	Insert(ctx context.Context, identity domain.SourceIdentity, request domain.ShipmentRequest) error
}

// ProductionOwnershipAuthority is the pilot admission control that answers who
// currently owns a whole admission scope. parcel-shipment consumes the decision
// and never derives one of its own.
type ProductionOwnershipAuthority interface {
	DecideProductionOwnership(ctx context.Context, scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error)
}

// SubmissionIdentityFactory issues the internal identities parcel-shipment owns.
// They are deliberately not accepted from the caller: a customer reference must
// not become an internal identity.
type SubmissionIdentityFactory interface {
	NextSubmissionVersionID(ctx context.Context) (domain.SubmissionVersionID, error)
	NextAcceptanceDecisionTaskID(ctx context.Context) (domain.AcceptanceDecisionTaskID, error)
}

type Clock interface {
	Now() time.Time
}
