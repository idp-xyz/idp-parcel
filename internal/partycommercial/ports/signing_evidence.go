package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 签署证据挂在客户合同或供应商协议的版本旁。登记册只追加：同标识同内容是重放，异内容是冲突，
// 更正是另一条新证据。要求规则一户一行，缺行就是没登记。

// SigningEvidenceSaveOutcome 是一条签署证据登记的落点。
type SigningEvidenceSaveOutcome uint8

const (
	SigningEvidenceSaveOutcomeInvalid SigningEvidenceSaveOutcome = iota
	SigningEvidenceSaved
	SigningEvidenceAlreadyRegistered
	SigningEvidenceContentConflict
	SigningEvidenceTargetNotFound
	SigningEvidenceAlreadyCorrected
	SigningEvidenceCrossVersion
	SigningEvidenceCorrectsNotFound
)

func (outcome SigningEvidenceSaveOutcome) String() string {
	switch outcome {
	case SigningEvidenceSaved:
		return "SAVED"
	case SigningEvidenceAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case SigningEvidenceContentConflict:
		return "CONTENT_CONFLICT"
	case SigningEvidenceTargetNotFound:
		return "TARGET_NOT_FOUND"
	case SigningEvidenceAlreadyCorrected:
		return "ALREADY_CORRECTED"
	case SigningEvidenceCrossVersion:
		return "CROSS_VERSION"
	case SigningEvidenceCorrectsNotFound:
		return "CORRECTS_NOT_FOUND"
	default:
		return ""
	}
}

// SigningEvidenceRequirementSaveOutcome 是签署证据要求规则的落点。一户一行，撞键不覆盖。
type SigningEvidenceRequirementSaveOutcome uint8

const (
	SigningEvidenceRequirementSaveOutcomeInvalid SigningEvidenceRequirementSaveOutcome = iota
	SigningEvidenceRequirementSaved
	SigningEvidenceRequirementAlreadyRegistered
	SigningEvidenceRequirementContentConflict
)

func (outcome SigningEvidenceRequirementSaveOutcome) String() string {
	switch outcome {
	case SigningEvidenceRequirementSaved:
		return "SAVED"
	case SigningEvidenceRequirementAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case SigningEvidenceRequirementContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// SigningEvidenceView 是发布门与读面用的读口。found=false 就是要求还没登记，发布不设这道门，读口答未配置。
type SigningEvidenceView interface {
	LoadSigningEvidenceRequirement(ctx context.Context, tenant domain.TenantID) (domain.SigningEvidenceRequirement, bool, error)
	HasSigningEvidence(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CommercialObjectKind,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
	) (bool, error)
	ListSigningEvidence(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CommercialObjectKind,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
	) ([]domain.SigningEvidence, error)
}

// SigningEvidenceRegistry 在读口上加两个只追加的写口。写口无事务即拒。
type SigningEvidenceRegistry interface {
	SigningEvidenceView
	SaveSigningEvidence(ctx context.Context, evidence domain.SigningEvidence) (SigningEvidenceSaveOutcome, error)
	SaveSigningEvidenceRequirement(ctx context.Context, rule domain.SigningEvidenceRequirement) (SigningEvidenceRequirementSaveOutcome, error)
}
