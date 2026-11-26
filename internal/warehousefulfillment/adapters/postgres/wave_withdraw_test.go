package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (world receiptWorld) withdraw(t *testing.T, handler *application.WaveHandler, id string, requests ...domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var withdrawErr error
		result, withdrawErr = handler.Withdraw(ctx, application.WithdrawWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
			Operator: valueOf(t, domain.NewRegistrant, "operator-2"), Requests: requests,
		})
		return withdrawErr
	})
	if err != nil {
		t.Fatalf("撤单 %s：%v", id, err)
	}
	return result
}

func TestWithdrawingFromAWaveRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = waves.WithdrawFromWave(context.Background(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWaveID, "WAVE"),
		valueOf(t, domain.NewOutboundRequestID, "OBR-1"), valueOf(t, domain.NewRegistrant, "operator-2"), preAdviceNumberingInstant)
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("WithdrawFromWave：%v", err)
	}
}

// Covers: 同一个波次里两张请求的任务被两笔事务同时拣完，波次存下的状态是已完成。
// 第一笔拣完先不提交，第二笔随后拣另一张请求的任务、卡在波次的行锁上，确认它真的卡住了才放第一笔提交——
// 走到的是「等锁、锁放开后看得到前一笔提交的任务再重算」那条路，不是两笔碰巧一前一后。
func TestTwoConcurrentPicksInOneWaveLeaveItCompleted(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 20)
	released := world.releaseRequests(t, handler, "WAVE-RACE", reserve("REQ-RACE-A"), reserve("REQ-RACE-B"))
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放两张 %s", released.Outcome())
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	observer := newBlockedSessionObserver(t, world.pool)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	pick := func(ctx context.Context, index int) error {
		task := released.Tasks()[index]
		task.Picked = task.Quantity
		task.Status = "DONE"
		task.Staging = valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT")
		return waves.SavePick(ctx, tenant, task, domain.OutboundPicked, "", "SYN-FACT-RACE-"+task.ID.String(), 0, preAdviceNumberingInstant)
	}
	ctx := t.Context()
	picked := make(chan struct{})
	release := make(chan struct{})
	var firstErr, secondErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		firstErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			pickErr := pick(txCtx, 0)
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
		secondErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error { return pick(txCtx, 1) })
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("两笔拣货：%v / %v", firstErr, secondErr)
	}
	if !blocked {
		t.Error("第二笔没有卡在波次的行锁上，没有走到「等锁」那条路")
	}
	if got := world.waveRow(t, "WAVE-RACE").Status; got != ports.WaveCompleted {
		t.Fatalf("两条任务都拣完之后波次存的是 %s", got)
	}
}

func (world receiptWorld) requestStatus(t *testing.T, request domain.OutboundRequestID) string {
	t.Helper()
	var status string
	if err := world.pool.QueryRow(t.Context(),
		`SELECT status FROM warehouse_fulfillment.outbound_request WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`,
		request.String()).Scan(&status); err != nil {
		t.Fatalf("读请求状态：%v", err)
	}
	return status
}

func (world receiptWorld) waveRow(t *testing.T, id string) ports.WaveRow {
	t.Helper()
	store, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	row, found, err := store.LoadWaveRow(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id)
	if err != nil || !found {
		t.Fatalf("读波次 %s：found %v err %v", id, found, err)
	}
	return row
}

func withdrawalWorld(t *testing.T, stock int64) (receiptWorld, *application.WaveHandler, func(string) domain.OutboundRequestID) {
	t.Helper()
	world := newReceiptWorld(t)
	world.seedStorageStock(t, stock)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-OUT", 1, func(spec *domain.ZoneContentSpec) {
		spec.Usage = domain.ZoneUsageOutboundStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-OUT", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-OUT")
	}))
	outbound := world.outbound(t)
	handler := world.waves(t)
	if world.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	reserve := func(key string) domain.OutboundRequestID {
		t.Helper()
		accepted := world.acceptOutbound(t, outbound, outboundCommand(t, key, 10))
		request, ok := accepted.Request()
		if accepted.Outcome() != application.OutboundReserved || !ok {
			t.Fatalf("预占 %s：%s", key, accepted.Outcome())
		}
		return request.ID()
	}
	return world, handler, reserve
}

// Covers: 放出后撤单。撤出的请求回到已预占、预占不动，它在这个波次里待执行的任务作废，别的请求照旧；成员行留着并标成已撤出。
// 同一份撤单重放答已有结果。撤出的请求可以再放进别的波次。波次里的请求全撤出之后波次是已取消。
// 放波次按原来的单重放仍答已有结果。
func TestWithdrawingARequestFromAWaveReturnsItToReserved(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 30)
	first, second := reserve("REQ-WD-A"), reserve("REQ-WD-B")
	if released := world.releaseRequests(t, handler, "WAVE-WD", first, second); released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放两张 %s", released.Outcome())
	}
	if got := world.withdraw(t, handler, "WAVE-WD-NONE", first).Outcome(); got != application.WaveNotFound {
		t.Fatalf("没有的波次 = %s", got)
	}
	outsider := reserve("REQ-WD-OUTSIDER")
	reserved := reservedQuantity(t, world)
	if got := world.withdraw(t, handler, "WAVE-WD", first, outsider).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("不在这个波次里的请求 = %s", got)
	}
	if world.requestStatus(t, first) != "ALLOCATED" {
		t.Fatal("点名里有一张不成立，别的也不该撤")
	}

	if got := world.withdraw(t, handler, "WAVE-WD", first).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("撤出 = %s", got)
	}
	if world.requestStatus(t, first) != "RESERVED" || world.requestStatus(t, second) != "ALLOCATED" || reservedQuantity(t, world) != reserved {
		t.Fatalf("撤出之后 %s / %s 预占 %d（原 %d）", world.requestStatus(t, first), world.requestStatus(t, second), reservedQuantity(t, world), reserved)
	}
	row := world.waveRow(t, "WAVE-WD")
	if row.Status != ports.WaveReleased || len(row.Requests) != 2 || len(row.Withdrawn) != 1 || row.Withdrawn[0] != first.String() {
		t.Fatalf("撤出一张之后的波次 %+v", row)
	}
	for _, task := range row.Tasks {
		want := "OPEN"
		if task.RequestID == first.String() {
			want = "CANCELLED"
		}
		if task.Status != want {
			t.Fatalf("任务 %s 是 %s，应为 %s", task.TaskID, task.Status, want)
		}
	}
	if got := world.withdraw(t, handler, "WAVE-WD", first).Outcome(); got != application.WaveExisting {
		t.Fatalf("重放撤单 = %s", got)
	}
	if got := world.releaseRequests(t, handler, "WAVE-WD", first, second).Outcome(); got != application.WaveExisting {
		t.Fatalf("按原来的单重放放波次 = %s", got)
	}

	again := world.releaseRequests(t, handler, "WAVE-WD-AGAIN", first)
	if again.Outcome() != application.WaveAllocated || world.requestStatus(t, first) != "ALLOCATED" {
		t.Fatalf("撤出之后再放 %s，请求 %s", again.Outcome(), world.requestStatus(t, first))
	}
	if got := world.withdraw(t, handler, "WAVE-WD-AGAIN", first).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("从第二个波次撤出 = %s", got)
	}
	if got := world.waveRow(t, "WAVE-WD-AGAIN").Status; got != ports.WaveCancelled {
		t.Fatalf("全撤出的波次 = %s", got)
	}
	if world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 30 {
		t.Fatal("撤单不该动库存")
	}
}

// Covers: 拣过的请求不能撤——拣出来的货要回库，那是截单的事。点名的几张里有一张撤不了，就一张都不撤。
// 已经不是已放波次的请求（拣完了、截掉了）同样撤不了。
func TestAPickedRequestCannotBeWithdrawnAndTheWithdrawalIsAllOrNothing(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 30)
	picked, waiting, cut := reserve("REQ-WD-PICKED"), reserve("REQ-WD-WAITING"), reserve("REQ-WD-CUT")
	released := world.releaseRequests(t, handler, "WAVE-WD-MIX", picked, waiting, cut)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 3 {
		t.Fatalf("放三张 %s", released.Outcome())
	}
	var pickedTask domain.PickTaskID
	for _, task := range released.Tasks() {
		if task.Request == picked {
			pickedTask = task.ID
		}
	}
	if world.pick(t, handler, pickedTask, 10).Outcome() != application.WavePicked {
		t.Fatal("没有拣成")
	}
	if got := world.withdraw(t, handler, "WAVE-WD-MIX", waiting, picked).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("带着一张拣过的 = %s", got)
	}
	if world.requestStatus(t, waiting) != "ALLOCATED" || len(world.waveRow(t, "WAVE-WD-MIX").Withdrawn) != 0 {
		t.Fatal("有一张撤不了，另一张也不该撤")
	}
	if world.cutoff(t, handler, cut, 0, "SYN-CUT-WD").Outcome() != application.CutoffCancelled {
		t.Fatal("截单没有成立")
	}
	if got := world.withdraw(t, handler, "WAVE-WD-MIX", cut).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("截掉的请求 = %s", got)
	}
	if got := world.withdraw(t, handler, "WAVE-WD-MIX", waiting).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("只撤还没拣的那张 = %s", got)
	}
	if got := world.waveRow(t, "WAVE-WD-MIX").Status; got != ports.WaveCompleted {
		t.Fatalf("一张拣完、一张撤出、一张截掉 = %s", got)
	}
}

// Covers: 撤出的任务作废之后被人照旧拣了，那一笔记成迟到、货排进回库，不算这张请求的已拣。
// 货回库之后请求再放进波次、拣完，复核只要对这一次拣的数量，打包也只装这一次拣的。
func TestALatePickOnAWithdrawnTaskDoesNotCountTowardTheRequest(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 10)
	request := reserve("REQ-WD-LATE")
	first := world.releaseRequests(t, handler, "WAVE-WD-LATE-1", request)
	if first.Outcome() != application.WaveAllocated || len(first.Tasks()) != 1 {
		t.Fatalf("第一次放出 %s", first.Outcome())
	}
	stale := first.Tasks()[0].ID
	if got := world.withdraw(t, handler, "WAVE-WD-LATE-1", request).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("撤出 = %s", got)
	}
	if got := world.pick(t, handler, stale, 10).Outcome(); got != application.WaveRecorded {
		t.Fatalf("作废之后照旧拣 = %s", got)
	}
	if world.requestStatus(t, request) != "RESERVED" || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 10 {
		t.Fatalf("迟到之后请求 %s，待出库 %d", world.requestStatus(t, request), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	returned := world.completeReturn(t, handler, request.String()+"/"+stale.String(), "SYN-FACT-WD-RETURN", "SYN-LOC-STORE")
	if returned.Outcome() != application.WaveRecorded || world.stockUnitAt(t, "SYN-LOC-STORE", "件") != 10 || world.requestStatus(t, request) != "RESERVED" {
		t.Fatalf("回库 %s，存储 %d，请求 %s", returned.Outcome(), world.stockUnitAt(t, "SYN-LOC-STORE", "件"), world.requestStatus(t, request))
	}

	second := world.releaseRequests(t, handler, "WAVE-WD-LATE-2", request)
	if second.Outcome() != application.WaveAllocated || len(second.Tasks()) != 1 {
		t.Fatalf("再放 %s", second.Outcome())
	}
	if world.pick(t, handler, second.Tasks()[0].ID, 10).Outcome() != application.WavePicked || world.requestStatus(t, request) != "PICKED" {
		t.Fatalf("再放之后拣货，请求 %s", world.requestStatus(t, request))
	}
	if got := world.checkOutbound(t, handler, request, "SYN-CHECK-WD-LATE", 10).Outcome(); got != application.WaveChecked {
		t.Fatalf("复核这一次拣的 10 件 = %s", got)
	}
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		result, packErr := handler.Pack(ctx, application.PackCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, "SYN-PACK-WD-LATE"),
			Request: request, Parcels: []application.PackParcelCommand{parcelOf(t, 1, 10)},
		})
		if packErr != nil {
			return packErr
		}
		if result.Outcome() != application.WavePacked {
			return errors.New(result.Outcome().String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("打包这一次拣的 10 件：%v", err)
	}
}

// Covers: 撤出的任务被迟到拣过之后，这张请求照样截得掉：迟到的货已经排过回库，截单不再给它排第二条。
func TestAWithdrawnRequestWithALatePickCanStillBeCutOff(t *testing.T) {
	world, handler, reserve := withdrawalWorld(t, 10)
	request := reserve("REQ-WD-LATE-CUT")
	released := world.releaseRequests(t, handler, "WAVE-WD-LATE-CUT", request)
	if released.Outcome() != application.WaveAllocated {
		t.Fatalf("放出 %s", released.Outcome())
	}
	if got := world.withdraw(t, handler, "WAVE-WD-LATE-CUT", request).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("撤出 = %s", got)
	}
	if got := world.pick(t, handler, released.Tasks()[0].ID, 10).Outcome(); got != application.WaveRecorded {
		t.Fatalf("作废之后照旧拣 = %s", got)
	}
	if got := world.cutoff(t, handler, request, 0, "SYN-CUT-WD-LATE").Outcome(); got != application.CutoffCancelled {
		t.Fatalf("迟到之后截单 = %s", got)
	}
	if world.requestStatus(t, request) != "CANCELLED" || reservedQuantity(t, world) != 0 {
		t.Fatalf("截单之后请求 %s，预占 %d", world.requestStatus(t, request), reservedQuantity(t, world))
	}
	var returns int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.return_task WHERE tenant_id = 'SYN-TENANT-01' AND request_id = $1`,
		request.String()).Scan(&returns); err != nil || returns != 1 {
		t.Fatalf("回库任务 %d 条 err %v", returns, err)
	}
}

// Covers: 预取过的请求撤出之后再放进波次，不再交第二份估算委托，放波次照样成立。
func TestAWithdrawnRequestKeepsItsPrefetchWhenReleasedAgain(t *testing.T) {
	measured := newMeasurementWorld(t)
	measured.seedStorageStock(t, 10)
	if measured.record(t, measured.clock.Now(), goodsLevelMeasureCommand(
		t, "SYN-FACT-MEASURE-WD", "DEV-1", measured.clock.Now(), 1, "SYN-WH-01", "SYN-OWNER-01", "SYN-GOODS-01", "件",
		"10", "10", "10", "CM", "1", "KG",
	)).Outcome().String() == "" {
		t.Fatal("仓测没有记下")
	}
	materials, err := adapter.NewPackagingMaterials(measured.db)
	if err != nil {
		t.Fatal(err)
	}
	if savePackagingMaterial(t, packagingMaterialStores{transactor: measured.transactor, pool: measured.pool, packagingMaterials: materials}, packagingMaterialFixture(t, "SYN-TENANT-01", "SYN-BOX-PRE", 1, nil)) != ports.RegistryRevisionSaved {
		t.Fatal("包材没有记下")
	}
	accepted := measured.acceptOutbound(t, measured.outbound(t), outboundCommand(t, "REQ-WD-PREFETCH", 10))
	request, ok := accepted.Request()
	if accepted.Outcome() != application.OutboundReserved || !ok {
		t.Fatalf("预占 %s", accepted.Outcome())
	}
	commissioner := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-WD"}
	handler := measured.waves(t).WithPrefetch(commissioner, measured.measurements, materials)
	if measured.adoptStrategy(t, handler).Outcome() != application.WaveStrategyRegistered {
		t.Fatal("策略没有采用")
	}
	release := func(id string) application.WaveResult {
		t.Helper()
		var released application.WaveResult
		if err := measured.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var releaseErr error
			released, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
				Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
				Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
				Requests: []domain.OutboundRequestID{request.ID()}, Prefetch: true,
				Packaging: valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-PRE"),
			})
			return releaseErr
		}); err != nil {
			t.Fatalf("放波次 %s：%v", id, err)
		}
		return released
	}
	if release("WAVE-WD-PRE-1").Outcome() != application.WaveAllocated || commissioner.called != 1 {
		t.Fatalf("第一次放出，预取 %d 次", commissioner.called)
	}
	if got := measured.withdraw(t, handler, "WAVE-WD-PRE-1", request.ID()).Outcome(); got != application.WaveWithdrawn {
		t.Fatalf("撤出 = %s", got)
	}
	if release("WAVE-WD-PRE-2").Outcome() != application.WaveAllocated || commissioner.called != 1 {
		t.Fatalf("撤出之后再放，预取 %d 次", commissioner.called)
	}
}
