package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedMasterDocument 是重建入口因库里那一行本身而拒绝时的理由。与
// ErrInvalidMasterDocument 分开：后者说「此刻要登记的这份不合规则」，前者说「这份已经登记过的东西
// 不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器。
var ErrInvalidRehydratedMasterDocument = errors.New("transport fulfillment: invalid rehydrated master document")

// RehydrateMasterDocumentSpec 是一份总单某一版在库里的样子。状态、改变时间、前版、替代者四者的配合
// 关系由重建门核，不由适配器拼；关联从子表装回，顺序由重建门整理。
type RehydrateMasterDocumentSpec struct {
	TenantID     TenantID
	Document     MasterDocumentReference
	Version      MasterDocumentVersion
	Issuer       MasterDocumentIssuerReference
	Scope        TransportScopeReference
	Commission   TransportCommissionReference
	Booking      BookingReference
	Associations []MasterDocumentAssociation
	Standing     MasterDocumentStanding
	ChangedAt    time.Time
	Supersedes   MasterDocumentVersion
	ReplacedBy   MasterDocumentReference
}

// RehydrateMasterDocument 从库里读到的产物重建一个版本。字段一律当数据收下、不重算（ADR-0028 同款）：
// 登记判断在 RegisterMasterDocument 与三条改变的门，这里只挡一行坏数据变成一份看起来合法的总单。
func RehydrateMasterDocument(spec RehydrateMasterDocumentSpec) (MasterDocument, error) {
	if !spec.TenantID.valid() || !spec.Document.valid() || !spec.Version.valid() ||
		!spec.Issuer.valid() || !spec.Scope.valid() {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("租户、总单身份、版本、签发方或范围缺失")
	}
	associations, err := canonicalAssociations(spec.Associations)
	if err != nil {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("关联立不住或重复")
	}
	if !spec.Standing.valid() {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("状态不在封闭集合内")
	}
	if spec.Supersedes.valid() && spec.Supersedes == spec.Version {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("前版引用指向版本自己")
	}
	// 首版不回指、无改变时间；后续版本两者都有——关联重述也是后续版本，所以这一格不看状态。
	if spec.Supersedes.valid() != !spec.ChangedAt.IsZero() {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("回指与改变时间不成对")
	}
	if !spec.Supersedes.valid() && spec.Standing != MasterDocumentInForce {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("首版却已撤销或已替代")
	}
	if (spec.Standing == MasterDocumentSuperseded) != spec.ReplacedBy.valid() {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("替代者只在已替代时出现，且已替代必有替代者")
	}
	if spec.ReplacedBy.valid() && spec.ReplacedBy == spec.Document {
		return MasterDocument{}, rehydratedMasterDocumentRefusal("替代者是总单自己")
	}
	return MasterDocument{
		tenantID:     spec.TenantID,
		document:     spec.Document,
		version:      spec.Version,
		issuer:       spec.Issuer,
		scope:        spec.Scope,
		commission:   spec.Commission,
		booking:      spec.Booking,
		associations: associations,
		standing:     spec.Standing,
		changedAt:    spec.ChangedAt.UTC(),
		supersedes:   spec.Supersedes,
		replacedBy:   spec.ReplacedBy,
	}, nil
}

func rehydratedMasterDocumentRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedMasterDocument, reason)
}
