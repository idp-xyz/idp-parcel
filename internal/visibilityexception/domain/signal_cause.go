package domain

import (
	"strings"
	"time"
)

// CausalConditionReference 指名信号发作期的因果条件：「同一对象、信号类型、因果条件和连续影响期」里的那一维（CONTEXT）。
// 发作期按（租户、对象、类型、因果条件）定位，缺这一维时一件对象同一类型的头一次命中开了发作期，此后另一个原因的命中也只给
// 它记一次命中、不再分诊。
//
// 因果条件是什么由形成那一类信号的一方说，本类型只是开放引用；产品自带的两种写法见 SourceFactCausalCondition 与
// SupersessionForkCausalCondition，形状各带前缀，两类信号即使撞了类型词也不会撞因果条件。
type CausalConditionReference struct{ requiredValue }

func NewCausalConditionReference(value string) (CausalConditionReference, error) {
	required, err := newRequiredValue("causal condition reference", value)
	return CausalConditionReference{required}, err
}

// CausalConditionRecordedBeforeCauses 是因果条件一维立起来之前落库的发作期回填的那一格（VE 迁移 0030）。它们当初按（对象，
// 类型）定位，记的是哪一次原因已经说不出，事实依据也是空的：回填成这一格如实说「登记于因果条件之前」，而不是替它们猜一个原因。
// 新命中带的因果条件永远不等于它，所以存量发作期不再认领任何新命中；开期的构造门拒它，只有重建门放它进来。
const CausalConditionRecordedBeforeCauses = "RECORDED_BEFORE_CAUSAL_CONDITION"

// SourceFactCausalCondition 以一份源事实本身为因果条件：信号说的是那一份事实这一次发生了什么，同一份事实的重投与重放落在同一期，
// 另一份事实另开一期。出发门禁阻断信号用它——一次违规出发就是一个原因（代决复核报告 M18）。
func SourceFactCausalCondition(source SourceContext, fact SourceFactReference) (CausalConditionReference, error) {
	if !source.valid() || !fact.valid() {
		return CausalConditionReference{}, ErrInvalidEpisode
	}
	return NewCausalConditionReference("source-fact/" + source.String() + "/" + fact.String())
}

// SupersessionForkCausalCondition 以一条事实流（同一源上下文、同一事实引用）处在替代链分叉为因果条件：分叉不解，这条事实流在投影上
// 一直待确认，再派生时的每一次命中都是同一个原因；同包裹另一条事实流分叉是另一个原因。取事实流而不取分叉点的前身版本，是因为
// 分叉解没解按链头判（ADR-0245），链头说的是这条事实流。
func SupersessionForkCausalCondition(source SourceContext, fact SourceFactReference) (CausalConditionReference, error) {
	if !source.valid() || !fact.valid() {
		return CausalConditionReference{}, ErrInvalidEpisode
	}
	return NewCausalConditionReference("supersession-fork/" + source.String() + "/" + fact.String())
}

// SignalFactBasis 是信号的一条事实依据：「每个信号必须保存……事实依据」（CONTEXT）。Source、Fact、Version 指回本上下文已接受事实
// 的键维，人工复核据它追得回是哪一份事实的哪一版；Judgement 是那份源事实里随带的判断标识，可缺——出发门禁读数逐对象那一格的
// 关务判断标识就记在这里，复核时追得回关务凭哪一份判断答的阻断。
type SignalFactBasis struct {
	Source    SourceContext
	Fact      SourceFactReference
	Version   SourceFactVersion
	Judgement string
}

func (basis SignalFactBasis) valid() bool {
	if !basis.Source.valid() || !basis.Fact.valid() || !basis.Version.valid() {
		return false
	}
	return basis.Judgement == "" || strings.TrimSpace(basis.Judgement) != ""
}

// SignalHit 是一次信号命中带来的全部东西：对象、类型、因果条件、规则版本、可信度、事实依据与判断时间。开期与重开都只凭它，
// 编排不替命中补任何一样。
type SignalHit struct {
	Kind       ExceptionSignalKindReference
	Parcel     TrackedParcelReference
	Cause      CausalConditionReference
	Rule       SignalRuleVersionReference
	Confidence ConfidenceReference
	Basis      []SignalFactBasis
	At         time.Time
}

// Complete 判一次命中立不立得起来：各样齐全、事实依据至少一条且每条立得起来。编排受理那一步与开期、重开的构造门共用这一处
// 判据。存量那一格的因果条件不许新命中用：用了它，新命中就会落进存量发作期，那正是回填要挡住的。
func (hit SignalHit) Complete() bool {
	return hit.valid()
}

func (hit SignalHit) valid() bool {
	if !hit.Kind.valid() || !hit.Parcel.valid() || !hit.Cause.valid() || !hit.Rule.valid() ||
		!hit.Confidence.valid() || hit.At.IsZero() || len(hit.Basis) == 0 ||
		hit.Cause.String() == CausalConditionRecordedBeforeCauses {
		return false
	}
	for _, basis := range hit.Basis {
		if !basis.valid() {
			return false
		}
	}
	return true
}

func copyBasis(basis []SignalFactBasis) []SignalFactBasis {
	return append([]SignalFactBasis(nil), basis...)
}
