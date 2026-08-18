package partycommercial

import (
	"context"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// KnownPrefixIntakeQualificationEvidence 按引用前缀把证明交给已登记的权威口。
// 未登记前缀答未证明，不得已证明（ADR-0063）。生产装配不登记任何前缀——不接假
// 关务身份，也不把 UC-NR-002 禁限运登记进来。
type KnownPrefixIntakeQualificationEvidence struct {
	byPrefix map[string]psports.IntakeQualificationEvidenceView
}

func NewKnownPrefixIntakeQualificationEvidence(
	byPrefix map[string]psports.IntakeQualificationEvidenceView,
) KnownPrefixIntakeQualificationEvidence {
	return KnownPrefixIntakeQualificationEvidence{byPrefix: byPrefix}
}

var _ psports.IntakeQualificationEvidenceView = KnownPrefixIntakeQualificationEvidence{}

func (view KnownPrefixIntakeQualificationEvidence) ProveIntakeQualification(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	source psdomain.IntakeSource,
	rule psdomain.QualificationRuleReference,
	asOf time.Time,
) (psports.IntakeQualificationProof, error) {
	inner, ok := view.byPrefix[psdomain.QualificationRulePrefix(rule)]
	if !ok || inner == nil {
		return psports.IntakeQualificationUnproven, nil
	}
	return inner.ProveIntakeQualification(ctx, identity, source, rule, asOf)
}
