package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedEffectiveTimeRule 是重建入口因快照数据本身而拒绝时的理由。与
// ErrInvalidEffectiveTimeRule 分开：后者说「此刻要登记的这份不合规则」，前者说「这份已经登记过
// 的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器。
var ErrInvalidRehydratedEffectiveTimeRule = errors.New("transport fulfillment: invalid rehydrated effective time rule")

// RehydrateEffectiveTimeRuleSpec 是一版规则在库里的样子：正文三列与前版各自装回。
type RehydrateEffectiveTimeRuleSpec struct {
	TenantID          TenantID
	Source            TrackingSourceReference
	Version           EffectiveTimeRuleVersion
	SourceTimeMeaning SourceTimeMeaning
	Anchor            EffectiveTimeAnchor
	Offset            time.Duration
	Supersedes        EffectiveTimeRuleVersion
}

// RehydrateEffectiveTimeRule 从库里读到的产物重建一版规则。字段一律当数据收下，不重算（ADR-0028
// 同款）：登记判断在 RegisterEffectiveTimeRule / Revise 那两道门，这里只挡一行坏数据变成一版看起来
// 合法的规则。
func RehydrateEffectiveTimeRule(spec RehydrateEffectiveTimeRuleSpec) (EffectiveTimeRule, error) {
	if !spec.TenantID.valid() || !spec.Source.valid() || !spec.Version.valid() {
		return EffectiveTimeRule{}, rehydratedRuleRefusal("租户、轨迹源或版本缺失")
	}
	content := EffectiveTimeRuleContent{
		SourceTimeMeaning: spec.SourceTimeMeaning,
		Anchor:            spec.Anchor,
		Offset:            spec.Offset,
	}
	if !content.valid() {
		return EffectiveTimeRule{}, rehydratedRuleRefusal("时间字段含义或锚点不在封闭集合内")
	}
	if spec.Supersedes.valid() && spec.Supersedes == spec.Version {
		return EffectiveTimeRule{}, rehydratedRuleRefusal("前版引用指向版本自己")
	}
	return EffectiveTimeRule{
		tenantID:   spec.TenantID,
		source:     spec.Source,
		version:    spec.Version,
		content:    content,
		supersedes: spec.Supersedes,
	}, nil
}

func rehydratedRuleRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedEffectiveTimeRule, reason)
}
