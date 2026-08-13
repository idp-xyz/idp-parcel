package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedPublication 是发布重建入口因快照数据本身而拒绝时给出的理由。
// 与 ErrInvalidCommercialVersion 分开：后者说「此刻要构造的这份不合规则」，前者说
// 「这份已经登记过的东西不可能是本上下文发布出来的」——处置是去查库里那一行或写它
// 的适配器（与其他上下文的重建哨兵同一条分格纪律）。
var ErrInvalidRehydratedPublication = errors.New("party commercial: invalid rehydrated commercial version")

// RehydrateCommercialVersionSpec 是一份已发布版本在库里的样子。字段一律当数据收下，
// 不重算（ADR-0028）：批准与引用的把门在 Publish 那道门，这里只挡一行坏数据变成一份
// 看起来合法的版本。
type RehydrateCommercialVersionSpec struct {
	TenantID      TenantID
	Kind          CommercialObjectKind
	ObjectID      CommercialObjectID
	Version       CommercialVersionLabel
	Scope         CommercialScopeReference
	ContentDigest CommercialContentDigest
	Effective     EffectiveInterval
	Status        CommercialVersionStatus
	Approval      ApprovalBasis
	PublishedAt   time.Time
	EffectiveAt   time.Time
	ClosedAt      time.Time
	RetirementRef RetirementReference
	Successor     CommercialVersionLabel
	References    map[CommercialObjectKind]CommercialObjectID
}

// RehydrateCommercialVersion 从库里读到的产物重建一份已发布版本。
//
// 校验对齐发布与生命周期各道门的不变量：登记册只收已发布（草稿拒）；批准完备且发布
// 不早于批准；`已生效`起必带生效时刻；三种收尾态必带收尾时刻，退役带决定引用、替代带
// 后继且不自指；引用经 declaredReferences 同一套整理（自引用拒）。
func RehydrateCommercialVersion(spec RehydrateCommercialVersionSpec) (CommercialVersion, error) {
	if !spec.TenantID.valid() || spec.Kind == CommercialObjectKindInvalid ||
		!spec.ObjectID.valid() || !spec.Version.valid() || !spec.Scope.valid() ||
		!spec.ContentDigest.valid() || !spec.Effective.valid() {
		return CommercialVersion{}, rehydratedPublicationRefusal("版本身份、范围、摘要或区间缺失")
	}
	if !spec.Approval.valid() {
		return CommercialVersion{}, rehydratedPublicationRefusal("批准依据缺失——没有批准的发布不可能越过发布门")
	}
	if spec.PublishedAt.IsZero() || spec.PublishedAt.Before(spec.Approval.ApprovedAt()) {
		return CommercialVersion{}, rehydratedPublicationRefusal("发布时间缺失或早于批准")
	}

	switch spec.Status {
	case CommercialVersionPublished:
		if !spec.EffectiveAt.IsZero() || !spec.ClosedAt.IsZero() ||
			spec.RetirementRef.valid() || spec.Successor.valid() {
			return CommercialVersion{}, rehydratedPublicationRefusal("已发布未生效的版本带着生效或收尾痕迹")
		}
	case CommercialVersionEffective:
		if spec.EffectiveAt.IsZero() {
			return CommercialVersion{}, rehydratedPublicationRefusal("已生效的版本缺生效时刻")
		}
		if !spec.ClosedAt.IsZero() || spec.RetirementRef.valid() || spec.Successor.valid() {
			return CommercialVersion{}, rehydratedPublicationRefusal("仍生效的版本带着收尾痕迹")
		}
	case CommercialVersionExpired, CommercialVersionRetired, CommercialVersionSuperseded:
		if spec.EffectiveAt.IsZero() || spec.ClosedAt.IsZero() {
			return CommercialVersion{}, rehydratedPublicationRefusal("收尾的版本缺生效或收尾时刻")
		}
		if spec.Status == CommercialVersionRetired && !spec.RetirementRef.valid() {
			return CommercialVersion{}, rehydratedPublicationRefusal("退役的版本缺退役决定引用")
		}
		if spec.Status == CommercialVersionSuperseded &&
			(!spec.Successor.valid() || spec.Successor == spec.Version) {
			return CommercialVersion{}, rehydratedPublicationRefusal("替代的版本缺后继或后继指向自己")
		}
	case CommercialVersionDraft:
		return CommercialVersion{}, rehydratedPublicationRefusal("草稿不入发布登记册")
	default:
		return CommercialVersion{}, rehydratedPublicationRefusal(fmt.Sprintf("版本状态不是本上下文的取值：%d", uint8(spec.Status)))
	}

	references, err := declaredReferences(spec.Kind, spec.ObjectID, spec.References)
	if err != nil {
		return CommercialVersion{}, rehydratedPublicationRefusal("指名引用立不起来（自引用或空引用）")
	}
	return CommercialVersion{
		tenant:        spec.TenantID,
		kind:          spec.Kind,
		objectID:      spec.ObjectID,
		version:       spec.Version,
		scope:         spec.Scope,
		contentDigest: spec.ContentDigest,
		effective:     spec.Effective,
		status:        spec.Status,
		approval:      spec.Approval,
		publishedAt:   spec.PublishedAt.UTC(),
		effectiveAt:   spec.EffectiveAt.UTC(),
		closedAt:      spec.ClosedAt.UTC(),
		retirementRef: spec.RetirementRef,
		successor:     spec.Successor,
		references:    references,
	}, nil
}

func rehydratedPublicationRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedPublication, reason)
}
