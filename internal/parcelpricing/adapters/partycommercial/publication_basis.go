package partycommercial

import (
	"context"

	pricingports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// PublicationBasis 把计价的发布问句译成参与方与商业的裁法。计价领域不认得报价。
type PublicationBasis struct {
	view ports.QuotationPublicationView
}

func NewPublicationBasis(view ports.QuotationPublicationView) (*PublicationBasis, error) {
	if view == nil {
		return nil, errPublicationBasis("view is nil")
	}
	return &PublicationBasis{view: view}, nil
}

var _ pricingports.PublicationBasisReader = (*PublicationBasis)(nil)

func (reader *PublicationBasis) JudgePublicationBasis(ctx context.Context, query pricingports.PublicationBasisQuery) (pricingports.PublicationBasisStanding, error) {
	tenant, err := pcdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return 0, err
	}
	judgment, err := application.JudgePublicationBasis(ctx, reader.view, application.PublicationBasisQuery{
		Tenant: tenant, PlanID: query.PlanID, PlanVersion: query.PlanVersion,
		ContentDigest: query.ContentDigest, Basis: query.Basis,
	})
	if err != nil {
		return 0, err
	}
	switch judgment {
	case application.PublicationBasisAcceptanceRequired:
		return pricingports.PublicationBasisAcceptanceRequired, nil
	case application.PublicationBasisCitationRejected:
		return pricingports.PublicationBasisCitationRejected, nil
	case application.PublicationBasisNotApplicable, application.PublicationBasisUnconfigured,
		application.PublicationBasisNotRequired, application.PublicationBasisSatisfied:
		return pricingports.PublicationBasisAllow, nil
	default:
		return 0, errPublicationBasis(judgment.String())
	}
}

type publicationBasisError string

func errPublicationBasis(text string) error { return publicationBasisError(text) }

func (err publicationBasisError) Error() string {
	return "parcel pricing: publication basis: " + string(err)
}
