package domain

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// 本文件是价卡草稿「已校验 → 已批准 → 已发布」两步推进（ADR-0101 决定五、六；票 price-card-import/04）。批准与录入是
// 两个操作者动作：两身份都从操作者信封来、都记在草稿上且可比。能不能批由租户的审批职责规则说，规则本身是实例半边
// （参数登记册「价卡发布审批职责规则」），这里只立规则的形与批准门。形状同 party-commercial 的审批职责规则，但不共用
// 那一条——租户对价卡与对商业版本的审批要求可以不同（spec 自决第 2 格）。
//
// 发布本身不在这里发生：草稿交出登记，由既有价卡登记用例落册；这里只在落定之后记下发布时刻。

var (
	// ErrPriceCardDraftNotValidated 表示草稿不在`已校验`：`草稿`没有可批的内容，`已批准`与`已发布`已过了这一格。
	ErrPriceCardDraftNotValidated = errors.New("parcel pricing: price card draft is not validated")
	// ErrPriceCardDraftNotApproved 表示草稿还没批准（或发布时刻已记）：登记与发布只跟在批准之后。
	ErrPriceCardDraftNotApproved = errors.New("parcel pricing: price card draft is not approved")
	// ErrDraftApproverIsSubmitter 是规则「录入者与批准者须为不同主体」那一格拒下的形状。恢复动作是换一个人来批，
	// 不是改草稿——与授予不足分格。
	ErrDraftApproverIsSubmitter = errors.New("parcel pricing: the approver is the submitter of this draft")
	// ErrDraftApproverLacksRequiredGrant 是规则「批准者须持某一格授予」那一格拒下的形状：批准者的授予集里没有它。
	// 恢复动作是换人，或租户改规则——都不是改草稿。
	ErrDraftApproverLacksRequiredGrant = errors.New("parcel pricing: the approver does not hold the grant the approval duty rule requires")
	// ErrPriceCardApprovalDutyRuleTenantMismatch 拦的是拿他租户的规则裁本租户的草稿——租户是身份不是过滤器，这只可能
	// 是装配或调用方的错，不是业务答案。
	ErrPriceCardApprovalDutyRuleTenantMismatch = errors.New("parcel pricing: the approval duty rule belongs to another tenant")
	ErrInvalidPriceCardApprovalDutyRule        = errors.New("parcel pricing: invalid price card approval duty rule")
	ErrInvalidOperatorSubject                  = errors.New("parcel pricing: invalid operator subject")
	ErrInvalidOperatorGrant                    = errors.New("parcel pricing: invalid operator grant")
)

// OperatorGrant 是操作者持有的一格授予：信封里认证出的生效授予，在本上下文里只以名字出现、只比相等。哪些名字可授
// 归接入身份能力的授权模型，本上下文不枚举、不解释。
type OperatorGrant struct{ name string }

func NewOperatorGrant(name string) (OperatorGrant, error) {
	if !trimmed(name) {
		return OperatorGrant{}, ErrInvalidOperatorGrant
	}
	return OperatorGrant{name: name}, nil
}

func (grant OperatorGrant) String() string { return grant.name }

func (grant OperatorGrant) valid() bool { return trimmed(grant.name) }

// OperatorSubject 是本上下文消费操作者信封的那一个面（票 04 第 2 条）：主体引用加它持有的授予集，由 Intake 把信封译成
// 它。主体引用与草稿上的录入者同一种写法，批准门才比得了「是不是同一个人」。授予集可以为空——没带授予不是错，只是
// 批不了要求授予的草稿。
type OperatorSubject struct {
	reference string
	grants    []OperatorGrant
}

func NewOperatorSubject(reference string, grants []OperatorGrant) (OperatorSubject, error) {
	if !trimmed(reference) {
		return OperatorSubject{}, ErrInvalidOperatorSubject
	}
	for _, grant := range grants {
		if !grant.valid() {
			return OperatorSubject{}, fmt.Errorf("%w: blank grant in the grant set", ErrInvalidOperatorSubject)
		}
	}
	return OperatorSubject{reference: reference, grants: slices.Clone(grants)}, nil
}

func (subject OperatorSubject) Reference() string { return subject.reference }

// Grants 交回授予集（副本）。
func (subject OperatorSubject) Grants() []OperatorGrant { return slices.Clone(subject.grants) }

func (subject OperatorSubject) Holds(grant OperatorGrant) bool {
	return slices.Contains(subject.grants, grant)
}

func (subject OperatorSubject) valid() bool { return trimmed(subject.reference) }

// PriceCardApprovalDutyRule 是租户对价卡发布的审批职责规则：录入者与批准者须否为不同主体、批准者须持哪一格授予。
// 两格都不要求也是一条合法的租户声明——「单人可批」由租户说出，不由系统默认；规则未登记时批准门答`未配置`、不放行
// （ADR-0101 决定六：不写死双人，也不写死单人）。
type PriceCardApprovalDutyRule struct {
	tenant           TenantID
	distinctSubjects bool
	requiredGrant    OperatorGrant
}

// NewPriceCardApprovalDutyRule 立一条规则。requiredGrant 零值即不要求授予。
func NewPriceCardApprovalDutyRule(tenant TenantID, distinctSubjects bool, requiredGrant OperatorGrant) (PriceCardApprovalDutyRule, error) {
	if !tenant.valid() || (requiredGrant != OperatorGrant{} && !requiredGrant.valid()) {
		return PriceCardApprovalDutyRule{}, ErrInvalidPriceCardApprovalDutyRule
	}
	return PriceCardApprovalDutyRule{tenant: tenant, distinctSubjects: distinctSubjects, requiredGrant: requiredGrant}, nil
}

func (rule PriceCardApprovalDutyRule) Tenant() TenantID { return rule.tenant }

func (rule PriceCardApprovalDutyRule) RequiresDistinctSubjects() bool { return rule.distinctSubjects }

// RequiredGrant 交回规则要求批准者持有的授予；第二个返回值为假即不要求。
func (rule PriceCardApprovalDutyRule) RequiredGrant() (OperatorGrant, bool) {
	return rule.requiredGrant, rule.requiredGrant.valid()
}

func (rule PriceCardApprovalDutyRule) valid() bool { return rule.tenant.valid() }

// Approve 由另一个操作者动作把`已校验`推进到`已批准`，记下批准者与批准时刻。规则逐格裁：要求不同主体时批准者不得是
// 录入者；要求持某一格授予时批准者的授予集里必须有它。规则缺席不在这里答——那是`未配置`，由读规则的一方如实交回；
// 这里收到的必须是一条成立的、本租户的规则。
func (draft PriceCardDraft) Approve(approver OperatorSubject, rule PriceCardApprovalDutyRule, approvedAt time.Time) (PriceCardDraft, error) {
	if draft.status != PriceCardDraftStatusValidated {
		return PriceCardDraft{}, fmt.Errorf("%w: draft is %s", ErrPriceCardDraftNotValidated, draft.status)
	}
	if !approver.valid() {
		return PriceCardDraft{}, ErrInvalidOperatorSubject
	}
	if !rule.valid() {
		return PriceCardDraft{}, ErrInvalidPriceCardApprovalDutyRule
	}
	if rule.tenant != draft.tenant {
		return PriceCardDraft{}, ErrPriceCardApprovalDutyRuleTenantMismatch
	}
	if approvedAt.IsZero() || approvedAt.Before(draft.submittedAt) {
		return PriceCardDraft{}, fmt.Errorf("%w：批准时刻缺或早于录入", ErrInvalidPriceCardDraft)
	}
	if rule.distinctSubjects && approver.reference == draft.submitter {
		return PriceCardDraft{}, ErrDraftApproverIsSubmitter
	}
	if grant, required := rule.RequiredGrant(); required && !approver.Holds(grant) {
		return PriceCardDraft{}, ErrDraftApproverLacksRequiredGrant
	}
	draft.status = PriceCardDraftStatusApproved
	draft.approver = approver.reference
	draft.approvedAt = approvedAt.UTC()
	return draft, nil
}

// Registration 交回发布要交给既有价卡登记的那一份（ADR-0101 决定五）：方案、源文件身份与方向授权引用即草稿上记下的
// 那几样，发布批准责任方即批准者主体。方案是同一个值、不重解析不重算（决定四），规范化版本与内容摘要随它原样过去，
// 登记册记下的摘要因此与草稿上的逐字节相等。只有`已批准`及其后的草稿有登记可交。
func (draft PriceCardDraft) Registration() (PriceCardRegistration, error) {
	if draft.status != PriceCardDraftStatusApproved && draft.status != PriceCardDraftStatusPublished {
		return PriceCardRegistration{}, fmt.Errorf("%w: draft is %s", ErrPriceCardDraftNotApproved, draft.status)
	}
	return NewPriceCardRegistration(draft.tenant, draft.content.Plan, draft.source,
		draft.content.DirectionAuthorization, draft.approver)
}

// MarkPublished 在登记落定之后把`已批准`推进到`已发布`，且发布时刻不得早于批准。
func (draft PriceCardDraft) MarkPublished(publishedAt time.Time) (PriceCardDraft, error) {
	if draft.status != PriceCardDraftStatusApproved {
		return PriceCardDraft{}, fmt.Errorf("%w: draft is %s", ErrPriceCardDraftNotApproved, draft.status)
	}
	if publishedAt.IsZero() || publishedAt.Before(draft.approvedAt) {
		return PriceCardDraft{}, fmt.Errorf("%w：发布时刻缺或早于批准", ErrInvalidPriceCardDraft)
	}
	draft.status = PriceCardDraftStatusPublished
	draft.publishedAt = publishedAt.UTC()
	return draft, nil
}
