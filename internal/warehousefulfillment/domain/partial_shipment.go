package domain

// PartialShipment 是一张出库请求拣不齐时货主愿意怎么发。三种都是货主的意思，产品不替他选：
// 没写明的就是没写明，不当成其中任何一种。
type PartialShipment uint8

const (
	// PartialShipmentUnset 是没写明。
	PartialShipmentUnset PartialShipment = iota
	// PartialShipmentShipComplete 是整单发：拣不齐就不发，等货或截单。
	PartialShipmentShipComplete
	// PartialShipmentShortShip 是少发：拣到的发走，没拣到的不再发。
	PartialShipmentShortShip
	// PartialShipmentSplitShip 是拆单发：拣到的发走，没拣到的拆成一张新请求继续等货。
	PartialShipmentSplitShip
)

func (rule PartialShipment) String() string {
	switch rule {
	case PartialShipmentShipComplete:
		return "SHIP_COMPLETE"
	case PartialShipmentShortShip:
		return "SHORT_SHIP"
	case PartialShipmentSplitShip:
		return "SPLIT_SHIP"
	default:
		return ""
	}
}

// Given 回答这一格写明了没有。
func (rule PartialShipment) Given() bool { return rule != PartialShipmentUnset }

// NewPartialShipment 只收三个词。空串不是「没写明」的写法：没写明就不带这一格。
func NewPartialShipment(word string) (PartialShipment, error) {
	for _, rule := range []PartialShipment{PartialShipmentShipComplete, PartialShipmentShortShip, PartialShipmentSplitShip} {
		if rule.String() == word {
			return rule, nil
		}
	}
	return PartialShipmentUnset, newRequiredValueError("partial shipment")
}

// PartialShipmentSource 是一张请求此刻适用的部分发货规则出自哪一层。
type PartialShipmentSource uint8

const (
	// PartialShipmentFromNowhere 是哪一层都没写明：这张请求的部分发货未配置。
	PartialShipmentFromNowhere PartialShipmentSource = iota
	// PartialShipmentFromConsent 是受理之后货主对这一张请求另外表示的意思，由运营凭依据代录。
	PartialShipmentFromConsent
	// PartialShipmentFromOrder 是这张请求提交时带的。
	PartialShipmentFromOrder
	// PartialShipmentFromOwner 是货主登记的默认。
	PartialShipmentFromOwner
)

func (source PartialShipmentSource) String() string {
	switch source {
	case PartialShipmentFromConsent:
		return "ORDER_CONSENT"
	case PartialShipmentFromOrder:
		return "ORDER"
	case PartialShipmentFromOwner:
		return "OWNER"
	default:
		return ""
	}
}

// EffectivePartialShipment 定一张请求此刻适用哪一层：越贴近这一张请求、越晚说的越优先。
// 受理之后代录的那一句盖过提交时带的，提交时带的盖过货主的默认；哪一层都没写明就是未配置。
// 订单上写了整单发，货主默认允许少发也不能少发——货主对这一张另有交代。
func EffectivePartialShipment(consent, order, owner PartialShipment) (PartialShipment, PartialShipmentSource) {
	switch {
	case consent.Given():
		return consent, PartialShipmentFromConsent
	case order.Given():
		return order, PartialShipmentFromOrder
	case owner.Given():
		return owner, PartialShipmentFromOwner
	default:
		return PartialShipmentUnset, PartialShipmentFromNowhere
	}
}
