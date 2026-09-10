package partycommercial

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是派送要求缝三——**交付条件引用**（ADR-0114 决定三；ADR-0133；票 tf-segment-lifecycle-closure/14）的消费侧适配器。
// 与同包的 CarrierIdentityDirectory 并列、各答各的，不合成「PC 门面」：那只答承运主体身份登了没有，这只答某个载运对象
// 有没有交付条件引用，两件事的所有者、键与失败形状都不同。
//
// 缝要跨两个提供方（ADR-0133 决定二）：对象 → 委托接受时固定的商业解析回指归 parcel-shipment（按租户 + 包裹身份答），
// 回指 → 有没有交付条件归 party-commercial（按租户 + 回指答）。两份翻译都在本适配器内，两个提供方互不认识对方的词；
// 本上下文不自己解析商业依据、不读任何条件内容——交付方式允许集、收件范围规则是实例半边，一个字都不进 TF（票面红线）。

// CommercialResolutionReferenceSource 是本适配器向 parcel-shipment 取商业解析回指的窄口。真实装配交给 PS 的
// adapters/postgres.ShipmentRequests（它兼实现 PS ports.CommercialResolutionReferenceView）；这里只声明读的那一个方法，
// 不 import PS 的 application / ports，判据同 BusinessPartySource。
type CommercialResolutionReferenceSource interface {
	LoadCommercialResolutionReference(
		ctx context.Context,
		tenant psdomain.TenantID,
		parcel psdomain.DeclaredParcelID,
	) (psdomain.CommercialResolutionID, bool, error)
}

// DeliveryConditionReferenceSource 是本适配器向 party-commercial 问「这份回指的闭包有没有交付条件」的窄口。真实装配交给
// PC 的 adapters/postgres.NewDeliveryConditions（它实现 PC ports.DeliveryConditionView）；同样只声明这一个方法。
type DeliveryConditionReferenceSource interface {
	LoadDeliveryConditionReference(
		ctx context.Context,
		tenant pcdomain.TenantID,
		resolution pcdomain.ResolutionID,
	) (pcdomain.DeliveryConditionReference, bool, error)
}

// DeliveryConditionSource 实现 tfports.DeliveryConditionSource：两段翻译各自封闭、逐格有落点（票面做法「适配器」那一步）。
//
// 第一段（PS 三格）：回指 → 进第二段；没有 → RequirementMissing；error → 原样上抛。
// 第二段（PC 四格）：交付条件引用 → RequirementResolved + 回指的 String() **逐字**（引用就是这个回指，ADR-0133 决定一；
// 两段式串只许 PC 一处拼，ADR-0080 决定七，本上下文不拆不拼）；没有交付条件 → RequirementMissing；闭包不在场 error 与
// 闭包未采用客户合同版本 error → 原样上抛，**不折成 Missing**（对一份已接受委托的回指闭包不在场是提供方缺数据，不是
// 「没登条件」）。任一侧集外取值 → ErrUntranslatableAnswer。
//
// **两个「没有」都译成 RequirementMissing——取甲**（票面做法「先定两个『没有』在 TF 结果上的落法」那一步的默认；
// ADR-0133 越权风险点 1），但它们是两行、来自两个所有者、恢复动作不同（ADR-0029）：
//   - PS 的「没有」：对象不属任何已接受委托的成员集合（集运单元、不可见对象、仅已提交），没有采用的合同可指——要去
//     parcel-shipment 那一侧问这个对象为什么不在册；
//   - PC 的「没有」：闭包在场、合同也采用了，只是服务产品与客户合同两层都没登交付条件声明——商业责任方去 party-commercial
//     登声明（这一族的表与写口 party-commercial-context-gaps/11 已落，今天缺的是租户登的声明——实例半边；没有租户
//     登过之前每一份都会答这一格，那是真话）。
//
// 执行器今天只有一格 REQUIREMENT_MISSING / DELIVERY_CONDITION 承接两行；要分子原因是 TF 结果形状的事，等三条缝都接上
// 再看要不要统一加（票面同一步写的默认判据），届时改这里两行与执行器，不改端口。
//
// `CarriedObjectReference` 今天没有种类维（票 12 评审记过）：集运单元引用原样译成 PS 声明包裹身份去问，PS 按统一
// 不可见结果答没有 → Missing；本票不立种类维，要在 TF 侧先分流得另立票让两条缝同时短路。
type DeliveryConditionSource struct {
	resolutions CommercialResolutionReferenceSource
	conditions  DeliveryConditionReferenceSource
}

// NewDeliveryConditionSource 两只读口都要：缺一只，对应那一段就永远答不出，而「答不出」在读口上会长成 error，日后有人会
// 把它读成提供方坏了（判据同 NewCarrierIdentityDirectory）。
func NewDeliveryConditionSource(
	resolutions CommercialResolutionReferenceSource,
	conditions DeliveryConditionReferenceSource,
) (*DeliveryConditionSource, error) {
	if resolutions == nil {
		return nil, fmt.Errorf("transport fulfillment partycommercial adapter: commercial resolution reference source is nil")
	}
	if conditions == nil {
		return nil, fmt.Errorf("transport fulfillment partycommercial adapter: delivery condition reference source is nil")
	}
	return &DeliveryConditionSource{resolutions: resolutions, conditions: conditions}, nil
}

var _ tfports.DeliveryConditionSource = (*DeliveryConditionSource)(nil)

// LoadDeliveryConditions 按（租户，载运对象）先向 PS 取回指、再向 PC 问有没有交付条件，译成端口两格。
func (source *DeliveryConditionSource) LoadDeliveryConditions(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
) (string, tfports.RequirementResolution, error) {
	// 第一段：parcel-shipment。租户与包裹身份在两个上下文里是同一串字面（与票 12 的地点适配器同一条约定）。
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(object.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	resolution, present, err := source.resolutions.LoadCommercialResolutionReference(ctx, psTenant, parcel)
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("load delivery conditions: parcel shipment: %w", err)
	}
	if !present {
		// PS 的「没有」：对象无采用的合同。不进第二段——没有回指可问。
		return "", tfports.RequirementMissing, nil
	}
	if resolution.String() == "" {
		// present 却没有回指：PS 读口违约，不是「没有」。
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: parcel shipment answered present with an empty resolution reference",
			ErrUntranslatableAnswer)
	}

	// 第二段：party-commercial。回指按 ADR-0027 的回指入口重建成 PC 的解析标识，不拆不拼。
	pcTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	pcResolution, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: resolution reference: %v", ErrUntranslatableAnswer, err)
	}
	reference, declared, err := source.conditions.LoadDeliveryConditionReference(ctx, pcTenant, pcResolution)
	if err != nil {
		// 含 pcdomain.ErrDeliveryConditionClosureAbsent 与 ErrDeliveryConditionContractNotAdopted：提供方缺数据 / 装配缺陷，
		// 原样上抛让执行器落 DELIVERY_CONDITION_SOURCE_UNAVAILABLE，不冒充「没登条件」。
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("load delivery conditions: party commercial: %w", err)
	}
	if !declared {
		// PC 的「没有」：合同无交付条件声明。
		return "", tfports.RequirementMissing, nil
	}
	if reference.String() == "" || reference.Resolution() != pcResolution {
		// declared 却没有引用、或引用指着另一份闭包：PC 读口违约，不是任何一格业务答案。
		return "", tfports.RequirementResolutionInvalid, fmt.Errorf("%w: party commercial answered %q for resolution %q",
			ErrUntranslatableAnswer, reference, pcResolution)
	}
	return resolution.String(), tfports.RequirementResolved, nil
}
