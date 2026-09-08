package domain

import (
	"errors"
	"fmt"
	"time"
)

// 本文件是待批准发布（ADR-0126 Decision 三）：运营操作者面录入的一份商业版本在发布之前的载体，
// 一版一行，状态三格封闭。它不是商业版本——任何解析读不到它；发布把已批准的载体交给既有的受控
// 发布用例，形成的商业版本照旧不可覆盖（CONTEXT「待批准发布」词条）。
//
// 批准与录入是两个操作者动作：两身份都从操作者信封来、都记且可比。能不能批由租户的审批职责规则
// 说（PAR-COM-18），规则本身是实例半边；这里只立规则的形与批准门。

var (
	ErrInvalidPublicationDraft = errors.New("party commercial: invalid publication draft")
	// ErrPublicationDraftNotPending 表示载体已不在`待批准`：批准只对待批准的载体有意义。
	ErrPublicationDraftNotPending = errors.New("party commercial: publication draft is not pending approval")
	// ErrPublicationDraftNotApproved 表示载体还没有批准（或已经发布）：发布只接`已批准`的载体。
	ErrPublicationDraftNotApproved = errors.New("party commercial: publication draft is not approved")
	// ErrApproverIsSubmitter 是审批职责规则「录入者与批准者须为不同主体」那一格拒下的形状。恢复动作是换
	// 一个人来批，不是改载体——与其余拒绝分格（判据同 parcel-pricing 的四眼门）。
	ErrApproverIsSubmitter = errors.New("party commercial: the approver is the submitter of this draft")
	// ErrApproverLacksRequiredLevel 是「批准者须持某一格授予」那一格拒下的形状：批准者的授予集里没有规则
	// 要求的那一级。恢复动作同样是换人，或让租户改规则——都不是改载体。
	ErrApproverLacksRequiredLevel = errors.New("party commercial: the approver does not hold the authority level the approval duty rule requires")
	// ErrApprovalDutyRuleTenantMismatch 拦的是拿他租户的规则裁本租户的载体——租户是身份不是过滤器
	// （ADR-0003），这只可能是装配或调用方的错，不是业务答案。
	ErrApprovalDutyRuleTenantMismatch = errors.New("party commercial: the approval duty rule belongs to another tenant")
	ErrInvalidApprovalDutyRule        = errors.New("party commercial: invalid approval duty rule")
	ErrInvalidOperatorSubject         = errors.New("party commercial: invalid operator subject")
	// ErrInvalidRehydratedPublicationDraft 是载体重建门因行数据本身而拒绝时给出的理由：这一行不可能是本上下文
	// 录入或推进出来的，处置是去查库里那一行或写它的适配器（分格纪律同 ErrInvalidRehydratedPublication）。
	ErrInvalidRehydratedPublicationDraft = errors.New("party commercial: invalid rehydrated publication draft")
)

// OperatorSubjectReference 是操作者主体的引用——ADR-0100 信封里认证出来的那个操作者，在本上下文里
// 只以引用出现。它不是参与方身份也不是商业权限等级：登录的是谁与谁有权批是两件事。
type OperatorSubjectReference struct{ requiredValue }

func NewOperatorSubjectReference(value string) (OperatorSubjectReference, error) {
	required, err := newRequiredValue("operator subject reference", value)
	return OperatorSubjectReference{required}, err
}

// OperatorSubject 是本上下文消费操作者信封的那一个面（ADR-0126 Decision 五）：主体引用加它持有的
// 商业权限等级（授予集）。授予集可以为空——信封里没带等级不是错，只是批不了要求等级的载体。
type OperatorSubject struct {
	reference OperatorSubjectReference
	levels    []AuthorityLevel
}

func NewOperatorSubject(reference OperatorSubjectReference, levels []AuthorityLevel) (OperatorSubject, error) {
	if !reference.valid() {
		return OperatorSubject{}, ErrInvalidOperatorSubject
	}
	held := make([]AuthorityLevel, 0, len(levels))
	for _, level := range levels {
		if !level.valid() {
			return OperatorSubject{}, fmt.Errorf("%w: blank authority level in the grant set", ErrInvalidOperatorSubject)
		}
		held = append(held, level)
	}
	return OperatorSubject{reference: reference, levels: held}, nil
}

func (subject OperatorSubject) Reference() OperatorSubjectReference {
	return subject.reference
}

// Levels 交回授予集（副本）。
func (subject OperatorSubject) Levels() []AuthorityLevel {
	return append([]AuthorityLevel(nil), subject.levels...)
}

// Holds 答主体持不持某一格授予。
func (subject OperatorSubject) Holds(level AuthorityLevel) bool {
	for _, held := range subject.levels {
		if held == level {
			return true
		}
	}
	return false
}

func (subject OperatorSubject) valid() bool {
	return subject.reference.valid()
}

// ApprovalDutyRule 是租户的审批职责规则（PAR-COM-18）：录入者与批准者须否为不同主体、批准者须持哪一格
// 授予。两格都不要求也是一条合法的租户声明——「单人可批」由租户说出，不由系统默认（ADR-0101 决定六、
// ADR-0126 Decision 三：不写死双人也不写死单人）。
type ApprovalDutyRule struct {
	tenant           TenantID
	distinctSubjects bool
	approverLevel    AuthorityLevel
}

// NewApprovalDutyRule 建立一条规则。approverLevel 零值 = 不要求等级。
func NewApprovalDutyRule(tenant TenantID, distinctSubjects bool, approverLevel AuthorityLevel) (ApprovalDutyRule, error) {
	if !tenant.valid() {
		return ApprovalDutyRule{}, ErrInvalidApprovalDutyRule
	}
	return ApprovalDutyRule{tenant: tenant, distinctSubjects: distinctSubjects, approverLevel: approverLevel}, nil
}

func (rule ApprovalDutyRule) Tenant() TenantID {
	return rule.tenant
}

func (rule ApprovalDutyRule) RequiresDistinctSubjects() bool {
	return rule.distinctSubjects
}

// RequiredApproverLevel 交回规则要求批准者持有的等级；第二个返回值为假即不要求。
func (rule ApprovalDutyRule) RequiredApproverLevel() (AuthorityLevel, bool) {
	return rule.approverLevel, rule.approverLevel.valid()
}

func (rule ApprovalDutyRule) valid() bool {
	return rule.tenant.valid()
}

// PublicationDraftStatus 是载体的状态三格。批准是发布的完备性条件、不是版本的状态（CommercialVersionStatus
// 没有 APPROVED），所以这三格挂在载体上而不是版本上。
type PublicationDraftStatus uint8

const (
	PublicationDraftStatusInvalid PublicationDraftStatus = iota
	PublicationDraftPendingApproval
	PublicationDraftApproved
	PublicationDraftPublished
)

func (status PublicationDraftStatus) String() string {
	switch status {
	case PublicationDraftPendingApproval:
		return "PENDING_APPROVAL"
	case PublicationDraftApproved:
		return "APPROVED"
	case PublicationDraftPublished:
		return "PUBLISHED"
	default:
		return ""
	}
}

func (status PublicationDraftStatus) valid() bool {
	return status >= PublicationDraftPendingApproval && status <= PublicationDraftPublished
}

// PublicationDraftShell 是载体的版本壳：CommercialVersionSpec 去掉内容摘要那一格。摘要不在壳上，是因为
// 表单不算摘要——壳上没有它的位置，自报摘要在结构上就写不进来（伞票硬句）。
type PublicationDraftShell struct {
	TenantID   TenantID
	Kind       CommercialObjectKind
	ObjectID   CommercialObjectID
	Version    CommercialVersionLabel
	Scope      CommercialScopeReference
	Effective  EffectiveInterval
	References map[CommercialObjectKind]CommercialObjectID
}

// PublicationDraft 是一份待批准发布。值类型：每次推进返回新值、不改接收者，状态只能向前。
type PublicationDraft struct {
	shell       CommercialVersion
	content     PublicationContent
	canonical   CanonicalPublicationContent
	submitter   OperatorSubjectReference
	submittedAt time.Time
	status      PublicationDraftStatus
	approver    OperatorSubjectReference
	approvedAt  time.Time
	publishedAt time.Time
}

// SubmitPublicationDraft 由录入形成一份`待批准`载体。摘要在这里算——调用方交的是正文不是摘要；壳与正文的
// 类别必须一致；壳本身经 NewCommercialDraft 同一道门（身份四元、范围、区间、指名引用），载体不另立第二套
// 壳的判据。
func SubmitPublicationDraft(
	shell PublicationDraftShell,
	content PublicationContent,
	submitter OperatorSubjectReference,
	submittedAt time.Time,
) (PublicationDraft, error) {
	if !submitter.valid() || submittedAt.IsZero() {
		return PublicationDraft{}, fmt.Errorf("%w: submitter and submission time are required", ErrInvalidPublicationDraft)
	}
	if content.Kind != shell.Kind {
		return PublicationDraft{}, fmt.Errorf("%w: content is %s while the shell declares %s",
			ErrInvalidPublicationDraft, content.Kind, shell.Kind)
	}
	canonical, err := CanonicalizePublicationContent(content)
	if err != nil {
		return PublicationDraft{}, err
	}
	version, err := NewCommercialDraft(CommercialVersionSpec{
		TenantID:      shell.TenantID,
		Kind:          shell.Kind,
		ObjectID:      shell.ObjectID,
		Version:       shell.Version,
		Scope:         shell.Scope,
		ContentDigest: canonical.digest,
		Effective:     shell.Effective,
		References:    shell.References,
	})
	if err != nil {
		return PublicationDraft{}, err
	}
	return PublicationDraft{
		shell:       version,
		content:     content,
		canonical:   canonical,
		submitter:   submitter,
		submittedAt: submittedAt.UTC(),
		status:      PublicationDraftPendingApproval,
	}, nil
}

// Approve 由另一个操作者动作把`待批准`推进到`已批准`。规则逐格裁：要求不同主体时批准者不得是录入者；要求
// 持某一格授予时批准者的授予集里必须有它。规则缺席不在这里答——那是`未配置`，由读规则的一方如实交回；
// 这里收到的必须是一条成立的规则，且是本租户的。
func (draft PublicationDraft) Approve(
	approver OperatorSubject,
	rule ApprovalDutyRule,
	approvedAt time.Time,
) (PublicationDraft, error) {
	if draft.status != PublicationDraftPendingApproval {
		return PublicationDraft{}, ErrPublicationDraftNotPending
	}
	if !approver.valid() {
		return PublicationDraft{}, ErrInvalidOperatorSubject
	}
	if !rule.valid() {
		return PublicationDraft{}, ErrInvalidApprovalDutyRule
	}
	if rule.tenant != draft.shell.tenant {
		return PublicationDraft{}, ErrApprovalDutyRuleTenantMismatch
	}
	if approvedAt.IsZero() || approvedAt.Before(draft.submittedAt) {
		return PublicationDraft{}, fmt.Errorf("%w: approval time is missing or precedes submission", ErrInvalidPublicationDraft)
	}
	if rule.distinctSubjects && approver.reference == draft.submitter {
		return PublicationDraft{}, ErrApproverIsSubmitter
	}
	if level, required := rule.RequiredApproverLevel(); required && !approver.Holds(level) {
		return PublicationDraft{}, ErrApproverLacksRequiredLevel
	}
	draft.status = PublicationDraftApproved
	draft.approver = approver.reference
	draft.approvedAt = approvedAt.UTC()
	return draft, nil
}

// MarkPublished 在受控发布落定之后把`已批准`推进到`已发布`。发布本身不在这里发生——那是 PublishCommercialAuthority
// 用例的事；这里只记结果，且不得早于批准。
func (draft PublicationDraft) MarkPublished(publishedAt time.Time) (PublicationDraft, error) {
	if draft.status != PublicationDraftApproved {
		return PublicationDraft{}, ErrPublicationDraftNotApproved
	}
	if publishedAt.IsZero() || publishedAt.Before(draft.approvedAt) {
		return PublicationDraft{}, fmt.Errorf("%w: publication time is missing or precedes approval", ErrInvalidPublicationDraft)
	}
	draft.status = PublicationDraftPublished
	draft.publishedAt = publishedAt.UTC()
	return draft, nil
}

// PublicationSpec 交回受控发布要收的版本规格：壳加算出的摘要。发布用例的对账门由此恒成立——声明的与算出的
// 是同一个串。
func (draft PublicationDraft) PublicationSpec() CommercialVersionSpec {
	references := make(map[CommercialObjectKind]CommercialObjectID, len(draft.shell.references))
	for _, reference := range draft.shell.references {
		references[reference.kind] = reference.objectID
	}
	if len(references) == 0 {
		references = nil
	}
	return CommercialVersionSpec{
		TenantID:      draft.shell.tenant,
		Kind:          draft.shell.kind,
		ObjectID:      draft.shell.objectID,
		Version:       draft.shell.version,
		Scope:         draft.shell.scope,
		ContentDigest: draft.canonical.digest,
		Effective:     draft.shell.effective,
		References:    references,
	}
}

// PublicationApproval 交回受控发布要收的批准依据（ADR-0126 Decision 三）：批准引用 = 批准者主体、来源 = 载体
// 自己的引用、时刻 = 批准时刻。三者都由载体上已记的事实决定，同一载体两次交出逐字节相同——发布重放因此
// 是`重复`而不是`冲突`。只有`已批准`及其后的载体有批准依据可交。
func (draft PublicationDraft) PublicationApproval() (ApprovalBasis, error) {
	if draft.status != PublicationDraftApproved && draft.status != PublicationDraftPublished {
		return ApprovalBasis{}, ErrPublicationDraftNotApproved
	}
	reference, err := NewApprovalReference(draft.approver.String())
	if err != nil {
		return ApprovalBasis{}, err
	}
	return NewApprovalBasis(reference, draft.Reference(), draft.approvedAt)
}

// Reference 是载体自己的引用，作发布的批准来源。由版本身份决定、不含时刻——同一版本的载体只有一份。
func (draft PublicationDraft) Reference() CommercialSourceReference {
	source, err := NewCommercialSourceReference(
		"publication-draft/" + draft.shell.kind.String() + "/" + draft.shell.objectID.String() + "/" + draft.shell.version.String())
	if err != nil {
		return CommercialSourceReference{}
	}
	return source
}

// SameContentAs 答两份载体的正文是不是同一份：只看算出的摘要。登记册据此分「重放」与「修订」。
func (draft PublicationDraft) SameContentAs(other PublicationDraft) bool {
	return draft.canonical.digest == other.canonical.digest
}

func (draft PublicationDraft) Tenant() TenantID                { return draft.shell.tenant }
func (draft PublicationDraft) Kind() CommercialObjectKind      { return draft.shell.kind }
func (draft PublicationDraft) ObjectID() CommercialObjectID    { return draft.shell.objectID }
func (draft PublicationDraft) Version() CommercialVersionLabel { return draft.shell.version }
func (draft PublicationDraft) Scope() CommercialScopeReference { return draft.shell.scope }
func (draft PublicationDraft) Effective() EffectiveInterval    { return draft.shell.effective }
func (draft PublicationDraft) Content() PublicationContent     { return draft.content }
func (draft PublicationDraft) Canonical() CanonicalPublicationContent {
	return draft.canonical
}
func (draft PublicationDraft) Submitter() OperatorSubjectReference { return draft.submitter }
func (draft PublicationDraft) SubmittedAt() time.Time              { return draft.submittedAt }
func (draft PublicationDraft) Status() PublicationDraftStatus      { return draft.status }

// DeclaredReferences 报出壳上的指名引用。
func (draft PublicationDraft) DeclaredReferences() []DeclaredReference {
	return draft.shell.DeclaredReferences()
}

func (draft PublicationDraft) Approver() (OperatorSubjectReference, bool) {
	return draft.approver, draft.approver.valid()
}

func (draft PublicationDraft) ApprovedAt() (time.Time, bool) {
	if draft.approvedAt.IsZero() {
		return time.Time{}, false
	}
	return draft.approvedAt, true
}

func (draft PublicationDraft) PublishedAt() (time.Time, bool) {
	if draft.publishedAt.IsZero() {
		return time.Time{}, false
	}
	return draft.publishedAt, true
}

// RehydratePublicationDraftSpec 是一份载体在库里的样子：壳、规范化版本与快照文档、列上的摘要、录入者与
// 三格状态各自的痕迹。字段一律当数据收下，只校不变量（ADR-0028）。
type RehydratePublicationDraftSpec struct {
	Shell            PublicationDraftShell
	Canonicalization string
	Document         []byte
	Digest           CommercialContentDigest
	Submitter        OperatorSubjectReference
	SubmittedAt      time.Time
	Status           PublicationDraftStatus
	Approver         OperatorSubjectReference
	ApprovedAt       time.Time
	PublishedAt      time.Time
}

// RehydratePublicationDraft 从库里读到的行重建一份载体。
//
// 快照折回正文再规范化一遍、与列上的摘要比：快照与摘要是一样东西的两面，对不上就是行坏了。这不是重算判断
// ——摘要不是判断，是正文的派生；本构建不认识的规范化版本折不回来，那一行如实拒（操作者重新录入即可，
// 载体不是权威记录）。三格状态各自带该带的痕迹：`待批准`没有批准与发布痕迹；`已批准`带批准者与时刻、没有
// 发布时刻；`已发布`三样都带且顺序向后。
func RehydratePublicationDraft(spec RehydratePublicationDraftSpec) (PublicationDraft, error) {
	if !spec.Submitter.valid() || spec.SubmittedAt.IsZero() {
		return PublicationDraft{}, rehydratedDraftRefusal("录入者或录入时刻缺失")
	}
	if !spec.Status.valid() {
		return PublicationDraft{}, rehydratedDraftRefusal(fmt.Sprintf("状态不是本上下文的取值：%d", uint8(spec.Status)))
	}
	content, err := RehydratePublicationContent(spec.Canonicalization, spec.Document)
	if err != nil {
		return PublicationDraft{}, rehydratedDraftRefusal(fmt.Sprintf("正文快照折不回来：%v", err))
	}
	if content.Kind != spec.Shell.Kind {
		return PublicationDraft{}, rehydratedDraftRefusal("快照里的册与壳声明的类别不是同一个")
	}
	canonical, err := CanonicalizePublicationContent(content)
	if err != nil {
		return PublicationDraft{}, rehydratedDraftRefusal(fmt.Sprintf("快照折回的正文规范化不了：%v", err))
	}
	if canonical.digest != spec.Digest {
		return PublicationDraft{}, rehydratedDraftRefusal("列上的摘要与快照算出的不一致")
	}
	version, err := NewCommercialDraft(CommercialVersionSpec{
		TenantID:      spec.Shell.TenantID,
		Kind:          spec.Shell.Kind,
		ObjectID:      spec.Shell.ObjectID,
		Version:       spec.Shell.Version,
		Scope:         spec.Shell.Scope,
		ContentDigest: canonical.digest,
		Effective:     spec.Shell.Effective,
		References:    spec.Shell.References,
	})
	if err != nil {
		return PublicationDraft{}, rehydratedDraftRefusal(fmt.Sprintf("壳立不起来：%v", err))
	}

	hasApproval := spec.Approver.valid() || !spec.ApprovedAt.IsZero()
	switch spec.Status {
	case PublicationDraftPendingApproval:
		if hasApproval || !spec.PublishedAt.IsZero() {
			return PublicationDraft{}, rehydratedDraftRefusal("待批准的载体带着批准或发布痕迹")
		}
	case PublicationDraftApproved:
		if !spec.Approver.valid() || spec.ApprovedAt.IsZero() || spec.ApprovedAt.Before(spec.SubmittedAt) {
			return PublicationDraft{}, rehydratedDraftRefusal("已批准的载体缺批准者或批准时刻，或批准早于录入")
		}
		if !spec.PublishedAt.IsZero() {
			return PublicationDraft{}, rehydratedDraftRefusal("已批准未发布的载体带着发布时刻")
		}
	case PublicationDraftPublished:
		if !spec.Approver.valid() || spec.ApprovedAt.IsZero() || spec.ApprovedAt.Before(spec.SubmittedAt) {
			return PublicationDraft{}, rehydratedDraftRefusal("已发布的载体缺批准者或批准时刻，或批准早于录入")
		}
		if spec.PublishedAt.IsZero() || spec.PublishedAt.Before(spec.ApprovedAt) {
			return PublicationDraft{}, rehydratedDraftRefusal("已发布的载体缺发布时刻，或发布早于批准")
		}
	}

	return PublicationDraft{
		shell:       version,
		content:     content,
		canonical:   canonical,
		submitter:   spec.Submitter,
		submittedAt: spec.SubmittedAt.UTC(),
		status:      spec.Status,
		approver:    spec.Approver,
		approvedAt:  spec.ApprovedAt.UTC(),
		publishedAt: spec.PublishedAt.UTC(),
	}, nil
}

func rehydratedDraftRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedPublicationDraft, reason)
}
