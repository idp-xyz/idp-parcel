package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CommercialBasisOutcome reports whether a unique commercial basis was adopted.
// parcel-shipment deliberately does not mirror party-commercial's full result
// algebra: the distinction between no-basis, conflict and pending belongs to
// that context, and copying it here would create a second place to maintain it.
type CommercialBasisOutcome uint8

const (
	CommercialBasisOutcomeInvalid CommercialBasisOutcome = iota
	CommercialBasisUnique
	CommercialBasisNotApplicable
	CommercialBasisConflict
	CommercialBasisPending
)

// AcceptanceJudgmentOutcome is an application processing result for one pass of
// the acceptance decision task. Neither value is acceptance or rejection: the
// task stays continuable and the request stays submitted.
type AcceptanceJudgmentOutcome uint8

const (
	AcceptanceJudgmentOutcomeInvalid AcceptanceJudgmentOutcome = iota
	AcceptanceJudgmentAdvanced
	AcceptanceJudgmentUndecided
)

func (outcome AcceptanceJudgmentOutcome) String() string {
	switch outcome {
	case AcceptanceJudgmentAdvanced:
		return "ADVANCED"
	case AcceptanceJudgmentUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type AdvanceAcceptanceJudgmentCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
}

type AdvanceAcceptanceJudgmentResult struct {
	outcome      AcceptanceJudgmentOutcome
	judgment     domain.ReachabilityJudgment
	hasJudgment  bool
	continuation domain.OwnershipContinuationReference
}

func (result AdvanceAcceptanceJudgmentResult) Outcome() AcceptanceJudgmentOutcome {
	return result.outcome
}

func (result AdvanceAcceptanceJudgmentResult) ReachabilityJudgment() (domain.ReachabilityJudgment, bool) {
	return result.judgment, result.hasJudgment
}

func (result AdvanceAcceptanceJudgmentResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// State reports the request's lifecycle state after this pass. It is always
// submitted: an acceptance decision task advancing — however the judgement came
// out — is not an acceptance or a rejection.
func (result AdvanceAcceptanceJudgmentResult) State() domain.ShipmentRequestState {
	return domain.ShipmentRequestSubmitted
}

type AdvanceAcceptanceJudgmentHandler struct {
	commercial   ports.CommercialBasisResolver
	reachability ports.ReachabilityAssessor
	recorder     ports.AcceptanceJudgmentRecorder
	clock        ports.Clock
}

func NewAdvanceAcceptanceJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	reachability ports.ReachabilityAssessor,
	recorder ports.AcceptanceJudgmentRecorder,
	clock ports.Clock,
) *AdvanceAcceptanceJudgmentHandler {
	return &AdvanceAcceptanceJudgmentHandler{
		commercial:   commercial,
		reachability: reachability,
		recorder:     recorder,
		clock:        clock,
	}
}

// Handle advances one acceptance decision task by one step: adopt a unique
// commercial basis, form the reachability anchor that basis declares, and record
// the judgement the routing authority returns.
//
// It never forms acceptance or rejection, and it never substitutes its own clock
// for a declared anchor. Without a unique basis, or without a declared anchor for
// this judgement, it stops and stays continuable rather than proceeding on an
// instant nobody authorised.
func (handler *AdvanceAcceptanceJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
) (AdvanceAcceptanceJudgmentResult, error) {
	basis, err := handler.commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("resolve commercial basis: %w", err)
	}
	if basis.ResolutionID().String() == "" {
		return handler.undecided(command, "COMMERCIAL_BASIS_NOT_UNIQUE"), nil
	}

	asOf, declared := basis.AsOfFor(domain.ReachabilityJudgmentKind)
	if !declared {
		return handler.undecided(command, "REACHABILITY_AS_OF_NOT_DECLARED"), nil
	}

	judgment, err := handler.reachability.AssessParcelReachability(ctx, ports.ReachabilityRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		DeclaredParcelID:  command.DeclaredParcelID,
		AsOf:              asOf,
	})
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("assess parcel reachability: %w", err)
	}
	if err := handler.recorder.RecordReachabilityJudgment(ctx, command.ShipmentRequestID, judgment); err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("record reachability judgment: %w", err)
	}

	return AdvanceAcceptanceJudgmentResult{
		outcome:     AcceptanceJudgmentAdvanced,
		judgment:    judgment,
		hasJudgment: true,
	}, nil
}

func (handler *AdvanceAcceptanceJudgmentHandler) undecided(
	command AdvanceAcceptanceJudgmentCommand,
	reason string,
) AdvanceAcceptanceJudgmentResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
		command.DeclaredParcelID.String(),
	}, "\x00")))
	continuation, err := domain.NewOwnershipContinuationReference("CONT-" + hex.EncodeToString(digest[:8]))
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{outcome: AcceptanceJudgmentUndecided}
	}
	return AdvanceAcceptanceJudgmentResult{
		outcome:      AcceptanceJudgmentUndecided,
		continuation: continuation,
	}
}
