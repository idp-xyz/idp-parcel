// Package application orchestrates the parcel shipment use cases. It depends on
// the domain, on parcel owned ports and on the minimal framework contracts; it
// never depends on HTTP, SQL or a driver type.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// EventSource is the authoritative source of parcel shipment integration
// events.
const EventSource = "go.idp.xyz/idp-parcel/parcel-shipment"

// SubmitResult is the application outcome of one intake call. It is not a
// business decision: this slice never produces accepted or rejected.
type SubmitResult string

const (
	// ResultSubmitted means at least one shipment request reached Submitted.
	ResultSubmitted SubmitResult = "SUBMITTED"
	// ResultExisting means the call was recognised as a repeat of an already
	// preserved logical request and the original result is returned.
	ResultExisting SubmitResult = "EXISTING"
	// ResultIntakeConflict means the same logical request identity arrived with
	// different content. Nothing is overwritten and no second request is built.
	ResultIntakeConflict SubmitResult = "INTAKE_CONFLICT"
	// ResultNotAdmitted means no minimal shipment request identity could be
	// established. No placeholder request and no rejection decision is created.
	ResultNotAdmitted SubmitResult = "NOT_ADMITTED"
	// ResultUndecided means this attempt formed no business decision and the
	// caller can safely resume through the stable request correlation.
	ResultUndecided SubmitResult = "UNDECIDED"
)

// DeclaredShipmentRequest is one shipment request boundary the intake adapter
// resolved from the raw content.
type DeclaredShipmentRequest struct {
	ShipmentRequestID domain.ShipmentRequestID
	DeclaredParcels   []domain.DeclaredParcel
}

// SubmitCommand is the channel neutral submission command. Every scope value is
// explicit; nothing is completed from a context or a request header.
type SubmitCommand struct {
	SourceKey         domain.SourceKey
	RawContentRef     string
	PayloadDigest     domain.PayloadDigest
	SubmissionBatchID domain.SubmissionBatchID
	Requests          []DeclaredShipmentRequest
	SourceOccurredAt  time.Time
	SystemReceivedAt  time.Time
	CorrelationID     string
	ParseRuleVersion  string

	// NotAdmittedReason is set by the intake adapter when it could not
	// establish a customer scope, a request boundary or a minimal member
	// identity. It is a deterministic parse error, never a business rejection.
	NotAdmittedReason string
}

// RequestResult is the per shipment request outcome of one submission.
type RequestResult struct {
	ShipmentRequestID domain.ShipmentRequestID
	State             domain.LifecycleState
	SubmissionVersion domain.SubmissionVersion
	EventID           string
	Outcome           domain.RequestOutcome
	// Undecided marks a request whose own transaction did not commit. A sibling
	// request that legitimately reached Submitted is not rolled back with it.
	Undecided bool
	Reason    string
}

// SubmitResponse is the intake response. It carries the stable correlation the
// caller uses to look the original result up instead of resubmitting.
type SubmitResponse struct {
	Result            SubmitResult
	SourceKey         domain.SourceKey
	SubmissionBatchID domain.SubmissionBatchID
	Requests          []RequestResult
	Reason            string
}

// SubmitHandler implements `PS-W1-S1`: preserve the source, then submit each
// shipment request in its own transaction together with its outbox intent.
type SubmitHandler struct {
	transactor bentoapp.Transactor
	sources    ports.SourceSubmissionRepository
	requests   ports.ShipmentRequestRepository
	batches    ports.SubmissionBatchRepository
	outbox     eventing.OutboxWriter
	eventIDs   bentoapp.IDGenerator[string]
	clock      bentoapp.Clock
}

// NewSubmitHandler assembles the use case. Every collaborator is required.
func NewSubmitHandler(
	transactor bentoapp.Transactor,
	sources ports.SourceSubmissionRepository,
	requests ports.ShipmentRequestRepository,
	batches ports.SubmissionBatchRepository,
	outbox eventing.OutboxWriter,
	eventIDs bentoapp.IDGenerator[string],
	clock bentoapp.Clock,
) (*SubmitHandler, error) {
	if transactor == nil || sources == nil || requests == nil ||
		batches == nil || outbox == nil || eventIDs == nil || clock == nil {
		return nil, errors.New("parcelshipment: submit handler requires all collaborators")
	}
	return &SubmitHandler{
		transactor: transactor,
		sources:    sources,
		requests:   requests,
		batches:    batches,
		outbox:     outbox,
		eventIDs:   eventIDs,
		clock:      clock,
	}, nil
}

// Handle runs the source preservation transaction and then one independent
// transaction per shipment request.
func (h *SubmitHandler) Handle(ctx context.Context, command SubmitCommand) (SubmitResponse, error) {
	if err := command.SourceKey.Validate(); err != nil {
		return SubmitResponse{}, err
	}

	preserved, existing, err := h.preserveSource(ctx, command)
	if err != nil {
		return SubmitResponse{}, err
	}
	if existing != nil {
		return *existing, nil
	}

	if command.NotAdmittedReason != "" {
		return h.recordNotAdmitted(ctx, command, preserved)
	}
	if len(command.Requests) == 0 {
		return SubmitResponse{}, domain.ErrShipmentRequestIDMissing
	}

	// The batch grouping exists before the first request so each request can
	// reference it. Grouping is not a decision and owns no service
	// responsibility.
	if err := h.ensureBatch(ctx, command); err != nil {
		return SubmitResponse{}, err
	}

	results := make([]RequestResult, 0, len(command.Requests))
	submitted := make([]domain.ShipmentRequestID, 0, len(command.Requests))
	for _, declared := range command.Requests {
		result := h.submitOne(ctx, command, declared)
		results = append(results, result)
		if !result.Undecided {
			submitted = append(submitted, result.ShipmentRequestID)
		}
	}

	if err := h.recordBatch(ctx, command, results); err != nil {
		return SubmitResponse{}, err
	}
	if len(submitted) > 0 {
		if err := h.linkPreservedSource(ctx, preserved, command, submitted); err != nil {
			return SubmitResponse{}, err
		}
	}

	response := SubmitResponse{
		Result:            ResultSubmitted,
		SourceKey:         command.SourceKey,
		SubmissionBatchID: command.SubmissionBatchID,
		Requests:          results,
	}
	if len(submitted) == 0 {
		response.Result = ResultUndecided
	}
	return response, nil
}

// preserveSource runs the first transaction. It returns the preserved record
// for a first submission, or a response for a repeat or a conflict.
func (h *SubmitHandler) preserveSource(
	ctx context.Context,
	command SubmitCommand,
) (*domain.SourceSubmission, *SubmitResponse, error) {
	submission, err := domain.PreserveSource(
		command.SourceKey,
		command.RawContentRef,
		command.PayloadDigest,
		command.SourceOccurredAt,
		command.SystemReceivedAt,
		command.CorrelationID,
	)
	if err != nil {
		return nil, nil, err
	}

	err = h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return h.sources.Preserve(txCtx, submission)
	})
	switch {
	case err == nil:
		return submission, nil, nil
	case errors.Is(err, ports.ErrAlreadyExists):
		response, err := h.resolveRepeat(ctx, command)
		if err != nil {
			return nil, nil, err
		}
		return nil, &response, nil
	default:
		return nil, nil, fmt.Errorf("preserve source submission: %w", err)
	}
}

// resolveRepeat answers a repeated intake call. The same digest returns the
// original result; a different digest is a conflict that preserves the original
// content and never builds a second request.
func (h *SubmitHandler) resolveRepeat(ctx context.Context, command SubmitCommand) (SubmitResponse, error) {
	original, err := h.sources.Find(ctx, command.SourceKey)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("read preserved submission: %w", err)
	}

	if !original.SameContent(command.PayloadDigest) {
		return SubmitResponse{
			Result:    ResultIntakeConflict,
			SourceKey: command.SourceKey,
			Reason:    "payload digest differs from the preserved submission",
		}, nil
	}

	if original.Outcome() == domain.IntakeNotAdmitted {
		return SubmitResponse{
			Result:    ResultNotAdmitted,
			SourceKey: command.SourceKey,
			Reason:    original.NotAdmittedReason(),
		}, nil
	}

	response := SubmitResponse{
		Result:            ResultExisting,
		SourceKey:         command.SourceKey,
		SubmissionBatchID: original.SubmissionBatchID(),
	}
	for _, id := range original.ShipmentRequestIDs() {
		request, _, err := h.requests.Load(ctx, domain.ShipmentRequestKey{
			Scope:             command.SourceKey.Scope,
			ShipmentRequestID: id,
		})
		if err != nil {
			return SubmitResponse{}, fmt.Errorf("read existing shipment request: %w", err)
		}
		response.Requests = append(response.Requests, RequestResult{
			ShipmentRequestID: id,
			State:             request.State(),
			SubmissionVersion: request.SubmissionVersion(),
			EventID:           request.EventID(),
			Outcome:           domain.OutcomeSubmitted,
		})
	}
	return response, nil
}

func (h *SubmitHandler) recordNotAdmitted(
	ctx context.Context,
	command SubmitCommand,
	preserved *domain.SourceSubmission,
) (SubmitResponse, error) {
	if err := preserved.RecordNotAdmitted(command.NotAdmittedReason, command.ParseRuleVersion); err != nil {
		return SubmitResponse{}, err
	}
	err := h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return h.sources.RecordOutcome(txCtx, preserved)
	})
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("record not admitted intake: %w", err)
	}
	return SubmitResponse{
		Result:    ResultNotAdmitted,
		SourceKey: command.SourceKey,
		Reason:    command.NotAdmittedReason,
	}, nil
}

// submitOne commits one shipment request together with its outbox intent. A
// failure here leaves the sibling requests of the same batch untouched.
func (h *SubmitHandler) submitOne(
	ctx context.Context,
	command SubmitCommand,
	declared DeclaredShipmentRequest,
) RequestResult {
	eventID, err := h.eventIDs.NewID(ctx)
	if err != nil {
		return undecided(declared.ShipmentRequestID, "event id unavailable")
	}

	request, err := domain.SubmitShipmentRequest(
		domain.ShipmentRequestKey{
			Scope:             command.SourceKey.Scope,
			ShipmentRequestID: declared.ShipmentRequestID,
		},
		command.SubmissionBatchID,
		command.SourceKey,
		declared.DeclaredParcels,
		eventID,
		command.SourceOccurredAt,
		h.clock.Now(),
	)
	if err != nil {
		return undecided(declared.ShipmentRequestID, err.Error())
	}

	envelope, err := h.envelopeFor(request)
	if err != nil {
		return undecided(declared.ShipmentRequestID, err.Error())
	}

	err = h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := h.requests.Insert(txCtx, request); err != nil {
			return err
		}
		return h.outbox.Enqueue(txCtx, envelope)
	})
	if err != nil {
		// A commit whose outcome is unknown must not be replayed blindly. The
		// caller looks the original result up through the stable correlation.
		if errors.Is(err, bentoapp.ErrCommitUncertain) {
			return undecided(declared.ShipmentRequestID, "commit outcome uncertain")
		}
		return undecided(declared.ShipmentRequestID, err.Error())
	}

	request.Events().Acknowledge()
	return RequestResult{
		ShipmentRequestID: declared.ShipmentRequestID,
		State:             request.State(),
		SubmissionVersion: request.SubmissionVersion(),
		EventID:           eventID,
		Outcome:           domain.OutcomeSubmitted,
	}
}

// submittedPayload is the minimal event payload. It carries identities and
// versions only: no address, contact, goods or declaration content.
type submittedPayload struct {
	ShipmentRequestID string   `json:"shipmentRequestId"`
	SubmissionBatchID string   `json:"submissionBatchId"`
	SubmissionVersion uint32   `json:"submissionVersion"`
	DeclaredParcelIDs []string `json:"declaredParcelIds"`
	Source            string   `json:"source"`
	SourceRequestKey  string   `json:"sourceRequestKey"`
}

func (h *SubmitHandler) envelopeFor(request *domain.ShipmentRequest) (eventing.Envelope, error) {
	parcelIDs := make([]string, 0, len(request.DeclaredParcelIDs()))
	for _, id := range request.DeclaredParcelIDs() {
		parcelIDs = append(parcelIDs, string(id))
	}

	payload, err := json.Marshal(submittedPayload{
		ShipmentRequestID: string(request.Key().ShipmentRequestID),
		SubmissionBatchID: string(request.SubmissionBatchID()),
		SubmissionVersion: uint32(request.SubmissionVersion()),
		DeclaredParcelIDs: parcelIDs,
		Source:            string(request.SourceKey().Source),
		SourceRequestKey:  string(request.SourceKey().SourceRequestKey),
	})
	if err != nil {
		return eventing.Envelope{}, fmt.Errorf("encode submitted payload: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersionV1,
		ID:          eventing.EventID(request.EventID()),
		Source:      EventSource,
		Type:        eventing.EventType(domain.EventTypeShipmentRequestSubmitted),
		Version:     1,
		// Parcel writes its authoritative scope itself; the framework never
		// completes it from a context.
		Scope:        request.Key().Scope.String(),
		Subject:      string(request.Key().ShipmentRequestID),
		PartitionKey: request.PartitionKey(),
		// The domain occurrence time is the source occurrence time; the record
		// time is when Parcel wrote it.
		OccurredAt:    request.SourceOccurredAt(),
		RecordedAt:    request.SubmittedAt(),
		CorrelationID: string(request.SourceKey().SourceRequestKey),
		ContentType:   "application/json",
		Payload:       payload,
	}
	if err := envelope.Validate(); err != nil {
		return eventing.Envelope{}, err
	}
	return envelope, nil
}

func (h *SubmitHandler) ensureBatch(ctx context.Context, command SubmitCommand) error {
	err := h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return h.batches.EnsureBatch(
			txCtx,
			command.SourceKey.Scope,
			command.SubmissionBatchID,
			command.SourceKey,
			h.clock.Now(),
		)
	})
	if err != nil {
		return fmt.Errorf("ensure submission batch: %w", err)
	}
	return nil
}

func (h *SubmitHandler) recordBatch(ctx context.Context, command SubmitCommand, results []RequestResult) error {
	refs := make([]domain.BatchRequestRef, 0, len(results))
	for _, result := range results {
		if result.Undecided {
			continue
		}
		refs = append(refs, domain.BatchRequestRef{
			ShipmentRequestID: result.ShipmentRequestID,
			Outcome:           result.Outcome,
		})
	}
	if len(refs) == 0 {
		return nil
	}

	batch := domain.BatchResult{
		Scope:      command.SourceKey.Scope,
		BatchID:    command.SubmissionBatchID,
		SourceKey:  command.SourceKey,
		Requests:   refs,
		RecordedAt: h.clock.Now(),
	}
	if err := batch.Validate(); err != nil {
		return err
	}
	return h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return h.batches.Record(txCtx, batch)
	})
}

func (h *SubmitHandler) linkPreservedSource(
	ctx context.Context,
	preserved *domain.SourceSubmission,
	command SubmitCommand,
	submitted []domain.ShipmentRequestID,
) error {
	if err := preserved.RecordAdmitted(command.SubmissionBatchID, submitted, command.ParseRuleVersion); err != nil {
		return err
	}
	return h.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return h.sources.RecordOutcome(txCtx, preserved)
	})
}

func undecided(id domain.ShipmentRequestID, reason string) RequestResult {
	return RequestResult{
		ShipmentRequestID: id,
		Undecided:         true,
		Reason:            reason,
	}
}
