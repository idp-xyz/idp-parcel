package domain

import (
	"errors"
	"math/big"
)

var (
	// ErrInvalidCreditBasis 拒绝立不住的授信依据：没有出处、额度为负、两格都不在场、比例格基数集外。
	ErrInvalidCreditBasis = errors.New("settlement accounting: invalid credit basis")
	// ErrCreditBasisNotRatio 拒绝对金额额度做折算：金额额度就是额度，没有基数可乘；走到这里是编排少判了一步。
	ErrCreditBasisNotRatio = errors.New("settlement accounting: credit basis is not a ratio")
	// ErrCreditRatioBaseUndeclared 拒绝对没有声明基数的比例额度做折算：那是 ADR-0129 之前固定的存量正文，
	// 编排该停在 CREDIT_RATIO_BASE_UNDECIDED，不该来问它相对什么。
	ErrCreditRatioBaseUndeclared = errors.New("settlement accounting: credit ratio base is undeclared")
	// ErrCreditLimitOverflow 拒绝一份乘出来装不进最小货币单位整数的额度——那不是一个能授的数。
	ErrCreditLimitOverflow = errors.New("settlement accounting: credit limit overflows")
)

// CreditPolicyReference 指名本次账期控制的额度出自哪一版信用政策（「对象/版本」写法，与
// AdoptedPolicyReference 同形）。它与 AdoptedPolicyReference、ControlPolicyReference 是三份不同的
// 采用依据：结算政策说按账期结算、控制策略说要做信用校验、信用政策说额度是多少——CONTEXT 要
// 每项信用暴露保存实际采用的政策，三份哪一份都替不了另一份（ADR-0127）。
type CreditPolicyReference struct{ requiredValue }

func NewCreditPolicyReference(value string) (CreditPolicyReference, error) {
	required, err := newRequiredValue("credit policy reference", value)
	return CreditPolicyReference{required}, err
}

// IsZero 答「有没有出处」：授权额度、暴露与账本行三处问的是同一个问题，让它只有一个名字。
func (reference CreditPolicyReference) IsZero() bool {
	return reference.value == ""
}

// CreditRatioBase 是比例额度相对于什么的封闭集，镜像 party-commercial 的同名封闭集而不 import 它
// （纪律同 SettlementMethod / CreditBasis）；成员只收本上下文凭自己的账本与状况登记算得出的那几格
// （ADR-0129 决定二），各格的取数路径在 CreditRatioBaseView 的实现里。
//
// 零值是「未声明」：不是一格可取的基数，只在译 ADR-0129 之前固定的存量比例正文时出现，编排据它停在
// CREDIT_RATIO_BASE_UNDECIDED——那一格不折 0 不折无限。
type CreditRatioBase uint8

const (
	CreditRatioBaseUndeclared CreditRatioBase = iota
	// PostedBalanceBase 入账余额：本作用域运营结算余额里由外部已确认收付款形成的那一项。
	PostedBalanceBase
	// PriorPeriodConfirmedChargesBase 上一结算周期已确认费用合计：本账户最近一张已发布对账单的费用行之和。
	PriorPeriodConfirmedChargesBase
)

func (base CreditRatioBase) valid() bool {
	switch base {
	case PostedBalanceBase, PriorPeriodConfirmedChargesBase:
		return true
	default:
		return false
	}
}

func (base CreditRatioBase) String() string {
	switch base {
	case PostedBalanceBase:
		return "POSTED_BALANCE"
	case PriorPeriodConfirmedChargesBase:
		return "PRIOR_PERIOD_CONFIRMED_CHARGES"
	default:
		return ""
	}
}

// CreditBasis 是商业侧随闭包交来的授信依据：出自哪一版信用政策、授权多少额度。额度两格封闭
// ——金额（最小货币单位）或比例（万分比 + 基数），镜像 party-commercial 的 CreditLimit 而不 import 它
// （纪律同 SettlementMethod）。比例相对于什么由政策正文声明（ADR-0129），本上下文按声明的基数取自己的
// 数折成金额（LimitOnBase）；基数未声明的比例只在存量正文里出现，只被如实携带。
//
// 它不带余额、不带已占用、不带任何调整——那些是本上下文自己的账本；政策只提供业务判断依据。
type CreditBasis struct {
	policy           CreditPolicyReference
	amountMinor      int64
	amount           bool
	ratioBasisPoints int64
	ratio            bool
	ratioBase        CreditRatioBase
}

// NewCreditAmountBasis 以金额额度建立一份授信依据。零额度是合法的商业声明（「授予零信用」），
// 负值在哪一格都无意义。
func NewCreditAmountBasis(policy CreditPolicyReference, amountMinor int64) (CreditBasis, error) {
	if policy.IsZero() || amountMinor < 0 {
		return CreditBasis{}, ErrInvalidCreditBasis
	}
	return CreditBasis{policy: policy, amountMinor: amountMinor, amount: true}, nil
}

// NewCreditRatioBasis 以比例额度（万分比）与声明的基数建立一份授信依据；基数集外或未声明拒——新交来的
// 比例正文必带基数（提供方构造门守着），这里没有基数只能是适配器译错。
func NewCreditRatioBasis(policy CreditPolicyReference, basisPoints int64, base CreditRatioBase) (CreditBasis, error) {
	if policy.IsZero() || basisPoints < 0 || !base.valid() {
		return CreditBasis{}, ErrInvalidCreditBasis
	}
	return CreditBasis{policy: policy, ratioBasisPoints: basisPoints, ratio: true, ratioBase: base}, nil
}

// NewUndeclaredCreditRatioBasis 只为译 ADR-0129 之前固定的、没有基数的存量比例正文：如实携带「未声明」，
// 编排据它停在 CREDIT_RATIO_BASE_UNDECIDED。新形成的比例依据不走这里。
func NewUndeclaredCreditRatioBasis(policy CreditPolicyReference, basisPoints int64) (CreditBasis, error) {
	if policy.IsZero() || basisPoints < 0 {
		return CreditBasis{}, ErrInvalidCreditBasis
	}
	return CreditBasis{policy: policy, ratioBasisPoints: basisPoints, ratio: true}, nil
}

func (basis CreditBasis) Policy() CreditPolicyReference {
	return basis.policy
}

// AmountMinor 的第二个返回值把「本格是比例因而没有金额」与「有金额」分开——不带它，一份金额为 0
// 的额度与一份比例额度读起来一样。
func (basis CreditBasis) AmountMinor() (int64, bool) {
	return basis.amountMinor, basis.amount
}

// RatioBasisPoints 与 AmountMinor 对称。
func (basis CreditBasis) RatioBasisPoints() (int64, bool) {
	return basis.ratioBasisPoints, basis.ratio
}

// RatioBase 交回比例额度声明的基数；第二个返回值为假是金额额度或未声明基数的存量比例——调用方先问
// RatioBasisPoints 再问这里，才分得开是哪一种。
func (basis CreditBasis) RatioBase() (CreditRatioBase, bool) {
	return basis.ratioBase, basis.ratio && basis.ratioBase.valid()
}

// LimitOnBase 把比例额度按基数在本上下文账本里的取值折成金额（ADR-0129 决定三）：
// ⌊基数 × 万分比 ÷ 10000⌋，向下取整到最小货币单位——按比例授信从不多授一个最小单位。
//
// 基数为负时额度为 0：入账余额为负是账户已欠款，账上没有任何可按比例放大的资金，0 是账本如实的答案，
// 不是替取不到的基数补零（取不到根本到不了这里，编排停在自己的`待判断`）。折出的 0 让暴露落`业务限制`，
// 恢复动作是客户入账——那正是`业务限制`这一格的用途。
//
// 乘法走 big.Int 再收回 int64：基数与万分比都是 int64，乘积可能装不下；装不下的额度不是一个能授的数，
// 报错而不是静默绕回。
func (basis CreditBasis) LimitOnBase(baseMinor int64) (int64, error) {
	if !basis.ratio {
		return 0, ErrCreditBasisNotRatio
	}
	if !basis.ratioBase.valid() {
		return 0, ErrCreditRatioBaseUndeclared
	}
	if baseMinor <= 0 {
		return 0, nil
	}
	product := new(big.Int).Mul(big.NewInt(baseMinor), big.NewInt(basis.ratioBasisPoints))
	product.Quo(product, big.NewInt(10_000))
	if !product.IsInt64() {
		return 0, ErrCreditLimitOverflow
	}
	return product.Int64(), nil
}
