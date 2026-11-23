package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestAReturnIsAdvisedCountedAndDisposedWithoutReversingTheOriginalShipment(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-RET", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageReturnStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-RET", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-RET")
	}))
	handler := world.returns(t)
	accepted := world.acceptReturn(t, handler, "return-1", "TRK-1")
	if accepted.Outcome() != application.ReturnAccepted || world.stockUnitAt(t, "SYN-LOC-RET", "件") != 0 {
		t.Fatalf("预报 %s 库存 %d", accepted.Outcome(), world.stockUnitAt(t, "SYN-LOC-RET", "件"))
	}
	advice, ok := accepted.Advice()
	if !ok || advice.Number == "" {
		t.Fatal("预报没有单据号")
	}
	if again := world.acceptReturn(t, handler, "return-1", "TRK-1"); again.Outcome() != application.ReturnExisting {
		t.Fatalf("重投 %s", again.Outcome())
	}
	if world.receiveReturn(t, handler, "FACT-1", "TRK-1", "").Outcome() != application.ReturnCounted || world.stockUnitAt(t, "SYN-LOC-RET", "件") != 2 {
		t.Fatalf("清点后库存 %d", world.stockUnitAt(t, "SYN-LOC-RET", "件"))
	}
	var movements int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement WHERE tenant_id = $1 AND reason = 'OUTBOUND_CONFIRMATION'`,
		"SYN-TENANT-01").Scan(&movements); err != nil || movements != 0 {
		t.Fatalf("原出库确认被冲了 %d %v", movements, err)
	}
	unknown := world.receiveReturn(t, handler, "FACT-2", "TRK-UNKNOWN", "")
	if unknown.Outcome() != application.ReturnUnclaimed || world.unclaimedCount(t) != 1 || world.stockUnitAt(t, "SYN-LOC-RET", "件") != 2 {
		t.Fatalf("认不出 %s 到货 %d", unknown.Outcome(), world.unclaimedCount(t))
	}
	rules, err := adapter.NewUnclaimedDisposalRules(world.db)
	if err != nil {
		t.Fatal(err)
	}
	claims := application.NewManageUnclaimedArrivalHandler(world.arrivals, rules, world.preAdvices, world.warehouses, alwaysOwner{}, world.clock)
	arrivalID := valueOf(t, domain.NewUnclaimedArrivalID, unknown.Arrival())
	err = world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		claimed, claimErr := claims.Claim(ctx, application.ClaimUnclaimedArrivalCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), DecisionID: valueOf(t, domain.NewRequestKey, "claim-1"),
			ArrivalID: arrivalID, Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		if claimErr != nil {
			return claimErr
		}
		if claimed.Outcome() != application.ClaimUnclaimedClaimed {
			return fmt.Errorf("认领 %s", claimed.Outcome())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if world.receiveReturn(t, handler, "FACT-3", "TRK-UNKNOWN", unknown.Arrival()).Outcome() != application.ReturnCounted {
		t.Fatal("认领后再入库没有清点")
	}
	if world.disposeReturn(t, handler, "DEC-1", "FACT-1", domain.ReturnDestinationGoodPutaway).Outcome() != application.ReturnDisposed {
		t.Fatal("去向没有记下")
	}
	if world.disposeReturn(t, handler, "DEC-2", "FACT-1", domain.ReturnDestinationDestroy).Outcome() != application.ReturnDisposed {
		t.Fatal("销毁去向没有记下")
	}
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement WHERE tenant_id = $1 AND reason = 'DESTRUCTION'`,
		"SYN-TENANT-01").Scan(&movements); err != nil || movements != 0 || world.stockUnitAt(t, "SYN-LOC-RET", "件") < 2 {
		t.Fatalf("处置入了账 销毁 %d 库存 %d", movements, world.stockUnitAt(t, "SYN-LOC-RET", "件"))
	}
	world.moveReturnAside(t)
	if world.disposeReturn(t, handler, "DEC-3", "FACT-1", domain.ReturnDestinationGoodPutaway).Outcome() != application.ReturnContentInvalid {
		t.Fatal("货已不在暂存仍作出了处置")
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_request (
		    tenant_id, request_id, owner_account_id, warehouse_code, origin, request_key, dispatch,
		    recipient, address, status, numbering_absence, received_at)
		 VALUES ('SYN-TENANT-01','OBR-RET','SYN-OWNER-01','SYN-WH-01','CUSTOMER','REQ-RET','OWNER_LABEL',
		         '收件人','地址','PACKED','NUMBERING_TIME_ZONE_NOT_CONFIGURED', now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_owner_label (tenant_id, request_id, parcel_sequence, external_number)
		 VALUES ('SYN-TENANT-01','OBR-RET',1,'EXT-PACK')`); err != nil {
		t.Fatal(err)
	}
	if world.receiveReturn(t, handler, "FACT-4", "EXT-PACK", "").Outcome() != application.ReturnCounted {
		t.Fatal("装箱外部单号没有认出货主")
	}
	missing := world.acceptReturnWarehouse(t, handler, "SYN-WH-MISSING")
	if missing.Outcome() != application.ReturnWarehouseNotFound {
		t.Fatalf("没登记的仓 %s", missing.Outcome())
	}
}

func (world receiptWorld) returns(t *testing.T) *application.ReturnHandler {
	t.Helper()
	store, err := adapter.NewReturns(world.db)
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := adapter.NewReturnNumbers(world.db, world.assigner)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewReturnIdentities()
	if err != nil {
		t.Fatal(err)
	}
	unclaimed := application.NewRegisterUnclaimedArrivalHandler(world.arrivals, world.arrivalIDs, world.warehouses, world.clock)
	return application.NewReturnHandler(
		store, identities, numbers, world.warehouses, world.locations, world.zones, world.warehouses, alwaysOwner{},
		world.poster(), world.ledger, unclaimed, world.arrivals, world.clock,
	)
}

func (world receiptWorld) acceptReturn(t *testing.T, handler *application.ReturnHandler, key, tracking string) application.ReturnResult {
	t.Helper()
	return world.acceptReturnWarehouse(t, handler, "SYN-WH-01", key, tracking)
}

func (world receiptWorld) acceptReturnWarehouse(t *testing.T, handler *application.ReturnHandler, warehouse string, keyAndTracking ...string) application.ReturnResult {
	t.Helper()
	key, tracking := "return-missing", "TRK-MISSING"
	if len(keyAndTracking) == 2 {
		key, tracking = keyAndTracking[0], keyAndTracking[1]
	}
	var result application.ReturnResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var acceptErr error
		result, acceptErr = handler.Accept(ctx, application.AcceptReturnCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Warehouse: valueOf(t, domain.NewWarehouseCode, warehouse), Origin: domain.ReturnOriginCustomer, Key: key, Tracking: tracking,
			Lines: []ports.ReturnLine{{
				Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), Quantity: 2,
			}},
		})
		return acceptErr
	})
	if err != nil {
		t.Fatalf("预报：%v", err)
	}
	return result
}

func (world receiptWorld) receiveReturn(t *testing.T, handler *application.ReturnHandler, fact, tracking, arrival string) application.ReturnResult {
	t.Helper()
	var result application.ReturnResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var receiveErr error
		result, receiveErr = handler.Receive(ctx, application.ReceiveReturnCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewReturnFactID, fact),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RET"),
			Tracking: tracking, Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"),
			State: domain.StockStateGood, Quantity: 2, Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
			Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"), Arrival: returnArrival(t, arrival),
		})
		return receiveErr
	})
	if err != nil {
		t.Fatalf("到仓：%v", err)
	}
	return result
}

func (world receiptWorld) moveReturnAside(t *testing.T) {
	t.Helper()
	rows, err := world.pool.Query(t.Context(),
		`SELECT inbound_batch_id, sum(quantity_delta)
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = 'SYN-TENANT-01' AND location_code = 'SYN-LOC-RET'
		  GROUP BY inbound_batch_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type held struct {
		batch string
		qty   int64
	}
	var cells []held
	for rows.Next() {
		var cell held
		if err := rows.Scan(&cell.batch, &cell.qty); err != nil {
			t.Fatal(err)
		}
		if cell.qty > 0 {
			cells = append(cells, cell)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	err = world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		for index, cell := range cells {
			batch, err := domain.NewInboundBatchID(cell.batch)
			if err != nil {
				return err
			}
			from, err := domain.NewStockCell(domain.StockCellSpec{
				Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RET"), Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
				Unit: valueOf(t, domain.NewGoodsUnitName, "件"), State: domain.StockStateGood,
			})
			if err != nil {
				return err
			}
			from = from.WithInboundBatch(batch)
			place, err := domain.NewStockCell(domain.StockCellSpec{
				Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"), Goods: from.Goods(), Unit: from.Unit(), State: from.State(),
			})
			if err != nil {
				return err
			}
			place = place.WithInboundBatch(batch)
			source, err := domain.NewSourceFactReference("move-aside-" + cell.batch)
			if err != nil {
				return err
			}
			posted, err := world.poster().Post(ctx, application.PostStockMovementCommand{
				Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
				Request: domain.NewStockMovementRequest(
					valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
					domain.StockReasonInternalMove, source, world.clock.Now(),
					[]domain.StockLineRequest{
						domain.NewStockLineRequest(domain.StockOut, cell.qty, from, time.Time{}),
						domain.NewStockLineRequest(domain.StockIn, cell.qty, place, time.Time{}),
					},
				),
			})
			if err != nil {
				return err
			}
			if posted.Outcome() != application.PostStockPosted {
				return errMoveAside(index)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("移出退货暂存：%v", err)
	}
}

type errMoveAside int

func (step errMoveAside) Error() string { return "internal move was not posted" }

func returnArrival(t *testing.T, arrival string) domain.UnclaimedArrivalID {
	t.Helper()
	if arrival == "" {
		return domain.UnclaimedArrivalID{}
	}
	return valueOf(t, domain.NewUnclaimedArrivalID, arrival)
}

func (world receiptWorld) disposeReturn(t *testing.T, handler *application.ReturnHandler, decision, fact string, destination domain.ReturnDestination) application.ReturnResult {
	t.Helper()
	var result application.ReturnResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var disposeErr error
		result, disposeErr = handler.Dispose(ctx, application.DisposeReturnCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Decision: valueOf(t, domain.NewReturnDecisionID, decision),
			Fact: valueOf(t, domain.NewReturnFactID, fact), Destination: destination, Origin: domain.ReturnOriginCustomer,
			Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		})
		return disposeErr
	})
	if err != nil {
		t.Fatalf("处置：%v", err)
	}
	return result
}
