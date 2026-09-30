package domain

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var (
	// ErrInvalidAmountGrammar 说明文法参数或主张不是这套封闭文法能算的输入。
	ErrInvalidAmountGrammar = errors.New("settlement accounting: invalid amount grammar")
	// ErrAmountGrammarOverflow 说明比例折算的商装不进一个金额。不绕回，也不改成未配置。
	ErrAmountGrammarOverflow = errors.New("settlement accounting: amount grammar overflow")
)

// AmountGrammarSubject 标明这三项数值挂在哪一种已经采用的引用上。赔付、退款和应追偿
// 挂金额规则版本；代垫回收挂合同责任。两处不共用一行。
type AmountGrammarSubject uint8

const (
	AmountGrammarSubjectInvalid AmountGrammarSubject = iota
	AmountGrammarClaimRule
	AmountGrammarRecoveryContract
)

func (subject AmountGrammarSubject) valid() bool {
	return subject == AmountGrammarClaimRule || subject == AmountGrammarRecoveryContract
}

func (subject AmountGrammarSubject) String() string {
	switch subject {
	case AmountGrammarClaimRule:
		return "CLAIM_RULE"
	case AmountGrammarRecoveryContract:
		return "RECOVERY_CONTRACT"
	default:
		return ""
	}
}

func AmountGrammarSubjectFromName(name string) (AmountGrammarSubject, error) {
	switch name {
	case "CLAIM_RULE":
		return AmountGrammarClaimRule, nil
	case "RECOVERY_CONTRACT":
		return AmountGrammarRecoveryContract, nil
	default:
		return AmountGrammarSubjectInvalid, fmt.Errorf("%w: amount grammar subject", ErrInvalidAmountGrammar)
	}
}

// AmountGrammar 是限额、比例、免赔三项租户取值。零值不是一份登记：全零（限额 0、
// 比例 0、免赔 0）是租户可以登记的取值，和「没登记」不是一回事，所以构造门留一个记号。
type AmountGrammar struct {
	limitMinor       int64
	ratioBasisPoints int64
	deductibleMinor  int64
	ok               bool
}

// NewAmountGrammar 收下三项取值。比例是万分比，封在 0 到 10000：这是文法的闭合范围，
// 不是替租户填的 100%。超出范围拒，不夹到边界。
func NewAmountGrammar(limitMinor, ratioBasisPoints, deductibleMinor int64) (AmountGrammar, error) {
	if limitMinor < 0 || deductibleMinor < 0 || ratioBasisPoints < 0 || ratioBasisPoints > 10_000 {
		return AmountGrammar{}, fmt.Errorf("%w: limit, ratio or deductible", ErrInvalidAmountGrammar)
	}
	return AmountGrammar{
		limitMinor:       limitMinor,
		ratioBasisPoints: ratioBasisPoints,
		deductibleMinor:  deductibleMinor,
		ok:               true,
	}, nil
}

func (grammar AmountGrammar) LimitMinor() int64 { return grammar.limitMinor }

func (grammar AmountGrammar) RatioBasisPoints() int64 { return grammar.ratioBasisPoints }

func (grammar AmountGrammar) DeductibleMinor() int64 { return grammar.deductibleMinor }

// Same 比较两份已构造的取值。零值与任何登记都不相同，包括全零那一份登记。
func (grammar AmountGrammar) Same(other AmountGrammar) bool {
	return grammar.ok && other.ok &&
		grammar.limitMinor == other.limitMinor &&
		grammar.ratioBasisPoints == other.ratioBasisPoints &&
		grammar.deductibleMinor == other.deductibleMinor
}

// AmountComposition 是一次文法展开。金额为 0 时 FormsAmount 为假：不形成金额事实，
// 也不是未配置。
type AmountComposition struct {
	assertedMinor        int64
	deductibleMinor      int64
	afterDeductibleMinor int64
	ratioBasisPoints     int64
	scaledMinor          int64
	limitMinor           int64
	amountMinor          int64
	capped               bool
}

func (composition AmountComposition) AssertedMinor() int64 { return composition.assertedMinor }

func (composition AmountComposition) DeductibleMinor() int64 { return composition.deductibleMinor }

func (composition AmountComposition) AfterDeductibleMinor() int64 {
	return composition.afterDeductibleMinor
}

func (composition AmountComposition) RatioBasisPoints() int64 { return composition.ratioBasisPoints }

func (composition AmountComposition) ScaledMinor() int64 { return composition.scaledMinor }

func (composition AmountComposition) LimitMinor() int64 { return composition.limitMinor }

func (composition AmountComposition) AmountMinor() int64 { return composition.amountMinor }

func (composition AmountComposition) Capped() bool { return composition.capped }

func (composition AmountComposition) FormsAmount() bool { return composition.amountMinor > 0 }

// Compose 按固定顺序把主张收成一个金额：先减免税赔，不足记 0；再按万分比向下取整到
// 最小货币单位；再以限额封顶。顺序是产品策略，不由这次登记的数值决定。比例先乘再减
// 免赔会得到另一个数，本方法不提供那一条。
//
// 乘法走 big.Int。比例封在 10000 以内时商不会超过主张，仍然核对装不进 int64 就报溢出，
// 不静默绕回。
func (grammar AmountGrammar) Compose(assertedMinor int64) (AmountComposition, error) {
	if !grammar.ok || assertedMinor <= 0 {
		return AmountComposition{}, fmt.Errorf("%w: assertion", ErrInvalidAmountGrammar)
	}
	after := assertedMinor - grammar.deductibleMinor
	if after < 0 {
		after = 0
	}
	product := new(big.Int).Mul(big.NewInt(after), big.NewInt(grammar.ratioBasisPoints))
	product.Quo(product, big.NewInt(10_000))
	if !product.IsInt64() {
		return AmountComposition{}, ErrAmountGrammarOverflow
	}
	scaled := product.Int64()
	amount := scaled
	capped := false
	if amount > grammar.limitMinor {
		amount = grammar.limitMinor
		capped = true
	}
	return AmountComposition{
		assertedMinor:        assertedMinor,
		deductibleMinor:      grammar.deductibleMinor,
		afterDeductibleMinor: after,
		ratioBasisPoints:     grammar.ratioBasisPoints,
		scaledMinor:          scaled,
		limitMinor:           grammar.limitMinor,
		amountMinor:          amount,
		capped:               capped,
	}, nil
}

// AmountGrammarRegistration 把三项取值挂到一条已采用的引用上。引用本身仍由金额规则
// 版本册或合同责任读口持有，这里不另造版本。
type AmountGrammarRegistration struct {
	subject AmountGrammarSubject
	ref     string
	grammar AmountGrammar
}

func NewAmountGrammarRegistration(
	subject AmountGrammarSubject,
	ref string,
	grammar AmountGrammar,
) (AmountGrammarRegistration, error) {
	if !subject.valid() || strings.TrimSpace(ref) == "" || !grammar.ok {
		return AmountGrammarRegistration{}, fmt.Errorf("%w: amount grammar registration", ErrBlankValue)
	}
	return AmountGrammarRegistration{subject: subject, ref: ref, grammar: grammar}, nil
}

func (registration AmountGrammarRegistration) Subject() AmountGrammarSubject {
	return registration.subject
}

func (registration AmountGrammarRegistration) Ref() string { return registration.ref }

func (registration AmountGrammarRegistration) Grammar() AmountGrammar { return registration.grammar }

func (registration AmountGrammarRegistration) SameRegistration(other AmountGrammarRegistration) bool {
	return registration.subject == other.subject &&
		registration.ref == other.ref &&
		registration.grammar.Same(other.grammar)
}
