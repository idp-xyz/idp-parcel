package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// HandleClaimOutcome 是索赔与追偿编排各入口共用的应用处理结果。收到、过审、结论、
// 复核与追偿各占其格——「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」
// （CONTEXT），压成一格就再也分不出客户此刻等在哪一步。
//
// `并发改动`同理自占一格，不并进未决：未决是「等依赖」，那条路的恢复动作是等它回来
// 再重试同一份；并发冲突要的是重读当前那一版再重判，两者压成一格会让恢复动作指错。
type HandleClaimOutcome uint8

const (
	HandleClaimOutcomeInvalid HandleClaimOutcome = iota
	ClaimReceived
	ClaimExistingResult
	ClaimScreened
	ClaimAwaitingSupplement
	ClaimScreenAlreadyRecorded
	ClaimConcluded
	ClaimConclusionAlreadyRecorded
	ClaimReviewed
	ClaimReviewWindowClosed
	ClaimConcurrentlyChanged
	RecoveryOpened
	RecoveryExistingResult
	RecoveryActionRecorded
	HandleClaimUndecided
	HandleClaimNotAccepted
)

func (outcome HandleClaimOutcome) String() string {
	switch outcome {
	case ClaimReceived:
		return "CLAIM_RECEIVED"
	case ClaimExistingResult:
		return "CLAIM_EXISTING_RESULT"
	case ClaimScreened:
		return "CLAIM_SCREENED"
	case ClaimAwaitingSupplement:
		return "CLAIM_AWAITING_SUPPLEMENT"
	case ClaimScreenAlreadyRecorded:
		return "SCREEN_ALREADY_RECORDED"
	case ClaimConcluded:
		return "CLAIM_CONCLUDED"
	case ClaimConclusionAlreadyRecorded:
		return "CONCLUSION_ALREADY_RECORDED"
	case ClaimReviewed:
		return "CLAIM_REVIEWED"
	case ClaimReviewWindowClosed:
		return "REVIEW_WINDOW_CLOSED"
	case ClaimConcurrentlyChanged:
		return "CLAIM_CONCURRENTLY_CHANGED"
	case RecoveryOpened:
		return "RECOVERY_OPENED"
	case RecoveryExistingResult:
		return "RECOVERY_EXISTING_RESULT"
	case RecoveryActionRecorded:
		return "RECOVERY_ACTION_RECORDED"
	case HandleClaimUndecided:
		return "UNDECIDED"
	case HandleClaimNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// HandleClaimUndecidedReason 指名本轮停在哪一步。资格审核那一段按**维**分格，不合成
// 一个笼统的「资格未配置」：各维的恢复动作各不相同——时限维等租户登记起算事件与业务
// 日历，授权维等查询补上申请人，重复关系维等一次领域裁断，材料维等证据归集接上。压
// 成一格，看到未决的人就不知道该去做哪一件事。
type HandleClaimUndecidedReason uint8

const (
	HandleClaimUndecidedReasonNone HandleClaimUndecidedReason = iota
	ClaimStoreUnavailable
	EligibilityRulesUnavailable
	EligibilityCatalogueNotConfigured
	EligibilityFilingDeadlineNotRegistered
	EligibilityAuthorizationNotRegistered
	EligibilityApplicantNotCarried
	EligibilityDuplicateUnresolved
	EligibilityMaterialsNotRegistered
	EligibilityEvidenceUnavailable
	EligibilitySupplementIncomplete
	EligibilitySupplementWindowClosed
	RecoveryStoreUnavailable
	RecoveryIdentityUnavailable
)

func (reason HandleClaimUndecidedReason) String() string {
	switch reason {
	case ClaimStoreUnavailable:
		return "CLAIM_STORE_UNAVAILABLE"
	case EligibilityRulesUnavailable:
		return "ELIGIBILITY_RULES_UNAVAILABLE"
	case EligibilityCatalogueNotConfigured:
		return "ELIGIBILITY_CATALOGUE_NOT_CONFIGURED"
	case EligibilityFilingDeadlineNotRegistered:
		return "ELIGIBILITY_FILING_DEADLINE_NOT_REGISTERED"
	case EligibilityAuthorizationNotRegistered:
		return "ELIGIBILITY_AUTHORIZATION_NOT_REGISTERED"
	case EligibilityApplicantNotCarried:
		return "ELIGIBILITY_APPLICANT_NOT_CARRIED"
	case EligibilityDuplicateUnresolved:
		return "ELIGIBILITY_DUPLICATE_UNRESOLVED"
	case EligibilityMaterialsNotRegistered:
		return "ELIGIBILITY_MATERIALS_NOT_REGISTERED"
	case EligibilityEvidenceUnavailable:
		return "ELIGIBILITY_EVIDENCE_UNAVAILABLE"
	case EligibilitySupplementIncomplete:
		return "ELIGIBILITY_SUPPLEMENT_INCOMPLETE"
	case EligibilitySupplementWindowClosed:
		return "ELIGIBILITY_SUPPLEMENT_WINDOW_CLOSED"
	case RecoveryStoreUnavailable:
		return "RECOVERY_STORE_UNAVAILABLE"
	case RecoveryIdentityUnavailable:
		return "RECOVERY_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// ReceiveClaimCommand 携带一项索赔的原始提交。项标识由客户提交侧建立并随批次唯一，
// 编排不签发——重复到达要靠它认得出自己。租户显式随命令到达（ADR-0003）：批次引用
// 只在租户内唯一，编排不替提交侧补租户。
type ReceiveClaimCommand struct {
	TenantID    domain.TenantID
	Batch       domain.ClaimBatchReference
	Item        domain.ClaimItemID
	Customer    domain.CustomerAccountReference
	Contract    domain.ContractScopeReference
	Target      domain.RequestScopeReference
	Kind        domain.ClaimKindReference
	SubmittedAt time.Time
}

// ScreenClaimCommand 请求对一项已受理索赔执行资格审核。结果由资格目录给出，命令不带
// ——带了就是让调用方替规则作判断。
type ScreenClaimCommand struct {
	TenantID domain.TenantID
	Batch    domain.ClaimBatchReference
	Item     domain.ClaimItemID
}

// ConcludeClaimCommand 携带授权审核方依据证据形成的责任结论与复核截止。结论与期限
// 来自适用合同与证据判断，编排只入账不改写。
type ConcludeClaimCommand struct {
	TenantID   domain.TenantID
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Conclusion domain.LiabilityConclusion
	ReviewBy   time.Time
}

// ReviewClaimCommand 携带复核形成的新结论。复核期限由领域按原结论固定的截止判断。
type ReviewClaimCommand struct {
	TenantID   domain.TenantID
	Batch      domain.ClaimBatchReference
	Item       domain.ClaimItemID
	Conclusion domain.LiabilityConclusion
}

// OpenRecoveryCommand 携带一项追偿事项的全部要件。这里没有任何索赔字段——追偿在
// 通知或主张条件成立时独立发起，不等客户索赔、责任结论或赔付（CONTEXT）。
type OpenRecoveryCommand struct {
	TenantID     domain.TenantID
	Case         domain.CaseID
	Counterparty domain.CounterpartyReference
	Basis        domain.LiabilityBasisReference
	LegalEntity  domain.LegalEntityReference
	Scope        domain.RequestScopeReference
	Evidence     domain.RequestEvidenceReference
	Deadline     time.Time
}

// RecordRecoveryCommand 追记一次追偿动作节点。时间取动作实际发生时刻（对外动作的
// 业务时间），不取本方时钟。
type RecordRecoveryCommand struct {
	TenantID   domain.TenantID
	Matter     domain.RecoveryMatterID
	Kind       domain.RecoveryActionKind
	ContentRef string
	Milestone  domain.RecoveryActionMilestone
	OccurredAt time.Time
}

type HandleClaimResult struct {
	outcome    HandleClaimOutcome
	claim      *domain.ClaimItem
	matter     domain.RecoveryMatter
	hasMatter  bool
	action     domain.RecoveryAction
	hasAction  bool
	reason     HandleClaimUndecidedReason
	handoffRef string
}

func (result HandleClaimResult) Outcome() HandleClaimOutcome {
	return result.outcome
}

// Claim 只在索赔项在场时给出。
func (result HandleClaimResult) Claim() (*domain.ClaimItem, bool) {
	return result.claim, result.claim != nil
}

// Recovery 只在追偿事项成立（本轮或此前）时给出。
func (result HandleClaimResult) Recovery() (domain.RecoveryMatter, bool) {
	return result.matter, result.hasMatter
}

// RecoveryAction 只在动作节点入账时给出。
func (result HandleClaimResult) RecoveryAction() (domain.RecoveryAction, bool) {
	return result.action, result.hasAction
}

func (result HandleClaimResult) UndecidedReason() HandleClaimUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明责任结论已成立但意图还没交出去，重放会重发同一份。
func (result HandleClaimResult) HandoffReference() string {
	return result.handoffRef
}

type HandleClaimDeps struct {
	Claims      ports.ClaimStore
	Eligibility ports.EligibilityRuleView
	Evidence    ports.ClaimEvidenceView
	Recoveries  ports.RecoveryStore
	Identities  ports.RecoveryIdentityFactory
	Settlement  ports.LiabilityHandoff
	Clock       ports.Clock
}

type HandleClaimHandler struct {
	deps HandleClaimDeps
}

func NewHandleClaimHandler(deps HandleClaimDeps) *HandleClaimHandler {
	return &HandleClaimHandler{deps: deps}
}

// ReceiveClaim 受理一项索赔：要件缺一即未受理；幂等按（批次+项），重复到达返回原项，
// 批次内各项独立——部分成功由「每项是独立对象」承担，这里没有整批动作。受理只保留
// 原始提交事实，不发结算意图：`UC-SA-007` 消费的是责任结论，不是收件回执。
func (handler *HandleClaimHandler) ReceiveClaim(
	ctx context.Context,
	command ReceiveClaimCommand,
) (HandleClaimResult, error) {
	if command.TenantID.String() == "" ||
		command.Batch.String() == "" ||
		command.Item.String() == "" ||
		command.Customer.String() == "" ||
		command.Contract.String() == "" ||
		command.Target.String() == "" ||
		command.Kind.String() == "" ||
		command.SubmittedAt.IsZero() {
		return HandleClaimResult{outcome: HandleClaimNotAccepted}, nil
	}

	existing, found, err := handler.deps.Claims.FindByBatchItem(ctx, command.TenantID, command.Batch, command.Item)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
	}
	if found {
		return HandleClaimResult{outcome: ClaimExistingResult, claim: existing}, nil
	}

	claim, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:          command.Item,
		Batch:       command.Batch,
		Customer:    command.Customer,
		Contract:    command.Contract,
		Target:      command.Target,
		Kind:        command.Kind,
		SubmittedAt: command.SubmittedAt,
	})
	if err != nil {
		return HandleClaimResult{}, fmt.Errorf("receive claim item: %w", err)
	}
	saved, err := handler.deps.Claims.Save(ctx, command.TenantID, claim)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
	}
	switch saved {
	case ports.ClaimSaved:
		return HandleClaimResult{outcome: ClaimReceived, claim: claim}, nil
	case ports.ClaimRevisionConflict:
		// 受理这一步的冲突只有一个来源：上面那次取回之后、本次写入之前，另一方把同一
		// （批次+项）建了出来。幂等按（批次+项），因此答案与「取回时就已存在」同格
		// ——读回赢家如实交出，不新开一格让调用方以为发生了别的事。
		existing, found, err := handler.deps.Claims.FindByBatchItem(
			ctx, command.TenantID, command.Batch, command.Item)
		if err != nil || !found {
			return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
		}
		return HandleClaimResult{outcome: ClaimExistingResult, claim: existing}, nil
	default:
		return HandleClaimResult{}, fmt.Errorf("receive claim: unexpected save outcome %d", saved)
	}
}

// recordClaim 落一步判断转移并把仓储的写入代数译成应答。三个判断入口共用这一段：
// 各写一遍迟早分叉，而分叉的方向恰好是把冲突悄悄并回未决。
//
// 第二个返回值为 false 时调用方原样交出第一、第三个返回值。冲突落`并发改动`：另一方
// 已经推进过这项索赔，本方手里的判断是在一份过期快照上作出的，交出去就是让客户看到
// 一个已经不成立的结论。它带上本轮那份索赔，调用方据以知道自己判的是哪一版。
func (handler *HandleClaimHandler) recordClaim(
	ctx context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) (HandleClaimResult, bool, error) {
	saved, err := handler.deps.Claims.Save(ctx, tenant, claim)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, false, nil
	}
	switch saved {
	case ports.ClaimSaved:
		return HandleClaimResult{}, true, nil
	case ports.ClaimRevisionConflict:
		return HandleClaimResult{outcome: ClaimConcurrentlyChanged, claim: claim}, false, nil
	default:
		return HandleClaimResult{}, false, fmt.Errorf("record claim: unexpected save outcome %d", saved)
	}
}

// 逐维核对的依据词。每一维核出什么都留一串，随资格结果一并入账——CONTEXT 要求
// 「结果保存合同、首次索赔期限、授权、重复关系和材料依据」，五样都在依据里点名，
// 事后才追得回这次审核核过什么、哪一维当时核不了。
const (
	basisKindCovered            = "CONTRACT_SCOPE_COVERS_KIND"
	basisKindNotCovered         = "CLAIM_KIND_NOT_IN_CONTRACT_SCOPE"
	basisFilingDeadlineMet      = "FILING_DEADLINE_MET"
	basisFilingDeadlineExceeded = "FILING_DEADLINE_EXCEEDED"
	basisFilingDeadlineAbsent   = "FILING_DEADLINE_RULE_NOT_REGISTERED"
	basisAuthorizationAbsent    = "AUTHORIZATION_CATALOGUE_NOT_REGISTERED"
	basisApplicantNotCarried    = "AUTHORIZATION_APPLICANT_NOT_CARRIED"
	basisDuplicateNone          = "DUPLICATE_NONE"
	basisDuplicateFound         = "DUPLICATE_FOUND"
	basisDuplicateUnavailable   = "DUPLICATE_LOOKUP_UNAVAILABLE"
	basisMaterialsComplete      = "MATERIALS_COMPLETE"
	basisMaterialsShort         = "MATERIALS_SHORT"
	basisMaterialsAbsent        = "MATERIALS_RULE_NOT_REGISTERED"
	basisEvidenceUnknown        = "MATERIALS_RECEIPT_UNKNOWN"
)

// dimensionOutcome 是一维核对的落法。四格不合并：`核不了`与`这一维不通过`天差地别
// ——前者等一件外部的事，后者是已经作出的判断。
type dimensionOutcome uint8

const (
	dimensionPassed dimensionOutcome = iota
	// dimensionDenied 只许由 ADR-0051 认可的两个永久格给出：合同责任范围不承担该
	// 索赔类型、超过首次索赔期限。别的维核出问题一律不落这一格——落进来就是一次
	// 不可经补充翻案的拒赔。
	dimensionDenied
	dimensionShortOfMaterials
	dimensionUncheckable
)

// dimensionVerdict 是一维的核对结果。basis 恒有；missing 只在差材料时有；reason 只
// 在核不了时有。
type dimensionVerdict struct {
	outcome dimensionOutcome
	basis   string
	missing []domain.MaterialRequirementReference
	reason  HandleClaimUndecidedReason
}

// ScreenClaim 执行资格审核：目录交出规则，编排拿着规则逐维核对事实。
//
// 拆开的理由见 .scratch/ve-claim-eligibility-dimensions 切块 (b)：重复关系要查同租户
// 已有的索赔项，最低材料要看已收到的证据，两样都不是目录行；让目录去读它们会造出
// 一个既是目录又能读业务数据的东西。目录只答「规则是什么」，仓储与证据答「事实是
// 什么」，本函数把两边合起来。
//
// 定局按不可逆程度排：**永久不予受理 > 等待补充 > 未决 > 通过**。
//   - 不予受理排最前且只收 ADR-0051 的两个永久格——它是唯一写下去就再也审不了的答案。
//   - 等待补充胜过未决：它可重入，且正是 CONTEXT 对「资料不足」规定的动作；换成未决
//     就什么也不记，客户永远不知道该补什么。
//   - 通过排最后且要求每一维都肯定通过。少核一维就答通过，等于把没审完的索赔永久
//     标成已过审——那与默认拒赔是同一个错的两面。
//
// 「已审过」只指终局格。停在`等待补充`的索赔可以再审——材料补齐后重判正是它存在的
// 理由（ADR-0051）。
func (handler *HandleClaimHandler) ScreenClaim(
	ctx context.Context,
	command ScreenClaimCommand,
) (HandleClaimResult, error) {
	claim, result, ok := handler.loadClaim(ctx, command.TenantID, command.Batch, command.Item)
	if !ok {
		return result, nil
	}

	rules, declared, err := handler.deps.Eligibility.RulesForClaim(ctx, ports.EligibilityQuery{
		Batch:    command.Batch,
		Item:     command.Item,
		Customer: claim.Customer(),
		Contract: claim.Contract(),
		Target:   claim.Target(),
		Kind:     claim.Kind(),
	})
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: EligibilityRulesUnavailable}, nil
	}
	if !declared {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: EligibilityCatalogueNotConfigured}, nil
	}

	// 五维一律核完再定局，不在中途短路：CONTEXT 要求资格结果保存合同、首次索赔期限、
	// 授权、重复关系和材料依据五样依据，短路会让先出结果的那一维把其余四样从记录里
	// 抹掉，而事后没人能从一句`不予受理`里读出当时另外四维是什么情形。
	// 维序只影响一件事：多维同时核不了时未决报哪一个。**申请人授权排在最后**，因为
	// 它今天核不了不是这项索赔缺了什么，而是查询还不带申请人（切块 (c)）——那一维对
	// 每项索赔一律核不了，排在前面就会把「这个租户还没登记材料清单」「这项索赔的证据
	// 查不到」这些真正修得动的缺口全盖住，读到未决的人只会反复看到同一句话。
	verdicts := []dimensionVerdict{
		judgeContractScope(rules),
		judgeFilingDeadline(rules, claim),
		handler.judgeDuplicateRelation(ctx, command.TenantID, claim),
		handler.judgeMinimumMaterials(ctx, command.TenantID, command.Batch, command.Item, rules),
		judgeApplicantAuthorization(rules),
	}
	basis := composeScreenBasis(rules, verdicts)

	outcome, reason, screenErr := handler.applyScreen(claim, verdicts, basis, rules)
	if outcome == HandleClaimOutcomeInvalid {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: reason}, nil
	}
	if screenErr != nil {
		switch {
		case errors.Is(screenErr, domain.ErrClaimAlreadyScreened):
			return HandleClaimResult{outcome: ClaimScreenAlreadyRecorded, claim: claim}, nil
		case errors.Is(screenErr, domain.ErrClaimWithdrawn):
			return HandleClaimResult{outcome: HandleClaimNotAccepted, claim: claim}, nil
		default:
			return HandleClaimResult{}, fmt.Errorf("screen eligibility: %w", screenErr)
		}
	}
	if answer, ok, err := handler.recordClaim(ctx, command.TenantID, claim); !ok {
		return answer, err
	}
	return HandleClaimResult{outcome: outcome, claim: claim}, nil
}

// applyScreen 按定局顺序把逐维结果落到索赔项上。第一个返回值为 HandleClaimOutcomeInvalid
// 即本轮停在未决，第二个返回值指名停在哪一件事上——那时索赔项一字未动。
//
// 第三态与终局两条路分开走，因为领域入口就不是同一个：`等待补充`要带四件落点
// （ADR-0051），而 ScreenEligibility 只收终局格。
func (handler *HandleClaimHandler) applyScreen(
	claim *domain.ClaimItem,
	verdicts []dimensionVerdict,
	basis string,
	rules ports.EligibilityRules,
) (HandleClaimOutcome, HandleClaimUndecidedReason, error) {
	now := handler.deps.Clock.Now()

	for _, verdict := range verdicts {
		if verdict.outcome == dimensionDenied {
			return ClaimScreened, HandleClaimUndecidedReasonNone,
				claim.ScreenEligibility(domain.ClaimIneligible, basis, now)
		}
	}

	if missing := collectMissingMaterials(verdicts); len(missing) > 0 {
		// 补充期限已经不在未来：CONTEXT 说补充期限届满只触发资格复核，规则未定或延期
		// 待确认时保持待决定并升级，**不能默认拒赔**。所以这里停在未决等人来看，既不
		// 拿一个过去的截止去立第三态，也不把它读成逾期未补。
		if !rules.Materials.SupplementDeadline.After(now) {
			return HandleClaimOutcomeInvalid, EligibilitySupplementWindowClosed, nil
		}
		requirement, ok := supplementRequirementFor(claim, rules, missing)
		if !ok {
			return HandleClaimOutcomeInvalid, EligibilitySupplementIncomplete, nil
		}
		return ClaimAwaitingSupplement, HandleClaimUndecidedReasonNone,
			claim.AwaitSupplement(basis, requirement, now)
	}

	for _, verdict := range verdicts {
		if verdict.outcome == dimensionUncheckable {
			return HandleClaimOutcomeInvalid, verdict.reason, nil
		}
	}
	// 这一格今天到不了：申请人授权那一维恒答核不了，上面那圈必然先返回。补上申请人
	// 维的切块 (c) 会把它接通，**那时它才第一次可测**——在此之前不要照着它推断`通过`
	// 已经验过，它写下的是不可逆的终局格。
	return ClaimScreened, HandleClaimUndecidedReasonNone,
		claim.ScreenEligibility(domain.ClaimEligible, basis, now)
}

// judgeContractScope 核合同责任范围承不承担这个索赔类型。这是目录独力判完的一维，
// 也是它唯一能得出永久`不予受理`的一维：承担与否不随材料补充而变，变了就是换了合同
// 范围，而换范围按 CONTEXT 是另一个索赔项。
func judgeContractScope(rules ports.EligibilityRules) dimensionVerdict {
	if !rules.KindCovered {
		return dimensionVerdict{outcome: dimensionDenied, basis: basisKindNotCovered}
	}
	return dimensionVerdict{outcome: dimensionPassed, basis: basisKindCovered}
}

// judgeFilingDeadline 核首次索赔期限。规则未登记时如实答核不了——起算事件与业务日历
// 是租户登记的实例参数（`PAR-VIS-08`），拿本方时钟凑一个默认时限，就把「还没登记」
// 变成了一次有依据的超期拒赔，而那一格按 ADR-0051 永久成立、补不回来。
func judgeFilingDeadline(rules ports.EligibilityRules, claim *domain.ClaimItem) dimensionVerdict {
	rule := rules.FilingDeadline
	if !rule.Registered || rule.Deadline.IsZero() || rule.RuleVersion == "" ||
		rule.StartEvent == "" || rule.Calendar == "" {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisFilingDeadlineAbsent,
			reason:  EligibilityFilingDeadlineNotRegistered,
		}
	}
	if claim.SubmittedAt().After(rule.Deadline) {
		return dimensionVerdict{
			outcome: dimensionDenied,
			basis:   basisFilingDeadlineExceeded + "/" + rule.RuleVersion + "/" + rule.StartEvent,
		}
	}
	return dimensionVerdict{
		outcome: dimensionPassed,
		basis:   basisFilingDeadlineMet + "/" + rule.RuleVersion + "/" + rule.StartEvent,
	}
}

// judgeApplicantAuthorization 核申请人授权。这一维今天结构上核不了：EligibilityQuery
// 里只有客户账户，而申请人与账户不是一回事（`AT-VE-125` 把两者并列）。补上那一维是
// 切块 (c) 的事；在它到来之前如实答核不了——答通过就是把一次没核过的授权记成已核。
func judgeApplicantAuthorization(rules ports.EligibilityRules) dimensionVerdict {
	if !rules.Authorization.Registered {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisAuthorizationAbsent,
			reason:  EligibilityAuthorizationNotRegistered,
		}
	}
	return dimensionVerdict{
		outcome: dimensionUncheckable,
		basis:   basisApplicantNotCarried + "/" + rules.Authorization.RuleVersion,
		reason:  EligibilityApplicantNotCarried,
	}
}

// judgeDuplicateRelation 核重复关系：同租户下同（客户账户+目标范围+索赔类型）还有没有
// 别的在办索赔。
//
// 数出来有重复时落`核不了`而不是`不予受理`：重复成立意味着什么是一次尚未作出的领域
// 裁断——ADR-0051 只认可两个永久格，重复不在其中。在这里替它拍板，就是拿一个计数造出
// 第三个永久拒赔格。没有重复则这一维肯定通过，那是事实本身给的答案，不需要规则。
func (handler *HandleClaimHandler) judgeDuplicateRelation(
	ctx context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) dimensionVerdict {
	count, err := handler.deps.Claims.CountLiveScopeClaims(
		ctx, tenant, claim.Customer(), claim.Target(), claim.Kind(), claim.ID())
	if err != nil {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisDuplicateUnavailable,
			reason:  ClaimStoreUnavailable,
		}
	}
	if count > 0 {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisDuplicateFound + "/" + strconv.Itoa(count),
			reason:  EligibilityDuplicateUnresolved,
		}
	}
	return dimensionVerdict{outcome: dimensionPassed, basis: basisDuplicateNone}
}

// judgeMinimumMaterials 核最低材料要求：目录给必须齐备的清单，证据侧给已经收到的，
// 相减即缺口。
//
// 缺口非空落`差材料`而不是`不予受理`——这是本切片最要紧的一格。`parcel-shipment` 的
// 收寄资格视图在证据取不到时如实答「未成立」并点名首项缺口，那一格在 PS 可续办；把
// 那份写法搬到这里会变成不可逆的默认拒赔，而编译与测试都不会拦。
func (handler *HandleClaimHandler) judgeMinimumMaterials(
	ctx context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
	rules ports.EligibilityRules,
) dimensionVerdict {
	if !rules.Materials.Registered {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisMaterialsAbsent,
			reason:  EligibilityMaterialsNotRegistered,
		}
	}
	received, known, err := handler.deps.Evidence.ReceivedMaterials(ctx, tenant, batch, item)
	if err != nil || !known {
		return dimensionVerdict{
			outcome: dimensionUncheckable,
			basis:   basisEvidenceUnknown,
			reason:  EligibilityEvidenceUnavailable,
		}
	}

	present := make(map[string]struct{}, len(received))
	for _, material := range received {
		present[material.String()] = struct{}{}
	}
	var missing []domain.MaterialRequirementReference
	for _, required := range rules.Materials.Required {
		if _, ok := present[required.String()]; !ok {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return dimensionVerdict{
			outcome: dimensionShortOfMaterials,
			basis:   basisMaterialsShort + "/" + rules.Materials.RuleVersion + "/" + joinMaterials(missing),
			missing: missing,
		}
	}
	return dimensionVerdict{
		outcome: dimensionPassed,
		basis:   basisMaterialsComplete + "/" + rules.Materials.RuleVersion,
	}
}

// supplementRequirementFor 凑齐等待补充的四件落点。缺少材料由差集得出、补充范围取
// 索赔自己固定的目标范围（CONTEXT 硬句：每个索赔项固定一个目标包裹或明确服务范围，
// 要补的材料只能落在那个范围里）、通知依据与当前截止由规则给。
//
// 凑不齐即答不成立，调用方停在未决而不是记一个残缺的第三态：没有这四件的`等待补充`
// 与`资格尚未审核`分不开（ADR-0051）。这里没有一处编排自造的词——材料名全来自目录
// 签发的引用，编排只作差集与排序。
func supplementRequirementFor(
	claim *domain.ClaimItem,
	rules ports.EligibilityRules,
	missing []domain.MaterialRequirementReference,
) (domain.SupplementRequirement, bool) {
	materials, err := domain.NewMissingMaterialsReference(joinMaterials(missing))
	if err != nil {
		return domain.SupplementRequirement{}, false
	}
	scope, err := domain.NewSupplementScopeReference(claim.Target().String())
	if err != nil {
		return domain.SupplementRequirement{}, false
	}
	requirement, err := domain.NewSupplementRequirement(
		materials, scope, rules.Materials.Notice, rules.Materials.SupplementDeadline)
	if err != nil {
		return domain.SupplementRequirement{}, false
	}
	return requirement, true
}

// joinMaterials 把材料引用排序后连成一串。排序是为了同一组缺口每次得出同一串——
// 缺少材料要随索赔项永久留底，顺序一变，两次实为同一缺口的记录就看着像变过。
func joinMaterials(materials []domain.MaterialRequirementReference) string {
	values := make([]string, 0, len(materials))
	for _, material := range materials {
		values = append(values, material.String())
	}
	sort.Strings(values)
	return strings.Join(values, "+")
}

func collectMissingMaterials(verdicts []dimensionVerdict) []domain.MaterialRequirementReference {
	var missing []domain.MaterialRequirementReference
	for _, verdict := range verdicts {
		if verdict.outcome == dimensionShortOfMaterials {
			missing = append(missing, verdict.missing...)
		}
	}
	return missing
}

// composeScreenBasis 把五维依据连成随资格结果入账的那一串，前缀规则版本。CONTEXT
// 要求结果保存合同、首次索赔期限、授权、重复关系和材料依据——五样各占一段，缺哪一段
// 都说明有一维没核过。
func composeScreenBasis(rules ports.EligibilityRules, verdicts []dimensionVerdict) string {
	parts := make([]string, 0, len(verdicts)+1)
	parts = append(parts, "eligibility-rules/"+rules.RuleVersion)
	for _, verdict := range verdicts {
		parts = append(parts, verdict.basis)
	}
	return strings.Join(parts, "; ")
}

// ConcludeClaim 入账责任结论并把它交给结算：资格未审或未通过形不成结论（领域把门，
// 编排答未受理不撞错）；已有结论不覆盖，重放只把同一份意图再交一次（ADR-0043）。
func (handler *HandleClaimHandler) ConcludeClaim(
	ctx context.Context,
	command ConcludeClaimCommand,
) (HandleClaimResult, error) {
	claim, result, ok := handler.loadClaim(ctx, command.TenantID, command.Batch, command.Item)
	if !ok {
		return result, nil
	}

	if err := claim.ConcludeLiability(command.Conclusion, command.ReviewBy, handler.deps.Clock.Now()); err != nil {
		switch {
		case errors.Is(err, domain.ErrClaimAlreadyConcluded):
			return HandleClaimResult{
				outcome:    ClaimConclusionAlreadyRecorded,
				claim:      claim,
				handoffRef: handler.handOffLiability(ctx, command.TenantID, claim),
			}, nil
		case errors.Is(err, domain.ErrClaimNotScreened), errors.Is(err, domain.ErrClaimWithdrawn):
			return HandleClaimResult{outcome: HandleClaimNotAccepted, claim: claim}, nil
		default:
			return HandleClaimResult{}, fmt.Errorf("conclude liability: %w", err)
		}
	}
	// 冲突时不交结算意图：结论没落库，交出去就是让下游按一份不存在的结论算钱。
	if answer, ok, err := handler.recordClaim(ctx, command.TenantID, claim); !ok {
		return answer, err
	}
	return HandleClaimResult{
		outcome:    ClaimConcluded,
		claim:      claim,
		handoffRef: handler.handOffLiability(ctx, command.TenantID, claim),
	}, nil
}

// ReviewClaim 受控复核：期限内换出新结论版本、原结论保留并把新版本交给结算；期限
// 届满即不受理复核，原结论不变——那是有依据的业务答案，不是故障。
func (handler *HandleClaimHandler) ReviewClaim(
	ctx context.Context,
	command ReviewClaimCommand,
) (HandleClaimResult, error) {
	claim, result, ok := handler.loadClaim(ctx, command.TenantID, command.Batch, command.Item)
	if !ok {
		return result, nil
	}

	if err := claim.ReviewConclusion(command.Conclusion, handler.deps.Clock.Now()); err != nil {
		switch {
		case errors.Is(err, domain.ErrReviewWindowClosed):
			return HandleClaimResult{outcome: ClaimReviewWindowClosed, claim: claim}, nil
		case errors.Is(err, domain.ErrClaimNotScreened),
			errors.Is(err, domain.ErrClaimWithdrawn),
			errors.Is(err, domain.ErrInvalidClaim):
			// 未有结论无从复核、已撤回不复核、同结论的复核没有内容——都是调用方立不住
			// 的请求，如实拒。
			return HandleClaimResult{outcome: HandleClaimNotAccepted, claim: claim}, nil
		default:
			return HandleClaimResult{}, fmt.Errorf("review conclusion: %w", err)
		}
	}
	// 冲突时不交结算意图：换出的新结论版本没落库，交出去就是让结算按一份不存在的
	// 复核改口径，而原结论在库里仍然有效。
	if answer, ok, err := handler.recordClaim(ctx, command.TenantID, claim); !ok {
		return answer, err
	}
	return HandleClaimResult{
		outcome:    ClaimReviewed,
		claim:      claim,
		handoffRef: handler.handOffLiability(ctx, command.TenantID, claim),
	}, nil
}

// OpenRecovery 独立建立追偿事项：不读任何索赔——通知或主张条件成立即可发起，客户
// 索赔、责任结论与赔付都不是前置。幂等按（案件+相对方+范围）。
func (handler *HandleClaimHandler) OpenRecovery(
	ctx context.Context,
	command OpenRecoveryCommand,
) (HandleClaimResult, error) {
	if command.TenantID.String() == "" ||
		command.Case.String() == "" ||
		command.Counterparty.String() == "" ||
		command.Basis.String() == "" ||
		command.LegalEntity.String() == "" ||
		command.Scope.String() == "" ||
		command.Evidence.String() == "" ||
		command.Deadline.IsZero() {
		return HandleClaimResult{outcome: HandleClaimNotAccepted}, nil
	}

	existing, found, err := handler.deps.Recoveries.FindCurrent(
		ctx, command.TenantID, command.Case, command.Counterparty, command.Scope)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
	}
	if found {
		return HandleClaimResult{outcome: RecoveryExistingResult, matter: existing, hasMatter: true}, nil
	}

	matterID, err := handler.deps.Identities.NextRecoveryMatterID(ctx)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryIdentityUnavailable}, nil
	}
	matter, err := domain.OpenRecoveryMatter(domain.RecoveryMatterSpec{
		ID:           matterID,
		Case:         command.Case,
		Counterparty: command.Counterparty,
		Basis:        command.Basis,
		LegalEntity:  command.LegalEntity,
		Scope:        command.Scope,
		Evidence:     command.Evidence,
		Deadline:     command.Deadline,
		OpenedAt:     handler.deps.Clock.Now(),
	})
	if err != nil {
		return HandleClaimResult{}, fmt.Errorf("open recovery matter: %w", err)
	}
	saved, err := handler.deps.Recoveries.Save(ctx, command.TenantID, matter)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
	}
	switch saved {
	case ports.RecoverySaved:
		return HandleClaimResult{outcome: RecoveryOpened, matter: matter, hasMatter: true}, nil
	case ports.RecoveryAlreadyRecorded:
		existing, found, err := handler.deps.Recoveries.FindCurrent(
			ctx, command.TenantID, command.Case, command.Counterparty, command.Scope)
		if err != nil || !found {
			return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
		}
		return HandleClaimResult{outcome: RecoveryExistingResult, matter: existing, hasMatter: true}, nil
	default:
		return HandleClaimResult{}, fmt.Errorf("open recovery: unexpected save outcome %d", saved)
	}
}

// RecordRecovery 追记一次追偿动作节点：预先通知与正式主张各有各的尝试序列（attempt
// 按事项+种类递增），失败后的重试是新记录不是改写，所有尝试与内容版本保留。义务判据
// 取事项固定的责任依据——哪个节点满足期限义务由它背后的协议或条款说了算，这里不判。
func (handler *HandleClaimHandler) RecordRecovery(
	ctx context.Context,
	command RecordRecoveryCommand,
) (HandleClaimResult, error) {
	if command.TenantID.String() == "" ||
		command.Matter.String() == "" ||
		command.Kind.String() == "" ||
		command.ContentRef == "" ||
		command.Milestone.String() == "" ||
		command.OccurredAt.IsZero() {
		return HandleClaimResult{outcome: HandleClaimNotAccepted}, nil
	}

	matter, found, err := handler.deps.Recoveries.FindByID(ctx, command.TenantID, command.Matter)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
	}
	if !found {
		return HandleClaimResult{outcome: HandleClaimNotAccepted}, nil
	}

	attempts, err := handler.deps.Recoveries.CountActions(ctx, command.TenantID, command.Matter, command.Kind)
	if err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
	}
	action, err := domain.RecordRecoveryAction(
		command.Matter,
		command.Kind,
		command.ContentRef,
		command.Milestone,
		matter.Basis(),
		command.OccurredAt,
		attempts+1,
	)
	if err != nil {
		return HandleClaimResult{}, fmt.Errorf("record recovery action: %w", err)
	}
	if err := handler.deps.Recoveries.AppendAction(ctx, command.TenantID, action); err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: RecoveryStoreUnavailable}, nil
	}
	return HandleClaimResult{outcome: RecoveryActionRecorded, matter: matter, hasMatter: true, action: action, hasAction: true}, nil
}

// loadClaim 取回（租户+批次+项）指名的索赔。第三个返回值为 false 时第二个返回值即
// 应答——三个判断入口共用同一段取回与未受理分流，各写一遍迟早分叉。
func (handler *HandleClaimHandler) loadClaim(
	ctx context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) (*domain.ClaimItem, HandleClaimResult, bool) {
	if tenant.String() == "" || batch.String() == "" || item.String() == "" {
		return nil, HandleClaimResult{outcome: HandleClaimNotAccepted}, false
	}
	claim, found, err := handler.deps.Claims.FindByBatchItem(ctx, tenant, batch, item)
	if err != nil {
		return nil, HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, false
	}
	if !found {
		return nil, HandleClaimResult{outcome: HandleClaimNotAccepted}, false
	}
	return claim, HandleClaimResult{}, true
}

// handOffLiability 把当前责任结论交给结算侧，交不出去时交回发布续办引用（ADR-0043）。
// 复核换出的新结论走同一条缝：意图仍由索赔项认领，重发携带的是当前版本。
func (handler *HandleClaimHandler) handOffLiability(ctx context.Context, tenant domain.TenantID, claim *domain.ClaimItem) string {
	if err := handler.deps.Settlement.HandOffLiability(ctx, ports.LiabilityHandoffIntent{
		TenantID: tenant,
		Claim:    claim,
	}); err != nil {
		return "CONT-" + shortDigest("LIABILITY_HANDOFF", claim.ID().String())
	}
	return ""
}
