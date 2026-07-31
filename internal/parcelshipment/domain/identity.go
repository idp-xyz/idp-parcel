// Package domain owns the parcel shipment business model: source submissions,
// shipment requests, declared parcels and their submission versions. It depends
// on the standard library and the framework domain event contract only.
package domain

import (
	"errors"
	"strings"
)

// ErrScopeIncomplete reports a value that cannot be attributed to a tenant and
// a customer account. Parcel refuses such a value rather than defaulting it.
var ErrScopeIncomplete = errors.New("parcelshipment: tenant and customer account are required")

// TenantID is the group tenant a record belongs to.
type TenantID string

// CustomerAccountID is the shipper customer account within a tenant.
type CustomerAccountID string

// Source names the intake channel a submission arrived through.
type Source string

// SourceRequestKey is the intake owned idempotency key. Each intake adapter
// binds its real composition; a customer reference is never used directly as a
// global key.
type SourceRequestKey string

// PayloadDigest is the digest of the preserved raw content.
type PayloadDigest string

// SubmissionBatchID groups the shipment requests of one intake call. A batch
// groups processing only; it never owns service responsibility.
type SubmissionBatchID string

// ShipmentRequestID is the internal identity of one shipment request.
type ShipmentRequestID string

// DeclaredParcelID is the internal identity of one declared parcel.
type DeclaredParcelID string

// Scope is the mandatory isolation scope of every parcel shipment record.
// A query, a key or an event that cannot state it is refused.
type Scope struct {
	TenantID          TenantID
	CustomerAccountID CustomerAccountID
}

// Validate rejects an incomplete scope.
func (s Scope) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" ||
		strings.TrimSpace(string(s.CustomerAccountID)) == "" {
		return ErrScopeIncomplete
	}
	return nil
}

// String renders the scope for an event envelope. It is a composite technical
// identifier, never a display value.
func (s Scope) String() string {
	return string(s.TenantID) + "/" + string(s.CustomerAccountID)
}

// SourceKey identifies one logical intake request. Persistence uniqueness
// covers at least the tenant, the customer account, the source and the source
// request key, so the same external key in another scope stays isolated.
type SourceKey struct {
	Scope            Scope
	Source           Source
	SourceRequestKey SourceRequestKey
}

// Validate rejects an incomplete source key.
func (k SourceKey) Validate() error {
	if err := k.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(k.Source)) == "" ||
		strings.TrimSpace(string(k.SourceRequestKey)) == "" {
		return ErrSourceKeyIncomplete
	}
	return nil
}

// ErrSourceKeyIncomplete reports an intake key that cannot establish idempotency.
var ErrSourceKeyIncomplete = errors.New("parcelshipment: source and source request key are required")

// ShipmentRequestKey is the strongly typed key of one shipment request
// aggregate. Every repository call carries it in full.
type ShipmentRequestKey struct {
	Scope             Scope
	ShipmentRequestID ShipmentRequestID
}

// Validate rejects an incomplete aggregate key.
func (k ShipmentRequestKey) Validate() error {
	if err := k.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(string(k.ShipmentRequestID)) == "" {
		return ErrShipmentRequestIDMissing
	}
	return nil
}

// ErrShipmentRequestIDMissing reports a key without a shipment request identity.
var ErrShipmentRequestIDMissing = errors.New("parcelshipment: shipment request id is required")
