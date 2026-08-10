// Package application 在领域内核与 parcel-shipment 自有端口之上编排本上下文的用例。
// 它不含任何持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
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
	default:
		return ""
	}
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
	identities ports.SubmissionIdentityFactory
	clock      ports.Clock
}

func NewSubmitShipmentRequestHandler(
	sources ports.SourceSubmissionRepository,
	requests ports.ShipmentRequestRepository,
	ownership ports.ProductionOwnershipAuthority,
	identities ports.SubmissionIdentityFactory,
	clock ports.Clock,
) *SubmitShipmentRequestHandler {
	return &SubmitShipmentRequestHandler{
		sources:    sources,
		requests:   requests,
		ownership:  ownership,
		identities: identities,
		clock:      clock,
	}
}

// Handle 先保全来源，再为完整拟受理范围取得生产归属，之后才建立`已提交`委托。它不形成
// 接受、拒绝、可达性或财务控制结果。
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

	decision, err := handler.ownership.DecideProductionOwnership(ctx, command.AdmissionScope)
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("decide production ownership: %w", err)
	}

	// 门禁评估与建单共用一次时钟读数，这样委托的提交时刻绝不会落在其门禁所评估的
	// 时刻之外。
	decidedAt := handler.clock.Now()
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
	})
	if err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("submit shipment request: %w", err)
	}
	if err := handler.requests.Insert(ctx, command.Identity, request); err != nil {
		return SubmitShipmentRequestResult{}, fmt.Errorf("insert shipment request: %w", err)
	}

	return SubmitShipmentRequestResult{
		outcome:           OutcomeSubmitted,
		shipmentRequestID: request.ShipmentRequestID(),
		hasRequest:        true,
		ownershipDecision: decision,
		hasDecision:       true,
	}, nil
}

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

// blockedOutcome 让三种拒绝各自成立。其他权威承接该范围、权威无法确定、以及本产品暂停
// 新准入，是对不同问题的不同回答；其中暂停既不是第四种权威身份，也不是客户业务拒绝。
func blockedOutcome(decision domain.ProductionOwnershipDecision) SubmitOutcome {
	switch decision.Authority() {
	case domain.ProductionAuthorityOther:
		return OutcomeOtherProductionAuthority
	case domain.ProductionAuthorityUnresolved:
		return OutcomeOwnershipUnresolved
	}
	if decision.AdmissionControl() == domain.AdmissionControlPaused {
		return OutcomeAdmissionPaused
	}
	return OutcomeOwnershipUnresolved
}
