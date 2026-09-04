package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidIntakeSource     = errors.New("parcel shipment: invalid intake source")
	ErrInvalidNetworkIntake    = errors.New("parcel shipment: invalid effective network intake")
	ErrInvalidFormalCommitment = errors.New("parcel shipment: invalid formal commitment")
)

// IntakeSourceKind 是合格物理来源的封闭二值：节点收寄（node-operations）或场外揽收
// （transport-fulfillment）。刻意没有第三格——普通扫描、卸载、车辆到场、任务创建与面单
// 结果不是来源（`AT-PS-043`），封闭集合让「升级」在类型上就不可能。
type IntakeSourceKind uint8

const (
	IntakeSourceKindInvalid IntakeSourceKind = iota
	NodeIntakeSource
	OffsitePickupSource
)

func (kind IntakeSourceKind) valid() bool {
	return kind == NodeIntakeSource || kind == OffsitePickupSource
}

func (kind IntakeSourceKind) String() string {
	switch kind {
	case NodeIntakeSource:
		return "NODE_INTAKE"
	case OffsitePickupSource:
		return "OFFSITE_PICKUP"
	default:
		return ""
	}
}

// SourceObjectReference 指名来源侧的作业实物或载运对象。物理事实属 node-operations 与
// transport-fulfillment，这里只引用不复制。
type SourceObjectReference struct{ requiredValue }

func NewSourceObjectReference(value string) (SourceObjectReference, error) {
	required, err := newRequiredValue("source object reference", value)
	return SourceObjectReference{required}, err
}

// IntakePlaceReference 指名实际收寄地点（节点或场外接货位置）。
type IntakePlaceReference struct{ requiredValue }

func NewIntakePlaceReference(value string) (IntakePlaceReference, error) {
	required, err := newRequiredValue("intake place reference", value)
	return IntakePlaceReference{required}, err
}

// IntakeControlReference 指名来源取得控制的依据（节点控制或运输控制）。与财务语汇的
// ControlBasisReference 分开：两个「控制」说的不是一件事。
type IntakeControlReference struct{ requiredValue }

func NewIntakeControlReference(value string) (IntakeControlReference, error) {
	required, err := newRequiredValue("intake control reference", value)
	return IntakeControlReference{required}, err
}

// SourceResultVersion 指名来源结果的版本。「同一包裹、同一来源类型和同一来源结果版本
// 只能形成一个有效网络收寄采用结果」——幂等的比对锚就是它；更正与撤销形成新版本。
type SourceResultVersion struct{ requiredValue }

func NewSourceResultVersion(value string) (SourceResultVersion, error) {
	required, err := newRequiredValue("source result version", value)
	return SourceResultVersion{required}, err
}

// IntakeSourceSpec 是一份合格物理来源引用所需的全部输入。
type IntakeSourceSpec struct {
	Kind       IntakeSourceKind
	Object     SourceObjectReference
	Parcel     DeclaredParcelID
	Place      IntakePlaceReference
	Control    IntakeControlReference
	Version    SourceResultVersion
	OccurredAt time.Time
	// Corrects 是来源自报的「本版本更正哪一版」（ADR-0117 决定一）。它只从来源所有者的
	// 事实带出（揽收那一路是 OffsitePickup.Corrects()），消费方适配器只翻译不推断；缺席即
	// 首登。同来源更正与另一来源竞争在采用口分格，靠的就是这一格，不靠键相同。
	Corrects SourceResultVersion
}

// IntakeSource 是对一份节点收寄或场外揽收结果的只读引用（强类型来源联合）。对象、地点、
// 控制依据、版本与实际发生时间缺一即立不起来——启动条件把五样列为来源的最低语义。
type IntakeSource struct {
	kind       IntakeSourceKind
	object     SourceObjectReference
	parcel     DeclaredParcelID
	place      IntakePlaceReference
	control    IntakeControlReference
	version    SourceResultVersion
	occurredAt time.Time
	corrects   SourceResultVersion
}

func NewIntakeSource(spec IntakeSourceSpec) (IntakeSource, error) {
	if !spec.Kind.valid() ||
		!spec.Object.valid() ||
		!spec.Parcel.valid() ||
		!spec.Place.valid() ||
		!spec.Control.valid() ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() {
		return IntakeSource{}, ErrInvalidIntakeSource
	}
	// 自指的更正没有前版可接，也分不出它是首登还是更正。
	if spec.Corrects.valid() && spec.Corrects == spec.Version {
		return IntakeSource{}, ErrInvalidIntakeSource
	}
	return IntakeSource{
		kind:       spec.Kind,
		object:     spec.Object,
		parcel:     spec.Parcel,
		place:      spec.Place,
		control:    spec.Control,
		version:    spec.Version,
		occurredAt: spec.OccurredAt.UTC(),
		corrects:   spec.Corrects,
	}, nil
}

func (source IntakeSource) Kind() IntakeSourceKind {
	return source.kind
}

func (source IntakeSource) Object() SourceObjectReference {
	return source.object
}

func (source IntakeSource) Parcel() DeclaredParcelID {
	return source.parcel
}

func (source IntakeSource) Place() IntakePlaceReference {
	return source.place
}

func (source IntakeSource) Control() IntakeControlReference {
	return source.control
}

func (source IntakeSource) Version() SourceResultVersion {
	return source.version
}

// OccurredAt 是物理收寄的实际发生时间——责任起点与正式承诺的生效时间都从它来，处理
// 时间与消息到达时间不能替代（UC-PS-003 硬句）。
func (source IntakeSource) OccurredAt() time.Time {
	return source.occurredAt
}

// Corrects 只在来源自报为更正版本时给出：它更正的那一版。首登来源答 false。
func (source IntakeSource) Corrects() (SourceResultVersion, bool) {
	return source.corrects, source.corrects.valid()
}

// EffectiveNetworkIntake 是 parcel-shipment 把合格物理来源统一解释成的网络服务有效收寄
// 采用结果：来源引用 + 接受基线锚 + 责任起点。它不复制物理事实，也不是终局结果。
type EffectiveNetworkIntake struct {
	source   IntakeSource
	baseline SubmissionVersionID
}

// AdoptNetworkIntake 把一份来源采用为有效网络收寄。基线锚必备：采用结果属于「接受基线
// 中的这个包裹」，没有基线的采用挂不回任何一次接受。
func AdoptNetworkIntake(source IntakeSource, baseline SubmissionVersionID) (EffectiveNetworkIntake, error) {
	if !source.kind.valid() || !baseline.valid() {
		return EffectiveNetworkIntake{}, ErrInvalidNetworkIntake
	}
	return EffectiveNetworkIntake{source: source, baseline: baseline}, nil
}

func (intake EffectiveNetworkIntake) Source() IntakeSource {
	return intake.source
}

func (intake EffectiveNetworkIntake) Baseline() SubmissionVersionID {
	return intake.baseline
}

// ResponsibilityStart 是责任起点：被采用来源的实际发生时间。它是派生读口而不是独立
// 字段——两个字段就可能各说各话。
func (intake EffectiveNetworkIntake) ResponsibilityStart() time.Time {
	return intake.source.occurredAt
}

// CommitmentVersionID 是正式承诺的版本标识。承诺调整形成新版本，不覆盖原承诺。
type CommitmentVersionID struct{ requiredValue }

func NewCommitmentVersionID(value string) (CommitmentVersionID, error) {
	required, err := newRequiredValue("commitment version ID", value)
	return CommitmentVersionID{required}, err
}

// ExpectedCommitmentReference 指名接受时形成的预计承诺。正式承诺必须引用它、不能覆盖
// 它——预计承诺、正式承诺、路由计划、ETA 与实际结果分别存在（ADR-0004 的分层）。
type ExpectedCommitmentReference struct{ requiredValue }

func NewExpectedCommitmentReference(value string) (ExpectedCommitmentReference, error) {
	required, err := newRequiredValue("expected commitment reference", value)
	return ExpectedCommitmentReference{required}, err
}

// CommitmentAdjustmentReason 指名一次承诺调整的原因。没有原因的调整与静默改写分不开。
type CommitmentAdjustmentReason struct{ requiredValue }

func NewCommitmentAdjustmentReason(value string) (CommitmentAdjustmentReason, error) {
	required, err := newRequiredValue("commitment adjustment reason", value)
	return CommitmentAdjustmentReason{required}, err
}

// FormalCommitment 是包裹进入网络服务责任范围时对客户成立的承诺版本。生效时间在构造内
// 取自被采用来源的实际发生时间——不收时间参数，处理时间与消息时间在类型上就进不来。
type FormalCommitment struct {
	version      CommitmentVersionID
	parcel       DeclaredParcelID
	intake       EffectiveNetworkIntake
	expected     ExpectedCommitmentReference
	effectiveAt  time.Time
	priorVersion CommitmentVersionID
	reason       CommitmentAdjustmentReason
}

// FormFormalCommitment 在有效网络收寄之上形成首个承诺版本。预计承诺引用必备——正式
// 承诺必须引用而不覆盖预计承诺。
func FormFormalCommitment(
	version CommitmentVersionID,
	intake EffectiveNetworkIntake,
	expected ExpectedCommitmentReference,
) (FormalCommitment, error) {
	if !version.valid() || !intake.source.kind.valid() || !expected.valid() {
		return FormalCommitment{}, ErrInvalidFormalCommitment
	}
	return FormalCommitment{
		version:     version,
		parcel:      intake.source.parcel,
		intake:      intake,
		expected:    expected,
		effectiveAt: intake.ResponsibilityStart(),
	}, nil
}

func (commitment FormalCommitment) Version() CommitmentVersionID {
	return commitment.version
}

func (commitment FormalCommitment) Parcel() DeclaredParcelID {
	return commitment.parcel
}

func (commitment FormalCommitment) Intake() EffectiveNetworkIntake {
	return commitment.intake
}

func (commitment FormalCommitment) Expected() ExpectedCommitmentReference {
	return commitment.expected
}

// EffectiveAt 恒等于被采用来源的实际发生时间。
func (commitment FormalCommitment) EffectiveAt() time.Time {
	return commitment.effectiveAt
}

// PriorVersion 只在调整版本上给出，指回被调整的那一版。
func (commitment FormalCommitment) PriorVersion() (CommitmentVersionID, bool) {
	return commitment.priorVersion, commitment.priorVersion.valid()
}

// AdjustmentReason 只在调整版本上给出。
func (commitment FormalCommitment) AdjustmentReason() (CommitmentAdjustmentReason, bool) {
	return commitment.reason, commitment.reason.valid()
}

// Adjust 形成带原因的新承诺版本（`AT-PS-048`/`AT-PS-050`）：路由、ETA 或现实履约变化
// 不能静默修改正式承诺——调整必须换版本号、带原因、指回前版；原承诺不可变地保留在
// 前一版上。
func (commitment FormalCommitment) Adjust(
	version CommitmentVersionID,
	reason CommitmentAdjustmentReason,
) (FormalCommitment, error) {
	if !version.valid() || !reason.valid() || version == commitment.version {
		return FormalCommitment{}, ErrInvalidFormalCommitment
	}
	adjusted := commitment
	adjusted.version = version
	adjusted.priorVersion = commitment.version
	adjusted.reason = reason
	return adjusted, nil
}

// RestateOnCorrectedIntake 以更正后的有效网络收寄重述承诺（`AT-PS-050`，ADR-0117 决定三）：
// 与 Adjust 同样换版本号、带原因、指回前版，不同的是收寄换成更正后的那一份，于是生效时间
// 随更正后的发生时刻走——Adjust 写给的是不改收寄的调整（路由、ETA、现实履约），来源更正
// 改的恰是收寄本身，用 Adjust 会把旧的发生时刻带进新版本。
//
// 更正后的收寄必须仍是这份承诺的收寄：同包裹、同接受基线、同来源种类。换了任何一样都是
// 另一份采用，不是这份的更正——它得走自己的采用判断，不能借前版的承诺号接续。
func (commitment FormalCommitment) RestateOnCorrectedIntake(
	version CommitmentVersionID,
	intake EffectiveNetworkIntake,
	reason CommitmentAdjustmentReason,
) (FormalCommitment, error) {
	if !version.valid() || !reason.valid() || version == commitment.version ||
		!intake.source.kind.valid() ||
		intake.source.parcel != commitment.parcel ||
		intake.baseline != commitment.intake.baseline ||
		intake.source.kind != commitment.intake.source.kind {
		return FormalCommitment{}, ErrInvalidFormalCommitment
	}
	return FormalCommitment{
		version:      version,
		parcel:       commitment.parcel,
		intake:       intake,
		expected:     commitment.expected,
		effectiveAt:  intake.ResponsibilityStart(),
		priorVersion: commitment.version,
		reason:       reason,
	}, nil
}

// QualificationRuleReference 是消费方对收寄硬资格引用的转写（ADR-0063）。它对应
// party-commercial 声明里的开放 RuleReference，但不把提供方类型带进本上下文端口。
// 正文、阈值与取值仍在执行该规则的权威方；本类型只携带引用字符串。
type QualificationRuleReference struct{ requiredValue }

func NewQualificationRuleReference(value string) (QualificationRuleReference, error) {
	required, err := newRequiredValue("qualification rule reference", value)
	return QualificationRuleReference{required}, err
}

// QualificationRulePrefix 取引用的权威前缀（第一个 `/` 之前）。没有 `/` 时整串即前缀。
// 未知前缀不得被证明为已成立——前缀只用于找权威方，不得把后缀拆成关务案件或申报单元
// 身份（ADR-0063）。
func QualificationRulePrefix(ref QualificationRuleReference) string {
	value := ref.String()
	if i := strings.IndexByte(value, '/'); i > 0 {
		return value[:i]
	}
	return value
}
