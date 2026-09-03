package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidExternalTrackingFact = errors.New("transport fulfillment: invalid external carrier tracking fact")
	// ErrOccurredAtNotGivenBySource 与形状错误分格：恢复动作不是补字段，而是把这条素材如实
	// 留痕为「源未给发生时间」——ADR-0102 决定二，这一格不留例外，也不许拿接收时间顶替。
	ErrOccurredAtNotGivenBySource   = errors.New("transport fulfillment: the tracking source gave no occurrence time")
	ErrInvalidEffectiveTimeJudgment = errors.New("transport fulfillment: invalid effective time judgment")
)

// TrackingSourceReference 指名一个已登记的轨迹源（CONTEXT「轨迹源」）。一家聚合平台是一个源，
// 一家直连承运商也是一个源；源的账号、地址、节奏属实例参数，这里只引用。
type TrackingSourceReference struct{ requiredValue }

func NewTrackingSourceReference(value string) (TrackingSourceReference, error) {
	required, err := newRequiredValue("tracking source reference", value)
	return TrackingSourceReference{required}, err
}

// ExternalCarrierCredentialReference 指名素材所对的外部承运凭证。凭证分配给的可能是运输委托、
// 订舱、班次或载运对象，不解释为包裹的当前运单号（CONTEXT「外部承运凭证」）。
type ExternalCarrierCredentialReference struct{ requiredValue }

func NewExternalCarrierCredentialReference(value string) (ExternalCarrierCredentialReference, error) {
	required, err := newRequiredValue("external carrier credential reference", value)
	return ExternalCarrierCredentialReference{required}, err
}

// SourceEventReference 是轨迹源给出的事件标识。它由源铸，本上下文不代铸；源未给时事实上就
// 没有它，不按内容摘要补一个（ADR-0102 决定四）。
type SourceEventReference struct{ requiredValue }

func NewSourceEventReference(value string) (SourceEventReference, error) {
	required, err := newRequiredValue("source event reference", value)
	return SourceEventReference{required}, err
}

// RawStatusReference 是源的原始状态词或码，原样保存、不解释、不映射。「外部承运方报了
// `DELIVERED`」是事实，「它对本仓意味着已交付」是另一次判断（ADR-0102 决定五）。
type RawStatusReference struct{ requiredValue }

func NewRawStatusReference(value string) (RawStatusReference, error) {
	required, err := newRequiredValue("raw status reference", value)
	return RawStatusReference{required}, err
}

// ExternalTrackingFactReference 是本上下文为一条外部承运轨迹事实另铸的身份，与源事件标识
// 分开保存、不互相顶替。
type ExternalTrackingFactReference struct{ requiredValue }

func NewExternalTrackingFactReference(value string) (ExternalTrackingFactReference, error) {
	required, err := newRequiredValue("external tracking fact reference", value)
	return ExternalTrackingFactReference{required}, err
}

// ExternalTrackingFactVersion 是事实的版本。源声明更正与本上下文的有效时间判断都换新版本、
// 回指前版，原版本与原判断继续保留。
type ExternalTrackingFactVersion struct{ requiredValue }

func NewExternalTrackingFactVersion(value string) (ExternalTrackingFactVersion, error) {
	required, err := newRequiredValue("external tracking fact version", value)
	return ExternalTrackingFactVersion{required}, err
}

// EffectiveTimeRuleReference 指名该轨迹源已登记并带版本的有效时间规则。版本必备：不带版本
// 就说不清一条历史事实是按哪一版判过的。规则的形状与取值属实例参数（`PAR-INT-02`），这里只
// 记「采用了哪一版」。
type EffectiveTimeRuleReference struct {
	rule    requiredValue
	version requiredValue
}

func NewEffectiveTimeRuleReference(rule, version string) (EffectiveTimeRuleReference, error) {
	requiredRule, err := newRequiredValue("effective time rule reference", rule)
	if err != nil {
		return EffectiveTimeRuleReference{}, err
	}
	requiredVersion, err := newRequiredValue("effective time rule version", version)
	if err != nil {
		return EffectiveTimeRuleReference{}, err
	}
	return EffectiveTimeRuleReference{rule: requiredRule, version: requiredVersion}, nil
}

func (reference EffectiveTimeRuleReference) Rule() string    { return reference.rule.String() }
func (reference EffectiveTimeRuleReference) Version() string { return reference.version.String() }

func (reference EffectiveTimeRuleReference) valid() bool {
	return reference.rule.valid() && reference.version.valid()
}

// EffectiveTimeBasis 说一条事实的有效时间是怎么来的。「待判断」与两种「判断过」在类型上分开
// ——默认相等会让「所有者判断过」与「没人判断过」长成同一张脸（ADR-0102 决定三）。
type EffectiveTimeBasis uint8

const (
	EffectiveTimeBasisInvalid EffectiveTimeBasis = iota
	EffectiveTimePending
	EffectiveTimeJudgedExplicitly
	EffectiveTimeJudgedByRule
)

func (basis EffectiveTimeBasis) String() string {
	switch basis {
	case EffectiveTimePending:
		return "PENDING"
	case EffectiveTimeJudgedExplicitly:
		return "JUDGED_EXPLICITLY"
	case EffectiveTimeJudgedByRule:
		return "JUDGED_BY_RULE"
	default:
		return ""
	}
}

func (basis EffectiveTimeBasis) valid() bool {
	return basis >= EffectiveTimePending && basis <= EffectiveTimeJudgedByRule
}

// EffectiveTimeJudgment 是本上下文对「这条外部状态从何时起对本仓有效」的一次判断，或如实的
// 「待判断」。它不是外部事实自己的时间，因此不在 ADR-0023「不代铸」的射程内；但它必须是显式
// 的——零值不是待判断，构造门会拒。
type EffectiveTimeJudgment struct {
	basis EffectiveTimeBasis
	at    time.Time
	rule  EffectiveTimeRuleReference
}

// PendingEffectiveTime 是「有效时间待判断」。带着它的事实留在所有者手上，不提供给
// `visibility-exception`。
func PendingEffectiveTime() EffectiveTimeJudgment {
	return EffectiveTimeJudgment{basis: EffectiveTimePending}
}

// JudgeEffectiveTimeExplicitly 由所有者就这一条显式给出有效时间。与发生时间相等也算判断过
// ——区别在依据上，不在值上。
func JudgeEffectiveTimeExplicitly(at time.Time) (EffectiveTimeJudgment, error) {
	if at.IsZero() {
		return EffectiveTimeJudgment{}, ErrInvalidEffectiveTimeJudgment
	}
	return EffectiveTimeJudgment{basis: EffectiveTimeJudgedExplicitly, at: at.UTC()}, nil
}

// JudgeEffectiveTimeByRule 依据该源已登记并带版本的规则形成有效时间，事实上记下采用了哪一版。
func JudgeEffectiveTimeByRule(rule EffectiveTimeRuleReference, at time.Time) (EffectiveTimeJudgment, error) {
	if !rule.valid() || at.IsZero() {
		return EffectiveTimeJudgment{}, ErrInvalidEffectiveTimeJudgment
	}
	return EffectiveTimeJudgment{basis: EffectiveTimeJudgedByRule, at: at.UTC(), rule: rule}, nil
}

func (judgment EffectiveTimeJudgment) Basis() EffectiveTimeBasis {
	return judgment.basis
}

// EffectiveAt 只在判断过时给出时间；待判断第二个返回值为 false。
func (judgment EffectiveTimeJudgment) EffectiveAt() (time.Time, bool) {
	if !judgment.Judged() {
		return time.Time{}, false
	}
	return judgment.at, true
}

// Rule 只在按规则判断时给出采用的规则版本。
func (judgment EffectiveTimeJudgment) Rule() (EffectiveTimeRuleReference, bool) {
	if judgment.basis != EffectiveTimeJudgedByRule {
		return EffectiveTimeRuleReference{}, false
	}
	return judgment.rule, true
}

func (judgment EffectiveTimeJudgment) Judged() bool {
	return judgment.basis == EffectiveTimeJudgedExplicitly || judgment.basis == EffectiveTimeJudgedByRule
}

func (judgment EffectiveTimeJudgment) valid() bool {
	switch judgment.basis {
	case EffectiveTimePending:
		return judgment.at.IsZero() && !judgment.rule.valid()
	case EffectiveTimeJudgedExplicitly:
		return !judgment.at.IsZero() && !judgment.rule.valid()
	case EffectiveTimeJudgedByRule:
		return !judgment.at.IsZero() && judgment.rule.valid()
	default:
		return false
	}
}

// VersionOrigin 说一个版本是怎么来的：素材到达（首次认领或源声明的更正），还是本上下文的一次
// 有效时间判断。它决定幂等锚认不认这一版——（源，源事件）只锚素材到达形成的版本，判断形成的
// 版本原样携带源事件却不是一次新的到达。
type VersionOrigin uint8

const (
	VersionOriginInvalid VersionOrigin = iota
	VersionFromMaterial
	VersionFromJudgment
)

func (origin VersionOrigin) String() string {
	switch origin {
	case VersionFromMaterial:
		return "MATERIAL"
	case VersionFromJudgment:
		return "JUDGMENT"
	default:
		return ""
	}
}

// ExternalTrackingFactSpec 是认领一条素材为外部承运轨迹事实所需的全部输入。三个时间里只有
// 两个在这里以时间出现：OccurredAt 由源给，ReceivedAt 由本上下文在接到素材时铸；有效时间不是
// 时间而是一次判断（Effective）。
//
// SourceEvent 可缺席（源未给）。CorrectionOf 是源显式声明的「本条更正了哪条源事件」，原样带；
// Supersedes 是本上下文把那条声明解析到自己某一版之后的回指——没有声明就没有回指，本上下文
// 不从到达先后、发生时间先后或状态词推断谁取代谁（CONTEXT 规则节）。
type ExternalTrackingFactSpec struct {
	TenantID     TenantID
	Fact         ExternalTrackingFactReference
	Version      ExternalTrackingFactVersion
	Source       TrackingSourceReference
	Credential   ExternalCarrierCredentialReference
	Object       CarriedObjectReference
	SourceEvent  SourceEventReference
	Status       RawStatusReference
	OccurredAt   time.Time
	ReceivedAt   time.Time
	Effective    EffectiveTimeJudgment
	CorrectionOf SourceEventReference
	Supersedes   ExternalTrackingFactVersion
}

// ExternalCarrierTrackingFact 是本上下文把轨迹源就明确外部承运凭证所指对象报来的一条原始
// 素材认领为自己承运来源事实后形成的记录（CONTEXT「外部承运轨迹事实」）。它与自营作业事实
// 并列而不混同：它是来源事实，不是有效收寄、权威运输交接、实际移动或有效交付。
type ExternalCarrierTrackingFact struct {
	tenantID     TenantID
	fact         ExternalTrackingFactReference
	version      ExternalTrackingFactVersion
	source       TrackingSourceReference
	credential   ExternalCarrierCredentialReference
	object       CarriedObjectReference
	sourceEvent  SourceEventReference
	status       RawStatusReference
	occurredAt   time.Time
	receivedAt   time.Time
	effective    EffectiveTimeJudgment
	correctionOf SourceEventReference
	supersedes   ExternalTrackingFactVersion
	origin       VersionOrigin
}

// AdoptExternalCarrierTracking 是认领的构造门。源未给发生时间单独一格拒绝——那条素材要走
// 留痕，不是改输入；其余缺失是形状错误。
func AdoptExternalCarrierTracking(spec ExternalTrackingFactSpec) (ExternalCarrierTrackingFact, error) {
	if spec.OccurredAt.IsZero() {
		return ExternalCarrierTrackingFact{}, ErrOccurredAtNotGivenBySource
	}
	if !spec.TenantID.valid() || !spec.Fact.valid() || !spec.Version.valid() ||
		!spec.Source.valid() || !spec.Credential.valid() || !spec.Object.valid() ||
		!spec.Status.valid() || spec.ReceivedAt.IsZero() || !spec.Effective.valid() {
		return ExternalCarrierTrackingFact{}, ErrInvalidExternalTrackingFact
	}
	if spec.Supersedes.valid() {
		// 回指前版只能是解析源声明的结果；没有声明的回指就是本上下文自己推断了取代关系。
		if !spec.CorrectionOf.valid() || spec.Supersedes == spec.Version {
			return ExternalCarrierTrackingFact{}, ErrInvalidExternalTrackingFact
		}
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
		effective:    spec.Effective,
		correctionOf: spec.CorrectionOf,
		supersedes:   spec.Supersedes,
		origin:       VersionFromMaterial,
	}, nil
}

// Origin 说本版本是素材到达形成的还是判断形成的。
func (fact ExternalCarrierTrackingFact) Origin() VersionOrigin { return fact.origin }

func (fact ExternalCarrierTrackingFact) TenantID() TenantID                   { return fact.tenantID }
func (fact ExternalCarrierTrackingFact) Fact() ExternalTrackingFactReference  { return fact.fact }
func (fact ExternalCarrierTrackingFact) Version() ExternalTrackingFactVersion { return fact.version }
func (fact ExternalCarrierTrackingFact) Source() TrackingSourceReference      { return fact.source }
func (fact ExternalCarrierTrackingFact) Credential() ExternalCarrierCredentialReference {
	return fact.credential
}
func (fact ExternalCarrierTrackingFact) Object() CarriedObjectReference   { return fact.object }
func (fact ExternalCarrierTrackingFact) Status() RawStatusReference       { return fact.status }
func (fact ExternalCarrierTrackingFact) Effective() EffectiveTimeJudgment { return fact.effective }

// SourceEvent 在源给了事件标识时交回它。
func (fact ExternalCarrierTrackingFact) SourceEvent() (SourceEventReference, bool) {
	return fact.sourceEvent, fact.sourceEvent.valid()
}

// OccurredAt 是轨迹源给出的发生时间。它只能来自源。
func (fact ExternalCarrierTrackingFact) OccurredAt() time.Time { return fact.occurredAt }

// ReceivedAt 是本上下文接到这条素材的时间。
func (fact ExternalCarrierTrackingFact) ReceivedAt() time.Time { return fact.receivedAt }

// EffectiveAt 只在有效时间判断过时给出；待判断第二个返回值为 false。
func (fact ExternalCarrierTrackingFact) EffectiveAt() (time.Time, bool) {
	return fact.effective.EffectiveAt()
}

// CorrectionOf 交回源显式声明的被更正源事件；源没声明第二个返回值为 false。
func (fact ExternalCarrierTrackingFact) CorrectionOf() (SourceEventReference, bool) {
	return fact.correctionOf, fact.correctionOf.valid()
}

// Supersedes 交回本版本回指的前版；首版第二个返回值为 false。
func (fact ExternalCarrierTrackingFact) Supersedes() (ExternalTrackingFactVersion, bool) {
	return fact.supersedes, fact.supersedes.valid()
}

// JudgeEffectiveTime 为一条事实落一次有效时间判断，形成回指本版的新版本；源给的内容一字不动，
// 原版本与原判断保留（值语义）。沿用原版本号就是覆盖；「判断为待判断」不是判断。
func (fact ExternalCarrierTrackingFact) JudgeEffectiveTime(
	judgment EffectiveTimeJudgment,
	version ExternalTrackingFactVersion,
) (ExternalCarrierTrackingFact, error) {
	if !judgment.valid() || !judgment.Judged() {
		return ExternalCarrierTrackingFact{}, ErrInvalidEffectiveTimeJudgment
	}
	if !fact.version.valid() || !version.valid() || version == fact.version {
		return ExternalCarrierTrackingFact{}, ErrInvalidExternalTrackingFact
	}
	judged := fact
	judged.version = version
	judged.effective = judgment
	judged.supersedes = fact.version
	judged.origin = VersionFromJudgment
	return judged, nil
}
