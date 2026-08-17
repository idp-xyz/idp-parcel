package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidRehydratedResolution 是解析重建入口因快照数据本身而拒绝时给出的理由。
// 与解析当时的领域错误分开：后者说「此刻解不出来」，前者说「这份已经固定过的东西
// 不可能是本上下文写出来的」（ADR-0028）。
var ErrInvalidRehydratedResolution = errors.New("party commercial: invalid rehydrated commercial resolution")

// RehydrateAdoptedBasisSpec 是闭包里一项已采用依据在库里的样子。版本身份始终固定；
// 服务产品快照在场时一并重建（ADR-0050），价格/结算政策嵌套仍随闭包另票。
type RehydrateAdoptedBasisSpec struct {
	Kind              CommercialObjectKind
	Version           CommercialVersion
	ServiceProduct    ServiceProduct
	HasServiceProduct bool
}

// RehydrateCommercialClosureSpec 是一次已固定解析在库里的样子。字段一律当数据收下，
// 不重算解析标识（ADR-0028）。
type RehydrateCommercialClosureSpec struct {
	Outcome      ResolutionOutcome
	ResolutionID ResolutionID
	Key          ClosureResolutionKey
	Anchor       SelectionAnchor
	ViewRevision AuthorityViewRevision
	Adopted      []RehydrateAdoptedBasisSpec
}

// RehydrateCommercialClosure 从库里读到的产物重建一份唯一已解析的闭包。本票只固定
// 这一次解析，其它结局没有可回指的标识，重建门直接拒。
func RehydrateCommercialClosure(spec RehydrateCommercialClosureSpec) (CommercialClosure, error) {
	if spec.Outcome != UniquelyResolved {
		return CommercialClosure{}, rehydratedResolutionRefusal("本票只固定唯一已解析")
	}
	if !spec.ResolutionID.valid() || !spec.Key.minimumIdentityEstablished() ||
		!spec.Anchor.valid() || !spec.ViewRevision.valid() {
		return CommercialClosure{}, rehydratedResolutionRefusal("标识、解析键、锚点或视图修订缺失")
	}
	if spec.Key.Anchor.At() != spec.Anchor.At() ||
		spec.Key.Anchor.PolicyVersion() != spec.Anchor.PolicyVersion() {
		return CommercialClosure{}, rehydratedResolutionRefusal("闭包锚点与解析键锚点不一致")
	}
	if len(spec.Adopted) == 0 {
		return CommercialClosure{}, rehydratedResolutionRefusal("唯一解析没有采用依据")
	}

	adopted := make([]AdoptedBasis, 0, len(spec.Adopted))
	for _, item := range spec.Adopted {
		if item.Kind == CommercialObjectKindInvalid ||
			item.Version.status == CommercialVersionStatusInvalid ||
			item.Version.status == CommercialVersionDraft {
			return CommercialClosure{}, rehydratedResolutionRefusal("采用依据缺席或仍是草稿")
		}
		basis := AdoptedBasis{kind: item.Kind, version: item.Version}
		if item.HasServiceProduct {
			if item.Kind != ServiceProductObject ||
				item.ServiceProduct.version.objectID != item.Version.objectID ||
				item.ServiceProduct.version.version != item.Version.version ||
				!item.ServiceProduct.form.valid() {
				return CommercialClosure{}, rehydratedResolutionRefusal("服务产品快照与采用版本对不上")
			}
			basis.serviceProduct = item.ServiceProduct
			basis.hasServiceProduct = true
		}
		adopted = append(adopted, basis)
	}

	return CommercialClosure{
		outcome:      UniquelyResolved,
		resolutionID: spec.ResolutionID,
		key:          spec.Key,
		anchor:       spec.Anchor,
		viewRevision: spec.ViewRevision,
		adopted:      adopted,
	}, nil
}

func rehydratedResolutionRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedResolution, reason)
}
