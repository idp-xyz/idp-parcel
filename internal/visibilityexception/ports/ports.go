// Package ports 定义 visibility-exception 应用层与外界的边界。领域包不依赖 HTTP/pgx
// 的纪律与其余上下文一致。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type Clock interface {
	Now() time.Time
}

// FactKey 是已接受源事实的幂等键：来源上下文+事实引用+来源版本。
type FactKey struct {
	Source  domain.SourceContext
	Fact    domain.SourceFactReference
	Version domain.SourceFactVersion
}

// FactRecord 是一份已保存的事实及其内容指纹（同键异内容是来源冲突不是重放）。
type FactRecord struct {
	Key           FactKey
	ContentDigest string
	Fact          domain.AcceptedSourceFact
}

type FactSaveOutcome uint8

const (
	FactSaveOutcomeInvalid FactSaveOutcome = iota
	FactSaved
	FactAlreadyRecorded
)

// AcceptedFactStore 按幂等键找回并保存事实；FindByParcel 交回该包裹全部已接受事实
// ——投影派生的输入。事实只增不删（来源更正是新版本新键）。
type AcceptedFactStore interface {
	FindByKey(ctx context.Context, key FactKey) (FactRecord, bool, error)
	FindByParcel(ctx context.Context, parcel domain.TrackedParcelReference) ([]FactRecord, error)
	Save(ctx context.Context, record FactRecord) (FactSaveOutcome, error)
}

// MilestoneAnswer 是映射目录对一份事实的归类答复。
type MilestoneAnswer struct {
	Milestone  domain.MilestoneReference
	Classified bool
	Mapping    domain.MappingVersionReference
}

// MilestoneMappingView 按版本化映射归类事实。第二个返回值为 false 即「映射目录未
// 配置」——那不是未决而是整体无法可靠映射，事实按未归类进投影（CONTEXT：不强行
// 映射）；依赖调不通作为错误返回。
type MilestoneMappingView interface {
	ClassifyFact(ctx context.Context, fact domain.AcceptedSourceFact) (MilestoneAnswer, bool, error)
}

// ProjectionStore 保存当前投影版本。原版本由重派生的指回关系承担历史，库只管当前。
type ProjectionStore interface {
	FindCurrent(ctx context.Context, parcel domain.TrackedParcelReference) (domain.TrackingProjection, bool, error)
	Save(ctx context.Context, projection domain.TrackingProjection) error
}

// ProjectionIdentityFactory 签发投影版本标识。
type ProjectionIdentityFactory interface {
	NextProjectionVersionID(ctx context.Context) (domain.ProjectionVersionID, error)
}

// ProjectionHandoffIntent 把新派生的投影交给适用下游（客户视图派生正是消费者）。
// 意图由投影版本认领，重放重发同一份（ADR-0043）。
type ProjectionHandoffIntent struct {
	Projection domain.TrackingProjection
}

// ProjectionHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017 的
// Bento/Outbox 闸门。
type ProjectionHandoff interface {
	HandOffProjection(ctx context.Context, intent ProjectionHandoffIntent) error
}

// DimensionDisclosure 是披露策略对客户视图一维的答复：获准展示时带内容来处，
// 内容尚未到位时待确认，授权或披露规则不允许时不展示。State 取领域的封闭三态，
// 翻译成维度由编排经领域构造器完成——展示无内容在那里立不起来。
type DimensionDisclosure struct {
	State   domain.DimensionState
	Content domain.ViewContentReference
}

// DisclosureAnswer 是披露策略对四个展示维各自的答复（公开里程碑、获准展示的 ETA、
// 客户服务终局、追踪说明）。
type DisclosureAnswer struct {
	Milestones DimensionDisclosure
	ETA        DimensionDisclosure
	Final      DimensionDisclosure
	Note       DimensionDisclosure
}

// DisclosurePolicyView 回答「对这个货主客户账户与这份投影，四维各自获准展示什么」。
// 客户可见性按服务产品、客户合同和信息披露规则形成（CONTEXT），那些都不属本上下文——
// 这里只消费答复，绝不自行推导一份。
//
// 第二个返回值为 false 即「披露规则未配置」——真实披露范围、地点粒度与通知策略属
// 待登记实例参数，没有租户就没有规则。那不是未决而是如实的空白：四维全部待确认，
// 如实说等，不虚构可见性，也不把没人作过的披露决定说成「不展示」。依赖调不通作为
// 错误返回，由应用层形成未决。
type DisclosurePolicyView interface {
	AssessDisclosure(
		ctx context.Context,
		customer domain.CustomerAccountReference,
		projection domain.TrackingProjection,
	) (DisclosureAnswer, bool, error)
}

// CustomerViewStore 保存客户当前视图版本。键含客户账户——账户隔离是字段不是约定，
// 按包裹一个键会让两个客户的授权范围共用一份视图。原版本由替代关系承担历史，库只管
// 当前。
type CustomerViewStore interface {
	FindCurrent(
		ctx context.Context,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
	) (domain.CustomerTrackingView, bool, error)
	Save(ctx context.Context, view domain.CustomerTrackingView) error
}

// CustomerViewIdentityFactory 签发客户视图版本标识。与投影身份工厂分开：投影派生与
// 视图派生由不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type CustomerViewIdentityFactory interface {
	NextCustomerViewVersionID(ctx context.Context) (domain.CustomerViewVersionID, error)
}

// CustomerViewHandoffIntent 把新发布的客户视图交给适用下游（门户展示、通知判断的
// 输入）。意图由视图版本认领，重放重发同一份（ADR-0043）。
type CustomerViewHandoffIntent struct {
	View domain.CustomerTrackingView
}

// CustomerViewHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type CustomerViewHandoff interface {
	HandOffCustomerView(ctx context.Context, intent CustomerViewHandoffIntent) error
}
