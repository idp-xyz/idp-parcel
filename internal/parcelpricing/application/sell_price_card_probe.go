package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// SellPriceCardProbe 在收寄改道时按有序范围探哪一格的基础运费表有这一票。不入册。
// 与按列表形成评价用同一套在用价卡、输入读口和不入册探评：两边对「缺格才问下一格」的判法必须相同。
type SellPriceCardProbe struct {
	cards    ports.PriceCardInForceResolver
	inputs   ports.PricingInputResolver
	evaluate *EvaluatePricingHandler
}

func NewSellPriceCardProbe(
	cards ports.PriceCardInForceResolver,
	inputs ports.PricingInputResolver,
	evaluate *EvaluatePricingHandler,
) (*SellPriceCardProbe, error) {
	switch {
	case cards == nil:
		return nil, fmt.Errorf("%w: price card in-force resolver", ErrNilDependency)
	case inputs == nil:
		return nil, fmt.Errorf("%w: pricing input resolver", ErrNilDependency)
	case evaluate == nil:
		return nil, fmt.Errorf("%w: evaluate pricing handler", ErrNilDependency)
	}
	return &SellPriceCardProbe{cards: cards, inputs: inputs, evaluate: evaluate}, nil
}

// Resolve 顺着范围列表探。没有在用卡就看下一格；适用冲突立刻停。逐委托卡整票缺格、逐包裹卡每一件都缺格，
// 且后面还有范围，才跳过。依赖读不回是 error，调用方据此答未决，不把读失败当成缺格。
func (probe *SellPriceCardProbe) Resolve(
	ctx context.Context,
	tenant domain.TenantID,
	shipment string,
	members []string,
	currency domain.Currency,
	scopes []domain.PricingScopeID,
	at time.Time,
	product domain.ServiceProductReference,
) (ports.PriceCardInForceResolution, error) {
	var last ports.PriceCardInForceResolution
	for index, scope := range scopes {
		resolution, err := probe.cards.ResolveInForce(ctx, tenant, scope,
			domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, at, product)
		if err != nil {
			return ports.PriceCardInForceResolution{}, err
		}
		lastScope := index == len(scopes)-1
		switch resolution.Outcome {
		case ports.PriceCardNotConfigured:
			last = resolution
			continue
		case ports.PriceCardApplicabilityConflict:
			return resolution, nil
		case ports.PriceCardVersionInForce:
		default:
			return ports.PriceCardInForceResolution{}, fmt.Errorf("sell price card probe: unhandled in-force outcome %d", resolution.Outcome)
		}
		miss, err := probe.scopeMisses(ctx, tenant, shipment, members, currency, scope, at, resolution.Plan, product)
		if err != nil {
			return ports.PriceCardInForceResolution{}, err
		}
		if miss && !lastScope {
			continue
		}
		return resolution, nil
	}
	if last.Outcome == ports.PriceCardInForceOutcomeInvalid {
		last.Outcome = ports.PriceCardNotConfigured
	}
	return last, nil
}

func (probe *SellPriceCardProbe) scopeMisses(
	ctx context.Context,
	tenant domain.TenantID,
	shipment string,
	members []string,
	currency domain.Currency,
	scope domain.PricingScopeID,
	at time.Time,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, error) {
	switch plan.Aggregation() {
	case domain.AggregationPerShipment:
		return probe.shipmentMisses(ctx, tenant, shipment, members, currency, scope, at, plan, product)
	case domain.AggregationPerPackage:
		return probe.packagesMiss(ctx, tenant, members, currency, scope, at, plan, product)
	default:
		return false, nil
	}
}

func (probe *SellPriceCardProbe) shipmentMisses(
	ctx context.Context,
	tenant domain.TenantID,
	shipment string,
	members []string,
	currency domain.Currency,
	scope domain.PricingScopeID,
	at time.Time,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, error) {
	if shipment == "" || len(members) == 0 {
		return false, nil
	}
	return probe.inputMisses(ctx, ports.PricingInputQuery{
		Tenant:      tenant,
		Scope:       scope,
		Purpose:     domain.PricingPurposeCustomerCharge,
		BasisAt:     at,
		Sources:     ports.EligibleSourceReferences{Shipment: shipment, Members: append([]string(nil), members...)},
		Settlement:  currency,
		Aggregation: domain.AggregationPerShipment,
	}, plan, product)
}

func (probe *SellPriceCardProbe) packagesMiss(
	ctx context.Context,
	tenant domain.TenantID,
	members []string,
	currency domain.Currency,
	scope domain.PricingScopeID,
	at time.Time,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, error) {
	if len(members) == 0 {
		return false, nil
	}
	for _, member := range members {
		miss, err := probe.inputMisses(ctx, ports.PricingInputQuery{
			Tenant:      tenant,
			Scope:       scope,
			Purpose:     domain.PricingPurposeCustomerCharge,
			BasisAt:     at,
			Sources:     ports.EligibleSourceReferences{Parcel: member},
			Settlement:  currency,
			Aggregation: domain.AggregationPerPackage,
		}, plan, product)
		if err != nil || !miss {
			return false, err
		}
	}
	return true, nil
}

func (probe *SellPriceCardProbe) inputMisses(
	ctx context.Context,
	query ports.PricingInputQuery,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, error) {
	resolved, err := probe.inputs.ResolvePricingInput(ctx, query)
	if err != nil {
		return false, err
	}
	if resolved.Outcome != ports.PricingInputResolved {
		return false, nil
	}
	input := resolved.Input
	if product.Declared() {
		input, err = input.WithServiceProduct(product)
		if err != nil {
			return false, err
		}
	}
	probed, undecided, err := probeResolvedPlan(ctx, probe.evaluate, plan, input, domain.EvidenceSynthetic)
	if err != nil {
		return false, err
	}
	if undecided {
		return false, fmt.Errorf("sell price card probe: pricing readings are unavailable")
	}
	return probed.BaseRateTableMiss(), nil
}

// probeResolvedPlan 先补齐目录与序列读数，再算一份不入册的评价，只为看它停在哪。半套装配是装配错误。
// 补读数读不回时第二个返回值为真：在用与否未知，不能当成这一格的结论。
func probeResolvedPlan(
	ctx context.Context,
	evaluate *EvaluatePricingHandler,
	plan domain.PricingPlanVersion,
	input domain.PricingInputSnapshot,
	evidence domain.EvidenceKind,
) (domain.PricingEvaluation, bool, error) {
	completion := evaluate.completion()
	if err := completion.halfWired(); err != nil {
		return domain.PricingEvaluation{}, false, err
	}
	id, err := domain.NewEvaluationID("EVAL-PROBE")
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("probe sell price source: %w", err)
	}
	request, err := domain.NewEvaluationRequest(id, plan, input, evidence)
	if err != nil {
		return domain.PricingEvaluation{}, false, fmt.Errorf("probe sell price source: %w", err)
	}
	request, _, err = completion.complete(ctx, request)
	if err != nil {
		return domain.PricingEvaluation{}, true, nil
	}
	return domain.EvaluatePricing(request), false, nil
}
