package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedExternalTrackingFact 是重建入口因快照数据本身而拒绝时的理由。与
// ErrInvalidExternalTrackingFact 分开：后者说「此刻要认领的这份不合规则」，前者说「这份已经
// 登记过的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器。
var ErrInvalidRehydratedExternalTrackingFact = errors.New("transport fulfillment: invalid rehydrated external carrier tracking fact")

// RehydrateExternalTrackingFactSpec 是一条外部承运轨迹事实某一版在库里的样子。有效时间判断
// 拆成依据、时间、规则三列装回；三者的配合关系由重建门核，不由适配器拼。
type RehydrateExternalTrackingFactSpec struct {
	TenantID             TenantID
	Fact                 ExternalTrackingFactReference
	Version              ExternalTrackingFactVersion
	Source               TrackingSourceReference
	Credential           ExternalCarrierCredentialReference
	Object               CarriedObjectReference
	SourceEvent          SourceEventReference
	Status               RawStatusReference
	OccurredAt           time.Time
	ReceivedAt           time.Time
	EffectiveBasis       EffectiveTimeBasis
	EffectiveAt          time.Time
	EffectiveRule        string
	EffectiveRuleVersion string
	CorrectionOf         SourceEventReference
	Supersedes           ExternalTrackingFactVersion
	Origin               VersionOrigin
}

// RehydrateExternalTrackingFact 从库里读到的产物重建一条事实。字段一律当数据收下，不重算
// （ADR-0028 同款）：认领判断在 AdoptExternalCarrierTracking 那道门，这里只挡一行坏数据变成
// 一份看起来合法的事实。
func RehydrateExternalTrackingFact(spec RehydrateExternalTrackingFactSpec) (ExternalCarrierTrackingFact, error) {
	if !spec.TenantID.valid() || !spec.Fact.valid() || !spec.Version.valid() ||
		!spec.Source.valid() || !spec.Credential.valid() || !spec.Object.valid() || !spec.Status.valid() {
		return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("事实身份、轨迹源、凭证、对象或状态词缺失")
	}
	if spec.OccurredAt.IsZero() {
		return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("没有源给的发生时间——这种素材本该留痕，不该成为事实")
	}
	if spec.ReceivedAt.IsZero() {
		return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("接收时间缺失")
	}
	effective, err := rehydrateEffectiveTime(spec)
	if err != nil {
		return ExternalCarrierTrackingFact{}, err
	}
	switch spec.Origin {
	case VersionFromMaterial:
		// 素材到达形成的版本回指前版只有一条来路：解析了源声明的更正。
		if spec.Supersedes.valid() && !spec.CorrectionOf.valid() {
			return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("素材版本回指前版却没有源更正声明")
		}
	case VersionFromJudgment:
		// 判断形成的版本必定回指被判断的那一版，且必定判断过——「判断为待判断」不是判断。
		if !spec.Supersedes.valid() || !effective.Judged() {
			return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("判断版本不回指前版或仍是待判断")
		}
	default:
		return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("版本来路不在封闭集合内")
	}
	if spec.Supersedes.valid() && spec.Supersedes == spec.Version {
		return ExternalCarrierTrackingFact{}, rehydratedTrackingRefusal("前版引用指向版本自己")
	}
	return ExternalCarrierTrackingFact{
		tenantID:     spec.TenantID,
		fact:         spec.Fact,
		version:      spec.Version,
		source:       spec.Source,
		credential:   spec.Credential,
		object:       spec.Object,
		sourceEvent:  spec.SourceEvent,
		status:       spec.Status,
		occurredAt:   spec.OccurredAt.UTC(),
		receivedAt:   spec.ReceivedAt.UTC(),
		effective:    effective,
		correctionOf: spec.CorrectionOf,
		supersedes:   spec.Supersedes,
		origin:       spec.Origin,
	}, nil
}

func rehydrateEffectiveTime(spec RehydrateExternalTrackingFactSpec) (EffectiveTimeJudgment, error) {
	switch spec.EffectiveBasis {
	case EffectiveTimePending:
		if !spec.EffectiveAt.IsZero() || spec.EffectiveRule != "" || spec.EffectiveRuleVersion != "" {
			return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("待判断却带着有效时间或规则")
		}
		return PendingEffectiveTime(), nil
	case EffectiveTimeJudgedExplicitly:
		if spec.EffectiveRule != "" || spec.EffectiveRuleVersion != "" {
			return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("显式判断却带着规则")
		}
		judgment, err := JudgeEffectiveTimeExplicitly(spec.EffectiveAt)
		if err != nil {
			return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("显式判断没有时间")
		}
		return judgment, nil
	case EffectiveTimeJudgedByRule:
		rule, err := NewEffectiveTimeRuleReference(spec.EffectiveRule, spec.EffectiveRuleVersion)
		if err != nil {
			return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("按规则判断却说不出规则与版本")
		}
		judgment, err := JudgeEffectiveTimeByRule(rule, spec.EffectiveAt)
		if err != nil {
			return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("按规则判断没有时间")
		}
		return judgment, nil
	default:
		return EffectiveTimeJudgment{}, rehydratedTrackingRefusal("有效时间依据不在封闭集合内")
	}
}

func rehydratedTrackingRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedExternalTrackingFact, reason)
}
