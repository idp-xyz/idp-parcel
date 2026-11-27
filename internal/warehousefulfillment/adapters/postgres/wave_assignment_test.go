package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const (
	pickerOne = "https://issuer.example.invalid#syn-picker-1"
	pickerTwo = "https://issuer.example.invalid#syn-picker-2"
)

func (world receiptWorld) assignWave(t *testing.T, handler *application.WaveHandler, wave, assignee string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var assignErr error
		result, assignErr = handler.AssignWave(ctx, application.AssignWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Wave: valueOf(t, domain.NewWaveID, wave),
			Assignee: assignee, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return assignErr
	})
	if err != nil {
		t.Fatalf("指派 %s：%v", wave, err)
	}
	return result
}

func (world receiptWorld) claimWave(t *testing.T, handler *application.WaveHandler, wave, warehouse, operator string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var claimErr error
		result, claimErr = handler.ClaimWave(ctx, application.ClaimWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Wave: valueOf(t, domain.NewWaveID, wave),
			Warehouse: valueOf(t, domain.NewWarehouseCode, warehouse), Operator: operator,
		})
		return claimErr
	})
	if err != nil {
		t.Fatalf("领波次 %s：%v", wave, err)
	}
	return result
}

func (world receiptWorld) pickAs(t *testing.T, handler *application.WaveHandler, fact string, task domain.PickTaskID, quantity int64, operator string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var pickErr error
		result, pickErr = handler.Pick(ctx, application.PickCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewPickFactID, fact),
			TaskID: task, Quantity: quantity, Target: valueOf(t, domain.NewLocationCode, "SYN-LOC-OUT"), Operator: operator,
		})
		return pickErr
	})
	if err != nil {
		t.Fatalf("拣货 %s：%v", fact, err)
	}
	return result
}

func (world receiptWorld) assignWaveZone(t *testing.T, handler *application.WaveHandler, wave, zone, assignee string) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var assignErr error
		result, assignErr = handler.AssignWave(ctx, application.AssignWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Wave: valueOf(t, domain.NewWaveID, wave),
			Zone: valueOf(t, domain.NewZoneCode, zone), Assignee: assignee, Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return assignErr
	})
	if err != nil {
		t.Fatalf("按库区指派 %s %s：%v", wave, zone, err)
	}
	return result
}

// Covers: 按库区派工。一个波次的两条任务分在两个库区：一个库区单独派给甲之后，乙拣那个库区的格不成立、甲照拣；
// 没单独派的库区照整个波次的指派走（波次派给乙，乙拣得了、甲拣不了）。库区的指派可以改、可以取消，取消之后那个库区
// 回到照整个波次的走；读口上任务带着所在库区、波次带着单独派了人的库区。这个仓没有的库区派不上。
func TestAWaveZoneAssignedToOnePickerOverridesTheWaveAssignment(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	// 第二个库位挪到另一个库区：登一个新库区，再给库位登一版修订指过去。
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE-B", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE-2", 2, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE-B")
	}))
	request := reserve("REQ-ZONE", 10)
	released := world.releaseRequests(t, handler, "WAVE-ZONE", request)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 2 {
		t.Fatalf("放出 %s，任务 %d 条", released.Outcome(), len(released.Tasks()))
	}
	inA, inB := taskAt(t, released.Tasks(), "SYN-LOC-STORE"), taskAt(t, released.Tasks(), "SYN-LOC-STORE-2")

	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-NOT-THERE", pickerOne).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("派给这个仓没有的库区 = %s", got)
	}
	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-STORE-B", pickerOne); got.Outcome() != application.WaveAssigned || got.Assignee() != pickerOne {
		t.Fatalf("按库区指派 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-STORE-B", pickerOne).Outcome(); got != application.WaveExisting {
		t.Fatalf("同一个库区再派给同一个人 = %s", got)
	}
	if got := world.assignWave(t, handler, "WAVE-ZONE", pickerTwo).Outcome(); got != application.WaveAssigned {
		t.Fatalf("整个波次派给乙 = %s", got)
	}
	row := world.waveRow(t, "WAVE-ZONE")
	if row.Assignee != pickerTwo || len(row.ZoneAssignees) != 1 || row.ZoneAssignees[0].ZoneCode != "SYN-ZONE-STORE-B" || row.ZoneAssignees[0].Assignee != pickerOne {
		t.Fatalf("读口上的指派：波次 %q，库区 %+v", row.Assignee, row.ZoneAssignees)
	}
	zones := map[string]string{}
	for _, task := range row.Tasks {
		zones[task.LocationCode] = task.ZoneCode
	}
	if zones["SYN-LOC-STORE"] != "SYN-ZONE-STORE" || zones["SYN-LOC-STORE-2"] != "SYN-ZONE-STORE-B" {
		t.Fatalf("读口上任务所在的库区 %v", zones)
	}

	// 单独派了人的库区认那个人；没单独派的库区认整个波次派给的人。
	if got := world.pickAs(t, handler, "SYN-FACT-ZONE-B-OTHER", inB.ID, 4, pickerTwo); got.Outcome() != application.WaveAssignedToOther || got.Assignee() != pickerOne {
		t.Fatalf("乙拣派给甲的库区 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.pickAs(t, handler, "SYN-FACT-ZONE-A-OTHER", inA.ID, 6, pickerOne); got.Outcome() != application.WaveAssignedToOther || got.Assignee() != pickerTwo {
		t.Fatalf("甲拣没单独派的库区 = %s %q", got.Outcome(), got.Assignee())
	}
	if world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatalf("被拒的两笔动了货：待出库 %d", world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	if got := world.pickAs(t, handler, "SYN-FACT-ZONE-A-MINE", inA.ID, 6, pickerTwo).Outcome(); got != application.WavePicked {
		t.Fatalf("乙拣整个波次派给他的库区 = %s", got)
	}

	// 改派与取消：取消之后这个库区回到照整个波次的指派走。
	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-STORE-B", pickerTwo); got.Outcome() != application.WaveAssigned || got.Assignee() != pickerTwo {
		t.Fatalf("库区改派 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-STORE-B", "").Outcome(); got != application.WaveUnassigned || len(world.waveRow(t, "WAVE-ZONE").ZoneAssignees) != 0 {
		t.Fatalf("取消库区指派 = %s，还剩 %+v", got, world.waveRow(t, "WAVE-ZONE").ZoneAssignees)
	}
	if got := world.pickAs(t, handler, "SYN-FACT-ZONE-B-AFTER", inB.ID, 4, pickerOne); got.Outcome() != application.WaveAssignedToOther || got.Assignee() != pickerTwo {
		t.Fatalf("取消库区指派之后甲拣 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.pickAs(t, handler, "SYN-FACT-ZONE-B-MINE", inB.ID, 4, pickerTwo).Outcome(); got != application.WavePicked {
		t.Fatalf("取消库区指派之后乙拣 = %s", got)
	}
	if got := world.assignWaveZone(t, handler, "WAVE-ZONE", "SYN-ZONE-STORE-B", pickerOne).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("拣完的波次按库区派 = %s", got)
	}
}

// Covers: PBC-08——按库区派工只经框架事务。
func TestSavingAWaveZoneAssigneeRefusesToRunOutsideATransaction(t *testing.T) {
	world, _, _ := twoCellWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	err = waves.SaveWaveZoneAssignee(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWaveID, "WAVE-ZONE"),
		valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE"), pickerOne, "operator-1", world.clock.Now())
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务按库区派：err = %v, want ErrTransactionRequired", err)
	}
}

// Covers: 波次派给一个人之后，别人领不了、拣它待执行的任务也不成立（货不动、任务仍待执行），派给的那个人照拣。
// 没人领的波次谁领归谁；管波次的人可以改派、取消指派，取消之后别人又能领。同一个人再派、再领答已有结果。
// 拣完的波次不能再派；没有这个波次答查无，波次不属于设备绑的仓答内容不合。
func TestAWaveAssignedToOnePickerIsNotPickedByAnother(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	first, second := reserve("REQ-ASSIGN-1", 2), reserve("REQ-ASSIGN-2", 2)
	assigned := world.releaseRequests(t, handler, "WAVE-ASSIGNED", first)
	free := world.releaseRequests(t, handler, "WAVE-FREE", second)
	if assigned.Outcome() != application.WaveAllocated || free.Outcome() != application.WaveAllocated {
		t.Fatalf("放出 %s / %s", assigned.Outcome(), free.Outcome())
	}
	if got := world.assignWave(t, handler, "WAVE-ASSIGNED", pickerOne); got.Outcome() != application.WaveAssigned || got.Assignee() != pickerOne {
		t.Fatalf("指派 = %s %q", got.Outcome(), got.Assignee())
	}
	if world.waveRow(t, "WAVE-ASSIGNED").Assignee != pickerOne || world.waveRow(t, "WAVE-FREE").Assignee != "" {
		t.Fatalf("读口上的拣货人 %q / %q", world.waveRow(t, "WAVE-ASSIGNED").Assignee, world.waveRow(t, "WAVE-FREE").Assignee)
	}
	if got := world.assignWave(t, handler, "WAVE-ASSIGNED", pickerOne).Outcome(); got != application.WaveExisting {
		t.Fatalf("再派给同一个人 = %s", got)
	}
	if got := world.claimWave(t, handler, "WAVE-ASSIGNED", "SYN-WH-01", pickerTwo); got.Outcome() != application.WaveAssignedToOther || got.Assignee() != pickerOne {
		t.Fatalf("别人来领 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.claimWave(t, handler, "WAVE-ASSIGNED", "SYN-WH-01", pickerOne).Outcome(); got != application.WaveExisting {
		t.Fatalf("本人再领 = %s", got)
	}
	task := assigned.Tasks()[0]
	refused := world.pickAs(t, handler, "SYN-FACT-ASSIGN-OTHER", task.ID, 2, pickerTwo)
	if refused.Outcome() != application.WaveAssignedToOther || refused.Assignee() != pickerOne || world.stockUnitAt(t, "SYN-LOC-OUT", "件") != 0 {
		t.Fatalf("别人拣 = %s，待出库 %d", refused.Outcome(), world.stockUnitAt(t, "SYN-LOC-OUT", "件"))
	}
	if row := world.waveRow(t, "WAVE-ASSIGNED"); len(row.Tasks) != 1 || row.Tasks[0].Status != "OPEN" {
		t.Fatalf("被拒之后的任务 %+v", row.Tasks)
	}

	// 没人领的波次：谁领归谁，领了之后改派、取消都由管波次的人来。
	if got := world.claimWave(t, handler, "WAVE-NOT-THERE", "SYN-WH-01", pickerTwo).Outcome(); got != application.WaveNotFound {
		t.Fatalf("没有的波次 = %s", got)
	}
	if got := world.claimWave(t, handler, "WAVE-FREE", "SYN-WH-ELSEWHERE", pickerTwo).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("别的仓的设备来领 = %s", got)
	}
	if got := world.claimWave(t, handler, "WAVE-FREE", "SYN-WH-01", pickerTwo); got.Outcome() != application.WaveAssigned || got.Assignee() != pickerTwo {
		t.Fatalf("领没人领的波次 = %s %q", got.Outcome(), got.Assignee())
	}
	if got := world.assignWave(t, handler, "WAVE-FREE", pickerOne).Outcome(); got != application.WaveAssigned {
		t.Fatalf("改派 = %s", got)
	}
	if got := world.assignWave(t, handler, "WAVE-FREE", "").Outcome(); got != application.WaveUnassigned || world.waveRow(t, "WAVE-FREE").Assignee != "" {
		t.Fatalf("取消指派 = %s，拣货人 %q", got, world.waveRow(t, "WAVE-FREE").Assignee)
	}
	if got := world.pickAs(t, handler, "SYN-FACT-ASSIGN-FREE", free.Tasks()[0].ID, 2, pickerTwo).Outcome(); got != application.WavePicked {
		t.Fatalf("取消指派之后别人拣 = %s", got)
	}

	if got := world.pickAs(t, handler, "SYN-FACT-ASSIGN-MINE", task.ID, 2, pickerOne).Outcome(); got != application.WavePicked {
		t.Fatalf("派给的那个人拣 = %s", got)
	}
	if got := world.assignWave(t, handler, "WAVE-ASSIGNED", pickerTwo).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("拣完的波次改派 = %s", got)
	}
	if got := world.claimWave(t, handler, "WAVE-FREE", "SYN-WH-01", pickerOne).Outcome(); got != application.WaveStatusMismatch {
		t.Fatalf("拣完的波次来领 = %s", got)
	}
}

func TestWritingAWaveAssignmentRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, wave := valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWaveID, "WAVE-1")
	if err := waves.SaveWaveAssignee(context.Background(), tenant, wave, pickerOne, "operator-1", preAdviceNumberingInstant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveWaveAssignee：%v", err)
	}
	if _, _, err := waves.LoadWaveAssignment(context.Background(), tenant, wave); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("LoadWaveAssignment：%v", err)
	}
}
