package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// twoCellWorld 把同一批货摆在两个库位上：SYN-LOC-STORE 六件、SYN-LOC-STORE-2 四件。分配先挑库位码小的那一格。
func twoCellWorld(t *testing.T) (receiptWorld, *application.WaveHandler, func(string, int64) domain.OutboundRequestID) {
	t.Helper()
	world, handler, _ := withdrawalWorld(t, 10)
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE-2", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	moved := world.registerJob(t, world.internalJobs(t), moveCommand(t, "SYN-FACT-SPLIT-CELL", 4, "SYN-LOC-STORE", "SYN-LOC-STORE-2"))
	if moved.Outcome() != application.RegisterInternalJobRegistered || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 6 || world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 4 {
		t.Fatalf("分成两格 %s：%d / %d", moved.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-STORE-2", "件"))
	}
	outbound := world.outbound(t)
	reserve := func(key string, quantity int64) domain.OutboundRequestID {
		t.Helper()
		accepted := world.acceptOutbound(t, outbound, outboundCommand(t, key, quantity))
		request, ok := accepted.Request()
		if accepted.Outcome() != application.OutboundReserved || !ok {
			t.Fatalf("预占 %s：%s", key, accepted.Outcome())
		}
		return request.ID()
	}
	return world, handler, reserve
}

func taskAt(t *testing.T, tasks []ports.PickTask, location string) ports.PickTask {
	t.Helper()
	for _, task := range tasks {
		if task.Location.String() == location {
			return task
		}
	}
	t.Fatalf("没有落在 %s 的任务：%+v", location, tasks)
	return ports.PickTask{}
}

// Covers: 少拣之后少的数量当场在同一个波次里换别的库位再分配。少拣的那个库位账上还有余量也不再用——
// 拣货的人刚在那里确认过没有。请求不退出波次，仍是已放波次；换出来的格拣完，请求是已拣货，复核对的是两格实拣的合计。
func TestAShortPickIsReallocatedToAnotherLocationInTheSameWave(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	request := reserve("REQ-SHORT-MOVE", 3)
	released := world.releaseRequests(t, handler, "WAVE-SHORT-MOVE", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 || released.Tasks()[0].Location.String() != "SYN-LOC-STORE" {
		t.Fatalf("放出 %s %+v", released.Outcome(), released.Tasks())
	}
	short := world.pick(t, handler, released.Tasks()[0].ID, 1)
	if short.Outcome() != application.WavePicked || len(short.Tasks()) != 2 {
		t.Fatalf("少拣 %s，交回 %d 条任务", short.Outcome(), len(short.Tasks()))
	}
	// 少拣的库位账上还剩 5 件、没有别的任务占着；不避开它的话，差的 2 件会原地再分一遍。
	replacement := taskAt(t, short.Tasks(), "SYN-LOC-STORE-2")
	if replacement.Quantity != 2 || replacement.Status != "OPEN" || replacement.WaveID.String() != "WAVE-SHORT-MOVE" {
		t.Fatalf("换出来的任务 %+v", replacement)
	}
	row := world.waveRow(t, "WAVE-SHORT-MOVE")
	if world.requestStatus(t, request) != "ALLOCATED" || row.Status != ports.WavePicking || len(row.Tasks) != 2 || reservedQuantity(t, world) != 2 {
		t.Fatalf("少拣之后请求 %s，波次 %s，任务 %d 条，预占 %d", world.requestStatus(t, request), row.Status, len(row.Tasks), reservedQuantity(t, world))
	}
	if world.pick(t, handler, replacement.ID, 2).Outcome() != application.WavePicked {
		t.Fatal("换出来的那一格没有拣成")
	}
	if world.requestStatus(t, request) != "PICKED" || world.waveRow(t, "WAVE-SHORT-MOVE").Status != ports.WaveCompleted || reservedQuantity(t, world) != 0 {
		t.Fatalf("补拣之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 3 || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 5 || world.stockUnitAt(t, "SYN-LOC-STORE-2", "件") != 2 {
		t.Fatalf("待出库 %d，少拣那个库位账上 %d，另一个库位 %d", world.stockUnitAt(t, "SYN-LOC-OUT", "件"), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.stockUnitAt(t, "SYN-LOC-STORE-2", "件"))
	}
	if got := world.checkOutbound(t, handler, request, "SYN-CHECK-SHORT-MOVE", 3).Outcome(); got != application.WaveChecked {
		t.Fatalf("复核两格合计 3 件 = %s", got)
	}
}

// Covers: 一张请求有两条任务，头一条少拣、换不出别的格时，它仍是已放波次，不能被再放一次；
// 第二条拣完，任务都有了结果，少的那几件还在预占里，请求才回到已预占，不当成已拣货放去复核。
func TestAShortPickKeepsTheRequestInTheWaveUntilItsOtherTasksSettle(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	request := reserve("REQ-SHORT-STAY", 10)
	released := world.releaseRequests(t, handler, "WAVE-SHORT-STAY", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %d 条", released.Outcome(), len(released.Tasks()))
	}
	first, second := taskAt(t, released.Tasks(), "SYN-LOC-STORE"), taskAt(t, released.Tasks(), "SYN-LOC-STORE-2")
	short := world.pick(t, handler, first.ID, 2)
	if short.Outcome() != application.WavePicked || len(short.Tasks()) != 1 {
		t.Fatalf("少拣 %s，交回 %d 条任务（另一格的货都分给了第二条任务，换不出）", short.Outcome(), len(short.Tasks()))
	}
	if world.requestStatus(t, request) != "ALLOCATED" || world.waveRow(t, "WAVE-SHORT-STAY").Status != ports.WavePicking {
		t.Fatalf("还有一条任务等着拣时请求 %s", world.requestStatus(t, request))
	}
	if got := world.releaseRequests(t, handler, "WAVE-SHORT-AGAIN", request).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("带着没拣的任务再放一次 = %s", got)
	}
	if world.pick(t, handler, second.ID, 4).Outcome() != application.WavePicked {
		t.Fatal("第二条没有拣成")
	}
	if world.requestStatus(t, request) != "RESERVED" || world.waveRow(t, "WAVE-SHORT-STAY").Status != ports.WaveCompleted || reservedQuantity(t, world) != 4 {
		t.Fatalf("任务都有了结果之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
}

// Covers: 同一张请求的两条任务被两个人同时拣完，请求是已拣货、预占扣到零。
// 第一笔拣完先不提交，第二笔随后拣另一条、卡在这位货主的入账次序上，确认它真的卡住了才放第一笔提交——
// 第二笔判断用的预占与任务是排到之后读的，不是排队之前读的那份旧的。
func TestTwoConcurrentPicksOfOneRequestLeaveItPicked(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	request := reserve("REQ-RACE-ONE", 10)
	released := world.releaseRequests(t, handler, "WAVE-RACE-ONE", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %d 条", released.Outcome(), len(released.Tasks()))
	}
	observer := newBlockedSessionObserver(t, world.pool)
	pick := func(ctx context.Context, task ports.PickTask) (application.WaveOutcome, error) {
		result, err := handler.Pick(ctx, application.PickCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, "SYN-FACT-"+task.ID.String()),
			TaskID: task.ID, Quantity: task.Quantity, Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT"),
		})
		return result.Outcome(), err
	}
	ctx := t.Context()
	picked := make(chan struct{})
	release := make(chan struct{})
	var firstOutcome, secondOutcome application.WaveOutcome
	var firstErr, secondErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		firstErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var pickErr error
			firstOutcome, pickErr = pick(txCtx, released.Tasks()[0])
			close(picked)
			if pickErr == nil {
				<-release
			}
			return pickErr
		})
	}()
	<-picked
	wait.Add(1)
	go func() {
		defer wait.Done()
		secondErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var pickErr error
			secondOutcome, pickErr = pick(txCtx, released.Tasks()[1])
			return pickErr
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if firstErr != nil || secondErr != nil || firstOutcome != application.WavePicked || secondOutcome != application.WavePicked {
		t.Fatalf("两笔拣货：%s %v / %s %v", firstOutcome, firstErr, secondOutcome, secondErr)
	}
	if !blocked {
		t.Error("第二笔没有卡在入账次序上，没有走到「排队」那条路")
	}
	if world.requestStatus(t, request) != "PICKED" || reservedQuantity(t, world) != 0 {
		t.Fatalf("两条任务都拣完之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
}

// Covers: 拣哪种货扣哪种货的预占。一张请求有两行预占时，拣第二行那种货，扣的是第二行，头一行原样留着。
func TestAPickReducesTheReservationOfTheGoodsItPicked(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	request := reserve("REQ-REDUCE", 6)
	released := world.releaseRequests(t, handler, "WAVE-REDUCE", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 {
		t.Fatalf("放出 %s", released.Outcome())
	}
	// 第二种货不必真的有库存：这里只看预占扣在哪一行。预占行与它的拣货任务直接摆进去。
	if _, err := world.pool.Exec(t.Context(),
		`INSERT INTO warehouse_fulfillment.outbound_reservation
		    (tenant_id, request_id, line_index, goods_code, stock_state, quantity, from_unlabeled, wanted)
		 VALUES ('SYN-TENANT-01', $1, 1, 'SYN-GOODS-OTHER', 'GOOD', 5, 0, 5)`, request.String()); err != nil {
		t.Fatalf("摆第二行预占：%v", err)
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	other := released.Tasks()[0]
	other.ID = valueOf(t, domain.NewPickTaskID, "PKT-REDUCE-OTHER")
	other.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-OTHER")
	other.Quantity = 5
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if err := waves.InsertOpenTask(ctx, tenant, other); err != nil {
			return err
		}
		other.Picked = 3
		other.Status = "SHORT"
		other.Staging = valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT")
		return waves.SavePick(ctx, tenant, other, domain.OutboundAllocated, "SDC-REDUCE-OTHER", "SYN-FACT-REDUCE-OTHER", 2, preAdviceNumberingInstant)
	}); err != nil {
		t.Fatalf("拣第二种货：%v", err)
	}
	lines := map[int]int64{}
	rows, err := world.pool.Query(t.Context(),
		`SELECT line_index, quantity FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`, request.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var index int
		var quantity int64
		if err := rows.Scan(&index, &quantity); err != nil {
			t.Fatal(err)
		}
		lines[index] = quantity
	}
	if lines[0] != 6 || lines[1] != 2 {
		t.Fatalf("预占各行 %v：头一行应仍是 6，第二行应剩 2", lines)
	}
}

// liveCounting 照生产装配的口径建盘点与盘点差额批准两个编排：占用按账上的预占、冻结、盘点锁与待执行任务算，
// 并接上拣货差异。用不看占用的编排去测，「预占还指着这批货时盘点开不开得了」这一步等于没测。
func (world receiptWorld) liveCounting(t *testing.T) (*application.CountHandler, *application.CountApprovalHandler, *adapter.Waves) {
	t.Helper()
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := adapter.NewCounts(world.db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewCountIdentities()
	if err != nil {
		t.Fatal(err)
	}
	differences, err := whidentity.NewStockDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	holds := world.ledgerHolds(t)
	counter := application.NewCountHandler(store, identities, differences, world.warehouses, world.locations, world.zones, world.goods, holds, world.ledger, world.clock, world.warehouses).WithPickDifferences(waves)
	approver := application.NewCountApprovalHandler(store, store, world.poster(), holds, world.clock, world.warehouses).WithPickDifferences(waves)
	return counter, approver, waves
}

func pickDifferencesOf(t *testing.T, waves *adapter.Waves, raw string) []ports.PickDifferenceRow {
	t.Helper()
	query, err := ports.PickDifferencePages.DecodeRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	page, err := waves.ListPickDifferences(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), 20, query)
	if err != nil {
		t.Fatalf("拣货差异列表：%v", err)
	}
	return page.Rows
}

// shortSpotWorld 摆出少拣之后的局面：头一张请求在 SYN-LOC-STORE 少拣了 2 件、换到另一个库位补；
// 第二张请求 4 件已预占，别处只剩 2 件没被占着，放波次只能靠少拣的那个库位。
func shortSpotWorld(t *testing.T) (receiptWorld, *application.WaveHandler, domain.OutboundRequestID, domain.OutboundRequestID) {
	t.Helper()
	world, handler, reserve := twoCellWorld(t)
	first := reserve("REQ-SPOT-A", 3)
	released := world.releaseRequests(t, handler, "WAVE-SPOT-A", first)
	if released.Outcome() != application.WaveAllocated || world.pick(t, handler, released.Tasks()[0].ID, 1).Outcome() != application.WavePicked {
		t.Fatal("头一张没有少拣成")
	}
	second := reserve("REQ-SPOT-B", 4)
	if got := world.releaseRequests(t, handler, "WAVE-SPOT-B", second).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("差异没了结时放第二张 = %s（少拣的库位不该再分）", got)
	}
	return world, handler, first, second
}

// Covers: 少拣留下的差异还没了结时，那个库位上的这种货不再分给拣货任务——别的请求放波次分不到它。
// 预占还指着这批货，盘点照样开得了：挂着拣货差异的格不算进「挤占预占」。盘出来账实相符，差异随盘点完成了结并
// 记下是哪一份盘点，库位照常再分配。范围盖不到的（别的货主、别的商品、别的仓、盘点开始之后才记下的）不了结。
func TestAShortLocationIsNotAllocatedUntilACountSettlesIt(t *testing.T) {
	world, handler, first, second := shortSpotWorld(t)
	counter, _, waves := world.liveCounting(t)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	pending := pickDifferencesOf(t, waves, "status=PENDING&warehouseCode=SYN-WH-01")
	if len(pending) != 1 || pending[0].LocationCode != "SYN-LOC-STORE" || pending[0].Quantity != 2 || pending[0].TaskQuantity != 3 || pending[0].Picked != 1 ||
		pending[0].WaveID != "WAVE-SPOT-A" || pending[0].RequestID != first.String() || pending[0].RecordedAt == nil || pending[0].ClosedAt != nil {
		t.Fatalf("待处理的差异 %+v", pending)
	}

	store := valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE")
	inScope := ports.PickDifferenceClosure{
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Locations: []domain.LocationCode{store},
		Before: *pending[0].RecordedAt, Count: "CNT-OUT-OF-SCOPE", At: world.clock.Now(),
	}
	outOfScope := map[string]func(*ports.PickDifferenceClosure){
		"别的仓": func(closure *ports.PickDifferenceClosure) {
			closure.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-99")
		},
		"别的库位": func(closure *ports.PickDifferenceClosure) {
			closure.Locations = []domain.LocationCode{valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE-2")}
		},
		"别的货主": func(closure *ports.PickDifferenceClosure) {
			closure.Owner = valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-99")
		},
		"别的商品": func(closure *ports.PickDifferenceClosure) {
			closure.Goods = []domain.GoodsCode{valueOf(t, domain.NewGoodsCode, "SYN-GOODS-99")}
		},
		"盘点开始之后才记下的": func(closure *ports.PickDifferenceClosure) { closure.Before = pending[0].RecordedAt.Add(-time.Minute) },
		"盘出差额、点名留着的": func(closure *ports.PickDifferenceClosure) {
			closure.Keep = []ports.PickDifferenceSpot{{
				Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), Location: store, Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
			}}
		},
	}
	for name, narrow := range outOfScope {
		closure := inScope
		narrow(&closure)
		if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			return waves.ClosePickDifferences(ctx, tenant, closure)
		}); err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if left := pickDifferencesOf(t, waves, "status=PENDING"); len(left) != 1 {
			t.Fatalf("%s：范围盖不到的差异被了结了", name)
		}
	}

	created := world.createCount(t, counter, "SYN-COUNT-SPOT")
	count, ok := created.Count()
	if created.Outcome() != application.CountCreated || !ok {
		t.Fatalf("建盘点 %s", created.Outcome())
	}
	// 这一刻这种货可分配 9 件、预占 6 件、要锁的是少拣库位的 5 件：不把挂着拣货差异的格放过去，盘点开始不了。
	if got := world.startCount(t, counter, count.ID()).Outcome(); got != application.CountStarted {
		t.Fatalf("预占还指着这批货时开始盘点 = %s", got)
	}
	if got := world.releaseRequests(t, handler, "WAVE-SPOT-B", second).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("盘点还没盘完时放第二张 = %s", got)
	}
	if world.recordCount(t, counter, count.ID(), "SYN-FACT-COUNT-SPOT", 5).Outcome() != application.CountRecorded {
		t.Fatal("清点没有记下")
	}
	if world.completeCount(t, counter, count.ID()).Outcome() != application.CountCounted {
		t.Fatal("盘点没有完成")
	}
	if left := pickDifferencesOf(t, waves, "status=PENDING"); len(left) != 0 {
		t.Fatalf("盘完之后还待处理的差异 %+v", left)
	}
	closed := pickDifferencesOf(t, waves, "status=CLOSED")
	if len(closed) != 1 || closed[0].ClosedByCount != count.ID().String() || closed[0].ClosedAt == nil {
		t.Fatalf("了结的差异 %+v", closed)
	}
	again := world.releaseRequests(t, handler, "WAVE-SPOT-B", second)
	if again.Outcome() != application.WaveAllocated || taskAt(t, again.Tasks(), "SYN-LOC-STORE").Quantity == 0 {
		t.Fatalf("盘完之后放第二张 %s %+v", again.Outcome(), again.Tasks())
	}
}

// Covers: 盘出差额的库位，盘点完成时拣货差异先不了结，库位仍不分配——账还没调，放开只会再派人去拿账上多出来的货。
// 那份差额批准入账之后，拣货差异了结，库位按调过的账再分配。
func TestAShortLocationStaysBlockedUntilTheCountGapIsDecided(t *testing.T) {
	world, handler, _, second := shortSpotWorld(t)
	counter, approver, waves := world.liveCounting(t)
	created := world.createCount(t, counter, "SYN-COUNT-GAP")
	count, ok := created.Count()
	if created.Outcome() != application.CountCreated || !ok {
		t.Fatalf("建盘点 %s", created.Outcome())
	}
	if got := world.startCount(t, counter, count.ID()).Outcome(); got != application.CountStarted {
		t.Fatalf("开始盘点 = %s", got)
	}
	// 账上 5 件，实盘 3 件。
	if world.recordCount(t, counter, count.ID(), "SYN-FACT-COUNT-GAP", 3).Outcome() != application.CountRecorded {
		t.Fatal("清点没有记下")
	}
	if world.completeCount(t, counter, count.ID()).Outcome() != application.CountCounted {
		t.Fatal("盘点没有完成")
	}
	if left := pickDifferencesOf(t, waves, "status=PENDING"); len(left) != 1 {
		t.Fatalf("盘出差额、还没批准时待处理的差异 %+v", left)
	}
	if got := world.releaseRequests(t, handler, "WAVE-SPOT-B", second).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("差额还没批准时放第二张 = %s", got)
	}
	if world.registerDuty(t, approver, false).Outcome() != application.ApprovalDutyRegistered {
		t.Fatal("批准职责没有登记")
	}
	if got := world.approveDifference(t, approver, world.onlyDifference(t, count.ID())).Outcome(); got != application.ApprovalPosted {
		t.Fatalf("批准盘亏 = %s", got)
	}
	closed := pickDifferencesOf(t, waves, "status=CLOSED")
	if world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 3 || len(closed) != 1 || closed[0].ClosedByCount != count.ID().String() {
		t.Fatalf("批准之后库位账上 %d，了结的差异 %+v", world.stockUnitAt(t, "SYN-LOC-STORE", "件"), closed)
	}
	again := world.releaseRequests(t, handler, "WAVE-SPOT-B", second)
	if again.Outcome() != application.WaveAllocated || taskAt(t, again.Tasks(), "SYN-LOC-STORE").Quantity != 3 {
		t.Fatalf("调账之后放第二张 %s %+v", again.Outcome(), again.Tasks())
	}
}

func TestClosingPickDifferencesRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = waves.ClosePickDifferences(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.PickDifferenceClosure{
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Locations: []domain.LocationCode{valueOf(t, domain.NewLocationCode, "SYN-LOC-STORE")},
		Before: preAdviceNumberingInstant, Count: "CNT-1", At: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ClosePickDifferences：%v", err)
	}
}
