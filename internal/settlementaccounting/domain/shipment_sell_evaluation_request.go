package domain

import (
	"fmt"
	"time"
)

// ShipmentSellEvaluationRequestID 是委托级销售评价请求的铸造身份。与包裹那一路的 SellEvaluationRequestID 分成两种类型，
// 两种请求不能拿同一把标识去对方的册上查。
type ShipmentSellEvaluationRequestID struct{ requiredValue }

func NewShipmentSellEvaluationRequestID(value string) (ShipmentSellEvaluationRequestID, error) {
	required, err := newRequiredValue("shipment sell evaluation request ID", value)
	return ShipmentSellEvaluationRequestID{required}, err
}

// ShipmentReference 指名一份委托，取 parcel-shipment 的委托标识原样。
type ShipmentReference struct{ requiredValue }

func NewShipmentReference(value string) (ShipmentReference, error) {
	required, err := newRequiredValue("shipment reference", value)
	return ShipmentReference{required}, err
}

// ShipmentSellEvaluationRequestNaturalKey 是委托级请求的自然键：同一份委托在同一种委托级时点上按代次排。
// 租户在登记册键上另带。
type ShipmentSellEvaluationRequestNaturalKey struct {
	Shipment ShipmentReference
	Moment   ShipmentBillingMoment
}

// ShipmentSellEvaluationRequestSpec 是提交一份委托级销售评价请求所需的全部输入。
// Members 是形成这一拍时仍然有效、并且都已经完成有效网络收寄的那些件。费用形成之后成员再变，不靠改这份清单重算。
type ShipmentSellEvaluationRequestSpec struct {
	ID                 ShipmentSellEvaluationRequestID
	Shipment           ShipmentReference
	Members            []ParcelReference
	Account            SettlementAccountID
	SettlementCurrency CurrencyCode
	PricingScope       EstimatePricingScope
	PricingScopes      []EstimatePricingScope
	Moment             ShipmentBillingMoment
	Source             SourceFactReference
	PricingBasisAt     time.Time
	Generation         int
	Previous           ShipmentSellEvaluationRequestID
	ResumeBasis        ResumeBasis
}

// ShipmentSellEvaluationRequest 是向计价提出的一次委托级 SELL·客户费用评价。类型上没有金额。
type ShipmentSellEvaluationRequest struct {
	id                 ShipmentSellEvaluationRequestID
	naturalKey         ShipmentSellEvaluationRequestNaturalKey
	members            []ParcelReference
	account            SettlementAccountID
	settlementCurrency CurrencyCode
	pricingScope       EstimatePricingScope
	pricingScopes      []EstimatePricingScope
	source             SourceFactReference
	pricingBasisAt     time.Time
	generation         int
	previous           ShipmentSellEvaluationRequestID
	resumeBasis        ResumeBasis
}

// shipmentCustomerChargePrefix 是由委托级请求派生的预估客户费用标识类别段。与包裹那一路的 SELLCC 分开，
// 两路费用标识不会撞在同一串上。
const shipmentCustomerChargePrefix = "SHPCC"

func SubmitShipmentSellEvaluationRequest(spec ShipmentSellEvaluationRequestSpec) (ShipmentSellEvaluationRequest, error) {
	members, err := shipmentMembers(spec.Members)
	if err != nil {
		return ShipmentSellEvaluationRequest{}, err
	}
	if !spec.ID.valid() || !spec.Shipment.valid() || !spec.Account.valid() || !spec.SettlementCurrency.valid() ||
		!spec.PricingScope.valid() || !spec.Moment.valid() || !spec.Source.valid() || spec.PricingBasisAt.IsZero() {
		return ShipmentSellEvaluationRequest{}, ErrInvalidSellEvaluationRequest
	}
	scopes, err := sellPriceScopes(spec.PricingScope, spec.PricingScopes)
	if err != nil {
		return ShipmentSellEvaluationRequest{}, err
	}
	generation, err := normalizeLineage(spec.Generation, spec.Previous.valid(), spec.ResumeBasis)
	if err != nil {
		return ShipmentSellEvaluationRequest{}, fmt.Errorf("%w: %w", ErrInvalidSellEvaluationRequest, err)
	}
	if spec.Previous.valid() && spec.Previous == spec.ID {
		return ShipmentSellEvaluationRequest{}, fmt.Errorf("%w: a request cannot follow itself", ErrInvalidSellEvaluationRequest)
	}
	return ShipmentSellEvaluationRequest{
		id:                 spec.ID,
		naturalKey:         ShipmentSellEvaluationRequestNaturalKey{Shipment: spec.Shipment, Moment: spec.Moment},
		members:            members,
		account:            spec.Account,
		settlementCurrency: spec.SettlementCurrency,
		pricingScope:       spec.PricingScope,
		pricingScopes:      scopes,
		source:             spec.Source,
		pricingBasisAt:     spec.PricingBasisAt.UTC(),
		generation:         generation,
		previous:           spec.Previous,
		resumeBasis:        spec.ResumeBasis,
	}, nil
}

func shipmentMembers(members []ParcelReference) ([]ParcelReference, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("%w: shipment sell request has no members", ErrInvalidSellEvaluationRequest)
	}
	copied := make([]ParcelReference, len(members))
	seen := make(map[string]struct{}, len(members))
	for index, member := range members {
		if !member.valid() {
			return nil, fmt.Errorf("%w: shipment sell request member", ErrInvalidSellEvaluationRequest)
		}
		if _, duplicate := seen[member.String()]; duplicate {
			return nil, fmt.Errorf("%w: shipment sell request repeats %s", ErrInvalidSellEvaluationRequest, member)
		}
		seen[member.String()] = struct{}{}
		copied[index] = member
	}
	return copied, nil
}

func (request ShipmentSellEvaluationRequest) ID() ShipmentSellEvaluationRequestID {
	return request.id
}
func (request ShipmentSellEvaluationRequest) Shipment() ShipmentReference {
	return request.naturalKey.Shipment
}
func (request ShipmentSellEvaluationRequest) Moment() ShipmentBillingMoment {
	return request.naturalKey.Moment
}
func (request ShipmentSellEvaluationRequest) Members() []ParcelReference {
	return append([]ParcelReference(nil), request.members...)
}
func (request ShipmentSellEvaluationRequest) Account() SettlementAccountID { return request.account }
func (request ShipmentSellEvaluationRequest) SettlementCurrency() CurrencyCode {
	return request.settlementCurrency
}
func (request ShipmentSellEvaluationRequest) PricingScope() EstimatePricingScope {
	return request.pricingScope
}
func (request ShipmentSellEvaluationRequest) PricingScopes() ([]EstimatePricingScope, bool) {
	if len(request.pricingScopes) < 2 {
		return nil, false
	}
	return append([]EstimatePricingScope(nil), request.pricingScopes...), true
}
func (request ShipmentSellEvaluationRequest) Source() SourceFactReference { return request.source }
func (request ShipmentSellEvaluationRequest) PricingBasisAt() time.Time {
	return request.pricingBasisAt
}
func (request ShipmentSellEvaluationRequest) Generation() int { return request.generation }
func (request ShipmentSellEvaluationRequest) Previous() (ShipmentSellEvaluationRequestID, bool) {
	return request.previous, request.previous.valid()
}
func (request ShipmentSellEvaluationRequest) ResumeBasis() ResumeBasis { return request.resumeBasis }

// EstimatedCustomerChargeID 由请求标识派生费用标识。委托级费用仍是客户费用，身份不占用包裹费用的主键。
func (request ShipmentSellEvaluationRequest) EstimatedCustomerChargeID() CustomerChargeID {
	return CustomerChargeID{requiredValue{value: shipmentCustomerChargePrefix + "-" + request.id.String()}}
}

// SameQuestion 比的是这一问的内容。代次与标识不在里面：同一问的下一代问的还是这些。
func (request ShipmentSellEvaluationRequest) SameQuestion(other ShipmentSellEvaluationRequest) bool {
	if request.naturalKey != other.naturalKey || request.account != other.account ||
		request.settlementCurrency != other.settlementCurrency || request.pricingScope != other.pricingScope ||
		request.source != other.source || !request.pricingBasisAt.Equal(other.pricingBasisAt) ||
		len(request.members) != len(other.members) || len(request.pricingScopes) != len(other.pricingScopes) {
		return false
	}
	for index := range request.members {
		if request.members[index] != other.members[index] {
			return false
		}
	}
	for index := range request.pricingScopes {
		if request.pricingScopes[index] != other.pricingScopes[index] {
			return false
		}
	}
	return true
}
