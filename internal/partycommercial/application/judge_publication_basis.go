package application

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 有发出记录的草稿，发布要不要以一条有效接受为前提。
// 没有发出记录，这道门不适用。有发出记录而租户没登记，不设门，读口另答未配置。
// 带了依据就必须是这一版的一条接受，不管门开没开，免得记下一条对不上的引用。
// 合同还没指名这个计价范围时，不在这里发布新合同。下一步是走已有的合同发布，指名这个范围。

type PublicationBasisJudgment uint8

const (
	PublicationBasisJudgmentInvalid PublicationBasisJudgment = iota
	PublicationBasisNotApplicable
	PublicationBasisUnconfigured
	PublicationBasisNotRequired
	PublicationBasisSatisfied
	PublicationBasisAcceptanceRequired
	PublicationBasisCitationRejected
)

func (judgment PublicationBasisJudgment) String() string {
	switch judgment {
	case PublicationBasisNotApplicable:
		return "NOT_APPLICABLE"
	case PublicationBasisUnconfigured:
		return "UNCONFIGURED"
	case PublicationBasisNotRequired:
		return "NOT_REQUIRED"
	case PublicationBasisSatisfied:
		return "SATISFIED"
	case PublicationBasisAcceptanceRequired:
		return "ACCEPTANCE_REQUIRED"
	case PublicationBasisCitationRejected:
		return "CITATION_REJECTED"
	default:
		return ""
	}
}

func (judgment PublicationBasisJudgment) AllowsPublication() bool {
	switch judgment {
	case PublicationBasisNotApplicable, PublicationBasisUnconfigured, PublicationBasisNotRequired, PublicationBasisSatisfied:
		return true
	default:
		return false
	}
}

type PublicationBasisQuery struct {
	Tenant        domain.TenantID
	PlanID        string
	PlanVersion   string
	ContentDigest string
	Basis         string
}

func JudgePublicationBasis(ctx context.Context, view ports.QuotationPublicationView, query PublicationBasisQuery) (PublicationBasisJudgment, error) {
	if view == nil || query.Tenant.String() == "" || query.PlanID == "" || query.PlanVersion == "" || query.ContentDigest == "" {
		return 0, fmt.Errorf("party commercial: publication basis needs the view, tenant, plan and digest")
	}
	sends, err := view.ListQuotationSendsForPlan(ctx, query.Tenant, query.PlanID, query.PlanVersion)
	if err != nil {
		return 0, err
	}
	basis := strings.TrimSpace(query.Basis)
	if basis != "" {
		digest, accepted, found, replyErr := view.AcceptedQuotationReply(ctx, query.Tenant, basis)
		if replyErr != nil {
			return 0, replyErr
		}
		if !found || !accepted || digest != query.ContentDigest {
			return PublicationBasisCitationRejected, nil
		}
		matched := false
		for _, send := range sends {
			if send.ID == basis && send.Digest == query.ContentDigest {
				matched = true
			}
		}
		if !matched {
			return PublicationBasisCitationRejected, nil
		}
	}
	if len(sends) == 0 {
		return PublicationBasisNotApplicable, nil
	}
	requires, configured, err := view.LoadQuotationPublicationRequirement(ctx, query.Tenant)
	if err != nil {
		return 0, err
	}
	if !configured {
		return PublicationBasisUnconfigured, nil
	}
	if !requires {
		return PublicationBasisNotRequired, nil
	}
	if basis == "" {
		return PublicationBasisAcceptanceRequired, nil
	}
	return PublicationBasisSatisfied, nil
}

type QuotationPublicationRequirementOutcome uint8

const (
	QuotationPublicationRequirementOutcomeInvalid QuotationPublicationRequirementOutcome = iota
	QuotationPublicationRequirementRecorded
	QuotationPublicationRequirementReplay
	QuotationPublicationRequirementConflict
)

func (outcome QuotationPublicationRequirementOutcome) String() string {
	switch outcome {
	case QuotationPublicationRequirementRecorded:
		return "RECORDED"
	case QuotationPublicationRequirementReplay:
		return "ALREADY_RECORDED"
	case QuotationPublicationRequirementConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

func RegisterQuotationPublicationRequirement(
	ctx context.Context,
	view ports.QuotationPublicationView,
	requirement domain.QuotationPublicationRequirement,
) (QuotationPublicationRequirementOutcome, error) {
	if view == nil || requirement.Tenant().String() == "" {
		return 0, fmt.Errorf("party commercial: publication requirement needs a view and a tenant")
	}
	same, saved, err := view.SaveQuotationPublicationRequirement(ctx, requirement)
	if err != nil {
		return 0, err
	}
	switch {
	case saved:
		return QuotationPublicationRequirementRecorded, nil
	case same:
		return QuotationPublicationRequirementReplay, nil
	default:
		return QuotationPublicationRequirementConflict, nil
	}
}
