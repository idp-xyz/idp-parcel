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

// StockImports 是建账导入的 PostgreSQL 适配器。加锁与写入在没有环境事务时拒绝。
type StockImports struct {
	db *bentopg.DB
}

func NewStockImports(db *bentopg.DB) (*StockImports, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &StockImports{db: db}, nil
}

var _ ports.StockImportStore = (*StockImports)(nil)

const stockImportColumns = `tenant_id, import_id, owner_account_id, warehouse_code, reason, status, submitter, submitted_at, poster, posted_at, goods_movement_id, carton_movement_id, rejector, rejected_at, rejection_note`

const stockImportListed = `WITH listed AS (
	SELECT ` + stockImportColumns + `
	  FROM warehouse_fulfillment.stock_import
	 WHERE tenant_id = $1
)`

var stockImportListBinding = ports.StockImportPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"submittedAt": "submitted_at",
		"importId":    "import_id",
	},
	Identity: []string{"import_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"status":        "status",
	},
	Keyword: []string{"import_id", "owner_account_id", "warehouse_code", "reason"},
})

func (store *StockImports) Hold(ctx context.Context, tenant domain.TenantID, id domain.StockImportID) error {
	const operation = "hold stock import"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.stock_import'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), id.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *StockImports) Load(ctx context.Context, tenant domain.TenantID, id domain.StockImportID) (domain.StockImport, bool, error) {
	const operation = "load stock import"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.StockImport{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+stockImportColumns+` FROM warehouse_fulfillment.stock_import WHERE tenant_id = $1 AND import_id = $2`,
		tenant.String(), id.String())
	item, err := store.scanImport(ctx, querier, row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StockImport{}, false, nil
	}
	if err != nil {
		return domain.StockImport{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return item, true, nil
}

func (store *StockImports) SavePending(ctx context.Context, item domain.StockImport) error {
	const operation = "save pending stock import"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_import (
		    tenant_id, import_id, owner_account_id, warehouse_code, reason, status, submitter, submitted_at
		 ) VALUES ($1,$2,$3,$4,$5,'PENDING',$6,$7)`,
		item.Tenant().String(), item.ID().String(), item.Owner().String(), item.Warehouse().String(),
		item.Reason().String(), item.Submitter().String(), item.SubmittedAt(),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: insert did not add a row", operation)
	}
	for index, line := range item.GoodsLines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.stock_import_goods_line (
			    tenant_id, import_id, position, location_code, goods_code, unit_name, stock_state, quantity,
			    batch_number, expiry_date, label_alias, declared_inbound_at
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			item.Tenant().String(), item.ID().String(), index, line.Location().String(), line.Goods().String(),
			line.Unit().String(), line.State().String(), line.Quantity(),
			nullableText(line.BatchNumber().String()), nullableExpiry(line.Expiry()), nullableText(line.Alias().String()),
			nullableTime(line.DeclaredInboundAt()),
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
	}
	for index, line := range item.CartonLines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.stock_import_carton_line (
			    tenant_id, import_id, position, location_code, carton_number, declared_inbound_at
			 ) VALUES ($1,$2,$3,$4,$5,$6)`,
			item.Tenant().String(), item.ID().String(), index, line.Location().String(), line.Number().String(),
			nullableTime(line.DeclaredInboundAt()),
		); err != nil {
			return fmt.Errorf("%s: %w", operation, err)
		}
		for contentIndex, content := range line.Contents() {
			if _, err := executor.Exec(ctx,
				`INSERT INTO warehouse_fulfillment.stock_import_carton_content (
				    tenant_id, import_id, line_position, content_position, goods_code, quantity
				 ) VALUES ($1,$2,$3,$4,$5,$6)`,
				item.Tenant().String(), item.ID().String(), index, contentIndex, content.Goods().String(), content.Quantity(),
			); err != nil {
				return fmt.Errorf("%s: %w", operation, err)
			}
		}
	}
	return nil
}

func (store *StockImports) MarkPosted(ctx context.Context, item domain.StockImport) error {
	const operation = "mark stock import posted"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_import
		    SET status = 'POSTED', poster = $3, posted_at = $4, goods_movement_id = $5, carton_movement_id = $6
		  WHERE tenant_id = $1 AND import_id = $2 AND status = 'PENDING'`,
		item.Tenant().String(), item.ID().String(), item.Poster().String(), item.PostedAt(),
		nullableText(item.GoodsMovementID().String()), nullableText(item.CartonMovementID().String()),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: pending row was not updated", operation)
	}
	return nil
}

func (store *StockImports) MarkRejected(ctx context.Context, item domain.StockImport) error {
	const operation = "mark stock import rejected"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_import
		    SET status = 'REJECTED', rejector = $3, rejected_at = $4, rejection_note = $5
		  WHERE tenant_id = $1 AND import_id = $2 AND status = 'PENDING'`,
		item.Tenant().String(), item.ID().String(), item.Rejector().String(), item.RejectedAt(), nullableText(item.RejectionNote()),
	)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: pending row was not updated", operation)
	}
	return nil
}

func (store *StockImports) List(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.StockImport], error) {
	const operation = "list stock imports"
	fail := func(err error) (cataloguepage.Result[domain.StockImport], error) {
		return cataloguepage.Result[domain.StockImport]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := stockImportListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, stockImportListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, stockImportListed+" "+plan.PageSQL(stockImportColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []domain.StockImport
	for rows.Next() {
		item, err := store.scanImport(ctx, querier, rows)
		if err != nil {
			return fail(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item domain.StockImport, sortField string) cataloguepage.Position {
		switch sortField {
		case "importId":
			return cataloguepage.Position{Value: item.ID().String(), Identity: []string{item.ID().String()}}
		default:
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.SubmittedAt()), Identity: []string{item.ID().String()}}
		}
	})
}

type stockImportHeader struct {
	tenant, id, owner, warehouse, reason, status, submitter string
	submittedAt                                             time.Time
	poster, goodsMove, cartonMove, rejector, note           *string
	postedAt, rejectedAt                                    *time.Time
}

func (store *StockImports) scanImport(ctx context.Context, querier bentopg.Querier, row interface{ Scan(dest ...any) error }) (domain.StockImport, error) {
	var header stockImportHeader
	if err := row.Scan(
		&header.tenant, &header.id, &header.owner, &header.warehouse, &header.reason, &header.status, &header.submitter, &header.submittedAt,
		&header.poster, &header.postedAt, &header.goodsMove, &header.cartonMove, &header.rejector, &header.rejectedAt, &header.note,
	); err != nil {
		return domain.StockImport{}, err
	}
	goods, err := loadStockImportGoods(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.StockImport{}, err
	}
	cartons, err := loadStockImportCartons(ctx, querier, header.tenant, header.id)
	if err != nil {
		return domain.StockImport{}, err
	}
	return rebuildStockImport(header, goods, cartons)
}

func loadStockImportGoods(ctx context.Context, querier bentopg.Querier, tenant, id string) ([]domain.StockImportGoodsLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT location_code, goods_code, unit_name, stock_state, quantity, batch_number, expiry_date, label_alias, declared_inbound_at
		   FROM warehouse_fulfillment.stock_import_goods_line
		  WHERE tenant_id = $1 AND import_id = $2
		  ORDER BY position`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []domain.StockImportGoodsLine
	for rows.Next() {
		var location, goods, unit, state string
		var quantity int64
		var batch, alias *string
		var expiry *time.Time
		var declared *time.Time
		if err := rows.Scan(&location, &goods, &unit, &state, &quantity, &batch, &expiry, &alias, &declared); err != nil {
			return nil, err
		}
		line, err := buildGoodsLine(location, goods, unit, state, quantity, batch, expiry, alias, declared)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadStockImportCartons(ctx context.Context, querier bentopg.Querier, tenant, id string) ([]domain.StockImportCartonLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT position, location_code, carton_number, declared_inbound_at
		   FROM warehouse_fulfillment.stock_import_carton_line
		  WHERE tenant_id = $1 AND import_id = $2
		  ORDER BY position`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type storedCarton struct {
		position int
		line     domain.StockImportCartonLine
	}
	var stored []storedCarton
	for rows.Next() {
		var position int
		var location, number string
		var declared *time.Time
		if err := rows.Scan(&position, &location, &number, &declared); err != nil {
			return nil, err
		}
		line, err := domain.NewStockImportCartonLine(
			mustLocation(location), mustCarton(number), nil, timeOrZero(declared),
		)
		if err != nil {
			return nil, err
		}
		stored = append(stored, storedCarton{position: position, line: line})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	contents, err := querier.Query(ctx,
		`SELECT line_position, goods_code, quantity
		   FROM warehouse_fulfillment.stock_import_carton_content
		  WHERE tenant_id = $1 AND import_id = $2
		  ORDER BY line_position, content_position`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer contents.Close()
	byLine := map[int][]domain.StockImportCartonContent{}
	for contents.Next() {
		var position int
		var goods string
		var quantity int64
		if err := contents.Scan(&position, &goods, &quantity); err != nil {
			return nil, err
		}
		content, err := domain.NewStockImportCartonContent(mustGoods(goods), quantity)
		if err != nil {
			return nil, err
		}
		byLine[position] = append(byLine[position], content)
	}
	if err := contents.Err(); err != nil {
		return nil, err
	}
	lines := make([]domain.StockImportCartonLine, 0, len(stored))
	for _, item := range stored {
		line, err := domain.NewStockImportCartonLine(item.line.Location(), item.line.Number(), byLine[item.position], item.line.DeclaredInboundAt())
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func rebuildStockImport(header stockImportHeader, goods []domain.StockImportGoodsLine, cartons []domain.StockImportCartonLine) (domain.StockImport, error) {
	pending, err := domain.NewPendingStockImport(
		mustTenant(header.tenant), mustImport(header.id), mustOwner(header.owner), mustWarehouse(header.warehouse),
		mustReason(header.reason), goods, cartons, mustRegistrant(header.submitter), header.submittedAt,
	)
	if err != nil {
		return domain.StockImport{}, err
	}
	switch header.status {
	case "PENDING":
		return pending, nil
	case "POSTED":
		return pending.Post(mustRegistrant(valueOf(header.poster)), timeOrZero(header.postedAt), mustMovement(valueOf(header.goodsMove)), mustCartonMovement(valueOf(header.cartonMove)))
	case "REJECTED":
		return pending.Reject(mustRegistrant(valueOf(header.rejector)), timeOrZero(header.rejectedAt), valueOf(header.note))
	default:
		return domain.StockImport{}, fmt.Errorf("stock import status %s", header.status)
	}
}

func buildGoodsLine(location, goods, unit, state string, quantity int64, batch *string, expiry *time.Time, alias *string, declared *time.Time) (domain.StockImportGoodsLine, error) {
	stockState, err := domain.NewStockState(state)
	if err != nil {
		return domain.StockImportGoodsLine{}, err
	}
	var batchNumber domain.BatchNumber
	if batch != nil {
		batchNumber, err = domain.NewBatchNumber(*batch)
		if err != nil {
			return domain.StockImportGoodsLine{}, err
		}
	}
	var expiryDate domain.ExpiryDate
	if expiry != nil {
		expiryDate, err = domain.NewExpiryDate(expiry.Format(time.DateOnly))
		if err != nil {
			return domain.StockImportGoodsLine{}, err
		}
	}
	var aliasCode domain.GoodsAliasCode
	if alias != nil {
		aliasCode, err = domain.NewGoodsAliasCode(*alias)
		if err != nil {
			return domain.StockImportGoodsLine{}, err
		}
	}
	return domain.NewStockImportGoodsLine(
		mustLocation(location), mustGoods(goods), mustUnit(unit), stockState, quantity, batchNumber, expiryDate, aliasCode, timeOrZero(declared),
	)
}

func nullableExpiry(expiry domain.ExpiryDate) any {
	if !expiry.Given() {
		return nil
	}
	return expiry.String()
}

func nullableTime(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return at
}

func timeOrZero(at *time.Time) time.Time {
	if at == nil {
		return time.Time{}
	}
	return *at
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mustTenant(value string) domain.TenantID {
	parsed, err := domain.NewTenantID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustImport(value string) domain.StockImportID {
	parsed, err := domain.NewStockImportID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustOwner(value string) domain.OwnerAccountID {
	parsed, err := domain.NewOwnerAccountID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustWarehouse(value string) domain.WarehouseCode {
	parsed, err := domain.NewWarehouseCode(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustReason(value string) domain.StockImportReason {
	parsed, err := domain.NewStockImportReason(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustRegistrant(value string) domain.Registrant {
	parsed, err := domain.NewRegistrant(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustLocation(value string) domain.LocationCode {
	parsed, err := domain.NewLocationCode(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustGoods(value string) domain.GoodsCode {
	parsed, err := domain.NewGoodsCode(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustUnit(value string) domain.GoodsUnitName {
	parsed, err := domain.NewGoodsUnitName(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustCarton(value string) domain.CartonNumber {
	parsed, err := domain.NewCartonNumber(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustMovement(value string) domain.StockMovementID {
	if value == "" {
		return domain.StockMovementID{}
	}
	parsed, err := domain.NewStockMovementID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustCartonMovement(value string) domain.CartonMovementID {
	if value == "" {
		return domain.CartonMovementID{}
	}
	parsed, err := domain.NewCartonMovementID(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
