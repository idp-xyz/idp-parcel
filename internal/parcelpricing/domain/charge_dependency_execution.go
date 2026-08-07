package domain

import "fmt"

// dependencyBasis resolves the amount a percent charge is charged on.
//
// The basis is a sum over charge lines the card names, so a percent charge can
// only be computed once every line it reads already carries an amount. That is
// why the resolution is ordered by dependency rather than by declaration: two
// percent charges declared in one order may have to be computed in the other.
type dependencyBasis struct {
	dependencies map[string]ChargeDependency
	amounts      map[string]Decimal
	currency     Currency
}

func newDependencyBasis(structures PricingPlanStructures, currency Currency) dependencyBasis {
	indexed := make(map[string]ChargeDependency, len(structures.dependencies))
	for _, dependency := range structures.dependencies {
		indexed[dependency.id] = dependency
	}
	return dependencyBasis{dependencies: indexed, amounts: make(map[string]Decimal), currency: currency}
}

// record adds a settled charge line to the pool later bases are summed from.
// Deducted lines reduce a basis for the same reason they reduce a total: the
// card's "all other charges" means the net of them.
func (basis dependencyBasis) record(code ChargeCode, effect ChargeEffect, amount Money) error {
	existing, seen := basis.amounts[code.String()]
	if !seen {
		existing = NewDecimalFromInt64(0)
	}
	var updated Decimal
	var err error
	switch effect {
	case ChargeEffectAdd:
		updated, err = existing.Add(amount.amount)
	case ChargeEffectDeduct:
		updated, err = existing.Sub(amount.amount)
	default:
		return ErrInvalidChargeLine
	}
	if err != nil {
		return err
	}
	basis.amounts[code.String()] = updated
	return nil
}

// sum totals the codes a dependency composes. A code the evaluation never
// produced contributes nothing rather than erroring: the card may name a charge
// that simply did not apply to this parcel.
func (basis dependencyBasis) sum(dependency ChargeDependency) (Money, error) {
	total := NewDecimalFromInt64(0)
	var err error
	switch dependency.composition {
	case ChargeBasisAllCharges:
		excluded := codeSet(dependency.excludes)
		// The dependent charge is never part of its own basis; including it
		// would make the amount depend on itself.
		excluded[dependency.dependent.String()] = struct{}{}
		for code, amount := range basis.amounts {
			if _, skip := excluded[code]; skip {
				continue
			}
			if total, err = total.Add(amount); err != nil {
				return Money{}, err
			}
		}
	case ChargeBasisListedCharges:
		excluded := codeSet(dependency.excludes)
		for _, code := range dependency.includes {
			if _, skip := excluded[code.String()]; skip {
				continue
			}
			amount, present := basis.amounts[code.String()]
			if !present {
				continue
			}
			if total, err = total.Add(amount); err != nil {
				return Money{}, err
			}
		}
	default:
		return Money{}, ErrInvalidChargeDependency
	}
	return NewMoney(total, basis.currency)
}

// percentShare divides a basis-times-percentage product by 100. Dividing by a
// power of ten is a shift of the decimal point, so the result is exact and no
// precision has to be declared — which matters because the card states a fuel
// rate without stating how to round the product of applying it.
func percentShare(product Decimal) (Decimal, error) {
	if !product.valid() {
		return Decimal{}, ErrInvalidDecimal
	}
	if product.IsZero() {
		return product, nil
	}
	shifted := Decimal{coefficient: product.coefficient, scale: product.scale + 2}
	if !shifted.valid() {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}
	return shifted, nil
}

// share values one percent-of-basis calculation against the pool collected so
// far.
func (basis dependencyBasis) share(calculation SurchargeCalculation) (Money, error) {
	percentage, dependencyID, ok := calculation.PercentOfBasis()
	if !ok {
		return Money{}, ErrInvalidSurchargeRule
	}
	dependency, declared := basis.dependencies[dependencyID]
	if !declared {
		return Money{}, fmt.Errorf("%w: undeclared basis %s", ErrInvalidChargeDependency, dependencyID)
	}
	amount, err := basis.sum(dependency)
	if err != nil {
		return Money{}, err
	}
	product, err := amount.amount.Mul(percentage)
	if err != nil {
		return Money{}, err
	}
	hundredths, err := percentShare(product)
	if err != nil {
		return Money{}, err
	}
	return NewMoney(hundredths, basis.currency)
}

func codeSet(codes []ChargeCode) map[string]struct{} {
	set := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		set[code.String()] = struct{}{}
	}
	return set
}

// orderPercentOutcomes returns the matched percent charges in an order where
// every charge is computed after the charges its basis reads.
//
// A basis that reaches back to the charge it feeds has no fixed point, and
// choosing an evaluation order would invent one, so a cycle is a conflict for a
// human to resolve rather than a number this package picks.
func orderPercentOutcomes(pending []*surchargeOutcome, basis dependencyBasis) ([]*surchargeOutcome, error) {
	producedBy := make(map[string]*surchargeOutcome, len(pending))
	for _, outcome := range pending {
		producedBy[outcome.rule.chargeCode.String()] = outcome
	}

	ordered := make([]*surchargeOutcome, 0, len(pending))
	const (
		visiting = 1
		visited  = 2
	)
	state := make(map[string]int, len(pending))

	var visit func(outcome *surchargeOutcome) error
	visit = func(outcome *surchargeOutcome) error {
		code := outcome.rule.chargeCode.String()
		switch state[code] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("%w: charge %s takes part in a circular basis", ErrRateTableConflict, code)
		}
		state[code] = visiting
		// A greater-of may read more than one basis, so the codes it depends on
		// are the union over every basis its operands name.
		for _, dependencyID := range outcome.rule.calculation.basisDependencyIDs() {
			dependency, declared := basis.dependencies[dependencyID]
			if !declared {
				return fmt.Errorf("%w: %s names undeclared basis %s", ErrInvalidChargeDependency, outcome.rule.id, dependencyID)
			}
			for _, required := range basisCodes(dependency, basis, producedBy) {
				if next, produced := producedBy[required]; produced && next != outcome {
					if err := visit(next); err != nil {
						return err
					}
				}
			}
		}
		state[code] = visited
		ordered = append(ordered, outcome)
		return nil
	}

	for _, outcome := range pending {
		if err := visit(outcome); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// basisCodes names the charge codes a dependency reads. An all-charges basis
// reads everything already recorded **and** every percent charge still pending,
// because those join the pool before it is summed; leaving the pending ones out
// would order the resolution as if they did not exist and read a zero for them.
func basisCodes(dependency ChargeDependency, basis dependencyBasis, pending map[string]*surchargeOutcome) []string {
	excluded := codeSet(dependency.excludes)
	excluded[dependency.dependent.String()] = struct{}{}
	if dependency.composition == ChargeBasisListedCharges {
		codes := make([]string, 0, len(dependency.includes))
		for _, code := range dependency.includes {
			if _, skip := excluded[code.String()]; skip {
				continue
			}
			codes = append(codes, code.String())
		}
		return codes
	}
	codes := make([]string, 0, len(basis.amounts)+len(pending))
	for code := range basis.amounts {
		if _, skip := excluded[code]; skip {
			continue
		}
		codes = append(codes, code)
	}
	for code := range pending {
		if _, skip := excluded[code]; skip {
			continue
		}
		codes = append(codes, code)
	}
	return codes
}
