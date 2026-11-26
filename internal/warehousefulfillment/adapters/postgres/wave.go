package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Waves 是分配策略、波次与拣货任务的 PostgreSQL 适配器。
type Waves struct {
	db *bentopg.DB
}

func NewWaves(db *bentopg.DB) (*Waves, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Waves{db: db}, nil
}

var _ ports.WaveStore = (*Waves)(nil)

func (store *Waves) LoadStrategy(ctx context.Context, tenant domain.TenantID) (ports.AllocationStrategy, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AllocationStrategy{}, false, err
	}
	var revision int
	var operator string
	var at time.Time
	err = querier.QueryRow(ctx,
		`SELECT revision, operator_id, registered_at
		   FROM warehouse_fulfillment.allocation_strategy
		  WHERE tenant_id = $1
		  ORDER BY revision DESC
		  LIMIT 1`, tenant.String()).Scan(&revision, &operator, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AllocationStrategy{}, false, nil
	}
	if err != nil {
		return ports.AllocationStrategy{}, false, fmt.Errorf("load allocation strategy: %w", err)
	}
	return ports.AllocationStrategy{Revision: revision, Operator: operator, RegisteredAt: at}, true, nil
}

func (store *Waves) SaveStrategy(ctx context.Context, tenant domain.TenantID, strategy ports.AllocationStrategy) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save allocation strategy: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.allocation_strategy (tenant_id, revision, operator_id, registered_at)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), strategy.Revision, strategy.Operator, strategy.RegisteredAt)
	if err != nil {
		return fmt.Errorf("save allocation strategy: %w", err)
	}
	return nil
}

func (store *Waves) LoadWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]domain.OutboundRequestID, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, err
	}
	var exists bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2)`,
		tenant.String(), id.String()).Scan(&exists); err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := querier.Query(ctx,
		`SELECT request_id FROM warehouse_fulfillment.wave_request
		  WHERE tenant_id = $1 AND wave_id = $2
		  ORDER BY request_id`, tenant.String(), id.String())
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var requests []domain.OutboundRequestID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, false, err
		}
		parsed, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return nil, false, err
		}
		requests = append(requests, parsed)
	}
	return requests, true, rows.Err()
}

func (store *Waves) SaveWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, warehouse domain.WarehouseCode, operator domain.Registrant, prefetch, batch bool, at time.Time, requests []domain.OutboundRequestID, tasks []ports.PickTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.wave (tenant_id, wave_id, warehouse_code, operator_id, prefetch, released_at, status, batch_pick)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		// 任务还没写进去，先按「没有任务」的答案落下；任务写完再照规则重算，放出时的状态不另写一份。
		tenant.String(), id.String(), warehouse.String(), operator.String(), prefetch, at, ports.WaveCancelled, batch); err != nil {
		return fmt.Errorf("save wave: %w", err)
	}
	seen := map[string]struct{}{}
	for _, request := range requests {
		if _, ok := seen[request.String()]; ok {
			continue
		}
		seen[request.String()] = struct{}{}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.wave_request (tenant_id, wave_id, request_id) VALUES ($1,$2,$3)`,
			tenant.String(), id.String(), request.String()); err != nil {
			return fmt.Errorf("save wave request: %w", err)
		}
		tag, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_request
			    SET status = 'ALLOCATED'
			  WHERE tenant_id = $1 AND request_id = $2 AND status = 'RESERVED'`,
			tenant.String(), request.String())
		if err != nil {
			return fmt.Errorf("allocate outbound request: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("allocate outbound request: %s was not reserved", request.String())
		}
	}
	for _, task := range tasks {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.pick_task (
			    tenant_id, task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state,
			    inbound_batch_id, quantity, picked, status, fact_id, label_alias
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,'OPEN',NULL,$11)`,
			tenant.String(), task.ID.String(), task.WaveID.String(), task.Request.String(), task.Location.String(),
			task.Goods.String(), task.Unit.String(), task.State.String(), task.Inbound.String(), task.Quantity,
			optionalText(task.Alias.String(), task.Alias.String() != "")); err != nil {
			return fmt.Errorf("save pick task: %w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.wave AS target
		    SET single_unit = `+waveIsSingleUnit+`, batch_pick = target.batch_pick AND NOT `+waveIsSingleUnit+`
		  WHERE target.tenant_id = $1 AND target.wave_id = $2`,
		tenant.String(), id.String()); err != nil {
		return fmt.Errorf("save wave: %w", err)
	}
	return refreshWaveStatus(ctx, executor, tenant, `target.wave_id = $2`, id.String())
}

func (store *Waves) WavePicksMerged(ctx context.Context, tenant domain.TenantID, id domain.WaveID) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var merged bool
	err = querier.QueryRow(ctx,
		`SELECT single_unit OR batch_pick FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2`,
		tenant.String(), id.String()).Scan(&merged)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("wave picks merged: %w", err)
	}
	return merged, nil
}

// SingleUnitAwaitingCheck 只认还留在单品单件波次里的请求（撤出的不算），拣的那一件取它在那个波次里拣满的任务。
func (store *Waves) SingleUnitAwaitingCheck(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) ([]ports.SingleUnitCandidate, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT request.request_id, coalesce(request.document_number, ''), request.owner_account_id, task.goods_code
		   FROM warehouse_fulfillment.outbound_request request
		   JOIN warehouse_fulfillment.wave_request member
		     ON member.tenant_id = request.tenant_id AND member.request_id = request.request_id AND member.withdrawn_at IS NULL
		   JOIN warehouse_fulfillment.wave wave
		     ON wave.tenant_id = member.tenant_id AND wave.wave_id = member.wave_id AND wave.single_unit
		   JOIN warehouse_fulfillment.pick_task task
		     ON task.tenant_id = request.tenant_id AND task.request_id = request.request_id
		    AND task.wave_id = wave.wave_id AND task.status = 'DONE'
		  WHERE request.tenant_id = $1 AND request.warehouse_code = $2 AND request.status = 'PICKED'
		  ORDER BY wave.released_at, request.received_at, request.request_id
		  LIMIT $3`,
		tenant.String(), warehouse.String(), ports.SingleUnitCandidateLimit)
	if err != nil {
		return nil, fmt.Errorf("single unit awaiting check: %w", err)
	}
	defer rows.Close()
	var candidates []ports.SingleUnitCandidate
	for rows.Next() {
		var request, owner, goods string
		var candidate ports.SingleUnitCandidate
		if err := rows.Scan(&request, &candidate.DocumentNumber, &owner, &goods); err != nil {
			return nil, fmt.Errorf("single unit awaiting check: %w", err)
		}
		if candidate.Request, err = domain.NewOutboundRequestID(request); err != nil {
			return nil, fmt.Errorf("single unit awaiting check: %w", err)
		}
		if candidate.Owner, err = domain.NewOwnerAccountID(owner); err != nil {
			return nil, fmt.Errorf("single unit awaiting check: %w", err)
		}
		if candidate.Goods, err = domain.NewGoodsCode(goods); err != nil {
			return nil, fmt.Errorf("single unit awaiting check: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("single unit awaiting check: %w", err)
	}
	return candidates, nil
}

// waveIsSingleUnit 是「单品单件波次」的那一条规则：波次里有出库请求，并且每一张要拣的都只有一件——
// 预占需求合计是 1 个基本单位（套装按组件展开之后算）。放出之后不往波次里加单，所以放出时算一次就定了；
// 迁移回填已有波次时用的也是这一条。
const waveIsSingleUnit = `(
	EXISTS (
	    SELECT 1 FROM warehouse_fulfillment.wave_request member
	     WHERE member.tenant_id = target.tenant_id AND member.wave_id = target.wave_id)
	AND NOT EXISTS (
	    SELECT 1 FROM warehouse_fulfillment.wave_request member
	     WHERE member.tenant_id = target.tenant_id AND member.wave_id = target.wave_id
	       AND (SELECT coalesce(sum(reservation.wanted), 0)
	              FROM warehouse_fulfillment.outbound_reservation reservation
	             WHERE reservation.tenant_id = member.tenant_id
	               AND reservation.request_id = member.request_id) <> 1)
)`

// waveStatusOfTasks 是波次状态的那一条规则，口径写在 ports.WaveReleased 那一组常量上。
// 迁移回填已有波次时用的也是这一条。
const waveStatusOfTasks = `(
	SELECT CASE WHEN count(*) FILTER (WHERE task.status = 'OPEN') > 0
	             AND count(*) FILTER (WHERE task.status IN ('DONE', 'SHORT')) = 0 THEN '` + ports.WaveReleased + `'
	            WHEN count(*) FILTER (WHERE task.status = 'OPEN') > 0 THEN '` + ports.WavePicking + `'
	            WHEN count(*) FILTER (WHERE task.status IN ('DONE', 'SHORT')) > 0 THEN '` + ports.WaveCompleted + `'
	            ELSE '` + ports.WaveCancelled + `' END
	  FROM warehouse_fulfillment.pick_task task
	 WHERE task.tenant_id = target.tenant_id AND task.wave_id = target.wave_id)`

// wavesOfRequest 圈出一张出库请求有任务在里面的各个波次：少拣之后再放的请求，任务可以分在两个波次里。
const wavesOfRequest = `target.wave_id IN (
	SELECT member.wave_id FROM warehouse_fulfillment.pick_task member
	 WHERE member.tenant_id = $1 AND member.request_id = $2)`

// lockWaves 在动拣货任务之前先锁住 which 圈出的波次行，锁到事务结束。
// 一个波次里可以有几个货主的单，各货主的拣货不共用入账次序，截单更不走那道次序：两笔事务各改同一个波次里不同请求的任务，
// 后重算的那一笔若只靠重算那一句去等行锁，醒来之后数任务用的仍是等锁之前的快照，看不到先提交的那一笔，
// 存下的状态就停在「还在拣」。所以锁要单独一句、赶在改任务之前拿：拿到之后的每一句都看得到前一个持锁者提交的任务，
// 同一个波次的任务改动从此一笔一笔排着来。几个波次按波次号的先后去锁，两笔事务不会各拿一半互相等。
// 各写口都是先锁波次、再动任务，次序一致。which 的 $1 是租户，$2 是 scope。
func lockWaves(ctx context.Context, executor bentopg.Executor, tenant domain.TenantID, which, scope string) error {
	if _, err := executor.Exec(ctx,
		`SELECT target.wave_id FROM warehouse_fulfillment.wave AS target
		  WHERE target.tenant_id = $1 AND `+which+`
		  ORDER BY target.wave_id
		    FOR NO KEY UPDATE`,
		tenant.String(), scope); err != nil {
		return fmt.Errorf("lock waves: %w", err)
	}
	return nil
}

// refreshWaveStatus 照 waveStatusOfTasks 重算 which 圈出的波次。凡是让波次的状态可能变的写口——把任务从待执行改成
// 拣满、少拣或作废的，往波次里添任务的——都先调 lockWaves、改完任务再在同一笔事务里调它：波次上存的状态只从这里来，
// 不在别处各写各的。SaveLatePick 把作废改成迟到，两种都不计入状态；SaveOpenTask 只改库位与数量：这两处不调。
// which 的 $1 是租户，$2 是 scope。
func refreshWaveStatus(ctx context.Context, executor bentopg.Executor, tenant domain.TenantID, which, scope string) error {
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.wave AS target SET status = `+waveStatusOfTasks+`
		  WHERE target.tenant_id = $1 AND `+which,
		tenant.String(), scope); err != nil {
		return fmt.Errorf("refresh wave status: %w", err)
	}
	return nil
}

func (store *Waves) LoadWaveMembers(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]ports.WaveMember, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, err
	}
	var exists bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2)`,
		tenant.String(), id.String()).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("load wave members: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := querier.Query(ctx,
		`SELECT request_id, withdrawn_at IS NOT NULL FROM warehouse_fulfillment.wave_request
		  WHERE tenant_id = $1 AND wave_id = $2
		  ORDER BY request_id`, tenant.String(), id.String())
	if err != nil {
		return nil, false, fmt.Errorf("load wave members: %w", err)
	}
	defer rows.Close()
	var members []ports.WaveMember
	for rows.Next() {
		var raw string
		var withdrawn bool
		if err := rows.Scan(&raw, &withdrawn); err != nil {
			return nil, false, fmt.Errorf("load wave members: %w", err)
		}
		request, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return nil, false, err
		}
		members = append(members, ports.WaveMember{Request: request, Withdrawn: withdrawn})
	}
	return members, true, rows.Err()
}

// WithdrawFromWave 三句各带自己的前置条件：成员行还没撤过、请求仍是已放波次。有一句没改到行就整笔不成立。
// 拣货与放波次排在编排先拿到的入账次序里，改不到行的只会是不走那道次序的截单抢在了前面：请求已经不是已放波次，
// 这一笔连同前面作废的任务一起退回，截单的结果不动。
func (store *Waves) WithdrawFromWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, request domain.OutboundRequestID, operator domain.Registrant, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("withdraw from wave: %w", err)
	}
	if err := lockWaves(ctx, executor, tenant, `target.wave_id = $2`, id.String()); err != nil {
		return err
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.wave_request
		    SET withdrawn_at = $4, withdrawn_by = $5
		  WHERE tenant_id = $1 AND wave_id = $2 AND request_id = $3 AND withdrawn_at IS NULL`,
		tenant.String(), id.String(), request.String(), at.UTC(), operator.String())
	if err != nil {
		return fmt.Errorf("withdraw from wave: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("withdraw from wave: %s was not a standing member of %s", request.String(), id.String())
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task SET status = 'CANCELLED'
		  WHERE tenant_id = $1 AND wave_id = $2 AND request_id = $3 AND status = 'OPEN'`,
		tenant.String(), id.String(), request.String()); err != nil {
		return fmt.Errorf("withdraw from wave: %w", err)
	}
	tag, err = executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'RESERVED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'ALLOCATED'`,
		tenant.String(), request.String())
	if err != nil {
		return fmt.Errorf("withdraw from wave: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("withdraw from wave: %s was not allocated", request.String())
	}
	return refreshWaveStatus(ctx, executor, tenant, `target.wave_id = $2`, id.String())
}

func (store *Waves) LoadTask(ctx context.Context, tenant domain.TenantID, id domain.PickTaskID) (ports.PickTask, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PickTask{}, false, err
	}
	task, err := scanPickTask(querier.QueryRow(ctx,
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id, staging_location, label_alias
		   FROM warehouse_fulfillment.pick_task
		  WHERE tenant_id = $1 AND task_id = $2`, tenant.String(), id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PickTask{}, false, nil
	}
	if err != nil {
		return ports.PickTask{}, false, fmt.Errorf("load pick task: %w", err)
	}
	return task, true, nil
}

func (store *Waves) TasksOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PickTask, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id, staging_location, label_alias
		   FROM warehouse_fulfillment.pick_task
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY task_id`, tenant.String(), request.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []ports.PickTask
	for rows.Next() {
		task, err := scanPickTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// OpenAllocation 是这一格上状态仍为 OPEN 的拣货任务还没拣走的数量。拣完、短拣、取消与迟到都不再占。
func (store *Waves) OpenAllocation(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("open allocation: %w", err)
	}
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(task.quantity - task.picked), 0)::bigint
		   FROM warehouse_fulfillment.pick_task task
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
		   JOIN warehouse_fulfillment.wave wave
		     ON wave.tenant_id = task.tenant_id AND wave.wave_id = task.wave_id
		  WHERE task.tenant_id = $1 AND request.owner_account_id = $2 AND wave.warehouse_code = $3
		    AND task.status = 'OPEN'
		    AND task.location_code = $4 AND task.goods_code = $5 AND task.unit_name = $6 AND task.stock_state = $7
		    AND task.label_alias IS NOT DISTINCT FROM $8 AND task.inbound_batch_id = $9`,
		tenant.String(), owner.String(), warehouse.String(),
		cell.Location().String(), cell.Goods().String(), cell.Unit().String(), cell.State().String(),
		optionalText(cell.Alias().String(), cell.Alias().String() != ""), cell.InboundBatch().String(),
	).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("open allocation: %w", err)
	}
	return quantity, nil
}

func (store *Waves) OpenTasksOn(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) ([]ports.PickTask, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("open pick tasks: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT task.task_id, task.wave_id, task.request_id, task.location_code, task.goods_code, task.unit_name, task.stock_state,
		        task.inbound_batch_id, task.quantity, task.picked, task.status, task.fact_id, task.staging_location, task.label_alias
		   FROM warehouse_fulfillment.pick_task task
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
		   JOIN warehouse_fulfillment.wave wave
		     ON wave.tenant_id = task.tenant_id AND wave.wave_id = task.wave_id
		  WHERE task.tenant_id = $1 AND request.owner_account_id = $2 AND wave.warehouse_code = $3
		    AND task.status = 'OPEN'
		    AND task.location_code = $4 AND task.goods_code = $5 AND task.unit_name = $6 AND task.stock_state = $7
		    AND task.label_alias IS NOT DISTINCT FROM $8 AND task.inbound_batch_id = $9
		  ORDER BY task.task_id`,
		tenant.String(), owner.String(), warehouse.String(),
		cell.Location().String(), cell.Goods().String(), cell.Unit().String(), cell.State().String(),
		optionalText(cell.Alias().String(), cell.Alias().String() != ""), cell.InboundBatch().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("open pick tasks: %w", err)
	}
	defer rows.Close()
	var tasks []ports.PickTask
	for rows.Next() {
		task, err := scanPickTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (store *Waves) SaveOpenTask(ctx context.Context, tenant domain.TenantID, task ports.PickTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save open pick: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task
		    SET location_code = $3, quantity = $4
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'OPEN'`,
		tenant.String(), task.ID.String(), task.Location.String(), task.Quantity)
	if err != nil {
		return fmt.Errorf("save open pick: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save open pick: %s was not open", task.ID.String())
	}
	return nil
}

func (store *Waves) InsertOpenTask(ctx context.Context, tenant domain.TenantID, task ports.PickTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("insert open pick: %w", err)
	}
	if err := lockWaves(ctx, executor, tenant, `target.wave_id = $2`, task.WaveID.String()); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.pick_task (
		    tenant_id, task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state,
		    inbound_batch_id, quantity, picked, status, fact_id, label_alias
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,'OPEN',NULL,$11)`,
		tenant.String(), task.ID.String(), task.WaveID.String(), task.Request.String(), task.Location.String(),
		task.Goods.String(), task.Unit.String(), task.State.String(), task.Inbound.String(), task.Quantity,
		optionalText(task.Alias.String(), task.Alias.String() != "")); err != nil {
		return fmt.Errorf("insert open pick: %w", err)
	}
	return refreshWaveStatus(ctx, executor, tenant, `target.wave_id = $2`, task.WaveID.String())
}

func (store *Waves) SavePick(ctx context.Context, tenant domain.TenantID, task ports.PickTask, requestStatus domain.OutboundStatus, differenceID, factID string, short int64, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	if err := lockWaves(ctx, executor, tenant, wavesOfRequest, task.Request.String()); err != nil {
		return err
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task
		    SET picked = $3, status = $4, fact_id = $5, staging_location = NULLIF($6, '')
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'OPEN'`,
		tenant.String(), task.ID.String(), task.Picked, task.Status, factID, task.Staging.String())
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save pick: %s was not open", task.ID.String())
	}
	if err := reduceReservation(ctx, executor, tenant, task, task.Picked); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request
		    SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), task.Request.String(), requestStatus.String()); err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	if short > 0 {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.pick_difference (tenant_id, difference_id, task_id, fact_id, quantity, status, recorded_at)
			 VALUES ($1,$2,$3,$4,$5,'PENDING',$6)`,
			tenant.String(), differenceID, task.ID.String(), factID, short, at.UTC()); err != nil {
			return fmt.Errorf("save pick difference: %w", err)
		}
	}
	return refreshWaveStatus(ctx, executor, tenant, wavesOfRequest, task.Request.String())
}

var _ ports.PickDifferences = (*Waves)(nil)

func (store *Waves) ShortSpots(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.ShortSpot, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("short spots: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT task.location_code, task.goods_code
		   FROM warehouse_fulfillment.pick_difference difference
		   JOIN warehouse_fulfillment.pick_task task
		     ON task.tenant_id = difference.tenant_id AND task.task_id = difference.task_id
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
		  WHERE difference.tenant_id = $1 AND difference.status = 'PENDING'
		    AND request.owner_account_id = $2 AND request.warehouse_code = $3
		  ORDER BY task.location_code, task.goods_code`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, fmt.Errorf("short spots: %w", err)
	}
	defer rows.Close()
	var spots []ports.ShortSpot
	for rows.Next() {
		var location, goods string
		if err := rows.Scan(&location, &goods); err != nil {
			return nil, fmt.Errorf("short spots: %w", err)
		}
		parsedLocation, err := domain.NewLocationCode(location)
		if err != nil {
			return nil, err
		}
		parsedGoods, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		spots = append(spots, ports.ShortSpot{Location: parsedLocation, Goods: parsedGoods})
	}
	return spots, rows.Err()
}

// ClosePickDifferences 了结 closure 圈出的待处理拣货差异，记下是哪一份盘点了结的。
func (store *Waves) ClosePickDifferences(ctx context.Context, tenant domain.TenantID, closure ports.PickDifferenceClosure) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("close pick differences: %w", err)
	}
	places := make([]string, 0, len(closure.Locations))
	for _, location := range closure.Locations {
		places = append(places, location.String())
	}
	codes := make([]string, 0, len(closure.Goods))
	for _, code := range closure.Goods {
		codes = append(codes, code.String())
	}
	keptOwners := make([]string, 0, len(closure.Keep))
	keptPlaces := make([]string, 0, len(closure.Keep))
	keptGoods := make([]string, 0, len(closure.Keep))
	for _, spot := range closure.Keep {
		keptOwners = append(keptOwners, spot.Owner.String())
		keptPlaces = append(keptPlaces, spot.Location.String())
		keptGoods = append(keptGoods, spot.Goods.String())
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_difference AS difference
		    SET status = 'CLOSED', closed_at = $3, closed_by_count = $4
		   FROM warehouse_fulfillment.pick_task task
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
		  WHERE difference.tenant_id = $1 AND difference.status = 'PENDING'
		    AND task.tenant_id = difference.tenant_id AND task.task_id = difference.task_id
		    AND request.warehouse_code = $2 AND task.location_code = ANY($5::text[])
		    AND ($6 = '' OR request.owner_account_id = $6)
		    AND (cardinality($7::text[]) = 0 OR task.goods_code = ANY($7::text[]))
		    AND (difference.recorded_at IS NULL OR difference.recorded_at <= $8)
		    AND NOT EXISTS (
		        SELECT 1 FROM unnest($9::text[], $10::text[], $11::text[]) AS kept(owner_account_id, location_code, goods_code)
		         WHERE kept.owner_account_id = request.owner_account_id
		           AND kept.location_code = task.location_code AND kept.goods_code = task.goods_code)`,
		tenant.String(), closure.Warehouse.String(), closure.At.UTC(), closure.Count, places, closure.Owner.String(), codes,
		closure.Before.UTC(), keptOwners, keptPlaces, keptGoods); err != nil {
		return fmt.Errorf("close pick differences: %w", err)
	}
	return nil
}

func (store *Waves) SaveCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, observed int64, matched bool, at time.Time, next domain.OutboundStatus) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_check (tenant_id, fact_id, request_id, observed, matched, checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), fact.String(), request.String(), observed, matched, at); err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'PICKED'`,
		tenant.String(), request.String(), next.String()); err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	return nil
}

func (store *Waves) LoadCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (int64, bool, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, false, false, err
	}
	var observed int64
	var matched bool
	err = querier.QueryRow(ctx,
		`SELECT observed, matched FROM warehouse_fulfillment.outbound_check WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), fact.String()).Scan(&observed, &matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("load outbound check: %w", err)
	}
	return observed, matched, true, nil
}

func (store *Waves) SavePack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, parcels []ports.PackParcel, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_pack (tenant_id, fact_id, request_id, packed_at) VALUES ($1,$2,$3,$4)`,
		tenant.String(), fact.String(), request.String(), at); err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	for _, parcel := range parcels {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_parcel (
			    tenant_id, request_id, parcel_sequence, packaging_material, weight_value, weight_unit,
			    length_value, width_value, height_value, dimension_unit
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			tenant.String(), request.String(), parcel.Sequence, parcel.Packaging, parcel.Weight, parcel.WeightUnit,
			parcel.Length, parcel.Width, parcel.Height, parcel.Dimension); err != nil {
			return fmt.Errorf("save parcel: %w", err)
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_parcel_line (
			    tenant_id, request_id, parcel_sequence, line_index, goods_code, unit_name, quantity
			 ) VALUES ($1,$2,$3,0,$4,$5,$6)`,
			tenant.String(), request.String(), parcel.Sequence, parcel.Goods.String(), parcel.Unit.String(), parcel.Quantity); err != nil {
			return fmt.Errorf("save parcel line: %w", err)
		}
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'PACKED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'CHECKED'`,
		tenant.String(), request.String())
	if err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save pack: %s was not checked", request.String())
	}
	return nil
}

func (store *Waves) ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT parcel.parcel_sequence, parcel.packaging_material, parcel.weight_value, parcel.weight_unit,
		        parcel.length_value, parcel.width_value, parcel.height_value, parcel.dimension_unit,
		        parcel.shipment_request_id, parcel.declared_parcel_id, parcel.commission_state, parcel.correction_state,
		        line.goods_code, line.unit_name, line.quantity
		   FROM warehouse_fulfillment.outbound_parcel AS parcel
		   JOIN warehouse_fulfillment.outbound_parcel_line AS line
		     ON line.tenant_id = parcel.tenant_id AND line.request_id = parcel.request_id
		    AND line.parcel_sequence = parcel.parcel_sequence AND line.line_index = 0
		  WHERE parcel.tenant_id = $1 AND parcel.request_id = $2
		  ORDER BY parcel.parcel_sequence`,
		tenant.String(), request.String())
	if err != nil {
		return nil, fmt.Errorf("list parcels: %w", err)
	}
	defer rows.Close()
	var parcels []ports.PackParcel
	for rows.Next() {
		var parcel ports.PackParcel
		var shipment, declared, commission, correction *string
		var goods, unit string
		if err := rows.Scan(&parcel.Sequence, &parcel.Packaging, &parcel.Weight, &parcel.WeightUnit,
			&parcel.Length, &parcel.Width, &parcel.Height, &parcel.Dimension, &shipment, &declared, &commission, &correction, &goods, &unit, &parcel.Quantity); err != nil {
			return nil, fmt.Errorf("list parcels: %w", err)
		}
		if shipment != nil {
			parcel.ShipmentRequestID = *shipment
		}
		if declared != nil {
			parcel.DeclaredParcelID = *declared
		}
		if commission != nil {
			parcel.CommissionState = *commission
		}
		if correction != nil {
			parcel.CorrectionState = *correction
		}
		parcel.Goods, err = domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		parcel.Unit, err = domain.NewGoodsUnitName(unit)
		if err != nil {
			return nil, err
		}
		parcels = append(parcels, parcel)
	}
	return parcels, rows.Err()
}

func (store *Waves) MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID, declaredParcelID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("mark shipment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel
		    SET shipment_request_id = $4, declared_parcel_id = NULLIF($5, '')
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND shipment_request_id IS NULL`,
		tenant.String(), request.String(), sequence, shipmentRequestID, declaredParcelID)
	if err != nil {
		return fmt.Errorf("mark shipment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark shipment: parcel %d of %s was already submitted", sequence, request.String())
	}
	return nil
}

func (store *Waves) LoadAcceptance(ctx context.Context, tenant domain.TenantID, decisionID string) (string, string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", "", false, err
	}
	var source, key, state string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key, state FROM warehouse_fulfillment.acceptance_posting
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), decisionID).Scan(&source, &key, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("load acceptance: %w", err)
	}
	return source, key, state, true, nil
}

func (store *Waves) SaveAcceptance(ctx context.Context, tenant domain.TenantID, decisionID, source, key, state string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save acceptance: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.acceptance_posting (
		    tenant_id, decision_id, source, source_request_key, state, posted_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), decisionID, source, key, state, at); err != nil {
		return fmt.Errorf("save acceptance: %w", err)
	}
	return nil
}

func (store *Waves) SetCommissionState(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set commission state: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET commission_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3
		    AND shipment_request_id IS NOT NULL
		    AND (commission_state IS NULL OR commission_state = 'AWAITING_SUPPLEMENT')`,
		tenant.String(), request.String(), sequence, state)
	if err != nil {
		return fmt.Errorf("set commission state: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("set commission state: parcel %d of %s was not awaiting a decision", sequence, request.String())
	}
	return nil
}

func (store *Waves) LoadBasis(ctx context.Context, tenant domain.TenantID, sourceKey string) (string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var version string
	err = querier.QueryRow(ctx,
		`SELECT submission_version_id FROM warehouse_fulfillment.supplement_posting
		  WHERE tenant_id = $1 AND source_request_key = $2
		  ORDER BY posted_at DESC LIMIT 1`,
		tenant.String(), sourceKey).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load supplement basis: %w", err)
	}
	return version, true, nil
}

func (store *Waves) ClearAwaiting(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("clear awaiting supplement: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET commission_state = NULL
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND commission_state = 'AWAITING_SUPPLEMENT'`,
		tenant.String(), request.String(), sequence)
	if err != nil {
		return fmt.Errorf("clear awaiting supplement: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("clear awaiting supplement: parcel %d of %s was not awaiting", sequence, request.String())
	}
	return nil
}

func (store *Waves) SetCorrection(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set correction: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET correction_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence, state); err != nil {
		return fmt.Errorf("set correction: %w", err)
	}
	return nil
}

func (store *Waves) ParcelByDeclared(ctx context.Context, tenant domain.TenantID, declaredParcelID string) (domain.OutboundRequestID, int, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, err
	}
	var request string
	var sequence int
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND declared_parcel_id = $2`,
		tenant.String(), declaredParcelID).Scan(&request, &sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OutboundRequestID{}, 0, false, nil
	}
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, fmt.Errorf("find declared parcel: %w", err)
	}
	id, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, err
	}
	return id, sequence, true, nil
}

func (store *Waves) LoadSourceData(ctx context.Context, tenant domain.TenantID, version string) (string, string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", "", false, err
	}
	var source, key, adoption string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key, adoption FROM warehouse_fulfillment.source_data_posting
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant.String(), version).Scan(&source, &key, &adoption)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("load source data: %w", err)
	}
	return source, key, adoption, true, nil
}

func (store *Waves) SaveSourceData(ctx context.Context, tenant domain.TenantID, version, source, key, adoption string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save source data: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.source_data_posting (
		    tenant_id, version_id, source, source_request_key, adoption, posted_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), version, source, key, adoption, at); err != nil {
		return fmt.Errorf("save source data: %w", err)
	}
	return nil
}

func (store *Waves) LoadCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string) (bool, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, false, err
	}
	var matched bool
	err = querier.QueryRow(ctx,
		`SELECT matched FROM warehouse_fulfillment.cancellation_posting
		  WHERE tenant_id = $1 AND request_key = $2 AND parcel_id = $3`,
		tenant.String(), requestKey, parcelID).Scan(&matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("load cancellation: %w", err)
	}
	return matched, true, nil
}

func (store *Waves) SaveCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string, matched bool, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save cancellation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.cancellation_posting (
		    tenant_id, request_key, parcel_id, matched, posted_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		tenant.String(), requestKey, parcelID, matched, at); err != nil {
		return fmt.Errorf("save cancellation: %w", err)
	}
	return nil
}

func (store *Waves) LoadSupplement(ctx context.Context, tenant domain.TenantID, version string) (string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", false, err
	}
	var source, key string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key FROM warehouse_fulfillment.supplement_posting
		  WHERE tenant_id = $1 AND submission_version_id = $2`,
		tenant.String(), version).Scan(&source, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load supplement: %w", err)
	}
	return source, key, true, nil
}

func (store *Waves) SaveSupplement(ctx context.Context, tenant domain.TenantID, version, source, key string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save supplement: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.supplement_posting (
		    tenant_id, submission_version_id, source, source_request_key, posted_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		tenant.String(), version, source, key, at); err != nil {
		return fmt.Errorf("save supplement: %w", err)
	}
	return nil
}

func (store *Waves) LoadPack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var found bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.outbound_pack WHERE tenant_id = $1 AND fact_id = $2)`,
		tenant.String(), fact.String()).Scan(&found)
	return found, err
}

// reduceReservation 把拣走的数量从这张请求的预占里扣掉。先扣这条任务拣的那种货的行（商品、库存状态、标签别名相同），
// 再扣别的行：一张请求有几种货时，拣哪种扣哪种，行上剩的才是那种货还没拣到的数量——少拣之后回到已预占再放，
// 分配照行上的数量来，扣错了行就会给拣齐的那种货再分一遍，缺的那种反而不分。别的行只在对得上的行不够扣时才动，
// 合计仍等于还没拣到的数量。
func reduceReservation(ctx context.Context, executor bentopg.Executor, tenant domain.TenantID, task ports.PickTask, picked int64) error {
	request := task.Request
	rows, err := executor.Query(ctx,
		`SELECT line_index, quantity
		   FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY (goods_code = $3 AND stock_state = $4 AND label_alias IS NOT DISTINCT FROM $5) DESC, line_index`,
		tenant.String(), request.String(), task.Goods.String(), task.State.String(),
		optionalText(task.Alias.String(), task.Alias.String() != ""))
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	defer rows.Close()
	type row struct {
		index, quantity int64
	}
	var listed []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.index, &item.quantity); err != nil {
			rows.Close()
			return err
		}
		listed = append(listed, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	left := picked
	for _, item := range listed {
		take := item.quantity
		if take > left {
			take = left
		}
		if _, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_reservation
			    SET quantity = $4, from_unlabeled = LEAST(from_unlabeled, $4)
			  WHERE tenant_id = $1 AND request_id = $2 AND line_index = $3`,
			tenant.String(), request.String(), item.index, item.quantity-take); err != nil {
			return fmt.Errorf("save pick: %w", err)
		}
		left -= take
	}
	return nil
}

type pickRowScanner interface {
	Scan(dest ...any) error
}

func scanPickTask(row pickRowScanner) (ports.PickTask, error) {
	var task, wave, request, location, goods, unit, state, inbound, status string
	var fact, staging, alias *string
	var quantity, picked int64
	if err := row.Scan(&task, &wave, &request, &location, &goods, &unit, &state, &inbound, &quantity, &picked, &status, &fact, &staging, &alias); err != nil {
		return ports.PickTask{}, err
	}
	parsedTask, err := domain.NewPickTaskID(task)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedWave, err := domain.NewWaveID(wave)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedRequest, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedLocation, err := domain.NewLocationCode(location)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedGoods, err := domain.NewGoodsCode(goods)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedUnit, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedState, err := domain.NewStockState(state)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedInbound, err := domain.NewInboundBatchID(inbound)
	if err != nil {
		return ports.PickTask{}, err
	}
	var parsedStaging domain.LocationCode
	if staging != nil && *staging != "" {
		parsedStaging, err = domain.NewLocationCode(*staging)
		if err != nil {
			return ports.PickTask{}, err
		}
	}
	var parsedAlias domain.GoodsAliasCode
	if alias != nil && *alias != "" {
		parsedAlias, err = domain.NewGoodsAliasCode(*alias)
		if err != nil {
			return ports.PickTask{}, err
		}
	}
	return ports.PickTask{
		ID: parsedTask, WaveID: parsedWave, Request: parsedRequest, Location: parsedLocation, Staging: parsedStaging, Goods: parsedGoods,
		Unit: parsedUnit, State: parsedState, Alias: parsedAlias, Inbound: parsedInbound, Quantity: quantity, Picked: picked, Status: status,
		Fact: valueOrEmpty(fact),
	}, nil
}

func (store *Waves) LoadConfirmation(ctx context.Context, tenant domain.TenantID, factID string) (ports.OutboundConfirmation, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OutboundConfirmation{}, false, err
	}
	var request, kind, missing string
	var sequence int
	var offline bool
	var external, party, credential *string
	var occurred time.Time
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence, kind, offline, missing, external_number, pickup_party, pickup_credential, occurred_at
		   FROM warehouse_fulfillment.outbound_confirmation
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&request, &sequence, &kind, &offline, &missing, &external, &party, &credential, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OutboundConfirmation{}, false, nil
	}
	if err != nil {
		return ports.OutboundConfirmation{}, false, fmt.Errorf("load outbound confirmation: %w", err)
	}
	parsed, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.OutboundConfirmation{}, false, err
	}
	return ports.OutboundConfirmation{
		FactID: factID, Request: parsed, Sequence: sequence, Kind: kind, Offline: offline, Missing: missing,
		ExternalNumber: valueOrEmpty(external), PickupParty: valueOrEmpty(party), PickupCredential: valueOrEmpty(credential),
		OccurredAt: occurred,
	}, true, nil
}

func (store *Waves) SaveConfirmation(ctx context.Context, tenant domain.TenantID, confirmation ports.OutboundConfirmation) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_confirmation (
		    tenant_id, fact_id, request_id, parcel_sequence, kind, offline, missing,
		    external_number, pickup_party, pickup_credential, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7, NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), $11)`,
		tenant.String(), confirmation.FactID, confirmation.Request.String(), confirmation.Sequence, confirmation.Kind,
		confirmation.Offline, confirmation.Missing, confirmation.ExternalNumber, confirmation.PickupParty,
		confirmation.PickupCredential, confirmation.OccurredAt); err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET confirmation_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND confirmation_state IS NULL`,
		tenant.String(), confirmation.Request.String(), confirmation.Sequence, confirmation.State)
	if err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save outbound confirmation: parcel %d of %s was already confirmed", confirmation.Sequence, confirmation.Request.String())
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'SHIPPED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'PACKED'
		    AND NOT EXISTS (
		        SELECT 1 FROM warehouse_fulfillment.outbound_parcel
		         WHERE tenant_id = $1 AND request_id = $2 AND confirmation_state IS NULL)`,
		tenant.String(), confirmation.Request.String()); err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	return nil
}

func (store *Waves) ParcelConfirmed(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var state *string
	err = querier.QueryRow(ctx,
		`SELECT confirmation_state FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load parcel confirmation: %w", err)
	}
	return state != nil && *state != "", nil
}

func (store *Waves) LoadOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var number string
	err = querier.QueryRow(ctx,
		`SELECT external_number FROM warehouse_fulfillment.outbound_owner_label
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence).Scan(&number)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load owner label: %w", err)
	}
	return number, true, nil
}

func (store *Waves) SaveOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, externalNumber string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save owner label: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_owner_label (tenant_id, request_id, parcel_sequence, external_number)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), request.String(), sequence, externalNumber); err != nil {
		return fmt.Errorf("save owner label: %w", err)
	}
	return nil
}

func (store *Waves) OwnerLabelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (map[int]string, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT parcel_sequence, external_number FROM warehouse_fulfillment.outbound_owner_label
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String())
	if err != nil {
		return nil, fmt.Errorf("list owner labels: %w", err)
	}
	defer rows.Close()
	labels := map[int]string{}
	for rows.Next() {
		var sequence int
		var number string
		if err := rows.Scan(&sequence, &number); err != nil {
			return nil, err
		}
		labels[sequence] = number
	}
	return labels, rows.Err()
}

func (store *Waves) LoadCorrection(ctx context.Context, tenant domain.TenantID, factID string) (ports.ConfirmationCorrection, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ConfirmationCorrection{}, false, err
	}
	var kind, operator, status string
	var occurred *time.Time
	err = querier.QueryRow(ctx,
		`SELECT kind, occurred_at, operator_id, status FROM warehouse_fulfillment.outbound_confirmation_correction
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&kind, &occurred, &operator, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConfirmationCorrection{}, false, nil
	}
	if err != nil {
		return ports.ConfirmationCorrection{}, false, fmt.Errorf("load confirmation correction: %w", err)
	}
	correction := ports.ConfirmationCorrection{FactID: factID, Kind: kind, Operator: operator, Status: status}
	if occurred != nil {
		correction.OccurredAt = *occurred
	}
	return correction, true, nil
}

func (store *Waves) SaveCorrection(ctx context.Context, tenant domain.TenantID, correction ports.ConfirmationCorrection) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save confirmation correction: %w", err)
	}
	var occurred *time.Time
	if !correction.OccurredAt.IsZero() {
		at := correction.OccurredAt.UTC()
		occurred = &at
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_confirmation_correction (
		    tenant_id, fact_id, kind, occurred_at, operator_id, status)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), correction.FactID, correction.Kind, occurred, correction.Operator, correction.Status); err != nil {
		return fmt.Errorf("save confirmation correction: %w", err)
	}
	if correction.Kind == "TIME" {
		if _, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_confirmation SET occurred_at = $3
			  WHERE tenant_id = $1 AND fact_id = $2`,
			tenant.String(), correction.FactID, correction.OccurredAt.UTC()); err != nil {
			return fmt.Errorf("save confirmation correction: %w", err)
		}
	}
	return nil
}

func (store *Waves) PostReplenishment(ctx context.Context, tenant domain.TenantID, factID string, approver domain.Registrant, dutyRevision int, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_confirmation_correction
		    SET status = 'POSTED', approver_id = $3, duty_revision = $4, approved_at = $5
		  WHERE tenant_id = $1 AND fact_id = $2 AND status = 'PENDING'`,
		tenant.String(), factID, approver.String(), dutyRevision, at.UTC())
	if err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("post replenishment: %s was not pending", factID)
	}
	var request string
	var sequence int
	if err := executor.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_confirmation
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&request, &sequence); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET confirmation_state = NULL
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request, sequence); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'PACKED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'SHIPPED'`,
		tenant.String(), request); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	return nil
}

func (store *Waves) LoadCutoff(ctx context.Context, tenant domain.TenantID, key string) (string, int, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", 0, false, err
	}
	var request string
	var sequence *int
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_cutoff
		  WHERE tenant_id = $1 AND cutoff_key = $2`,
		tenant.String(), key).Scan(&request, &sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("load cutoff: %w", err)
	}
	if sequence == nil {
		return request, 0, true, nil
	}
	return request, *sequence, true, nil
}

func (store *Waves) SaveCutoff(ctx context.Context, tenant domain.TenantID, key, requestID string, sequence int, basis, outcome string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save cutoff: %w", err)
	}
	var parcel *int
	if sequence > 0 {
		parcel = &sequence
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_cutoff (
		    tenant_id, cutoff_key, request_id, parcel_sequence, basis, outcome, posted_at)
		 VALUES ($1,$2,$3,$4, NULLIF($5,''), $6, $7)`,
		tenant.String(), key, requestID, parcel, basis, outcome, at.UTC()); err != nil {
		return fmt.Errorf("save cutoff: %w", err)
	}
	return nil
}

func (store *Waves) LoadShortPickDisposal(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ports.ShortPickDisposal, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ShortPickDisposal{}, false, err
	}
	disposal := ports.ShortPickDisposal{Request: request}
	var remainder *string
	err = querier.QueryRow(ctx,
		`SELECT partial_shipment, partial_shipment_source, wanted, shipped, remainder_request_id, operator_id, recorded_at
		   FROM warehouse_fulfillment.outbound_short_pick_disposal
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String()).Scan(&disposal.Rule, &disposal.Source, &disposal.Wanted, &disposal.Shipped,
		&remainder, &disposal.Operator, &disposal.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ShortPickDisposal{}, false, nil
	}
	if err != nil {
		return ports.ShortPickDisposal{}, false, fmt.Errorf("load short pick disposal: %w", err)
	}
	disposal.Remainder = valueOrEmpty(remainder)
	return disposal, true, nil
}

// SaveShortPickDisposal 三件事在一处做：落册、放开没拣到的预占、请求进已拣货。请求不是已预占时整份不成立——
// 调用方在拿到入账次序之后读过状态，这里再认一次是防它在别处被改成了别的写法。
func (store *Waves) SaveShortPickDisposal(ctx context.Context, tenant domain.TenantID, disposal ports.ShortPickDisposal) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save short pick disposal: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_short_pick_disposal (
		    tenant_id, request_id, partial_shipment, partial_shipment_source, wanted, shipped,
		    remainder_request_id, operator_id, recorded_at)
		 VALUES ($1,$2,$3,$4,$5,$6, NULLIF($7,''), $8, $9)`,
		tenant.String(), disposal.Request.String(), disposal.Rule, disposal.Source, disposal.Wanted, disposal.Shipped,
		disposal.Remainder, disposal.Operator, disposal.RecordedAt.UTC()); err != nil {
		return fmt.Errorf("save short pick disposal: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_reservation
		    SET quantity = 0, from_unlabeled = 0
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), disposal.Request.String()); err != nil {
		return fmt.Errorf("save short pick disposal: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'PICKED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'RESERVED'`,
		tenant.String(), disposal.Request.String())
	if err != nil {
		return fmt.Errorf("save short pick disposal: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save short pick disposal: %s was not reserved", disposal.Request.String())
	}
	return nil
}

func (store *Waves) ReleaseReservations(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("release reservations: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_reservation
		    SET quantity = 0, from_unlabeled = 0
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String()); err != nil {
		return fmt.Errorf("release reservations: %w", err)
	}
	return nil
}

func (store *Waves) CancelOpenTasks(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("cancel open tasks: %w", err)
	}
	if err := lockWaves(ctx, executor, tenant, wavesOfRequest, request.String()); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task SET status = 'CANCELLED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'OPEN'`,
		tenant.String(), request.String()); err != nil {
		return fmt.Errorf("cancel open tasks: %w", err)
	}
	return refreshWaveStatus(ctx, executor, tenant, wavesOfRequest, request.String())
}

func (store *Waves) SaveReturnTask(ctx context.Context, tenant domain.TenantID, task ports.ReturnTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save return task: %w", err)
	}
	var sequence *int
	if task.Sequence > 0 {
		value := task.Sequence
		sequence = &value
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.return_task (
		    tenant_id, return_id, request_id, parcel_sequence, location_code, goods_code, unit_name,
		    stock_state, inbound_batch_id, quantity, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'OPEN')`,
		tenant.String(), task.ID, task.Request.String(), sequence, task.Location.String(), task.Goods.String(),
		task.Unit.String(), task.State.String(), task.Inbound.String(), task.Quantity); err != nil {
		return fmt.Errorf("save return task: %w", err)
	}
	return nil
}

func (store *Waves) SavePrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, record ports.PrefetchRecord) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save prefetch: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request
		    SET prefetch_shipment_id = $3,
		        prefetch_declared_parcel_id = NULLIF($4, ''),
		        prefetch_weight = NULLIF($5, ''),
		        prefetch_weight_unit = NULLIF($6, ''),
		        prefetch_length = NULLIF($7, ''),
		        prefetch_width = NULLIF($8, ''),
		        prefetch_height = NULLIF($9, ''),
		        prefetch_dimension_unit = NULLIF($10, '')
		  WHERE tenant_id = $1 AND request_id = $2 AND prefetch_shipment_id IS NULL`,
		tenant.String(), request.String(), record.ShipmentID, record.DeclaredParcelID,
		record.Weight, record.WeightUnit, record.Length, record.Width, record.Height, record.Dimension)
	if err != nil {
		return fmt.Errorf("save prefetch: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save prefetch: %s already has a shipment", request.String())
	}
	return nil
}

func (store *Waves) LoadPrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ports.PrefetchRecord, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PrefetchRecord{}, false, err
	}
	var shipment, declared, weight, weightUnit, length, width, height, dimension *string
	err = querier.QueryRow(ctx,
		`SELECT prefetch_shipment_id, prefetch_declared_parcel_id,
		        prefetch_weight, prefetch_weight_unit, prefetch_length, prefetch_width, prefetch_height, prefetch_dimension_unit
		   FROM warehouse_fulfillment.outbound_request
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String()).Scan(&shipment, &declared, &weight, &weightUnit, &length, &width, &height, &dimension)
	if errors.Is(err, pgx.ErrNoRows) || shipment == nil || *shipment == "" {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ports.PrefetchRecord{}, false, fmt.Errorf("load prefetch: %w", err)
		}
		return ports.PrefetchRecord{}, false, nil
	}
	if err != nil {
		return ports.PrefetchRecord{}, false, fmt.Errorf("load prefetch: %w", err)
	}
	stringOf := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	return ports.PrefetchRecord{
		ShipmentID:       *shipment,
		DeclaredParcelID: stringOf(declared),
		Weight:           stringOf(weight),
		WeightUnit:       stringOf(weightUnit),
		Length:           stringOf(length),
		Width:            stringOf(width),
		Height:           stringOf(height),
		Dimension:        stringOf(dimension),
	}, true, nil
}

func (store *Waves) SetRequestStatus(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, status domain.OutboundStatus) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set request status: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String(), status.String()); err != nil {
		return fmt.Errorf("set request status: %w", err)
	}
	return nil
}

func (store *Waves) MarkParcelCutoff(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (domain.OutboundStatus, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET cutoff_state = 'CANCELLED'
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3
		    AND cutoff_state IS NULL AND confirmation_state IS NULL`,
		tenant.String(), request.String(), sequence)
	if err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: parcel %d of %s was not open", sequence, request.String())
	}
	var remaining int
	if err := executor.QueryRow(ctx,
		`SELECT count(*) FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND cutoff_state IS NULL AND confirmation_state IS NULL`,
		tenant.String(), request.String()).Scan(&remaining); err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	next := domain.OutboundPartiallyCancelled
	if remaining == 0 {
		next = domain.OutboundCancelled
	}
	if err := store.SetRequestStatus(ctx, tenant, request, next); err != nil {
		return domain.OutboundStatusInvalid, err
	}
	return next, nil
}

func (store *Waves) SaveLatePick(ctx context.Context, tenant domain.TenantID, task ports.PickTask, factID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save late pick: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task
		    SET picked = $3, status = 'LATE', fact_id = $4, staging_location = NULLIF($5, '')
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'CANCELLED'`,
		tenant.String(), task.ID.String(), task.Picked, factID, task.Staging.String())
	if err != nil {
		return fmt.Errorf("save late pick: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save late pick: %s was not cancelled", task.ID.String())
	}
	return nil
}

func (store *Waves) LoadReturnTask(ctx context.Context, tenant domain.TenantID, id string) (ports.ReturnTask, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	var request, location, goods, unit, state, inbound, status string
	var sequence *int
	var fact *string
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence, location_code, goods_code, unit_name, stock_state,
		        inbound_batch_id, quantity, status, fact_id
		   FROM warehouse_fulfillment.return_task
		  WHERE tenant_id = $1 AND return_id = $2`,
		tenant.String(), id).Scan(&request, &sequence, &location, &goods, &unit, &state, &inbound, &quantity, &status, &fact)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReturnTask{}, false, nil
	}
	if err != nil {
		return ports.ReturnTask{}, false, fmt.Errorf("load return task: %w", err)
	}
	parsedRequest, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedLocation, err := domain.NewLocationCode(location)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedGoods, err := domain.NewGoodsCode(goods)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedUnit, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedState, err := domain.NewStockState(state)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedInbound, err := domain.NewInboundBatchID(inbound)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	task := ports.ReturnTask{
		ID: id, Request: parsedRequest, Location: parsedLocation, Goods: parsedGoods, Unit: parsedUnit,
		State: parsedState, Inbound: parsedInbound, Quantity: quantity, Status: status, Fact: valueOrEmpty(fact),
	}
	if sequence != nil {
		task.Sequence = *sequence
	}
	return task, true, nil
}

func (store *Waves) CompleteReturn(ctx context.Context, tenant domain.TenantID, id, factID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("complete return: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.return_task SET status = 'DONE', fact_id = $3
		  WHERE tenant_id = $1 AND return_id = $2 AND status = 'OPEN'`,
		tenant.String(), id, factID)
	if err != nil {
		return fmt.Errorf("complete return: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("complete return: %s was not open", id)
	}
	return nil
}

func (store *Waves) OpenReturns(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	err = querier.QueryRow(ctx,
		`SELECT count(*) FROM warehouse_fulfillment.return_task
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'OPEN'`,
		tenant.String(), request.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open returns: %w", err)
	}
	return count, nil
}
