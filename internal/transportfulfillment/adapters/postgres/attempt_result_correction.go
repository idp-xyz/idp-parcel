package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 待确认的逐对象尝试结果的更正册（票 docs-consistency/code/90；迁移 0055）。更正只追加，首次提交的结果行不改写；读回经领域
// 更正的构造门重建（回指待确认、过确定结果的构造门），库内 CHECK 守不到的跨行关系（对象在尝试范围内、时序）靠这一道。

// PickupResultCorrections 实现 ports.PickupResultCorrections。
type PickupResultCorrections struct {
	db *bentopg.DB
}

func NewPickupResultCorrections(db *bentopg.DB) (*PickupResultCorrections, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &PickupResultCorrections{db: db}, nil
}

var _ ports.PickupResultCorrections = (*PickupResultCorrections)(nil)

// LoadForCorrection 取回尝试、该对象首次提交的结果与已落的更正，并锁住这格结果行直到事务结束：同一格的并发更正排队，
// 后到的读到先到的更正。读本身带锁，所以按框架合同无事务即拒（与写口同一纪律）。
func (repository *PickupResultCorrections) LoadForCorrection(
	ctx context.Context,
	key ports.PickupAttemptKey,
	object domain.CarriedObjectReference,
) (ports.PickupResultCorrectionTarget, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}

	var attemptRef, taskRef, executedBy, placeRef, evidence, outcomeName string
	var rescheduledFrom, basis, source *string
	var objectsJSON []byte
	var plannedFrom, plannedTo, arrivedAt, occurredAt time.Time
	err = executor.QueryRow(ctx,
		`SELECT a.attempt_ref, a.task_ref, a.executed_by, a.place_ref,
		        a.planned_from, a.planned_to, a.arrived_at, a.evidence_ref,
		        a.rescheduled_from, a.objects,
		        r.outcome, r.basis, r.source_ref, r.occurred_at
		   FROM transport_fulfillment.pickup_attempt AS a
		   JOIN transport_fulfillment.pickup_attempt_result AS r
		     ON r.tenant_id = a.tenant_id
		    AND r.source_id = a.source_id
		  WHERE a.tenant_id = $1
		    AND a.source_id = $2
		    AND r.object_ref = $3
		    FOR UPDATE OF r`,
		key.TenantID.String(), key.SourceID, object.String(),
	).Scan(&attemptRef, &taskRef, &executedBy, &placeRef,
		&plannedFrom, &plannedTo, &arrivedAt, &evidence,
		&rescheduledFrom, &objectsJSON,
		&outcomeName, &basis, &source, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PickupResultCorrectionTarget{}, false, nil
	}
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}

	attempt, err := rebuildAttempt(key.TenantID, attemptRef, taskRef, executedBy, placeRef, evidence,
		rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}
	outcome, err := attemptOutcomeFrom(outcomeName)
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}
	var basisRef domain.AttemptResultBasisReference
	if basis != nil {
		if basisRef, err = domain.NewAttemptResultBasisReference(*basis); err != nil {
			return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
		}
	}
	result, err := rebuildPickupResult(attempt, object, outcome, basisRef, source, occurredAt)
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}

	target := ports.PickupResultCorrectionTarget{Attempt: attempt, Result: result}
	stored, present, err := loadPickupCorrection(ctx, executor, key, attempt, result)
	if err != nil {
		return ports.PickupResultCorrectionTarget{}, false, fmt.Errorf("load pickup result for correction: %w", err)
	}
	target.Correction, target.HasCorrection = stored, present
	return target, true, nil
}

// loadPickupCorrection 读一格结果的更正（若有）。pending 是同一格首次提交的结果，更正经领域构造门回指它。
func loadPickupCorrection(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.PickupAttemptKey,
	attempt domain.FulfillmentAttempt,
	pending domain.AttemptObjectResult,
) (ports.PickupResultCorrectionRecord, bool, error) {
	var outcomeName, sourceRef string
	var basis, control, decidedBy *string
	var occurredAt, correctedAt, recordedAt time.Time
	err := querier.QueryRow(ctx,
		`SELECT outcome, basis, source_ref, control_ref, occurred_at, corrected_at, decided_by, recorded_at
		   FROM transport_fulfillment.pickup_attempt_result_correction
		  WHERE tenant_id = $1 AND source_id = $2 AND object_ref = $3`,
		key.TenantID.String(), key.SourceID, pending.Object().String(),
	).Scan(&outcomeName, &basis, &sourceRef, &control, &occurredAt, &correctedAt, &decidedBy, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PickupResultCorrectionRecord{}, false, nil
	}
	if err != nil {
		return ports.PickupResultCorrectionRecord{}, false, err
	}
	record, err := rebuildPickupCorrection(key, attempt, pending, outcomeName, basis, sourceRef, control, occurredAt, correctedAt, decidedBy, recordedAt)
	return record, err == nil, err
}

func rebuildPickupCorrection(
	key ports.PickupAttemptKey,
	attempt domain.FulfillmentAttempt,
	pending domain.AttemptObjectResult,
	outcomeName string,
	basis *string,
	sourceRef string,
	control *string,
	occurredAt, correctedAt time.Time,
	decidedBy *string,
	recordedAt time.Time,
) (ports.PickupResultCorrectionRecord, error) {
	outcome, err := attemptOutcomeFrom(outcomeName)
	if err != nil {
		return ports.PickupResultCorrectionRecord{}, err
	}
	var basisRef domain.AttemptResultBasisReference
	if basis != nil {
		if basisRef, err = domain.NewAttemptResultBasisReference(*basis); err != nil {
			return ports.PickupResultCorrectionRecord{}, err
		}
	}
	source, err := domain.NewAttemptResultSourceReference(sourceRef)
	if err != nil {
		return ports.PickupResultCorrectionRecord{}, err
	}
	correction, err := domain.CorrectPendingAttemptObjectResult(attempt, pending, outcome, basisRef, source, occurredAt, correctedAt)
	if err != nil {
		return ports.PickupResultCorrectionRecord{}, fmt.Errorf("pickup result correction does not rebuild: %w", err)
	}
	record := ports.PickupResultCorrectionRecord{Key: key, Correction: correction, RecordedAt: recordedAt.UTC()}
	if control != nil {
		if record.Control, err = domain.NewTransportControlReference(*control); err != nil {
			return ports.PickupResultCorrectionRecord{}, err
		}
	}
	if decidedBy != nil {
		if record.DecidedBy, err = domain.NewRegistrantReference(*decidedBy); err != nil {
			return ports.PickupResultCorrectionRecord{}, err
		}
	}
	return record, nil
}

// Save 追加一条更正。同一格已有更正时答`已有记录`，不覆盖先到者；锁没守住才会走到这一格。
func (repository *PickupResultCorrections) Save(
	ctx context.Context,
	record ports.PickupResultCorrectionRecord,
) (ports.PickupResultCorrectionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PickupResultCorrectionSaveOutcomeInvalid, fmt.Errorf("save pickup result correction: %w", err)
	}
	effective := record.Correction.Effective()
	var basis, control, decidedBy *string
	if value, present := effective.Basis(); present {
		raw := value.String()
		basis = &raw
	}
	if record.Control.String() != "" {
		raw := record.Control.String()
		control = &raw
	}
	if record.DecidedBy.String() != "" {
		raw := record.DecidedBy.String()
		decidedBy = &raw
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt_result_correction
			(tenant_id, source_id, object_ref, outcome, basis, source_ref, control_ref,
			 occurred_at, corrected_at, decided_by, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.SourceID, effective.Object().String(),
		effective.Outcome().String(), basis, record.Correction.Source().String(), control,
		effective.OccurredAt().UTC(), record.Correction.CorrectedAt().UTC(), decidedBy, record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.PickupResultCorrectionSaveOutcomeInvalid, fmt.Errorf("save pickup result correction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.PickupResultCorrectionAlreadyRecorded, nil
	}
	return ports.PickupResultCorrectionSaved, nil
}

// FindEffectiveByKey 实现 ports.FailedAttemptSource：取回一次揽收尝试及其逐对象当前有效的结果——待确认那一格若已更正，
// 读更正出来的确定结果，其余仍是首次提交的。Pickups 不随更正变：更正成揽收到手的场外揽收在对象级揽收登记册里，不在这里。
// 返回的记录不再满足「成功结果必有揽收」的接力核对（那条核对只对首次提交的记录成立，FindByKey 照旧做）。
func (repository *PickupAttempts) FindEffectiveByKey(
	ctx context.Context,
	key ports.PickupAttemptKey,
) (ports.PickupAttemptRecord, bool, error) {
	record, found, err := repository.FindByKey(ctx, key)
	if err != nil || !found {
		return record, found, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find effective pickup attempt: %w", err)
	}
	for index, result := range record.Results {
		if !result.Outcome().PendingConfirmation() {
			continue
		}
		correction, present, err := loadPickupCorrection(ctx, querier, key, record.Attempt, result)
		if err != nil {
			return ports.PickupAttemptRecord{}, false, fmt.Errorf("find effective pickup attempt: %w", err)
		}
		if present {
			record.Results[index] = correction.Correction.Effective()
		}
	}
	return record, true, nil
}

var _ ports.FailedAttemptSource = (*PickupAttempts)(nil)

// DeliveryResultCorrections 实现 ports.DeliveryResultCorrections。
type DeliveryResultCorrections struct {
	db *bentopg.DB
}

func NewDeliveryResultCorrections(db *bentopg.DB) (*DeliveryResultCorrections, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryResultCorrections{db: db}, nil
}

var _ ports.DeliveryResultCorrections = (*DeliveryResultCorrections)(nil)

// LoadForCorrection 取回尝试、该对象首次提交的结果与已落的更正，并锁住这格结果行直到事务结束；无事务即拒，理由同揽收一侧。
func (repository *DeliveryResultCorrections) LoadForCorrection(
	ctx context.Context,
	key ports.DeliveryAttemptKey,
	object domain.CarriedObjectReference,
) (ports.DeliveryResultCorrectionTarget, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}

	var taskRef, executedBy, placeRef, evidence, outcomeName string
	var rescheduledFrom, basis, source *string
	var objectsJSON []byte
	var plannedFrom, plannedTo, arrivedAt, occurredAt time.Time
	err = executor.QueryRow(ctx,
		`SELECT a.task_ref, a.executed_by, a.place_ref,
		        a.planned_from, a.planned_to, a.arrived_at, a.evidence_ref,
		        a.rescheduled_from, a.objects,
		        r.outcome, r.basis, r.source_ref, r.occurred_at
		   FROM transport_fulfillment.delivery_attempt AS a
		   JOIN transport_fulfillment.delivery_attempt_result AS r
		     ON r.tenant_id = a.tenant_id
		    AND r.attempt_ref = a.attempt_ref
		  WHERE a.tenant_id = $1
		    AND a.attempt_ref = $2
		    AND r.object_ref = $3
		    FOR UPDATE OF r`,
		key.TenantID.String(), key.Attempt.String(), object.String(),
	).Scan(&taskRef, &executedBy, &placeRef,
		&plannedFrom, &plannedTo, &arrivedAt, &evidence,
		&rescheduledFrom, &objectsJSON,
		&outcomeName, &basis, &source, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeliveryResultCorrectionTarget{}, false, nil
	}
	if err != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}

	attempt, err := rebuildAttempt(key.TenantID, key.Attempt.String(), taskRef, executedBy, placeRef, evidence,
		rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}
	result, err := rebuildDeliveryResult(attempt, object, outcomeName, basis, source, occurredAt)
	if err != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}

	target := ports.DeliveryResultCorrectionTarget{Attempt: attempt, Result: result}
	var correctedOutcome, correctedSource string
	var correctedBasis, decidedBy *string
	var correctedOccurredAt, correctedAt, recordedAt time.Time
	err = executor.QueryRow(ctx,
		`SELECT outcome, basis, source_ref, occurred_at, corrected_at, decided_by, recorded_at
		   FROM transport_fulfillment.delivery_attempt_result_correction
		  WHERE tenant_id = $1 AND attempt_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Attempt.String(), object.String(),
	).Scan(&correctedOutcome, &correctedBasis, &correctedSource, &correctedOccurredAt, &correctedAt, &decidedBy, &recordedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return target, true, nil
	case err != nil:
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}
	correction, err := rebuildDeliveryCorrection(attempt, result, correctedOutcome, correctedBasis, correctedSource, correctedOccurredAt, correctedAt)
	if err != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
	}
	target.Correction = ports.DeliveryResultCorrectionRecord{Key: key, Correction: correction, RecordedAt: recordedAt.UTC()}
	if decidedBy != nil {
		if target.Correction.DecidedBy, err = domain.NewRegistrantReference(*decidedBy); err != nil {
			return ports.DeliveryResultCorrectionTarget{}, false, fmt.Errorf("load delivery result for correction: %w", err)
		}
	}
	target.HasCorrection = true
	return target, true, nil
}

// rebuildDeliveryCorrection 经领域更正的构造门重建一条派送结果更正：回指首次提交的待确认、过确定结果的构造门。
func rebuildDeliveryCorrection(
	attempt domain.FulfillmentAttempt,
	pending domain.DeliveryAttemptResult,
	outcomeName string,
	basis *string,
	sourceRef string,
	occurredAt, correctedAt time.Time,
) (domain.DeliveryAttemptResultCorrection, error) {
	outcome, err := deliveryOutcomeFrom(outcomeName)
	if err != nil {
		return domain.DeliveryAttemptResultCorrection{}, err
	}
	var basisRef domain.AttemptResultBasisReference
	if basis != nil {
		if basisRef, err = domain.NewAttemptResultBasisReference(*basis); err != nil {
			return domain.DeliveryAttemptResultCorrection{}, err
		}
	}
	source, err := domain.NewAttemptResultSourceReference(sourceRef)
	if err != nil {
		return domain.DeliveryAttemptResultCorrection{}, err
	}
	correction, err := domain.CorrectPendingDeliveryAttemptResult(attempt, pending, outcome, basisRef, source, occurredAt, correctedAt)
	if err != nil {
		return domain.DeliveryAttemptResultCorrection{}, fmt.Errorf("delivery result correction does not rebuild: %w", err)
	}
	return correction, nil
}

// Save 追加一条更正，纪律同揽收一侧。
func (repository *DeliveryResultCorrections) Save(
	ctx context.Context,
	record ports.DeliveryResultCorrectionRecord,
) (ports.DeliveryResultCorrectionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeliveryResultCorrectionSaveOutcomeInvalid, fmt.Errorf("save delivery result correction: %w", err)
	}
	effective := record.Correction.Effective()
	var basis, decidedBy *string
	if value, present := effective.Basis(); present {
		raw := value.String()
		basis = &raw
	}
	if record.DecidedBy.String() != "" {
		raw := record.DecidedBy.String()
		decidedBy = &raw
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_attempt_result_correction
			(tenant_id, attempt_ref, object_ref, outcome, basis, source_ref,
			 occurred_at, corrected_at, decided_by, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(), record.Key.Attempt.String(), effective.Object().String(),
		effective.Outcome().String(), basis, record.Correction.Source().String(),
		effective.OccurredAt().UTC(), record.Correction.CorrectedAt().UTC(), decidedBy, record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.DeliveryResultCorrectionSaveOutcomeInvalid, fmt.Errorf("save delivery result correction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DeliveryResultCorrectionAlreadyRecorded, nil
	}
	return ports.DeliveryResultCorrectionSaved, nil
}
