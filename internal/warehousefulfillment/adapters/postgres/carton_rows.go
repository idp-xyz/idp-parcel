package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func insertCartonMovement(ctx context.Context, executor bentopg.Executor, movement domain.CartonMovement) (bool, error) {
	var inserted string
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.carton_movement (
		    tenant_id, movement_id, owner_account_id, warehouse_code, reason, source_reference, occurred_at, posted_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, reason, source_reference) DO NOTHING
		 RETURNING movement_id`,
		movement.Tenant().String(), movement.ID().String(), movement.Owner().String(), movement.Warehouse().String(),
		movement.Reason().String(), movement.Source().String(), movement.OccurredAt(), movement.PostedAt(),
	).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertCartonInboundBatch(ctx context.Context, executor bentopg.Executor, batch domain.CartonInboundBatch) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.carton_inbound_batch (
		    tenant_id, inbound_batch_id, owner_account_id, warehouse_code, carton_number, inbound_at, formed_by_movement_id
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		batch.Tenant().String(), batch.ID().String(), batch.Owner().String(), batch.Warehouse().String(),
		batch.Carton().String(), batch.InboundAt(), batch.MovementID().String(),
	)
	return err
}

func insertCartonMovementLine(
	ctx context.Context,
	executor bentopg.Executor,
	movement domain.CartonMovement,
	position int,
	line domain.CartonMovementLine,
) error {
	_, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.carton_movement_line (
		    tenant_id, movement_id, position, owner_account_id, warehouse_code,
		    location_code, carton_number, owner_carton_number, inbound_batch_id, quantity_delta
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 1)`,
		movement.Tenant().String(), movement.ID().String(), position, movement.Owner().String(), movement.Warehouse().String(),
		line.Location().String(), line.Carton().String(), nullableText(line.OwnerCarton().String()), line.Batch().String(),
	)
	return err
}

func onHandCartonNumbers(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
) ([]domain.CartonNumber, error) {
	rows, err := querier.Query(ctx,
		`SELECT carton_number
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3
		  GROUP BY carton_number
		 HAVING sum(quantity_delta) > 0
		  ORDER BY carton_number`,
		tenant.String(), owner.String(), warehouse.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var numbers []domain.CartonNumber
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		number, err := domain.NewCartonNumber(text)
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return numbers, nil
}

func insertCartonReceiptLines(ctx context.Context, executor bentopg.Executor, receipt domain.RecordedReceipt) error {
	for index, line := range receipt.CartonLines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.receipt_confirmation_carton_line (
			    tenant_id, fact_id, line_index, carton_number, warehouse_carton_number,
			    damaged, location_code, result, unposted_reason
			 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			receipt.Tenant().String(), receipt.FactID().String(), index, line.Number().String(),
			nullableText(line.WarehouseNumber().String()), line.Damaged(), line.Location().String(),
			nullableText(line.Result().String()), nullableText(line.Unposted().String()),
		); err != nil {
			return err
		}
	}
	return nil
}

func loadCartonReceiptLines(
	ctx context.Context,
	querier bentopg.Querier,
	tenant string,
	facts []string,
) (map[string][]domain.CartonReceiptLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT fact_id, line_index, carton_number, warehouse_carton_number, damaged, location_code, result, unposted_reason
		   FROM warehouse_fulfillment.receipt_confirmation_carton_line
		  WHERE tenant_id = $1 AND fact_id = ANY($2)
		  ORDER BY fact_id, line_index`, tenant, facts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grouped := map[string][]domain.CartonReceiptLine{}
	expect := map[string]int{}
	for rows.Next() {
		var (
			factID, carton, location  string
			index                     int
			warehouse, result, reason *string
			damaged                   bool
		)
		if err := rows.Scan(&factID, &index, &carton, &warehouse, &damaged, &location, &result, &reason); err != nil {
			return nil, err
		}
		if index != expect[factID] {
			return nil, fmt.Errorf("receipt %s carton line index %d is not contiguous", factID, index)
		}
		expect[factID] = index + 1
		line, err := cartonReceiptLineFromColumns(carton, warehouse, damaged, location, result, reason)
		if err != nil {
			return nil, err
		}
		grouped[factID] = append(grouped[factID], line)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grouped, nil
}

func cartonReceiptLineFromColumns(
	carton string,
	warehouse *string,
	damaged bool,
	location string,
	result, reason *string,
) (domain.CartonReceiptLine, error) {
	number, err := domain.NewCartonNumber(carton)
	if err != nil {
		return domain.CartonReceiptLine{}, err
	}
	var assigned domain.CartonNumber
	if warehouse != nil {
		assigned, err = domain.NewCartonNumber(*warehouse)
		if err != nil {
			return domain.CartonReceiptLine{}, err
		}
	}
	code, err := domain.NewLocationCode(location)
	if err != nil {
		return domain.CartonReceiptLine{}, err
	}
	line, err := domain.NewCartonReceiptLine(number, assigned, damaged, code)
	if err != nil {
		return domain.CartonReceiptLine{}, err
	}
	switch {
	case result != nil && *result == domain.ReceiptLinePosted.String() && reason == nil:
		return line.AsPosted()
	case result == nil && reason != nil && *reason == domain.ReceiptUnpostedWrongCarton.String():
		return line.AsWrongCarton()
	case result == nil && reason != nil && *reason == domain.ReceiptUnpostedCartonNumberInUse.String():
		return line.AsCartonNumberInUse()
	case result == nil && reason != nil && *reason == domain.ReceiptUnpostedNotReceivable.String():
		return line.AsNotReceivable()
	default:
		return domain.CartonReceiptLine{}, fmt.Errorf("carton receipt line result %v reason %v", result, reason)
	}
}
