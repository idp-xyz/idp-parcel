package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// PickupAttempts 实现 ports.PickupAttemptStore（写入代数同 ADR-0031）。
//
// 父行是尝试本身；逐对象结果与场外揽收分表。揽收行通过 outcome='PICKED_UP' 接到
// 结果行——失败对象在库里接不上揽收，形状上就进不了 Pickups。
type PickupAttempts struct {
	db *bentopg.DB
}

func NewPickupAttempts(db *bentopg.DB) (*PickupAttempts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &PickupAttempts{db: db}, nil
}

// FindByKey 按（租户+来源身份）取回已保存的尝试提交。否定结果只回 false。读回经
// 领域构造门重建，并核对接力：每个成功结果必须有对应揽收，失败对象不得出现在揽收里。
func (repository *PickupAttempts) FindByKey(
	ctx context.Context,
	key ports.PickupAttemptKey,
) (ports.PickupAttemptRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}

	var (
		attemptRef, taskRef, executedBy, placeRef, evidence, digest string
		rescheduledFrom                                             *string
		objectsJSON                                                 []byte
		plannedFrom, plannedTo, arrivedAt, recordedAt               time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT attempt_ref, task_ref, executed_by, place_ref,
		        planned_from, planned_to, arrived_at, evidence_ref,
		        rescheduled_from, objects, content_digest, recorded_at
		   FROM transport_fulfillment.pickup_attempt
		  WHERE tenant_id = $1
		    AND source_id = $2`,
		key.TenantID.String(),
		key.SourceID,
	).Scan(&attemptRef, &taskRef, &executedBy, &placeRef,
		&plannedFrom, &plannedTo, &arrivedAt, &evidence,
		&rescheduledFrom, &objectsJSON, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PickupAttemptRecord{}, false, nil
	}
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}

	attempt, err := rebuildAttempt(key.TenantID, attemptRef, taskRef, executedBy, placeRef, evidence, rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}

	results, err := repository.loadResults(ctx, querier, key, attempt)
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}
	pickups, err := repository.loadPickups(ctx, querier, key)
	if err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}
	if err := assertPickupCoupling(results, pickups); err != nil {
		return ports.PickupAttemptRecord{}, false, fmt.Errorf("find pickup attempt: %w", err)
	}

	return ports.PickupAttemptRecord{
		Key:           key,
		ContentDigest: digest,
		Attempt:       attempt,
		Results:       results,
		Pickups:       pickups,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次尝试提交。同来源已有记录时答`已有记录`，不覆盖先到者。先写父行再写
// 子行：撞键 Occurs 在父行，子行不会被第二份写入碰触。
func (repository *PickupAttempts) Save(
	ctx context.Context,
	record ports.PickupAttemptRecord,
) (ports.PickupSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
	}
	if err := assertPickupCoupling(record.Results, record.Pickups); err != nil {
		return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
	}

	objectIDs := make([]string, 0, len(record.Attempt.Objects()))
	for _, object := range record.Attempt.Objects() {
		objectIDs = append(objectIDs, object.String())
	}
	objectsJSON, err := json.Marshal(objectIDs)
	if err != nil {
		return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
	}

	var rescheduled *string
	if from, present := record.Attempt.RescheduledFrom(); present {
		raw := from.String()
		rescheduled = &raw
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.pickup_attempt
			(tenant_id, source_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, rescheduled_from,
			 objects, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.SourceID,
		record.Attempt.Attempt().String(),
		record.Attempt.Task().String(),
		record.Attempt.ExecutedBy().String(),
		record.Attempt.Place().String(),
		record.Attempt.PlannedFrom().UTC(),
		record.Attempt.PlannedTo().UTC(),
		record.Attempt.ArrivedAt().UTC(),
		record.Attempt.Evidence().String(),
		rescheduled,
		objectsJSON,
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.PickupAlreadyRecorded, nil
	}

	for _, result := range record.Results {
		var basis *string
		if value, present := result.Basis(); present {
			raw := value.String()
			basis = &raw
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.pickup_attempt_result
				(tenant_id, source_id, object_ref, outcome, basis, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			record.Key.TenantID.String(),
			record.Key.SourceID,
			result.Object().String(),
			result.Outcome().String(),
			basis,
			result.OccurredAt().UTC(),
		); err != nil {
			return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
		}
	}
	for _, pickup := range record.Pickups {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.pickup_attempt_pickup
				(tenant_id, source_id, object_ref, outcome, task_ref, attempt_ref,
				 place_ref, control_ref, executed_by, version, occurred_at)
			 VALUES ($1, $2, $3, 'PICKED_UP', $4, $5, $6, $7, $8, $9, $10)`,
			record.Key.TenantID.String(),
			record.Key.SourceID,
			pickup.Object().String(),
			pickup.Task().String(),
			pickup.Attempt().String(),
			pickup.Place().String(),
			pickup.Control().String(),
			pickup.ExecutedBy().String(),
			pickup.Version().String(),
			pickup.OccurredAt().UTC(),
		); err != nil {
			return ports.PickupSaveOutcomeInvalid, fmt.Errorf("save pickup attempt: %w", err)
		}
	}
	return ports.PickupSaved, nil
}

func (repository *PickupAttempts) loadResults(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.PickupAttemptKey,
	attempt domain.FulfillmentAttempt,
) ([]domain.AttemptObjectResult, error) {
	rows, err := querier.Query(ctx,
		`SELECT object_ref, outcome, basis, occurred_at
		   FROM transport_fulfillment.pickup_attempt_result
		  WHERE tenant_id = $1 AND source_id = $2
		  ORDER BY object_ref`,
		key.TenantID.String(),
		key.SourceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.AttemptObjectResult
	for rows.Next() {
		var objectRef, outcomeName string
		var basis *string
		var occurredAt time.Time
		if err := rows.Scan(&objectRef, &outcomeName, &basis, &occurredAt); err != nil {
			return nil, err
		}
		object, err := domain.NewCarriedObjectReference(objectRef)
		if err != nil {
			return nil, err
		}
		outcome, err := attemptOutcomeFrom(outcomeName)
		if err != nil {
			return nil, err
		}
		var basisRef domain.AttemptResultBasisReference
		if basis != nil {
			basisRef, err = domain.NewAttemptResultBasisReference(*basis)
			if err != nil {
				return nil, err
			}
		}
		result, err := domain.FormAttemptObjectResult(attempt, object, outcome, basisRef, occurredAt)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (repository *PickupAttempts) loadPickups(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.PickupAttemptKey,
) ([]domain.OffsitePickup, error) {
	rows, err := querier.Query(ctx,
		`SELECT object_ref, task_ref, attempt_ref, place_ref, control_ref,
		        executed_by, version, occurred_at
		   FROM transport_fulfillment.pickup_attempt_pickup
		  WHERE tenant_id = $1 AND source_id = $2
		  ORDER BY object_ref`,
		key.TenantID.String(),
		key.SourceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pickups []domain.OffsitePickup
	for rows.Next() {
		var objectRef, taskRef, attemptRef, placeRef, controlRef, executedBy, version string
		var occurredAt time.Time
		if err := rows.Scan(&objectRef, &taskRef, &attemptRef, &placeRef, &controlRef, &executedBy, &version, &occurredAt); err != nil {
			return nil, err
		}
		object, err := domain.NewCarriedObjectReference(objectRef)
		if err != nil {
			return nil, err
		}
		task, err := domain.NewPickupTaskReference(taskRef)
		if err != nil {
			return nil, err
		}
		attempt, err := domain.NewAttemptReference(attemptRef)
		if err != nil {
			return nil, err
		}
		place, err := domain.NewPickupPlaceReference(placeRef)
		if err != nil {
			return nil, err
		}
		control, err := domain.NewTransportControlReference(controlRef)
		if err != nil {
			return nil, err
		}
		party, err := domain.NewExecutingPartyReference(executedBy)
		if err != nil {
			return nil, err
		}
		resultVersion, err := domain.NewPickupResultVersion(version)
		if err != nil {
			return nil, err
		}
		pickup, err := domain.FormOffsitePickup(domain.OffsitePickupSpec{
			TenantID:   key.TenantID,
			Object:     object,
			Task:       task,
			Attempt:    attempt,
			Place:      place,
			Control:    control,
			ExecutedBy: party,
			Version:    resultVersion,
			OccurredAt: occurredAt,
		})
		if err != nil {
			return nil, err
		}
		pickups = append(pickups, pickup)
	}
	return pickups, rows.Err()
}

func rebuildAttempt(
	tenant domain.TenantID,
	attemptRef, taskRef, executedBy, placeRef, evidence string,
	rescheduledFrom *string,
	objectsJSON []byte,
	plannedFrom, plannedTo, arrivedAt time.Time,
) (domain.FulfillmentAttempt, error) {
	attemptID, err := domain.NewAttemptReference(attemptRef)
	if err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	task, err := domain.NewDispatchTaskReference(taskRef)
	if err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	party, err := domain.NewExecutingPartyReference(executedBy)
	if err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	place, err := domain.NewAttemptPlaceReference(placeRef)
	if err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	evidenceRef, err := domain.NewAttemptEvidenceReference(evidence)
	if err != nil {
		return domain.FulfillmentAttempt{}, err
	}

	var objectIDs []string
	if err := json.Unmarshal(objectsJSON, &objectIDs); err != nil {
		return domain.FulfillmentAttempt{}, err
	}
	objects := make([]domain.CarriedObjectReference, 0, len(objectIDs))
	for _, raw := range objectIDs {
		object, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.FulfillmentAttempt{}, err
		}
		objects = append(objects, object)
	}

	spec := domain.FulfillmentAttemptSpec{
		TenantID:    tenant,
		Attempt:     attemptID,
		Task:        task,
		ExecutedBy:  party,
		Place:       place,
		PlannedFrom: plannedFrom,
		PlannedTo:   plannedTo,
		ArrivedAt:   arrivedAt,
		Objects:     objects,
		Evidence:    evidenceRef,
	}
	if rescheduledFrom != nil {
		from, err := domain.NewAttemptReference(*rescheduledFrom)
		if err != nil {
			return domain.FulfillmentAttempt{}, err
		}
		spec.RescheduledFrom = from
	}
	return domain.FormFulfillmentAttempt(spec)
}

func assertPickupCoupling(results []domain.AttemptObjectResult, pickups []domain.OffsitePickup) error {
	succeeded := make(map[string]struct{}, len(results))
	for _, result := range results {
		if result.Outcome().Succeeded() {
			succeeded[result.Object().String()] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(pickups))
	for _, pickup := range pickups {
		id := pickup.Object().String()
		if _, ok := succeeded[id]; !ok {
			return fmt.Errorf("pickup for %s has no matching picked-up result", id)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("duplicate pickup for %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(succeeded) {
		return fmt.Errorf("picked-up result missing its offsite pickup")
	}
	return nil
}

func attemptOutcomeFrom(raw string) (domain.AttemptObjectOutcome, error) {
	switch raw {
	case domain.ObjectPickedUp.String():
		return domain.ObjectPickedUp, nil
	case domain.CustomerAbsent.String():
		return domain.CustomerAbsent, nil
	case domain.GoodsNotReady.String():
		return domain.GoodsNotReady, nil
	case domain.PackagingUnacceptable.String():
		return domain.PackagingUnacceptable, nil
	default:
		return 0, fmt.Errorf("unknown attempt object outcome %q", raw)
	}
}
