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

// OutboundRequests 是出库请求、预占与缺货处置的 PostgreSQL 适配器。
type OutboundRequests struct {
	db *bentopg.DB
}

func NewOutboundRequests(db *bentopg.DB) (*OutboundRequests, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &OutboundRequests{db: db}, nil
}

var _ ports.OutboundStore = (*OutboundRequests)(nil)

func (store *OutboundRequests) LoadByKey(ctx context.Context, tenant domain.TenantID, origin domain.OutboundOrigin, key domain.RequestKey) (domain.OutboundRequest, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	request, found, err := scanOutbound(ctx, querier, tenant,
		`request_id = (SELECT request_id FROM warehouse_fulfillment.outbound_request
		    WHERE tenant_id = $1 AND origin = $2 AND request_key = $3)`,
		tenant.String(), origin.String(), key.String())
	return request, found, err
}

func (store *OutboundRequests) LoadByID(ctx context.Context, tenant domain.TenantID, id domain.OutboundRequestID) (domain.OutboundRequest, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	return scanOutbound(ctx, querier, tenant, `request_id = $2`, tenant.String(), id.String())
}

func (store *OutboundRequests) ListChildren(ctx context.Context, tenant domain.TenantID, parent domain.OutboundRequestID) ([]domain.OutboundRequest, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT request_id FROM warehouse_fulfillment.outbound_request
		  WHERE tenant_id = $1 AND parent_request_id = $2
		  ORDER BY request_id`,
		tenant.String(), parent.String())
	if err != nil {
		return nil, fmt.Errorf("list outbound children: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	children := make([]domain.OutboundRequest, 0, len(ids))
	for _, id := range ids {
		parsed, err := domain.NewOutboundRequestID(id)
		if err != nil {
			return nil, err
		}
		child, found, err := store.LoadByID(ctx, tenant, parsed)
		if err != nil || !found {
			return nil, err
		}
		children = append(children, child)
	}
	return children, nil
}

func (store *OutboundRequests) ReleaseForSplit(ctx context.Context, tenant domain.TenantID, id domain.OutboundRequestID) (bool, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("split outbound request: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request
		    SET status = 'SPLIT'
		  WHERE tenant_id = $1 AND request_id = $2
		    AND status IN ('RESERVED', 'PARTIALLY_RESERVED', 'PENDING_POLICY')`,
		tenant.String(), id.String())
	if err != nil {
		return false, fmt.Errorf("split outbound request: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if _, err := executor.Exec(ctx,
		`DELETE FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), id.String()); err != nil {
		return false, fmt.Errorf("split outbound request: %w", err)
	}
	return true, nil
}

func (store *OutboundRequests) Save(ctx context.Context, request domain.OutboundRequest) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save outbound request: %w", err)
	}
	number, absence := (*string)(nil), (*string)(nil)
	if assigned, ok := request.DocumentNumber(); ok {
		text := assigned.String()
		number = &text
	}
	if request.NumberingAbsence().String() != "" {
		text := request.NumberingAbsence().String()
		absence = &text
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_request (
		    tenant_id, request_id, owner_account_id, warehouse_code, origin, request_key, dispatch,
		    service_product, recipient, address, basis, operator_id, status, document_number, numbering_absence, received_at,
		    parent_request_id
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		request.Tenant().String(), request.ID().String(), request.Owner().String(), request.Warehouse().String(),
		request.Origin().String(), request.Key().String(), request.Dispatch().String(), nullableText(request.Service()),
		request.Recipient(), request.Address(), nullableText(request.Basis()), nullableText(request.Operator()),
		request.Status().String(), number, absence, request.ReceivedAt(), nullableText(request.Parent().String()),
	); err != nil {
		return fmt.Errorf("save outbound request: %w", err)
	}
	for index, line := range request.Lines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_request_line (
			    tenant_id, request_id, line_index, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date, quantity
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			request.Tenant().String(), request.ID().String(), index, line.Goods().String(), line.Unit().String(), line.State().String(),
			nullableText(line.Alias().String()), nullableText(line.Batch().String()), nullableDate(line.Expiry()), line.Quantity(),
		); err != nil {
			return fmt.Errorf("save outbound request line: %w", err)
		}
	}
	for index, item := range request.Reservations() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_reservation (
			    tenant_id, request_id, line_index, goods_code, stock_state, label_alias, batch_number, expiry_date,
			    quantity, from_unlabeled, wanted
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			request.Tenant().String(), request.ID().String(), index, item.Goods.String(), item.State.String(),
			nullableText(item.Alias.String()), nullableText(item.Batch.String()), nullableDate(item.Expiry),
			item.Quantity, item.FromUnlabeled, item.Wanted,
		); err != nil {
			return fmt.Errorf("save outbound reservation: %w", err)
		}
	}
	return nil
}

func (store *OutboundRequests) ListReservations(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.StoredReservation, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT request.warehouse_code, reservation.goods_code, reservation.stock_state, reservation.label_alias, reservation.batch_number,
		        reservation.expiry_date::text, reservation.quantity, reservation.from_unlabeled
		   FROM warehouse_fulfillment.outbound_reservation reservation
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = reservation.tenant_id AND request.request_id = reservation.request_id
		  WHERE request.tenant_id = $1 AND request.owner_account_id = $2
		    AND ($3 = '' OR request.warehouse_code = $3)
		    AND request.status IN ('RESERVED', 'PARTIALLY_RESERVED', 'ALLOCATED')
		  ORDER BY reservation.request_id, reservation.line_index`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, fmt.Errorf("list outbound reservations: %w", err)
	}
	defer rows.Close()
	var listed []ports.StoredReservation
	for rows.Next() {
		item, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		listed = append(listed, item)
	}
	return listed, rows.Err()
}

func (store *OutboundRequests) UnassignedReservation(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, state domain.StockState, alias domain.GoodsAliasCode, batch domain.BatchNumber, expiry domain.ExpiryDate) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, err
	}
	column := "reservation.from_unlabeled"
	aliasSQL := ""
	args := []any{tenant.String(), owner.String(), warehouse.String(), goods.String(), state.String(), nullableText(batch.String()), nullableDate(expiry)}
	if alias.String() != "" {
		column = "(reservation.quantity - reservation.from_unlabeled)"
		aliasSQL = " AND reservation.label_alias = $8"
		args = append(args, alias.String())
	}
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(SUM(`+column+`), 0)
		   FROM warehouse_fulfillment.outbound_reservation reservation
		   JOIN warehouse_fulfillment.outbound_request request
		     ON request.tenant_id = reservation.tenant_id AND request.request_id = reservation.request_id
		  WHERE request.tenant_id = $1 AND request.owner_account_id = $2 AND request.warehouse_code = $3
		    AND request.status IN ('RESERVED', 'PARTIALLY_RESERVED', 'ALLOCATED')
		    AND reservation.goods_code = $4 AND reservation.stock_state = $5
		    AND reservation.batch_number IS NOT DISTINCT FROM $6
		    AND reservation.expiry_date IS NOT DISTINCT FROM $7`+aliasSQL,
		args...).Scan(&quantity)
	if err != nil {
		return 0, fmt.Errorf("unassigned reservation: %w", err)
	}
	return quantity, nil
}

func (store *OutboundRequests) LoadPolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID) (ports.StoredShortagePolicy, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StoredShortagePolicy{}, false, err
	}
	var revision int
	var kind, operator string
	var at time.Time
	err = querier.QueryRow(ctx,
		`SELECT revision, kind, operator_id, registered_at
		   FROM warehouse_fulfillment.shortage_policy
		  WHERE tenant_id = $1 AND owner_account_id = $2
		  ORDER BY revision DESC
		  LIMIT 1`,
		tenant.String(), owner.String()).Scan(&revision, &kind, &operator, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StoredShortagePolicy{}, false, nil
	}
	if err != nil {
		return ports.StoredShortagePolicy{}, false, fmt.Errorf("load shortage policy: %w", err)
	}
	parsed, err := domain.NewShortagePolicyKind(kind)
	if err != nil {
		return ports.StoredShortagePolicy{}, false, err
	}
	return ports.StoredShortagePolicy{Revision: revision, Kind: parsed, Operator: operator, RegisteredAt: at}, true, nil
}

func (store *OutboundRequests) SavePolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, policy ports.StoredShortagePolicy) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save shortage policy: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.shortage_policy (
		    tenant_id, owner_account_id, revision, kind, operator_id, registered_at
		 ) VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), owner.String(), policy.Revision, policy.Kind.String(), policy.Operator, policy.RegisteredAt)
	if err != nil {
		return fmt.Errorf("save shortage policy: %w", err)
	}
	return nil
}

func scanOutbound(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, where string, args ...any) (domain.OutboundRequest, bool, error) {
	var id, owner, warehouse, origin, key, dispatch, recipient, address, status string
	var service, basis, operator, number, absence, parent *string
	var received time.Time
	err := querier.QueryRow(ctx,
		`SELECT request_id, owner_account_id, warehouse_code, origin, request_key, dispatch, service_product,
		        recipient, address, basis, operator_id, status, document_number, numbering_absence, received_at, parent_request_id
		   FROM warehouse_fulfillment.outbound_request
		  WHERE tenant_id = $1 AND `+where,
		args...).Scan(&id, &owner, &warehouse, &origin, &key, &dispatch, &service, &recipient, &address, &basis, &operator, &status, &number, &absence, &received, &parent)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OutboundRequest{}, false, nil
	}
	if err != nil {
		return domain.OutboundRequest{}, false, fmt.Errorf("load outbound request: %w", err)
	}
	lines, err := loadOutboundLines(ctx, querier, tenant, id)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	reservations, err := loadOutboundReservations(ctx, querier, tenant, id)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedOrigin, err := domain.NewOutboundOrigin(origin)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedDispatch, err := domain.NewDispatchMethod(dispatch)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedStatus, err := domain.NewOutboundStatus(status)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedKey, err := domain.NewRequestKey(key)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedID, err := domain.NewOutboundRequestID(id)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedOwner, err := domain.NewOwnerAccountID(owner)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	parsedWarehouse, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	var document domain.DocumentNumber
	if number != nil {
		document, err = domain.NewDocumentNumber(*number)
		if err != nil {
			return domain.OutboundRequest{}, false, err
		}
	}
	numbering := domain.NumberingAbsenceNone
	if absence != nil {
		if *absence != domain.NumberingTimeZoneNotConfigured.String() {
			return domain.OutboundRequest{}, false, fmt.Errorf("load outbound request: numbering absence %s", *absence)
		}
		numbering = domain.NumberingTimeZoneNotConfigured
	}
	var parsedParent domain.OutboundRequestID
	if parent != nil {
		parsedParent, err = domain.NewOutboundRequestID(*parent)
		if err != nil {
			return domain.OutboundRequest{}, false, err
		}
	}
	request, err := domain.NewOutboundRequest(domain.OutboundRequestSpec{
		Tenant: tenant, ID: parsedID, Owner: parsedOwner, Warehouse: parsedWarehouse, Origin: parsedOrigin, Key: parsedKey,
		Dispatch: parsedDispatch, Service: valueOrEmpty(service), Recipient: recipient, Address: address,
		Basis: valueOrEmpty(basis), Operator: valueOrEmpty(operator), Status: parsedStatus, Parent: parsedParent,
		Number: document, Absence: numbering, Lines: lines, Reservations: reservations, ReceivedAt: received,
	})
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	return request, true, nil
}

func loadOutboundLines(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, id string) ([]domain.OutboundLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text, quantity
		   FROM warehouse_fulfillment.outbound_request_line
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY line_index`,
		tenant.String(), id)
	if err != nil {
		return nil, fmt.Errorf("load outbound lines: %w", err)
	}
	defer rows.Close()
	var lines []domain.OutboundLine
	for rows.Next() {
		var goods, unit, state string
		var alias, batch, expiry *string
		var quantity int64
		if err := rows.Scan(&goods, &unit, &state, &alias, &batch, &expiry, &quantity); err != nil {
			return nil, err
		}
		parsedGoods, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		parsedUnit, err := domain.NewGoodsUnitName(unit)
		if err != nil {
			return nil, err
		}
		parsedState, err := domain.NewStockState(state)
		if err != nil {
			return nil, err
		}
		line, err := domain.NewOutboundLine(domain.OutboundLineSpec{
			Goods: parsedGoods, Unit: parsedUnit, State: parsedState, Alias: mustAlias(alias), Batch: mustBatch(batch),
			Expiry: mustExpiry(expiry), Quantity: quantity,
		})
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func loadOutboundReservations(ctx context.Context, querier bentopg.Querier, tenant domain.TenantID, id string) ([]domain.OutboundReservation, error) {
	rows, err := querier.Query(ctx,
		`SELECT goods_code, stock_state, label_alias, batch_number, expiry_date::text, quantity, from_unlabeled, wanted
		   FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY line_index`,
		tenant.String(), id)
	if err != nil {
		return nil, fmt.Errorf("load outbound reservations: %w", err)
	}
	defer rows.Close()
	var listed []domain.OutboundReservation
	for rows.Next() {
		var goods, state string
		var alias, batch, expiry *string
		var quantity, fromUnlabeled, wanted int64
		if err := rows.Scan(&goods, &state, &alias, &batch, &expiry, &quantity, &fromUnlabeled, &wanted); err != nil {
			return nil, err
		}
		parsedGoods, err := domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		parsedState, err := domain.NewStockState(state)
		if err != nil {
			return nil, err
		}
		listed = append(listed, domain.OutboundReservation{
			Goods: parsedGoods, State: parsedState, Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry),
			Quantity: quantity, FromUnlabeled: fromUnlabeled, Wanted: wanted,
		})
	}
	return listed, rows.Err()
}

type reservationScanner interface {
	Scan(dest ...any) error
}

func scanReservation(row reservationScanner) (ports.StoredReservation, error) {
	var warehouse, goods, state string
	var alias, batch, expiry *string
	var quantity, fromUnlabeled int64
	if err := row.Scan(&warehouse, &goods, &state, &alias, &batch, &expiry, &quantity, &fromUnlabeled); err != nil {
		return ports.StoredReservation{}, err
	}
	parsedWarehouse, err := domain.NewWarehouseCode(warehouse)
	if err != nil {
		return ports.StoredReservation{}, err
	}
	parsedGoods, err := domain.NewGoodsCode(goods)
	if err != nil {
		return ports.StoredReservation{}, err
	}
	parsedState, err := domain.NewStockState(state)
	if err != nil {
		return ports.StoredReservation{}, err
	}
	return ports.StoredReservation{
		Warehouse: parsedWarehouse, Goods: parsedGoods, State: parsedState, Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry),
		Quantity: quantity, FromUnlabeled: fromUnlabeled,
	}, nil
}
