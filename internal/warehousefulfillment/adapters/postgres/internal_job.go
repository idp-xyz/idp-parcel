package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// InternalJobs 是库内作业事实、库存差异与次品转良品的 PostgreSQL 适配器。
// 加锁与写入在没有环境事务时拒绝。
type InternalJobs struct {
	db *bentopg.DB
}

func NewInternalJobs(db *bentopg.DB) (*InternalJobs, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &InternalJobs{db: db}, nil
}

var (
	_ ports.InternalJobStore      = (*InternalJobs)(nil)
	_ ports.JobDiscrepancyStore   = (*InternalJobs)(nil)
	_ ports.ConditionRestoreStore = (*InternalJobs)(nil)
)

const internalJobColumns = `
	tenant_id, fact_id, kind, warehouse_code, device_id, operator_id, occurred_at, device_sequence, received_at`

const internalJobListed = `WITH listed AS (
	SELECT ` + internalJobColumns + `
	  FROM warehouse_fulfillment.internal_job
	 WHERE tenant_id = $1
)`

var internalJobListBinding = ports.InternalJobPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"receivedAt": "received_at",
		"factId":     "fact_id",
	},
	Identity: []string{"fact_id"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"kind":          "kind",
	},
	Keyword: []string{"fact_id", "warehouse_code"},
})

func (store *InternalJobs) HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) error {
	return store.hold(ctx, "hold internal job", "warehouse_fulfillment.internal_job", tenant.String(), factID.String())
}

func (store *InternalJobs) HoldDecision(ctx context.Context, tenant domain.TenantID, id domain.ConditionRestoreID) error {
	return store.hold(ctx, "hold condition restore", "warehouse_fulfillment.condition_restore", tenant.String(), id.String())
}

func (store *InternalJobs) hold(ctx context.Context, operation, namespace, tenant, id string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2 || chr(31) || $3))`,
		namespace, tenant, id,
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *InternalJobs) Load(ctx context.Context, tenant domain.TenantID, factID domain.InternalJobFactID) (domain.RecordedInternalJob, bool, error) {
	const operation = "load internal job"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.RecordedInternalJob{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	jobs, err := loadInternalJobs(ctx, querier, `tenant_id = $1 AND fact_id = $2`, tenant.String(), factID.String())
	if err != nil {
		return domain.RecordedInternalJob{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	switch len(jobs) {
	case 0:
		return domain.RecordedInternalJob{}, false, nil
	case 1:
		return jobs[0], true, nil
	default:
		return domain.RecordedInternalJob{}, false, fmt.Errorf("%s: fact %s returned more than one row", operation, factID)
	}
}

func (store *InternalJobs) Save(ctx context.Context, job domain.RecordedInternalJob, discrepancies []domain.StockDiscrepancy) error {
	const operation = "save internal job"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.internal_job (
		    tenant_id, fact_id, kind, warehouse_code, device_id, operator_id, occurred_at, device_sequence, received_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		job.Tenant().String(), job.FactID().String(), job.Kind().String(), job.Warehouse().String(),
		job.Device().String(), job.Operator().String(), job.OccurredAt(), job.DeviceSequence(), job.ReceivedAt(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	for _, discrepancy := range discrepancies {
		if err := insertStockDiscrepancy(ctx, executor, discrepancy); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	for position, line := range job.Lines() {
		if err := insertInternalJobLine(ctx, executor, job, position, line); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	return nil
}

func insertStockDiscrepancy(ctx context.Context, executor bentopg.Executor, discrepancy domain.StockDiscrepancy) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_discrepancy (
		    tenant_id, discrepancy_id, fact_id, line_position, owner_account_id, warehouse_code,
		    location_code, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date,
		    inbound_batch_id, source_location, carton_number, note, quantity, short_on_book, held_quantity,
		    carton_missing, status
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,'PENDING')`,
		discrepancy.Tenant().String(), discrepancy.ID().String(), discrepancy.FactID().String(), discrepancy.Line(),
		discrepancy.Owner().String(), discrepancy.Warehouse().String(),
		nullableText(discrepancy.Location().String()), nullableText(discrepancy.Goods().String()),
		nullableText(discrepancy.Unit().String()), nullableText(discrepancy.State().String()),
		nullableText(discrepancy.Alias().String()), nullableText(discrepancy.Batch().String()), nullableDate(discrepancy.Expiry()),
		nullableText(discrepancy.InboundBatch().String()), nullableText(discrepancy.Source().String()),
		nullableText(discrepancy.Carton().String()), nullableText(discrepancy.Note()),
		discrepancy.Quantity(), discrepancy.ShortOnBook(), discrepancy.Held(), discrepancy.CartonMissing(),
	)
	return err
}

func insertInternalJobLine(ctx context.Context, executor bentopg.Executor, job domain.RecordedInternalJob, position int, line domain.RecordedInternalJobLine) error {
	input := line.Input()
	result := line.Result()
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.internal_job_line (
		    tenant_id, fact_id, position, owner_account_id, goods_code, unit_name, stock_state, label_alias,
		    batch_number, expiry_date, inbound_batch_id, source_location, target_location, quantity,
		    to_unit, to_quantity, to_state, carton_number, note, posted_quantity, posted_inbound_quantity,
		    short_on_book, held_quantity, carton_missing, movement_id, discrepancy_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`,
		job.Tenant().String(), job.FactID().String(), position, input.Owner().String(),
		nullableText(input.Goods().String()), nullableText(input.Unit().String()), nullableText(input.State().String()),
		nullableText(input.Alias().String()), nullableText(input.Batch().String()), nullableDate(input.Expiry()),
		nullableText(input.NamedBatch().String()), nullableText(input.Source().String()), nullableText(input.Target().String()),
		input.Quantity(), nullableText(input.ToUnit().String()), nullablePositive(input.ToQuantity()), nullableText(input.ToState().String()),
		nullableText(input.Carton().String()), nullableText(input.Note()),
		result.Posted, result.PostedInbound, result.ShortOnBook, result.Held, result.CartonMissing,
		nullableText(result.Movement), nullableText(result.Discrepancy.String()),
	); err != nil {
		return err
	}
	for batchPosition, batch := range result.Batches {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.internal_job_line_batch (
			    tenant_id, fact_id, position, batch_position, inbound_batch_id
			 ) VALUES ($1,$2,$3,$4,$5)`,
			job.Tenant().String(), job.FactID().String(), position, batchPosition, batch.String(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (store *InternalJobs) List(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.RecordedInternalJob], error) {
	const operation = "list internal jobs"
	fail := func(err error) (cataloguepage.Result[domain.RecordedInternalJob], error) {
		return cataloguepage.Result[domain.RecordedInternalJob]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" {
		return fail(errors.New("tenant is required"))
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := internalJobListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, internalJobListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, internalJobListed+" "+plan.PageSQL(internalJobColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	var headers []internalJobHeader
	for rows.Next() {
		header, err := scanInternalJobHeader(rows)
		if err != nil {
			rows.Close()
			return fail(err)
		}
		headers = append(headers, header)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fail(err)
	}
	rows.Close()
	jobs := make([]domain.RecordedInternalJob, 0, len(headers))
	for _, header := range headers {
		lines, err := loadInternalJobLines(ctx, querier, header.tenant, header.factID)
		if err != nil {
			return fail(err)
		}
		job, err := header.recorded(lines)
		if err != nil {
			return fail(err)
		}
		jobs = append(jobs, job)
	}
	page, err := cataloguepage.Finish(query, jobs, limit, total, func(job domain.RecordedInternalJob, sortField string) cataloguepage.Position {
		switch sortField {
		case "factId":
			return cataloguepage.Position{Value: job.FactID().String(), Identity: []string{job.FactID().String()}}
		default:
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(job.ReceivedAt()), Identity: []string{job.FactID().String()}}
		}
	})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

func (store *InternalJobs) LoadDecision(ctx context.Context, tenant domain.TenantID, id domain.ConditionRestoreID) (domain.ConditionRestore, bool, error) {
	const operation = "load condition restore"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ConditionRestore{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx,
		`SELECT tenant_id, decision_id, operator_id, occurred_at, received_at, warehouse_code, owner_account_id,
		        location_code, goods_code, unit_name, label_alias, batch_number, expiry_date::text, inbound_batch_id,
		        quantity, reason, movement_id
		   FROM warehouse_fulfillment.condition_restore
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), id.String(),
	)
	restore, found, err := scanConditionRestore(row)
	if err != nil {
		return domain.ConditionRestore{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return restore, found, nil
}

func (store *InternalJobs) SaveDecision(ctx context.Context, restore domain.ConditionRestore) error {
	const operation = "save condition restore"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.condition_restore (
		    tenant_id, decision_id, operator_id, occurred_at, received_at, warehouse_code, owner_account_id,
		    location_code, goods_code, unit_name, label_alias, batch_number, expiry_date, inbound_batch_id,
		    quantity, reason, movement_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		restore.Tenant().String(), restore.ID().String(), restore.Operator().String(), restore.OccurredAt(), restore.ReceivedAt(),
		restore.Warehouse().String(), restore.Owner().String(), restore.Location().String(), restore.Goods().String(), restore.Unit().String(),
		nullableText(restore.Alias().String()), nullableText(restore.Batch().String()), nullableDate(restore.Expiry()),
		nullableText(restore.NamedBatch().String()), restore.Quantity(), restore.Reason(), restore.Movement().String(),
	); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

type internalJobHeader struct {
	tenant         domain.TenantID
	factID         domain.InternalJobFactID
	kind           domain.InternalJobKind
	warehouse      domain.WarehouseCode
	device         domain.ReceiptDeviceID
	operator       domain.ReceiptOperatorID
	occurredAt     time.Time
	deviceSequence int64
	receivedAt     time.Time
}

func (header internalJobHeader) recorded(lines []domain.RecordedInternalJobLine) (domain.RecordedInternalJob, error) {
	inputs := make([]domain.InternalJobLine, len(lines))
	for index, line := range lines {
		inputs[index] = line.Input()
	}
	return domain.NewRecordedInternalJob(header.tenant, header.factID, domain.InternalJobSubmission{
		Kind: header.kind, Device: header.device, Operator: header.operator, OccurredAt: header.occurredAt,
		DeviceSequence: header.deviceSequence, Warehouse: header.warehouse, Lines: inputs,
	}, header.receivedAt, lines)
}

func loadInternalJobs(ctx context.Context, querier bentopg.Querier, where string, args ...any) ([]domain.RecordedInternalJob, error) {
	rows, err := querier.Query(ctx, `SELECT `+internalJobColumns+` FROM warehouse_fulfillment.internal_job WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	var headers []internalJobHeader
	for rows.Next() {
		header, err := scanInternalJobHeader(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		headers = append(headers, header)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	jobs := make([]domain.RecordedInternalJob, 0, len(headers))
	for _, header := range headers {
		lines, err := loadInternalJobLines(ctx, querier, header.tenant, header.factID)
		if err != nil {
			return nil, err
		}
		job, err := header.recorded(lines)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func loadInternalJobLines(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, factID domain.InternalJobFactID) ([]domain.RecordedInternalJobLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT position, owner_account_id, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text,
		        inbound_batch_id, source_location, target_location, quantity, to_unit, to_quantity, to_state, carton_number, note,
		        posted_quantity, posted_inbound_quantity, short_on_book, held_quantity, carton_missing, movement_id, discrepancy_id
		   FROM warehouse_fulfillment.internal_job_line
		  WHERE tenant_id = $1 AND fact_id = $2
		  ORDER BY position`,
		tenant.String(), factID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []domain.RecordedInternalJobLine
	var positions []int
	for rows.Next() {
		var position int
		line, err := scanInternalJobLine(rows, &position)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	batches, err := loadInternalJobBatches(ctx, querier, tenant, factID)
	if err != nil {
		return nil, err
	}
	for index, position := range positions {
		result := lines[index].Result()
		result.Batches = batches[position]
		lines[index] = domain.NewRecordedInternalJobLine(lines[index].Input(), result)
	}
	return lines, nil
}

func loadInternalJobBatches(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, factID domain.InternalJobFactID) (map[int][]domain.InboundBatchID, error) {
	rows, err := querier.Query(ctx,
		`SELECT position, inbound_batch_id
		   FROM warehouse_fulfillment.internal_job_line_batch
		  WHERE tenant_id = $1 AND fact_id = $2
		  ORDER BY position, batch_position`,
		tenant.String(), factID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grouped := map[int][]domain.InboundBatchID{}
	for rows.Next() {
		var position int
		var raw string
		if err := rows.Scan(&position, &raw); err != nil {
			return nil, err
		}
		id, err := domain.NewInboundBatchID(raw)
		if err != nil {
			return nil, err
		}
		grouped[position] = append(grouped[position], id)
	}
	return grouped, rows.Err()
}

func scanInternalJobHeader(scanner rowScanner) (internalJobHeader, error) {
	var (
		rawTenant, rawFact, rawKind, rawWarehouse, rawDevice, rawOperator string
		occurred, received                                                time.Time
		sequence                                                          int64
	)
	if err := scanner.Scan(&rawTenant, &rawFact, &rawKind, &rawWarehouse, &rawDevice, &rawOperator, &occurred, &sequence, &received); err != nil {
		return internalJobHeader{}, err
	}
	tenant, err := domain.NewTenantID(rawTenant)
	if err != nil {
		return internalJobHeader{}, err
	}
	factID, err := domain.NewInternalJobFactID(rawFact)
	if err != nil {
		return internalJobHeader{}, err
	}
	kind, err := parseInternalJobKind(rawKind)
	if err != nil {
		return internalJobHeader{}, err
	}
	warehouse, err := domain.NewWarehouseCode(rawWarehouse)
	if err != nil {
		return internalJobHeader{}, err
	}
	device, err := domain.NewReceiptDeviceID(rawDevice)
	if err != nil {
		return internalJobHeader{}, err
	}
	operator, err := domain.NewReceiptOperatorID(rawOperator)
	if err != nil {
		return internalJobHeader{}, err
	}
	return internalJobHeader{
		tenant: tenant, factID: factID, kind: kind, warehouse: warehouse, device: device, operator: operator,
		occurredAt: occurred, deviceSequence: sequence, receivedAt: received,
	}, nil
}

func scanInternalJobLine(scanner rowScanner, position *int) (domain.RecordedInternalJobLine, error) {
	var (
		owner, goods, unit, state, alias, batch, expiry, named, source, target *string
		toUnit, toState, carton, note, movement, discrepancy                   *string
		quantity, toQuantity                                                   *int64
		posted, postedInbound, short, held                                     int64
		missing                                                                bool
		rawQuantity                                                            int64
	)
	if err := scanner.Scan(
		position, &owner, &goods, &unit, &state, &alias, &batch, &expiry, &named, &source, &target, &rawQuantity,
		&toUnit, &toQuantity, &toState, &carton, &note, &posted, &postedInbound, &short, &held, &missing, &movement, &discrepancy,
	); err != nil {
		return domain.RecordedInternalJobLine{}, err
	}
	_ = quantity
	input, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: mustOwner(owner), Goods: mustGoods(goods), Unit: mustUnit(unit), State: mustState(state),
		Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry), NamedBatch: mustInbound(named),
		Source: mustLocation(source), Target: mustLocation(target), Quantity: rawQuantity, ToUnit: mustUnit(toUnit),
		ToQuantity: valueOrZero(toQuantity), ToState: mustState(toState), Carton: mustCarton(carton), Note: valueOrEmpty(note),
	})
	if err != nil {
		return domain.RecordedInternalJobLine{}, err
	}
	result := domain.InternalJobLineResult{
		Posted: posted, PostedInbound: postedInbound, ShortOnBook: short, Held: held, CartonMissing: missing,
		Movement: valueOrEmpty(movement),
	}
	if discrepancy != nil && *discrepancy != "" {
		id, err := domain.NewStockDiscrepancyID(*discrepancy)
		if err != nil {
			return domain.RecordedInternalJobLine{}, err
		}
		result.Discrepancy = id
	}
	return domain.NewRecordedInternalJobLine(input, result), nil
}

func scanConditionRestore(row pgx.Row) (domain.ConditionRestore, bool, error) {
	var (
		rawTenant, rawID, rawOperator, rawWarehouse, rawOwner, rawLocation, rawGoods, rawUnit string
		alias, batch, expiry, named                                                           *string
		occurred, received                                                                    time.Time
		quantity                                                                              int64
		reason, rawMovement                                                                   string
	)
	err := row.Scan(
		&rawTenant, &rawID, &rawOperator, &occurred, &received, &rawWarehouse, &rawOwner, &rawLocation, &rawGoods, &rawUnit,
		&alias, &batch, &expiry, &named, &quantity, &reason, &rawMovement,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ConditionRestore{}, false, nil
	}
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	tenant, err := domain.NewTenantID(rawTenant)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	id, err := domain.NewConditionRestoreID(rawID)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	operator, err := domain.NewRegistrant(rawOperator)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	warehouse, err := domain.NewWarehouseCode(rawWarehouse)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	owner, err := domain.NewOwnerAccountID(rawOwner)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	location, err := domain.NewLocationCode(rawLocation)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	goods, err := domain.NewGoodsCode(rawGoods)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	unit, err := domain.NewGoodsUnitName(rawUnit)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	movement, err := domain.NewStockMovementID(rawMovement)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	parsedExpiry, err := mustExpiryText(expiry)
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	restore, err := domain.NewConditionRestore(domain.ConditionRestoreSpec{
		Tenant: tenant, ID: id, Operator: operator, OccurredAt: occurred, ReceivedAt: received,
		Warehouse: warehouse, Owner: owner, Location: location, Goods: goods, Unit: unit,
		Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: parsedExpiry, NamedBatch: mustInbound(named),
		Quantity: quantity, Reason: reason, Movement: movement,
	})
	if err != nil {
		return domain.ConditionRestore{}, false, err
	}
	return restore, true, nil
}

func parseInternalJobKind(word string) (domain.InternalJobKind, error) {
	for _, kind := range []domain.InternalJobKind{domain.InternalJobMove, domain.InternalJobForm, domain.InternalJobCondition, domain.InternalJobShortage} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return domain.InternalJobKindInvalid, fmt.Errorf("unknown internal job kind %q", word)
}

func nullableDate(date domain.ExpiryDate) any {
	if !date.Given() {
		return nil
	}
	return date.String()
}

func nullablePositive(quantity int64) any {
	if quantity <= 0 {
		return nil
	}
	return quantity
}

func mustOwner(value *string) domain.OwnerAccountID {
	parsed, _ := domain.NewOwnerAccountID(valueOrEmpty(value))
	return parsed
}

func mustGoods(value *string) domain.GoodsCode {
	if value == nil {
		return domain.GoodsCode{}
	}
	parsed, _ := domain.NewGoodsCode(*value)
	return parsed
}

func mustUnit(value *string) domain.GoodsUnitName {
	if value == nil {
		return domain.GoodsUnitName{}
	}
	parsed, _ := domain.NewGoodsUnitName(*value)
	return parsed
}

func mustState(value *string) domain.StockState {
	if value == nil {
		return domain.StockStateInvalid
	}
	parsed, _ := domain.NewStockState(*value)
	return parsed
}

func mustAlias(value *string) domain.GoodsAliasCode {
	if value == nil {
		return domain.GoodsAliasCode{}
	}
	parsed, _ := domain.NewGoodsAliasCode(*value)
	return parsed
}

func mustBatch(value *string) domain.BatchNumber {
	if value == nil {
		return domain.BatchNumber{}
	}
	parsed, _ := domain.NewBatchNumber(*value)
	return parsed
}

func mustExpiry(value *string) domain.ExpiryDate {
	parsed, _ := mustExpiryText(value)
	return parsed
}

func mustExpiryText(value *string) (domain.ExpiryDate, error) {
	if value == nil || *value == "" {
		return domain.ExpiryDate{}, nil
	}
	text := *value
	if len(text) >= 10 {
		text = text[:10]
	}
	return domain.NewExpiryDate(text)
}

func mustInbound(value *string) domain.InboundBatchID {
	if value == nil {
		return domain.InboundBatchID{}
	}
	parsed, _ := domain.NewInboundBatchID(*value)
	return parsed
}

func mustLocation(value *string) domain.LocationCode {
	if value == nil {
		return domain.LocationCode{}
	}
	parsed, _ := domain.NewLocationCode(*value)
	return parsed
}

func mustCarton(value *string) domain.CartonNumber {
	if value == nil {
		return domain.CartonNumber{}
	}
	parsed, _ := domain.NewCartonNumber(*value)
	return parsed
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (store *InternalJobs) LoadDiscrepancy(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID) (ports.StoredJobDiscrepancy, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StoredJobDiscrepancy{}, false, err
	}
	var fact, kind, operator, owner, warehouse, status string
	var location, goods, unit, state, alias, batch, expiry, inbound, source, carton *string
	var quantity int64
	var linePosition int
	var missing bool
	err = querier.QueryRow(ctx,
		`SELECT discrepancy.fact_id, discrepancy.line_position, job.kind, job.operator_id, discrepancy.owner_account_id, discrepancy.warehouse_code,
		        discrepancy.location_code, discrepancy.goods_code, discrepancy.unit_name, discrepancy.stock_state,
		        discrepancy.label_alias, discrepancy.batch_number, discrepancy.expiry_date::text, discrepancy.inbound_batch_id,
		        discrepancy.source_location, discrepancy.carton_number, discrepancy.quantity, discrepancy.carton_missing, discrepancy.status
		   FROM warehouse_fulfillment.stock_discrepancy discrepancy
		   JOIN warehouse_fulfillment.internal_job job
		     ON job.tenant_id = discrepancy.tenant_id AND job.fact_id = discrepancy.fact_id
		  WHERE discrepancy.tenant_id = $1 AND discrepancy.discrepancy_id = $2`,
		tenant.String(), id.String()).Scan(
		&fact, &linePosition, &kind, &operator, &owner, &warehouse, &location, &goods, &unit, &state, &alias, &batch, &expiry,
		&inbound, &source, &carton, &quantity, &missing, &status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StoredJobDiscrepancy{}, false, nil
	}
	if err != nil {
		return ports.StoredJobDiscrepancy{}, false, fmt.Errorf("load job discrepancy: %w", err)
	}
	return ports.StoredJobDiscrepancy{
		Tenant: tenant, ID: id, FactID: mustFact(fact), Line: linePosition, Kind: kind, Operator: operator, Owner: mustOwnerID(owner), Warehouse: mustWarehouse(warehouse),
		Location: mustLocation(location), Source: mustLocation(source), Goods: mustGoods(goods), Unit: mustUnit(unit), State: mustState(state),
		Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry), Inbound: mustInbound(inbound), Quantity: quantity,
		Carton: mustCarton(carton), CartonMissing: missing, Status: status,
	}, true, nil
}

func (store *InternalJobs) MarkDiscrepancyApproved(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, note string, dutyRevision int) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("approve job discrepancy: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_discrepancy
		    SET status = 'APPROVED', approved_by = $3, approved_at = $4, approval_note = $5, duty_revision = $6
		  WHERE tenant_id = $1 AND discrepancy_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, nullableText(note), dutyRevision)
	if err != nil {
		return fmt.Errorf("approve job discrepancy: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("approve job discrepancy: %s was not pending", id.String())
	}
	return nil
}

func (store *InternalJobs) MarkDiscrepancyRejected(ctx context.Context, tenant domain.TenantID, id domain.StockDiscrepancyID, operator domain.Registrant, at time.Time, reason string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("reject job discrepancy: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_discrepancy
		    SET status = 'REJECTED', rejected_by = $3, rejected_at = $4, reject_reason = $5
		  WHERE tenant_id = $1 AND discrepancy_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, reason)
	if err != nil {
		return fmt.Errorf("reject job discrepancy: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("reject job discrepancy: %s was not pending", id.String())
	}
	return nil
}
