package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 轨迹源有效时间规则目录（label-channel/19）。CONTEXT「轨迹源」把有效时间规则列为该源的实例参数；
// 本文件是登记那份参数的机制越过提交边界的形状。EffectiveTimeRules（external_tracking_fact.go）由这本
// 目录的适配器实现——「按该源已登记并带版本的规则形成有效时间」读的就是这里登记的内容。

// EffectiveTimeRuleKey 是某源规则某一版的幂等键。版本在键里：换版是新版本回指前版，原版本不被改写。
type EffectiveTimeRuleKey struct {
	TenantID domain.TenantID
	Source   domain.TrackingSourceReference
	Version  domain.EffectiveTimeRuleVersion
}

// EffectiveTimeRuleRecord 是一个版本越过提交边界留下的东西。RecordedAt 是登记落库的时刻——规则没有
// 自己的业务时间：哪一版在用由回指链派生，不由时间区间派生。
type EffectiveTimeRuleRecord struct {
	Key        EffectiveTimeRuleKey
	Rule       domain.EffectiveTimeRule
	RecordedAt time.Time
}

// EffectiveTimeRuleSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）；撞的是重放
// 还是改内容由调用方读回既有版本比对——写口只答「这一键已经有了」。
type EffectiveTimeRuleSaveOutcome uint8

const (
	EffectiveTimeRuleSaveOutcomeInvalid EffectiveTimeRuleSaveOutcome = iota
	EffectiveTimeRuleSaved
	EffectiveTimeRuleAlreadyRegistered
)

func (outcome EffectiveTimeRuleSaveOutcome) String() string {
	switch outcome {
	case EffectiveTimeRuleSaved:
		return "SAVED"
	case EffectiveTimeRuleAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// EffectiveTimeRuleRegistry 按键找回并登记规则版本。只插不改。
//
// FindCurrent 交回某源此刻未被任何版本回指的那一版——「当前」是按回指派生的问答，不是存下来的标记；
// 没有版本即该源尚无规则，EffectiveTimeRules 据此答`无`。ListVersions 按登记先后交回全部版本。
type EffectiveTimeRuleRegistry interface {
	FindByKey(ctx context.Context, key EffectiveTimeRuleKey) (EffectiveTimeRuleRecord, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.TrackingSourceReference,
	) (EffectiveTimeRuleRecord, bool, error)
	ListVersions(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.TrackingSourceReference,
	) ([]EffectiveTimeRuleRecord, error)
	Save(ctx context.Context, record EffectiveTimeRuleRecord) (EffectiveTimeRuleSaveOutcome, error)
}
