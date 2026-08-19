// Package domain 承载可见性与异常的领域模型：全程追踪投影、标准追踪里程碑与异常
// 信号。它基于各源上下文已经接受的事实形成只读旅程视图——不拥有节点、运输、关务等
// 源事实，不解除来源限制，也不使任何源事实失效。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue                = errors.New("visibility exception: blank value")
	ErrInvalidSourceFact         = errors.New("visibility exception: invalid accepted source fact")
	ErrInvalidClassification     = errors.New("visibility exception: invalid milestone classification")
	ErrInvalidTrackingProjection = errors.New("visibility exception: invalid tracking projection")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// SourceContext 是可提供已接受事实的源上下文封闭集合。投影只消费这五处的事实——
// 来源消息、原始扫描或外部状态码未经业务所有者接受，在类型上就没有入口。
type SourceContext uint8

const (
	SourceContextInvalid SourceContext = iota
	SourceParcelShipment
	SourceNetworkRouting
	SourceNodeOperations
	SourceTransportFulfillment
	SourceCustomsCompliance
)

func (source SourceContext) valid() bool {
	switch source {
	case SourceParcelShipment, SourceNetworkRouting, SourceNodeOperations,
		SourceTransportFulfillment, SourceCustomsCompliance:
		return true
	default:
		return false
	}
}

func (source SourceContext) String() string {
	switch source {
	case SourceParcelShipment:
		return "PARCEL_SHIPMENT"
	case SourceNetworkRouting:
		return "NETWORK_ROUTING"
	case SourceNodeOperations:
		return "NODE_OPERATIONS"
	case SourceTransportFulfillment:
		return "TRANSPORT_FULFILLMENT"
	case SourceCustomsCompliance:
		return "CUSTOMS_COMPLIANCE"
	default:
		return ""
	}
}

// TrackedParcelReference 指名被追踪的包裹。包裹身份与谱系属 parcel-shipment，这里
// 只引用。
type TrackedParcelReference struct{ requiredValue }

func NewTrackedParcelReference(value string) (TrackedParcelReference, error) {
	required, err := newRequiredValue("tracked parcel reference", value)
	return TrackedParcelReference{required}, err
}

// SourceFactReference 指名源上下文里那份已接受的事实本体。投影必须引用来源事实、
// 不得复制形成第二套含义不同的事实——引用类型正是「不复制」的落点。
type SourceFactReference struct{ requiredValue }

func NewSourceFactReference(value string) (SourceFactReference, error) {
	required, err := newRequiredValue("source fact reference", value)
	return SourceFactReference{required}, err
}

// SourceFactVersion 是源事实的版本。来源更正与有效性变化换版本，投影据以重新派生。
type SourceFactVersion struct{ requiredValue }

func NewSourceFactVersion(value string) (SourceFactVersion, error) {
	required, err := newRequiredValue("source fact version", value)
	return SourceFactVersion{required}, err
}

// SourceFactKind 是源上下文拥有的事实类型，不封闭枚举。标准里程碑映射按源上下文与
// 事实类型版本化登记，一行覆盖此后同类型事实，不得按单条事实引用建目录。
type SourceFactKind struct{ requiredValue }

func NewSourceFactKind(value string) (SourceFactKind, error) {
	required, err := newRequiredValue("source fact kind", value)
	return SourceFactKind{required}, err
}

// AcceptedSourceFactSpec 是一份已接受源事实引用所需的全部输入。
type AcceptedSourceFactSpec struct {
	Source  SourceContext
	Parcel  TrackedParcelReference
	Fact    SourceFactReference
	Kind    SourceFactKind
	Version SourceFactVersion
	// Supersedes 承载来源事实替代关系（CONTEXT 词条）：由源上下文随更正一并给出，
	// 指名本份取代的那一版。关系只在同一源上下文、同一事实引用的版本之间成立——
	// 事实引用不含版本，前身因而天然同引用；本上下文只登记不裁决，不从业务时间、
	// 到达先后或任何其他线索推断谁更正了谁。无前身是常态（首登事实留零值）。
	Supersedes  SourceFactVersion
	OccurredAt  time.Time
	EffectiveAt time.Time
	ReceivedAt  time.Time
}

// AcceptedSourceFact 是对源上下文已接受事实的只读引用。业务发生时间、有效时间与
// 接收时间分别保存（CONTEXT 硬句）——三者合并成一个时间字段，迟到事实与更正就再也
// 分不出「什么时候发生」与「什么时候才知道」。已接受事实必须携带源上下文拥有的事实
// 类型；幂等键仍是引用与版本，类型与前身引用进内容指纹、类型进里程碑映射键。
type AcceptedSourceFact struct {
	source      SourceContext
	parcel      TrackedParcelReference
	fact        SourceFactReference
	kind        SourceFactKind
	version     SourceFactVersion
	supersedes  SourceFactVersion
	occurredAt  time.Time
	effectiveAt time.Time
	receivedAt  time.Time
}

func NewAcceptedSourceFact(spec AcceptedSourceFactSpec) (AcceptedSourceFact, error) {
	if !spec.Source.valid() ||
		!spec.Parcel.valid() ||
		!spec.Fact.valid() ||
		!spec.Kind.valid() ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() ||
		spec.EffectiveAt.IsZero() ||
		spec.ReceivedAt.IsZero() {
		return AcceptedSourceFact{}, ErrInvalidSourceFact
	}
	// 指名自己为前身的「替代」是坏引用：沿用原版本号就是覆盖，不是更正
	// （与 TF 侧 TransportHandover.Correct 拒绝沿用原版本号同一条道理）。
	if spec.Supersedes.valid() && spec.Supersedes == spec.Version {
		return AcceptedSourceFact{}, ErrInvalidSourceFact
	}
	return AcceptedSourceFact{
		source:      spec.Source,
		parcel:      spec.Parcel,
		fact:        spec.Fact,
		kind:        spec.Kind,
		version:     spec.Version,
		supersedes:  spec.Supersedes,
		occurredAt:  spec.OccurredAt.UTC(),
		effectiveAt: spec.EffectiveAt.UTC(),
		receivedAt:  spec.ReceivedAt.UTC(),
	}, nil
}

func (fact AcceptedSourceFact) Source() SourceContext {
	return fact.source
}

func (fact AcceptedSourceFact) Parcel() TrackedParcelReference {
	return fact.parcel
}

func (fact AcceptedSourceFact) Fact() SourceFactReference {
	return fact.fact
}

func (fact AcceptedSourceFact) Kind() SourceFactKind {
	return fact.kind
}

func (fact AcceptedSourceFact) Version() SourceFactVersion {
	return fact.version
}

// Supersedes 给出源上下文指名的前身版本；首登事实第二个返回值为 false。
func (fact AcceptedSourceFact) Supersedes() (SourceFactVersion, bool) {
	return fact.supersedes, fact.supersedes.valid()
}

func (fact AcceptedSourceFact) OccurredAt() time.Time {
	return fact.occurredAt
}

func (fact AcceptedSourceFact) EffectiveAt() time.Time {
	return fact.effectiveAt
}

func (fact AcceptedSourceFact) ReceivedAt() time.Time {
	return fact.receivedAt
}

// MilestoneReference 指名一个标准追踪里程碑。里程碑目录及其映射属实例参数。
type MilestoneReference struct{ requiredValue }

func NewMilestoneReference(value string) (MilestoneReference, error) {
	required, err := newRequiredValue("milestone reference", value)
	return MilestoneReference{required}, err
}

// MappingVersionReference 指名判定所用的里程碑映射版本。映射必须有版本和适用范围
// （CONTEXT 硬句）——没有版本的映射改一次就没人说得清历史投影用的是哪套话。
type MappingVersionReference struct{ requiredValue }

func NewMappingVersionReference(value string) (MappingVersionReference, error) {
	required, err := newRequiredValue("mapping version reference", value)
	return MappingVersionReference{required}, err
}

// MilestoneClassification 是一份事实的里程碑归类结果：归入某里程碑，或如实保持
// 未归类。未归类是真话不是缺陷——无法可靠映射时不得为了时间线完整强行映射成
// 「运输中」或其他宽泛结果（CONTEXT 硬句）。
type MilestoneClassification struct {
	fact       AcceptedSourceFact
	milestone  MilestoneReference
	mapping    MappingVersionReference
	classified bool
}

// ClassifyMilestone 按映射版本把事实归入里程碑。映射版本必备——归类可追溯到哪套
// 映射话语。
func ClassifyMilestone(
	fact AcceptedSourceFact,
	milestone MilestoneReference,
	mapping MappingVersionReference,
) (MilestoneClassification, error) {
	if !fact.source.valid() || !milestone.valid() || !mapping.valid() {
		return MilestoneClassification{}, ErrInvalidClassification
	}
	return MilestoneClassification{
		fact:       fact,
		milestone:  milestone,
		mapping:    mapping,
		classified: true,
	}, nil
}

// LeaveUnclassified 如实记下「这份事实在此映射版本下无法可靠归类」。映射版本仍然
// 必备：连「按哪套话语归不进」都说不出的未归类无从续办。
func LeaveUnclassified(
	fact AcceptedSourceFact,
	mapping MappingVersionReference,
) (MilestoneClassification, error) {
	if !fact.source.valid() || !mapping.valid() {
		return MilestoneClassification{}, ErrInvalidClassification
	}
	return MilestoneClassification{fact: fact, mapping: mapping}, nil
}

func (classification MilestoneClassification) Fact() AcceptedSourceFact {
	return classification.fact
}

// Milestone 报告归入的里程碑及是否归类成功。未归类时第二个返回值为 false。
func (classification MilestoneClassification) Milestone() (MilestoneReference, bool) {
	return classification.milestone, classification.classified
}

func (classification MilestoneClassification) MappingVersion() MappingVersionReference {
	return classification.mapping
}

// ProjectionVersionID 是投影版本标识。重新派生换版本，原版本继续保留。
type ProjectionVersionID struct{ requiredValue }

func NewProjectionVersionID(value string) (ProjectionVersionID, error) {
	required, err := newRequiredValue("projection version ID", value)
	return ProjectionVersionID{required}, err
}

// TrackingProjection 是面向明确包裹的只读旅程视图版本：已接受事实的语义化编排。
// 它可以重新派生，但不保存第二套源事实——字段里只有引用与归类，没有任何源事实的
// 内容拷贝。
type TrackingProjection struct {
	version      ProjectionVersionID
	parcel       TrackedParcelReference
	entries      []MilestoneClassification
	priorVersion ProjectionVersionID
	derivedAt    time.Time
}

// DeriveTrackingProjection 依据归类完成的事实集派生投影版本。全部条目必须属于同一
// 包裹——旅程视图面向明确包裹，混入别人的事实就是把两条轨迹拼成一条；条目按业务
// 发生时间自然排序由调用方保证读侧便利，这里不强制（迟到事实的插入位置属编排）。
func DeriveTrackingProjection(
	version ProjectionVersionID,
	parcel TrackedParcelReference,
	entries []MilestoneClassification,
	derivedAt time.Time,
) (TrackingProjection, error) {
	if !version.valid() || !parcel.valid() || len(entries) == 0 || derivedAt.IsZero() {
		return TrackingProjection{}, ErrInvalidTrackingProjection
	}
	for _, entry := range entries {
		if !entry.fact.source.valid() || entry.fact.parcel != parcel {
			return TrackingProjection{}, ErrInvalidTrackingProjection
		}
	}
	return TrackingProjection{
		version:   version,
		parcel:    parcel,
		entries:   append([]MilestoneClassification(nil), entries...),
		derivedAt: derivedAt.UTC(),
	}, nil
}

func (projection TrackingProjection) Version() ProjectionVersionID {
	return projection.version
}

func (projection TrackingProjection) Parcel() TrackedParcelReference {
	return projection.parcel
}

func (projection TrackingProjection) Entries() []MilestoneClassification {
	return append([]MilestoneClassification(nil), projection.entries...)
}

func (projection TrackingProjection) DerivedAt() time.Time {
	return projection.derivedAt
}

// PriorVersion 只在重派生版本上给出，指回被替代的那一版。
func (projection TrackingProjection) PriorVersion() (ProjectionVersionID, bool) {
	return projection.priorVersion, projection.priorVersion.valid()
}

// Rederive 依据迟到事实、来源更正或映射版本变化形成新的当前投影（CONTEXT 生命周期）：
// 换版本、换条目、指回原版；原投影版本继续保留——已经发布的客户信息靠这条关系追加
// 更正，不靠改写。
func (projection TrackingProjection) Rederive(
	version ProjectionVersionID,
	entries []MilestoneClassification,
	derivedAt time.Time,
) (TrackingProjection, error) {
	if version == projection.version {
		return TrackingProjection{}, ErrInvalidTrackingProjection
	}
	rederived, err := DeriveTrackingProjection(version, projection.parcel, entries, derivedAt)
	if err != nil {
		return TrackingProjection{}, err
	}
	rederived.priorVersion = projection.version
	return rederived, nil
}

// RehydrateTrackingProjection 从已存版本行重建投影——当前版或留存的历史版皆可
// （ADR-0065：版本只增不改写，历史可按版本读回）；前身关系由 prior 指名。
func RehydrateTrackingProjection(
	version ProjectionVersionID,
	parcel TrackedParcelReference,
	entries []MilestoneClassification,
	derivedAt time.Time,
	prior ProjectionVersionID,
) (TrackingProjection, error) {
	projection, err := DeriveTrackingProjection(version, parcel, entries, derivedAt)
	if err != nil {
		return TrackingProjection{}, err
	}
	if prior.valid() {
		if prior == version {
			return TrackingProjection{}, ErrInvalidTrackingProjection
		}
		projection.priorVersion = prior
	}
	return projection, nil
}
