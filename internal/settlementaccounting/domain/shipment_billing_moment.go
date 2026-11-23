package domain

import "fmt"

// ShipmentBillingMoment 是委托级计费时点。集合里只有一种：当前有效各件的有效网络收寄都已经采用。
// 「委托已接受」不在这里——服务还没发生，收寄前可以取消。第一件到了就计也不在这里：少一件还不是这一拍。
// 选卡用的时刻不是另一种时点，固定取第一件的责任起点，不交给租户再选。
type ShipmentBillingMoment uint8

const (
	ShipmentBillingMomentInvalid ShipmentBillingMoment = iota
	// ShipmentMembersIntakeAdopted 是「当前有效各件的有效网络收寄已齐」。
	ShipmentMembersIntakeAdopted
)

func (moment ShipmentBillingMoment) String() string {
	switch moment {
	case ShipmentMembersIntakeAdopted:
		return "SHIPMENT_MEMBERS_INTAKE_ADOPTED"
	default:
		return ""
	}
}

func (moment ShipmentBillingMoment) valid() bool {
	return moment.String() != ""
}

// ShipmentBillingMomentFromName 把册上的时点词译回领域常量。词表外的拒，不默认成这一种。
func ShipmentBillingMomentFromName(name string) (ShipmentBillingMoment, error) {
	if ShipmentMembersIntakeAdopted.String() == name {
		return ShipmentMembersIntakeAdopted, nil
	}
	return ShipmentBillingMomentInvalid, fmt.Errorf("%w: shipment billing moment %q", ErrBlankValue, name)
}
