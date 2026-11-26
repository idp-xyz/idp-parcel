package postgres_test

import (
	"context"
	"errors"
	"net/url"
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

// Covers: 箱号只在一个仓里唯一。两个仓各有一个同名的箱时，列表一页一行往下翻，两行都出来、不重不漏——
// 翻页的定位带着仓，不是只凭箱号。
func TestTheToteListPagesAcrossWarehousesThatShareAToteCode(t *testing.T) {
	world, handler, _ := withdrawalWorld(t, 10)
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-02", 1, nil))
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01", "SYN-TOTE-02").Outcome() != application.WaveTotesRegistered ||
		world.registerTotes(t, handler, "SYN-WH-02", "SYN-TOTE-01").Outcome() != application.WaveTotesRegistered {
		t.Fatal("两个仓的箱没有登记成")
	}
	store, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	raw := ""
	for step := 0; step < 5; step++ {
		query, err := ports.TotePages.DecodeRaw(raw)
		if err != nil {
			t.Fatalf("查询 %q：%v", raw, err)
		}
		page, err := store.ListTotes(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 1, query)
		if err != nil {
			t.Fatalf("列周转箱：%v", err)
		}
		for _, row := range page.Rows {
			seen = append(seen, row.WarehouseCode+"/"+row.ToteCode)
		}
		next := page.Page(1).Next
		if next == nil {
			break
		}
		raw = "after=" + url.QueryEscape(*next)
	}
	if len(seen) != 3 || seen[0] != "SYN-WH-01/SYN-TOTE-01" || seen[1] != "SYN-WH-02/SYN-TOTE-01" || seen[2] != "SYN-WH-01/SYN-TOTE-02" {
		t.Fatalf("一页一行翻下来 %v", seen)
	}
}

func (world receiptWorld) pickInto(t *testing.T, handler *application.WaveHandler, task domain.PickTaskID, quantity int64, tote string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var pickErr error
		result, pickErr = handler.Pick(ctx, application.PickCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, "SYN-FACT-"+task.String()),
			TaskID: task, Quantity: quantity, Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT"),
			Tote: valueOf(t, domain.NewToteCode, tote),
		})
		return pickErr
	})
	if err != nil {
		t.Fatalf("拣货进箱 %s：%v", tote, err)
	}
	return result
}

func (world receiptWorld) releaseTote(t *testing.T, handler *application.WaveHandler, code string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		result, releaseErr = handler.ReleaseTote(ctx, application.ReleaseToteCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
			Code: valueOf(t, domain.NewToteCode, code), Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return releaseErr
	})
	if err != nil {
		t.Fatalf("放开周转箱 %s：%v", code, err)
	}
	return result
}

func (world receiptWorld) totesOf(t *testing.T, request domain.OutboundRequestID) []ports.OutboundToteDetail {
	t.Helper()
	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	detail, found, err := reads.OutboundRequestDetail(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request.String())
	if err != nil || !found {
		t.Fatalf("读出库请求 %s：found %v err %v", request, found, err)
	}
	return detail.Totes
}

// Covers: 拣货时带上箱号，箱就绑到这张出库请求上；同一张请求再往这个箱里拣照旧，也可以换一个箱。
// 箱绑着别的请求、没登记、已停用时这一笔拣货整份不成立——任务仍待执行，货没进待出库。
// 打完包货都进了包裹，这张请求的箱全放开，可以绑别的请求。
func TestPickingIntoAToteBindsItUntilTheRequestIsPacked(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01", "SYN-TOTE-02", "SYN-TOTE-03").Outcome() != application.WaveTotesRegistered ||
		world.retireTote(t, handler, "SYN-TOTE-03").Outcome() != application.WaveToteRetired {
		t.Fatal("箱没有备好")
	}
	first := reserve("REQ-TOTE-A", 10)
	released := world.releaseRequests(t, handler, "WAVE-TOTE-A", first)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %d 条", released.Outcome(), len(released.Tasks()))
	}
	near, far := taskAt(t, released.Tasks(), "SYN-LOC-STORE"), taskAt(t, released.Tasks(), "SYN-LOC-STORE-2")
	for name, tote := range map[string]string{"没登记的箱": "SYN-TOTE-NOT-THERE", "已停用的箱": "SYN-TOTE-03"} {
		want := application.WaveToteNotFound
		if tote == "SYN-TOTE-03" {
			want = application.WaveToteNotAccepting
		}
		if got := world.pickInto(t, handler, near.ID, 6, tote).Outcome(); got != want {
			t.Fatalf("%s = %s", name, got)
		}
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 || len(world.totesOf(t, first)) != 0 {
		t.Fatalf("不认的箱之后待出库 %d，绑定 %+v", world.stockUnitAt(t, "SYN-LOC-OUT", "件"), world.totesOf(t, first))
	}
	if got := world.pickInto(t, handler, near.ID, 6, "SYN-TOTE-01").Outcome(); got != application.WavePicked {
		t.Fatalf("拣进一号箱 = %s", got)
	}
	if got := world.pickInto(t, handler, far.ID, 4, "SYN-TOTE-02").Outcome(); got != application.WavePicked {
		t.Fatalf("同一张请求换一个箱 = %s", got)
	}
	bound := world.totesOf(t, first)
	if len(bound) != 2 || bound[0].ToteCode != "SYN-TOTE-01" || bound[0].ReleasedAt != nil || bound[1].ToteCode != "SYN-TOTE-02" {
		t.Fatalf("绑着的箱 %+v", bound)
	}
	if rows := world.totes(t, "state=IN_USE"); len(rows) != 2 || rows["SYN-TOTE-01"].RequestID != first.String() {
		t.Fatalf("名册上在用的 %+v", rows)
	}
	if got := world.retireTote(t, handler, "SYN-TOTE-01").Outcome(); got != application.WaveToteInUse {
		t.Fatalf("绑着请求的箱停用 = %s", got)
	}
	if world.checkOutbound(t, handler, first, "SYN-CHECK-TOTE", 10).Outcome() != application.WaveChecked ||
		world.packOutbound(t, handler, first, "SYN-PACK-TOTE", 6, 4).Outcome() != application.WavePacked {
		t.Fatal("复核打包没有成")
	}
	bound = world.totesOf(t, first)
	if len(bound) != 2 || bound[0].ReleasedAt == nil || bound[1].ReleasedAt == nil || len(world.totes(t, "state=IN_USE")) != 0 {
		t.Fatalf("打完包之后 %+v", bound)
	}
}

// Covers: 一个箱同一时刻只装一张出库请求：绑着一张请求的箱，别的请求拣不进去；由人放开之后才行，放开记下是谁放的。
// 没绑着的箱再放答已有结果，没登记的箱答查无此箱。截单的请求等货都回了库才放开箱。
func TestAToteHoldsOneRequestAtATime(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 20)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01").Outcome() != application.WaveTotesRegistered {
		t.Fatal("箱没有登记成")
	}
	first, second := reserve("REQ-TOTE-ONE"), reserve("REQ-TOTE-TWO")
	released := world.releaseRequests(t, handler, "WAVE-TOTE-TWO", first, second)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s", released.Outcome())
	}
	var mine, other ports.PickTask
	for _, task := range released.Tasks() {
		if task.Request == first {
			mine = task
		} else {
			other = task
		}
	}
	if got := world.pickInto(t, handler, mine.ID, 10, "SYN-TOTE-01").Outcome(); got != application.WavePicked {
		t.Fatalf("头一张拣进箱 = %s", got)
	}
	if got := world.pickInto(t, handler, other.ID, 10, "SYN-TOTE-01").Outcome(); got != application.WaveToteInUse {
		t.Fatalf("第二张拣进同一个箱 = %s", got)
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 10 || len(world.totesOf(t, second)) != 0 {
		t.Fatalf("拒了之后待出库 %d", world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	if got := world.releaseTote(t, handler, "SYN-TOTE-NOT-THERE").Outcome(); got != application.WaveToteNotFound {
		t.Fatalf("放开没登记的箱 = %s", got)
	}
	if got := world.releaseTote(t, handler, "SYN-TOTE-01").Outcome(); got != application.WaveToteReleased {
		t.Fatalf("由人放开 = %s", got)
	}
	if got := world.releaseTote(t, handler, "SYN-TOTE-01").Outcome(); got != application.WaveExisting {
		t.Fatalf("空着的箱再放 = %s", got)
	}
	var releasedBy *string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT released_by FROM warehouse_fulfillment.tote_binding WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`,
		first.String()).Scan(&releasedBy); err != nil || releasedBy == nil || *releasedBy != "operator-2" {
		t.Fatalf("由人放开记下的人 %v err %v", releasedBy, err)
	}
	if got := world.pickInto(t, handler, other.ID, 10, "SYN-TOTE-01").Outcome(); got != application.WavePicked {
		t.Fatalf("放开之后第二张拣进去 = %s", got)
	}

	// 截单：货已经拣出来，要回库。箱在货都回了库之后才空出来。
	if world.cutoff(t, handler, second, 0, "SYN-CUT-TOTE").Outcome() != application.CutoffReturnRequired {
		t.Fatal("截单没有成立")
	}
	if held := world.totesOf(t, second); len(held) != 1 || held[0].ReleasedAt != nil {
		t.Fatalf("等回库时 %+v", held)
	}
	var returnID string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT return_id FROM warehouse_fulfillment.return_task WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1 AND status = 'OPEN'`,
		second.String()).Scan(&returnID); err != nil {
		t.Fatal(err)
	}
	if world.completeReturn(t, handler, returnID, "SYN-FACT-RETURN-TOTE", "SYN-LOC-STORE").Outcome() != application.WaveRecorded {
		t.Fatal("回库没有成")
	}
	if held := world.totesOf(t, second); len(held) != 1 || held[0].ReleasedAt == nil || world.requestStatus(t, second) != "CANCELLED" {
		t.Fatalf("货都回了库之后 %+v，请求 %s", held, world.requestStatus(t, second))
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
	request := valueOf(t, domain.NewOutboundRequestID, "OBR-1")
	if _, err := waves.BindTote(context.Background(), tenant, warehouse, code, request, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("BindTote：%v", err)
	}
	if err := waves.ReleaseTotes(context.Background(), tenant, request, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ReleaseTotes：%v", err)
	}
	if _, err := waves.ReleaseTote(context.Background(), tenant, warehouse, code, operator, preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ReleaseTote：%v", err)
	}
	err = waves.SaveToteRequirement(context.Background(), tenant, warehouse, ports.ToteRequirement{
		Revision: 1, Required: true, Operator: "operator-2", RegisteredAt: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveToteRequirement：%v", err)
	}
}

func (world receiptWorld) requireTotes(t *testing.T, handler *application.WaveHandler, warehouse string, revision int, required bool) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterToteRequirement(ctx, application.RegisterToteRequirementCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, warehouse),
			Revision: revision, Required: required, Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
		})
		return registerErr
	})
	if err != nil {
		t.Fatalf("登记必须用箱：%v", err)
	}
	return result
}

// Covers: 「这个仓拣货必须用周转箱」是租户按仓登记的。没登记时拣货带不带箱都收；登记成要求之后，不带箱的拣货整份不成立、
// 任务仍待执行，带箱的照常；再登一版说不要求，不带箱的又收。修订从 1 起连续，原样重放答已有结果，跳号是内容冲突；
// 没登记的仓、代理仓不收。设置总览只列登记过的仓。
func TestPickingWithoutAToteIsRefusedOnlyWhereTheTenantRequiresTotes(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01").Outcome() != application.WaveTotesRegistered {
		t.Fatal("箱没有登记成")
	}
	request := reserve("REQ-TOTE-RULE", 10)
	released := world.releaseRequests(t, handler, "WAVE-TOTE-RULE", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s", released.Outcome())
	}
	near, far := taskAt(t, released.Tasks(), "SYN-LOC-STORE"), taskAt(t, released.Tasks(), "SYN-LOC-STORE-2")

	if got := world.requireTotes(t, handler, "SYN-WH-01", 2, true).Outcome(); got != application.WaveConflict {
		t.Fatalf("头一版不是 1 = %s", got)
	}
	if got := world.requireTotes(t, handler, "SYN-WH-MISSING", 1, true).Outcome(); got != application.WaveWarehouseNotFound {
		t.Fatalf("没登记的仓 = %s", got)
	}
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	if got := world.requireTotes(t, handler, "SYN-WH-AGENT", 1, true).Outcome(); got != application.WaveWarehouseNotAccepting {
		t.Fatalf("代理仓 = %s", got)
	}
	if got := world.requireTotes(t, handler, "SYN-WH-01", 1, true).Outcome(); got != application.WaveStrategyRegistered {
		t.Fatalf("登记要求 = %s", got)
	}
	if got := world.requireTotes(t, handler, "SYN-WH-01", 1, true).Outcome(); got != application.WaveExisting {
		t.Fatalf("原样重放 = %s", got)
	}
	if got := world.requireTotes(t, handler, "SYN-WH-01", 1, false).Outcome(); got != application.WaveConflict {
		t.Fatalf("同号换了内容 = %s", got)
	}

	if got := world.pick(t, handler, near.ID, 6).Outcome(); got != application.WaveToteRequired {
		t.Fatalf("要求用箱时不带箱 = %s", got)
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 || world.requestStatus(t, request) != "ALLOCATED" {
		t.Fatalf("不成立之后待出库 %d，请求 %s", world.stockUnitAt(t, "SYN-LOC-OUT", "件"), world.requestStatus(t, request))
	}
	if got := world.pickInto(t, handler, near.ID, 6, "SYN-TOTE-01").Outcome(); got != application.WavePicked {
		t.Fatalf("要求用箱时带箱 = %s", got)
	}

	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := reads.SettingsOverview(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"))
	if err != nil || len(overview.ToteRequirements) != 1 || overview.ToteRequirements[0].WarehouseCode != "SYN-WH-01" ||
		overview.ToteRequirements[0].Revision != 1 || !overview.ToteRequirements[0].Required || overview.ToteRequirements[0].Operator != "operator-2" {
		t.Fatalf("设置总览 %+v err %v", overview.ToteRequirements, err)
	}

	if got := world.requireTotes(t, handler, "SYN-WH-01", 2, false).Outcome(); got != application.WaveStrategyRegistered {
		t.Fatalf("第二版说不要求 = %s", got)
	}
	if got := world.pick(t, handler, far.ID, 4).Outcome(); got != application.WavePicked {
		t.Fatalf("不要求之后不带箱 = %s", got)
	}
}
