package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// estimateAcrossSellPriceSources 按合同声明的顺序走范围（ADR-0380 决定三）。没有在用卡，或这一张卡对整票基础运费表都缺格，
// 才问下一个；适用冲突停住。逐委托卡整票试算，逐包裹卡逐件试算。一张逐包裹卡只有在每一件都缺格、且后面还有范围时才整张跳过，
// 这样下一张逐委托卡才能接到整票；已经有一件命中时，只让缺格的那几件继续问后面的逐包裹范围。
func (handler *FormPreAcceptanceEstimatesHandler) estimateAcrossSellPriceSources(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	declaration ports.SubmissionDeclaration,
) (FormPreAcceptanceEstimatesResult, error) {
	prefix := make([]SellPriceSourceStep, 0, len(command.Scopes))
	for index, scope := range command.Scopes {
		resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, scope,
			domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, command.At, declaration.ServiceProduct)
		if err != nil {
			return undecidedPreAcceptance(PreAcceptancePriceCardResolutionUnavailable, domain.VersionReference{}), nil
		}
		last := index == len(command.Scopes)-1
		switch resolution.Outcome {
		case ports.PriceCardNotConfigured:
			prefix = append(prefix, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceNoCard})
			continue
		case ports.PriceCardApplicabilityConflict:
			trail := append(append([]SellPriceSourceStep{}, prefix...), SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceConflict})
			estimates := make([]PreAcceptanceParcelEstimate, len(declaration.Parcels))
			for parcelIndex, parcel := range declaration.Parcels {
				estimates[parcelIndex] = PreAcceptanceParcelEstimate{Parcel: parcel.Parcel, SourceTrail: append([]SellPriceSourceStep{}, trail...)}
			}
			return FormPreAcceptanceEstimatesResult{
				Outcome:   PreAcceptancePriceCardApplicabilityConflict,
				Conflict:  append([]domain.VersionReference(nil), resolution.Candidates...),
				Estimates: estimates,
			}, nil
		case ports.PriceCardVersionInForce:
		default:
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance estimates: unhandled price card resolution %d", resolution.Outcome)
		}

		if resolution.Plan.Aggregation() == domain.AggregationPerShipment {
			result, err := handler.estimateWholeShipment(ctx, command, resolution.Plan, declaration)
			if err != nil {
				return result, err
			}
			if !last && wholeShipmentBaseRateMiss(result) {
				prefix = append(prefix, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing})
				continue
			}
			return stampWholeShipment(result, prefix, scope), nil
		}

		estimates, allMiss, undecided, err := handler.estimateDeclaredParcels(ctx, command, resolution.Plan, declaration)
		if err != nil {
			return FormPreAcceptanceEstimatesResult{}, err
		}
		if undecided {
			return undecidedPreAcceptance(PreAcceptanceReadingCompletionUnavailable, resolution.Plan.Reference()), nil
		}
		if !last && allMiss {
			prefix = append(prefix, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing})
			continue
		}
		return handler.keepPackageScope(ctx, command, declaration, estimates, prefix, index, scope)
	}
	return noCardPreAcceptance(declaration, prefix), nil
}

func wholeShipmentBaseRateMiss(result FormPreAcceptanceEstimatesResult) bool {
	if result.Outcome != PreAcceptanceEstimatesFormed || len(result.Estimates) != 1 {
		return false
	}
	estimate := result.Estimates[0]
	return estimate.Answer == EstimateCandidateEvaluated && estimate.Evaluation.BaseRateTableMiss()
}

func stampWholeShipment(result FormPreAcceptanceEstimatesResult, prefix []SellPriceSourceStep, scope domain.PricingScopeID) FormPreAcceptanceEstimatesResult {
	kind := SellPriceSourceStopped
	if len(result.Estimates) == 1 && result.Estimates[0].Answer == EstimateCandidateEvaluated {
		switch {
		case result.Estimates[0].Evaluation.BaseRateTableMiss():
			kind = SellPriceSourceBaseRateMissing
		case result.Estimates[0].Evaluation.Status() == domain.EvaluationCompleted:
			kind = SellPriceSourceSelected
		}
	}
	trail := append(append([]SellPriceSourceStep{}, prefix...), SellPriceSourceStep{Scope: scope, Kind: kind})
	for index := range result.Estimates {
		result.Estimates[index].Scope = scope
		result.Estimates[index].SourceTrail = append([]SellPriceSourceStep{}, trail...)
	}
	return result
}

// estimateDeclaredParcels 用这一张逐包裹卡把每一件声明都试一遍。allMiss 为真表示每一件都已评价且基础运费表缺格：
// 缺测量、待判断、命中都不算整票缺格，不能因此整张跳到下一范围。
func (handler *FormPreAcceptanceEstimatesHandler) estimateDeclaredParcels(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	plan domain.PricingPlanVersion,
	declaration ports.SubmissionDeclaration,
) ([]PreAcceptanceParcelEstimate, bool, bool, error) {
	estimates := make([]PreAcceptanceParcelEstimate, len(declaration.Parcels))
	allMiss := len(declaration.Parcels) > 0
	settlement := command.Settlement
	for index, parcel := range declaration.Parcels {
		estimates[index] = PreAcceptanceParcelEstimate{Parcel: parcel.Parcel}
		if parcel.Weight == nil {
			estimates[index].Answer = EstimateCandidateInputIncomplete
			estimates[index].Missing = []string{EstimateMissingMeasurement}
			allMiss = false
			continue
		}
		one, err := handler.deps.Estimates.Handle(ctx, FormPlanGivenEstimateEvaluationsCommand{
			Tenant:   command.Tenant,
			Plan:     plan,
			Evidence: handler.deps.Evidence,
			Declarations: []PlanGivenEstimateDeclaration{{
				Subject:        preAcceptanceSubject(command, parcel.Parcel),
				BasisAt:        command.At,
				Weight:         *parcel.Weight,
				Dimensions:     parcel.Dimensions,
				Zone:           command.Zone,
				Route:          declaration.Route,
				Settlement:     &settlement,
				ServiceProduct: declaration.ServiceProduct,
			}},
		})
		if err != nil {
			return nil, false, false, fmt.Errorf("pre-acceptance estimates: %w", err)
		}
		switch one.Outcome {
		case PlanGivenEstimatesUndecided:
			return nil, false, true, nil
		case PlanGivenEstimatesFormed:
		default:
			return nil, false, false, fmt.Errorf("pre-acceptance estimates: unhandled plan-given outcome %d", one.Outcome)
		}
		got := one.Estimates[0]
		estimates[index].Answer = got.Answer
		estimates[index].Evaluation = got.Evaluation
		estimates[index].Missing = append([]string(nil), got.Missing...)
		if got.Answer != EstimateCandidateEvaluated || !got.Evaluation.BaseRateTableMiss() {
			allMiss = false
		}
	}
	return estimates, allMiss, false, nil
}

// keepPackageScope 留下这一张逐包裹卡上已经算出的结果。缺格的个别件继续问后面的范围；至少一件不是缺格，或已经是最后一格，
// 都不把整张卡跳过。
func (handler *FormPreAcceptanceEstimatesHandler) keepPackageScope(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	declaration ports.SubmissionDeclaration,
	estimates []PreAcceptanceParcelEstimate,
	prefix []SellPriceSourceStep,
	index int,
	scope domain.PricingScopeID,
) (FormPreAcceptanceEstimatesResult, error) {
	last := index == len(command.Scopes)-1
	formedAny := false
	for parcelIndex, parcel := range declaration.Parcels {
		estimate := estimates[parcelIndex]
		if parcel.Weight == nil {
			continue
		}
		miss := estimate.Answer == EstimateCandidateEvaluated && estimate.Evaluation.BaseRateTableMiss()
		if miss && !last {
			continued, conflict, undecided, err := handler.continueParcel(ctx, command, declaration, parcel,
				append(append([]SellPriceSourceStep{}, prefix...), SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing}),
				index+1)
			if err != nil {
				return FormPreAcceptanceEstimatesResult{}, err
			}
			if undecided {
				return undecidedPreAcceptance(PreAcceptanceReadingCompletionUnavailable, estimate.Evaluation.PlanReference()), nil
			}
			estimates[parcelIndex] = continued
			if conflict != nil {
				return FormPreAcceptanceEstimatesResult{
					Outcome:   PreAcceptancePriceCardApplicabilityConflict,
					Conflict:  append([]domain.VersionReference(nil), conflict.Candidates...),
					Estimates: estimates,
				}, nil
			}
			if continued.Answer == EstimateCandidateEvaluated {
				formedAny = true
			}
			continue
		}
		kind := SellPriceSourceSelected
		if estimate.Evaluation.BaseRateTableMiss() {
			kind = SellPriceSourceBaseRateMissing
		} else if estimate.Answer != EstimateCandidateEvaluated || estimate.Evaluation.Status() != domain.EvaluationCompleted {
			kind = SellPriceSourceStopped
		}
		estimate.Scope = scope
		estimate.SourceTrail = append(append([]SellPriceSourceStep{}, prefix...), SellPriceSourceStep{Scope: scope, Kind: kind})
		estimates[parcelIndex] = estimate
		if estimate.Answer == EstimateCandidateEvaluated {
			formedAny = true
		}
	}
	return packageScopeResult(estimates, formedAny), nil
}

func (handler *FormPreAcceptanceEstimatesHandler) continueParcel(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	declaration ports.SubmissionDeclaration,
	parcel ports.DeclaredParcel,
	trail []SellPriceSourceStep,
	from int,
) (PreAcceptanceParcelEstimate, *ports.PriceCardInForceResolution, bool, error) {
	estimate := PreAcceptanceParcelEstimate{Parcel: parcel.Parcel}
	settlement := command.Settlement
	for stepIndex := from; stepIndex < len(command.Scopes); stepIndex++ {
		scope := command.Scopes[stepIndex]
		resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, scope,
			domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, command.At, declaration.ServiceProduct)
		if err != nil {
			return PreAcceptanceParcelEstimate{}, nil, true, nil
		}
		last := stepIndex == len(command.Scopes)-1
		switch resolution.Outcome {
		case ports.PriceCardNotConfigured:
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceNoCard})
			if last {
				estimate.SourceTrail = append([]SellPriceSourceStep{}, trail...)
			}
			continue
		case ports.PriceCardApplicabilityConflict:
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceConflict})
			estimate.SourceTrail = append([]SellPriceSourceStep{}, trail...)
			return estimate, &resolution, false, nil
		case ports.PriceCardVersionInForce:
		default:
			return PreAcceptanceParcelEstimate{}, nil, false, fmt.Errorf("pre-acceptance estimates: unhandled price card resolution %d", resolution.Outcome)
		}
		one, err := handler.deps.Estimates.Handle(ctx, FormPlanGivenEstimateEvaluationsCommand{
			Tenant:   command.Tenant,
			Plan:     resolution.Plan,
			Evidence: handler.deps.Evidence,
			Declarations: []PlanGivenEstimateDeclaration{{
				Subject:        preAcceptanceSubject(command, parcel.Parcel),
				BasisAt:        command.At,
				Weight:         *parcel.Weight,
				Dimensions:     parcel.Dimensions,
				Zone:           command.Zone,
				Route:          declaration.Route,
				Settlement:     &settlement,
				ServiceProduct: declaration.ServiceProduct,
			}},
		})
		if err != nil {
			return PreAcceptanceParcelEstimate{}, nil, false, fmt.Errorf("pre-acceptance estimates: %w", err)
		}
		switch one.Outcome {
		case PlanGivenEstimatesUndecided:
			return PreAcceptanceParcelEstimate{}, nil, true, nil
		case PlanGivenEstimatesFormed:
		default:
			return PreAcceptanceParcelEstimate{}, nil, false, fmt.Errorf("pre-acceptance estimates: unhandled plan-given outcome %d", one.Outcome)
		}
		got := one.Estimates[0]
		if got.Answer == EstimateCandidateEvaluated && got.Evaluation.BaseRateTableMiss() && !last {
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing})
			continue
		}
		kind := SellPriceSourceSelected
		if got.Evaluation.BaseRateTableMiss() {
			kind = SellPriceSourceBaseRateMissing
		} else if got.Answer != EstimateCandidateEvaluated || got.Evaluation.Status() != domain.EvaluationCompleted {
			kind = SellPriceSourceStopped
		}
		trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: kind})
		estimate.Answer = got.Answer
		estimate.Evaluation = got.Evaluation
		estimate.Missing = append([]string(nil), got.Missing...)
		estimate.Scope = scope
		estimate.SourceTrail = append([]SellPriceSourceStep{}, trail...)
		return estimate, nil, false, nil
	}
	estimate.SourceTrail = append([]SellPriceSourceStep{}, trail...)
	return estimate, nil, false, nil
}

func packageScopeResult(estimates []PreAcceptanceParcelEstimate, formedAny bool) FormPreAcceptanceEstimatesResult {
	if !formedAny {
		for _, estimate := range estimates {
			if len(estimate.SourceTrail) == 0 {
				continue
			}
			if estimate.SourceTrail[len(estimate.SourceTrail)-1].Kind == SellPriceSourceNoCard {
				return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptancePriceCardNotConfigured, Estimates: estimates}
			}
		}
	}
	result := FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceEstimatesFormed, Estimates: estimates}
	for _, estimate := range estimates {
		if estimate.Answer == EstimateCandidateInputIncomplete {
			result.Outcome = PreAcceptanceDeclarationIncomplete
		}
	}
	return result
}

func noCardPreAcceptance(declaration ports.SubmissionDeclaration, prefix []SellPriceSourceStep) FormPreAcceptanceEstimatesResult {
	estimates := make([]PreAcceptanceParcelEstimate, len(declaration.Parcels))
	for index, parcel := range declaration.Parcels {
		estimates[index] = PreAcceptanceParcelEstimate{
			Parcel: parcel.Parcel, SourceTrail: append([]SellPriceSourceStep{}, prefix...),
		}
	}
	return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptancePriceCardNotConfigured, Estimates: estimates}
}
