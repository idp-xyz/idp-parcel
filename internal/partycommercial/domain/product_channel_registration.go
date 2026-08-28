package domain

import "errors"

var (
	// ErrInvalidChannelBinding 拒绝立不住的渠道绑定格：已配置绑定必须至少一个渠道
	// 产品引用、引用逐个有效且不重复；零值绑定（既没配置也没显式声明未配置）同样
	// 立不住——“未配置”是登记者说出的一句商业声明，不是忘了填的空。
	ErrInvalidChannelBinding = errors.New("party commercial: invalid product channel binding")
	// ErrInvalidMappingRegistration 拒绝立不住的映射登记信封。
	ErrInvalidMappingRegistration = errors.New("party commercial: invalid product channel mapping registration")
)

// ProductChannelMappingID 是一笔产品—渠道映射在登记册上的稳定标识。判据同
// RelationshipID：映射正文是产品×渠道×区间，登记册需要一个可回指的键，键在信封上。
type ProductChannelMappingID struct{ requiredValue }

func NewProductChannelMappingID(value string) (ProductChannelMappingID, error) {
	required, err := newRequiredValue("product channel mapping id", value)
	return ProductChannelMappingID{required}, err
}

// MappingBasisReference 指向一次映射登记所依据的东西（商业决定、协议或渠道接入
// 证据）。判据同 IdentityBasisReference，但映射不是身份，两个引用不共用一个类型。
type MappingBasisReference struct{ requiredValue }

func NewMappingBasisReference(value string) (MappingBasisReference, error) {
	required, err := newRequiredValue("mapping basis reference", value)
	return MappingBasisReference{required}, err
}

// ProductChannelBinding 是映射登记上的渠道绑定格，两格封闭：已配置（一个或多个渠道
// 产品标识引用，CONTEXT“一个服务产品版本可以映射一个或多个渠道产品”）或显式“未配置”
// （登记者声明该产品尚无可用渠道候选）。渠道只以标识引用，渠道本体不在本上下文预造
// （ADR-0072 否决预拟渠道表，渠道本体等 PAR-INT-01 的接入证据）。
//
// declared 让零值立不住：显式“未配置”与忘了填必须可分辨——前者是登记进册的商业
// 声明，后者是输入缺件。
type ProductChannelBinding struct {
	declared bool
	channels []ChannelProductReference
}

// NewConfiguredChannelBinding 建立已配置的绑定格。对渠道集合的门与
// NewProductChannelMapping 同款：至少一个、逐个有效、不重复。
func NewConfiguredChannelBinding(channels []ChannelProductReference) (ProductChannelBinding, error) {
	if len(channels) == 0 {
		return ProductChannelBinding{}, ErrInvalidChannelBinding
	}
	seen := make(map[ChannelProductReference]struct{}, len(channels))
	for _, channel := range channels {
		if !channel.valid() {
			return ProductChannelBinding{}, ErrInvalidChannelBinding
		}
		if _, exists := seen[channel]; exists {
			return ProductChannelBinding{}, ErrInvalidChannelBinding
		}
		seen[channel] = struct{}{}
	}
	return ProductChannelBinding{
		declared: true,
		channels: append([]ChannelProductReference(nil), channels...),
	}, nil
}

// UnconfiguredChannelBinding 是显式的“未配置”声明：该产品尚无可用渠道候选，不为
// 任何新的渠道决策提供候选。渠道接入后以新的登记修订配置绑定，本格保留为历史修订。
func UnconfiguredChannelBinding() ProductChannelBinding {
	return ProductChannelBinding{declared: true}
}

func (binding ProductChannelBinding) Configured() bool {
	return len(binding.channels) > 0
}

// Channels 交回引用副本。未配置的绑定交回空列表——它本来就没有候选可交。
func (binding ProductChannelBinding) Channels() []ChannelProductReference {
	return append([]ChannelProductReference(nil), binding.channels...)
}

func (binding ProductChannelBinding) valid() bool {
	return binding.declared
}

// ProductChannelMappingSpec 是一笔映射登记的正文：服务产品版本引用 + 渠道绑定格 +
// 有效区间 + 登记依据。产品以（对象标识+版本号）引用——版本正文在版本册上，映射
// 不抄第二份；引用是否指着一份已发布且未收尾的服务产品版本由写入用例把门，信封
// 不复核一份不在场的版本壳。
type ProductChannelMappingSpec struct {
	Product        CommercialObjectID
	ProductVersion CommercialVersionLabel
	Binding        ProductChannelBinding
	Effective      EffectiveInterval
	Basis          MappingBasisReference
}

// ProductChannelMappingRegistration 给一笔产品—渠道映射一个登记册身份：租户 + 映射
// 标识 + 修订。修订从 1 起连续递增，调整绑定或区间都形成新修订，不原地改写
// （CONTEXT“产品—渠道映射的登记按修订版本化且不可覆盖”）。
//
// 它刻意不包 ProductChannelMapping：那是解析侧的形状（携完整 ServiceProduct，含形态），
// 包进来会让登记快照抄一份版本册拥有的正文。登记面只持引用。
type ProductChannelMappingRegistration struct {
	tenant   TenantID
	id       ProductChannelMappingID
	revision int
	spec     ProductChannelMappingSpec
}

func NewProductChannelMappingRegistration(
	tenant TenantID,
	id ProductChannelMappingID,
	revision int,
	spec ProductChannelMappingSpec,
) (ProductChannelMappingRegistration, error) {
	if !tenant.valid() || !id.valid() || revision < 1 ||
		!spec.Product.valid() || !spec.ProductVersion.valid() ||
		!spec.Binding.valid() || !spec.Effective.valid() || !spec.Basis.valid() {
		return ProductChannelMappingRegistration{}, ErrInvalidMappingRegistration
	}
	return ProductChannelMappingRegistration{
		tenant:   tenant,
		id:       id,
		revision: revision,
		spec:     spec,
	}, nil
}

func (registration ProductChannelMappingRegistration) Tenant() TenantID {
	return registration.tenant
}

func (registration ProductChannelMappingRegistration) ID() ProductChannelMappingID {
	return registration.id
}

func (registration ProductChannelMappingRegistration) Revision() int {
	return registration.revision
}

func (registration ProductChannelMappingRegistration) Product() CommercialObjectID {
	return registration.spec.Product
}

func (registration ProductChannelMappingRegistration) ProductVersion() CommercialVersionLabel {
	return registration.spec.ProductVersion
}

func (registration ProductChannelMappingRegistration) Binding() ProductChannelBinding {
	return registration.spec.Binding
}

func (registration ProductChannelMappingRegistration) Effective() EffectiveInterval {
	return registration.spec.Effective
}

func (registration ProductChannelMappingRegistration) Basis() MappingBasisReference {
	return registration.spec.Basis
}
