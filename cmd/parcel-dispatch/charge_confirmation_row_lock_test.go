package main

import (
	"context"
	"sync"
	"testing"
	"time"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: 到点自动确认与形成后的跟进确认共用的那只确认编排，在生产装配里取的是真的费用行锁：另一笔事务持着这笔费用的行锁
// 未提交时，确认要等；对方提交之后才往下走。装配若换成一只不排队的锁，确认就会夹进复磅形成「读到仍是预估」与「记替代」
// 之间。真库，合成串，证据只记 S。
func TestTheConfirmationOnBasisInTheProductionAssemblyWaitsForTheChargeRowLock(t *testing.T) {
	_, db, store := wiredBeat(t)
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)
	seedConfirmingTenant(t, db, tenant)

	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	chargeID := saValue(t, sadomain.NewCustomerChargeID, "SELLCC-SELLREQ-SYN-CONFIRM")
	charge, err := sadomain.FormCustomerCharge(sadomain.CustomerChargeSpec{
		ID:                 chargeID,
		FeeItem:            saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-FREIGHT"),
		Evaluation:         saValue(t, sadomain.NewSellEvaluationReference, "SYN-EVAL-SELL-LOCK"),
		OriginalCurrency:   saValue(t, sadomain.NewCurrencyCode, "CNY"),
		OriginalMinor:      12000,
		SettlementCurrency: saValue(t, sadomain.NewCurrencyCode, "CNY"),
		SettlementMinor:    12000,
		Stage:              sadomain.ChargeEstimated,
		FormedAt:           beatInstant(),
		SourceOccurredAt:   beatInstant(),
	})
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := charges.SaveEstimated(txCtx, tenant, charge)
		return err
	})
	confirmation, err := chargeConfirmationOnBasis(db, store, systemClock{})
	if err != nil {
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
		outcome saapplication.ConfirmOnBasisOutcome
		err     error
	}
	basis := saapplication.ConfirmationBasis{
		Kind:      sadomain.EffectiveDeliveryFinalizedBasis(),
		Reference: saValue(t, sadomain.NewConfirmationBasisReference, "final-outcome/SYN-PARCEL-01/EFFECTIVE_DELIVERY/SYN-OUTCOME-V1"),
	}
	answered := make(chan answer, 1)
	go func() {
		var got answer
		got.err = db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			result, err := confirmation.OnCharge(txCtx, tenant, chargeID, basis)
			got.outcome = result.Outcome()
			return err
		})
		answered <- got
	}()
	select {
	case got := <-answered:
		t.Fatalf("确认没等对方持着的费用行锁就答了：%s err = %v", got.outcome, got.err)
	case <-time.After(500 * time.Millisecond):
	}

	releaseHold()
	if err := <-holdDone; err != nil {
		t.Fatalf("持锁一头提交：%v", err)
	}
	if got := <-answered; got.err != nil || got.outcome != saapplication.ConfirmOnBasisConfirmAttempted {
		t.Fatalf("对方提交之后的确认 = %s err = %v，want 走到确认", got.outcome, got.err)
	}
}
