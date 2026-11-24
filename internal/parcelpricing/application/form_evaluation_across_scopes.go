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

		// 请求主体与这一格的卡不是同一种聚合时，按卡自己的主体探基础运费表。整票都缺格才问下一格；
		// 有一件命中就停在这一格，正式形成仍用请求自己的主体，既有的聚合方式不符照旧答冲突。
		// 最后一格没有下一格可问，不探，直接走原来的形成。
		if index != last {
			skipped, undecided, err := handler.aggregationMissSkipsScope(ctx, command, resolution, product)
			if err != nil {
				return FormEvaluationFromRequestResult{}, err
			}
			if undecided {
				return trailed(undecidedFormation(RequestEvaluationFormationUndecided),
					append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceUndecided})), nil
			}
			if skipped {
				trail = append(trail, SellPriceSourceStep{Scope: scope, Kind: SellPriceSourceBaseRateMissing})
				continue
			}
		}

		input, stopped, halt := handler.resolveInput(ctx, command, resolution.Plan.Aggregation())
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
// aggregationMissSkipsScope 只在请求主体与卡的聚合方式对不上时探路。对得上时交回不跳过，原来的探针照旧。
// 委托级请求撞上逐包裹卡：逐件探，全部成员都缺格才跳。包裹级请求撞上逐委托卡：按包裹所属委托的当前有效成员
// 探整票，整票缺格才跳。依赖读不回是未决，不是缺格。
func (handler *FormEvaluationFromRequestHandler) aggregationMissSkipsScope(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
	resolution ports.PriceCardInForceResolution,
	product domain.ServiceProductReference,
) (bool, bool, error) {
	switch {
	case command.Sources.ShipmentReferenced() && resolution.Plan.Aggregation() == domain.AggregationPerPackage:
		return handler.packageCardMissesEveryMember(ctx, command, resolution.Plan, product)
	case command.Sources.ParcelReferenced() && resolution.Plan.Aggregation() == domain.AggregationPerShipment:
		return handler.shipmentCardMissesTheParcel(ctx, command, resolution.Plan, product)
	default:
		return false, false, nil
	}
}

func (handler *FormEvaluationFromRequestHandler) packageCardMissesEveryMember(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, bool, error) {
	if len(command.Sources.Members) == 0 {
		return false, false, nil
	}
	for _, member := range command.Sources.Members {
		piece := command
		piece.Sources = ports.EligibleSourceReferences{Parcel: member}
		miss, undecided, err := handler.probeCommand(ctx, piece, plan, product, domain.AggregationPerPackage)
		if err != nil || undecided || !miss {
			return false, undecided, err
		}
	}
	return true, false, nil
}

func (handler *FormEvaluationFromRequestHandler) shipmentCardMissesTheParcel(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
) (bool, bool, error) {
	if handler.deps.ShipmentMembers == nil {
		return false, false, nil
	}
	picture, found, err := handler.deps.ShipmentMembers.LoadShipmentMembers(ctx, command.Tenant, command.Sources.Parcel)
	if err != nil {
		return false, true, nil
	}
	if !found || picture.Shipment == "" || len(picture.Members) == 0 {
		return false, false, nil
	}
	shipment := command
	shipment.Sources = ports.EligibleSourceReferences{Shipment: picture.Shipment, Members: append([]string(nil), picture.Members...)}
	return handler.probeCommand(ctx, shipment, plan, product, domain.AggregationPerShipment)
}

// probeCommand 造这一格的输入并做不入册探评。miss 只在评价已经形成且基础运费表缺格时为真。
// 输入不可得不是缺格：停在这一格，让正式形成去说缺什么。依赖读不回是未决。
func (handler *FormEvaluationFromRequestHandler) probeCommand(
	ctx context.Context,
	command FormEvaluationFromRequestCommand,
	plan domain.PricingPlanVersion,
	product domain.ServiceProductReference,
	aggregation domain.AggregationMode,
) (bool, bool, error) {
	input, stopped, halt := handler.resolveInput(ctx, command, aggregation)
	if halt {
		return false, stopped.Outcome == RequestEvaluationUndecided, nil
	}
	if product.Declared() {
		var err error
		if input, err = input.WithServiceProduct(product); err != nil {
			return false, false, fmt.Errorf("record service product %s on the pricing input: %w", product, err)
		}
	}
	probed, undecided, err := handler.probeResolvedScope(ctx, plan, input, command.Evidence)
	if err != nil || undecided {
		return false, undecided, err
	}
	return probed.BaseRateTableMiss(), false, nil
}

func (handler *FormEvaluationFromRequestHandler) probeResolvedScope(
	ctx context.Context,
	plan domain.PricingPlanVersion,
	input domain.PricingInputSnapshot,
	evidence domain.EvidenceKind,
) (domain.PricingEvaluation, bool, error) {
	return probeResolvedPlan(ctx, handler.deps.Evaluate, plan, input, evidence)
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
