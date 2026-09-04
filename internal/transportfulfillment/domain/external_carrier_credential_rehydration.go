package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedExternalCarrierCredential 是重建入口因库里那一行本身而拒绝时的理由。与
// ErrInvalidExternalCarrierCredential 分开：后者说「此刻要登记的这份不合规则」，前者说「这份已经
// 登记过的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器。
var ErrInvalidRehydratedExternalCarrierCredential = errors.New("transport fulfillment: invalid rehydrated external carrier credential")

// RehydrateExternalCarrierCredentialSpec 是一份凭证某一版在库里的样子。适用范围拆成起止两列
// 装回；状态、终点、前版、替代者四者的配合关系由重建门核，不由适配器拼。
type RehydrateExternalCarrierCredentialSpec struct {
	TenantID       TenantID
	Credential     ExternalCarrierCredentialReference
	Version        ExternalCarrierCredentialVersion
	Assigner       CredentialAssignerReference
	Identifies     IdentifiedObject
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
	Standing       CredentialStanding
	ChangedAt      time.Time
	Supersedes     ExternalCarrierCredentialVersion
	ReplacedBy     ExternalCarrierCredentialReference
}

// RehydrateExternalCarrierCredential 从库里读到的产物重建一个版本。字段一律当数据收下、不重算
// （ADR-0028 同款）：登记判断在 RegisterExternalCarrierCredential 与三条改变适用关系的门，这里
// 只挡一行坏数据变成一份看起来合法的凭证。
func RehydrateExternalCarrierCredential(spec RehydrateExternalCarrierCredentialSpec) (ExternalCarrierCredential, error) {
	if !spec.TenantID.valid() || !spec.Credential.valid() || !spec.Version.valid() ||
		!spec.Assigner.valid() || !spec.Identifies.valid() {
		return ExternalCarrierCredential{}, rehydratedCredentialRefusal("租户、凭证身份、版本、分配方或标识对象缺失")
	}
	applicability, err := NewCredentialApplicability(spec.EffectiveFrom, spec.EffectiveUntil)
	if err != nil {
		return ExternalCarrierCredential{}, rehydratedCredentialRefusal("适用范围立不住")
	}
	if !spec.Standing.valid() {
		return ExternalCarrierCredential{}, rehydratedCredentialRefusal("状态不在封闭集合内")
	}
	if spec.Supersedes.valid() && spec.Supersedes == spec.Version {
		return ExternalCarrierCredential{}, rehydratedCredentialRefusal("前版引用指向版本自己")
	}
	_, closed := applicability.Until()
	switch spec.Standing {
	case CredentialApplicable:
		// 适用中的版本没有「改变」可记：它要么是首版，要么根本不该存在——本上下文只在改变适用
		// 关系时才发新版本。
		if !spec.ChangedAt.IsZero() || spec.Supersedes.valid() || spec.ReplacedBy.valid() {
			return ExternalCarrierCredential{}, rehydratedCredentialRefusal("适用中的版本带着改变时间、前版或替代者")
		}
	default:
		// 作废、失效、替代都是一次改变：必有业务时间、必回指前版、必有落定的终点。
		if spec.ChangedAt.IsZero() || !spec.Supersedes.valid() || !closed {
			return ExternalCarrierCredential{}, rehydratedCredentialRefusal("不适用的版本缺改变时间、前版或终点")
		}
		if (spec.Standing == CredentialSuperseded) != spec.ReplacedBy.valid() {
			return ExternalCarrierCredential{}, rehydratedCredentialRefusal("替代者只在已替代时出现，且已替代必有替代者")
		}
		if spec.ReplacedBy.valid() && spec.ReplacedBy == spec.Credential {
			return ExternalCarrierCredential{}, rehydratedCredentialRefusal("替代者是凭证自己")
		}
	}
	return ExternalCarrierCredential{
		tenantID:      spec.TenantID,
		credential:    spec.Credential,
		version:       spec.Version,
		assigner:      spec.Assigner,
		identifies:    spec.Identifies,
		applicability: applicability,
		standing:      spec.Standing,
		changedAt:     spec.ChangedAt.UTC(),
		supersedes:    spec.Supersedes,
		replacedBy:    spec.ReplacedBy,
	}, nil
}

func rehydratedCredentialRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedExternalCarrierCredential, reason)
}
