package domain

import (
	"fmt"
	"strings"
)

// FirstContinueRate prices a first weight and then charges whole continuation
// steps. It is the shape a commercial express agent quotes; a bracket ladder
// cannot express it, because the amount above the first weight is derived from
// how many steps the weight reaches rather than looked up. See `PA-PP-01`.
type FirstContinueRate struct {
	id          RateEntryID
	zone        string
	firstWeight Weight
	firstAmount Money
	step        Weight
	stepAmount  Money
}

func NewFirstContinueRate(
	id RateEntryID,
	zone string,
	firstWeight Weight,
	firstAmount Money,
	step Weight,
	stepAmount Money,
) (FirstContinueRate, error) {
	rate := FirstContinueRate{id: id, zone: zone, firstWeight: firstWeight, firstAmount: firstAmount, step: step, stepAmount: stepAmount}
	if !rate.valid() {
		return FirstContinueRate{}, ErrInvalidRateEntry
	}
	if firstWeight.unit != step.unit {
		return FirstContinueRate{}, ErrWeightUnitMismatch
	}
	if firstAmount.currency != stepAmount.currency {
		return FirstContinueRate{}, ErrCurrencyMismatch
	}
	return rate, nil
}

func (rate FirstContinueRate) ID() RateEntryID     { return rate.id }
func (rate FirstContinueRate) Zone() string        { return rate.zone }
func (rate FirstContinueRate) FirstWeight() Weight { return rate.firstWeight }
func (rate FirstContinueRate) FirstAmount() Money  { return rate.firstAmount }
func (rate FirstContinueRate) Step() Weight        { return rate.step }
func (rate FirstContinueRate) StepAmount() Money   { return rate.stepAmount }

func (rate FirstContinueRate) valid() bool {
	return rate.id.valid() && strings.TrimSpace(rate.zone) == rate.zone && strings.TrimSpace(rate.zone) != "" &&
		rate.firstWeight.valid() && rate.firstWeight.value.Sign() > 0 &&
		rate.step.valid() && rate.step.value.Sign() > 0 &&
		rate.firstAmount.valid() && rate.stepAmount.valid() &&
		rate.firstWeight.unit == rate.step.unit && rate.firstAmount.currency == rate.stepAmount.currency
}

// price charges the first weight, then one whole step for every started step
// above it. A partial step is a whole step: that is what "续重" means on a card.
func (rate FirstContinueRate) price(weight Weight) (Money, string, error) {
	if weight.unit != rate.firstWeight.unit {
		return Money{}, "", ErrWeightUnitMismatch
	}
	if weight.value.Cmp(rate.firstWeight.value) <= 0 {
		return rate.firstAmount, fmt.Sprintf("first weight %s %s covers %s %s at %s",
			rate.firstWeight.value.String(), rate.firstWeight.unit, weight.value.String(), weight.unit, rate.firstAmount.amount.String()), nil
	}
	excess, err := weight.value.Sub(rate.firstWeight.value)
	if err != nil {
		return Money{}, "", err
	}
	steps, err := excess.DivRoundToIncrement(rate.step.value, NewDecimalFromInt64(1), RoundingCeiling)
	if err != nil {
		return Money{}, "", err
	}
	continuation, err := steps.Mul(rate.stepAmount.amount)
	if err != nil {
		return Money{}, "", err
	}
	total, err := rate.firstAmount.amount.Add(continuation)
	if err != nil {
		return Money{}, "", err
	}
	amount, err := NewMoney(total, rate.firstAmount.currency)
	if err != nil {
		return Money{}, "", err
	}
	explanation := fmt.Sprintf("first weight %s %s at %s plus %s continuation step(s) of %s %s at %s each",
		rate.firstWeight.value.String(), rate.firstWeight.unit, rate.firstAmount.amount.String(),
		steps.String(), rate.step.value.String(), rate.step.unit, rate.stepAmount.amount.String())
	return amount, explanation, nil
}

// UnitPriceRate quotes one price per unit of chargeable weight with no
// brackets. Economy lines and postal small packets price this way. See
// `PA-PP-01`.
type UnitPriceRate struct {
	id            RateEntryID
	zone          string
	amountPerUnit Money
}

func NewUnitPriceRate(id RateEntryID, zone string, amountPerUnit Money) (UnitPriceRate, error) {
	rate := UnitPriceRate{id: id, zone: zone, amountPerUnit: amountPerUnit}
	if !rate.valid() {
		return UnitPriceRate{}, ErrInvalidRateEntry
	}
	return rate, nil
}

func (rate UnitPriceRate) ID() RateEntryID      { return rate.id }
func (rate UnitPriceRate) Zone() string         { return rate.zone }
func (rate UnitPriceRate) AmountPerUnit() Money { return rate.amountPerUnit }

func (rate UnitPriceRate) valid() bool {
	return rate.id.valid() && strings.TrimSpace(rate.zone) == rate.zone && strings.TrimSpace(rate.zone) != "" &&
		rate.amountPerUnit.valid() && rate.amountPerUnit.amount.Sign() > 0
}

func (rate UnitPriceRate) price(weight Weight) (Money, string, error) {
	product, err := weight.value.Mul(rate.amountPerUnit.amount)
	if err != nil {
		return Money{}, "", err
	}
	amount, err := NewMoney(product, rate.amountPerUnit.currency)
	if err != nil {
		return Money{}, "", err
	}
	explanation := fmt.Sprintf("%s %s at %s per %s", weight.value.String(), weight.unit, rate.amountPerUnit.amount.String(), weight.unit)
	return amount, explanation, nil
}

// RateSelection is what a lookup produces. Two of the three families derive
// their amount rather than matching a row, so a lookup cannot return a bracket:
// it returns the amount, the rate that produced it, and how it was reached.
type RateSelection struct {
	family      RateTableFamily
	id          RateEntryID
	zone        string
	amount      Money
	explanation string
}

func (selection RateSelection) Family() RateTableFamily { return selection.family }
func (selection RateSelection) ID() RateEntryID         { return selection.id }
func (selection RateSelection) Zone() string            { return selection.zone }
func (selection RateSelection) Amount() Money           { return selection.amount }
func (selection RateSelection) Explanation() string     { return selection.explanation }

func (selection RateSelection) valid() bool {
	return selection.family.valid() && selection.id.valid() &&
		strings.TrimSpace(selection.zone) == selection.zone && strings.TrimSpace(selection.zone) != "" &&
		selection.amount.valid() && strings.TrimSpace(selection.explanation) != ""
}
