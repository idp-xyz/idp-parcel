package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件是渠道候选装配（票 `label-channel/12`）。落点由票 `01` 的裁决决定：择优住
// `parcel-shipment`，装配随之落本上下文的 `adapters/partycommercial/`。
//
// **收窄这件事本身不在这里实现。** 「客户约束只收窄不扩张」是 party-commercial 的规则，
// 写在它自己的 ProductChannelMapping.CandidatesAllowedBy 上并由它自己的测试守着。本装配
// 只是**调用**它——把两个读口的答复拼成一份映射，再问它这个时点有哪些候选。照票面红线，
// 不在本上下文复制第二套映射。

var (
	// ErrChannelConstraintNotConfigured 说这一票的客户渠道约束此刻答不上来。
	//
	// 它必须与「客户明确没有约束」分成两格，因为 CandidatesAllowedBy 在约束为空时交回
	// **全部**可用渠道——那对它自己是对的，但把「答不上来」折成空切片传进去，一次读取
	// 失败就静默变成一次范围放大，而放大出来的渠道可能正是客户禁止过的那一个。
	//
	// 约束的登记形状属实例半边且今天尚不存在（PC CONTEXT 有「可复用渠道约束」这个词条，
	// Go 里没有对应登记册）。机制这里只把「没有配置」如实交出来，不代拟一个范围。
	ErrChannelConstraintNotConfigured = errors.New("parcel shipment: channel constraint not configured")
	// ErrProductChannelMappingNotRegistered 说这笔映射从未登记。它与「登记了但此刻没有
	// 候选」分开：后者是映射如实作过答（到期、或被约束收窄到空），前者是根本没人登记过
	// 这一笔，续办是去登记而不是换渠道。
	ErrProductChannelMappingNotRegistered = errors.New("parcel shipment: product channel mapping not registered")
	// ErrMappedServiceProductNotEffective 说映射指名的那份服务产品版本此刻不在册或不已生效。
	//
	// 停下而不是照样产候选：映射的候选逻辑以「属于一份已生效产品」为前提，一份草稿、已到期
	// 或已退役的产品继续给出新候选，等于让收尾过的商业决定重新参与新的采购。
	ErrMappedServiceProductNotEffective = errors.New("parcel shipment: mapped service product is not effective")
)

// ChannelConstraint 是一次装配适用的客户渠道约束，三态：
// 限定为若干渠道、明确无约束、未配置（零值）。
//
// 零值读作未配置，与 psdomain.ChannelCandidateCost 的零值读作未确立同一手法：把最危险
// 的那一格放在零值上，漏判的方向因此是停下而不是放行。
type ChannelConstraint struct {
	declared bool
	allowed  []pcdomain.ChannelProductReference
}

// ConstrainedToChannels 造一份「限定为这几个渠道」的约束。
//
// 它拒空列表：一份点名了零个渠道的约束与「没有约束」在取值上无法分开，而两者的后果相反
// （前者一个都不许，后者全都可以）。客户确实没有约束时用 UnconstrainedChannels。
func ConstrainedToChannels(allowed ...pcdomain.ChannelProductReference) (ChannelConstraint, error) {
	if len(allowed) == 0 {
		return ChannelConstraint{}, ErrChannelConstraintNotConfigured
	}
	return ChannelConstraint{
		declared: true,
		allowed:  append([]pcdomain.ChannelProductReference(nil), allowed...),
	}, nil
}

// UnconstrainedChannels 造一份「客户明确没有渠道约束」。它与未配置的零值分开，正是本类型
// 存在的理由——见 ErrChannelConstraintNotConfigured。
func UnconstrainedChannels() ChannelConstraint {
	return ChannelConstraint{declared: true}
}

// Declared 报告约束是否答上来了。false 即未配置——调用方不得据此当作无约束。
func (constraint ChannelConstraint) Declared() bool {
	return constraint.declared
}

// ChannelConstraintSource 交回某次装配适用的客户渠道约束。
//
// 接口留在本包而不进 psports：约束的取数路径是消费方自己的实例半边（同 ADR-0025 下
// ResolutionKeySource 留在本包的那条理由），提供方不拥有「这次装配该用哪份约束」。
type ChannelConstraintSource interface {
	ChannelConstraintFor(ctx context.Context, query ChannelCandidateQuery) (ChannelConstraint, error)
}

// ProductChannelMappingReader 是产品—渠道映射登记册的只读半边。
//
// 不直接依赖 pcports.ProductChannelMappingRegistry：那个口带着 SaveMapping，而装配是纯
// 读路径。依赖整口会让每个调用方（含测试替身）持有一个它绝不该调的写方法。
type ProductChannelMappingReader interface {
	LoadLatestMapping(
		ctx context.Context,
		tenant pcdomain.TenantID,
		mapping pcdomain.ProductChannelMappingID,
	) (pcdomain.ProductChannelMappingRegistration, bool, error)
}

// ChannelCandidateQuery 是一次装配的输入：在哪个租户的哪个商业范围下、按哪笔映射、
// 对准哪个时点。时点由调用方给而不是取当下时钟——同一份委托重算两次必须得到同一批
// 候选，读时钟会让它随调用时刻漂移。
type ChannelCandidateQuery struct {
	Tenant  pcdomain.TenantID
	Scope   pcdomain.CommercialScopeReference
	Mapping pcdomain.ProductChannelMappingID
	At      time.Time
}

// ChannelCandidateAssemblerDeps 收拢三个协作方。
type ChannelCandidateAssemblerDeps struct {
	Mappings    ProductChannelMappingReader
	Publication pcports.CommercialPublicationView
	Constraints ChannelConstraintSource
}

// ChannelCandidateAssembler 从产品—渠道映射与客户渠道约束装出渠道择优所需的候选集合。
type ChannelCandidateAssembler struct {
	mappings    ProductChannelMappingReader
	publication pcports.CommercialPublicationView
	constraints ChannelConstraintSource
}

func NewChannelCandidateAssembler(deps ChannelCandidateAssemblerDeps) *ChannelCandidateAssembler {
	return &ChannelCandidateAssembler{
		mappings:    deps.Mappings,
		publication: deps.Publication,
		constraints: deps.Constraints,
	}
}

// AssembleChannelCandidates 交回该时点可参与择优的渠道候选。
func (assembler *ChannelCandidateAssembler) AssembleChannelCandidates(
	ctx context.Context,
	query ChannelCandidateQuery,
) ([]psdomain.ChannelCandidateID, error) {
	// 约束先问，且答不上来即停：后面每一步都会让候选集合变大，先放行再补判就等于让
	// 一次读取失败先把范围撑开。
	constraint, err := assembler.constraints.ChannelConstraintFor(ctx, query)
	if err != nil {
		return nil, err
	}
	if !constraint.Declared() {
		return nil, ErrChannelConstraintNotConfigured
	}

	mapping, err := assembler.mappingFor(ctx, query)
	if err != nil {
		return nil, err
	}
	// 收窄由提供方的规则做，本装配不判断谁该留下。
	allowed := mapping.CandidatesAllowedBy(constraint.allowed, query.At)

	candidates := make([]psdomain.ChannelCandidateID, 0, len(allowed))
	for _, channel := range allowed {
		candidate, err := psdomain.NewChannelCandidateID(channel.String())
		if err != nil {
			return nil, fmt.Errorf("%w: channel candidate %q: %v", ErrUntranslatableAnswer, channel.String(), err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

// mappingFor 把两个读口的答复拼成一份 ProductChannelMapping。
//
// 这一次 join 落在消费侧是有代价的，但三条路里它最轻：登记册交回的 Registration 只带产品
// 的标识与版本号，而候选逻辑住在 ProductChannelMapping 上、后者要一份**已生效**的服务产品
// 版本。那条不变量不是仪式——它正是「退役产品不再给出新候选」的守卫，绕过去就等于让一份
// 已收尾的产品继续产候选，所以这里如实去发布册取回它，取不到就停。
func (assembler *ChannelCandidateAssembler) mappingFor(
	ctx context.Context,
	query ChannelCandidateQuery,
) (pcdomain.ProductChannelMapping, error) {
	registration, found, err := assembler.mappings.LoadLatestMapping(ctx, query.Tenant, query.Mapping)
	if err != nil {
		return pcdomain.ProductChannelMapping{}, fmt.Errorf("load product channel mapping: %w", err)
	}
	if !found {
		return pcdomain.ProductChannelMapping{}, ErrProductChannelMappingNotRegistered
	}

	registry, err := assembler.publication.LoadForScope(ctx, query.Tenant, query.Scope)
	if err != nil {
		return pcdomain.ProductChannelMapping{}, fmt.Errorf("load commercial publication: %w", err)
	}
	product, found := effectiveServiceProductIn(registry, registration)
	if !found {
		return pcdomain.ProductChannelMapping{}, ErrMappedServiceProductNotEffective
	}

	mapping, err := pcdomain.NewProductChannelMapping(
		product,
		registration.Binding().Channels(),
		registration.Effective(),
	)
	if err != nil {
		return pcdomain.ProductChannelMapping{}, fmt.Errorf("%w: product channel mapping: %v", ErrUntranslatableAnswer, err)
	}
	return mapping, nil
}

// effectiveServiceProductIn 按登记指名的产品标识与版本号，从册子里挑出那一份服务产品。
//
// 挑选写在这里而不是复用提供方的同类查找（它是私有的），但挑的只是**哪一条**，不是
// 「它算不算数」——后者仍由 NewServiceProduct 的不变量与册子里登记的状态决定。
func effectiveServiceProductIn(
	registry *pcdomain.CommercialRegistry,
	registration pcdomain.ProductChannelMappingRegistration,
) (pcdomain.ServiceProduct, bool) {
	if registry == nil {
		return pcdomain.ServiceProduct{}, false
	}
	for _, product := range registry.ServiceProducts() {
		version := product.Version()
		if version.ObjectID() == registration.Product() && version.Version() == registration.ProductVersion() {
			return product, true
		}
	}
	return pcdomain.ServiceProduct{}, false
}
