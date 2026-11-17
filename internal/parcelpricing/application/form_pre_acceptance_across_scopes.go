package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// estimateAcrossSellPriceSources 让每一件声明包裹各自按有序范围列表问（ADR-0380 决定三）。选卡按范围只问一次，
// 件与件的差别在基础运费表有没有这一格。没有在用卡或基础运费表缺格才问下一个；适用冲突停住，后面的范围不问。
func (handler *FormPreAcceptanceEstimatesHandler) estimateAcrossSellPriceSources(
	ctx context.Context,
	command FormPreAcceptanceEstimatesCommand,
	declaration ports.SubmissionDeclaration,
) (FormPreAcceptanceEstimatesResult, error) {
	resolved := make([]ports.PriceCardInForceResolution, len(command.Scopes))
	for index, scope := range command.Scopes {
		resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, scope,
			domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, command.At, declaration.ServiceProduct)
		if err != nil {
			return undecidedPreAcceptance(PreAcceptancePriceCardResolutionUnavailable, domain.VersionReference{}), nil
		}
		switch resolution.Outcome {
		case ports.PriceCardNotConfigured, ports.PriceCardApplicabilityConflict, ports.PriceCardVersionInForce:
		default:
			return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance estimates: unhandled price card resolution %d", resolution.Outcome)
		}
		resolved[index] = resolution
	}

	estimates := make([]PreAcceptanceParcelEstimate, len(declaration.Parcels))
	settlement := command.Settlement
	formedAny := false
	for index, parcel := range declaration.Parcels {
		estimate := PreAcceptanceParcelEstimate{Parcel: parcel.Parcel}
		if parcel.Weight == nil {
			estimate.Answer = EstimateCandidateInputIncomplete
			estimate.Missing = []string{EstimateMissingMeasurement}
			estimates[index] = estimate
			continue
		}
		var trail []SellPriceSourceStep
		for stepIndex, scope := range command.Scopes {
			resolution := resolved[stepIndex]
			last := stepIndex == len(command.Scopes)-1
			switch resolution.Outcome {
			case ports.PriceCardNotConfigured:
				trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceNoCard})
				if last {
					estimate.SourceTrail = trail
				}
				continue
			case ports.PriceCardApplicabilityConflict:
				trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceConflict})
				estimate.SourceTrail = trail
				estimates[index] = estimate
				return FormPreAcceptanceEstimatesResult{
					Outcome:   PreAcceptancePriceCardApplicabilityConflict,
					Conflict:  append([]domain.VersionReference(nil), resolution.Candidates...),
					Estimates: estimates,
				}, nil
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
				return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance estimates: %w", err)
			}
			switch one.Outcome {
			case PlanGivenEstimatesUndecided:
				return undecidedPreAcceptance(PreAcceptanceReadingCompletionUnavailable, resolution.Plan.Reference()), nil
			case PlanGivenEstimatesFormed:
			default:
				return FormPreAcceptanceEstimatesResult{}, fmt.Errorf("pre-acceptance estimates: unhandled plan-given outcome %d", one.Outcome)
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
			estimate.SourceTrail = trail
			if got.Answer == EstimateCandidateEvaluated {
				formedAny = true
			}
			break
		}
		estimates[index] = estimate
	}

	if !formedAny {
		for _, estimate := range estimates {
			if len(estimate.SourceTrail) == 0 {
				continue
			}
			if estimate.SourceTrail[len(estimate.SourceTrail)-1].Kind == SellPriceSourceNoCard {
				return FormPreAcceptanceEstimatesResult{Outcome: PreAcceptancePriceCardNotConfigured, Estimates: estimates}, nil
			}
		}
	}
	result := FormPreAcceptanceEstimatesResult{Outcome: PreAcceptanceEstimatesFormed, Estimates: estimates}
	for _, estimate := range estimates {
		// 只把缺测量或邮编、分区算成申报不全。问到最后一格仍没有在用卡的件，答案在它的来源列表上，
		// 不把整份改口成缺申报。
		if estimate.Answer == EstimateCandidateInputIncomplete {
			result.Outcome = PreAcceptanceDeclarationIncomplete
		}
	}
	return result, nil
}
