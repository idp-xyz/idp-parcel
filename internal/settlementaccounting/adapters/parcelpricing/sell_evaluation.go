package parcelpricing

import (
	"context"
	"fmt"
	"strings"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SellEvaluationAdapter 实现 saports.SellEvaluationView。金额换写与 BUY 读口同一套，方向不同。
//
// 卡没声明金额取整时不报错，交回 saports.SellEvaluationAmountUndeclared 那一格（理由在那一格的头注），连同回指与费用
// 代码；BUY 读口同此。
type SellEvaluationAdapter struct {
	evaluations EvaluationSource
}

func NewSellEvaluationAdapter(evaluations EvaluationSource) (*SellEvaluationAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: evaluation source is nil")
	}
	return &SellEvaluationAdapter{evaluations: evaluations}, nil
}

func (adapter *SellEvaluationAdapter) LoadSellEvaluation(
	ctx context.Context,
	tenant sadomain.TenantID,
	reference sadomain.SellEvaluationReference,
) (saports.SellEvaluationAdoption, bool, error) {
	id, err := ppdomain.NewEvaluationID(reference.String())
	if err != nil {
		return saports.SellEvaluationAdoption{}, false, nil
	}
	evaluation, found, err := adapter.evaluations.FindByID(ctx, id)
	if err != nil {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("load sell evaluation: %w", err)
	}
	if !found || evaluation.Input().TenantID().String() != tenant.String() {
		return saports.SellEvaluationAdoption{}, false, nil
	}
	if evaluation.Direction() != ppdomain.PricingDirectionSell || evaluation.Purpose() != ppdomain.PricingPurposeCustomerCharge {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: %s / %s", saports.ErrNotASellEvaluation, evaluation.Direction(), evaluation.Purpose())
	}
	outcome := sellOutcomeOf(evaluation.Status())
	adoption := saports.SellEvaluationAdoption{
		Evaluation:   reference,
		Outcome:      outcome,
		PricingScope: evaluation.Input().Scope().String(),
		SourceTrail:  sellPriceSourceSteps(evaluation.Explanation()),
	}
	if outcome == saports.SellEvaluationOutcomeInvalid {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: evaluation status %q", ErrUntranslatableAnswer, evaluation.Status())
	}
	if outcome == saports.SellEvaluationPending {
		adoption.PendingCause = pendingCauseOf(evaluation.Issues())
	}
	if requested, ok := evaluation.RequestReference(); ok {
		request, err := sadomain.NewSellEvaluationRequestID(requested.String())
		if err != nil {
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: request reference: %v", ErrUntranslatableAnswer, err)
		}
		adoption.Request = request
	}
	for _, line := range evaluation.ChargeLines() {
		code, err := sadomain.NewPricingChargeCode(line.Code().String())
		if err != nil {
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: charge line %s: %v", ErrUntranslatableAnswer, line.ID(), err)
		}
		adoption.ChargeCodes = append(adoption.ChargeCodes, code)
	}
	if outcome != saports.SellEvaluationCompleted {
		return adoption, true, nil
	}
	total, ok := evaluation.Total()
	if !ok {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: completed evaluation without a total", ErrUntranslatableAnswer)
	}
	for _, issue := range evaluation.Issues() {
		if issue.Code() == amountPrecisionUndeclared {
			adoption.Outcome = saports.SellEvaluationAmountUndeclared
			return adoption, true, nil
		}
	}
	steps := evaluation.AmountRounding()
	totalDigits, declared := incrementDigits(steps, ppdomain.AmountRoundingTotal)
	if !declared {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: completed evaluation carries no TOTAL rounding step", ErrUntranslatableAnswer)
	}
	settlementCurrency, err := sadomain.NewCurrencyCode(total.Currency().String())
	if err != nil {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: settlement currency: %v", ErrUntranslatableAnswer, err)
	}
	settlementMinor, err := minorUnitsOf(total.Amount().String(), totalDigits)
	if err != nil {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: total %s: %v", ErrUntranslatableAnswer, total.Amount(), err)
	}
	adoption.SettlementCurrency, adoption.SettlementMinor = settlementCurrency, settlementMinor
	adoption.OriginalCurrency, adoption.OriginalMinor = settlementCurrency, settlementMinor
	if conversion, converted := evaluation.ConversionStep(); converted {
		original := conversion.Original()
		originalCurrency, err := sadomain.NewCurrencyCode(original.Currency().String())
		if err != nil {
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: original currency: %v", ErrUntranslatableAnswer, err)
		}
		originalDigits, declared := incrementDigits(steps, ppdomain.AmountRoundingPerLine)
		if !declared {
			originalDigits = fractionDigits(original.Amount().String())
		}
		originalMinor, err := minorUnitsOf(original.Amount().String(), originalDigits)
		if err != nil {
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: original amount %s: %v", ErrUntranslatableAnswer, original.Amount(), err)
		}
		stepReference, err := sadomain.NewConversionStepReference(conversion.SeriesReference().ID() + "@" + conversion.SeriesReference().Version())
		if err != nil {
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: conversion step: %v", ErrUntranslatableAnswer, err)
		}
		adoption.OriginalCurrency, adoption.OriginalMinor, adoption.Conversion = originalCurrency, originalMinor, stepReference
	}
	return adoption, true, nil
}

// sellPriceSourceSteps 从评价解释里摘出按有序列表问过的每一格。解释行由计价写成
// 「sell price source <范围>: <这一格>」，范围原样、不去空白。没有这种行就是只带一个范围。
func sellPriceSourceSteps(explanation []string) []saports.SellPriceSourceStep {
	const prefix = "sell price source "
	var steps []saports.SellPriceSourceStep
	for _, line := range explanation {
		rest, ok := strings.CutPrefix(line, prefix)
		if !ok {
			continue
		}
		scope, kind, found := strings.Cut(rest, ": ")
		if !found || scope == "" || kind == "" {
			continue
		}
		steps = append(steps, saports.SellPriceSourceStep{Scope: scope, Kind: kind})
	}
	return steps
}

func sellOutcomeOf(status ppdomain.EvaluationStatus) saports.SellEvaluationOutcome {
	switch status {
	case ppdomain.EvaluationCompleted:
		return saports.SellEvaluationCompleted
	case ppdomain.EvaluationPending:
		return saports.SellEvaluationPending
	case ppdomain.EvaluationUnratable:
		return saports.SellEvaluationUnratable
	case ppdomain.EvaluationConflict:
		return saports.SellEvaluationConflict
	case ppdomain.EvaluationFailed:
		return saports.SellEvaluationNotFormed
	default:
		return saports.SellEvaluationOutcomeInvalid
	}
}

var _ saports.SellEvaluationView = (*SellEvaluationAdapter)(nil)
