package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCommercialBasisSnapshot = errors.New("parcel shipment: invalid commercial basis snapshot")
	ErrInvalidDeclaredAsOf            = errors.New("parcel shipment: invalid declared as-of")
	ErrInvalidReachabilityJudgment    = errors.New("parcel shipment: invalid reachability judgment")
)

// 这里的类型是 parcel-shipment 自己对其他上下文所拥有事实的引用。party-commercial 与
// network-routing 各自保有自己的模型；本上下文只记录所采用的引用与快照——这正是两边
// 各自演进而互不改写对方对象的原因。

type CommercialResolutionID struct{ requiredValue }

func NewCommercialResolutionID(value string) (CommercialResolutionID, error) {
	required, err := newRequiredValue("commercial resolution ID", value)
	return CommercialResolutionID{required}, err
}

type RulePackageReference struct{ requiredValue }

func NewRulePackageReference(value string) (RulePackageReference, error) {
	required, err := newRequiredValue("rule package reference", value)
	return RulePackageReference{required}, err
}

type CommercialViewRevision struct{ requiredValue }

func NewCommercialViewRevision(value string) (CommercialViewRevision, error) {
	required, err := newRequiredValue("commercial view revision", value)
	return CommercialViewRevision{required}, err
}

type AsOfPolicyVersion struct{ requiredValue }

func NewAsOfPolicyVersion(value string) (AsOfPolicyVersion, error) {
	required, err := newRequiredValue("as-of policy version", value)
	return AsOfPolicyVersion{required}, err
}

type ReachabilityJudgmentID struct{ requiredValue }

func NewReachabilityJudgmentID(value string) (ReachabilityJudgmentID, error) {
	required, err := newRequiredValue("reachability judgment ID", value)
	return ReachabilityJudgmentID{required}, err
}

// JudgmentKind 指名一类由所采用规则包声明 `asOf` 策略的下游判断。取值与消费它的编排
// 同时出现；接受前财务控制暂缺，因为那一步还没有被编排。
type JudgmentKind uint8

const (
	JudgmentKindInvalid JudgmentKind = iota
	ReachabilityJudgmentKind
)

func (kind JudgmentKind) valid() bool {
	return kind == ReachabilityJudgmentKind
}

func (kind JudgmentKind) String() string {
	switch kind {
	case ReachabilityJudgmentKind:
		return "REACHABILITY"
	default:
		return ""
	}
}

// DeclaredAsOf 是所采用规则包为某一类判断声明的时点。parcel-shipment 据此形成值，
// 绝不自己发明一个，所以没有声明的判断根本无法继续。
type DeclaredAsOf struct {
	kind          JudgmentKind
	at            time.Time
	policyVersion AsOfPolicyVersion
}

func NewDeclaredAsOf(kind JudgmentKind, at time.Time, policyVersion AsOfPolicyVersion) (DeclaredAsOf, error) {
	if !kind.valid() || at.IsZero() || !policyVersion.valid() {
		return DeclaredAsOf{}, ErrInvalidDeclaredAsOf
	}
	return DeclaredAsOf{kind: kind, at: at.UTC(), policyVersion: policyVersion}, nil
}

func (declared DeclaredAsOf) Kind() JudgmentKind {
	return declared.kind
}

func (declared DeclaredAsOf) At() time.Time {
	return declared.at
}

func (declared DeclaredAsOf) PolicyVersion() AsOfPolicyVersion {
	return declared.policyVersion
}

// JudgmentAsOf 是实际送给权威提供方、并由其校验回显的时点。它与声明同形，因为形成值
// 时不得添加任何策略没有授权的东西。
type JudgmentAsOf = DeclaredAsOf

// CommercialBasisSnapshot 是 parcel-shipment 对一次唯一商业解析所保留的部分：解析
// 标识、采用的接单规则包、解析当时的权威视图修订，以及该规则包声明的各项时点。它不
// 持有任何商业版本内容，那些内容属 party-commercial。
type CommercialBasisSnapshot struct {
	resolutionID CommercialResolutionID
	rulePackage  RulePackageReference
	viewRevision CommercialViewRevision
	declaredAsOf []DeclaredAsOf
}

func NewCommercialBasisSnapshot(
	resolutionID CommercialResolutionID,
	rulePackage RulePackageReference,
	viewRevision CommercialViewRevision,
	declaredAsOf []DeclaredAsOf,
) (CommercialBasisSnapshot, error) {
	if !resolutionID.valid() || !rulePackage.valid() || !viewRevision.valid() {
		return CommercialBasisSnapshot{}, ErrInvalidCommercialBasisSnapshot
	}
	seen := make(map[JudgmentKind]struct{}, len(declaredAsOf))
	for _, declared := range declaredAsOf {
		if !declared.kind.valid() || declared.at.IsZero() || !declared.policyVersion.valid() {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		if _, exists := seen[declared.kind]; exists {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		seen[declared.kind] = struct{}{}
	}
	return CommercialBasisSnapshot{
		resolutionID: resolutionID,
		rulePackage:  rulePackage,
		viewRevision: viewRevision,
		declaredAsOf: append([]DeclaredAsOf(nil), declaredAsOf...),
	}, nil
}

func (snapshot CommercialBasisSnapshot) ResolutionID() CommercialResolutionID {
	return snapshot.resolutionID
}

func (snapshot CommercialBasisSnapshot) RulePackage() RulePackageReference {
	return snapshot.rulePackage
}

func (snapshot CommercialBasisSnapshot) ViewRevision() CommercialViewRevision {
	return snapshot.viewRevision
}

// AsOfFor 返回规则包为某一类判断声明的时点。没有声明时报告缺席而不是给默认值——在这里
// 顶上任何一个时刻，正是用例禁止的「用一个全局时间代替」。
func (snapshot CommercialBasisSnapshot) AsOfFor(kind JudgmentKind) (JudgmentAsOf, bool) {
	for _, declared := range snapshot.declaredAsOf {
		if declared.kind == kind {
			return declared, true
		}
	}
	return DeclaredAsOf{}, false
}

func (snapshot CommercialBasisSnapshot) valid() bool {
	return snapshot.resolutionID.valid() && snapshot.rulePackage.valid() && snapshot.viewRevision.valid()
}

// ReachabilityValue 以采用引用的形式镜像 network-routing 的三值判断。parcel-shipment
// 从不产生它，只记录拥有它的上下文判断了什么；三个取值没有一个是接受决定。
type ReachabilityValue uint8

const (
	ReachabilityValueInvalid ReachabilityValue = iota
	ReachabilityReachable
	ReachabilityUnreachable
	ReachabilityInsufficientEvidence
)

func (value ReachabilityValue) valid() bool {
	return value >= ReachabilityReachable && value <= ReachabilityInsufficientEvidence
}

func (value ReachabilityValue) String() string {
	switch value {
	case ReachabilityReachable:
		return "REACHABLE"
	case ReachabilityUnreachable:
		return "UNREACHABLE"
	case ReachabilityInsufficientEvidence:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return ""
	}
}

type ReachabilityJudgment struct {
	judgmentID ReachabilityJudgmentID
	parcelID   DeclaredParcelID
	value      ReachabilityValue
	asOf       JudgmentAsOf
}

func NewReachabilityJudgment(
	judgmentID ReachabilityJudgmentID,
	parcelID DeclaredParcelID,
	value ReachabilityValue,
	asOf JudgmentAsOf,
) (ReachabilityJudgment, error) {
	if !judgmentID.valid() || !parcelID.valid() || !value.valid() ||
		asOf.at.IsZero() || !asOf.policyVersion.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	return ReachabilityJudgment{judgmentID: judgmentID, parcelID: parcelID, value: value, asOf: asOf}, nil
}

func (judgment ReachabilityJudgment) JudgmentID() ReachabilityJudgmentID {
	return judgment.judgmentID
}

func (judgment ReachabilityJudgment) DeclaredParcelID() DeclaredParcelID {
	return judgment.parcelID
}

func (judgment ReachabilityJudgment) Value() ReachabilityValue {
	return judgment.value
}

func (judgment ReachabilityJudgment) AsOf() JudgmentAsOf {
	return judgment.asOf
}

func (judgment ReachabilityJudgment) valid() bool {
	return judgment.judgmentID.valid() && judgment.parcelID.valid() && judgment.value.valid()
}
