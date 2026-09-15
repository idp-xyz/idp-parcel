// Package application 在领域内核与 parcel-shipment 自有端口之上编排本上下文的用例。
// 它不含任何持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmitOutcome 是应用处理结果，不是委托的生命周期状态。只有 OutcomeSubmitted 会
// 建立委托，且这些取值没有一个表示委托已被接受或拒绝。
type SubmitOutcome uint8

const (
	OutcomeInvalid SubmitOutcome = iota
	OutcomeSubmitted
	OutcomeExistingResult
	OutcomeIngressConflict
	OutcomeInputNotAccepted
	OutcomeOtherProductionAuthority
	OutcomeOwnershipUnresolved
	// OutcomeAdmissionPaused 与 OutcomeOwnershipUnresolved 分开，因为暂停回答的是
	// 本产品当前是否接纳新准入，而不是这个范围归谁。合并会丢掉「权威其实已经确定」
	// 这个事实。
	OutcomeAdmissionPaused
	// OutcomePriorRequestNotFound 统一回答关联指名的三种不可见：查无此委托、编号不符、
	// 跨租户或跨客户指名。可区分即可枚举别人的委托（ADR-0029 的合并理由，`AT-PS-075`
	// 同款纪律在关联指名上一字不差地成立）。
	OutcomePriorRequestNotFound
	// OutcomeLinkIneligible 说原委托找到了、也确属这个客户，只是它的状态不承认这个
	// 关联方向——客户该走的是资料修订（待决）或等决定，不是关联新委托。与「查无」分格：
	// 两者的续办动作不同。
	OutcomeLinkIneligible
)

func (outcome SubmitOutcome) String() string {
	switch outcome {
	case OutcomeSubmitted:
		return "SUBMITTED"
	case OutcomeExistingResult:
		return "EXISTING_RESULT"
	case OutcomeIngressConflict:
		return "INGRESS_CONFLICT"
	case OutcomeInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case OutcomeOtherProductionAuthority:
		return "OTHER_PRODUCTION_AUTHORITY"
	case OutcomeOwnershipUnresolved:
		return "OWNERSHIP_UNRESOLVED"
	case OutcomeAdmissionPaused:
		return "ADMISSION_PAUSED"
	case OutcomePriorRequestNotFound:
		return "PRIOR_REQUEST_NOT_FOUND"
	case OutcomeLinkIneligible:
		return "LINK_INELIGIBLE"
	default:
		return ""
	}
}

// PriorRequestClaim 指名一份要与之建立关联的原委托（`AT-PS-036`②③、`AT-PS-076`）：
// 原委托的来源身份加委托编号双重指名——与撤回同一模式，编号单独出现无从校验归属——
// 再加关联方向。零值即首次委托，不指名任何出处。
type PriorRequestClaim struct {
	PriorIdentity  domain.SourceIdentity
	PriorRequestID domain.ShipmentRequestID
	Kind           domain.RequestLinkKind
}

func (claim PriorRequestClaim) requested() bool {
	return claim != (PriorRequestClaim{})
}

// complete 校验指名的三件套都在场。用导出访问器判空而不是领域内部的 valid()：本包在
// 领域边界之外，判据只能是「客户根本没把话说全」这一层。
func (claim PriorRequestClaim) complete() bool {
	return claim.PriorIdentity.TenantID().String() != "" &&
		claim.PriorIdentity.CustomerAccountID().String() != "" &&
		claim.PriorIdentity.Source().String() != "" &&
		claim.PriorIdentity.RequestKey().String() != "" &&
		claim.PriorRequestID.String() != "" &&
		claim.Kind.String() != ""
}

type SubmitShipmentRequestCommand struct {
	Identity          domain.SourceIdentity
	PayloadDigest     domain.PayloadDigest
	OccurredAt        time.Time
	ReceivedAt        time.Time
	BatchID           domain.SubmissionBatchID
	ShipmentRequestID domain.ShipmentRequestID
	DeclaredParcelIDs []domain.DeclaredParcelID
	AdmissionScope    domain.AdmissionScope
	ExpectedRevision  domain.ProductionOwnershipRevision
	// Link 缺席即首次委托。指名出处的提交与首次提交共用全部管线（来源保全、归属、
	// 门禁、判重）：关联新委托是一份完整的新委托，不是原委托的续篇。
	Link PriorRequestClaim
	// DeclaredProfiles 随首个提交版本申报的成员声明画像（ADR-0048），允许缺席或部分覆盖。
	DeclaredProfiles []domain.DeclaredParcelProfile
	// DeclaredElements 随首个提交版本申报的寄 / 收两段地址要素（pp-seams/05 裁决 3），与画像同口径允许缺席或只报一段。
	// 它与 PayloadDigest 由接单入口对同一份规范化输入的同一次 CanonicalizeSubmission 产出，本编排不重算、不核对。
	DeclaredElements domain.DeclaredAddressElements
}

// SubmitShipmentRequestResult 携带调用方可以据以行动的内容。委托与归属决定各自可选、
// 通过访问器报告：重放一份从未建过单的输入时没有委托可指名，而接入冲突根本不必问准入
// 权威。
type SubmitShipmentRequestResult struct {
	outcome           SubmitOutcome
	shipmentRequestID domain.ShipmentRequestID
	hasRequest        bool
	ownershipDecision domain.ProductionOwnershipDecision
	hasDecision       bool
	gateBlockReasons  []domain.FutureSubmissionBlockReason
}

func (result SubmitShipmentRequestResult) Outcome() SubmitOutcome {
	return result.outcome
}

func (result SubmitShipmentRequestResult) ShipmentRequestID() (domain.ShipmentRequestID, bool) {
	return result.shipmentRequestID, result.hasRequest
}

func (result SubmitShipmentRequestResult) OwnershipDecision() (domain.ProductionOwnershipDecision, bool) {
	return result.ownershipDecision, result.hasDecision
}

func (result SubmitShipmentRequestResult) GateBlockReasons() []domain.FutureSubmissionBlockReason {
	return append([]domain.FutureSubmissionBlockReason(nil), result.gateBlockReasons...)
}

type SubmitShipmentRequestHandler struct {
	sources    ports.SourceSubmissionRepository
	requests   ports.ShipmentRequestRepository
	ownership  ports.ProductionOwnershipAuthority
	handoff    ports.OtherProductionAuthorityChannel
	identities ports.SubmissionIdentityFactory
	clock      ports.Clock
}

func NewSubmitShipmentRequestHandler(
	sources ports.SourceSubmissionRepository,
	requests ports.ShipmentRequestRepository,
	ownership ports.ProductionOwnershipAuthority,
	handoff ports.OtherProductionAuthorityChannel,
	identities ports.SubmissionIdentityFactory,
	clock ports.Clock,
) *SubmitShipmentRequestHandler {
	return &SubmitShipmentRequestHandler{
		sources:    sources,
		requests:   requests,
		ownership:  ownership,
		handoff:    handoff,
		identities: identities,
		clock:      clock,
	}
}

// Handle 先保全来源，再为完整拟受理范围取得生产归属——判为`其他权威`时把范围交给那个权威并
// 评估交接——之后才建立`已提交`委托。它不形成接受、拒绝、可达性或财务控制结果。
func (handler *SubmitShipmentRequestHandler) Handle(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
) (SubmitShipmentRequestResult, error) {
	incoming, err := domain.NewSourceSubmissionFingerprint(
		command.Identity,
		command.PayloadDigest,
		command.OccurredAt,
		command.ReceivedAt,
	)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("preserve source: %w", err)
	}

	existing, found, err := handler.sources.FindPreserved(ctx, command.Identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find preserved source: %w", err)
	}
	if found {
		return handler.resolvePreserved(ctx, existing, incoming)
	}

	if err := handler.sources.Preserve(ctx, incoming); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("preserve source: %w", err)
	}

	// 用例步骤 3A 先于 3B：无法建立最小委托身份的输入，根本没有可拿去问准入权威的
	// 拟受理范围。
	candidate, err := domain.NewSubmissionCandidate(
		incoming,
		command.BatchID,
		command.ShipmentRequestID,
		command.DeclaredParcelIDs,
	)
	if err != nil {
		return SubmitShipmentRequestResult{outcome: OutcomeInputNotAccepted}, nil
	}

	link, refusal, err := handler.resolvePriorClaim(ctx, command)
	if err != nil {
		return SubmitShipmentRequestResult{}, err
	}
	if refusal != nil {
		return *refusal, nil
	}

	decision, err := handler.ownership.DecideProductionOwnership(ctx, command.AdmissionScope)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("decide production ownership: %w", err)
	}

	// 交接评估、门禁评估与建单共用一次时钟读数，这样委托的提交时刻绝不会落在其门禁所评估的
	// 时刻之外，交接评估的时刻也不会晚于据它形成的门禁。
	decidedAt := handler.clock.Now()
	if decision.Authority() == domain.ProductionAuthorityOther {
		decision, err = handler.handOverToOtherAuthority(ctx, decision, decidedAt)
		if err != nil {
			return SubmitShipmentRequestResult{}, err
		}
	}

	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		command.AdmissionScope.Digest(),
		command.ExpectedRevision,
		decidedAt,
	)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("evaluate future submission gate: %w", err)
	}
	if !gate.IsAllowed() {
		return SubmitShipmentRequestResult{
			outcome:           blockedOutcome(decision),
			ownershipDecision: decision,
			hasDecision:       true,
			gateBlockReasons:  gate.BlockReasons(),
		}, nil
	}

	versionID, err := handler.identities.NextSubmissionVersionID(ctx)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("next submission version ID: %w", err)
	}
	taskID, err := handler.identities.NextAcceptanceDecisionTaskID(ctx)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("next acceptance decision task ID: %w", err)
	}

	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   versionID,
		TaskID:      taskID,
		SubmittedAt: decidedAt,
		Link:        link,
		Profiles:    command.DeclaredProfiles,
		Elements:    command.DeclaredElements,
	})
	if errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		// 画像不贴合成员集合（指着不存在的成员、一员两张、半截测量）与候选立不起来同格：
		// 客户输入的问题，答`输入未受理`让他改请求，不是本方的故障。
		return SubmitShipmentRequestResult{outcome: OutcomeInputNotAccepted}, nil
	}
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("submit shipment request: %w", err)
	}
	inserted, err := handler.requests.Insert(ctx, command.Identity, request)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert shipment request: %w", err)
	}
	switch inserted {
	case ports.ShipmentRequestInserted:
		return SubmitShipmentRequestResult{
			outcome:           OutcomeSubmitted,
			shipmentRequestID: request.ShipmentRequestID(),
			hasRequest:        true,
			ownershipDecision: decision,
			hasDecision:       true,
		}, nil
	case ports.ShipmentRequestAlreadyExists:
		// 并发下另一方先建了单。回去按重放规则重答；本次已经 Preserve 过自己那一份，
		// 不再走 AppendObservation（那条路是给「未保全、只是又看见」用的）。
		return handler.resolveAfterInsertConflict(ctx, command.Identity, incoming)
	default:
		return SubmitShipmentRequestResult{}, ErrUnexpectedInsertOutcome
	}
}

// ErrUnexpectedInsertOutcome 说明建单仓储交回了封闭集合以外的答复。上抛而不译成业务结果：
// 集合外的取值没有恢复动作可派。
var ErrUnexpectedInsertOutcome = errors.New("parcel shipment: unexpected shipment request insert outcome")

// resolvePreserved 回答来源身份已被保全过的请求。它绝不重判归属、也不建第二份委托：
// 原内容不动，调用方拿到原结果。
func (handler *SubmitShipmentRequestHandler) resolvePreserved(
	ctx context.Context,
	existing domain.SourceSubmissionFingerprint,
	incoming domain.SourceSubmissionFingerprint,
) (SubmitShipmentRequestResult, error) {
	classification, err := domain.ClassifySourceSubmission(existing, incoming)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("classify source submission: %w", err)
	}
	if classification == domain.SourceConflict {
		return SubmitShipmentRequestResult{outcome: OutcomeIngressConflict}, nil
	}

	if err := handler.sources.AppendObservation(ctx, incoming); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("append source observation: %w", err)
	}

	result := SubmitShipmentRequestResult{outcome: OutcomeExistingResult}
	request, found, err := handler.requests.FindBySourceIdentity(ctx, existing.Identity())
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find existing shipment request: %w", err)
	}
	if found {
		result.shipmentRequestID = request.ShipmentRequestID()
		result.hasRequest = true
	}
	return result, nil
}

// resolveAfterInsertConflict 回答 Insert 交回「已存在」之后的那一格。与 resolvePreserved 共用
// 分类规则，但不再追加观察：走到这里时本次已经对本份输入做过 Preserve。
func (handler *SubmitShipmentRequestHandler) resolveAfterInsertConflict(
	ctx context.Context,
	identity domain.SourceIdentity,
	incoming domain.SourceSubmissionFingerprint,
) (SubmitShipmentRequestResult, error) {
	existing, found, err := handler.sources.FindPreserved(ctx, identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find preserved source after insert conflict: %w", err)
	}
	if !found {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert reported already exists but preserved source is missing")
	}

	classification, err := domain.ClassifySourceSubmission(existing, incoming)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("classify source submission: %w", err)
	}
	if classification == domain.SourceConflict {
		return SubmitShipmentRequestResult{outcome: OutcomeIngressConflict}, nil
	}

	result := SubmitShipmentRequestResult{outcome: OutcomeExistingResult}
	request, found, err := handler.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("find existing shipment request: %w", err)
	}
	if found {
		result.shipmentRequestID = request.ShipmentRequestID()
		result.hasRequest = true
	}
	return result, nil
}

// resolvePriorClaim 把关联指名换成已互证的关联出处（`AT-PS-036`②③、`AT-PS-076`）。
//
// 它在归属判定之前跑：指名立不住的关联没有必要消耗一次准入决定。三层裁决各归各格——
//
//  1. 话没说全（三件套缺项、指名自己）→ 输入未受理；
//  2. 统一不可见：跨租户或跨客户指名**不查库直接**答`查无原委托`，与真的查无、编号
//     不符同一个答案——可区分即可枚举别人的委托；
//  3. 方向与原委托终态不符 → 关联不适用，由领域互证裁定。待决委托没有方向可用，
//     普通纠错走同一委托的新提交版本。
//
// 原委托全程只读：出处落在新委托身上，原版本与原决定一概不动。
func (handler *SubmitShipmentRequestHandler) resolvePriorClaim(
	ctx context.Context,
	command SubmitShipmentRequestCommand,
) (domain.PriorRequestLink, *SubmitShipmentRequestResult, error) {
	claim := command.Link
	if !claim.requested() {
		return domain.PriorRequestLink{}, nil, nil
	}
	if !claim.complete() || claim.PriorRequestID == command.ShipmentRequestID {
		return domain.PriorRequestLink{}, &SubmitShipmentRequestResult{outcome: OutcomeInputNotAccepted}, nil
	}
	if claim.PriorIdentity.TenantID() != command.Identity.TenantID() ||
		claim.PriorIdentity.CustomerAccountID() != command.Identity.CustomerAccountID() {
		return domain.PriorRequestLink{}, &SubmitShipmentRequestResult{outcome: OutcomePriorRequestNotFound}, nil
	}

	prior, found, err := handler.requests.FindBySourceIdentity(ctx, claim.PriorIdentity)
	if err != nil {
		return domain.PriorRequestLink{}, nil, fmt.Errorf("find prior shipment request: %w", err)
	}
	if !found || prior.ShipmentRequestID() != claim.PriorRequestID {
		return domain.PriorRequestLink{}, &SubmitShipmentRequestResult{outcome: OutcomePriorRequestNotFound}, nil
	}

	link, err := domain.EstablishPriorRequestLink(prior, claim.Kind)
	if errors.Is(err, domain.ErrPriorStateIncompatibleWithLink) {
		return domain.PriorRequestLink{}, &SubmitShipmentRequestResult{outcome: OutcomeLinkIneligible}, nil
	}
	if err != nil {
		return domain.PriorRequestLink{}, nil, fmt.Errorf("establish prior request link: %w", err)
	}
	return link, nil, nil
}

// handOverToOtherAuthority 是 UC-PS-001 步骤 3B「其他权威时安全交接」那一步（ADR-0128 决定二）：
// 归属已凭治理接管记录判为`其他权威`之后，把完整拟受理范围投递给那个权威、观察确认、经
// AssessSafeHandoff 形成评估并记到决定上。先后固定——归属先定、交接后验：投递是对外动作、不可撤，
// 只在归属已定时做；反过来拿一次应答当归属证据，会让没有停写证据的一方凭应答成为权威。
//
// 尝试身份与续办引用都由决定派生而不签发：本上下文不持久化交接尝试，同一份决定重复走到这里必须
// 得到同一次尝试，对方才能据以认领重放。续办引用不看观察结果就形成——它说的是「从这一次尝试续办」，
// 原因由评估自己带；按观察结果决定给不给，等于在这里复刻一遍领域的判定条件。
//
// 出向通道自身失败原样上抛，不折成任何一格观察：那些格说的是对方的答复，不是本方的故障。
func (handler *SubmitShipmentRequestHandler) handOverToOtherAuthority(
	ctx context.Context,
	decision domain.ProductionOwnershipDecision,
	assessedAt time.Time,
) (domain.ProductionOwnershipDecision, error) {
	target, present := decision.OtherAuthorityReference()
	if !present {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("hand over to other authority: decision names no other authority")
	}
	attemptID, err := domain.NewHandoffAttemptID("PS-HANDOFF/" + decision.DecisionID().String())
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("hand over to other authority: attempt ID: %w", err)
	}
	continuationRef, err := domain.NewOwnershipContinuationReference("CONT-PS-HANDOFF/" + attemptID.String())
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("hand over to other authority: continuation reference: %w", err)
	}

	observed, err := handler.handoff.DeliverAdmissionScope(ctx, ports.ProductionHandoffDelivery{
		AttemptID:       attemptID,
		Scope:           decision.Scope(),
		TargetAuthority: target,
	})
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("deliver admission scope to other authority: %w", err)
	}

	assessment, err := domain.AssessSafeHandoff(domain.SafeHandoffAssessmentSpec{
		AttemptID:            attemptID,
		Scope:                decision.Scope(),
		TargetAuthority:      target,
		Observation:          observed.Observation,
		ConfirmedScopeDigest: observed.ConfirmedScopeDigest,
		ConfirmationRef:      observed.ConfirmationRef,
		QueryRef:             observed.QueryRef,
		ContinuationRef:      continuationRef,
		EffectiveAt:          observed.EffectiveAt,
		AssessedAt:           assessedAt,
	})
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("assess safe handoff: %w", err)
	}
	withHandoff, err := decision.WithSafeHandoff(assessment)
	if err != nil {
		return domain.ProductionOwnershipDecision{}, fmt.Errorf("record safe handoff on ownership decision: %w", err)
	}
	return withHandoff, nil
}

// blockedOutcome 让三种拒绝各自成立。其他权威承接该范围、权威无法确定、以及本产品暂停
// 新准入，是对不同问题的不同回答；其中暂停既不是第四种权威身份，也不是客户业务拒绝。
//
// `其他权威`只在交接评估**已确认**时才是「非本产品归属结束」（ADR-0128 决定三）：接管记录与交接
// 确认是两种证据，缺一不许交。评估未决（哪几种算未决、各自的证据形由 domain.HandoffObservation 与
// domain.HandoffUnresolvedReason 定，这里不复述）——归属决定仍是`其他权威`，结果却落「生产归属未决」，
// 续办引用由评估带出。没记评估的`其他权威`决定
// 走不到这里（编排在 Other 分支一律先交接），但若走到，也只能按未决答：宣布交出去了而没有确认，
// 与没有停写证据一样是替对方宣布交接完成。
func blockedOutcome(decision domain.ProductionOwnershipDecision) SubmitOutcome {
	switch decision.Authority() {
	case domain.ProductionAuthorityOther:
		if assessment, recorded := decision.SafeHandoff(); recorded && assessment.Status() == domain.SafeHandoffConfirmed {
			return OutcomeOtherProductionAuthority
		}
		return OutcomeOwnershipUnresolved
	case domain.ProductionAuthorityUnresolved:
		return OutcomeOwnershipUnresolved
	}
	if decision.AdmissionControl() == domain.AdmissionControlPaused {
		return OutcomeAdmissionPaused
	}
	return OutcomeOwnershipUnresolved
}
