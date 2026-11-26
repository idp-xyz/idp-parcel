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

func (world receiptWorld) releaseBatch(t *testing.T, handler *application.WaveHandler, id string, requests ...domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		result, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: requests, BatchPick: true,
		})
		return releaseErr
	})
	if err != nil {
		t.Fatalf("放合并拣货的波次：%v", err)
	}
	return result
}

func (world receiptWorld) sortInto(t *testing.T, handler *application.WaveHandler, fact, wave string, request domain.OutboundRequestID, quantity int64, tote string) application.WaveOutcome {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var sortErr error
		result, sortErr = handler.Sort(ctx, application.SortCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact),
			Wave: valueOf(t, domain.NewWaveID, wave), Request: request, Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
			Quantity: quantity, Tote: valueOf(t, domain.NewToteCode, tote), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		})
		return sortErr
	})
	if err != nil {
		t.Fatalf("分拣 %s：%v", fact, err)
	}
	return result.Outcome()
}

func (world receiptWorld) sortPlan(t *testing.T, wave string) ports.SortPlan {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	plan, found, err := waves.LoadSortPlan(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), wave)
	if err != nil || !found {
		t.Fatalf("读分拣情况 %s：found %v err %v", wave, found, err)
	}
	return plan
}

func planRequest(t *testing.T, plan ports.SortPlan, request domain.OutboundRequestID) ports.SortPlanRequest {
	t.Helper()
	for _, item := range plan.Requests {
		if item.Request == request.String() {
			return item
		}
	}
	t.Fatalf("分拣情况里没有 %s：%+v", request, plan.Requests)
	return ports.SortPlanRequest{}
}

// Covers: 合并拣货的波次拣出来的货，按出库请求一件一件投进它的周转箱；投放不能超过已经拣出来而还没投放的件数，
// 箱照绑箱的规矩绑到请求上（没登记、已停用、绑着别的请求的不收），分完之前不能复核、分完才能。投放不动库存。
// 同一个事实号同样内容重放答已有结果，内容不同答冲突。复核过的请求不再收投放；打完包箱放开，别的请求可以用。
func TestABatchPickedRequestIsSortedIntoItsToteBeforeItCanBeChecked(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01", "SYN-TOTE-02", "SYN-TOTE-03").Outcome() != application.WaveTotesRegistered ||
		world.retireTote(t, handler, "SYN-TOTE-03").Outcome() != application.WaveToteRetired {
		t.Fatal("箱没有备好")
	}
	first, second := reserve("REQ-SORT-A", 2), reserve("REQ-SORT-B", 3)
	released := world.releaseBatch(t, handler, "WAVE-SORT", first, second)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %+v", released.Outcome(), released.Tasks())
	}
	if waves := world.listedWaves(t, "batchPick=true"); len(waves) != 1 || !waves["WAVE-SORT"].BatchPick {
		t.Fatalf("按合并拣货筛波次 %+v", waves)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-EARLY", "WAVE-SORT", first, 1, "SYN-TOTE-01"); got != application.WaveContentInvalid {
		t.Fatalf("还没拣出来就投放 = %s", got)
	}
	if rows := world.totes(t, "state=IN_USE"); len(rows) != 0 {
		t.Fatalf("没成的投放绑上了箱 %+v", rows)
	}
	for _, task := range released.Tasks() {
		if world.pick(t, handler, task.ID, task.Quantity).Outcome() != application.WavePicked {
			t.Fatalf("任务 %s 没有拣成", task.ID)
		}
	}
	staged := world.stockUnitAt(t, "SYN-LOC-OUT", "件")
	if got := world.checkOutbound(t, handler, first, "SYN-CHECK-SORT", 2).Outcome(); got != application.WaveSortPending {
		t.Fatalf("没分拣就复核 = %s", got)
	}
	if world.requestStatus(t, first) != "PICKED" {
		t.Fatalf("没分拣就复核之后请求 %s", world.requestStatus(t, first))
	}

	if got := world.sortInto(t, handler, "SYN-SORT-A1", "WAVE-SORT", first, 1, "SYN-TOTE-01"); got != application.WaveSorted {
		t.Fatalf("头一件投进一号箱 = %s", got)
	}
	mine := planRequest(t, world.sortPlan(t, "WAVE-SORT"), first)
	if len(mine.Totes) != 1 || mine.Totes[0] != "SYN-TOTE-01" || len(mine.Lines) != 1 ||
		mine.Lines[0].Goods != "SYN-GOODS-01" || mine.Lines[0].Picked != 2 || mine.Lines[0].Sorted != 1 || mine.Lines[0].Open != 0 {
		t.Fatalf("投了一件之后的分拣情况 %+v", mine)
	}
	for name, attempt := range map[string]struct {
		request domain.OutboundRequestID
		tote    string
		want    application.WaveOutcome
	}{
		"箱绑着别的请求": {second, "SYN-TOTE-01", application.WaveToteInUse},
		"没登记的箱":   {first, "SYN-TOTE-NOT-THERE", application.WaveToteNotFound},
		"已停用的箱":   {first, "SYN-TOTE-03", application.WaveToteNotAccepting},
	} {
		if got := world.sortInto(t, handler, "SYN-SORT-REFUSED-"+attempt.tote+attempt.request.String(), "WAVE-SORT", attempt.request, 1, attempt.tote); got != attempt.want {
			t.Fatalf("%s = %s", name, got)
		}
	}
	if got := world.sortInto(t, handler, "SYN-SORT-TOO-MANY", "WAVE-SORT", first, 2, "SYN-TOTE-01"); got != application.WaveContentInvalid {
		t.Fatalf("投得比拣出来的多 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-A1", "WAVE-SORT", first, 1, "SYN-TOTE-01"); got != application.WaveExisting {
		t.Fatalf("同一个事实号重放 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-A1", "WAVE-SORT", first, 1, "SYN-TOTE-02"); got != application.WaveConflict {
		t.Fatalf("同一个事实号换了箱 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-A2", "WAVE-SORT", first, 1, "SYN-TOTE-02"); got != application.WaveSorted {
		t.Fatalf("同一张请求换一个箱投第二件 = %s", got)
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != staged {
		t.Fatalf("投放动了库存：待出库 %d，投放前 %d", world.stockUnitAt(t, "SYN-LOC-OUT", "件"), staged)
	}
	if got := world.checkOutbound(t, handler, first, "SYN-CHECK-SORT", 2).Outcome(); got != application.WaveChecked {
		t.Fatalf("分完之后复核 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-AFTER-CHECK", "WAVE-SORT", first, 1, "SYN-TOTE-01"); got != application.WaveStatusMismatch {
		t.Fatalf("复核过的请求再投放 = %s", got)
	}
	if world.packOutbound(t, handler, first, "SYN-PACK-SORT", 1, 1).Outcome() != application.WavePacked {
		t.Fatal("打包没有成")
	}
	plan := world.sortPlan(t, "WAVE-SORT")
	if !plan.Sorts || len(plan.Requests) != 1 || plan.Requests[0].Request != second.String() || len(plan.Requests[0].Totes) != 0 {
		t.Fatalf("打完包之后的分拣情况 %+v", plan)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-B", "WAVE-SORT", second, 3, "SYN-TOTE-01"); got != application.WaveSorted {
		t.Fatalf("放开的箱给第二张请求 = %s", got)
	}
	if got := world.checkOutbound(t, handler, second, "SYN-CHECK-SORT-B", 3).Outcome(); got != application.WaveChecked {
		t.Fatalf("第二张分完之后复核 = %s", got)
	}
}

// Covers: 逐单拣的波次不分拣：投放不收，复核照旧不等分拣，读口答不用分拣。没有这个波次、没有这张请求答查无。
// 放出来是单品单件波次的，选了合并拣货也不记成合并拣货——它到复核打包扫货配单，不用分拣。
func TestOnlyBatchPickWavesAreSorted(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if world.registerTotes(t, handler, "SYN-WH-01", "SYN-TOTE-01").Outcome() != application.WaveTotesRegistered {
		t.Fatal("箱没有登记成")
	}
	discrete, single := reserve("REQ-SORT-DISCRETE", 2), reserve("REQ-SORT-SINGLE", 1)
	plain := world.releaseRequests(t, handler, "WAVE-NO-SORT", discrete)
	lone := world.releaseBatch(t, handler, "WAVE-SINGLE-BATCH", single)
	if plain.Outcome() != application.WaveAllocated || lone.Outcome() != application.WaveAllocated {
		t.Fatalf("放出 %s / %s", plain.Outcome(), lone.Outcome())
	}
	if row := world.waveRow(t, "WAVE-SINGLE-BATCH"); !row.SingleUnit || row.BatchPick {
		t.Fatalf("单品单件波次选了合并拣货：%+v", row)
	}
	for _, task := range append(plain.Tasks(), lone.Tasks()...) {
		if world.pick(t, handler, task.ID, task.Quantity).Outcome() != application.WavePicked {
			t.Fatalf("任务 %s 没有拣成", task.ID)
		}
	}
	if got := world.sortInto(t, handler, "SYN-SORT-PLAIN", "WAVE-NO-SORT", discrete, 1, "SYN-TOTE-01"); got != application.WaveContentInvalid {
		t.Fatalf("逐单拣的波次投放 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-NO-WAVE", "WAVE-NOT-THERE", discrete, 1, "SYN-TOTE-01"); got != application.WaveNotFound {
		t.Fatalf("没有这个波次 = %s", got)
	}
	if got := world.sortInto(t, handler, "SYN-SORT-NO-REQUEST", "WAVE-NO-SORT", valueOf(t, domain.NewOutboundRequestID, "OBR-NOT-THERE"), 1, "SYN-TOTE-01"); got != application.WaveNotFound {
		t.Fatalf("没有这张请求 = %s", got)
	}
	if plan := world.sortPlan(t, "WAVE-NO-SORT"); plan.Sorts || len(plan.Requests) != 0 {
		t.Fatalf("逐单拣的波次的分拣情况 %+v", plan)
	}
	if waves := world.listedWaves(t, "batchPick=true"); len(waves) != 0 {
		t.Fatalf("没有合并拣货的波次，按它筛出了 %+v", waves)
	}
	if waves := world.listedWaves(t, "batchPick=false"); len(waves) != 2 {
		t.Fatalf("筛不是合并拣货的波次 %+v", waves)
	}
	if world.checkOutbound(t, handler, discrete, "SYN-CHECK-PLAIN", 2).Outcome() != application.WaveChecked ||
		world.checkOutbound(t, handler, single, "SYN-CHECK-SINGLE-BATCH", 1).Outcome() != application.WaveChecked {
		t.Fatal("不用分拣的两张请求复核没有成")
	}
}

func TestWritingASortPutRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = waves.SaveSortPut(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.SortPut{
		FactID: "SYN-SORT-01", Wave: valueOf(t, domain.NewWaveID, "WAVE-01"), Request: valueOf(t, domain.NewOutboundRequestID, "OBR-1"),
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Quantity: 1, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Tote: valueOf(t, domain.NewToteCode, "SYN-TOTE-01"), RecordedAt: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveSortPut：%v", err)
	}
}
