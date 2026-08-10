package domain

import "strings"

// TriggerKind 是触发条件的构成方式：一个判定条件，或其全部操作数成立，或任一操作数
// 成立。集合封闭为三种，正是这一点让触发条件可枚举——每个叶子的命中与未命中都能仅凭
// 版本清单报出来，而表达式引擎给不了这个。
type TriggerKind string

const (
	TriggerPredicate TriggerKind = "PREDICATE"
	TriggerAllOf     TriggerKind = "ALL_OF"
	TriggerAnyOf     TriggerKind = "ANY_OF"
)

func (kind TriggerKind) String() string { return string(kind) }

func (kind TriggerKind) valid() bool {
	switch kind {
	case TriggerPredicate, TriggerAllOf, TriggerAnyOf:
		return true
	default:
		return false
	}
}

// TriggerCondition 是一条规则声明的「什么情况下它成立」。卡写的是一条含多个替代项的
// 条款，而不是多条条款：`R40` 对超过 67.5 KG **或** 长于 274 CM **或** 长加围超过
// 419 CM 的件计收一次。把它拆成三条规则，会把卡只收一次的东西收三次，所以组合必须属于
// 规则本身，而不是规则被列出的方式的一个属性。
type TriggerCondition struct {
	kind      TriggerKind
	predicate FeatureCondition
	operands  []TriggerCondition
}

// NewTrigger 包装单个判定条件。规则一律声明触发条件，所以只有一个判定条件的规则也如实
// 这么表达，而不是换成另一种形状。
func NewTrigger(predicate FeatureCondition) (TriggerCondition, error) {
	trigger := TriggerCondition{kind: TriggerPredicate, predicate: predicate}
	if !trigger.valid() {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	return trigger, nil
}

func NewAllOfTrigger(operands ...TriggerCondition) (TriggerCondition, error) {
	return newCombinedTrigger(TriggerAllOf, operands)
}

func NewAnyOfTrigger(operands ...TriggerCondition) (TriggerCondition, error) {
	return newCombinedTrigger(TriggerAnyOf, operands)
}

func newCombinedTrigger(kind TriggerKind, operands []TriggerCondition) (TriggerCondition, error) {
	// 空组合没有卡可能表达的读法；当作恒真或恒假，都是在发明一条没人声明过的规则。
	if len(operands) == 0 {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	trigger := TriggerCondition{kind: kind, operands: append([]TriggerCondition(nil), operands...)}
	if !trigger.valid() {
		return TriggerCondition{}, ErrInvalidFeatureCondition
	}
	return trigger, nil
}

func (trigger TriggerCondition) Kind() TriggerKind { return trigger.kind }

func (trigger TriggerCondition) Predicate() (FeatureCondition, bool) {
	if trigger.kind != TriggerPredicate {
		return FeatureCondition{}, false
	}
	return trigger.predicate, true
}

func (trigger TriggerCondition) Operands() []TriggerCondition {
	return append([]TriggerCondition(nil), trigger.operands...)
}

// Matches 报出触发条件是否成立。它不做短路求值：组合中任何一处单位不一致都是卡必须
// 修正的声明错误，把它藏在一个恰好先成立的替代项后面，会让同一个方案每次运行给出不同
// 的解释。
func (trigger TriggerCondition) Matches(features PackageFeatures) (bool, error) {
	if !trigger.valid() || !features.valid() {
		return false, ErrInvalidFeatureCondition
	}
	if trigger.kind == TriggerPredicate {
		return trigger.predicate.Matches(features)
	}
	held := trigger.kind == TriggerAllOf
	for _, operand := range trigger.operands {
		matched, err := operand.Matches(features)
		if err != nil {
			return false, err
		}
		switch trigger.kind {
		case TriggerAllOf:
			held = held && matched
		case TriggerAnyOf:
			held = held || matched
		}
	}
	return held, nil
}

// describe 呈现整个组合，而不只是它的第一个叶子，这样未命中的解释才说得出是哪一条不
// 满足，而不是只点一个替代项、让其余的看不见。
func (trigger TriggerCondition) describe() string {
	switch trigger.kind {
	case TriggerPredicate:
		return trigger.predicate.describe()
	case TriggerAllOf, TriggerAnyOf:
		parts := make([]string, 0, len(trigger.operands))
		for _, operand := range trigger.operands {
			parts = append(parts, operand.describe())
		}
		label := "all of"
		if trigger.kind == TriggerAnyOf {
			label = "any of"
		}
		return label + " (" + strings.Join(parts, ", ") + ")"
	default:
		return ""
	}
}

func (trigger TriggerCondition) valid() bool {
	if !trigger.kind.valid() {
		return false
	}
	if trigger.kind == TriggerPredicate {
		return len(trigger.operands) == 0 && trigger.predicate.valid()
	}
	if len(trigger.operands) == 0 {
		return false
	}
	for _, operand := range trigger.operands {
		if !operand.valid() {
			return false
		}
	}
	return true
}
