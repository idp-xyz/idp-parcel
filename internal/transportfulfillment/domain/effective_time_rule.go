package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidEffectiveTimeRule 是规则登记或换版被拒的理由：形状不完整、锚点或时间字段含义不在
// 封闭集合内、沿用当前版本号覆盖。
var ErrInvalidEffectiveTimeRule = errors.New("transport fulfillment: invalid effective time rule")

// EffectiveTimeRuleVersion 是有效时间规则的版本。规则只追加：换一版回指前版，原版本原样保留——
// 一条历史事实按哪一版判过要说得清（EffectiveTimeRuleReference 要求版本必备）。
type EffectiveTimeRuleVersion struct{ requiredValue }

func NewEffectiveTimeRuleVersion(value string) (EffectiveTimeRuleVersion, error) {
	required, err := newRequiredValue("effective time rule version", value)
	return EffectiveTimeRuleVersion{required}, err
}

// SourceTimeMeaning 是所有者对「该源报来的时间字段是什么」的判断——ADR-0102 Consequences 点名
// 每接一个源都要先分清的那道：它是事件发生时间，还是源方处理时间。判错的方向是把后者当前者
// 收编，本仓在收编那一步不重新解释源给的时间（决定二），所以这道判断只能登记在规则上、由锚点
// 与偏移去承担它的后果。
type SourceTimeMeaning uint8

const (
	SourceTimeMeaningInvalid SourceTimeMeaning = iota
	SourceTimeIsEventOccurrence
	SourceTimeIsSourceProcessing
)

func (meaning SourceTimeMeaning) String() string {
	switch meaning {
	case SourceTimeIsEventOccurrence:
		return "EVENT_OCCURRENCE"
	case SourceTimeIsSourceProcessing:
		return "SOURCE_PROCESSING"
	default:
		return ""
	}
}

func (meaning SourceTimeMeaning) valid() bool {
	return meaning >= SourceTimeIsEventOccurrence && meaning <= SourceTimeIsSourceProcessing
}

// ParseSourceTimeMeaning 把库面或登记输入里的词认回封闭集合；词不在集合内即拒，不猜。
func ParseSourceTimeMeaning(raw string) (SourceTimeMeaning, error) {
	for meaning := SourceTimeIsEventOccurrence; meaning <= SourceTimeIsSourceProcessing; meaning++ {
		if meaning.String() == raw {
			return meaning, nil
		}
	}
	return SourceTimeMeaningInvalid, fmt.Errorf("%w: unknown source time meaning %q", ErrInvalidEffectiveTimeRule, raw)
}

// EffectiveTimeAnchor 是有效时间相对哪一个时间形成：源给的发生时间，或本上下文的接收时间。
// 两个都是显式登记的结果，没有缺省——「有效时间＝发生时间」在这里是一条登记出来的规则，不是
// 没登记时的默认值（ADR-0102 Alternatives 第二条否决的正是那个默认值）。
type EffectiveTimeAnchor uint8

const (
	EffectiveTimeAnchorInvalid EffectiveTimeAnchor = iota
	AnchoredAtOccurrence
	AnchoredAtReception
)

func (anchor EffectiveTimeAnchor) String() string {
	switch anchor {
	case AnchoredAtOccurrence:
		return "OCCURRED_AT"
	case AnchoredAtReception:
		return "RECEIVED_AT"
	default:
		return ""
	}
}

func (anchor EffectiveTimeAnchor) valid() bool {
	return anchor >= AnchoredAtOccurrence && anchor <= AnchoredAtReception
}

// ParseEffectiveTimeAnchor 把库面或登记输入里的锚点词认回封闭集合。
func ParseEffectiveTimeAnchor(raw string) (EffectiveTimeAnchor, error) {
	for anchor := AnchoredAtOccurrence; anchor <= AnchoredAtReception; anchor++ {
		if anchor.String() == raw {
			return anchor, nil
		}
	}
	return EffectiveTimeAnchorInvalid, fmt.Errorf("%w: unknown effective time anchor %q", ErrInvalidEffectiveTimeRule, raw)
}

// EffectiveTimeRuleContent 是一版规则的正文：源时间字段的含义、有效时间相对哪个时间、偏移多少。
// 它刻意只装得下这三件——规则不解释状态词（状态词的含义归里程碑映射登记册，ADR-0102 Consequences
// 第四条），也不看凭证或对象；一家源一条规则链。偏移允许为零或为负：源方处理时间常晚于事件本身，
// 所有者判定「有效时间在源方处理之前若干分钟」是正当的一格。
type EffectiveTimeRuleContent struct {
	SourceTimeMeaning SourceTimeMeaning
	Anchor            EffectiveTimeAnchor
	Offset            time.Duration
}

func (content EffectiveTimeRuleContent) valid() bool {
	return content.SourceTimeMeaning.valid() && content.Anchor.valid()
}

// EffectiveTimeRuleSpec 是登记一条规则首版所需的全部输入。规则按（租户，轨迹源）登记，没有独立
// 的规则名：一源一链，规则的引用就是它所属的源，版本另记。
type EffectiveTimeRuleSpec struct {
	TenantID TenantID
	Source   TrackingSourceReference
	Version  EffectiveTimeRuleVersion
	Content  EffectiveTimeRuleContent
}

// EffectiveTimeRule 是某轨迹源有效时间规则的一个版本（CONTEXT「轨迹源」：有效时间规则属该源的
// 实例参数；本类型是登记它的机制）。值语义：换版交回新值，原值不动。
type EffectiveTimeRule struct {
	tenantID   TenantID
	source     TrackingSourceReference
	version    EffectiveTimeRuleVersion
	content    EffectiveTimeRuleContent
	supersedes EffectiveTimeRuleVersion
}

// RegisterEffectiveTimeRule 是登记的构造门：首版不回指任何前版。
func RegisterEffectiveTimeRule(spec EffectiveTimeRuleSpec) (EffectiveTimeRule, error) {
	if !spec.TenantID.valid() || !spec.Source.valid() || !spec.Version.valid() || !spec.Content.valid() {
		return EffectiveTimeRule{}, ErrInvalidEffectiveTimeRule
	}
	return EffectiveTimeRule{
		tenantID: spec.TenantID,
		source:   spec.Source,
		version:  spec.Version,
		content:  spec.Content,
	}, nil
}

func (rule EffectiveTimeRule) TenantID() TenantID                { return rule.tenantID }
func (rule EffectiveTimeRule) Source() TrackingSourceReference   { return rule.source }
func (rule EffectiveTimeRule) Version() EffectiveTimeRuleVersion { return rule.version }
func (rule EffectiveTimeRule) Content() EffectiveTimeRuleContent { return rule.content }

// Supersedes 交回本版本回指的前版；首版第二个返回值为 false。
func (rule EffectiveTimeRule) Supersedes() (EffectiveTimeRuleVersion, bool) {
	return rule.supersedes, rule.supersedes.valid()
}

// Reference 是事实上记「按哪一版判的」时用的引用：规则名取所属源，版本取本版。
func (rule EffectiveTimeRule) Reference() EffectiveTimeRuleReference {
	return EffectiveTimeRuleReference{rule: rule.source.requiredValue, version: rule.version.requiredValue}
}

// Revise 换一版：新版本回指本版，租户与源原样带过去，正文换成新的。沿用本版本号就是覆盖，拒；
// 空正文不是规则，拒。不要求正文一定变——所有者登一版同内容的新版本是他的事，登记册只记版本链。
func (rule EffectiveTimeRule) Revise(
	version EffectiveTimeRuleVersion,
	content EffectiveTimeRuleContent,
) (EffectiveTimeRule, error) {
	if !rule.version.valid() || !version.valid() || version == rule.version || !content.valid() {
		return EffectiveTimeRule{}, ErrInvalidEffectiveTimeRule
	}
	revised := rule
	revised.version = version
	revised.content = content
	revised.supersedes = rule.version
	return revised, nil
}

// EffectiveTimeFor 按本版规则从一条素材的两个时间形成有效时间判断：锚点所指的时间加偏移。
// 判断回指本版（依据 JUDGED_BY_RULE）。锚点所指的时间为零值即拒——发生时间为零的素材根本不会
// 走到这里（ADR-0102 决定二先留痕），接收时间为零是端口没铸，两者都不是规则该补的。
func (rule EffectiveTimeRule) EffectiveTimeFor(occurredAt, receivedAt time.Time) (EffectiveTimeJudgment, error) {
	var anchored time.Time
	switch rule.content.Anchor {
	case AnchoredAtOccurrence:
		anchored = occurredAt
	case AnchoredAtReception:
		anchored = receivedAt
	default:
		return EffectiveTimeJudgment{}, ErrInvalidEffectiveTimeJudgment
	}
	if anchored.IsZero() {
		return EffectiveTimeJudgment{}, ErrInvalidEffectiveTimeJudgment
	}
	return JudgeEffectiveTimeByRule(rule.Reference(), anchored.Add(rule.content.Offset))
}

// Equal 按业务内容比较两个版本——同键异内容要答`内容冲突`而不是`已登记`（ADR-0031）。
func (rule EffectiveTimeRule) Equal(other EffectiveTimeRule) bool {
	return rule.tenantID == other.tenantID &&
		rule.source == other.source &&
		rule.version == other.version &&
		rule.content == other.content &&
		rule.supersedes == other.supersedes
}
