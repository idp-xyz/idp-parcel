package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidClaim          = errors.New("visibility exception: invalid customer claim")
	ErrClaimAlreadyScreened  = errors.New("visibility exception: the claim is already screened")
	ErrClaimNotScreened      = errors.New("visibility exception: the claim is not screened yet")
	ErrClaimAlreadyConcluded = errors.New("visibility exception: the claim is already concluded")
	ErrClaimWithdrawn        = errors.New("visibility exception: the claim is withdrawn")
	ErrReviewWindowClosed    = errors.New("visibility exception: the review window has closed")
)

// ClaimItemID 是客户索赔项的标识。
type ClaimItemID struct{ requiredValue }

func NewClaimItemID(value string) (ClaimItemID, error) {
	required, err := newRequiredValue("claim item ID", value)
	return ClaimItemID{required}, err
}

// ClaimBatchReference 指名索赔提交批次。批次允许部分成功——逐项独立受理、审核和
// 形成结论，这个独立性由「每项是独立对象」承担，批次只是归组。
type ClaimBatchReference struct{ requiredValue }

func NewClaimBatchReference(value string) (ClaimBatchReference, error) {
	required, err := newRequiredValue("claim batch reference", value)
	return ClaimBatchReference{required}, err
}

// ClaimKindReference 指名索赔类型。
type ClaimKindReference struct{ requiredValue }

func NewClaimKindReference(value string) (ClaimKindReference, error) {
	required, err := newRequiredValue("claim kind reference", value)
	return ClaimKindReference{required}, err
}

// ContractScopeReference 指名合同责任范围。
type ContractScopeReference struct{ requiredValue }

func NewContractScopeReference(value string) (ContractScopeReference, error) {
	required, err := newRequiredValue("contract scope reference", value)
	return ContractScopeReference{required}, err
}

// EligibilityScreen 是资格审核的封闭二值答复（通过/不通过带依据）。
type EligibilityScreen uint8

const (
	EligibilityScreenInvalid EligibilityScreen = iota
	ClaimEligible
	ClaimIneligible
)

func (screen EligibilityScreen) valid() bool {
	return screen == ClaimEligible || screen == ClaimIneligible
}

// LiabilityConclusion 是责任审核的封闭四值（CONTEXT 生命周期 252：「全部成立、部分
// 成立、不成立或当前无法认定」）。金额结算独立处理——这里没有金额。
type LiabilityConclusion uint8

const (
	LiabilityConclusionInvalid LiabilityConclusion = iota
	LiabilityFullyEstablished
	LiabilityPartiallyEstablished
	LiabilityNotEstablished
	LiabilityUndeterminable
)

func (conclusion LiabilityConclusion) valid() bool {
	return conclusion >= LiabilityFullyEstablished && conclusion <= LiabilityUndeterminable
}

func (conclusion LiabilityConclusion) String() string {
	switch conclusion {
	case LiabilityFullyEstablished:
		return "FULLY_ESTABLISHED"
	case LiabilityPartiallyEstablished:
		return "PARTIALLY_ESTABLISHED"
	case LiabilityNotEstablished:
		return "NOT_ESTABLISHED"
	case LiabilityUndeterminable:
		return "UNDETERMINABLE"
	default:
		return ""
	}
}

// ClaimItemSpec 是受理一项索赔所需的全部输入：一个货主客户账户、合同责任范围、目标
// 范围与索赔类型逐项固定（CONTEXT 硬句 172）。
type ClaimItemSpec struct {
	ID          ClaimItemID
	Batch       ClaimBatchReference
	Customer    CustomerAccountReference
	Contract    ContractScopeReference
	Target      RequestScopeReference
	Kind        ClaimKindReference
	SubmittedAt time.Time
}

// ClaimItem 是一项客户索赔。收到、通过资格审核和确认赔偿责任是三个不同判断（CONTEXT
// 硬句 173）——受理只保留原始提交事实，资格与责任各是显式一步；类型上没有赔付金额
// 字段，金额结算独立处理。
type ClaimItem struct {
	id              ClaimItemID
	batch           ClaimBatchReference
	customer        CustomerAccountReference
	contract        ContractScopeReference
	target          RequestScopeReference
	kind            ClaimKindReference
	submittedAt     time.Time
	screen          EligibilityScreen
	screenBasis     string
	conclusion      LiabilityConclusion
	concludedAt     time.Time
	reviewBy        time.Time
	priorConclusion LiabilityConclusion
	withdrawn       bool
	withdrawnAt     time.Time
}

// ReceiveClaimItem 受理一项索赔：先保留原始提交事实，资格判断是下一步（不在这里）。
func ReceiveClaimItem(spec ClaimItemSpec) (*ClaimItem, error) {
	if !spec.ID.valid() ||
		!spec.Batch.valid() ||
		!spec.Customer.valid() ||
		!spec.Contract.valid() ||
		!spec.Target.valid() ||
		!spec.Kind.valid() ||
		spec.SubmittedAt.IsZero() {
		return nil, ErrInvalidClaim
	}
	return &ClaimItem{
		id:          spec.ID,
		batch:       spec.Batch,
		customer:    spec.Customer,
		contract:    spec.Contract,
		target:      spec.Target,
		kind:        spec.Kind,
		submittedAt: spec.SubmittedAt.UTC(),
	}, nil
}

func (claim *ClaimItem) ID() ClaimItemID {
	return claim.id
}

func (claim *ClaimItem) Customer() CustomerAccountReference {
	return claim.customer
}

// Batch、Contract、Target 与 Kind 是（批次+项）存储键与资格审核查询的输入维——不
// 导出，适配器立不起键，资格规则也拿不到要核对的合同版本、目标范围与索赔类型。
func (claim *ClaimItem) Batch() ClaimBatchReference {
	return claim.batch
}

func (claim *ClaimItem) Contract() ContractScopeReference {
	return claim.contract
}

func (claim *ClaimItem) Target() RequestScopeReference {
	return claim.target
}

func (claim *ClaimItem) Kind() ClaimKindReference {
	return claim.kind
}

// SubmittedAt 是原始提交事实的时间——受理回执与首次索赔期限起算都读它。
func (claim *ClaimItem) SubmittedAt() time.Time {
	return claim.submittedAt
}

// Screen 报告资格审核结果及是否已审。
func (claim *ClaimItem) Screen() (EligibilityScreen, bool) {
	return claim.screen, claim.screen.valid()
}

// Conclusion 报告责任结论及是否已作出。
func (claim *ClaimItem) Conclusion() (LiabilityConclusion, bool) {
	return claim.conclusion, claim.conclusion.valid()
}

func (claim *ClaimItem) Withdrawn() bool {
	return claim.withdrawn
}

// PriorConclusion 只在受控复核换过结论的索赔上给出。
func (claim *ClaimItem) PriorConclusion() (LiabilityConclusion, bool) {
	return claim.priorConclusion, claim.priorConclusion.valid()
}

// ScreenEligibility 记录资格审核：按申请人授权、客户账户、合同版本、索赔时限、目标
// 范围、重复关系和最低材料要求判断（依据必带）；不通过不等于责任不成立——那是另一个
// 判断的事。已撤回或已审过的索赔不再审。
func (claim *ClaimItem) ScreenEligibility(screen EligibilityScreen, basis string, at time.Time) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	if _, screened := claim.Screen(); screened {
		return ErrClaimAlreadyScreened
	}
	if !screen.valid() || basis == "" || at.IsZero() || at.Before(claim.submittedAt) {
		return ErrInvalidClaim
	}
	claim.screen = screen
	claim.screenBasis = basis
	return nil
}

// ConcludeLiability 形成责任结论：结论按明确责任范围和证据形成，金额结算独立处理
// （CONTEXT 生命周期 252）。资格未审或未通过形不成责任结论；复核期限随结论固定。
func (claim *ClaimItem) ConcludeLiability(
	conclusion LiabilityConclusion,
	reviewBy time.Time,
	at time.Time,
) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	screen, screened := claim.Screen()
	if !screened || screen != ClaimEligible {
		return ErrClaimNotScreened
	}
	if _, concluded := claim.Conclusion(); concluded {
		return ErrClaimAlreadyConcluded
	}
	if !conclusion.valid() || reviewBy.IsZero() || at.IsZero() || !reviewBy.After(at) {
		return ErrInvalidClaim
	}
	claim.conclusion = conclusion
	claim.concludedAt = at.UTC()
	claim.reviewBy = reviewBy.UTC()
	return nil
}

// Withdraw 在最终责任结论前记录有效撤回：后续审核以已撤回结束，提交与证据保留
// （CONTEXT 生命周期 251）；撤回不取消异常案件或独立追偿事项——类型上没有那些字段。
// 已有结论的索赔撤不回。
func (claim *ClaimItem) Withdraw(at time.Time) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	if _, concluded := claim.Conclusion(); concluded {
		return ErrClaimAlreadyConcluded
	}
	if at.IsZero() || at.Before(claim.submittedAt) {
		return ErrInvalidClaim
	}
	claim.withdrawn = true
	claim.withdrawnAt = at.UTC()
	return nil
}

// ReviewConclusion 在复核期限内依据有效异议或关键新证据形成新的结论版本：原结论
// 保留在 PriorConclusion 上（CONTEXT 生命周期 253）；期限届满后拒——后续复核请求
// 形成有依据的不受理，不改变原责任结论（254）。
func (claim *ClaimItem) ReviewConclusion(
	conclusion LiabilityConclusion,
	at time.Time,
) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	current, concluded := claim.Conclusion()
	if !concluded {
		return ErrClaimNotScreened
	}
	if at.IsZero() || !conclusion.valid() || conclusion == current {
		return ErrInvalidClaim
	}
	if at.After(claim.reviewBy) {
		return ErrReviewWindowClosed
	}
	claim.priorConclusion = current
	claim.conclusion = conclusion
	claim.concludedAt = at.UTC()
	return nil
}
