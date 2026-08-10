package domain

// ExclusionRule 是卡上直接拒收包裹、而不是对它计费的条款。公开渠道条款把超限处理分档
// 表述——过第一道界限加一笔附加费，过第二道加更多，过最后一道则拒收——所以排除规则通过
// 与附加费相同的触发条件文法读取相同的特征，区别只在效果：附加费加钱，排除意味着根本
// 不存在价格。
//
// 它携带自己出自哪一条条款，因为 CONTEXT 要求结果指名排除依据；光有一个状态，在争议中
// 无法对着卡核对。
type ExclusionRule struct {
	id        string
	clause    string
	condition TriggerCondition
}

func NewExclusionRule(id, clause string, condition TriggerCondition) (ExclusionRule, error) {
	rule := ExclusionRule{id: id, clause: clause, condition: condition}
	if !rule.valid() {
		return ExclusionRule{}, ErrInvalidExclusionRule
	}
	return rule, nil
}

func (rule ExclusionRule) ID() string                  { return rule.id }
func (rule ExclusionRule) Clause() string              { return rule.clause }
func (rule ExclusionRule) Condition() TriggerCondition { return rule.condition }

func (rule ExclusionRule) valid() bool {
	return trimmed(rule.id) && trimmed(rule.clause) && rule.condition.valid()
}

// resolveExclusions 报出第一条拒收本包裹的已声明条款。即使已经有一条成立，其余条款仍
// 逐条判定：任何一处单位不一致都是卡必须修正的声明错误，让先命中的那条把它盖住，会让
// 同一个方案对一件包裹静默拒收、对下一件报错。报出哪一条是稳定的，因为结构集合按 id
// 有序保存它们。
func (structures PricingPlanStructures) resolveExclusions(features PackageFeatures) (ExclusionRule, bool, error) {
	var excluded ExclusionRule
	found := false
	for _, rule := range structures.exclusions {
		held, err := rule.condition.Matches(features)
		if err != nil {
			return ExclusionRule{}, false, err
		}
		if held && !found {
			excluded, found = rule, true
		}
	}
	return excluded, found, nil
}
