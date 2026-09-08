package domain

import "errors"

// ErrInvalidCreditBasis 拒绝立不住的授信依据：没有出处、额度为负、或两格都不在场。
var ErrInvalidCreditBasis = errors.New("settlement accounting: invalid credit basis")

// CreditPolicyReference 指名本次账期控制的额度出自哪一版信用政策（「对象/版本」写法，与
// AdoptedPolicyReference 同形）。它与 AdoptedPolicyReference、ControlPolicyReference 是三份不同的
// 采用依据：结算政策说按账期结算、控制策略说要做信用校验、信用政策说额度是多少——CONTEXT 要
// 每项信用暴露保存实际采用的政策，三份哪一份都替不了另一份（ADR-0127）。
type CreditPolicyReference struct{ requiredValue }

func NewCreditPolicyReference(value string) (CreditPolicyReference, error) {
	required, err := newRequiredValue("credit policy reference", value)
	return CreditPolicyReference{required}, err
}

// CreditBasis 是商业侧随闭包交来的授信依据：出自哪一版信用政策、授权多少额度。额度两格封闭
// ——金额（最小货币单位）或比例（万分比），镜像 party-commercial 的 CreditLimit 而不 import 它
// （纪律同 SettlementMethod）。比例相对于什么基数由本上下文的业务判断给出，而那一格今天未裁，
// 所以比例在这里只是被如实携带，不折成金额。
//
// 它不带余额、不带已占用、不带任何调整——那些是本上下文自己的账本；政策只提供业务判断依据。
type CreditBasis struct {
	policy           CreditPolicyReference
	amountMinor      int64
	amount           bool
	ratioBasisPoints int64
	ratio            bool
}

// NewCreditAmountBasis 以金额额度建立一份授信依据。零额度是合法的商业声明（「授予零信用」），
// 负值在哪一格都无意义。
func NewCreditAmountBasis(policy CreditPolicyReference, amountMinor int64) (CreditBasis, error) {
	if policy.String() == "" || amountMinor < 0 {
		return CreditBasis{}, ErrInvalidCreditBasis
	}
	return CreditBasis{policy: policy, amountMinor: amountMinor, amount: true}, nil
}

// NewCreditRatioBasis 以比例额度（万分比）建立一份授信依据。
func NewCreditRatioBasis(policy CreditPolicyReference, basisPoints int64) (CreditBasis, error) {
	if policy.String() == "" || basisPoints < 0 {
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
