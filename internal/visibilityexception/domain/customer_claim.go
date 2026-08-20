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

// EligibilityScreen 是资格审核的封闭三态（ADR-0051）：通过、不予受理、等待补充。
type EligibilityScreen uint8

const (
	EligibilityScreenInvalid EligibilityScreen = iota
	ClaimEligible
	ClaimIneligible
	ClaimAwaitingSupplement
)

func (screen EligibilityScreen) valid() bool {
	return screen == ClaimEligible || screen == ClaimIneligible || screen == ClaimAwaitingSupplement
}

// terminal 是终局格：通过或不予受理。等待补充可重入，终局才触发 ErrClaimAlreadyScreened。
func (screen EligibilityScreen) terminal() bool {
	return screen == ClaimEligible || screen == ClaimIneligible
}

func (screen EligibilityScreen) String() string {
	switch screen {
	case ClaimEligible:
		return "ELIGIBLE"
	case ClaimIneligible:
		return "INELIGIBLE"
	case ClaimAwaitingSupplement:
		return "AWAITING_SUPPLEMENT"
	default:
		return ""
	}
}

// MissingMaterialsReference 指名等待补充时固定的缺少材料。
type MissingMaterialsReference struct{ requiredValue }

func NewMissingMaterialsReference(value string) (MissingMaterialsReference, error) {
	required, err := newRequiredValue("missing materials reference", value)
	return MissingMaterialsReference{required}, err
}

// SupplementScopeReference 指名补充范围。
type SupplementScopeReference struct{ requiredValue }

func NewSupplementScopeReference(value string) (SupplementScopeReference, error) {
	required, err := newRequiredValue("supplement scope reference", value)
	return SupplementScopeReference{required}, err
}

// SupplementNoticeReference 指名补充通知依据。
type SupplementNoticeReference struct{ requiredValue }

func NewSupplementNoticeReference(value string) (SupplementNoticeReference, error) {
	required, err := newRequiredValue("supplement notice reference", value)
	return SupplementNoticeReference{required}, err
}

// SupplementRequirement 是等待补充的四件落点（CONTEXT：缺少材料、补充范围、通知
// 依据和当前截止时间）。缺一不可——没有这四件的「等待补充」与尚未审核分不开。
type SupplementRequirement struct {
	MissingMaterials MissingMaterialsReference
	Scope            SupplementScopeReference
	Notice           SupplementNoticeReference
	Deadline         time.Time
}

func NewSupplementRequirement(
	missing MissingMaterialsReference,
	scope SupplementScopeReference,
	notice SupplementNoticeReference,
	deadline time.Time,
) (SupplementRequirement, error) {
	if !missing.valid() || !scope.valid() || !notice.valid() || deadline.IsZero() {
		return SupplementRequirement{}, ErrInvalidClaim
	}
	return SupplementRequirement{
		MissingMaterials: missing,
		Scope:            scope,
		Notice:           notice,
		Deadline:         deadline.UTC(),
	}, nil
}

func (requirement SupplementRequirement) valid() bool {
	return requirement.MissingMaterials.valid() &&
		requirement.Scope.valid() &&
		requirement.Notice.valid() &&
		!requirement.Deadline.IsZero()
}

// Complete 让编排在调 AwaitSupplement 之前就分得出「目录答复残缺」与「这项索赔真的
// 该等补充」。少了它，两者都从 AwaitSupplement 撞出同一个 ErrInvalidClaim，而前者要
// 找目录补登记、后者是正常业务路径。
func (requirement SupplementRequirement) Complete() bool {
	return requirement.valid()
}

// SupplementDeadlineVersion 是一版补充期限。获批延期追加新版本，原期限保留。
type SupplementDeadlineVersion struct {
	Deadline      time.Time
	EstablishedAt time.Time
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
	// revision 是这份索赔被读出时的持久化修订，不是判断历史的一部分——三判各步推进
	// 的是上面那些列，这一格只回答「我是从哪一版读出来的」，供仓储作条件更新。受理
	// 出来的索赔还没落过库，它是零。
	revision        int64
	id              ClaimItemID
	batch           ClaimBatchReference
	customer        CustomerAccountReference
	contract        ContractScopeReference
	target          RequestScopeReference
	kind            ClaimKindReference
	submittedAt     time.Time
	screen          EligibilityScreen
	screenBasis     string
	supplement      SupplementRequirement
	deadlineHistory []SupplementDeadlineVersion
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

// Revision 交回本索赔被读出时的持久化修订，零即尚未落过库。它是 Save 的预期修订
// ——一个事实一处表达，不另作参数传（ADR-0031）：三判转移一律不动它，因此聚合带的
// 这一格与调用方本该递的那个值恒等，再开一个入参只会造出第二个来源。
func (claim *ClaimItem) Revision() int64 {
	return claim.revision
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

// Screen 报告资格审核结果及是否已有结果。等待补充算已有结果，但不是终局——终局
// 才触发 ErrClaimAlreadyScreened（ADR-0051）。
func (claim *ClaimItem) Screen() (EligibilityScreen, bool) {
	return claim.screen, claim.screen.valid()
}

// Supplement 只在等待补充时给出四件落点。
func (claim *ClaimItem) Supplement() (SupplementRequirement, bool) {
	return claim.supplement, claim.screen == ClaimAwaitingSupplement
}

// SupplementDeadlineHistory 交回已确立的补充期限版本，含当前截止。获批延期追加，
// 原期限保留。
func (claim *ClaimItem) SupplementDeadlineHistory() []SupplementDeadlineVersion {
	return append([]SupplementDeadlineVersion(nil), claim.deadlineHistory...)
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

// ClaimItemSnapshot 是持久化层重建索赔项所需的全量状态。三判各自的状态与依据、
// 复核前版、撤回时间都是已发生的判断历史——重建不重演三判方法（重演需要按原次序
// 原时间走一遍，而库里只有结果）。
type ClaimItemSnapshot struct {
	// Revision 随快照往返：重建门只接受 ≥ 1（重建的来源只有已落库的行），Snapshot
	// 原样折出，供仓储拿它作条件更新的期望值。
	Revision        int64
	ID              ClaimItemID
	Batch           ClaimBatchReference
	Customer        CustomerAccountReference
	Contract        ContractScopeReference
	Target          RequestScopeReference
	Kind            ClaimKindReference
	SubmittedAt     time.Time
	Screen          EligibilityScreen
	ScreenBasis     string
	Supplement      SupplementRequirement
	DeadlineHistory []SupplementDeadlineVersion
	Conclusion      LiabilityConclusion
	ConcludedAt     time.Time
	ReviewBy        time.Time
	PriorConclusion LiabilityConclusion
	Withdrawn       bool
	WithdrawnAt     time.Time
}

// Snapshot 折出索赔项的全量状态供持久化。
func (claim *ClaimItem) Snapshot() ClaimItemSnapshot {
	return ClaimItemSnapshot{
		Revision:        claim.revision,
		ID:              claim.id,
		Batch:           claim.batch,
		Customer:        claim.customer,
		Contract:        claim.contract,
		Target:          claim.target,
		Kind:            claim.kind,
		SubmittedAt:     claim.submittedAt,
		Screen:          claim.screen,
		ScreenBasis:     claim.screenBasis,
		Supplement:      claim.supplement,
		DeadlineHistory: append([]SupplementDeadlineVersion(nil), claim.deadlineHistory...),
		Conclusion:      claim.conclusion,
		ConcludedAt:     claim.concludedAt,
		ReviewBy:        claim.reviewBy,
		PriorConclusion: claim.priorConclusion,
		Withdrawn:       claim.withdrawn,
		WithdrawnAt:     claim.withdrawnAt,
	}
}

// RehydrateClaimItem 从快照重建索赔项并重验三判形状：审过必有依据、结论必经过审
// （资格通过）且带复核期限、前版只随复核出现且不等于现结论、撤回与结论互斥——
// 一次坏写入不得变成一个看起来合法的判断历史。
func RehydrateClaimItem(snapshot ClaimItemSnapshot) (*ClaimItem, error) {
	// 重建的来源只有已落库的行，而落库的行必有首版修订。零在这里进来说明快照不是从
	// 库里折出来的，放它过去会让一份凭空造的索赔冒充「读出来的那一版」，随后带着零去
	// 作条件更新——那正是丢更新回来的路。
	if snapshot.Revision < 1 {
		return nil, ErrInvalidClaim
	}
	if !snapshot.ID.valid() ||
		!snapshot.Batch.valid() ||
		!snapshot.Customer.valid() ||
		!snapshot.Contract.valid() ||
		!snapshot.Target.valid() ||
		!snapshot.Kind.valid() ||
		snapshot.SubmittedAt.IsZero() {
		return nil, ErrInvalidClaim
	}
	if snapshot.Screen.valid() != (snapshot.ScreenBasis != "") {
		return nil, ErrInvalidClaim
	}
	if snapshot.Conclusion.valid() {
		if snapshot.Screen != ClaimEligible ||
			snapshot.ConcludedAt.IsZero() || snapshot.ReviewBy.IsZero() ||
			snapshot.ConcludedAt.After(snapshot.ReviewBy) ||
			snapshot.Withdrawn {
			return nil, ErrInvalidClaim
		}
	} else if !snapshot.ConcludedAt.IsZero() || !snapshot.ReviewBy.IsZero() ||
		snapshot.PriorConclusion.valid() {
		return nil, ErrInvalidClaim
	}
	if snapshot.PriorConclusion.valid() && snapshot.PriorConclusion == snapshot.Conclusion {
		return nil, ErrInvalidClaim
	}
	if snapshot.Withdrawn != !snapshot.WithdrawnAt.IsZero() {
		return nil, ErrInvalidClaim
	}
	awaiting := snapshot.Screen == ClaimAwaitingSupplement
	if awaiting != snapshot.Supplement.valid() {
		return nil, ErrInvalidClaim
	}
	if awaiting {
		if len(snapshot.DeadlineHistory) == 0 {
			return nil, ErrInvalidClaim
		}
		last := snapshot.DeadlineHistory[len(snapshot.DeadlineHistory)-1]
		if !last.Deadline.Equal(snapshot.Supplement.Deadline.UTC()) || last.EstablishedAt.IsZero() {
			return nil, ErrInvalidClaim
		}
	} else if !snapshot.Screen.valid() && len(snapshot.DeadlineHistory) > 0 {
		return nil, ErrInvalidClaim
	}
	history := append([]SupplementDeadlineVersion(nil), snapshot.DeadlineHistory...)
	for i := range history {
		history[i].Deadline = history[i].Deadline.UTC()
		history[i].EstablishedAt = history[i].EstablishedAt.UTC()
	}
	supplement := snapshot.Supplement
	if supplement.valid() {
		supplement.Deadline = supplement.Deadline.UTC()
	}
	return &ClaimItem{
		revision:        snapshot.Revision,
		id:              snapshot.ID,
		batch:           snapshot.Batch,
		customer:        snapshot.Customer,
		contract:        snapshot.Contract,
		target:          snapshot.Target,
		kind:            snapshot.Kind,
		submittedAt:     snapshot.SubmittedAt.UTC(),
		screen:          snapshot.Screen,
		screenBasis:     snapshot.ScreenBasis,
		supplement:      supplement,
		deadlineHistory: history,
		conclusion:      snapshot.Conclusion,
		concludedAt:     snapshot.ConcludedAt.UTC(),
		reviewBy:        snapshot.ReviewBy.UTC(),
		priorConclusion: snapshot.PriorConclusion,
		withdrawn:       snapshot.Withdrawn,
		withdrawnAt:     snapshot.WithdrawnAt.UTC(),
	}, nil
}

// ScreenEligibility 记录终局资格审核（通过或不予受理）。等待补充走 AwaitSupplement。
// 已撤回或已落终局的索赔不再审；处于等待补充时允许重判到终局（ADR-0051）。
func (claim *ClaimItem) ScreenEligibility(screen EligibilityScreen, basis string, at time.Time) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	if claim.screen.terminal() {
		return ErrClaimAlreadyScreened
	}
	if !screen.terminal() || basis == "" || at.IsZero() || at.Before(claim.submittedAt) {
		return ErrInvalidClaim
	}
	claim.screen = screen
	claim.screenBasis = basis
	claim.supplement = SupplementRequirement{}
	return nil
}

// AwaitSupplement 进入或续写等待补充。四件落点必备。已落终局的索赔不可改走补充
// （合同不覆盖、超首次期限等永久格不得经补充翻案）。同一截止下更新缺少材料是
// 重新判断；换截止走 ExtendSupplementDeadline。
func (claim *ClaimItem) AwaitSupplement(basis string, requirement SupplementRequirement, at time.Time) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	if claim.screen.terminal() {
		return ErrClaimAlreadyScreened
	}
	if !requirement.valid() || basis == "" || at.IsZero() || at.Before(claim.submittedAt) {
		return ErrInvalidClaim
	}
	if !requirement.Deadline.After(at) {
		return ErrInvalidClaim
	}
	if claim.screen == ClaimAwaitingSupplement {
		if !requirement.Deadline.Equal(claim.supplement.Deadline) {
			return ErrInvalidClaim
		}
		claim.screenBasis = basis
		claim.supplement = requirement
		return nil
	}
	claim.screen = ClaimAwaitingSupplement
	claim.screenBasis = basis
	claim.supplement = requirement
	claim.deadlineHistory = []SupplementDeadlineVersion{{
		Deadline:      requirement.Deadline,
		EstablishedAt: at.UTC(),
	}}
	return nil
}

// ExtendSupplementDeadline 获批延期：新期限版本入列，原期限保留。必须已在等待补充。
func (claim *ClaimItem) ExtendSupplementDeadline(deadline time.Time, at time.Time) error {
	if claim.withdrawn {
		return ErrClaimWithdrawn
	}
	if claim.screen != ClaimAwaitingSupplement {
		return ErrInvalidClaim
	}
	if at.IsZero() || at.Before(claim.submittedAt) || !deadline.After(claim.supplement.Deadline) {
		return ErrInvalidClaim
	}
	claim.deadlineHistory = append(claim.deadlineHistory, SupplementDeadlineVersion{
		Deadline:      deadline.UTC(),
		EstablishedAt: at.UTC(),
	})
	claim.supplement.Deadline = deadline.UTC()
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
