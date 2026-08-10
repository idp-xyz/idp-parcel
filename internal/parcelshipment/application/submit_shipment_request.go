// Package application orchestrates parcel-shipment use cases over the domain
// kernel and the ports parcel-shipment owns. It holds no persistence,
// transaction or event mechanism; those stay behind the Bento gate (ADR-0017).
package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// SubmitOutcome is an application processing result, not a request lifecycle
// state. Only OutcomeSubmitted establishes a request, and none of these values
// means the request was accepted or rejected.
type SubmitOutcome uint8

const (
	OutcomeInvalid SubmitOutcome = iota
	OutcomeSubmitted
	OutcomeExistingResult
	OutcomeIngressConflict
	OutcomeInputNotAccepted
	OutcomeOtherProductionAuthority
	OutcomeOwnershipUnresolved
	// OutcomeAdmissionPaused is separate from OutcomeOwnershipUnresolved
	// because a pause answers whether this product currently admits new work,
	// not who owns the scope. Merging them would lose the fact that authority
	// was established.
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

// SubmitShipmentRequestResult carries what the caller may act on. The request
// and the ownership decision are each optional and reported through accessors:
// a replay of input that never built a request has no request to name, and an
// ingress conflict is answered without consulting the admission authority.
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

// Handle preserves the source, resolves production ownership for the whole
// admission scope, and only then establishes a submitted request. It forms no
// acceptance, rejection, reachability or financial control result.
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

	// Step 3A precedes 3B: input that cannot establish a minimum request
	// identity has no admission scope to ask the authority about.
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

	// One clock reading covers the gate and the submission, so a request can
	// never be stamped outside the instant its gate was evaluated for.
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

// resolvePreserved answers a request whose source identity was already
// preserved. It never re-decides ownership or builds a second request: the
// original content stands and the caller receives the original result.
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

// blockedOutcome keeps the three refusals distinct. Another authority owning the
// scope, an authority that could not be established, and this product pausing
// new admissions are different answers to different questions; a pause in
// particular is neither a fourth authority nor a customer rejection.
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
