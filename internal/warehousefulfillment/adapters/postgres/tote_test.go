package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) registerTotes(t *testing.T, handler *application.WaveHandler, warehouse string, codes ...string) application.WaveResult {
	t.Helper()
	command := application.RegisterTotesCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, warehouse),
		Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
	}
	for _, code := range codes {
		command.Codes = append(command.Codes, valueOf(t, domain.NewToteCode, code))
	}
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterTotes(ctx, command)
		return registerErr
	})
	if err != nil {
		t.Fatalf("登记周转箱：%v", err)
	}
	return result
}

func (world receiptWorld) retireTote(t *testing.T, handler *application.WaveHandler, code string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var retireErr error
		result, retireErr = handler.RetireTote(ctx, application.RetireToteCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
			Code: valueOf(t, domain.NewToteCode, code), Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return retireErr
	})
	if err != nil {
		t.Fatalf("停用周转箱 %s：%v", code, err)
	}
	return result
}

func (world receiptWorld) totes(t *testing.T, raw string) map[string]ports.ToteRow {
	t.Helper()
	store, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	query, err := ports.TotePages.DecodeRaw(raw)
	if err != nil {
		t.Fatalf("周转箱查询 %q：%v", raw, err)
	}
	page, err := store.ListTotes(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 50, query)
	if err != nil {
		t.Fatalf("列周转箱：%v", err)
	}
	rows := map[string]ports.ToteRow{}
	for _, row := range page.Rows {
		rows[row.ToteCode] = row
	}
	return rows
}

// Covers: 周转箱按仓登记，一次一批。已经在用的箱号照旧，一个新的都没有时答已有结果；
// 没登记的仓、代理仓不收；一个箱号都没有、超过一次的上限不收。
func TestTotesAreRegisteredPerWarehouseInBatches(t *testing.T) {
	world, handler, _ := withdrawalWorld(t, 10)
	if got := world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01", "SYN-TOTE-02", "SYN-TOTE-01").Outcome(); got != application.WaveTotesRegistered {
		t.Fatalf("登记两个箱 = %s", got)
	}
	if got := world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-02").Outcome(); got != application.WaveExisting {
		t.Fatalf("原样再登 = %s", got)
	}
	if got := world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-02", "SYN-TOTE-03").Outcome(); got != application.WaveTotesRegistered {
		t.Fatalf("一批里有一个新的 = %s", got)
	}
	if got := world.registerTotes(t, handler, "SYN-WH-MISSING", "SYN-TOTE-01").Outcome(); got != application.WaveWarehouseNotFound {
		t.Fatalf("没登记的仓 = %s", got)
	}
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	if got := world.registerTotes(t, handler, "SYN-WH-AGENT", "SYN-TOTE-01").Outcome(); got != application.WaveWarehouseNotAccepting {
		t.Fatalf("代理仓 = %s", got)
	}
	if got := world.registerTotes(t, handler, "SYN-WH-01").Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("一个箱号都没有 = %s", got)
	}
	many := make([]string, 501)
	for index := range many {
		many[index] = "SYN-TOTE-MANY-" + string(rune('A'+index%26)) + string(rune('A'+index/26))
	}
	if got := world.registerTotes(t, handler, "SYN-WH-01", many...).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("超过一次的上限 = %s", got)
	}
	rows := world.totes(t, "warehouseCode=SYN-WH-01")
	if len(rows) != 3 || rows["SYN-TOTE-01"].State != ports.ToteFree || rows["SYN-TOTE-03"].RegisteredBy != "operator-2" {
		t.Fatalf("名册 %+v", rows)
	}
}

// Covers: 停用的箱留在名册上、标成已停用，再登记同一个箱号就恢复在用。没有的箱答查无此箱，已停用的再停答已有结果。
// 箱上还绑着出库请求时不停。列表按状态筛，在用的带着绑着的出库请求。
func TestARetiredToteStaysOnTheRegisterAndABoundOneIsNotRetired(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 10)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01", "SYN-TOTE-02", "SYN-TOTE-03").Outcome() != application.WaveTotesRegistered {
		t.Fatal("没有登记成")
	}
	if got := world.retireTote(t, handler, "SYN-TOTE-NOT-THERE").Outcome(); got != application.WaveToteNotFound {
		t.Fatalf("没有的箱 = %s", got)
	}
	if got := world.retireTote(t, handler, "SYN-TOTE-01").Outcome(); got != application.WaveToteRetired {
		t.Fatalf("停用 = %s", got)
	}
	if got := world.retireTote(t, handler, "SYN-TOTE-01").Outcome(); got != application.WaveExisting {
		t.Fatalf("已停用的再停 = %s", got)
	}
	request := reserve("REQ-TOTE-HELD")
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.tote_binding (tenant_id, warehouse_code, tote_code, request_id, bound_at)
		 VALUES ('SYN-TENANT-01', 'SYN-WH-01', 'SYN-TOTE-02', $1, $2)`, request.String(), world.clock.Now()); err != nil {
		t.Fatalf("摆一条绑定：%v", err)
	}
	if got := world.retireTote(t, handler, "SYN-TOTE-02").Outcome(); got != application.WaveToteInUse {
		t.Fatalf("还绑着出库请求的箱 = %s", got)
	}
	rows := world.totes(t, "")
	if len(rows) != 3 || rows["SYN-TOTE-01"].State != ports.ToteRetired || rows["SYN-TOTE-01"].RetiredAt == nil || rows["SYN-TOTE-01"].RetiredBy != "operator-2" ||
		rows["SYN-TOTE-02"].State != ports.ToteInUse || rows["SYN-TOTE-02"].RequestID != request.String() || rows["SYN-TOTE-02"].BoundAt == nil ||
		rows["SYN-TOTE-03"].State != ports.ToteFree || rows["SYN-TOTE-03"].RequestID != "" {
		t.Fatalf("名册 %+v", rows)
	}
	for state, code := range map[string]string{"RETIRED": "SYN-TOTE-01", "IN_USE": "SYN-TOTE-02", "FREE": "SYN-TOTE-03"} {
		if filtered := world.totes(t, "state="+state); len(filtered) != 1 || filtered[code].State != state {
			t.Fatalf("按 %s 筛 %+v", state, filtered)
		}
	}
	if found := world.totes(t, "q="+request.String()); len(found) != 1 || found["SYN-TOTE-02"].State != ports.ToteInUse {
		t.Fatalf("按出库请求号搜 %+v", found)
	}
	if got := world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01").Outcome(); got != application.WaveTotesRegistered {
		t.Fatalf("停用的箱再登记 = %s", got)
	}
	if rows := world.totes(t, ""); rows["SYN-TOTE-01"].State != ports.ToteFree || rows["SYN-TOTE-01"].RetiredAt != nil {
		t.Fatalf("恢复在用之后 %+v", rows["SYN-TOTE-01"])
	}
}

func TestWritingTotesRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, warehouse := valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code, operator := valueOf(t, domain.NewToteCode, "SYN-TOTE-01"), valueOf(t, domain.NewRegistrant, "operator-2")
	if _, err := waves.RegisterTote(context.Background(), tenant, warehouse, code, operator, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RegisterTote：%v", err)
	}
	if err := waves.RetireTote(context.Background(), tenant, warehouse, code, operator, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RetireTote：%v", err)
	}
}
