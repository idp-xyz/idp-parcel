package domain

import "errors"

var errInvalidReturn = errors.New("warehouse fulfillment: invalid return")

// ReturnOrigin 是退货预报与回库处置的提交方。货主自己交，或运营代录。
type ReturnOrigin uint8

const (
	ReturnOriginInvalid ReturnOrigin = iota
	ReturnOriginCustomer
	ReturnOriginOperator
)

func (origin ReturnOrigin) String() string {
	switch origin {
	case ReturnOriginCustomer:
		return "CUSTOMER"
	case ReturnOriginOperator:
		return "OPERATOR"
	default:
		return ""
	}
}

func NewReturnOrigin(word string) (ReturnOrigin, error) {
	switch word {
	case ReturnOriginCustomer.String():
		return ReturnOriginCustomer, nil
	case ReturnOriginOperator.String():
		return ReturnOriginOperator, nil
	default:
		return ReturnOriginInvalid, errInvalidReturn
	}
}

// ReturnDestination 是回库处置的去向。处置只指名，不代替上架、转次品、销毁或退还的流水。
type ReturnDestination uint8

const (
	ReturnDestinationInvalid ReturnDestination = iota
	ReturnDestinationGoodPutaway
	ReturnDestinationDefective
	ReturnDestinationDestroy
	ReturnDestinationReturnToOwner
)

func (destination ReturnDestination) String() string {
	switch destination {
	case ReturnDestinationGoodPutaway:
		return "GOOD_PUTAWAY"
	case ReturnDestinationDefective:
		return "DEFECTIVE"
	case ReturnDestinationDestroy:
		return "DESTROY"
	case ReturnDestinationReturnToOwner:
		return "RETURN_TO_OWNER"
	default:
		return ""
	}
}

func NewReturnDestination(word string) (ReturnDestination, error) {
	switch word {
	case ReturnDestinationGoodPutaway.String():
		return ReturnDestinationGoodPutaway, nil
	case ReturnDestinationDefective.String():
		return ReturnDestinationDefective, nil
	case ReturnDestinationDestroy.String():
		return ReturnDestinationDestroy, nil
	case ReturnDestinationReturnToOwner.String():
		return ReturnDestinationReturnToOwner, nil
	default:
		return ReturnDestinationInvalid, errInvalidReturn
	}
}

type ReturnAdviceID struct{ requiredValue }

func NewReturnAdviceID(value string) (ReturnAdviceID, error) {
	required, err := newRequiredValue("return advice id", value)
	return ReturnAdviceID{required}, err
}

type ReturnFactID struct{ requiredValue }

func NewReturnFactID(value string) (ReturnFactID, error) {
	required, err := newRequiredValue("return fact id", value)
	return ReturnFactID{required}, err
}

type ReturnDecisionID struct{ requiredValue }

func NewReturnDecisionID(value string) (ReturnDecisionID, error) {
	required, err := newRequiredValue("return decision id", value)
	return ReturnDecisionID{required}, err
}
