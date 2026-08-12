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
