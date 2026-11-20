package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 签署证据的登记与读回。写口只追加；要求规则一户一行。读口在要求没登记时如实交回 found=false，不替租户设门。

type SigningEvidenceRegistrationOutcome uint8

const (
	SigningEvidenceRegistrationOutcomeInvalid SigningEvidenceRegistrationOutcome = iota
	SigningEvidenceRegistered
	SigningEvidenceAlreadyRegistered
	SigningEvidenceContentConflict
	SigningEvidenceTargetNotFound
	SigningEvidenceAlreadyCorrected
	SigningEvidenceCrossVersion
	SigningEvidenceCorrectsNotFound
)

func (outcome SigningEvidenceRegistrationOutcome) String() string {
	switch outcome {
	case SigningEvidenceRegistered:
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

type RegisterSigningEvidenceCommand struct {
	Evidence domain.SigningEvidence
}

type SigningEvidenceRegistrationResult struct {
	outcome SigningEvidenceRegistrationOutcome
}

func (result SigningEvidenceRegistrationResult) Outcome() SigningEvidenceRegistrationOutcome {
	return result.outcome
}

type RegisterSigningEvidenceHandler struct {
	registry ports.SigningEvidenceRegistry
}

func NewRegisterSigningEvidenceHandler(registry ports.SigningEvidenceRegistry) *RegisterSigningEvidenceHandler {
	return &RegisterSigningEvidenceHandler{registry: registry}
}

func (handler *RegisterSigningEvidenceHandler) Handle(
	ctx context.Context,
	command RegisterSigningEvidenceCommand,
) (SigningEvidenceRegistrationResult, error) {
	outcome, err := handler.registry.SaveSigningEvidence(ctx, command.Evidence)
	if err != nil {
		return SigningEvidenceRegistrationResult{}, fmt.Errorf("register signing evidence: %w", err)
	}
	mapped, ok := signingEvidenceRegistrationOutcome(outcome)
	if !ok {
		return SigningEvidenceRegistrationResult{}, fmt.Errorf("register signing evidence: unexpected outcome %q", outcome)
	}
	return SigningEvidenceRegistrationResult{outcome: mapped}, nil
}

func signingEvidenceRegistrationOutcome(outcome ports.SigningEvidenceSaveOutcome) (SigningEvidenceRegistrationOutcome, bool) {
	switch outcome {
	case ports.SigningEvidenceSaved:
		return SigningEvidenceRegistered, true
	case ports.SigningEvidenceAlreadyRegistered:
		return SigningEvidenceAlreadyRegistered, true
	case ports.SigningEvidenceContentConflict:
		return SigningEvidenceContentConflict, true
	case ports.SigningEvidenceTargetNotFound:
		return SigningEvidenceTargetNotFound, true
	case ports.SigningEvidenceAlreadyCorrected:
		return SigningEvidenceAlreadyCorrected, true
	case ports.SigningEvidenceCrossVersion:
		return SigningEvidenceCrossVersion, true
	case ports.SigningEvidenceCorrectsNotFound:
		return SigningEvidenceCorrectsNotFound, true
	default:
		return SigningEvidenceRegistrationOutcomeInvalid, false
	}
}

type SigningEvidenceRequirementOutcome uint8

const (
	SigningEvidenceRequirementOutcomeInvalid SigningEvidenceRequirementOutcome = iota
	SigningEvidenceRequirementRegistered
	SigningEvidenceRequirementAlreadyRegistered
	SigningEvidenceRequirementContentConflict
)

func (outcome SigningEvidenceRequirementOutcome) String() string {
	switch outcome {
	case SigningEvidenceRequirementRegistered:
		return "SAVED"
	case SigningEvidenceRequirementAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case SigningEvidenceRequirementContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

type RegisterSigningEvidenceRequirementCommand struct {
	Rule domain.SigningEvidenceRequirement
}

type SigningEvidenceRequirementResult struct {
	outcome SigningEvidenceRequirementOutcome
}

func (result SigningEvidenceRequirementResult) Outcome() SigningEvidenceRequirementOutcome {
	return result.outcome
}

type RegisterSigningEvidenceRequirementHandler struct {
	registry ports.SigningEvidenceRegistry
}

func NewRegisterSigningEvidenceRequirementHandler(registry ports.SigningEvidenceRegistry) *RegisterSigningEvidenceRequirementHandler {
	return &RegisterSigningEvidenceRequirementHandler{registry: registry}
}

func (handler *RegisterSigningEvidenceRequirementHandler) Handle(
	ctx context.Context,
	command RegisterSigningEvidenceRequirementCommand,
) (SigningEvidenceRequirementResult, error) {
	outcome, err := handler.registry.SaveSigningEvidenceRequirement(ctx, command.Rule)
	if err != nil {
		return SigningEvidenceRequirementResult{}, fmt.Errorf("register signing evidence requirement: %w", err)
	}
	switch outcome {
	case ports.SigningEvidenceRequirementSaved:
		return SigningEvidenceRequirementResult{outcome: SigningEvidenceRequirementRegistered}, nil
	case ports.SigningEvidenceRequirementAlreadyRegistered:
		return SigningEvidenceRequirementResult{outcome: SigningEvidenceRequirementAlreadyRegistered}, nil
	case ports.SigningEvidenceRequirementContentConflict:
		return SigningEvidenceRequirementResult{outcome: SigningEvidenceRequirementContentConflict}, nil
	default:
		return SigningEvidenceRequirementResult{}, fmt.Errorf("register signing evidence requirement: unexpected outcome %q", outcome)
	}
}

// SigningEvidenceRead 是某一版旁边的要求状态与证据历史。RequirementFound 为假就是没登记，不是「两个布尔都不要」。
type SigningEvidenceRead struct {
	RequirementFound bool
	Requirement      domain.SigningEvidenceRequirement
	Evidence         []domain.SigningEvidence
}

type QuerySigningEvidenceHandler struct {
	view ports.SigningEvidenceView
}

func NewQuerySigningEvidenceHandler(view ports.SigningEvidenceView) *QuerySigningEvidenceHandler {
	return &QuerySigningEvidenceHandler{view: view}
}

func (handler *QuerySigningEvidenceHandler) Handle(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CommercialObjectKind,
	objectID domain.CommercialObjectID,
	version domain.CommercialVersionLabel,
) (SigningEvidenceRead, error) {
	if kind != domain.CustomerContractObject && kind != domain.SupplierAgreementObject {
		return SigningEvidenceRead{}, fmt.Errorf("query signing evidence: %w", domain.ErrInvalidSigningEvidence)
	}
	rule, found, err := handler.view.LoadSigningEvidenceRequirement(ctx, tenant)
	if err != nil {
		return SigningEvidenceRead{}, fmt.Errorf("query signing evidence: %w", err)
	}
	listed, err := handler.view.ListSigningEvidence(ctx, tenant, kind, objectID, version)
	if err != nil {
		return SigningEvidenceRead{}, fmt.Errorf("query signing evidence: %w", err)
	}
	return SigningEvidenceRead{RequirementFound: found, Requirement: rule, Evidence: listed}, nil
}
