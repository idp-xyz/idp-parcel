package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 收费发生项不带金额。单位、库龄或时区没登记时不形成。一种来源一份，更正只追加版本，一版只能被更正一次。
func TestAChargeOccurrenceFormsWithoutAnAmountAndSkipsWhenUnitsAreMissing(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedWarehouse(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	early := world.chargeHandler(t, time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC))
	late := world.chargeHandler(t, preAdviceNumberingInstant)
	draft := application.ChargeDraft{
		Kind: "INBOUND_HANDLING", Source: "receipt-1", Owner: owner, Warehouse: warehouse, Occurred: preAdviceNumberingInstant,
		Measures: []ports.ChargeMeasure{{Unit: "PIECE", Quantity: "2"}},
	}
	if missed := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Form(ctx, tenant, draft)
	}); missed.Outcome() != application.ChargeNotConfigured || world.chargeCount(t) != 0 {
		t.Fatalf("没单位 %s 份数 %d", missed.Outcome(), world.chargeCount(t))
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return early.RegisterUnits(ctx, tenant, warehouse, "KG", "M3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("单位没有登记")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeNotConfigured {
		t.Fatal("库龄没登记却形成了仓租")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return early.RegisterAging(ctx, tenant, warehouse, "ACTUAL_BATCH")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("库龄没有登记")
	}
	formed := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Form(ctx, tenant, draft)
	})
	if formed.Outcome() != application.ChargeFormed || formed.Version() != 1 || formed.Occurrence() == "" {
		t.Fatalf("形成 %s %s %d", formed.Outcome(), formed.Occurrence(), formed.Version())
	}
	again := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Form(ctx, tenant, draft)
	})
	if again.Outcome() != application.ChargeExisting || again.Occurrence() != formed.Occurrence() || world.chargeCount(t) != 1 {
		t.Fatalf("重放 %s %s 份数 %d", again.Outcome(), again.Occurrence(), world.chargeCount(t))
	}
	voided := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Void(ctx, tenant, formed.Occurrence(), 1)
	})
	if voided.Outcome() != application.ChargeVoided || voided.Version() != 2 {
		t.Fatalf("作废 %s %d", voided.Outcome(), voided.Version())
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Restate(ctx, tenant, formed.Occurrence(), 1, draft)
	}).Outcome() != application.ChargeConflict {
		t.Fatal("同一版被更正了两次")
	}
	restated := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.Restate(ctx, tenant, formed.Occurrence(), 2, application.ChargeDraft{
			Measures: []ports.ChargeMeasure{{Unit: "PIECE", Quantity: "3"}},
		})
	})
	if restated.Outcome() != application.ChargeRestated || restated.Version() != 3 {
		t.Fatalf("重述 %s %d", restated.Outcome(), restated.Version())
	}
	voidedVersion, found, err := late.Read(t.Context(), tenant, formed.Occurrence(), 2)
	if err != nil || !found || voidedVersion.Correction != "VOID" || len(voidedVersion.Measures) != 0 || len(voidedVersion.Lines) != 0 {
		t.Fatalf("作废版 %+v %v", voidedVersion, err)
	}
	if added := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.FormValueAdded(ctx, tenant, "LABEL", application.ChargeDraft{
			Source: "job-1", Owner: owner, Warehouse: warehouse, Occurred: preAdviceNumberingInstant,
			Measures: []ports.ChargeMeasure{{Unit: "PIECE", Quantity: "1"}},
		})
	}); added.Outcome() != application.ChargeNotConfigured {
		t.Fatalf("没登记的增值项目 %s", added.Outcome())
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return early.RegisterProject(ctx, tenant, warehouse, "LABEL")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("项目没有登记")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.FormValueAdded(ctx, tenant, "LABEL", application.ChargeDraft{
			Source: "job-1", Owner: owner, Warehouse: warehouse, Occurred: preAdviceNumberingInstant,
			Measures: []ports.ChargeMeasure{{Unit: "PIECE", Quantity: "1"}},
		})
	}).Outcome() != application.ChargeFormed {
		t.Fatal("登记过的增值项目没有形成")
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeNotConfigured {
		t.Fatal("登记之前的日子被补上了")
	}
	closed := world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	})
	if closed.Outcome() != application.ChargeFormed {
		t.Fatalf("仓租 %s", closed.Outcome())
	}
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return late.CloseStorageDay(ctx, tenant, owner, warehouse, time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	}).Outcome() != application.ChargeExisting {
		t.Fatal("同一计费日又形成了一份")
	}
	body := world.chargePayload(t, formed.Occurrence())
	if len(body) != 3 || body["tenantId"] == nil || body["occurrenceId"] == nil || body["version"] == nil {
		t.Fatalf("信封 %+v", body)
	}
	if world.chargeAmountColumns(t) != 0 || world.chargeBodyMentionsAmount(t) {
		t.Fatal("发生项带了金额")
	}
	store, err := outbox.NewStore(world.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxChargeHandoff(world.db, store, world.clock)
	if err != nil {
		t.Fatal(err)
	}
	if err := handoff.HandOffChargeOccurrence(context.Background(), "SYN-TENANT-01", "CHO-OUTSIDE", 1); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HandOffChargeOccurrence：%v", err)
	}
}

func TestConfirmingAReceiptStandsWhenChargeUnitsAreNotRegistered(t *testing.T) {
	world := newReceiptWorld(t)
	world.charges = world.chargeHandler(t, world.clock.Now())
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-CHARGE-1", "SYN-GOODS-01", "件", 4)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-CHARGE-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 4,
	})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.chargeCount(t) != 0 || world.stockOf(t, "SYN-GOODS-01") != 4 {
		t.Fatalf("没收单位的收货 %s 份数 %d 库存 %d", confirmed.Outcome(), world.chargeCount(t), world.stockOf(t, "SYN-GOODS-01"))
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if world.inCharge(t, func(ctx context.Context) (application.ChargeResult, error) {
		return world.charges.RegisterUnits(ctx, tenant, valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), "KG", "M3")
	}).Outcome() != application.ChargeFormed {
		t.Fatal("单位没有登记")
	}
	second := world.adviseGoods(t, "REQ-CHARGE-2", "SYN-GOODS-01", "件", 3)
	confirmed = world.confirm(t, receiptCommand(t, "SYN-FACT-CHARGE-2", second.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 3,
	})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.chargeCount(t) != 1 {
		t.Fatalf("有单位的收货 %s 份数 %d", confirmed.Outcome(), world.chargeCount(t))
	}
	version, found, err := world.charges.Read(t.Context(), tenant, world.chargeID(t, "INBOUND_HANDLING"), 1)
	if err != nil || !found || version.Kind != "INBOUND_HANDLING" || len(version.Lines) != 1 || version.Lines[0].Kind != "RECEIPT_LINE" {
		t.Fatalf("入库发生项 %+v %v", version, err)
	}
	if attributeValue(version.Lines[0].Attributes, "ZONE_USAGE") != "RECEIVING_STAGING" {
		t.Fatalf("库区用途 %+v", version.Lines[0].Attributes)
	}
	if attributeValue(version.Attributes, "ARRIVAL_MODE") == "" {
		t.Fatal("到仓方式没有留下")
	}
}

func (world receiptWorld) chargeHandler(t *testing.T, at time.Time) *application.ChargeHandler {
	t.Helper()
	store, err := adapter.NewCharges(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewChargeIdentities()
	if err != nil {
		t.Fatal(err)
	}
	messages, err := outbox.NewStore(world.db)
	if err != nil {
		t.Fatal(err)
	}
	clock := fixedClock{at: at}
	handoff, err := adapter.NewOutboxChargeHandoff(world.db, messages, clock)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewChargeHandler(store, identities, handoff, world.warehouses, clock)
}

func (world receiptWorld) inCharge(t *testing.T, run func(context.Context) (application.ChargeResult, error)) application.ChargeResult {
	t.Helper()
	var result application.ChargeResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		result, runErr = run(ctx)
		return runErr
	})
	if err != nil {
		t.Fatalf("收费：%v", err)
	}
	return result
}

func (world receiptWorld) chargeCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.charge_occurrence WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) chargeID(t *testing.T, kind string) string {
	t.Helper()
	var id string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT occurrence_id FROM warehouse_fulfillment.charge_occurrence WHERE tenant_id = $1 AND kind = $2`,
		"SYN-TENANT-01", kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (world receiptWorld) chargePayload(t *testing.T, occurrence string) map[string]json.RawMessage {
	t.Helper()
	var raw []byte
	if err := world.pool.QueryRow(t.Context(),
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND payload::text LIKE $2`,
		adapter.ChargeOccurrenceEventType, "%"+occurrence+"%").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func (world receiptWorld) chargeAmountColumns(t *testing.T) int {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_schema = 'warehouse_fulfillment' AND table_name LIKE 'charge%' AND column_name = 'amount'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) chargeBodyMentionsAmount(t *testing.T) bool {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.charge_occurrence_version
		  WHERE measures::text LIKE '%amount%' OR attributes::text LIKE '%amount%' OR lines::text LIKE '%amount%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count != 0
}

func attributeValue(attributes []ports.ChargeAttribute, name string) string {
	for _, attribute := range attributes {
		if attribute.Name == name {
			return attribute.Value
		}
	}
	return ""
}
