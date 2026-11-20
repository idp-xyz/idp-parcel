package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestPreAdviceConcurrentSameKeySubmitsOnce(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, nil)
	world.seedGoods(t)
	command := preAdviceAcceptCommand(t, "REQ-SAME", nil)
	left, right := racePreAdvice(t, world,
		func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
			return world.handler().Accept(ctx, command)
		},
		func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
			return world.handler().Accept(ctx, command)
		},
	)
	if left.Outcome() != application.PreAdviceOutcomeAdvised || right.Outcome() != application.PreAdviceOutcomeExistingResult {
		t.Fatalf("并发提交 = %s、%s", left.Outcome(), right.Outcome())
	}
	leftAdvice, _ := left.PreAdvice()
	rightAdvice, hasRight := right.PreAdvice()
	leftNumber, _ := leftAdvice.DocumentNumber()
	rightNumber, _ := rightAdvice.DocumentNumber()
	if !hasRight || rightAdvice.ID() != leftAdvice.ID() || rightNumber != leftNumber {
		t.Fatalf("后一笔没有回显原预报与原号")
	}
	var advices, numbers int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*), count(document_number) FROM warehouse_fulfillment.inbound_pre_advice WHERE tenant_id = $1`,
		"SYN-TENANT-01").Scan(&advices, &numbers); err != nil {
		t.Fatal(err)
	}
	if advices != 1 || numbers != 1 {
		t.Fatalf("预报 %d 份、号 %d 个", advices, numbers)
	}
}

func TestPreAdviceWithdrawAndReceiptStartExcludeEachOther(t *testing.T) {
	t.Run("撤回先", func(t *testing.T) {
		world := newPreAdviceWorld(t, preAdviceNumberingInstant)
		world.seedWarehouse(t, nil)
		world.seedGoods(t)
		advice, _ := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", nil)).PreAdvice()
		var withdrawn application.AcceptPreAdviceResult
		var noted application.NotePreAdviceReceiptResult
		racePreAdvice(t, world,
			func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
				var err error
				withdrawn, err = world.handler().Withdraw(ctx, preAdviceWithdrawCommand(t, advice.ID(), "REQ-W"))
				return withdrawn, err
			},
			func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
				var err error
				noted, err = world.handler().NoteReceiptStarted(ctx, application.NotePreAdviceCommand{Tenant: advice.Tenant(), ID: advice.ID()})
				return application.AcceptPreAdviceResult{}, err
			},
		)
		reloaded, found, err := world.preAdvices.Find(t.Context(), advice.Tenant(), advice.ID())
		if err != nil || !found {
			t.Fatalf("读回 %v %v", found, err)
		}
		if withdrawn.Outcome() != application.PreAdviceOutcomeCancelled || noted.Outcome() != application.PreAdviceReceiptNotReceivable ||
			reloaded.Status() != domain.PreAdviceCancelled {
			t.Fatalf("撤回先：撤回 %s 收货缝 %s 状态 %s", withdrawn.Outcome(), noted.Outcome(), reloaded.Status())
		}
	})
	t.Run("收货缝先", func(t *testing.T) {
		world := newPreAdviceWorld(t, preAdviceNumberingInstant)
		world.seedWarehouse(t, nil)
		world.seedGoods(t)
		advice, _ := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", nil)).PreAdvice()
		var noted application.NotePreAdviceReceiptResult
		var withdrawn application.AcceptPreAdviceResult
		racePreAdvice(t, world,
			func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
				var err error
				noted, err = world.handler().NoteReceiptStarted(ctx, application.NotePreAdviceCommand{Tenant: advice.Tenant(), ID: advice.ID()})
				return application.AcceptPreAdviceResult{}, err
			},
			func(ctx context.Context) (application.AcceptPreAdviceResult, error) {
				var err error
				withdrawn, err = world.handler().Withdraw(ctx, preAdviceWithdrawCommand(t, advice.ID(), "REQ-W"))
				return withdrawn, err
			},
		)
		reloaded, found, err := world.preAdvices.Find(t.Context(), advice.Tenant(), advice.ID())
		if err != nil || !found {
			t.Fatalf("读回 %v %v", found, err)
		}
		status, hasStatus := withdrawn.Status()
		if noted.Outcome() != application.PreAdviceReceiptStarted || withdrawn.Outcome() != application.PreAdviceOutcomeReceiptStarted ||
			!hasStatus || status != domain.PreAdviceReceiving || reloaded.Status() != domain.PreAdviceReceiving {
			t.Fatalf("收货缝先：收货缝 %s 撤回 %s %s 状态 %s", noted.Outcome(), withdrawn.Outcome(), status, reloaded.Status())
		}
	})
}

// racePreAdvice 让第一笔在事务里做完后先不提交，第二笔起来后必须卡在锁上。观察连接确认了才放第一笔提交。
func racePreAdvice(
	t *testing.T,
	world preAdviceWorld,
	first, second func(context.Context) (application.AcceptPreAdviceResult, error),
) (application.AcceptPreAdviceResult, application.AcceptPreAdviceResult) {
	t.Helper()
	observer := newBlockedSessionObserver(t, world.pool)
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var left, right application.AcceptPreAdviceResult
	var leftErr, rightErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		leftErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			left, err = first(txCtx)
			close(inserted)
			if err == nil {
				<-release
			}
			return err
		})
	}()
	<-inserted
	wait.Add(1)
	go func() {
		defer wait.Done()
		rightErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			right, err = second(txCtx)
			return err
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if !blocked {
		t.Error("后一笔没有卡在锁上")
	}
	if leftErr != nil || rightErr != nil {
		t.Fatalf("两笔的错误：%v、%v", leftErr, rightErr)
	}
	return left, right
}
