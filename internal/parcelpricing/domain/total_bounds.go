package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidTotalChargeBounds 表示一组总额上下限立不住：一个都没声明、金额非正、上下限币种不同、下限高于上限、
// 币种与方案币种不一致，或卡声明了金额取整而上下限不落在进位单位的整数倍上。
var ErrInvalidTotalChargeBounds = errors.New("parcel pricing: invalid total charge bounds")

// 总额上下限补足行 / 冲减行的计价费用代码（ADR-0348 决定一）。它们是产品内置的稳定费用语义，不由价卡另起名字：
// 下游的费用项目映射按码认，每张卡各取一个名字，同一件事在不同卡上就成了不同的码。只有方案声明了对应的一端，
// 这个码才被占用；没声明的方案里别的规则照旧可以用同名的码。
const (
	minimumChargeCodeText = "MINIMUM_CHARGE"
	maximumChargeCodeText = "MAXIMUM_CHARGE"

	// totalBoundSourceReference 是这类费用行的来源引用：它不来自某条规则或某个价表条目，而来自方案级的总额上下限声明。
	totalBoundSourceReference = "total-charge-bounds"
)

// MinimumChargeCode 是最低收费补足行的计价费用代码。
func MinimumChargeCode() ChargeCode { return ChargeCode{value: minimumChargeCodeText} }

// MaximumChargeCode 是最高收费冲减行的计价费用代码。
func MaximumChargeCode() ChargeCode { return ChargeCode{value: maximumChargeCodeText} }

// TotalChargeBounds 是价卡对**单票合计**声明的下限与上限（CONTEXT「总额上下限」；ADR-0348）。两端各自可缺，
// 但至少声明一端。金额币种与方案计价币种一致那一道在 NewAggregatePricingPlanVersion 上判——这里自己不知道卡币种。
type TotalChargeBounds struct {
	minimum *Money
	maximum *Money
}

// NewTotalChargeBounds 立一组总额上下限，nil 表示那一端不声明。下限与上限都必须为正：零下限等于没声明，
// 零上限会把任何一票压成零金额，而 CONTEXT 不许以零金额的已完成评价表达无适用价格。
func NewTotalChargeBounds(minimum, maximum *Money) (TotalChargeBounds, error) {
	bounds := TotalChargeBounds{}
	if minimum != nil {
		declared := *minimum
		bounds.minimum = &declared
	}
	if maximum != nil {
		declared := *maximum
		bounds.maximum = &declared
	}
	if !bounds.valid() {
		return TotalChargeBounds{}, ErrInvalidTotalChargeBounds
	}
	return bounds, nil
}

// Minimum 交回声明的下限；未声明时第二个返回值为假。
func (bounds TotalChargeBounds) Minimum() (Money, bool) {
	if bounds.minimum == nil {
		return Money{}, false
	}
	return *bounds.minimum, true
}

// Maximum 交回声明的上限；未声明时第二个返回值为假。
func (bounds TotalChargeBounds) Maximum() (Money, bool) {
	if bounds.maximum == nil {
		return Money{}, false
	}
	return *bounds.maximum, true
}

func (bounds TotalChargeBounds) valid() bool {
	if bounds.minimum == nil && bounds.maximum == nil {
		return false
	}
	for _, declared := range []*Money{bounds.minimum, bounds.maximum} {
		if declared != nil && (!declared.valid() || declared.amount.Sign() <= 0) {
			return false
		}
	}
	if bounds.minimum != nil && bounds.maximum != nil {
		return bounds.minimum.currency == bounds.maximum.currency && bounds.minimum.amount.Cmp(bounds.maximum.amount) <= 0
	}
	return true
}

func (bounds TotalChargeBounds) currency() Currency {
	if bounds.minimum != nil {
		return bounds.minimum.currency
	}
	return bounds.maximum.currency
}

// coherentWith 判上下限与方案其余部分是否自洽：币种与计价币种一致；卡声明了金额取整时上下限落在进位单位的整数倍上
// （否则合计取整会把补足后的合计又改回下限之下、或把封顶后的合计改回上限之上，这条规则就名不副实）。
func (bounds TotalChargeBounds) coherentWith(currency Currency, rounding *AmountRoundingPolicy) error {
	if bounds.currency() != currency {
		return ErrCurrencyMismatch
	}
	if rounding == nil {
		return nil
	}
	for _, declared := range []*Money{bounds.minimum, bounds.maximum} {
		if declared == nil {
			continue
		}
		onGrid, err := declared.amount.RoundToIncrement(rounding.increment.amount, RoundingCeiling)
		if err != nil {
			return err
		}
		if !onGrid.Equal(declared.amount) {
			return fmt.Errorf("%w: %s %s is not a multiple of the amount rounding increment %s",
				ErrInvalidTotalChargeBounds, declared.amount.String(), declared.currency, rounding.increment.amount.String())
		}
	}
	return nil
}

// boundAdjustment 是总额上下限对一票合计做出的调整：补足行或冲减行。
type boundAdjustment struct {
	lineID string
	code   ChargeCode
	effect ChargeEffect
	amount Money
}

// adjust 拿各费用行之和对照上下限。合计低于下限给出补足行（金额为下限减合计，各行相加仍等于合计）；高于上限给出
// 冲减行（金额为合计减上限，方向 DEDUCT）；落在上下限之间（含恰好等于）不调整。返回的说明串每次都有，
// 不调整也如实留痕——只在触发时才写的解释，读的人分不清「判过没触发」与「根本没判过」（CONTEXT「未命中的规则也要在
// 解释中留痕」同一纪律）。
func (bounds TotalChargeBounds) adjust(subtotal Money) (*boundAdjustment, string, error) {
	if bounds.minimum != nil && subtotal.amount.Cmp(bounds.minimum.amount) < 0 {
		difference, err := bounds.minimum.amount.Sub(subtotal.amount)
		if err != nil {
			return nil, "", err
		}
		amount, err := NewMoney(difference, subtotal.currency)
		if err != nil {
			return nil, "", err
		}
		return &boundAdjustment{lineID: "bound:minimum", code: MinimumChargeCode(), effect: ChargeEffectAdd, amount: amount},
			fmt.Sprintf("total bounds: lines sum to %s %s, below the declared minimum %s %s; %s supplement of %s %s added",
				subtotal.amount.String(), subtotal.currency, bounds.minimum.amount.String(), bounds.minimum.currency,
				minimumChargeCodeText, amount.amount.String(), amount.currency), nil
	}
	if bounds.maximum != nil && subtotal.amount.Cmp(bounds.maximum.amount) > 0 {
		difference, err := subtotal.amount.Sub(bounds.maximum.amount)
		if err != nil {
			return nil, "", err
		}
		amount, err := NewMoney(difference, subtotal.currency)
		if err != nil {
			return nil, "", err
		}
		return &boundAdjustment{lineID: "bound:maximum", code: MaximumChargeCode(), effect: ChargeEffectDeduct, amount: amount},
			fmt.Sprintf("total bounds: lines sum to %s %s, above the declared maximum %s %s; %s reduction of %s %s deducted",
				subtotal.amount.String(), subtotal.currency, bounds.maximum.amount.String(), bounds.maximum.currency,
				maximumChargeCodeText, amount.amount.String(), amount.currency), nil
	}
	return nil, fmt.Sprintf("total bounds: lines sum to %s %s, %s; no adjustment",
		subtotal.amount.String(), subtotal.currency, bounds.describePosition(subtotal)), nil
}

// describePosition 写出合计相对上下限的位置：恰好等于哪一端，或落在声明的范围之内。
func (bounds TotalChargeBounds) describePosition(subtotal Money) string {
	if bounds.minimum != nil && subtotal.amount.Equal(bounds.minimum.amount) {
		return fmt.Sprintf("equal to the declared minimum %s %s", bounds.minimum.amount.String(), bounds.minimum.currency)
	}
	if bounds.maximum != nil && subtotal.amount.Equal(bounds.maximum.amount) {
		return fmt.Sprintf("equal to the declared maximum %s %s", bounds.maximum.amount.String(), bounds.maximum.currency)
	}
	return "within the declared bounds"
}

// WithTotalChargeBounds 返回携带总额上下限的结构集合（ADR-0348 决定一）。它做成 builder 而不是构造参数，理由同
// 金额取整策略：多数卡不声明，它是另一个轴。币种与取整的自洽那一道在 NewAggregatePricingPlanVersion 上判。
func (structures PricingPlanStructures) WithTotalChargeBounds(bounds TotalChargeBounds) (PricingPlanStructures, error) {
	if !bounds.valid() {
		return PricingPlanStructures{}, ErrInvalidTotalChargeBounds
	}
	declared := bounds
	structures.totalBounds = &declared
	if !structures.valid() {
		return PricingPlanStructures{}, ErrInvalidPlanStructures
	}
	return structures, nil
}

// TotalChargeBounds 交回卡声明的总额上下限；未声明时第二个返回值为假——那时评价与引入总额上下限之前逐字相同。
func (structures PricingPlanStructures) TotalChargeBounds() (TotalChargeBounds, bool) {
	if structures.totalBounds == nil {
		return TotalChargeBounds{}, false
	}
	return *structures.totalBounds, true
}

// reservedChargeCodes 报出总额上下限占用的费用代码：只占声明了的那一端。
func (structures PricingPlanStructures) reservedChargeCodes() []ChargeCode {
	if structures.totalBounds == nil {
		return nil
	}
	var codes []ChargeCode
	if structures.totalBounds.minimum != nil {
		codes = append(codes, MinimumChargeCode())
	}
	if structures.totalBounds.maximum != nil {
		codes = append(codes, MaximumChargeCode())
	}
	return codes
}

// coherentWithPlan 判本版新增的两项结构（总额上下限、按偏远档位查档的价表）与方案其余部分是否自洽，构造与重验共用
// 这一段，免得两处各写一遍而漂开。seenCodes 是方案里已被基础运费、固定规则、附加费占用的费用代码。
func (structures PricingPlanStructures) coherentWithPlan(currency Currency, seenCodes map[string]struct{}) error {
	if structures.totalBounds != nil {
		if err := structures.totalBounds.coherentWith(currency, structures.amountRounding); err != nil {
			return err
		}
		for _, code := range structures.reservedChargeCodes() {
			if _, exists := seenCodes[code.String()]; exists {
				return fmt.Errorf("%w: %s", ErrDuplicateChargeCode, code.String())
			}
			seenCodes[code.String()] = struct{}{}
		}
	}
	// 按偏远档位查档的价表读的是评价解出的档位，没绑偏远档位目录就永远解不出：这是卡自己的矛盾，要在构造期拦下，
	// 而不是到评价时才对每一票答待判断。
	if structures.declaresRemoteTierKey() {
		if _, bound := structures.catalogueLink(CatalogueKindRemoteTier); !bound {
			return fmt.Errorf("%w: a surcharge rate table keyed by remote tier needs the plan to bind a remote tier catalogue", ErrInvalidPricingPlan)
		}
	}
	return nil
}

// newTotalBoundChargeLine 把一次调整成形为费用行；序号由调用方接在已用的最大序号之后。
func newTotalBoundChargeLine(adjustment boundAdjustment, scope ChargeScope, amount Money, order int) (ChargeLine, error) {
	description := "Minimum charge supplement"
	if adjustment.effect == ChargeEffectDeduct {
		description = "Maximum charge reduction"
	}
	return newChargeLine(adjustment.lineID, ChargeLineTotalBound, adjustment.code, scope, ChargeBasisFixedAmount,
		ChargeMethodFixedAmount, description, adjustment.effect, amount, order, totalBoundSourceReference)
}

// validTotalBoundLine 判一条总额上下限费用行的码与方向是否配对：补足行是 ADD 的 MINIMUM_CHARGE，冲减行是 DEDUCT 的
// MAXIMUM_CHARGE。别的组合是读回的数据坏了。
func validTotalBoundLine(code ChargeCode, effect ChargeEffect) bool {
	switch {
	case code == MinimumChargeCode():
		return effect == ChargeEffectAdd
	case code == MaximumChargeCode():
		return effect == ChargeEffectDeduct
	default:
		return false
	}
}
