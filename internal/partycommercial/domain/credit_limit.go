package domain

import "errors"

// ErrInvalidCreditLimit 拒绝立不住的信用额度取值。
var ErrInvalidCreditLimit = errors.New("party commercial: invalid credit limit")

// CreditLimit 是信用政策授权的额度，两格封闭：金额（最小货币单位）或比例（万分比）。
//
// CONTEXT 说信用政策「按……金额或比例形成版本」。这里不是「一个数加一个说它是什么的标记」
// （票 party-commercial-context-gaps/03 的裁决）：额度 100 作金额与作比例在那一个数字里长得
// 一模一样，标记设错时没有任何东西能分辨，两种读法都产出一个合法的额度、只是差几个数量级。
// 两格各自构造，**值落在哪一格本身就是判别式**，它不可能与自己不一致。
//
// 比例取万分比整数而不是小数：本上下文没有 Decimal 类型（那是立场不是缺漏，见
// VolumetricFactorReference 的注释），而万分比是商业约定里最细的常用整数刻度，不必为它另裁精度。
//
// 零值立不住：`declared` 让「忘了填」与「授予零额度」分得开——后者要经 NewCreditAmountLimit(0)
// 或 NewCreditRatioLimit(0) 显式说出。零额度本身是合法的商业声明（CreditBasis.Applicable 分它
// 与「没有政策」），负值在哪一格都无意义。
type CreditLimit struct {
	declared    bool
	ratio       bool
	amountMinor int64
	ratioBps    int64
}

// NewCreditAmountLimit 以最小货币单位建立一份金额额度。
func NewCreditAmountLimit(minor int64) (CreditLimit, error) {
	if minor < 0 {
		return CreditLimit{}, ErrInvalidCreditLimit
	}
	return CreditLimit{declared: true, amountMinor: minor}, nil
}

// NewCreditRatioLimit 以万分比建立一份比例额度。比例相对于什么基数由消费方的业务判断给出，
// 本上下文只携带声明。
func NewCreditRatioLimit(basisPoints int64) (CreditLimit, error) {
	if basisPoints < 0 {
		return CreditLimit{}, ErrInvalidCreditLimit
	}
	return CreditLimit{declared: true, ratio: true, ratioBps: basisPoints}, nil
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
