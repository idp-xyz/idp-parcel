package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type stockPostOutcome struct {
	result application.PostStockMovementResult
	err    error
}

type stockRegisterOutcome struct {
	result application.RegisterGoodsResult
	err    error
}

// raceStockPosts 让第一笔记数落册后按住事务不提交，第二笔起来后必须卡在锁上，观察连接确认了才放第一笔提交。
func raceStockPosts(t *testing.T, stores stockStores, first, second domain.StockMovementRequest) (stockPostOutcome, stockPostOutcome) {
	t.Helper()
	observer := newBlockedSessionObserver(t, stores.pool)
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var left, right stockPostOutcome
	var wait sync.WaitGroup
	handler := stores.poster()
	wait.Add(1)
	go func() {
		defer wait.Done()
		left.err = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var postErr error
			left.result, postErr = handler.Post(txCtx, application.PostStockMovementCommand{
				Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: first,
			})
			close(inserted)
			if postErr == nil {
				<-release
			}
			return postErr
		})
	}()
	<-inserted
	wait.Add(1)
	go func() {
		defer wait.Done()
		right.err = stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var postErr error
			right.result, postErr = handler.Post(txCtx, application.PostStockMovementCommand{
				Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: second,
			})
			return postErr
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if !blocked {
		t.Error("第二笔记数没有卡在锁上")
	}
	return left, right
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——并发两笔取走同一格，合计超过余额的那一笔不落。后一笔确实在等锁，不是碰巧一前一后。
func TestStockConcurrentTakesLeaveTheRemainder(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-1", nil)
	posted := expectPostedMovement(t, stores.post(t, stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-IN",
		domain.NewStockLineRequest(domain.StockIn, 10, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))))
	cell := posted.Lines()[0].Cell()
	take := func(source string) domain.StockMovementRequest {
		return stockRequest(t, domain.StockReasonOutboundConfirmation, source,
			domain.NewStockLineRequest(domain.StockOut, 6, cell, time.Time{}))
	}
	left, right := raceStockPosts(t, stores, take("SYN-FACT-A"), take("SYN-FACT-B"))
	if left.err != nil || right.err != nil {
		t.Fatalf("错误 %v %v", left.err, right.err)
	}
	if left.result.Outcome() != application.PostStockPosted || right.result.Outcome() != application.PostStockRefused {
		t.Fatalf("两笔 = %s %s", left.result.Outcome(), right.result.Outcome())
	}
	if violations := right.result.Violations(); len(violations) != 1 || violations[0].Kind != application.StockViolationInsufficientBalance {
		t.Fatalf("后一笔的不合 = %v", right.result.Violations())
	}
	balance, err := balanceOf(t, stores, cell)
	if err != nil || balance != 4 {
		t.Fatalf("余额 = %d（%v），want 4", balance, err)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——并发同一判重位置、内容相同，恰一笔落册，另一笔答已在册。后一笔确实在等锁。
func TestStockConcurrentReplayOfOneSourcePostsOnce(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-1", nil)
	request := stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-SAME",
		domain.NewStockLineRequest(domain.StockIn, 2, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))
	left, right := raceStockPosts(t, stores, request, request)
	if left.err != nil || right.err != nil {
		t.Fatalf("错误 %v %v", left.err, right.err)
	}
	if left.result.Outcome() != application.PostStockPosted || right.result.Outcome() != application.PostStockAlreadyPosted {
		t.Fatalf("两笔 = %s %s", left.result.Outcome(), right.result.Outcome())
	}
	if movements := countStockRows(t, stores.pool, "stock_movement"); movements != 1 {
		t.Fatalf("流水 %d 笔，want 1", movements)
	}
}

// Covers: AT-WF-016（库存账的那一半）——改是否跟踪批号与记数并发，两种先后都不得留下「修订已经改成跟踪批号、账上却有一格不带批号的库存」。后一笔都卡在货主商品锁上。
func TestStockRevisionAndPostingCannotBothSucceed(t *testing.T) {
	t.Run("修订先", func(t *testing.T) {
		stores := newStockStores(t)
		stores.seedShelf(t)
		if first := stores.register(t, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)); first.Outcome() != application.GoodsOutcomeRegistered {
			t.Fatalf("首次 = %s", first.Outcome())
		}
		observer := newBlockedSessionObserver(t, stores.pool)
		inserted := make(chan struct{})
		release := make(chan struct{})
		var registered stockRegisterOutcome
		var posted stockPostOutcome
		var wait sync.WaitGroup
		change := goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
			spec.TrackBatch = true
		})
		receipt := stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-LATE",
			domain.NewStockLineRequest(domain.StockIn, 1, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))
		wait.Add(1)
		go func() {
			defer wait.Done()
			registered.err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				var registerErr error
				registered.result, registerErr = stores.registrar().Register(txCtx, change)
				close(inserted)
				if registerErr == nil {
					<-release
				}
				return registerErr
			})
		}()
		<-inserted
		wait.Add(1)
		go func() {
			defer wait.Done()
			posted.err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				var postErr error
				posted.result, postErr = stores.poster().Post(txCtx, application.PostStockMovementCommand{
					Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: receipt,
				})
				return postErr
			})
		}()
		blocked := observer.waitForBlockedSession(t.Context(), 10*time.Second)
		close(release)
		wait.Wait()
		if !blocked {
			t.Error("记数没有卡在货主商品锁上")
		}
		if registered.err != nil || posted.err != nil {
			t.Fatalf("错误 %v %v", registered.err, posted.err)
		}
		if registered.result.Outcome() != application.GoodsOutcomeRegistered || posted.result.Outcome() != application.PostStockRefused {
			t.Fatalf("修订 %s，记数 %s %v", registered.result.Outcome(), posted.result.Outcome(), posted.result.Violations())
		}
		if violations := posted.result.Violations(); len(violations) != 1 || violations[0].Kind != application.StockViolationBatchNumberRequired {
			t.Fatalf("记数的不合 = %v", posted.result.Violations())
		}
		if bad := trackedGoodsWithUnbatchedStock(t, stores); bad != 0 {
			t.Fatalf("跟踪批号与不带批号的库存同时成立的商品 %d", bad)
		}
	})
	t.Run("记数先", func(t *testing.T) {
		stores := newStockStores(t)
		stores.seedShelf(t)
		if first := stores.register(t, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil)); first.Outcome() != application.GoodsOutcomeRegistered {
			t.Fatalf("首次 = %s", first.Outcome())
		}
		observer := newBlockedSessionObserver(t, stores.pool)
		inserted := make(chan struct{})
		release := make(chan struct{})
		var posted stockPostOutcome
		var registered stockRegisterOutcome
		var wait sync.WaitGroup
		receipt := stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-FIRST",
			domain.NewStockLineRequest(domain.StockIn, 1, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))
		change := goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
			spec.TrackBatch = true
		})
		wait.Add(1)
		go func() {
			defer wait.Done()
			posted.err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				var postErr error
				posted.result, postErr = stores.poster().Post(txCtx, application.PostStockMovementCommand{
					Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Request: receipt,
				})
				close(inserted)
				if postErr == nil {
					<-release
				}
				return postErr
			})
		}()
		<-inserted
		wait.Add(1)
		go func() {
			defer wait.Done()
			registered.err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
				var registerErr error
				registered.result, registerErr = stores.registrar().Register(txCtx, change)
				return registerErr
			})
		}()
		blocked := observer.waitForBlockedSession(t.Context(), 10*time.Second)
		close(release)
		wait.Wait()
		if !blocked {
			t.Error("改跟踪属性没有卡在货主商品锁上")
		}
		if posted.err != nil || registered.err != nil {
			t.Fatalf("错误 %v %v", posted.err, registered.err)
		}
		if posted.result.Outcome() != application.PostStockPosted || registered.result.Outcome() != application.GoodsOutcomeImmutableField ||
			registered.result.ImmutableCell().Field() != domain.GoodsImmutableTrackBatch {
			t.Fatalf("记数 %s，修订 %s 指出 %s", posted.result.Outcome(), registered.result.Outcome(), registered.result.ImmutableCell().Field())
		}
		if bad := trackedGoodsWithUnbatchedStock(t, stores); bad != 0 {
			t.Fatalf("跟踪批号与不带批号的库存同时成立的商品 %d", bad)
		}
	})
}

// trackedGoodsWithUnbatchedStock 数「当前修订已改成跟踪批号，同时账上有记入行没带批号」的商品。两种先后都该是零。
func trackedGoodsWithUnbatchedStock(t *testing.T, stores stockStores) int {
	t.Helper()
	var count int
	err := stores.pool.QueryRow(t.Context(),
		`SELECT count(*)
		   FROM warehouse_fulfillment.goods_revision goods
		  WHERE goods.track_batch
		    AND goods.revision = (
		        SELECT max(latest.revision)
		          FROM warehouse_fulfillment.goods_revision latest
		         WHERE latest.tenant_id = goods.tenant_id
		           AND latest.owner_account_id = goods.owner_account_id
		           AND latest.goods_code = goods.goods_code
		    )
		    AND EXISTS (
		        SELECT 1
		          FROM warehouse_fulfillment.stock_movement_line line
		         WHERE line.tenant_id = goods.tenant_id
		           AND line.owner_account_id = goods.owner_account_id
		           AND line.goods_code = goods.goods_code
		           AND line.batch_number IS NULL
		           AND line.quantity_delta > 0
		    )`).Scan(&count)
	if err != nil {
		t.Fatalf("核对跟踪批号与不带批号的库存：%v", err)
	}
	return count
}
