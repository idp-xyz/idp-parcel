package domain

import (
	"errors"
	"time"
)

// ErrValidityCorrectionInvalid 是区间更正本身不成立（缺引用、缺时间或区间非法）。
// 它不是正文冲突，也不是草稿 Revise。
var ErrValidityCorrectionInvalid = errors.New("party commercial: invalid validity correction")

// ValidityCorrectionReference 指向外部源那次「更正历史有效区间」的证据，不是新的正文版本号。
type ValidityCorrectionReference struct{ requiredValue }

func NewValidityCorrectionReference(value string) (ValidityCorrectionReference, error) {
	required, err := newRequiredValue("validity correction reference", value)
	return ValidityCorrectionReference{required}, err
}

// ValidityCorrection 是登记册上的区间更正事实：指向原对象版本，携带新区间与更正引用。
// 原版本键下的正文、批准与原区间不被改写（ADR-0038 / AT-PC-013）。
type ValidityCorrection struct {
	tenant    TenantID
	kind      CommercialObjectKind
	objectID  CommercialObjectID
	version   CommercialVersionLabel
	corrected EffectiveInterval
	reference ValidityCorrectionReference
	at        time.Time
}

func (correction ValidityCorrection) Tenant() TenantID {
	return correction.tenant
}

func (correction ValidityCorrection) Kind() CommercialObjectKind {
	return correction.kind
}

func (correction ValidityCorrection) ObjectID() CommercialObjectID {
	return correction.objectID
}

func (correction ValidityCorrection) Version() CommercialVersionLabel {
	return correction.version
}

func (correction ValidityCorrection) CorrectedInterval() EffectiveInterval {
	return correction.corrected
}

func (correction ValidityCorrection) Reference() ValidityCorrectionReference {
	return correction.reference
}

func (correction ValidityCorrection) CorrectedAt() time.Time {
	return correction.at
}

// CorrectEffectiveInterval 从已发布（含其后生命周期）的版本构造一条区间更正。
// 草稿没有可更正的历史发布；改正文仍走 Revise / 新版本号，不走这里。
func (version CommercialVersion) CorrectEffectiveInterval(
	corrected EffectiveInterval,
	reference ValidityCorrectionReference,
	at time.Time,
) (ValidityCorrection, error) {
	if version.status == CommercialVersionStatusInvalid || version.status == CommercialVersionDraft {
		return ValidityCorrection{}, ErrInvalidCommercialTransition
	}
	if !corrected.valid() || !reference.valid() || at.IsZero() {
		return ValidityCorrection{}, ErrValidityCorrectionInvalid
	}
	return ValidityCorrection{
		tenant:    version.tenant,
		kind:      version.kind,
		objectID:  version.objectID,
		version:   version.version,
		corrected: corrected,
		reference: reference,
		at:        at.UTC(),
	}, nil
}

func sameValidityCorrection(left, right ValidityCorrection) bool {
	leftEnd, leftBounded := left.corrected.EndsAt()
	rightEnd, rightBounded := right.corrected.EndsAt()
	return left.tenant == right.tenant &&
		left.kind == right.kind &&
		left.objectID == right.objectID &&
		left.version == right.version &&
		left.reference == right.reference &&
		left.corrected.StartsAt().Equal(right.corrected.StartsAt()) &&
		leftBounded == rightBounded &&
		(!leftBounded || leftEnd.Equal(rightEnd)) &&
		left.at.Equal(right.at)
}
