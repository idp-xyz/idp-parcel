package domain

import "errors"

// ErrInvalidCreditLimit 拒绝立不住的信用额度取值。
var ErrInvalidCreditLimit = errors.New("party commercial: invalid credit limit")

// CreditRatioBase 是比例额度相对于什么的封闭集（ADR-0129）：一个「30%」没有分母不是业务判断依据，
// 分母属于同一条政策声明，由登记政策的人说出，不留给消费方判断补齐。
//
// 成员只收 settlement-accounting 凭自己的账本与状况登记算得出的那几格，词跟着 SA 算得出的那一项走：
// 「入账余额」是 SA CONTEXT 运营结算余额五项之一的原词，不叫「预付 / 保证金余额」——SA 没有保证金那一格，
// 一个指向它算不出的东西的词什么都声明不了。「合同声明基数」不在集合里：PC 合同没有这一格，收它就是给合同
// 长一格，那是另一张票。
//
// 零值是「未声明」：它不是一格可登的取值（valid 为假，构造门不收），只在重建门读回本记录之前登进去的存量
// 比例正文时出现——ADR-0028 只校验不重算，那些行如实读回为未声明，由 SA 停在它自己的`待判断`。
type CreditRatioBase uint8

const (
	CreditRatioBaseUndeclared CreditRatioBase = iota
	// PostedBalanceBase 入账余额：该结算作用域运营结算余额里由外部已确认收付款形成的那一项。
	PostedBalanceBase
	// PriorPeriodConfirmedChargesBase 上一结算周期已确认费用合计：该结算账户最近一张已发布对账单的费用行之和。
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

// String 交回批文 / 载荷 / 库列 / 词表共用的原词；未声明与集外一律空串，它们都不是能写出去的取值。
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

// CreditRatioBaseNamed 把原词反查回封闭集；空串与集外都答不认识——空串留给调用方按「缺席」而不是按
// 「填错」点名，两者要人做的事不同。
func CreditRatioBaseNamed(value string) (CreditRatioBase, bool) {
	for _, candidate := range []CreditRatioBase{PostedBalanceBase, PriorPeriodConfirmedChargesBase} {
		if candidate.String() == value {
			return candidate, true
		}
	}
	return CreditRatioBaseUndeclared, false
}

// CreditLimit 是信用政策授权的额度，两格封闭：金额（最小货币单位）或比例（万分比 + 基数）。
//
// CONTEXT 说信用政策「按……金额或比例形成版本」。这里不是「一个数加一个说它是什么的标记」
// （票 party-commercial-context-gaps/03 的裁决）：额度 100 作金额与作比例在那一个数字里长得
// 一模一样，标记设错时没有任何东西能分辨，两种读法都产出一个合法的额度、只是差几个数量级。
// 两格各自构造，**值落在哪一格本身就是判别式**，它不可能与自己不一致。
//
// 比例取万分比整数而不是小数：本上下文没有 Decimal 类型（那是立场不是缺漏，见
// VolumetricFactorReference 的注释），而万分比是商业约定里最细的常用整数刻度，不必为它另裁精度。
//
// 比例格自 ADR-0129 起带基数：比例在场基数必在场（构造门拒缺席），金额格没有基数这一格——「含则必填、
// 不含则必缺」（ADR-0044 同形）。基数不给默认：默认值是替租户拍板一个商业判断。
//
// 零值立不住：`declared` 让「忘了填」与「授予零额度」分得开——后者要经 NewCreditAmountLimit(0)
// 或 NewCreditRatioLimit(0, base) 显式说出。零额度本身是合法的商业声明（CreditBasis.Applicable 分它
// 与「没有政策」），负值在哪一格都无意义。
type CreditLimit struct {
	declared    bool
	ratio       bool
	amountMinor int64
	ratioBps    int64
	ratioBase   CreditRatioBase
}

// NewCreditAmountLimit 以最小货币单位建立一份金额额度。
func NewCreditAmountLimit(minor int64) (CreditLimit, error) {
	if minor < 0 {
		return CreditLimit{}, ErrInvalidCreditLimit
	}
	return CreditLimit{declared: true, amountMinor: minor}, nil
}

// NewCreditRatioLimit 以万分比与基数建立一份比例额度。基数缺席或集外一律拒：没有分母的比例不是额度，
// 而本上下文不替登记方挑一个（ADR-0129 决定一）。
func NewCreditRatioLimit(basisPoints int64, base CreditRatioBase) (CreditLimit, error) {
	if basisPoints < 0 || !base.valid() {
		return CreditLimit{}, ErrInvalidCreditLimit
	}
	return CreditLimit{declared: true, ratio: true, ratioBps: basisPoints, ratioBase: base}, nil
}

// RehydrateCreditRatioLimit 是比例额度的重建门（ADR-0028）：只为读回 ADR-0129 之前登进去的、没有基数的
// 存量比例正文——库列 NULL、闭包快照缺键——它们如实读回为未声明，不补默认也不拒（拒了那些已固定的
// 解析就读不回来）。基数在场时与构造门同判：集外仍是坏数据，报错不吸收。
//
// 新登记的正文不走这里：发布路与构造门只认 NewCreditRatioLimit。
func RehydrateCreditRatioLimit(basisPoints int64, base CreditRatioBase) (CreditLimit, error) {
	if basisPoints < 0 || (base != CreditRatioBaseUndeclared && !base.valid()) {
		return CreditLimit{}, ErrInvalidCreditLimit
	}
	return CreditLimit{declared: true, ratio: true, ratioBps: basisPoints, ratioBase: base}, nil
}

func (limit CreditLimit) valid() bool {
	return limit.declared
}

// AmountMinor 的第二个返回值把「本格是比例因而没有金额」与「本该有却缺了」分开。后者造不出来
// （构造门已拒），所以为假只意味着这是一份比例额度或零值——不带这个布尔的话，一份金额为 0 的
// 额度与一份比例额度读起来一样。
func (limit CreditLimit) AmountMinor() (int64, bool) {
	return limit.amountMinor, limit.declared && !limit.ratio
}

// RatioBasisPoints 与 AmountMinor 对称。
func (limit CreditLimit) RatioBasisPoints() (int64, bool) {
	return limit.ratioBps, limit.declared && limit.ratio
}

// RatioBase 交回比例额度声明的基数；第二个返回值为假是「金额额度」或「重建门读回的未声明存量比例」，
// 两者都没有基数可交——调用方先问 RatioBasisPoints 再问这里，才分得开是哪一种。
func (limit CreditLimit) RatioBase() (CreditRatioBase, bool) {
	return limit.ratioBase, limit.declared && limit.ratio && limit.ratioBase.valid()
}
