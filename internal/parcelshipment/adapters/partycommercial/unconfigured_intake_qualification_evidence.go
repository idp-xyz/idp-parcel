package partycommercial

import (
	"context"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconfiguredIntakeQualificationEvidence 是生产装配的诚实未配置证据口（ADR-0063）。
// 声明已经列出硬资格，但没有任何权威方被接上——包括未知前缀。一律答未证明，
// 不编造已证明，也不把这一格折成资格目录未配置。
type UnconfiguredIntakeQualificationEvidence struct{}

var _ psports.IntakeQualificationEvidenceView = UnconfiguredIntakeQualificationEvidence{}

func (UnconfiguredIntakeQualificationEvidence) ProveIntakeQualification(
	context.Context,
	psdomain.SourceIdentity,
	psdomain.IntakeSource,
	psdomain.QualificationRuleReference,
	time.Time,
) (psports.IntakeQualificationProof, error) {
	return psports.IntakeQualificationUnproven, nil
}
