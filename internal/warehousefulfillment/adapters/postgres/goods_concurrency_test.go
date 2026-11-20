package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 三件并发都靠同一货主上的事务级咨询锁，不靠碰巧的先后：第一笔插入后按住不提交，第二笔保存时卡在锁上，观察连接确认卡住了才放第一笔提交。

type goodsRaceOutcome struct {
	result ports.GoodsSaveResult
	err    error
}

func raceGoodsSaves(t *testing.T, stores goodsStores, first, second domain.GoodsRevision) (goodsRaceOutcome, goodsRaceOutcome) {
	t.Helper()
	observer := newBlockedSessionObserver(t, stores.pool)
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var firstOutcome, secondOutcome goodsRaceOutcome
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		firstOutcome.err = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			firstOutcome.result, saveErr = stores.goods.SaveGoodsRevision(txCtx, first)
			close(inserted)
			if saveErr == nil {
				<-release
			}
			return saveErr
		})
	}()
	<-inserted
	wait.Add(1)
	go func() {
		defer wait.Done()
		secondOutcome.err = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			secondOutcome.result, saveErr = stores.goods.SaveGoodsRevision(txCtx, second)
			return saveErr
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if !blocked {
		t.Errorf("第二笔没有卡在货主锁上")
	}
	return firstOutcome, secondOutcome
}

func TestGoodsRegistryConcurrentSavesAreSerializedPerOwner(t *testing.T) {
	t.Run("同一键内容相同", func(t *testing.T) {
		stores := newGoodsStores(t)
		first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil)
		second, err := domain.NewGoodsRevision(first.Tenant(), first.Owner(), first.Code(), 1, first.Content(), first.Submission(),
			valueOf(t, domain.NewRegistrant, "operator-2"), baseInstant.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		left, right := raceGoodsSaves(t, stores, first, second)
		if left.err != nil || right.err != nil {
			t.Fatalf("错误 %v %v", left.err, right.err)
		}
		if left.result.Outcome != ports.GoodsRevisionSaved || right.result.Outcome != ports.GoodsRevisionAlreadyRegistered {
			t.Fatalf("落点 %s %s", left.result.Outcome, right.result.Outcome)
		}
	})
	t.Run("同一键内容不同", func(t *testing.T) {
		stores := newGoodsStores(t)
		first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", nil)
		second := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", func(spec *domain.GoodsContentSpec) {
			spec.Name = valueOf(t, domain.NewGoodsName, "并发的另一份")
		})
		left, right := raceGoodsSaves(t, stores, first, second)
		if left.err != nil || right.err != nil {
			t.Fatalf("错误 %v %v", left.err, right.err)
		}
		if left.result.Outcome != ports.GoodsRevisionSaved || right.result.Outcome != ports.GoodsSourceConflict {
			t.Fatalf("落点 %s %s", left.result.Outcome, right.result.Outcome)
		}
	})
	t.Run("同一修订号", func(t *testing.T) {
		stores := newGoodsStores(t)
		first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-A", nil)
		second := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-B", func(spec *domain.GoodsContentSpec) {
			spec.Name = valueOf(t, domain.NewGoodsName, "另一笔首次登记")
		})
		left, right := raceGoodsSaves(t, stores, first, second)
		if left.err != nil || right.err != nil {
			t.Fatalf("错误 %v %v", left.err, right.err)
		}
		if left.result.Outcome != ports.GoodsRevisionSaved || right.result.Outcome != ports.GoodsBasisStale {
			t.Fatalf("落点 %s %s", left.result.Outcome, right.result.Outcome)
		}
	})
	t.Run("抢同一个码", func(t *testing.T) {
		stores := newGoodsStores(t)
		alias := func(spec *domain.GoodsContentSpec) {
			spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
				valueOf(t, domain.NewGoodsAliasCode, "SYN-SHARED"), domain.GoodsAliasBarcode, domain.PlatformName{}, domain.AliasValidity{},
			)}
		}
		first := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-1", alias)
		second := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-02", 1, "REQ-2", alias)
		left, right := raceGoodsSaves(t, stores, first, second)
		if left.err != nil || right.err != nil {
			t.Fatalf("错误 %v %v", left.err, right.err)
		}
		if left.result.Outcome != ports.GoodsRevisionSaved || right.result.Outcome != ports.GoodsIdentifierConflict || right.result.ConflictingCode.String() != "SYN-SHARED" {
			t.Fatalf("落点 %s %s %q", left.result.Outcome, right.result.Outcome, right.result.ConflictingCode)
		}
	})
}

type silentGoodsStock struct{}

func (silentGoodsStock) HasOnHandStock(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) (bool, error) {
	return false, nil
}

type presentOwnerAccounts struct{}

func (presentOwnerAccounts) OwnerAccountRegistered(context.Context, domain.TenantID, domain.OwnerAccountID) (bool, error) {
	return true, nil
}

// Covers: AT-WF-014、AT-WF-015（商品的核心）——登记用例在真库上并发：同一键按内容答，同一修订号答基准过期并回显当前修订。
func TestRegisterGoodsConcurrentRegistrationsOnTheRealRegister(t *testing.T) {
	stores := newGoodsStores(t)
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil))
	observer := newBlockedSessionObserver(t, stores.pool)
	ctx := t.Context()
	clock := fixedClock{at: baseInstant}
	command := func(key, name, registrant string) application.RegisterGoodsCommand {
		submission, err := domain.NewOwnerSubmission(
			valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01"), valueOf(t, domain.NewRequestKey, key))
		if err != nil {
			t.Fatal(err)
		}
		return application.RegisterGoodsCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			Code: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Content: goodsContentFixture(t, func(spec *domain.GoodsContentSpec) {
				spec.Name = valueOf(t, domain.NewGoodsName, name)
			}),
			Submission: submission, Registrant: valueOf(t, domain.NewRegistrant, registrant),
		}
	}
	firstCommand := command("REQ-1", "合成商品", "operator-1")
	secondCommand := command("REQ-1", "并发的另一份", "operator-2")
	inserted := make(chan struct{})
	release := make(chan struct{})
	holding := holdingGoodsRegistry{GoodsRegistry: stores.goods, afterSave: func() {
		close(inserted)
		<-release
	}}
	var firstResult, secondResult application.RegisterGoodsResult
	var firstErr, secondErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		handler := application.NewRegisterGoodsHandler(holding, stores.warehouses, silentGoodsStock{}, presentOwnerAccounts{}, clock)
		firstErr = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			firstResult, registerErr = handler.Register(txCtx, firstCommand)
			return registerErr
		})
	}()
	select {
	case <-inserted:
	case <-time.After(10 * time.Second):
		t.Fatal("第一笔没有走到保存")
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		handler := application.NewRegisterGoodsHandler(stores.goods, stores.warehouses, silentGoodsStock{}, presentOwnerAccounts{}, clock)
		secondErr = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var registerErr error
			secondResult, registerErr = handler.Register(txCtx, secondCommand)
			return registerErr
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if !blocked {
		t.Errorf("第二笔没有卡在货主锁上")
	}
	if firstErr != nil || secondErr != nil {
		t.Fatalf("错误 %v %v", firstErr, secondErr)
	}
	if firstResult.Outcome() != application.GoodsOutcomeRegistered || secondResult.Outcome() != application.GoodsOutcomeSourceConflict {
		t.Fatalf("结果 %s %s", firstResult.Outcome(), secondResult.Outcome())
	}
}

type holdingGoodsRegistry struct {
	ports.GoodsRegistry
	afterSave func()
}

func (registry holdingGoodsRegistry) SaveGoodsRevision(ctx context.Context, revision domain.GoodsRevision) (ports.GoodsSaveResult, error) {
	result, err := registry.GoodsRegistry.SaveGoodsRevision(ctx, revision)
	if registry.afterSave != nil {
		registry.afterSave()
	}
	return result, err
}
