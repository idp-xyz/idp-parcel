package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: 管理台手工确认在生产装配里取的是真的费用行锁：它不经包裹锁，复磅形成那一头在同一笔费用上记替代时，两头只靠这一行
// 排队。另一笔事务持着这笔费用的行锁未提交时，手工确认要等；对方提交之后才往下走。装配若换成一只不排队的锁，手工确认就能
// 夹在「读到未被替代」与「落确认」之间，把已被替代的预估确认掉。真库，合成串，证据只记 S。
func TestTheManualConfirmationInTheProductionAssemblyWaitsForTheChargeRowLock(t *testing.T) {
	db := evaluationRequestTestDB(t)
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	orchestrations, err := buildSettlementOrchestrations(db, numbers.Assigner)
	if err != nil {
		t.Fatalf("装配结算编排：%v", err)
	}
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	moments, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := sadomain.NewTenantID("SYN-TENANT-CONFIRM-LOCK")
	if err != nil {
		t.Fatal(err)
	}
	chargeID, err := sadomain.NewCustomerChargeID("SYN-CHARGE-CONFIRM-LOCK")
	if err != nil {
		t.Fatal(err)
	}
	feeItem, err := sadomain.NewFeeItemReference("SYN-FEE-ITEM-FREIGHT")
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := sadomain.NewSellEvaluationReference("SYN-EVAL-SELL-LOCK")
	if err != nil {
		t.Fatal(err)
	}
	currency, err := sadomain.NewCurrencyCode("CNY")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute)
	charge, err := sadomain.FormCustomerCharge(sadomain.CustomerChargeSpec{
		ID: chargeID, FeeItem: feeItem, Evaluation: evaluation,
		OriginalCurrency: currency, OriginalMinor: 12000, SettlementCurrency: currency, SettlementMinor: 12000,
		Stage: sadomain.ChargeEstimated, FormedAt: at, SourceOccurredAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := moments.SaveSettlementMoment(txCtx, tenant, sadomain.SettlementMomentConfirm, at); err != nil {
			return err
		}
		_, err := charges.SaveEstimated(txCtx, tenant, charge)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	held, release, holdDone, holdFinished := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan struct{})
	var releaseOnce sync.Once
	releaseHold := func() { releaseOnce.Do(func() { close(release) }) }
	// 断言失败时也放开持锁的一头：它的事务不结束，库就收不了尾，用例会挂到超时而不是报红。
	t.Cleanup(func() {
		releaseHold()
		<-holdFinished
	})
	go func() {
		defer close(holdFinished)
		holdDone <- db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if err := charges.LockChargeStage(txCtx, tenant, chargeID); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holdDone:
		t.Fatalf("持锁的一头没走到持锁就结束了：%v", err)
	case <-time.After(20 * time.Second):
		t.Fatal("持锁的一头 20 秒没走到持锁")
	}

	type answer struct {
		moment saapplication.SettlementMomentOutcome
		err    error
	}
	answered := make(chan answer, 1)
	go func() {
		var got answer
		got.moment, _, got.err = orchestrations.Confirm(context.Background(), saapplication.ConfirmChargeCommand{
			TenantID: tenant, ChargeID: chargeID.String(),
		})
		answered <- got
	}()
	select {
	case got := <-answered:
		t.Fatalf("手工确认没等对方持着的费用行锁就答了：时点 %s err = %v", got.moment, got.err)
	case <-time.After(500 * time.Millisecond):
	}

	releaseHold()
	if err := <-holdDone; err != nil {
		t.Fatalf("持锁一头提交：%v", err)
	}
	if got := <-answered; got.err != nil || got.moment != saapplication.SettlementMomentAdmitted {
		t.Fatalf("对方提交之后的手工确认 时点 = %s err = %v，want 时点放行、走到确认编排", got.moment, got.err)
	}
}
