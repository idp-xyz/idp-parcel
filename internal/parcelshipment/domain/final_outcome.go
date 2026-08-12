package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidResponsibilityOutcome = errors.New("parcel shipment: invalid responsibility outcome")
	ErrInvalidParcelFinalOutcome    = errors.New("parcel shipment: invalid parcel final outcome")
	ErrInvalidCompletionInput       = errors.New("parcel shipment: invalid completion derivation input")
)

// ResponsibilityOutcomeKind 是可作为终局候选来源的责任结果封闭集合（UC-PS-004 终局
// 来源责任矩阵的四行；取消不在此列——取消终局由 UC-PS-006 形成后直接进汇总）。班次
// 完成、运输段关闭、POD 上传、外部状态码、异常案件与客户通知都没有格可落：它们不是
// 责任结果，构造期就进不来（AT-PS-055/056/093 的类型面）。
type ResponsibilityOutcomeKind uint8

const (
	ResponsibilityOutcomeKindInvalid ResponsibilityOutcomeKind = iota
	EffectiveDeliveryOutcome
	ReturnCompletedOutcome
	ServiceTerminatedOutcome
	RegulatoryDispositionExecuted
)

func (kind ResponsibilityOutcomeKind) valid() bool {
	switch kind {
	case EffectiveDeliveryOutcome, ReturnCompletedOutcome,
		ServiceTerminatedOutcome, RegulatoryDispositionExecuted:
		return true
	default:
		return false
	}
}

func (kind ResponsibilityOutcomeKind) String() string {
	switch kind {
	case EffectiveDeliveryOutcome:
		return "EFFECTIVE_DELIVERY"
	case ReturnCompletedOutcome:
		return "RETURN_COMPLETED"
	case ServiceTerminatedOutcome:
		return "SERVICE_TERMINATED"
	case RegulatoryDispositionExecuted:
		return "REGULATORY_DISPOSITION_EXECUTED"
	default:
		return ""
	}
}

// ResponsibilityDecisionReference 指名责任结果背后的权威决定（交付判断、退运/终止
// 决定、监管处置决定）。
type ResponsibilityDecisionReference struct{ requiredValue }

func NewResponsibilityDecisionReference(value string) (ResponsibilityDecisionReference, error) {
	required, err := newRequiredValue("responsibility decision reference", value)
	return ResponsibilityDecisionReference{required}, err
}

// ExecutionEvidenceReference 指名实际执行事实（有效交付结果、退运执行、销毁执行）。
type ExecutionEvidenceReference struct{ requiredValue }

func NewExecutionEvidenceReference(value string) (ExecutionEvidenceReference, error) {
	required, err := newRequiredValue("execution evidence reference", value)
	return ExecutionEvidenceReference{required}, err
}

// ResponsibilityOutcomeVersion 是责任结果的版本标识：终局采用判断按它幂等，更正与
// 失效形成新版本。
type ResponsibilityOutcomeVersion struct{ requiredValue }

func NewResponsibilityOutcomeVersion(value string) (ResponsibilityOutcomeVersion, error) {
	required, err := newRequiredValue("responsibility outcome version", value)
	return ResponsibilityOutcomeVersion{required}, err
}

// ResponsibilityOutcomeSpec 是一份责任结果引用所需的全部输入。
type ResponsibilityOutcomeSpec struct {
	Kind       ResponsibilityOutcomeKind
	Parcel     DeclaredParcelID
	Decision   ResponsibilityDecisionReference
	Execution  ExecutionEvidenceReference
	Version    ResponsibilityOutcomeVersion
	OccurredAt time.Time
}

// ResponsibilityOutcome 是对权威源责任结果的只读引用（强类型结果联合）。决定与执行
// 双引用都必备——监管销毁只有决定没有执行事实不形成终局（AT-PS-091），退运只有决定
// 没有完成同理（AT-PS-057/058）：「决定不等于执行完成」在构造期落地。
type ResponsibilityOutcome struct {
	kind       ResponsibilityOutcomeKind
	parcel     DeclaredParcelID
	decision   ResponsibilityDecisionReference
	execution  ExecutionEvidenceReference
	version    ResponsibilityOutcomeVersion
	occurredAt time.Time
}

func NewResponsibilityOutcome(spec ResponsibilityOutcomeSpec) (ResponsibilityOutcome, error) {
	if !spec.Kind.valid() ||
		!spec.Parcel.valid() ||
		!spec.Decision.valid() ||
		!spec.Execution.valid() ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() {
		return ResponsibilityOutcome{}, ErrInvalidResponsibilityOutcome
	}
	return ResponsibilityOutcome{
		kind:       spec.Kind,
		parcel:     spec.Parcel,
		decision:   spec.Decision,
		execution:  spec.Execution,
		version:    spec.Version,
		occurredAt: spec.OccurredAt.UTC(),
	}, nil
}

func (outcome ResponsibilityOutcome) Kind() ResponsibilityOutcomeKind {
	return outcome.kind
}

func (outcome ResponsibilityOutcome) Parcel() DeclaredParcelID {
	return outcome.parcel
}

func (outcome ResponsibilityOutcome) Decision() ResponsibilityDecisionReference {
	return outcome.decision
}

func (outcome ResponsibilityOutcome) Execution() ExecutionEvidenceReference {
	return outcome.execution
}

func (outcome ResponsibilityOutcome) Version() ResponsibilityOutcomeVersion {
	return outcome.version
}

func (outcome ResponsibilityOutcome) OccurredAt() time.Time {
	return outcome.occurredAt
}

// FinalOutcomeVersionID 是终局判断的版本标识。重派生换版本，历史不删。
type FinalOutcomeVersionID struct{ requiredValue }

func NewFinalOutcomeVersionID(value string) (FinalOutcomeVersionID, error) {
	required, err := newRequiredValue("final outcome version ID", value)
	return FinalOutcomeVersionID{required}, err
}

// FinalKindReference 指名合同规则给出的终局类型。它是开放引用不是封闭枚举——网络
// 服务「不统一规定一个跨产品终局结果集合」（UC-PS-004 终局形成规则），类型目录属
// 商业规则的实例半边。
type FinalKindReference struct{ requiredValue }

func NewFinalKindReference(value string) (FinalKindReference, error) {
	required, err := newRequiredValue("final kind reference", value)
	return FinalKindReference{required}, err
}

// FinalRuleVersionReference 指名判定所依据的合同终局规则版本。
type FinalRuleVersionReference struct{ requiredValue }

func NewFinalRuleVersionReference(value string) (FinalRuleVersionReference, error) {
	required, err := newRequiredValue("final rule version reference", value)
	return FinalRuleVersionReference{required}, err
}

// RederivationReason 指名一次终局重派生的依据（来源更正、POD 失效等有效性变化）。
type RederivationReason struct{ requiredValue }

func NewRederivationReason(value string) (RederivationReason, error) {
	required, err := newRequiredValue("rederivation reason", value)
	return RederivationReason{required}, err
}

// ParcelFinalOutcomeSpec 是形成一份包裹终局所需的全部输入。
type ParcelFinalOutcomeSpec struct {
	Version     FinalOutcomeVersionID
	Parcel      DeclaredParcelID
	Kind        FinalKindReference
	Source      ResponsibilityOutcome
	RuleVersion FinalRuleVersionReference
}

// ParcelFinalOutcome 是 parcel-shipment 唯一拥有的包裹服务终局。规则版本必备——没有
// 合同规则支撑的终局与「有效交付即所有产品终局」的默认值分不开（红线）；生效时间在
// 构造内取自责任结果的业务时间，处理时间进不来。类型上没有任何转取消的方法：终局
// 形成后不得回退为取消是结构性的。
type ParcelFinalOutcome struct {
	version      FinalOutcomeVersionID
	parcel       DeclaredParcelID
	kind         FinalKindReference
	source       ResponsibilityOutcome
	ruleVersion  FinalRuleVersionReference
	effectiveAt  time.Time
	priorVersion FinalOutcomeVersionID
	reason       RederivationReason
}

func FormParcelFinalOutcome(spec ParcelFinalOutcomeSpec) (ParcelFinalOutcome, error) {
	if !spec.Version.valid() ||
		!spec.Parcel.valid() ||
		!spec.Kind.valid() ||
		!spec.Source.kind.valid() ||
		!spec.RuleVersion.valid() ||
		spec.Parcel != spec.Source.parcel {
		return ParcelFinalOutcome{}, ErrInvalidParcelFinalOutcome
	}
	return ParcelFinalOutcome{
		version:     spec.Version,
		parcel:      spec.Parcel,
		kind:        spec.Kind,
		source:      spec.Source,
		ruleVersion: spec.RuleVersion,
		effectiveAt: spec.Source.occurredAt,
	}, nil
}

func (final ParcelFinalOutcome) Version() FinalOutcomeVersionID {
	return final.version
}

func (final ParcelFinalOutcome) Parcel() DeclaredParcelID {
	return final.parcel
}

func (final ParcelFinalOutcome) Kind() FinalKindReference {
	return final.kind
}

func (final ParcelFinalOutcome) Source() ResponsibilityOutcome {
	return final.source
}

func (final ParcelFinalOutcome) RuleVersion() FinalRuleVersionReference {
	return final.ruleVersion
}

// EffectiveAt 恒等于被采用责任结果的业务时间。
func (final ParcelFinalOutcome) EffectiveAt() time.Time {
	return final.effectiveAt
}

// PriorVersion 只在重派生版本上给出，指回被替代的那一版。
func (final ParcelFinalOutcome) PriorVersion() (FinalOutcomeVersionID, bool) {
	return final.priorVersion, final.priorVersion.valid()
}

// RederivationBasis 只在重派生版本上给出。
func (final ParcelFinalOutcome) RederivationBasis() (RederivationReason, bool) {
	return final.reason, final.reason.valid()
}

// Rederive 依据来源更正或有效性变化形成新的当前判断版本（AT-PS-063）：换版本、带
// 原因、换新的责任结果、指回原版；原终局历史不可变地保留。这不是删除历史，也不是
// 业务重开——原版本还在它自己的位置上。
func (final ParcelFinalOutcome) Rederive(
	version FinalOutcomeVersionID,
	source ResponsibilityOutcome,
	reason RederivationReason,
) (ParcelFinalOutcome, error) {
	if !version.valid() || !reason.valid() || version == final.version {
		return ParcelFinalOutcome{}, ErrInvalidParcelFinalOutcome
	}
	if !source.kind.valid() || source.parcel != final.parcel {
		return ParcelFinalOutcome{}, ErrInvalidParcelFinalOutcome
	}
	rederived := final
	rederived.version = version
	rederived.source = source
	rederived.effectiveAt = source.occurredAt
	rederived.priorVersion = final.version
	rederived.reason = reason
	return rederived, nil
}

// CompletionState 是委托完成派生的封闭三值。
type CompletionState uint8

const (
	CompletionStateInvalid CompletionState = iota
	CompletionNone
	CompletionPartial
	CompletionComplete
)

func (state CompletionState) String() string {
	switch state {
	case CompletionNone:
		return "NONE"
	case CompletionPartial:
		return "PARTIAL"
	case CompletionComplete:
		return "COMPLETE"
	default:
		return ""
	}
}

// MemberFinalState 是完成派生的逐包裹输入：成员有没有适用终局，以及那份终局是不是
// 取消。取消终局与履约终局在完成度上同格（UC-PS-006 的取消直接进汇总），但委托级
// `已取消`的派生要分得开两种终局——所以这里带一个取消位，而不是让调用方拼两套输入。
type MemberFinalState struct {
	Parcel    DeclaredParcelID
	Finalized bool
	Cancelled bool
}

// ShipmentCompletionSummary 是委托完成的派生摘要：逐包裹结果的汇总，不是可编辑状态。
type ShipmentCompletionSummary struct {
	state        CompletionState
	finalized    int
	cancelled    int
	total        int
	allCancelled bool
}

// DeriveShipmentCompletion 依据当前有效包裹集合派生完成摘要（UC-PS-004 步骤 7）。
// 输入必须覆盖当前全部有效成员且不重复——漏一个成员的「全部完成」是假话；空集合
// 立不出摘要；标了取消却没标终局的成员是矛盾输入（取消是一种终局）。单个成员未决
// 只让摘要停在部分完成，不回滚其他终局（AT-PS-054）。
func DeriveShipmentCompletion(members []MemberFinalState) (ShipmentCompletionSummary, error) {
	if len(members) == 0 {
		return ShipmentCompletionSummary{}, ErrInvalidCompletionInput
	}
	seen := make(map[DeclaredParcelID]bool, len(members))
	finalized := 0
	cancelled := 0
	for _, member := range members {
		if !member.Parcel.valid() || seen[member.Parcel] {
			return ShipmentCompletionSummary{}, ErrInvalidCompletionInput
		}
		if member.Cancelled && !member.Finalized {
			return ShipmentCompletionSummary{}, ErrInvalidCompletionInput
		}
		seen[member.Parcel] = true
		if member.Finalized {
			finalized++
		}
		if member.Cancelled {
			cancelled++
		}
	}
	state := CompletionNone
	switch {
	case finalized == len(members):
		state = CompletionComplete
	case finalized > 0:
		state = CompletionPartial
	}
	return ShipmentCompletionSummary{
		state:        state,
		finalized:    finalized,
		cancelled:    cancelled,
		total:        len(members),
		allCancelled: cancelled == len(members),
	}, nil
}

func (summary ShipmentCompletionSummary) State() CompletionState {
	return summary.state
}

func (summary ShipmentCompletionSummary) Finalized() int {
	return summary.finalized
}

func (summary ShipmentCompletionSummary) Cancelled() int {
	return summary.cancelled
}

func (summary ShipmentCompletionSummary) Total() int {
	return summary.total
}

// DerivesShipmentCancelled 报告委托是否派生为`已取消`：只有全部当前有效包裹均为取消
// 终局且不存在非取消终局时才成立（UC-PS-006 一致性硬句）。一件已交付加一件已取消的
// 委托是`全部完成`而不是`已取消`——那件交付责任真实发生过。
func (summary ShipmentCompletionSummary) DerivesShipmentCancelled() bool {
	return summary.allCancelled
}
