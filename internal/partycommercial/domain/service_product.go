package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidServiceProduct        = errors.New("party commercial: invalid service product")
	ErrInvalidProductChannelMapping = errors.New("party commercial: invalid product channel mapping")
)

// ChannelProductReference points at a service offered by a channel provider —
// a carrier's own channel, an agent, a reseller or an aggregator. It is neither
// the operator's own service product nor the actual carrier of any shipment.
type ChannelProductReference struct{ requiredValue }

func NewChannelProductReference(value string) (ChannelProductReference, error) {
	required, err := newRequiredValue("channel product reference", value)
	return ChannelProductReference{required}, err
}

// ServiceProductForm is a facet of a service product, not a separate catalogue:
// a network product is a service product organised over the operator's own
// network, and the context forbids building a third product catalogue beside it.
//
// The standalone label-channel form is a long-term product form that `PAR-COM-12`
// declares inapplicable for the first release, so it is deliberately absent
// here rather than present and unimplemented.
type ServiceProductForm uint8

const (
	ServiceProductFormInvalid ServiceProductForm = iota
	NetworkServiceForm
)

func (form ServiceProductForm) valid() bool {
	return form == NetworkServiceForm
}

func (form ServiceProductForm) String() string {
	switch form {
	case NetworkServiceForm:
		return "NETWORK_SERVICE"
	default:
		return ""
	}
}

type ServiceProduct struct {
	version CommercialVersion
	form    ServiceProductForm
}

func NewServiceProduct(version CommercialVersion, form ServiceProductForm) (ServiceProduct, error) {
	if version.kind != ServiceProductObject ||
		version.status != CommercialVersionEffective ||
		!form.valid() {
		return ServiceProduct{}, ErrInvalidServiceProduct
	}
	return ServiceProduct{version: version, form: form}, nil
}

func (product ServiceProduct) Version() CommercialVersion {
	return product.version
}

func (product ServiceProduct) Form() ServiceProductForm {
	return product.form
}

// ProductChannelMapping is the versioned commercial relation between a service
// product version and the channel products usable for it. It defines a candidate
// range and nothing more: which channel a given transaction actually uses is
// recorded by the context that owns that transaction, so this type offers no
// selection, preference, default or lock.
type ProductChannelMapping struct {
	product   ServiceProduct
	channels  []ChannelProductReference
	effective EffectiveInterval
}

func NewProductChannelMapping(
	product ServiceProduct,
	channels []ChannelProductReference,
	effective EffectiveInterval,
) (ProductChannelMapping, error) {
	// A zero ServiceProduct has no valid form, so this rejects a mapping built
	// against a product that never went through NewServiceProduct.
	if !product.form.valid() || len(channels) == 0 || !effective.valid() {
		return ProductChannelMapping{}, ErrInvalidProductChannelMapping
	}
	seen := make(map[ChannelProductReference]struct{}, len(channels))
	for _, channel := range channels {
		if !channel.valid() {
			return ProductChannelMapping{}, ErrInvalidProductChannelMapping
		}
		if _, exists := seen[channel]; exists {
			return ProductChannelMapping{}, ErrInvalidProductChannelMapping
		}
		seen[channel] = struct{}{}
	}
	return ProductChannelMapping{
		product:   product,
		channels:  append([]ChannelProductReference(nil), channels...),
		effective: effective,
	}, nil
}

func (mapping ProductChannelMapping) Product() ServiceProduct {
	return mapping.product
}

// ChannelProducts is the mapping's full historical reference set. It stays
// intact after expiry: an expired mapping stops offering candidates for new
// selections but remains the basis existing shipments and transactions cite.
func (mapping ProductChannelMapping) ChannelProducts() []ChannelProductReference {
	return append([]ChannelProductReference(nil), mapping.channels...)
}

func (mapping ProductChannelMapping) Effective() EffectiveInterval {
	return mapping.effective
}

// CandidatesAt lists the channels available for a new selection at an instant.
// Outside the effective interval it lists none, which is how expiry excludes a
// channel from new decisions without erasing what it once supported.
func (mapping ProductChannelMapping) CandidatesAt(at time.Time) []ChannelProductReference {
	if !mapping.effective.Contains(at) {
		return nil
	}
	return mapping.ChannelProducts()
}

// CandidatesAllowedBy applies a customer's channel constraint. The constraint
// only narrows: a channel the customer names but the mapping does not offer
// never becomes a candidate, because the operator may choose only inside the
// commercially available range.
func (mapping ProductChannelMapping) CandidatesAllowedBy(
	allowed []ChannelProductReference,
	at time.Time,
) []ChannelProductReference {
	available := mapping.CandidatesAt(at)
	if len(allowed) == 0 {
		return available
	}

	permitted := make(map[ChannelProductReference]struct{}, len(allowed))
	for _, channel := range allowed {
		permitted[channel] = struct{}{}
	}
	narrowed := make([]ChannelProductReference, 0, len(available))
	for _, channel := range available {
		if _, ok := permitted[channel]; ok {
			narrowed = append(narrowed, channel)
		}
	}
	return narrowed
}
