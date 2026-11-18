package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// formAcrossSellPriceSources 按合同声明的顺序逐个范围选卡并评价（ADR-0380 决定三）。一份评价只出自一个范围。
// 只有两种落空才问下一个：这个范围没有在用的 SELL·客户费用卡；在用的卡的基础运费表没有这一格。其余结果停在
// 问到的这一格，后面的范围不问。全部落空时答最后一格：没有卡就不形成评价，基础运费表缺格则形成待判断评价。
func (handler *FormEvaluationFromRequestHandler) formAcrossSellPriceSources(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
	product domain.ServiceProductReference,
) (FormEvaluationFromRequestResult, error) {
	trail := make([]SellPriceSourceStep, 0, len(command.Scopes))
	last := len(command.Scopes) - 1
	for index, scope := range command.Scopes {
		command.Scope = scope
		resolution, err := handler.deps.PriceCards.ResolveInForce(ctx, command.Tenant, scope, command.Direction, command.Purpose, command.BasisAt, product)
		if err != nil {
			return trailed(undecidedFormation(RequestPriceCardResolutionUnavailable), append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceUndecided})), nil
		}
		switch resolution.Outcome {
		case ports.PriceCardNotConfigured:
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceNoCard})
			if index == last {
				return FormEvaluationFromRequestResult{
					Outcome:        RequestPriceCardNotConfigured,
					ServiceProduct: product,
					ScopeTrail:     trail,
				}, nil
			}
			continue
		case ports.PriceCardApplicabilityConflict:
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceConflict})
			return FormEvaluationFromRequestResult{
				Outcome:        RequestPriceCardApplicabilityConflict,
				Candidates:     append([]domain.VersionReference(nil), resolution.Candidates...),
				ServiceProduct: product,
				ScopeTrail:     trail,
			}, nil
		case ports.PriceCardVersionInForce:
		default:
			return FormEvaluationFromRequestResult{}, fmt.Errorf("%w: price card in-force outcome %d", ErrUnexpectedInForceOutcome, resolution.Outcome)
		}

		input, stopped, halt := handler.resolveInput(ctx, command)
		if halt {
			kind := SellPriceSourceInputUnavailable
			if stopped.Outcome == RequestEvaluationUndecided {
				kind = SellPriceSourceUndecided
			}
			return trailed(stopped, append(trail, SellPriceSourceStep{Scope: scope, Kind: kind})), nil
		}
		if product.Declared() {
			if input, err = input.WithServiceProduct(product); err != nil {
				return FormEvaluationFromRequestResult{}, fmt.Errorf("record service product %s on the pricing input: %w", product, err)
			}
		}
		probed, undecided, err := handler.probeResolvedScope(ctx, resolution.Plan, input, command.Evidence)
		if err != nil {
			return FormEvaluationFromRequestResult{}, err
		}
		if undecided {
			return trailed(undecidedFormation(RequestEvaluationFormationUndecided),
				append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceUndecided})), nil
		}
		if probed.BaseRateTableMiss() && index != last {
			trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing})
			continue
		}
		kind := SellPriceSourceSelected
		if probed.BaseRateTableMiss() {
			kind = SellPriceSourceBaseRateMissing
		} else if probed.Status() != domain.EvaluationCompleted {
			kind = SellPriceSourceStopped
		}
		trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: kind})
		formed, err := handler.formOneScope(ctx, command, product, sellPriceSourceNotes(trail))
		if err != nil {
			return formed, err
		}
		formed.ScopeTrail = append([]SellPriceSourceStep(nil), trail...)
		return formed, nil
	}
	return FormEvaluationFromRequestResult{}, fmt.Errorf("form evaluation across sell price sources: the list was empty")
}

func (command FormEvaluationFromRequestCommand) sellCustomerCharge() bool {
	return command.Direction == domain.PricingDirectionSell && command.Purpose == domain.PricingPurposeCustomerCharge
}

func (command FormEvaluationFromRequestCommand) walksSellPriceSources() bool {
	return len(command.Scopes) > 1 && command.sellCustomerCharge()
}

// probeResolvedScope 先按这张卡补齐目录与序列读数，再算一份不入册的评价，只为看它是不是停在基础运费表缺格上。
// 不补读数时，绑了分区目录的卡在看运费表之前就因没有分区落待判断，没谈到的线路不会去问下一个范围；目录解不出分区
// 仍是待判断，停在这一格（ADR-0380 决定三）。
//
// 补读数时依赖故障答 undecided，不上抛：在用与否未知，不能当成这一格的结论，也不能当成硬失败——正式形成那一步
// 读不回时答的是未决，消费侧按未决重投。半套装配是装配错误，照 EvaluatePricingHandler.Handle 上抛。
func (handler *FormEvaluationFromRequestHandler) probeResolvedScope(
	ctx context.Context,
	plan domain.PricingPlanVersion,
	input domain.PricingInputSnapshot,
	evidence domain.EvidenceKind,
) (domain.PricingEvaluation, bool, error) {
	completion := handler.deps.Evaluate.completion()
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

func sellPriceSourceNotes(trail []SellPriceSourceStep) []string {
	notes := make([]string, 0, len(trail))
	for _, step := range trail {
		notes = append(notes, fmt.Sprintf("sell price source %s: %s", step.Scope.String(), step.Kind))
	}
	return notes
}

func trailed(result FormEvaluationFromRequestResult, trail []SellPriceSourceStep) FormEvaluationFromRequestResult {
	result.ScopeTrail = trail
	return result
}
