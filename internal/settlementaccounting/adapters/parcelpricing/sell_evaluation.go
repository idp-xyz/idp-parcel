package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrNotASellEvaluation 表示指名的评价不是 SELL·CUSTOMER_CHARGE。BUY 评价不从这条读口形成客户费用。
var ErrNotASellEvaluation = errors.New(
	"settlement accounting parcelpricing adapter: evaluation is not SELL / CUSTOMER_CHARGE")

// SellEvaluationAdapter 实现 saports.SellEvaluationView。金额换写与 BUY 读口同一套，方向不同。
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
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: %s / %s", ErrNotASellEvaluation, evaluation.Direction(), evaluation.Purpose())
	}
	outcome := sellOutcomeOf(evaluation.Status())
	adoption := saports.SellEvaluationAdoption{Evaluation: reference, Outcome: outcome}
	if outcome == saports.SellEvaluationOutcomeInvalid {
		return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: evaluation status %q", ErrUntranslatableAnswer, evaluation.Status())
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
			return saports.SellEvaluationAdoption{}, false, fmt.Errorf("%w: evaluation %s", ErrAmountPrecisionUndeclared, reference)
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
