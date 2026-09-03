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

// DispatchTasks 实现 ports.DispatchTaskRegistry：任务一行，对象范围逐对象一行。
//
// 两张表同笔落。没有对象的任务是领域产不出的东西——`RehydrateDispatchTask` 对空对象集直接拒
// ——所以写口要求环境事务，宁可拒绝写入也不留下一个读不回来的任务。
type DispatchTasks struct {
	db *bentopg.DB
}

func NewDispatchTasks(db *bentopg.DB) (*DispatchTasks, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DispatchTasks{db: db}, nil
}

var _ ports.DispatchTaskRegistry = (*DispatchTasks)(nil)

// FindByKey 按（租户+任务）取回整图。否定结果只回 false。
//
// 读回过重建门，逐格完备性与成组关系在这里复验一遍——坏行在这里暴露，而不是流到判断里。
func (repository *DispatchTasks) FindByKey(
	ctx context.Context,
	key ports.DispatchTaskKey,
) (ports.DispatchTaskRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}

	var kindText, placeRef, conditionsRef, stateText string
	var windowFrom, windowTo, openedAt, recordedAt time.Time
	var reschedules int
	var closureBasis *string
	var closedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT kind, place_ref, window_from, window_to, conditions_ref, opened_at,
		        reschedules, state, closure_basis, closed_at, recorded_at
		   FROM transport_fulfillment.dispatch_task
		  WHERE tenant_id = $1 AND task_ref = $2`,
		key.TenantID.String(), key.Task.String(),
	).Scan(&kindText, &placeRef, &windowFrom, &windowTo, &conditionsRef, &openedAt,
		&reschedules, &stateText, &closureBasis, &closedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DispatchTaskRecord{}, false, nil
	}
	if err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}

	// 对象按引用排序：顺序不进任何判断，但让读回稳定，比对夹具时不必先排一遍。
	rows, err := querier.Query(ctx,
		`SELECT object_ref
		   FROM transport_fulfillment.dispatch_task_object
		  WHERE tenant_id = $1 AND task_ref = $2
		  ORDER BY object_ref`,
		key.TenantID.String(), key.Task.String(),
	)
	if err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	defer rows.Close()

	spec := domain.RehydrateDispatchTaskSpec{
		TenantID:    key.TenantID,
		Task:        key.Task,
		WindowFrom:  windowFrom.UTC(),
		WindowTo:    windowTo.UTC(),
		OpenedAt:    openedAt.UTC(),
		Reschedules: reschedules,
	}
	if spec.Kind, err = dispatchTaskKindFrom(kindText); err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	if spec.State, err = taskStateFrom(stateText); err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	// 逐列走各自的构造门装回，不按列直接拼结构体——构造门是坏行的第一道拦截，绕过它等于
	// 把库当成可信来源。
	if spec.Place, err = domain.NewAttemptPlaceReference(placeRef); err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	if spec.Conditions, err = domain.NewServiceConditionReference(conditionsRef); err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	if closureBasis != nil {
		if spec.ClosureBasis, err = domain.NewTaskClosureBasisReference(*closureBasis); err != nil {
			return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
		}
	}
	if closedAt != nil {
		spec.ClosedAt = closedAt.UTC()
	}

	for rows.Next() {
		var objectRef string
		if err := rows.Scan(&objectRef); err != nil {
			return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
		}
		object, err := domain.NewCarriedObjectReference(objectRef)
		if err != nil {
			return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
		}
		spec.Objects = append(spec.Objects, object)
	}
	if err := rows.Err(); err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}

	task, err := domain.RehydrateDispatchTask(spec)
	if err != nil {
		return ports.DispatchTaskRecord{}, false, fmt.Errorf("find dispatch task: %w", err)
	}
	return ports.DispatchTaskRecord{Key: key, Task: task, RecordedAt: recordedAt.UTC()}, true, nil
}

// Save 首登一项任务连同它全部对象。撞键答`已建立`（ADR-0031），编排据此读回赢家。
func (repository *DispatchTasks) Save(
	ctx context.Context,
	record ports.DispatchTaskRecord,
) (ports.DispatchTaskSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DispatchTaskSaveOutcomeInvalid, fmt.Errorf("save dispatch task: %w", err)
	}
	if err := assertDispatchTaskKeyAgrees(record); err != nil {
		return ports.DispatchTaskSaveOutcomeInvalid, fmt.Errorf("save dispatch task: %w", err)
	}

	windowFrom, windowTo := record.Task.Window()
	closureBasis, closedAt, closed := record.Task.Closure()
	var basisText *string
	if closed {
		text := closureBasis.String()
		basisText = &text
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.dispatch_task
		     (tenant_id, task_ref, kind, place_ref, window_from, window_to, conditions_ref,
		      opened_at, reschedules, state, closure_basis, closed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Task.String(),
		record.Task.Kind().String(),
		record.Task.Place().String(),
		windowFrom.UTC(),
		windowTo.UTC(),
		record.Task.Conditions().String(),
		record.Task.OpenedAt().UTC(),
		record.Task.Reschedules(),
		record.Task.State().String(),
		basisText,
		nullableTime(closedAt, closed),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.DispatchTaskSaveOutcomeInvalid, fmt.Errorf("save dispatch task: %w", err)
	}
	// 撞键只看任务那一行：对象挂在它下面，任务已在册就说明这一整图已经有人写过。
	if tag.RowsAffected() == 0 {
		return ports.DispatchTaskAlreadyOpen, nil
	}

	for _, object := range record.Task.Objects() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.dispatch_task_object
			     (tenant_id, task_ref, object_ref, recorded_at)
			 VALUES ($1, $2, $3, $4)`,
			record.Key.TenantID.String(),
			record.Key.Task.String(),
			object.String(),
			record.RecordedAt.UTC(),
		); err != nil {
			return ports.DispatchTaskSaveOutcomeInvalid, fmt.Errorf("save dispatch task: %w", err)
		}
	}
	return ports.DispatchTaskSaved, nil
}

// assertDispatchTaskKeyAgrees 挡住「键说的是一项任务、聚合说的是另一项」那种写入。
// 两边对不上时写进去的行读回来会是另一个任务，而两处都各自看着没错。
func assertDispatchTaskKeyAgrees(record ports.DispatchTaskRecord) error {
	if record.Key.TenantID != record.Task.TenantID() || record.Key.Task != record.Task.Task() {
		return fmt.Errorf("dispatch task key disagrees with the aggregate")
	}
	return nil
}

func dispatchTaskKindFrom(raw string) (domain.DispatchTaskKind, error) {
	switch raw {
	case domain.PickupDispatch.String():
		return domain.PickupDispatch, nil
	case domain.DeliveryDispatch.String():
		return domain.DeliveryDispatch, nil
	default:
		return domain.DispatchTaskKindInvalid, fmt.Errorf("unknown dispatch task kind %q", raw)
	}
}

func taskStateFrom(raw string) (domain.TaskState, error) {
	switch raw {
	case domain.TaskOpen.String():
		return domain.TaskOpen, nil
	case domain.TaskTerminated.String():
		return domain.TaskTerminated, nil
	case domain.TaskCompleted.String():
		return domain.TaskCompleted, nil
	default:
		return domain.TaskStateInvalid, fmt.Errorf("unknown dispatch task state %q", raw)
	}
}
