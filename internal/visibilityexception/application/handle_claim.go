package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// HandleClaimOutcome 是索赔与追偿编排各入口共用的应用处理结果。收到、过审、结论、
// 复核与追偿各占其格——「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」
// （CONTEXT），压成一格就再也分不出客户此刻等在哪一步。
type HandleClaimOutcome uint8

const (
	HandleClaimOutcomeInvalid HandleClaimOutcome = iota
	ClaimReceived
	ClaimExistingResult
	ClaimScreened
	ClaimScreenAlreadyRecorded
	ClaimConcluded
	ClaimConclusionAlreadyRecorded
	ClaimReviewed
	ClaimReviewWindowClosed
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

// HandleClaimUndecidedReason 指名本轮停在哪一步。`资格目录未配置`与`资格规则答不出`
// 分开：一个等租户登记索赔时限与材料目录，一个重试依赖。
type HandleClaimUndecidedReason uint8

const (
	HandleClaimUndecidedReasonNone HandleClaimUndecidedReason = iota
	ClaimStoreUnavailable
	EligibilityRulesUnavailable
	EligibilityCatalogueNotConfigured
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
	if err := handler.deps.Claims.Save(ctx, command.TenantID, claim); err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
	}
	return HandleClaimResult{outcome: ClaimReceived, claim: claim}, nil
}

// ScreenClaim 执行资格审核：目录未配置即未决——没有目录的资格审核无从作出，默认受理
// 与默认拒赔都是虚构；已审过的不再审（依据由目录给出，编排不自造）。
func (handler *HandleClaimHandler) ScreenClaim(
	ctx context.Context,
	command ScreenClaimCommand,
) (HandleClaimResult, error) {
	claim, result, ok := handler.loadClaim(ctx, command.TenantID, command.Batch, command.Item)
	if !ok {
		return result, nil
	}

	answer, configured, err := handler.deps.Eligibility.ScreenClaim(ctx, ports.EligibilityQuery{
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
	if !configured {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: EligibilityCatalogueNotConfigured}, nil
	}

	if err := claim.ScreenEligibility(answer.Screen, answer.Basis, handler.deps.Clock.Now()); err != nil {
		switch {
		case errors.Is(err, domain.ErrClaimAlreadyScreened):
			return HandleClaimResult{outcome: ClaimScreenAlreadyRecorded, claim: claim}, nil
		case errors.Is(err, domain.ErrClaimWithdrawn):
			return HandleClaimResult{outcome: HandleClaimNotAccepted, claim: claim}, nil
		default:
			return HandleClaimResult{}, fmt.Errorf("screen eligibility: %w", err)
		}
	}
	if err := handler.deps.Claims.Save(ctx, command.TenantID, claim); err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
	}
	return HandleClaimResult{outcome: ClaimScreened, claim: claim}, nil
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
	if err := handler.deps.Claims.Save(ctx, command.TenantID, claim); err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
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
	if err := handler.deps.Claims.Save(ctx, command.TenantID, claim); err != nil {
		return HandleClaimResult{outcome: HandleClaimUndecided, reason: ClaimStoreUnavailable}, nil
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
