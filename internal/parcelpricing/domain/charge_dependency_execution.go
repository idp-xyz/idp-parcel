package domain

import "fmt"

// dependencyBasis 解析一条百分比费用所依据的基数金额。
//
// 基数是对卡指名的那些费用行求和，所以只有当它读取的每一行都已经有金额时，百分比费用
// 才算得出来。这就是解析按依赖顺序而不是按声明顺序进行的原因：按一种顺序声明的两条
// 百分比费用，可能必须按相反的顺序计算。
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

// record 把一条已定值的费用行加入池中，后续基数从这个池里求和。抵减方向的行会减小
// 基数，理由与它减小合计的理由相同：卡上的「本票全部其他费用」指的是它们的净额。
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

// sum 对一条费用依赖所构成的各费用代码求和。本次评价从未产生过的代码贡献零而不是报错：
// 卡完全可以指名一项对这件包裹根本不适用的费用。
func (basis dependencyBasis) sum(dependency ChargeDependency) (Money, error) {
	total := NewDecimalFromInt64(0)
	var err error
	switch dependency.composition {
	case ChargeBasisAllCharges:
		excluded := codeSet(dependency.excludes)
		// 被依赖的费用行本身永不进入自己的基数；放进去会让金额依赖它自己。
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

// percentShare 把「基数乘百分比」的乘积除以 100。除以十的幂只是小数点移位，因此结果
// 精确、无需声明精度——这一点要紧，因为卡只给出燃油费率，并没有说施加它之后的乘积
// 该怎么取整。
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

// share 拿到目前为止收集的费用池，为一次`按基数百分比`计算定值。
func (basis dependencyBasis) share(calculation SurchargeCalculation, series map[ReferenceSeriesKind]ReferenceSeriesValue) (Money, error) {
	if calculation.method != ChargeMethodPercentOfBasis || calculation.basis == "" {
		return Money{}, ErrInvalidSurchargeRule
	}
	dependencyID := calculation.basis
	var percentage Decimal
	switch {
	case calculation.seriesKind != nil:
		reading, resolved := series[*calculation.seriesKind]
		if !resolved {
			return Money{}, fmt.Errorf("%w: %s", ErrMissingReferenceSeriesValue, *calculation.seriesKind)
		}
		rate, rateErr := calculation.effectiveRate(reading)
		if rateErr != nil {
			return Money{}, rateErr
		}
		percentage = rate
	case calculation.percentage != nil:
		percentage = *calculation.percentage
	default:
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

// orderPercentOutcomes 按「每条费用都排在其基数所读取的费用之后」的顺序，返回已命中的
// 百分比费用。
//
// 一个反向牵回到它所供养的那条费用的基数没有不动点，而选定一个求值顺序等于替它发明一个，
// 所以存在环时形成`冲突`、交给人裁决，而不是由本包挑一个数出来。
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
		// 取较大值可能读取不止一个基数，所以它依赖的费用代码，是其各操作数所指名的
		// 每一个基数的并集。
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

// basisCodes 指出一条费用依赖读取哪些费用代码。「本票全部其他费用」型基数读取的是已经
// 记录在池中的全部费用**以及**所有尚未定值的百分比费用，因为后者会在求和之前加入池；
// 漏掉待定的那些，会让解析顺序当它们不存在，从而给它们读到零。
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
