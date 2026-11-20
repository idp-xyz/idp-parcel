package domain

// UnclaimedArrivalID 是待认领到货的标识，由本上下文签发。前缀不在这个类型里：签发器单独打。
type UnclaimedArrivalID struct{ requiredValue }

func NewUnclaimedArrivalID(value string) (UnclaimedArrivalID, error) {
	required, err := newRequiredValue("unclaimed arrival ID", value)
	return UnclaimedArrivalID{required}, err
}

// UnclaimedArrival 是收货事实到达时预报已不能收货、原事实保全下来的那一件。
// Receipt 上是原事实的全部内容。货主取自预报，只作线索，不是认定这批货归它。这一段不认领。
type UnclaimedArrival struct {
	id      UnclaimedArrivalID
	owner   OwnerAccountID
	receipt RecordedReceipt
}

func NewUnclaimedArrival(id UnclaimedArrivalID, owner OwnerAccountID, receipt RecordedReceipt) (UnclaimedArrival, error) {
	arrival := UnclaimedArrival{id: id, owner: owner, receipt: receipt}
	if !arrival.id.valid() || !arrival.owner.valid() || arrival.receipt.Outcome() != RecordedReceiptDivertedToUnclaimed ||
		!arrival.receipt.Tenant().valid() || !arrival.receipt.FactID().valid() || !arrival.receipt.PreAdvice().valid() {
		return UnclaimedArrival{}, ErrInvalidUnclaimedArrival
	}
	return arrival, nil
}

func (arrival UnclaimedArrival) ID() UnclaimedArrivalID { return arrival.id }
func (arrival UnclaimedArrival) Owner() OwnerAccountID  { return arrival.owner }
func (arrival UnclaimedArrival) Tenant() TenantID       { return arrival.receipt.Tenant() }
func (arrival UnclaimedArrival) PreAdvice() PreAdviceID { return arrival.receipt.PreAdvice() }
func (arrival UnclaimedArrival) Receipt() RecordedReceipt {
	return arrival.receipt
}

// ErrInvalidUnclaimedArrival 是一件待认领到货凑不成自洽的一份。
var ErrInvalidUnclaimedArrival = errInvalidUnclaimedArrival{}

type errInvalidUnclaimedArrival struct{}

func (errInvalidUnclaimedArrival) Error() string {
	return "warehouse fulfillment: invalid unclaimed arrival"
}
