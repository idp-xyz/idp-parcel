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

// Counts 是盘点的 PostgreSQL 适配器。
type Counts struct {
	db *bentopg.DB
}

func NewCounts(db *bentopg.DB) (*Counts, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Counts{db: db}, nil
}

var _ ports.CountStore = (*Counts)(nil)

func (store *Counts) HoldDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) error {
	return store.hold(ctx, "hold count decision", "warehouse_fulfillment.stock_count.decision", tenant.String(), key.String())
}

func (store *Counts) HoldCount(ctx context.Context, tenant domain.TenantID, id domain.CountID) error {
	return store.hold(ctx, "hold count", "warehouse_fulfillment.stock_count", tenant.String(), id.String())
}

func (store *Counts) hold(ctx context.Context, operation, namespace, tenant, id string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	_, err = executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2 || chr(31) || $3))`, namespace, tenant, id)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *Counts) LoadByDecision(ctx context.Context, tenant domain.TenantID, key domain.RequestKey) (domain.Count, bool, error) {
	return store.loadWhere(ctx, `tenant_id = $1 AND decision_id = $2`, tenant.String(), key.String())
}

func (store *Counts) Load(ctx context.Context, tenant domain.TenantID, id domain.CountID) (domain.Count, bool, error) {
	return store.loadWhere(ctx, `tenant_id = $1 AND count_id = $2`, tenant.String(), id.String())
}

func (store *Counts) loadWhere(ctx context.Context, where string, args ...any) (domain.Count, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.Count{}, false, fmt.Errorf("load count: %w", err)
	}
	var tenant, id, decision, warehouse, operator, status string
	var owner, note, startedBy, completedBy, voidedBy, voidReason *string
	var created time.Time
	var started, completed, voided *time.Time
	err = querier.QueryRow(ctx,
		`SELECT tenant_id, count_id, decision_id, warehouse_code, owner_account_id, note, operator_id, created_at, status,
		        started_by, started_at, completed_by, completed_at, voided_by, voided_at, void_reason
		   FROM warehouse_fulfillment.stock_count WHERE `+where, args...).Scan(
		&tenant, &id, &decision, &warehouse, &owner, &note, &operator, &created, &status,
		&startedBy, &started, &completedBy, &completed, &voidedBy, &voided, &voidReason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Count{}, false, nil
	}
	if err != nil {
		return domain.Count{}, false, fmt.Errorf("load count: %w", err)
	}
	locations, err := store.codes(ctx, `SELECT location_code FROM warehouse_fulfillment.stock_count_location WHERE tenant_id = $1 AND count_id = $2 ORDER BY location_code`, tenant, id)
	if err != nil {
		return domain.Count{}, false, err
	}
	goods, err := store.codes(ctx, `SELECT goods_code FROM warehouse_fulfillment.stock_count_goods WHERE tenant_id = $1 AND count_id = $2 ORDER BY goods_code`, tenant, id)
	if err != nil {
		return domain.Count{}, false, err
	}
	spec := domain.CountSpec{
		Tenant: mustTenant(tenant), ID: mustCountID(id), DecisionID: mustRequest(&decision), Warehouse: mustWarehouse(warehouse),
		Locations: locationCodes(locations), Goods: goodsCodes(goods), Note: valueOrEmpty(note), Operator: mustRegistrant(&operator), CreatedAt: created,
	}
	if owner != nil {
		spec.Owner = mustOwnerID(*owner)
	}
	count, err := domain.NewCount(spec)
	if err != nil {
		return domain.Count{}, false, err
	}
	if status == "COUNTING" || status == "COUNTED" || status == "VOID" {
		when := created
		who := &operator
		if started != nil {
			when = *started
		}
		if startedBy != nil {
			who = startedBy
		}
		count, err = count.Start(mustRegistrant(who), when)
		if err != nil {
			return domain.Count{}, false, err
		}
	}
	if status == "COUNTED" {
		when := created
		who := &operator
		if completed != nil {
			when = *completed
		}
		if completedBy != nil {
			who = completedBy
		}
		count, err = count.Finish(mustRegistrant(who), when)
	}
	if status == "VOID" {
		when := created
		who := &operator
		if voided != nil {
			when = *voided
		}
		if voidedBy != nil {
			who = voidedBy
		}
		count, err = count.Void(mustRegistrant(who), when, valueOrEmpty(voidReason))
	}
	if err != nil {
		return domain.Count{}, false, err
	}
	return count, true, nil
}

func (store *Counts) codes(ctx context.Context, query, tenant, id string) ([]string, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx, query, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

func (store *Counts) Save(ctx context.Context, count domain.Count) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save count: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_count (
		    tenant_id, count_id, decision_id, warehouse_code, owner_account_id, note, operator_id, created_at, status
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PENDING')`,
		count.Tenant().String(), count.ID().String(), count.DecisionID().String(), count.Warehouse().String(),
		nullableText(count.Owner().String()), nullableText(count.Note()), count.Operator().String(), count.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("save count: %w", err)
	}
	for _, location := range count.Locations() {
		if _, err := executor.Exec(ctx, `INSERT INTO warehouse_fulfillment.stock_count_location (tenant_id, count_id, location_code) VALUES ($1,$2,$3)`,
			count.Tenant().String(), count.ID().String(), location.String()); err != nil {
			return fmt.Errorf("save count: %w", err)
		}
	}
	for _, goods := range count.Goods() {
		if _, err := executor.Exec(ctx, `INSERT INTO warehouse_fulfillment.stock_count_goods (tenant_id, count_id, goods_code) VALUES ($1,$2,$3)`,
			count.Tenant().String(), count.ID().String(), goods.String()); err != nil {
			return fmt.Errorf("save count: %w", err)
		}
	}
	return nil
}

func (store *Counts) SaveStart(ctx context.Context, count domain.Count, locks []domain.CountLock) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("start count: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_count SET status = 'COUNTING', started_by = $3, started_at = $4
		  WHERE tenant_id = $1 AND count_id = $2 AND status = 'PENDING'`,
		count.Tenant().String(), count.ID().String(), count.StartedBy().String(), count.StartedAt())
	if err != nil {
		return fmt.Errorf("start count: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("start count: count %s was not pending", count.ID().String())
	}
	for index, lock := range locks {
		cell := lock.Cell
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.stock_count_lock (
			    tenant_id, count_id, lock_index, owner_account_id, warehouse_code, location_code, goods_code, unit_name,
			    stock_state, label_alias, inbound_batch_id, batch_number, expiry_date, quantity
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::date,$14)`,
			count.Tenant().String(), count.ID().String(), index, lock.Owner.String(), count.Warehouse().String(), lock.Location.String(),
			cell.Goods().String(), cell.Unit().String(), cell.State().String(),
			optionalText(cell.Alias().String(), cell.Alias().String() != ""), cell.InboundBatch().String(),
			optionalText(cell.BatchNumber().String(), cell.BatchNumber().Given()), optionalText(cell.Expiry().String(), cell.Expiry().Given()),
			lock.Quantity,
		); err != nil {
			return fmt.Errorf("start count: %w", err)
		}
	}
	return nil
}

func (store *Counts) SaveVoid(ctx context.Context, count domain.Count) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("void count: %w", err)
	}
	_, err = executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_count
		    SET status = 'VOID', voided_by = $3, voided_at = $4, void_reason = $5
		  WHERE tenant_id = $1 AND count_id = $2 AND status IN ('PENDING', 'COUNTING')`,
		count.Tenant().String(), count.ID().String(), count.FinishedBy().String(), count.FinishedAt(), nullableText(count.VoidReason()))
	if err != nil {
		return fmt.Errorf("void count: %w", err)
	}
	_, err = executor.Exec(ctx, `DELETE FROM warehouse_fulfillment.stock_count_lock WHERE tenant_id = $1 AND count_id = $2`, count.Tenant().String(), count.ID().String())
	return err
}

func (store *Counts) SaveFinish(ctx context.Context, count domain.Count, differences []ports.CountDifference) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("finish count: %w", err)
	}
	_, err = executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_count SET status = 'COUNTED', completed_by = $3, completed_at = $4
		  WHERE tenant_id = $1 AND count_id = $2 AND status = 'COUNTING'`,
		count.Tenant().String(), count.ID().String(), count.FinishedBy().String(), count.FinishedAt())
	if err != nil {
		return fmt.Errorf("finish count: %w", err)
	}
	for _, difference := range differences {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.count_difference (
			    tenant_id, difference_id, count_id, owner_account_id, warehouse_code, location_code, goods_code, unit_name,
			    stock_state, label_alias, batch_number, expiry_date, book_quantity, counted_quantity, status
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::date,$13,$14,'PENDING')`,
			count.Tenant().String(), difference.ID.String(), count.ID().String(), difference.Owner.String(), count.Warehouse().String(),
			difference.Location.String(), difference.Goods.String(), difference.Unit.String(), difference.State.String(),
			optionalText(difference.Alias.String(), difference.Alias.String() != ""),
			optionalText(difference.Batch.String(), difference.Batch.Given()),
			optionalText(difference.Expiry.String(), difference.Expiry.Given()),
			difference.Book, difference.Counted,
		); err != nil {
			return fmt.Errorf("finish count: %w", err)
		}
	}
	_, err = executor.Exec(ctx, `DELETE FROM warehouse_fulfillment.stock_count_lock WHERE tenant_id = $1 AND count_id = $2`, count.Tenant().String(), count.ID().String())
	return err
}

func (store *Counts) CellLock(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, cell domain.StockCell) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("cell lock: %w", err)
	}
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(lock.quantity), 0)::bigint
		   FROM warehouse_fulfillment.stock_count_lock lock
		   JOIN warehouse_fulfillment.stock_count count
		     ON count.tenant_id = lock.tenant_id AND count.count_id = lock.count_id
		  WHERE lock.tenant_id = $1 AND lock.owner_account_id = $2 AND lock.warehouse_code = $3 AND count.status = 'COUNTING'
		    AND lock.location_code = $4 AND lock.goods_code = $5 AND lock.unit_name = $6 AND lock.stock_state = $7
		    AND lock.label_alias IS NOT DISTINCT FROM $8 AND lock.inbound_batch_id = $9
		    AND lock.batch_number IS NOT DISTINCT FROM $10 AND lock.expiry_date IS NOT DISTINCT FROM $11::date`,
		tenant.String(), owner.String(), warehouse.String(), cell.Location().String(), cell.Goods().String(), cell.Unit().String(), cell.State().String(),
		optionalText(cell.Alias().String(), cell.Alias().String() != ""), cell.InboundBatch().String(),
		optionalText(cell.BatchNumber().String(), cell.BatchNumber().Given()), optionalText(cell.Expiry().String(), cell.Expiry().Given()),
	).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("cell lock: %w", err)
	}
	return quantity, nil
}

func (store *Counts) CountingOverlap(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode, owner domain.OwnerAccountID, goods domain.GoodsCode) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var overlap bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1
		      FROM warehouse_fulfillment.stock_count count
		      JOIN warehouse_fulfillment.stock_count_location place
		        ON place.tenant_id = count.tenant_id AND place.count_id = count.count_id
		     WHERE count.tenant_id = $1 AND count.warehouse_code = $2 AND count.status = 'COUNTING' AND place.location_code = $3
		       AND (count.owner_account_id IS NULL OR count.owner_account_id = $4)
		       AND (
		            NOT EXISTS (SELECT 1 FROM warehouse_fulfillment.stock_count_goods goods WHERE goods.tenant_id = count.tenant_id AND goods.count_id = count.count_id)
		            OR EXISTS (SELECT 1 FROM warehouse_fulfillment.stock_count_goods goods WHERE goods.tenant_id = count.tenant_id AND goods.count_id = count.count_id AND goods.goods_code = $5)
		       )
		 )`,
		tenant.String(), warehouse.String(), location.String(), owner.String(), goods.String(),
	).Scan(&overlap)
	return overlap, err
}

func (store *Counts) SaveEntry(ctx context.Context, entry ports.CountEntry) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save count entry: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.count_entry (
		    tenant_id, fact_id, count_id, location_code, occurred_at, device_sequence, device_id, operator_id, received_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		entry.Tenant.String(), entry.FactID.String(), entry.CountID.String(), entry.Location.String(), entry.OccurredAt,
		entry.Sequence, entry.Device.String(), entry.Operator.String(), entry.ReceivedAt,
	)
	if err != nil {
		return fmt.Errorf("save count entry: %w", err)
	}
	for index, line := range entry.Lines {
		var owner, goods, unit, state any
		if line.Observed == "" {
			owner, goods, unit, state = line.Owner.String(), line.Goods.String(), line.Unit.String(), line.State.String()
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.count_entry_line (
			    tenant_id, fact_id, line_index, owner_account_id, goods_code, unit_name, stock_state, label_alias,
			    batch_number, expiry_date, quantity, observed
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::date,$11,$12)`,
			entry.Tenant.String(), entry.FactID.String(), index, owner, goods, unit, state,
			optionalText(line.Alias.String(), line.Alias.String() != ""),
			optionalText(line.Batch.String(), line.Batch.Given()),
			optionalText(line.Expiry.String(), line.Expiry.Given()),
			line.Quantity, nullableText(line.Observed),
		); err != nil {
			return fmt.Errorf("save count entry: %w", err)
		}
	}
	return nil
}

func (store *Counts) LoadEntries(ctx context.Context, tenant domain.TenantID, id domain.CountID) ([]ports.CountEntry, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT fact_id, location_code, occurred_at, device_sequence, device_id, operator_id, received_at
		   FROM warehouse_fulfillment.count_entry WHERE tenant_id = $1 AND count_id = $2`,
		tenant.String(), id.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []ports.CountEntry
	for rows.Next() {
		var fact, location, device, operator string
		var occurred, received time.Time
		var sequence int64
		if err := rows.Scan(&fact, &location, &occurred, &sequence, &device, &operator, &received); err != nil {
			return nil, err
		}
		entry := ports.CountEntry{
			Tenant: tenant, FactID: mustFact(fact), CountID: id, Location: mustLocation(&location), OccurredAt: occurred,
			Sequence: sequence, Device: mustDevice(device), Operator: mustOperator(operator), ReceivedAt: received,
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range entries {
		lines, err := store.entryLines(ctx, tenant.String(), entries[index].FactID.String())
		if err != nil {
			return nil, err
		}
		entries[index].Lines = lines
	}
	return entries, nil
}

func (store *Counts) entryLines(ctx context.Context, tenant, fact string) ([]ports.CountEntryLine, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT owner_account_id, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text, quantity, observed
		   FROM warehouse_fulfillment.count_entry_line WHERE tenant_id = $1 AND fact_id = $2 ORDER BY line_index`, tenant, fact)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []ports.CountEntryLine
	for rows.Next() {
		var owner, goods, unit, state, alias, batch, expiry, observed *string
		var quantity int64
		if err := rows.Scan(&owner, &goods, &unit, &state, &alias, &batch, &expiry, &quantity, &observed); err != nil {
			return nil, err
		}
		line := ports.CountEntryLine{Quantity: quantity, Observed: valueOrEmpty(observed), Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry)}
		if goods != nil {
			line.Owner = mustOwnerID(valueOrEmpty(owner))
			line.Goods = mustGoods(goods)
			line.Unit = mustUnit(unit)
			line.State = mustState(state)
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (store *Counts) LocationBalances(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, locations []domain.LocationCode) ([]ports.StockCellBalance, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(locations))
	for _, location := range locations {
		codes = append(codes, location.String())
	}
	rows, err := querier.Query(ctx,
		`SELECT line.warehouse_code, line.owner_account_id, line.location_code, line.goods_code, line.unit_name, line.stock_state,
		        line.label_alias, line.inbound_batch_id, line.batch_number, line.expiry_date::text, batch.inbound_at,
		        SUM(line.quantity_delta)::bigint
		   FROM warehouse_fulfillment.stock_movement_line line
		   JOIN warehouse_fulfillment.inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.warehouse_code = $2
		    AND (cardinality($3::text[]) = 0 OR line.location_code = ANY($3))
		  GROUP BY line.warehouse_code, line.owner_account_id, line.location_code, line.goods_code, line.unit_name, line.stock_state,
		           line.label_alias, line.inbound_batch_id, line.batch_number, line.expiry_date, batch.inbound_at
		 HAVING SUM(line.quantity_delta) > 0`,
		tenant.String(), warehouse.String(), codes)
	if err != nil {
		return nil, fmt.Errorf("location balances: %w", err)
	}
	defer rows.Close()
	var listed []ports.StockCellBalance
	for rows.Next() {
		var row ports.StockCellBalance
		if err := rows.Scan(&row.Warehouse, &row.Owner, &row.Location, &row.Goods, &row.Unit, &row.State, &row.Alias, &row.InboundBatch, &row.BatchNumber, &row.Expiry, &row.InboundAt, &row.Quantity); err != nil {
			return nil, err
		}
		listed = append(listed, row)
	}
	return listed, rows.Err()
}

func (store *Counts) LoadDifference(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID) (ports.StoredCountDifference, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StoredCountDifference{}, false, err
	}
	var owner, warehouse, countID, location, goods, unit, state, status string
	var alias, batch, expiry *string
	var book, counted int64
	err = querier.QueryRow(ctx,
		`SELECT count_id, owner_account_id, warehouse_code, location_code, goods_code, unit_name, stock_state,
		        label_alias, batch_number, expiry_date::text, book_quantity, counted_quantity, status
		   FROM warehouse_fulfillment.count_difference
		  WHERE tenant_id = $1 AND difference_id = $2`,
		tenant.String(), id.String()).Scan(
		&countID, &owner, &warehouse, &location, &goods, &unit, &state, &alias, &batch, &expiry, &book, &counted, &status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StoredCountDifference{}, false, nil
	}
	if err != nil {
		return ports.StoredCountDifference{}, false, fmt.Errorf("load difference: %w", err)
	}
	record := ports.StoredCountDifference{
		CountDifference: ports.CountDifference{
			ID: id, Owner: mustOwnerID(owner), Location: mustLocation(&location), Goods: mustGoods(&goods),
			Unit: mustUnit(&unit), State: mustState(&state), Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry),
			Book: book, Counted: counted,
		},
		Tenant: tenant, Warehouse: mustWarehouse(warehouse), CountID: mustCountID(countID), Status: status,
	}
	return record, true, nil
}

func (store *Counts) MarkApproved(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, note string, dutyRevision int) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("approve difference: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.count_difference
		    SET status = 'APPROVED', approved_by = $3, approved_at = $4, approval_note = $5, duty_revision = $6
		  WHERE tenant_id = $1 AND difference_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, nullableText(note), dutyRevision)
	if err != nil {
		return fmt.Errorf("approve difference: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("approve difference: %s was not pending", id.String())
	}
	return nil
}

func (store *Counts) MarkRejected(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, reason string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("reject difference: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.count_difference
		    SET status = 'REJECTED', rejected_by = $3, rejected_at = $4, reject_reason = $5
		  WHERE tenant_id = $1 AND difference_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, reason)
	if err != nil {
		return fmt.Errorf("reject difference: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("reject difference: %s was not pending", id.String())
	}
	return nil
}

func (store *Counts) SaveDuty(ctx context.Context, tenant domain.TenantID, duty ports.ApprovalDuty) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save approval duty: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.approval_duty (tenant_id, revision, distinct_subjects, operator_id, registered_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		tenant.String(), duty.Revision, duty.Distinct, duty.Operator.String(), duty.RegisteredAt)
	if err != nil {
		return fmt.Errorf("save approval duty: %w", err)
	}
	return nil
}

func (store *Counts) LoadDuty(ctx context.Context, tenant domain.TenantID) (ports.ApprovalDuty, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ApprovalDuty{}, false, err
	}
	var duty ports.ApprovalDuty
	var operator string
	err = querier.QueryRow(ctx,
		`SELECT revision, distinct_subjects, operator_id, registered_at
		   FROM warehouse_fulfillment.approval_duty
		  WHERE tenant_id = $1
		  ORDER BY revision DESC
		  LIMIT 1`, tenant.String()).Scan(&duty.Revision, &duty.Distinct, &operator, &duty.RegisteredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ApprovalDuty{}, false, nil
	}
	if err != nil {
		return ports.ApprovalDuty{}, false, fmt.Errorf("load approval duty: %w", err)
	}
	duty.Operator = mustRegistrant(&operator)
	return duty, true, nil
}

var _ ports.ApprovalDutyStore = (*Counts)(nil)

func mustFact(value string) domain.InternalJobFactID {
	parsed, _ := domain.NewInternalJobFactID(value)
	return parsed
}

func mustDevice(value string) domain.ReceiptDeviceID {
	parsed, _ := domain.NewReceiptDeviceID(value)
	return parsed
}

func mustOperator(value string) domain.ReceiptOperatorID {
	parsed, _ := domain.NewReceiptOperatorID(value)
	return parsed
}

func mustCountID(value string) domain.CountID {
	parsed, _ := domain.NewCountID(value)
	return parsed
}

func locationCodes(values []string) []domain.LocationCode {
	codes := make([]domain.LocationCode, 0, len(values))
	for _, value := range values {
		parsed := mustLocation(&value)
		codes = append(codes, parsed)
	}
	return codes
}

func goodsCodes(values []string) []domain.GoodsCode {
	codes := make([]domain.GoodsCode, 0, len(values))
	for _, value := range values {
		codes = append(codes, mustGoods(&value))
	}
	return codes
}
