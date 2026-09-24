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

// DeliveryAttempts 实现两个口：ports.DeliveryAttemptView 按（尝试、对象）取回派送尝试与该对象的结果，
// 供交付生效引用；ports.DeliveryAttemptStore 供执行侧登记尝试（与揽收侧的 PickupAttempts 对称，写入
// 代数同 ADR-0031）。
//
// 一只适配器、两个口：交付生效的依赖里只有读口，它在类型上写不了尝试。交付生效引用的是已经发生的
// 到场——一个入口同时造尝试和造交付，就没有东西拦得住「先声称到过场再声称交付成功」。
type DeliveryAttempts struct {
	db *bentopg.DB
}

func NewDeliveryAttempts(db *bentopg.DB) (*DeliveryAttempts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryAttempts{db: db}, nil
}

var (
	_ ports.DeliveryAttemptView  = (*DeliveryAttempts)(nil)
	_ ports.DeliveryAttemptStore = (*DeliveryAttempts)(nil)
)

// FindByKey 按（租户，尝试）取回已登记的尝试与全部对象结果。否定结果只回 false。读回经领域构造门重建，
// 编排拿它与新提交逐格比内容。
func (repository *DeliveryAttempts) FindByKey(
	ctx context.Context,
	key ports.DeliveryAttemptKey,
) (ports.DeliveryAttemptRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
	}

	var taskRef, executedBy, placeRef, evidence string
	var rescheduledFrom *string
	var objectsJSON []byte
	var plannedFrom, plannedTo, arrivedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT task_ref, executed_by, place_ref, planned_from, planned_to, arrived_at,
		        evidence_ref, rescheduled_from, objects, recorded_at
		   FROM transport_fulfillment.delivery_attempt
		  WHERE tenant_id = $1
		    AND attempt_ref = $2`,
		key.TenantID.String(),
		key.Attempt.String(),
	).Scan(&taskRef, &executedBy, &placeRef, &plannedFrom, &plannedTo, &arrivedAt,
		&evidence, &rescheduledFrom, &objectsJSON, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeliveryAttemptRecord{}, false, nil
	}
	if err != nil {
		return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
	}

	attempt, err := rebuildAttempt(key.TenantID, key.Attempt.String(), taskRef, executedBy, placeRef, evidence,
		rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_ref, outcome, basis, occurred_at
		   FROM transport_fulfillment.delivery_attempt_result
		  WHERE tenant_id = $1 AND attempt_ref = $2
		  ORDER BY object_ref`,
		key.TenantID.String(),
		key.Attempt.String(),
	)
	if err != nil {
		return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
	}
	defer rows.Close()

	record := ports.DeliveryAttemptRecord{Key: key, Attempt: attempt, RecordedAt: recordedAt.UTC()}
	for rows.Next() {
		var objectRef, outcomeName string
		var basis *string
		var occurredAt time.Time
		if err := rows.Scan(&objectRef, &outcomeName, &basis, &occurredAt); err != nil {
			return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
		}
		object, err := domain.NewCarriedObjectReference(objectRef)
		if err != nil {
			return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
		}
		result, err := rebuildDeliveryResult(attempt, object, outcomeName, basis, occurredAt)
		if err != nil {
			return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
		}
		record.Results = append(record.Results, result)
	}
	if err := rows.Err(); err != nil {
		return ports.DeliveryAttemptRecord{}, false, fmt.Errorf("find delivery attempt: %w", err)
	}
	return record, true, nil
}

// Save 写下一次派送尝试及其逐对象结果。同一（租户，尝试）已有记录时答`已有记录`，不覆盖先到者。先写父行
// 再写子行：撞键发生在父行，子行不会被第二份写入碰到。父子两行要一起成立，所以写口按框架合同无事务即拒，
// 事务边界归装配点。
func (repository *DeliveryAttempts) Save(
	ctx context.Context,
	record ports.DeliveryAttemptRecord,
) (ports.DeliveryAttemptSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeliveryAttemptSaveOutcomeInvalid, fmt.Errorf("save delivery attempt: %w", err)
	}
	// 键与尝试本体各带一份租户与尝试身份；两份不一致时写哪一份都是替调用方选，这里拒。
	if record.Key.TenantID != record.Attempt.TenantID() || record.Key.Attempt != record.Attempt.Attempt() {
		return ports.DeliveryAttemptSaveOutcomeInvalid,
			fmt.Errorf("save delivery attempt: key %s/%s does not name the attempt it carries",
				record.Key.TenantID, record.Key.Attempt)
	}

	objectIDs := make([]string, 0, len(record.Attempt.Objects()))
	for _, object := range record.Attempt.Objects() {
		objectIDs = append(objectIDs, object.String())
	}
	objectsJSON, err := json.Marshal(objectIDs)
	if err != nil {
		return ports.DeliveryAttemptSaveOutcomeInvalid, fmt.Errorf("save delivery attempt: %w", err)
	}
	var rescheduled *string
	if from, present := record.Attempt.RescheduledFrom(); present {
		raw := from.String()
		rescheduled = &raw
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_attempt
			(tenant_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, rescheduled_from,
			 objects, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Attempt.String(),
		record.Attempt.Task().String(),
		record.Attempt.ExecutedBy().String(),
		record.Attempt.Place().String(),
		record.Attempt.PlannedFrom().UTC(),
		record.Attempt.PlannedTo().UTC(),
		record.Attempt.ArrivedAt().UTC(),
		record.Attempt.Evidence().String(),
		rescheduled,
		objectsJSON,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.DeliveryAttemptSaveOutcomeInvalid, fmt.Errorf("save delivery attempt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DeliveryAttemptAlreadyRecorded, nil
	}

	for _, result := range record.Results {
		var basis *string
		if value, present := result.Basis(); present {
			raw := value.String()
			basis = &raw
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.delivery_attempt_result
				(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			record.Key.TenantID.String(),
			record.Key.Attempt.String(),
			result.Object().String(),
			result.Outcome().String(),
			basis,
			result.OccurredAt().UTC(),
		); err != nil {
			return ports.DeliveryAttemptSaveOutcomeInvalid, fmt.Errorf("save delivery attempt: %w", err)
		}
	}
	return ports.DeliveryAttemptSaved, nil
}

// LoadDeliveryResult 一次连表取回尝试与对象结果。
//
// 尝试与结果一起取而不分两次：分两次时中间那个瞬间足以让「尝试在、结果不在」变成
// 「两者都在」，而这个 found 正是编排用来分「指错」与「等谁」的——指名的尝试或对象
// 结果不存在是提交矛盾（DeliveryNotAccepted），读不到库才是未决。两者混一格，一次
// 库抖动就会被当成来源写错了单。
func (view *DeliveryAttempts) LoadDeliveryResult(
	ctx context.Context,
	tenant domain.TenantID,
	attempt domain.AttemptReference,
	object domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	var taskRef, executedBy, placeRef, evidence, outcomeName string
	var rescheduledFrom, basis *string
	var objectsJSON []byte
	var plannedFrom, plannedTo, arrivedAt, occurredAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT a.task_ref, a.executed_by, a.place_ref,
		        a.planned_from, a.planned_to, a.arrived_at, a.evidence_ref,
		        a.rescheduled_from, a.objects,
		        r.outcome, r.basis, r.occurred_at
		   FROM transport_fulfillment.delivery_attempt AS a
		   JOIN transport_fulfillment.delivery_attempt_result AS r
		     ON r.tenant_id = a.tenant_id
		    AND r.attempt_ref = a.attempt_ref
		  WHERE a.tenant_id = $1
		    AND a.attempt_ref = $2
		    AND r.object_ref = $3`,
		tenant.String(),
		attempt.String(),
		object.String(),
	).Scan(&taskRef, &executedBy, &placeRef,
		&plannedFrom, &plannedTo, &arrivedAt, &evidence,
		&rescheduledFrom, &objectsJSON,
		&outcomeName, &basis, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, nil
	}
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	// 复用揽收侧的尝试重建：同一个 FulfillmentAttempt 领域对象，两处各写一份重建会
	// 让同一格事实长出两个样子。
	fulfillmentAttempt, err := rebuildAttempt(
		tenant, attempt.String(), taskRef, executedBy, placeRef, evidence,
		rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	result, err := rebuildDeliveryResult(fulfillmentAttempt, object, outcomeName, basis, occurredAt)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}
	return fulfillmentAttempt, result, true, nil
}

// rebuildDeliveryResult 经 FormDeliveryAttemptResult 复验：对象在尝试范围内、失败带
// 依据、妥投不带依据、结果不早于到场。库内 CHECK 守得住前后两条，「对象在范围内」与
// 时序跨表守不了，靠这道门。
func rebuildDeliveryResult(
	attempt domain.FulfillmentAttempt,
	object domain.CarriedObjectReference,
	outcomeName string,
	basis *string,
	occurredAt time.Time,
) (domain.DeliveryAttemptResult, error) {
	outcome, err := deliveryOutcomeFrom(outcomeName)
	if err != nil {
		return domain.DeliveryAttemptResult{}, err
	}
	var basisRef domain.AttemptResultBasisReference
	if basis != nil {
		if basisRef, err = domain.NewAttemptResultBasisReference(*basis); err != nil {
			return domain.DeliveryAttemptResult{}, err
		}
	}
	return domain.FormDeliveryAttemptResult(attempt, object, outcome, basisRef, occurredAt)
}

func deliveryOutcomeFrom(raw string) (domain.DeliveryObjectOutcome, error) {
	switch raw {
	case domain.ObjectDelivered.String():
		return domain.ObjectDelivered, nil
	case domain.DeliveryRefused.String():
		return domain.DeliveryRefused, nil
	case domain.NoOneToReceive.String():
		return domain.NoOneToReceive, nil
	case domain.WrongAddress.String():
		return domain.WrongAddress, nil
	default:
		return 0, fmt.Errorf("unknown delivery object outcome %q", raw)
	}
}
