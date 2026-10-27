package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidETAVisibilityRule      = errors.New("visibility exception: invalid ETA customer visibility rule")
	ErrInvalidETAVisibilityJudgement = errors.New("visibility exception: invalid ETA customer visibility judgement")
	// ErrETAOutsideRuleTarget 说明候选 ETA 的里程碑不是适用规则指名的客户可见 ETA 目标里程碑。
	// 这不是判断出来的「不可见」：规则没有就这个里程碑的 ETA 说过话，不判。
	ErrETAOutsideRuleTarget = errors.New("visibility exception: the ETA milestone is not the rule's customer-facing target")
)

// ETAQualityGateVersion 指名 `PAR-VIS-03` 的 ETA 对客质量门槛版本。
type ETAQualityGateVersion struct{ requiredValue }

func NewETAQualityGateVersion(value string) (ETAQualityGateVersion, error) {
	required, err := newRequiredValue("ETA quality gate version", value)
	return ETAQualityGateVersion{required}, err
}

// CustomerServiceRuleVersion 指名 `PAR-COM-07` 的合同与客户服务规则版本。
type CustomerServiceRuleVersion struct{ requiredValue }

func NewCustomerServiceRuleVersion(value string) (CustomerServiceRuleVersion, error) {
	required, err := newRequiredValue("customer service rule version", value)
	return CustomerServiceRuleVersion{required}, err
}

// ETAStopShowingBoundary 指名获准对客的 ETA 何时停止展示的边界。边界怎样取值是租户登记的
// 内容（`PAR-VIS-03` 的「停止展示边界」），这里只留引用，不替登记册发明一套边界语法。
type ETAStopShowingBoundary struct{ requiredValue }

func NewETAStopShowingBoundary(value string) (ETAStopShowingBoundary, error) {
	required, err := newRequiredValue("ETA stop showing boundary", value)
	return ETAStopShowingBoundary{required}, err
}

// ETAExpression 是获准对客的 ETA 表达形态的封闭二值（CONTEXT「展示点值还是时间范围」）。
type ETAExpression uint8

const (
	ETAExpressionInvalid ETAExpression = iota
	ETAShownAsPointValue
	ETAShownAsTimeRange
)

func (expression ETAExpression) valid() bool {
	return expression == ETAShownAsPointValue || expression == ETAShownAsTimeRange
}

func (expression ETAExpression) String() string {
	switch expression {
	case ETAShownAsPointValue:
		return "POINT_VALUE"
	case ETAShownAsTimeRange:
		return "TIME_RANGE"
	default:
		return ""
	}
}

// ETAVisibilityRuleSpec 是租户为一个客户账户登记的 ETA 对客规则：对照哪一版质量门槛与哪一版
// 合同/客户服务规则、哪个里程碑的 ETA 是客户视图要的那一个、哪些可信度依据达得到门槛、对客
// 怎么表达、何时停止展示。
type ETAVisibilityRuleSpec struct {
	GateVersion     ETAQualityGateVersion
	ServiceRule     CustomerServiceRuleVersion
	TargetMilestone MilestoneReference
	Qualifying      []ConfidenceReference
	Expression      ETAExpression
	StopBoundary    ETAStopShowingBoundary
}

// ETAVisibilityRule 是适用于某个客户账户的一条 ETA 对客规则。可信度依据在内部 ETA 上只是一个
// 不透明引用（ConfidenceReference），不存在可比大小的数值，所以门槛只能表达为「哪些依据达标」
// 这一个集合——不在集合里的依据一律未达门槛，不替登记册推一条序。
type ETAVisibilityRule struct {
	gate         ETAQualityGateVersion
	service      CustomerServiceRuleVersion
	target       MilestoneReference
	qualifying   []ConfidenceReference
	expression   ETAExpression
	stopBoundary ETAStopShowingBoundary
}

func NewETAVisibilityRule(spec ETAVisibilityRuleSpec) (ETAVisibilityRule, error) {
	if !spec.GateVersion.valid() ||
		!spec.ServiceRule.valid() ||
		!spec.TargetMilestone.valid() ||
		len(spec.Qualifying) == 0 ||
		!spec.Expression.valid() ||
		!spec.StopBoundary.valid() {
		return ETAVisibilityRule{}, ErrInvalidETAVisibilityRule
	}
	qualifying := make([]ConfidenceReference, 0, len(spec.Qualifying))
	for _, confidence := range spec.Qualifying {
		if !confidence.valid() {
			return ETAVisibilityRule{}, ErrInvalidETAVisibilityRule
		}
		qualifying = append(qualifying, confidence)
	}
	return ETAVisibilityRule{
		gate:         spec.GateVersion,
		service:      spec.ServiceRule,
		target:       spec.TargetMilestone,
		qualifying:   qualifying,
		expression:   spec.Expression,
		stopBoundary: spec.StopBoundary,
	}, nil
}

func (rule ETAVisibilityRule) GateVersion() ETAQualityGateVersion {
	return rule.gate
}

func (rule ETAVisibilityRule) ServiceRule() CustomerServiceRuleVersion {
	return rule.service
}

func (rule ETAVisibilityRule) TargetMilestone() MilestoneReference {
	return rule.target
}

func (rule ETAVisibilityRule) Expression() ETAExpression {
	return rule.expression
}

func (rule ETAVisibilityRule) StopBoundary() ETAStopShowingBoundary {
	return rule.stopBoundary
}

// Qualifying 交回达标可信度依据的副本。
func (rule ETAVisibilityRule) Qualifying() []ConfidenceReference {
	return append([]ConfidenceReference(nil), rule.qualifying...)
}

func (rule ETAVisibilityRule) qualifies(confidence ConfidenceReference) bool {
	for _, accepted := range rule.qualifying {
		if accepted == confidence {
			return true
		}
	}
	return false
}

// ETARuleBasisState 是适用规则的封闭三态：有一条、没有、有两条以上同时适用。后两格是如实的
// 缺失与冲突（`AT-VE-170`），不是错误，也不是可以按默认值补上的空白。
type ETARuleBasisState uint8

const (
	ETARuleBasisInvalid ETARuleBasisState = iota
	ETARuleConfigured
	ETARuleNotConfigured
	ETARulesConflicting
)

func (state ETARuleBasisState) String() string {
	switch state {
	case ETARuleConfigured:
		return "CONFIGURED"
	case ETARuleNotConfigured:
		return "NOT_CONFIGURED"
	case ETARulesConflicting:
		return "CONFLICTING"
	default:
		return ""
	}
}

// ETAVisibilityRuleBasis 是判断时点上适用规则的答复。
type ETAVisibilityRuleBasis struct {
	state ETARuleBasisState
	rule  ETAVisibilityRule
}

func ConfiguredETARule(rule ETAVisibilityRule) ETAVisibilityRuleBasis {
	return ETAVisibilityRuleBasis{state: ETARuleConfigured, rule: rule}
}

func UnconfiguredETARule() ETAVisibilityRuleBasis {
	return ETAVisibilityRuleBasis{state: ETARuleNotConfigured}
}

func ConflictingETARules() ETAVisibilityRuleBasis {
	return ETAVisibilityRuleBasis{state: ETARulesConflicting}
}

func (basis ETAVisibilityRuleBasis) State() ETARuleBasisState {
	return basis.state
}

// Rule 只在规则唯一适用时给出。
func (basis ETAVisibilityRuleBasis) Rule() (ETAVisibilityRule, bool) {
	return basis.rule, basis.state == ETARuleConfigured
}

// ETAVisibilityConclusion 是 ETA 客户可见性判断结论的封闭三格（UC-VE-003 结果语义）。
type ETAVisibilityConclusion uint8

const (
	ETAVisibilityConclusionInvalid ETAVisibilityConclusion = iota
	CustomerVisibleETA
	ETANotVisible
	ETAVisibilityPendingJudgement
)

func (conclusion ETAVisibilityConclusion) String() string {
	switch conclusion {
	case CustomerVisibleETA:
		return "CUSTOMER_VISIBLE_ETA"
	case ETANotVisible:
		return "ETA_NOT_VISIBLE"
	case ETAVisibilityPendingJudgement:
		return "ETA_VISIBILITY_PENDING"
	default:
		return ""
	}
}

// ETAVisibilityReason 是结论的理由，也是质量评价的结果：达标与未达标两格就是评价本身，缺失与
// 冲突两格说明为什么还评不了。
type ETAVisibilityReason uint8

const (
	ETAVisibilityReasonInvalid ETAVisibilityReason = iota
	QualityMetTheGate
	QualityBelowTheGate
	CustomerRuleNotConfigured
	CustomerRuleInConflict
)

func (reason ETAVisibilityReason) String() string {
	switch reason {
	case QualityMetTheGate:
		return "QUALITY_MET_GATE"
	case QualityBelowTheGate:
		return "QUALITY_BELOW_GATE"
	case CustomerRuleNotConfigured:
		return "RULE_NOT_CONFIGURED"
	case CustomerRuleInConflict:
		return "RULE_CONFLICT"
	default:
		return ""
	}
}

// ETAVisibilityFinding 是对一份候选内部 ETA 与一份适用规则作出的发现：结论、理由和它依据的
// 版本。它是可比较的值——同一候选 ETA 版本、同一规则依据得同一发现，应用层据此判重放，不为
// 没变的依据再换一代判断。
type ETAVisibilityFinding struct {
	Parcel           TrackedParcelReference
	Milestone        MilestoneReference
	CandidateVersion ETAVersionID
	CandidateSource  ETASourceKind
	Conclusion       ETAVisibilityConclusion
	Reason           ETAVisibilityReason
	GateVersion      ETAQualityGateVersion
	ServiceRule      CustomerServiceRuleVersion
	Expression       ETAExpression
	StopBoundary     ETAStopShowingBoundary
}

// valid 钉结论与字段的成对：可见必带规则版本、表达与停止边界；不可见带规则版本、不带表达；
// 待判断什么规则字段都不带——没有适用规则就没有可借用的版本。
func (finding ETAVisibilityFinding) valid() bool {
	if !finding.Parcel.valid() ||
		!finding.Milestone.valid() ||
		!finding.CandidateVersion.valid() ||
		!finding.CandidateSource.valid() {
		return false
	}
	switch finding.Conclusion {
	case CustomerVisibleETA:
		return finding.Reason == QualityMetTheGate &&
			finding.GateVersion.valid() &&
			finding.ServiceRule.valid() &&
			finding.Expression.valid() &&
			finding.StopBoundary.valid()
	case ETANotVisible:
		return finding.Reason == QualityBelowTheGate &&
			finding.GateVersion.valid() &&
			finding.ServiceRule.valid() &&
			finding.Expression == ETAExpressionInvalid &&
			!finding.StopBoundary.valid()
	case ETAVisibilityPendingJudgement:
		return (finding.Reason == CustomerRuleNotConfigured || finding.Reason == CustomerRuleInConflict) &&
			!finding.GateVersion.valid() &&
			!finding.ServiceRule.valid() &&
			finding.Expression == ETAExpressionInvalid &&
			!finding.StopBoundary.valid()
	default:
		return false
	}
}

// AssessETAVisibility 对一份候选内部 ETA 作发现。适用规则缺失或冲突时落「待判断」，不按默认值
// 判为可见（`AT-VE-170`）；规则唯一适用时，候选的可信度依据在达标集合里即可见（`AT-VE-187`），
// 不在即不可见（`AT-VE-052`）。规则指名的目标里程碑不是候选 ETA 的里程碑时不判。
func AssessETAVisibility(eta ETAPrediction, basis ETAVisibilityRuleBasis) (ETAVisibilityFinding, error) {
	finding := ETAVisibilityFinding{
		Parcel:           eta.parcel,
		Milestone:        eta.milestone,
		CandidateVersion: eta.version,
		CandidateSource:  eta.source,
	}
	switch basis.state {
	case ETARuleNotConfigured:
		finding.Conclusion = ETAVisibilityPendingJudgement
		finding.Reason = CustomerRuleNotConfigured
	case ETARulesConflicting:
		finding.Conclusion = ETAVisibilityPendingJudgement
		finding.Reason = CustomerRuleInConflict
	case ETARuleConfigured:
		if basis.rule.target != eta.milestone {
			return ETAVisibilityFinding{}, ErrETAOutsideRuleTarget
		}
		finding.GateVersion = basis.rule.gate
		finding.ServiceRule = basis.rule.service
		if basis.rule.qualifies(eta.confidence) {
			finding.Conclusion = CustomerVisibleETA
			finding.Reason = QualityMetTheGate
			finding.Expression = basis.rule.expression
			finding.StopBoundary = basis.rule.stopBoundary
		} else {
			finding.Conclusion = ETANotVisible
			finding.Reason = QualityBelowTheGate
		}
	default:
		return ETAVisibilityFinding{}, ErrInvalidETAVisibilityJudgement
	}
	if !finding.valid() {
		return ETAVisibilityFinding{}, ErrInvalidETAVisibilityJudgement
	}
	return finding, nil
}

// ETAVisibilityJudgementVersionID 是 ETA 客户可见性判断的版本标识。新的 ETA、质量门槛或客户服务
// 规则版本形成新判断，原判断不覆盖。
type ETAVisibilityJudgementVersionID struct{ requiredValue }

func NewETAVisibilityJudgementVersionID(value string) (ETAVisibilityJudgementVersionID, error) {
	required, err := newRequiredValue("ETA visibility judgement version ID", value)
	return ETAVisibilityJudgementVersionID{required}, err
}

// ETAVisibilityJudgement 是针对一个货主客户账户、一件包裹、规则指名的目标里程碑，对一份候选内部
// ETA 作出的客户可见性判断（UC-VE-003 结果语义）。它引用候选 ETA 版本而不复制预测内容；
// 结论只有客户可见 ETA、ETA 不可见、ETA 可见性待判断三格，类型上没有任何旁路能把「待判断」
// 或「不可见」说成可见。
type ETAVisibilityJudgement struct {
	version  ETAVisibilityJudgementVersionID
	customer CustomerAccountReference
	finding  ETAVisibilityFinding
	judgedAt time.Time
	prior    ETAVisibilityJudgementVersionID
}

// FormETAVisibilityJudgement 形成一个账户在一件包裹上的首个判断版本。
func FormETAVisibilityJudgement(
	version ETAVisibilityJudgementVersionID,
	customer CustomerAccountReference,
	finding ETAVisibilityFinding,
	judgedAt time.Time,
) (ETAVisibilityJudgement, error) {
	if !version.valid() || !customer.valid() || !finding.valid() || judgedAt.IsZero() {
		return ETAVisibilityJudgement{}, ErrInvalidETAVisibilityJudgement
	}
	return ETAVisibilityJudgement{
		version:  version,
		customer: customer,
		finding:  finding,
		judgedAt: judgedAt.UTC(),
	}, nil
}

// Supersede 形成替代版本：换版本、换发现、指回原版；原判断一个字段都不动。
// 包裹必须同一件——别件包裹的判断不是这条判断线上的下一版。
func (judgement ETAVisibilityJudgement) Supersede(
	version ETAVisibilityJudgementVersionID,
	finding ETAVisibilityFinding,
	judgedAt time.Time,
) (ETAVisibilityJudgement, error) {
	if version == judgement.version || finding.Parcel != judgement.finding.Parcel {
		return ETAVisibilityJudgement{}, ErrInvalidETAVisibilityJudgement
	}
	superseded, err := FormETAVisibilityJudgement(version, judgement.customer, finding, judgedAt)
	if err != nil {
		return ETAVisibilityJudgement{}, err
	}
	superseded.prior = judgement.version
	return superseded, nil
}

// ETAVisibilityJudgementSnapshot 是持久化层重建判断所需的全量状态。
type ETAVisibilityJudgementSnapshot struct {
	Version  ETAVisibilityJudgementVersionID
	Customer CustomerAccountReference
	Finding  ETAVisibilityFinding
	JudgedAt time.Time
	Prior    ETAVisibilityJudgementVersionID
}

// RehydrateETAVisibilityJudgement 从快照重建判断。读回的东西同样要过一遍结论与字段成对的
// 不变量，否则一次坏写入会在这里变成一个看起来合法的「客户可见」。
func RehydrateETAVisibilityJudgement(snapshot ETAVisibilityJudgementSnapshot) (ETAVisibilityJudgement, error) {
	judgement, err := FormETAVisibilityJudgement(
		snapshot.Version, snapshot.Customer, snapshot.Finding, snapshot.JudgedAt)
	if err != nil {
		return ETAVisibilityJudgement{}, err
	}
	if snapshot.Prior.valid() {
		if snapshot.Prior == snapshot.Version {
			return ETAVisibilityJudgement{}, ErrInvalidETAVisibilityJudgement
		}
		judgement.prior = snapshot.Prior
	}
	return judgement, nil
}

func (judgement ETAVisibilityJudgement) Version() ETAVisibilityJudgementVersionID {
	return judgement.version
}

func (judgement ETAVisibilityJudgement) Customer() CustomerAccountReference {
	return judgement.customer
}

func (judgement ETAVisibilityJudgement) Finding() ETAVisibilityFinding {
	return judgement.finding
}

func (judgement ETAVisibilityJudgement) Parcel() TrackedParcelReference {
	return judgement.finding.Parcel
}

func (judgement ETAVisibilityJudgement) Milestone() MilestoneReference {
	return judgement.finding.Milestone
}

func (judgement ETAVisibilityJudgement) CandidateVersion() ETAVersionID {
	return judgement.finding.CandidateVersion
}

func (judgement ETAVisibilityJudgement) CandidateSource() ETASourceKind {
	return judgement.finding.CandidateSource
}

func (judgement ETAVisibilityJudgement) Conclusion() ETAVisibilityConclusion {
	return judgement.finding.Conclusion
}

func (judgement ETAVisibilityJudgement) Reason() ETAVisibilityReason {
	return judgement.finding.Reason
}

func (judgement ETAVisibilityJudgement) JudgedAt() time.Time {
	return judgement.judgedAt
}

// RuleBasis 给出判断所依据的门槛版本与客户服务规则版本；待判断没有适用规则，第三个返回值为假。
func (judgement ETAVisibilityJudgement) RuleBasis() (ETAQualityGateVersion, CustomerServiceRuleVersion, bool) {
	return judgement.finding.GateVersion, judgement.finding.ServiceRule, judgement.finding.GateVersion.valid()
}

// CustomerExpression 给出获准对客的表达与停止展示边界，只在结论为客户可见 ETA 时有。
func (judgement ETAVisibilityJudgement) CustomerExpression() (ETAExpression, ETAStopShowingBoundary, bool) {
	return judgement.finding.Expression, judgement.finding.StopBoundary,
		judgement.finding.Conclusion == CustomerVisibleETA
}

// PriorVersion 只在替代版本上给出，指回被替代的那一版。
func (judgement ETAVisibilityJudgement) PriorVersion() (ETAVisibilityJudgementVersionID, bool) {
	return judgement.prior, judgement.prior.valid()
}

// contentReference 是客户视图 ETA 一维展示时的内容来处：就是这一版判断。获准对客的 ETA
// 表达与停止展示边界挂在判断上，视图只指名来处；判断换版，来处就换，视图随之换代并指回前版。
func (judgement ETAVisibilityJudgement) contentReference() ViewContentReference {
	return ViewContentReference{judgement.version.requiredValue}
}
