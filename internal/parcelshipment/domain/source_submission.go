package domain

import (
	"errors"
	"time"
)

var ErrInvalidSourceSubmission = errors.New("parcel shipment: invalid source submission")

type SourceIdentity struct {
	tenantID          TenantID
	customerAccountID CustomerAccountID
	source            Source
	requestKey        SourceRequestKey
}

func NewSourceIdentity(
	tenantID TenantID,
	customerAccountID CustomerAccountID,
	source Source,
	requestKey SourceRequestKey,
) (SourceIdentity, error) {
	if !tenantID.valid() || !customerAccountID.valid() || !source.valid() || !requestKey.valid() {
		return SourceIdentity{}, ErrInvalidSourceSubmission
	}
	return SourceIdentity{
		tenantID:          tenantID,
		customerAccountID: customerAccountID,
		source:            source,
		requestKey:        requestKey,
	}, nil
}

func (identity SourceIdentity) TenantID() TenantID {
	return identity.tenantID
}

func (identity SourceIdentity) CustomerAccountID() CustomerAccountID {
	return identity.customerAccountID
}

func (identity SourceIdentity) Source() Source {
	return identity.source
}

func (identity SourceIdentity) RequestKey() SourceRequestKey {
	return identity.requestKey
}

func (identity SourceIdentity) valid() bool {
	return identity.tenantID.valid() &&
		identity.customerAccountID.valid() &&
		identity.source.valid() &&
		identity.requestKey.valid()
}

type SourceSubmissionFingerprint struct {
	identity   SourceIdentity
	digest     PayloadDigest
	occurredAt time.Time
	receivedAt time.Time
}

func NewSourceSubmissionFingerprint(
	identity SourceIdentity,
	digest PayloadDigest,
	occurredAt time.Time,
	receivedAt time.Time,
) (SourceSubmissionFingerprint, error) {
	if !identity.valid() || !digest.valid() || occurredAt.IsZero() || receivedAt.IsZero() {
		return SourceSubmissionFingerprint{}, ErrInvalidSourceSubmission
	}
	return SourceSubmissionFingerprint{
		identity:   identity,
		digest:     digest,
		occurredAt: occurredAt,
		receivedAt: receivedAt,
	}, nil
}

func (submission SourceSubmissionFingerprint) Identity() SourceIdentity {
	return submission.identity
}

func (submission SourceSubmissionFingerprint) Digest() PayloadDigest {
	return submission.digest
}

func (submission SourceSubmissionFingerprint) OccurredAt() time.Time {
	return submission.occurredAt
}

func (submission SourceSubmissionFingerprint) ReceivedAt() time.Time {
	return submission.receivedAt
}

func (submission SourceSubmissionFingerprint) valid() bool {
	return submission.identity.valid() &&
		submission.digest.valid() &&
		!submission.occurredAt.IsZero() &&
		!submission.receivedAt.IsZero()
}

type SourceClassification uint8

const (
	SourceDistinct SourceClassification = iota + 1
	SourceReplay
	SourceConflict
)

func ClassifySourceSubmission(
	existing SourceSubmissionFingerprint,
	incoming SourceSubmissionFingerprint,
) (SourceClassification, error) {
	if !existing.valid() || !incoming.valid() {
		return 0, ErrInvalidSourceSubmission
	}
	if existing.identity != incoming.identity {
		return SourceDistinct, nil
	}
	if existing.digest == incoming.digest {
		return SourceReplay, nil
	}
	return SourceConflict, nil
}
