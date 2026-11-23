// Package parcelpricing 是参与方与商业对小包计价的消费侧适配器：读已固定的销售价格内容，
// 并取对客价格表。计价不知道这份读是为了发出。
package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// priceCardDraftByReference 只取草稿册按方案版本读的那一半。
type priceCardDraftByReference interface {
	LoadPriceCardDraft(ctx context.Context, tenant ppdomain.TenantID, plan ppdomain.VersionReference) (ppdomain.PriceCardDraft, bool, error)
}

// QuotedPrices 把草稿册与价卡登记册译成发出要用的事实。不选卡：不问范围、时点或目的。
type QuotedPrices struct {
	drafts priceCardDraftByReference
	cards  ppports.RegisteredPriceCardRead
	sheets ppports.DisclosedRateSheetProjector
}

func NewQuotedPrices(
	drafts priceCardDraftByReference,
	cards ppports.RegisteredPriceCardRead,
	sheets ppports.DisclosedRateSheetProjector,
) (*QuotedPrices, error) {
	if drafts == nil || cards == nil || sheets == nil {
		return nil, fmt.Errorf("party commercial: quoted price reader needs the draft book, the price card register and the rate sheet projector")
	}
	return &QuotedPrices{drafts: drafts, cards: cards, sheets: sheets}, nil
}

var _ ports.QuotedPriceReader = (*QuotedPrices)(nil)

func (reader *QuotedPrices) ReadQuotedPrice(ctx context.Context, query ports.QuotedPriceQuery) (ports.QuotedPriceFact, error) {
	var none ports.QuotedPriceFact
	tenant, reference, err := pricingReference(query)
	if err != nil {
		return none, err
	}
	switch query.Lane {
	case domain.QuotationPriceFromApprovedDraft:
		return reader.readDraft(ctx, tenant, reference)
	case domain.QuotationPriceFromRegisteredCard:
		return reader.readCard(ctx, tenant, reference)
	default:
		return none, fmt.Errorf("party commercial: quoted price lane %s", query.Lane)
	}
}

func (reader *QuotedPrices) readDraft(ctx context.Context, tenant ppdomain.TenantID, reference ppdomain.VersionReference) (ports.QuotedPriceFact, error) {
	var none ports.QuotedPriceFact
	draft, found, err := reader.drafts.LoadPriceCardDraft(ctx, tenant, reference)
	if err != nil {
		return none, err
	}
	if !found {
		return none, nil
	}
	fact := ports.QuotedPriceFact{Found: true}
	switch draft.Status() {
	case ppdomain.PriceCardDraftStatusDraft, ppdomain.PriceCardDraftStatusValidated:
		fact.Standing = ports.QuotedPriceNotFixed
		return fact, nil
	case ppdomain.PriceCardDraftStatusWithdrawn:
		fact.Standing = ports.QuotedPriceWithdrawn
	case ppdomain.PriceCardDraftStatusPublished:
		fact.Standing = ports.QuotedPriceUseRegisteredCard
	case ppdomain.PriceCardDraftStatusApproved:
		fact.Standing = ports.QuotedPriceReady
	default:
		return none, fmt.Errorf("party commercial: price card draft status %s", draft.Status())
	}
	content, ok := draft.Content()
	if !ok {
		return none, fmt.Errorf("party commercial: price card draft %s has no content", draft.Status())
	}
	return reader.project(fact, content.Plan, content.DirectionAuthorization)
}

func (reader *QuotedPrices) readCard(ctx context.Context, tenant ppdomain.TenantID, reference ppdomain.VersionReference) (ports.QuotedPriceFact, error) {
	var none ports.QuotedPriceFact
	subject, found, err := reader.cards.ReadRegisteredPriceCard(ctx, tenant, reference)
	if err != nil {
		if errors.Is(err, ppdomain.ErrCanonicalizationVersionUnsupported) {
			return none, err
		}
		return none, err
	}
	if !found {
		return none, nil
	}
	fact := ports.QuotedPriceFact{Found: true, Standing: ports.QuotedPriceReady}
	return reader.project(fact, subject.Plan, subject.DirectionAuthorization)
}

func (reader *QuotedPrices) project(
	fact ports.QuotedPriceFact,
	plan ppdomain.PricingPlanVersion,
	authorization ppdomain.VersionReference,
) (ports.QuotedPriceFact, error) {
	fact.Canonicalization = plan.CanonicalizationVersion()
	fact.ContentDigest = plan.ContentDigest()
	fact.DirectionSell = plan.Direction() == ppdomain.PricingDirectionSell
	projection, err := reader.sheets.ProjectDisclosedRateSheet(plan, authorization)
	if err != nil {
		return ports.QuotedPriceFact{}, err
	}
	if projection.Gap.Where != "" {
		fact.GapWhere = projection.Gap.Where
		fact.GapConclusion = projection.Gap.Conclusion
		return fact, nil
	}
	fact.Workbook = projection.Document.Body
	fact.MediaType = projection.Document.MediaType
	return fact, nil
}

func pricingReference(query ports.QuotedPriceQuery) (ppdomain.TenantID, ppdomain.VersionReference, error) {
	var noneTenant ppdomain.TenantID
	var noneRef ppdomain.VersionReference
	if query.Tenant.String() == "" || query.PlanID == "" || query.PlanVersion == "" {
		return noneTenant, noneRef, fmt.Errorf("party commercial: quoted price needs a tenant and a plan reference")
	}
	tenant, err := ppdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return noneTenant, noneRef, fmt.Errorf("party commercial: quoted price tenant: %w", err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactPricingPlan, query.PlanID, query.PlanVersion)
	if err != nil {
		return noneTenant, noneRef, fmt.Errorf("party commercial: quoted price plan: %w", err)
	}
	return tenant, reference, nil
}
