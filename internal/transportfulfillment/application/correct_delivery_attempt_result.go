package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CorrectDeliveryAttemptResultCommand 携带一次派送结果更正的全部输入：指名被更正的那一格（尝试身份加对象）、更正成的确定
// 结果、所凭的后补来源（迟到的签收、复核后认定的地址……）与两个业务时刻。DecidedBy 同揽收一侧。
//
// 更正成妥投不带 POD：妥投的证据走交付登记进有效交付，更正只让那一格不再是待确认，交付登记于是走得过去。
type CorrectDeliveryAttemptResultCommand struct {
	TenantID    domain.TenantID
	Attempt     string
	Object      string
	Outcome     domain.DeliveryObjectOutcome
	Basis       domain.AttemptResultBasisReference
	Source      domain.AttemptResultSourceReference
	OccurredAt  time.Time
	CorrectedAt time.Time

	DecidedBy string
}

type CorrectDeliveryAttemptResultResult struct {
	outcome      AttemptResultCorrectionOutcome
	reason       AttemptResultCorrectionUndecidedReason
	record       ports.DeliveryResultCorrectionRecord
	hasRecord    bool
	continuation string
}

func (result CorrectDeliveryAttemptResultResult) Outcome() AttemptResultCorrectionOutcome {
	return result.outcome
}

func (result CorrectDeliveryAttemptResultResult) UndecidedReason() AttemptResultCorrectionUndecidedReason {
	return result.reason
}

// Record 交回在册的更正（成立或重放时在）。
func (result CorrectDeliveryAttemptResultResult) Record() (ports.DeliveryResultCorrectionRecord, bool) {
	return result.record, result.hasRecord
}

func (result CorrectDeliveryAttemptResultResult) ContinuationReference() string {
	return result.continuation
}

type CorrectDeliveryAttemptResultDeps struct {
	Corrections ports.DeliveryResultCorrections
	Clock       ports.Clock
}

type CorrectDeliveryAttemptResultHandler struct {
	deps CorrectDeliveryAttemptResultDeps
}

func NewCorrectDeliveryAttemptResultHandler(deps CorrectDeliveryAttemptResultDeps) *CorrectDeliveryAttemptResultHandler {
	return &CorrectDeliveryAttemptResultHandler{deps: deps}
}

// Correct 把一格待确认的派送结果更正成确定结果：受理 → 锁住并读回那一格 → 已有更正按内容作答 → 领域更正 → 追加更正。
// 必须在调用方的事务里跑（加锁与追加同一笔）；成立以外的格什么都不写。
func (handler *CorrectDeliveryAttemptResultHandler) Correct(
	ctx context.Context,
	command CorrectDeliveryAttemptResultCommand,
) (CorrectDeliveryAttemptResultResult, error) {
	notAccepted := CorrectDeliveryAttemptResultResult{outcome: AttemptResultCorrectionNotAccepted}
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		command.Outcome == domain.DeliveryObjectOutcomeInvalid || command.Outcome.PendingConfirmation() ||
		command.Source.String() == "" {
		return notAccepted, nil
	}
	attemptRef, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return notAccepted, nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return notAccepted, nil
	}
	var decidedBy domain.RegistrantReference
	if command.DecidedBy != "" {
		if decidedBy, err = domain.NewRegistrantReference(command.DecidedBy); err != nil {
			return notAccepted, nil
		}
	}

	key := ports.DeliveryAttemptKey{TenantID: command.TenantID, Attempt: attemptRef}
	target, found, err := handler.deps.Corrections.LoadForCorrection(ctx, key, object)
	if err != nil {
		return CorrectDeliveryAttemptResultResult{
			outcome: AttemptResultCorrectionUndecided, reason: AttemptResultCorrectionStoreUnavailable,
			continuation: attemptResultCorrectionContinuation("DELIVERY_ATTEMPT_RESULT_CORRECTION",
				command.TenantID.String(), attemptRef.String(), object.String()),
		}, nil
	}
	if !found {
		return notAccepted, nil
	}
	if target.HasCorrection {
		return answerExistingDeliveryCorrection(target.Correction, command), nil
	}

	correction, err := domain.CorrectPendingDeliveryAttemptResult(
		target.Attempt, target.Result, command.Outcome, command.Basis, command.Source, command.OccurredAt, command.CorrectedAt)
	switch {
	case err == nil:
	case isNotPending(err):
		return CorrectDeliveryAttemptResultResult{outcome: AttemptResultNotPending}, nil
	default:
		return notAccepted, nil
	}
	record := ports.DeliveryResultCorrectionRecord{
		Key:        key,
		Correction: correction,
		DecidedBy:  decidedBy,
		RecordedAt: handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Corrections.Save(ctx, record)
	if err != nil {
		return CorrectDeliveryAttemptResultResult{
			outcome: AttemptResultCorrectionUndecided, reason: AttemptResultCorrectionStoreUnavailable,
			continuation: attemptResultCorrectionContinuation("DELIVERY_ATTEMPT_RESULT_CORRECTION",
				command.TenantID.String(), attemptRef.String(), object.String()),
		}, nil
	}
	if saved != ports.DeliveryResultCorrectionSaved {
		return CorrectDeliveryAttemptResultResult{}, fmt.Errorf("delivery result correction was recorded under the lock by another writer: %d", saved)
	}
	return CorrectDeliveryAttemptResultResult{outcome: AttemptResultCorrected, record: record, hasRecord: true}, nil
}

// answerExistingDeliveryCorrection 按已落下的更正作答：说的同一件事是重放，说的另一件事是冲突，在册的不被顶替。
func answerExistingDeliveryCorrection(
	stored ports.DeliveryResultCorrectionRecord,
	command CorrectDeliveryAttemptResultCommand,
) CorrectDeliveryAttemptResultResult {
	effective := stored.Correction.Effective()
	storedBasis := ""
	if value, present := effective.Basis(); present {
		storedBasis = value.String()
	}
	same := correctionContent(effective.Outcome().String(), storedBasis, stored.Correction.Source().String(), "",
		effective.OccurredAt(), stored.Correction.CorrectedAt()) ==
		correctionContent(command.Outcome.String(), command.Basis.String(), command.Source.String(), "",
			command.OccurredAt, command.CorrectedAt)
	if !same {
		return CorrectDeliveryAttemptResultResult{outcome: AttemptResultCorrectionConflict}
	}
	return CorrectDeliveryAttemptResultResult{outcome: AttemptResultCorrectionExisting, record: stored, hasRecord: true}
}
