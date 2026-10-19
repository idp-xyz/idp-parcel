package domain

import "fmt"

// ParcelBillingMoment 是包裹计费时点（ADR-0190 决定二，产品策略）：一件包裹在哪一拍发起 SELL·客户费用评价。
// 集合封闭，加一种时点是产品发版的事，不是登一行的事。「委托已接受」有意不在里面：那时服务还没发生，委托取消后
// 没有让费用失效的路径，而且与受理前估价重叠。
type ParcelBillingMoment uint8

const (
	ParcelBillingMomentInvalid ParcelBillingMoment = iota
	// BillingMomentNetworkIntakeAdopted 是`有效网络收寄已采用`：小包托运采用了一份有效网络收寄，不采用的不算。
	BillingMomentNetworkIntakeAdopted
	// BillingMomentEffectiveDeliveryFinalized 是`有效交付已终局`：小包托运把这件包裹采用为终局「有效交付」。
	BillingMomentEffectiveDeliveryFinalized
)

func (moment ParcelBillingMoment) String() string {
	switch moment {
	case BillingMomentNetworkIntakeAdopted:
		return "NETWORK_INTAKE_ADOPTED"
	case BillingMomentEffectiveDeliveryFinalized:
		return "EFFECTIVE_DELIVERY_FINALIZED"
	default:
		return ""
	}
}

func (moment ParcelBillingMoment) valid() bool {
	return moment.String() != ""
}

// ParcelBillingMomentFromName 把册上与登记输入里的时点词译回领域常量。词与 String 以及库上的 CHECK 同字；词表外的
// 拒——库里出现一个本上下文不认识的时点只可能是适配器或迁移的 bug，读成某一格会让一户换了时点还看不出来。
func ParcelBillingMomentFromName(name string) (ParcelBillingMoment, error) {
	for _, candidate := range []ParcelBillingMoment{BillingMomentNetworkIntakeAdopted, BillingMomentEffectiveDeliveryFinalized} {
		if candidate.String() == name {
			return candidate, nil
		}
	}
	return ParcelBillingMomentInvalid, fmt.Errorf("%w: parcel billing moment %q", ErrBlankValue, name)
}
