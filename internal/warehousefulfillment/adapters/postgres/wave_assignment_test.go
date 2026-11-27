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
