package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type storedReceiptHeader struct {
	tenant           string
	factID           string
	device           string
	operator         string
	occurredAt       time.Time
	receivedAt       time.Time
	warehouse        string
	preAdvice        string
	unclaimedArrival *string
	shape            string
	outcome          string
}

func insertReceipt(ctx context.Context, executor bentopg.Executor, receipt domain.RecordedReceipt) (bool, error) {
	var id string
	arrivalRef, _ := receipt.UnclaimedArrival()
	err := executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.receipt_confirmation (
		    tenant_id, fact_id, device_id, operator_id, occurred_at, received_at,
		    warehouse_code, pre_advice_id, unclaimed_arrival_id, inbound_mode, outcome
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT ON CONSTRAINT receipt_confirmation_pkey DO NOTHING
		 RETURNING fact_id`,
		receipt.Tenant().String(), receipt.FactID().String(), receipt.Device().String(), receipt.Operator().String(),
		receipt.OccurredAt(), receipt.ReceivedAt(), receipt.Warehouse().String(), receipt.PreAdvice().String(),
		nullableText(arrivalRef.String()), receipt.Shape().String(), receipt.Outcome().String(),
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := insertReceiptLines(ctx, executor, receipt); err != nil {
		return false, err
	}
	if err := insertReceiptDifferences(ctx, executor, receipt); err != nil {
		return false, err
	}
	return true, nil
}

func insertReceiptLines(ctx context.Context, executor bentopg.Executor, receipt domain.RecordedReceipt) error {
	if receipt.Shape() == domain.InboundByCarton {
		return insertCartonReceiptLines(ctx, executor, receipt)
	}
	for index, line := range receipt.Lines() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.receipt_confirmation_line (
			    tenant_id, fact_id, line_index, observed_text, unit_name, stock_state, quantity,
			    batch_number, expiry_date, label_alias, location_code, goods_code, result, unposted_reason
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			receipt.Tenant().String(), receipt.FactID().String(), index, line.Observed(), line.Unit().String(),
			line.State().String(), line.Quantity(), nullableText(line.Batch().String()), nullableText(line.Expiry().String()),
			nullableText(line.Alias().String()), line.Location().String(), nullableText(line.Goods().String()),
			nullableText(line.Result().String()), nullableText(line.Unposted().String()),
		); err != nil {
			return err
		}
	}
	return nil
}

func insertReceiptDifferences(ctx context.Context, executor bentopg.Executor, receipt domain.RecordedReceipt) error {
	discrepancy, has := receipt.Discrepancy()
	if !has {
		return nil
	}
	for index, line := range receipt.Differences() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.receipt_discrepancy (
			    tenant_id, discrepancy_id, line_index, fact_id, kind, quantity, goods_code, observed_text, carton_number
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			receipt.Tenant().String(), discrepancy.String(), index, receipt.FactID().String(),
			line.Kind().String(), line.Quantity(), nullableText(line.Goods().String()), nullableText(line.Observed()),
			nullableText(line.Carton().String()),
		); err != nil {
			return err
		}
	}
	return nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func loadReceipts(ctx context.Context, querier bentopg.Querier, where string, args ...any) ([]domain.RecordedReceipt, error) {
	rows, err := querier.Query(ctx,
		`SELECT tenant_id, fact_id, device_id, operator_id, occurred_at, received_at,
		        warehouse_code, pre_advice_id, unclaimed_arrival_id, inbound_mode, outcome
		   FROM warehouse_fulfillment.receipt_confirmation
		  WHERE `+where+`
		  ORDER BY pre_advice_id, received_seq`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var headers []storedReceiptHeader
	for rows.Next() {
		var header storedReceiptHeader
		if err := rows.Scan(
			&header.tenant, &header.factID, &header.device, &header.operator, &header.occurredAt, &header.receivedAt,
			&header.warehouse, &header.preAdvice, &header.unclaimedArrival, &header.shape, &header.outcome,
		); err != nil {
			return nil, err
		}
		headers = append(headers, header)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(headers) == 0 {
		return nil, nil
	}
	facts := make([]string, len(headers))
	for index, header := range headers {
		facts[index] = header.factID
	}
	tenant := headers[0].tenant
	lines, err := loadReceiptLines(ctx, querier, tenant, facts)
	if err != nil {
		return nil, err
	}
	cartonLines, err := loadCartonReceiptLines(ctx, querier, tenant, facts)
	if err != nil {
		return nil, err
	}
	differences, discrepancyIDs, err := loadReceiptDifferences(ctx, querier, tenant, facts)
	if err != nil {
		return nil, err
	}
	receipts := make([]domain.RecordedReceipt, 0, len(headers))
	for _, header := range headers {
		receipt, err := header.receipt(lines[header.factID], cartonLines[header.factID], discrepancyIDs[header.factID], differences[header.factID])
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

func loadReceiptLines(ctx context.Context, querier bentopg.Querier, tenant string, facts []string) (map[string][]domain.ReceiptLine, error) {
	rows, err := querier.Query(ctx,
		`SELECT fact_id, line_index, observed_text, unit_name, stock_state, quantity,
		        batch_number, expiry_date::text, label_alias, location_code, goods_code, result, unposted_reason
		   FROM warehouse_fulfillment.receipt_confirmation_line
		  WHERE tenant_id = $1 AND fact_id = ANY($2)
		  ORDER BY fact_id, line_index`, tenant, facts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grouped := map[string][]domain.ReceiptLine{}
	expect := map[string]int{}
	for rows.Next() {
		var (
			factID, observed, unit, state, location     string
			index                                       int
			quantity                                    int64
			batch, expiry, alias, goods, result, reason *string
		)
		if err := rows.Scan(
			&factID, &index, &observed, &unit, &state, &quantity,
			&batch, &expiry, &alias, &location, &goods, &result, &reason,
		); err != nil {
			return nil, err
		}
		if index != expect[factID] {
			return nil, fmt.Errorf("receipt %s line index %d is not contiguous", factID, index)
		}
		expect[factID] = index + 1
		line, err := receiptLineFromColumns(observed, unit, state, quantity, batch, expiry, alias, location, goods, result, reason)
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

func loadReceiptDifferences(
	ctx context.Context,
	querier bentopg.Querier,
	tenant string,
	facts []string,
) (map[string][]domain.ReceiptDiscrepancyLine, map[string]domain.ReceiptDiscrepancyID, error) {
	rows, err := querier.Query(ctx,
		`SELECT fact_id, discrepancy_id, line_index, kind, quantity, goods_code, observed_text, carton_number
		   FROM warehouse_fulfillment.receipt_discrepancy
		  WHERE tenant_id = $1 AND fact_id = ANY($2)
		  ORDER BY fact_id, line_index`, tenant, facts)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	grouped := map[string][]domain.ReceiptDiscrepancyLine{}
	ids := map[string]domain.ReceiptDiscrepancyID{}
	expect := map[string]int{}
	for rows.Next() {
		var (
			factID, discrepancyID, kind string
			index                       int
			quantity                    int64
			goods, observed, carton     *string
		)
		if err := rows.Scan(&factID, &discrepancyID, &index, &kind, &quantity, &goods, &observed, &carton); err != nil {
			return nil, nil, err
		}
		if index != expect[factID] {
			return nil, nil, fmt.Errorf("receipt %s discrepancy index %d is not contiguous", factID, index)
		}
		expect[factID] = index + 1
		id, err := domain.NewReceiptDiscrepancyID(discrepancyID)
		if err != nil {
			return nil, nil, err
		}
		if current, seen := ids[factID]; seen && current != id {
			return nil, nil, fmt.Errorf("receipt %s has more than one discrepancy id", factID)
		}
		ids[factID] = id
		kindValue, err := domain.NewReceiptDiscrepancyKind(kind)
		if err != nil {
			return nil, nil, err
		}
		goodsCode := domain.GoodsCode{}
		if goods != nil {
			goodsCode, err = domain.NewGoodsCode(*goods)
			if err != nil {
				return nil, nil, err
			}
		}
		observedText := ""
		if observed != nil {
			observedText = *observed
		}
		var line domain.ReceiptDiscrepancyLine
		if carton != nil {
			number, err := domain.NewCartonNumber(*carton)
			if err != nil {
				return nil, nil, err
			}
			line, err = domain.NewCartonDiscrepancyLine(kindValue, number)
			if err != nil {
				return nil, nil, err
			}
		} else {
			line, err = domain.NewReceiptDiscrepancyLine(kindValue, quantity, goodsCode, observedText)
			if err != nil {
				return nil, nil, err
			}
		}
		grouped[factID] = append(grouped[factID], line)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return grouped, ids, nil
}

func (header storedReceiptHeader) receipt(
	lines []domain.ReceiptLine,
	cartonLines []domain.CartonReceiptLine,
	discrepancy domain.ReceiptDiscrepancyID,
	differences []domain.ReceiptDiscrepancyLine,
) (domain.RecordedReceipt, error) {
	if len(lines) == 0 && len(cartonLines) == 0 {
		return domain.RecordedReceipt{}, fmt.Errorf("receipt %s has no lines", header.factID)
	}
	tenant, err := domain.NewTenantID(header.tenant)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	factID, err := domain.NewReceiptFactID(header.factID)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	device, err := domain.NewReceiptDeviceID(header.device)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	operator, err := domain.NewReceiptOperatorID(header.operator)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	warehouse, err := domain.NewWarehouseCode(header.warehouse)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	preAdvice, err := domain.NewPreAdviceID(header.preAdvice)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	var arrivalID domain.UnclaimedArrivalID
	if header.unclaimedArrival != nil {
		arrivalID, err = domain.NewUnclaimedArrivalID(*header.unclaimedArrival)
		if err != nil {
			return domain.RecordedReceipt{}, err
		}
	}
	shape, err := domain.NewInboundMode(header.shape)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	outcome, err := recordedReceiptOutcome(header.outcome)
	if err != nil {
		return domain.RecordedReceipt{}, err
	}
	return domain.NewRecordedReceipt(domain.RecordedReceiptSpec{
		Tenant: tenant, FactID: factID, Device: device, Operator: operator,
		OccurredAt: header.occurredAt, ReceivedAt: header.receivedAt,
		Warehouse: warehouse, PreAdvice: preAdvice, UnclaimedArrival: arrivalID, Shape: shape, Outcome: outcome,
		Lines: lines, CartonLines: cartonLines, Discrepancy: discrepancy, Differences: differences,
	})
}

func recordedReceiptOutcome(word string) (domain.RecordedReceiptOutcome, error) {
	switch word {
	case domain.RecordedReceiptConfirmed.String():
		return domain.RecordedReceiptConfirmed, nil
	case domain.RecordedReceiptDivertedToUnclaimed.String():
		return domain.RecordedReceiptDivertedToUnclaimed, nil
	default:
		return domain.RecordedReceiptOutcomeInvalid, fmt.Errorf("receipt outcome %q", word)
	}
}

func receiptLineFromColumns(
	observed, unit, state string,
	quantity int64,
	batch, expiry, alias *string,
	location string,
	goods, result, reason *string,
) (domain.ReceiptLine, error) {
	unitName, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return domain.ReceiptLine{}, err
	}
	stockState, err := domain.NewStockState(state)
	if err != nil {
		return domain.ReceiptLine{}, err
	}
	locationCode, err := domain.NewLocationCode(location)
	if err != nil {
		return domain.ReceiptLine{}, err
	}
	number := domain.BatchNumber{}
	if batch != nil {
		number, err = domain.NewBatchNumber(*batch)
		if err != nil {
			return domain.ReceiptLine{}, err
		}
	}
	date := domain.ExpiryDate{}
	if expiry != nil {
		date, err = domain.NewExpiryDate(*expiry)
		if err != nil {
			return domain.ReceiptLine{}, err
		}
	}
	aliasCode := domain.GoodsAliasCode{}
	if alias != nil {
		aliasCode, err = domain.NewGoodsAliasCode(*alias)
		if err != nil {
			return domain.ReceiptLine{}, err
		}
	}
	line, err := domain.NewReceiptLine(observed, unitName, stockState, quantity, number, date, aliasCode, locationCode)
	if err != nil {
		return domain.ReceiptLine{}, err
	}
	switch {
	case result != nil && *result == domain.ReceiptLinePosted.String():
		if goods == nil {
			return domain.ReceiptLine{}, fmt.Errorf("posted receipt line has no goods")
		}
		goodsCode, err := domain.NewGoodsCode(*goods)
		if err != nil {
			return domain.ReceiptLine{}, err
		}
		return line.AsPosted(goodsCode)
	case result != nil && *result == domain.ReceiptLineUnknownGoods.String():
		return line.AsUnknown()
	case result == nil && reason != nil && *reason == domain.ReceiptUnpostedNotReceivable.String():
		return line.AsNotReceivable()
	default:
		return domain.ReceiptLine{}, fmt.Errorf("receipt line result %v reason %v", result, reason)
	}
}
