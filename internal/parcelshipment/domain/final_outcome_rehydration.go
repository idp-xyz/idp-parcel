package domain

import "errors"

// ErrInvalidRehydratedFinal 是终局判断从持久化行重建失败的信号（ADR-0030 的纪律）。
var ErrInvalidRehydratedFinal = errors.New("parcel shipment: rehydrated final outcome violates its invariants")

// RehydrateParcelFinalOutcomeSpec 是从行数据重建一份包裹终局所需的全部字段。
// PriorVersion 与 Reason 以零值表示首派生——两者必须同在或同缺。
//
// FormParcelFinalOutcome 与 Rederive 不能兼职重建：重派生版本的前版对象只剩一个
// 版本号留在行里，Rederive 要的却是前版本体——重建门以结果自证一致，不逆推形成
// 过程。
type RehydrateParcelFinalOutcomeSpec struct {
	Version      FinalOutcomeVersionID
	Parcel       DeclaredParcelID
	Kind         FinalKindReference
	Source       ResponsibilityOutcome
	RuleVersion  FinalRuleVersionReference
	PriorVersion FinalOutcomeVersionID
	Reason       RederivationReason
}

// RehydrateParcelFinalOutcome 重走 FormParcelFinalOutcome 的完备性检查，另验重派生
// 痕迹的两半互证：前版与原因同在或同缺、不得自指——一行「有前版没原因」的终局与
// 静默改写分不开。
func RehydrateParcelFinalOutcome(spec RehydrateParcelFinalOutcomeSpec) (ParcelFinalOutcome, error) {
	if !spec.Version.valid() ||
		!spec.Parcel.valid() ||
		!spec.Kind.valid() ||
		!spec.Source.kind.valid() ||
		!spec.RuleVersion.valid() ||
		spec.Parcel != spec.Source.parcel {
		return ParcelFinalOutcome{}, ErrInvalidRehydratedFinal
	}
	if spec.PriorVersion.valid() != spec.Reason.valid() {
		return ParcelFinalOutcome{}, ErrInvalidRehydratedFinal
	}
	if spec.PriorVersion.valid() && spec.PriorVersion == spec.Version {
		return ParcelFinalOutcome{}, ErrInvalidRehydratedFinal
	}
	return ParcelFinalOutcome{
		version:      spec.Version,
		parcel:       spec.Parcel,
		kind:         spec.Kind,
		source:       spec.Source,
		ruleVersion:  spec.RuleVersion,
		effectiveAt:  spec.Source.occurredAt,
		priorVersion: spec.PriorVersion,
		reason:       spec.Reason,
	}, nil
}
