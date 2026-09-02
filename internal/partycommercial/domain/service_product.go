package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidServiceProduct        = errors.New("party commercial: invalid service product")
	ErrInvalidProductChannelMapping = errors.New("party commercial: invalid product channel mapping")
)

// ChannelProductReference 指向渠道服务方提供的服务——承运商直营渠道、承运商代理商、
// 转售商或聚合平台。它既不是运营企业自己的服务产品，也不是任何一次运输的实际承运商。
type ChannelProductReference struct{ requiredValue }

func NewChannelProductReference(value string) (ChannelProductReference, error) {
	required, err := newRequiredValue("channel product reference", value)
	return ChannelProductReference{required}, err
}

// ServiceProductForm 是服务产品的一种服务形态，不是另一套目录：两种形态都由服务产品
// 版本承载，本上下文禁止在它们旁边再建第三套产品目录。
//
// 两格的分界是**责任起点**而不是渠道用不用得上：网络服务由运营企业自己的网络履约，
// 责任起于有效网络收寄结果；面单渠道服务不虚构运营企业收寄，责任来源于面单交易绑定
// 的账号与合同，运输观察从实际承运商收寄事实开始（PC CONTEXT「网络服务产品与面单渠道
// 服务必须分别表达服务责任」）。网络服务同样可以用外部渠道，所以不能按「有没有渠道」分。
//
// 面单渠道服务此前有意不列，依据是 `PAR-COM-12` 对首发不适用；ADR-0088 已把它改为纳入。
type ServiceProductForm uint8

const (
	ServiceProductFormInvalid ServiceProductForm = iota
	NetworkServiceForm
	LabelChannelServiceForm
)

func (form ServiceProductForm) valid() bool {
	return form == NetworkServiceForm || form == LabelChannelServiceForm
}

func (form ServiceProductForm) String() string {
	switch form {
	case NetworkServiceForm:
		return "NETWORK_SERVICE"
	case LabelChannelServiceForm:
		return "LABEL_CHANNEL_SERVICE"
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

// ProductChannelMapping 是服务产品版本与其可用渠道产品之间的版本化商业关系。它只
// 定义候选范围，别无其他：某次交易实际用了哪个渠道，由拥有该交易的上下文记录，
// 所以本类型不提供选择、优先级、默认值或锁定。
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
	// 零值 ServiceProduct 的服务形态不合法，这一条因此挡住了基于未经 NewServiceProduct
	// 构造的产品建立的映射。
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

// ChannelProducts 是该映射完整的历史引用集合。到期后它原样保留：过期映射不再为新的
// 渠道选择提供候选，但仍是既有委托和交易所引用的依据。
func (mapping ProductChannelMapping) ChannelProducts() []ChannelProductReference {
	return append([]ChannelProductReference(nil), mapping.channels...)
}

func (mapping ProductChannelMapping) Effective() EffectiveInterval {
	return mapping.effective
}

// CandidatesAt 列出某个时点上可用于新选择的渠道。落在有效期之外时一个都不列——
// 到期正是这样把渠道排除在新决定之外，同时不抹掉它当初支撑过的东西。
func (mapping ProductChannelMapping) CandidatesAt(at time.Time) []ChannelProductReference {
	if !mapping.effective.Contains(at) {
		return nil
	}
	return mapping.ChannelProducts()
}

// CandidatesAllowedBy 施加客户的渠道约束。约束只做收窄：客户点名但映射并未提供的
// 渠道不会因此成为候选，因为运营企业只能在商业上可用的范围内选择。
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
