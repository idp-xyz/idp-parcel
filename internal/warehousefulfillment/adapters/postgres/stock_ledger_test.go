package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func expectPostedMovement(t *testing.T, result application.PostStockMovementResult) domain.StockMovement {
	t.Helper()
	if result.Outcome() != application.PostStockPosted {
		t.Fatalf("结果 = %s，不合 %v，want POSTED", result.Outcome(), result.Violations())
	}
	movement, ok := result.Movement()
	if !ok {
		t.Fatal("POSTED 没有流水")
	}
	return movement
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——余额由流水行派生。带贴标别名、批号、效期的格与不带的格是两格，各算各的余额。
func TestStockLedgerRoundTripKeepsALabeledCellApartFromABareCell(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-BARE", nil)
	stores.saveGoods(t, "SYN-GOODS-LABELED", "REQ-LABELED", func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
		spec.TrackExpiry = true
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-01"), domain.GoodsAliasPlatformCode,
			valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"), domain.AliasValidity{},
		)}
	})
	bare := stockCell(t, "SYN-GOODS-01", nil)
	labeled := stockCell(t, "SYN-GOODS-LABELED", func(spec *domain.StockCellSpec) {
		spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-01")
		spec.BatchNumber = valueOfBatch(t, "SYN-LOT-01")
		spec.Expiry = valueOfExpiry(t, "2027-01-31")
	})
	request := stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-IN",
		domain.NewStockLineRequest(domain.StockIn, 5, bare, time.Time{}),
		domain.NewStockLineRequest(domain.StockIn, 3, labeled, time.Time{}),
	)
	posted := expectPostedMovement(t, stores.post(t, request))

	var loaded domain.StockMovement
	var found bool
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var loadErr error
		loaded, found, loadErr = stores.ledger.LoadMovementBySource(txCtx, posted.Tenant(), posted.Reason(), posted.Source())
		return loadErr
	})
	if err != nil || !found || !loaded.Request().SameAs(request) {
		t.Fatalf("读回的请求相同 = %v，找到 %v，err %v", found && loaded.Request().SameAs(request), found, err)
	}
	for _, line := range loaded.Lines() {
		batch, ok, loadErr := loadBatch(t, stores, line.Cell().InboundBatch())
		if loadErr != nil || !ok {
			t.Fatalf("入库批次 %s：%v %v", line.Cell().InboundBatch(), ok, loadErr)
		}
		if batch.Goods() != line.Cell().Goods() || batch.BatchNumber() != line.Cell().BatchNumber() || batch.Expiry() != line.Cell().Expiry() || !batch.InboundAt().Equal(posted.PostedAt()) {
			t.Fatalf("批次与行不一致：%s %s %s %s", batch.Goods(), batch.BatchNumber(), batch.Expiry(), batch.InboundAt())
		}
		balance, balErr := balanceOf(t, stores, line.Cell())
		if balErr != nil || balance != line.Quantity() {
			t.Fatalf("格 %s 余额 %d（%v），want %d", line.Cell().Goods(), balance, balErr, line.Quantity())
		}
		onHand, handErr := onHand(t, stores, line.Cell().Goods().String())
		if handErr != nil || !onHand {
			t.Fatalf("有库存 = %v（%v）", onHand, handErr)
		}
	}
	rows, err := stores.ledger.ListOnHandCells(t.Context(), posted.Tenant(), posted.Owner(), posted.Warehouse())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].GoodsCode != "SYN-GOODS-01" || rows[0].Quantity != 5 || rows[0].Alias != nil || rows[0].BatchNumber != nil ||
		rows[1].GoodsCode != "SYN-GOODS-LABELED" || rows[1].Quantity != 3 || rows[1].Alias == nil || *rows[1].Alias != "SYN-FNSKU-01" ||
		rows[1].BatchNumber == nil || *rows[1].BatchNumber != "SYN-LOT-01" || rows[1].Expiry == nil || *rows[1].Expiry != "2027-01-31" {
		t.Fatalf("在库各格 = %+v", rows)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——已入账的流水不改不删。全部取走之后没有余额为正的格，流水行都还在。
func TestStockLedgerKeepsLinesAfterTheBalanceIsTaken(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-1", nil)
	posted := expectPostedMovement(t, stores.post(t, stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-IN",
		domain.NewStockLineRequest(domain.StockIn, 4, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))))
	cell := posted.Lines()[0].Cell()
	taken := stores.post(t, stockRequest(t, domain.StockReasonOutboundConfirmation, "SYN-FACT-OUT",
		domain.NewStockLineRequest(domain.StockOut, 4, cell, time.Time{})))
	if taken.Outcome() != application.PostStockPosted {
		t.Fatalf("取走 = %s %v", taken.Outcome(), taken.Violations())
	}
	onHand, err := onHand(t, stores, "SYN-GOODS-01")
	if err != nil || onHand {
		t.Fatalf("全部取走之后有库存 = %v（%v）", onHand, err)
	}
	rows, err := stores.ledger.ListOnHandCells(t.Context(), posted.Tenant(), posted.Owner(), posted.Warehouse())
	if err != nil || len(rows) != 0 {
		t.Fatalf("在库各格 = %+v（%v）", rows, err)
	}
	if lines := countStockRows(t, stores.pool, "stock_movement_line"); lines != 2 {
		t.Fatalf("流水行 %d，want 记入与取走都还在", lines)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——落册答已在册或余额不足时不留下任何行，调用方的事务仍可提交。
func TestStockAppendRefusalsLeaveNoRows(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-1", nil)
	posted := expectPostedMovement(t, stores.post(t, stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-IN",
		domain.NewStockLineRequest(domain.StockIn, 4, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))))
	before := stockCountsOf(t, stores)
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, appendErr := stores.ledger.AppendMovement(txCtx, posted, nil)
		if appendErr != nil {
			return appendErr
		}
		if outcome != ports.StockAlreadyPosted {
			return errors.New("同一判重位置没有答已在册")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	over, err := domain.NewStockMovement(
		posted.Tenant(), valueOf(t, domain.NewStockMovementID, "SYN-MOVE-OVER"), posted.Owner(), posted.Warehouse(),
		domain.StockReasonOutboundConfirmation, valueOf(t, domain.NewSourceFactReference, "SYN-FACT-OVER"),
		baseInstant, baseInstant,
		[]domain.StockMovementLine{domain.NewStockMovementLine(domain.StockOut, 100, posted.Lines()[0].Cell(), time.Time{}, false)},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, appendErr := stores.ledger.AppendMovement(txCtx, over, nil)
		if appendErr != nil {
			return appendErr
		}
		if outcome != ports.StockInsufficientBalance {
			return errors.New("超额取走没有答余额不足")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if after := stockCountsOf(t, stores); after != before {
		t.Fatalf("拒绝之后行数 = %v，之前 %v", after, before)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——串行、落册与「有没有在库库存」都要环境事务，否则锁保不住。
func TestStockLedgerRefusesWorkOutsideATransaction(t *testing.T) {
	stores := newStockStores(t)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	if err := stores.ledger.SerializePosting(ctx, tenant, owner, warehouse); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 SerializePosting = %v", err)
	}
	if _, err := stores.ledger.AppendMovement(ctx, domain.StockMovement{}, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 AppendMovement = %v", err)
	}
	if _, err := stores.ledger.HasOnHandStock(ctx, tenant, owner, valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 HasOnHandStock = %v", err)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——库上拒数量为零、不认识的原因、不认识的库存状态。绕过写入侧直接插，证的是约束而不是应用层。
func TestStockLedgerChecksRejectIllegalRows(t *testing.T) {
	stores := newStockStores(t)
	ctx := t.Context()
	if _, err := stores.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_movement
		    (tenant_id, movement_id, owner_account_id, warehouse_code, reason, source_reference, occurred_at, posted_at)
		 VALUES ('SYN-TENANT-01', 'SYN-MOVE-OK', 'SYN-OWNER-01', 'SYN-WH-01', 'RECEIPT', 'SYN-FACT-OK', now(), now())`); err != nil {
		t.Fatalf("合法流水头：%v", err)
	}
	if _, err := stores.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.inbound_batch
		    (tenant_id, inbound_batch_id, owner_account_id, warehouse_code, goods_code, inbound_at, formed_by_movement_id)
		 VALUES ('SYN-TENANT-01', 'SYN-BATCH-OK', 'SYN-OWNER-01', 'SYN-WH-01', 'SYN-GOODS-01', now(), 'SYN-MOVE-OK')`); err != nil {
		t.Fatalf("合法入库批次：%v", err)
	}
	line := `INSERT INTO warehouse_fulfillment.stock_movement_line (
		    tenant_id, movement_id, position, owner_account_id, warehouse_code, location_code, goods_code, unit_name,
		    stock_state, inbound_batch_id, quantity_delta, formed_inbound_batch)
		 VALUES ('SYN-TENANT-01', 'SYN-MOVE-OK', $1, 'SYN-OWNER-01', 'SYN-WH-01', 'SYN-LOC-A', 'SYN-GOODS-01', '件', $2, 'SYN-BATCH-OK', $3, true)`
	if _, err := stores.pool.Exec(ctx, line, 0, "GOOD", int64(0)); pgCode(err) != "23514" {
		t.Fatalf("数量为零 = %v", err)
	}
	if _, err := stores.pool.Exec(ctx, line, 1, "BROKEN", int64(1)); pgCode(err) != "23514" {
		t.Fatalf("不认识的库存状态 = %v", err)
	}
	if _, err := stores.pool.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_movement
		    (tenant_id, movement_id, owner_account_id, warehouse_code, reason, source_reference, occurred_at, posted_at)
		 VALUES ('SYN-TENANT-01', 'SYN-MOVE-BAD', 'SYN-OWNER-01', 'SYN-WH-01', 'NOPE', 'SYN-FACT-BAD', now(), now())`); pgCode(err) != "23514" {
		t.Fatalf("不认识的原因 = %v", err)
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——流水标识与入库批次标识前缀不同，连续两次不同，并且过得了领域构造器。
func TestStockIdentitiesMintDistinctPrefixedIdentifiers(t *testing.T) {
	identities, err := whidentity.NewStockIdentities()
	if err != nil {
		t.Fatal(err)
	}
	first, err := identities.NextStockMovementID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.NextStockMovementID()
	if err != nil {
		t.Fatal(err)
	}
	batch, err := identities.NextInboundBatchID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first.String(), "SMV-") || !strings.HasPrefix(second.String(), "SMV-") || !strings.HasPrefix(batch.String(), "IBT-") {
		t.Fatalf("流水 %s %s，批次 %s", first, second, batch)
	}
}

// Covers: AT-WF-016（库存账的那一半）——商品有在库库存时改是否跟踪批号不成立并指出那一格；没有在库库存，或曾经有过、已经全部取走，改成立。
func TestStockRegisterGoodsRefusesTrackingChangesWhileStockIsOnHand(t *testing.T) {
	stores := newStockStores(t)
	stores.seedShelf(t)
	first := stores.register(t, goodsCommand(t, "SYN-GOODS-01", "REQ-1", domain.BasisRevision{}, nil))
	if first.Outcome() != application.GoodsOutcomeRegistered {
		t.Fatalf("首次登记 = %s", first.Outcome())
	}
	change := goodsCommand(t, "SYN-GOODS-01", "REQ-2", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
	})
	without := stores.register(t, change)
	if without.Outcome() != application.GoodsOutcomeRegistered {
		t.Fatalf("没有库存时改跟踪批号 = %s", without.Outcome())
	}

	stores = newStockStores(t)
	stores.seedShelf(t)
	stores.saveGoods(t, "SYN-GOODS-01", "REQ-1", nil)
	posted := expectPostedMovement(t, stores.post(t, stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-IN",
		domain.NewStockLineRequest(domain.StockIn, 2, stockCell(t, "SYN-GOODS-01", nil), time.Time{}))))
	blocked := stores.register(t, change)
	if blocked.Outcome() != application.GoodsOutcomeImmutableField || blocked.ImmutableCell().Field() != domain.GoodsImmutableTrackBatch {
		t.Fatalf("有库存时 = %s 指出 %s", blocked.Outcome(), blocked.ImmutableCell().Field())
	}
	taken := stores.post(t, stockRequest(t, domain.StockReasonOutboundConfirmation, "SYN-FACT-OUT",
		domain.NewStockLineRequest(domain.StockOut, 2, posted.Lines()[0].Cell(), time.Time{})))
	if taken.Outcome() != application.PostStockPosted {
		t.Fatalf("取走 = %s %v", taken.Outcome(), taken.Violations())
	}
	after := stores.register(t, goodsCommand(t, "SYN-GOODS-01", "REQ-3", domain.NewBasisRevision(1), func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
	}))
	if after.Outcome() != application.GoodsOutcomeRegistered {
		t.Fatalf("全部取走之后改跟踪批号 = %s %s", after.Outcome(), after.ImmutableCell().Field())
	}
}

func valueOfBatch(t *testing.T, raw string) domain.BatchNumber {
	t.Helper()
	number, err := domain.NewBatchNumber(raw)
	if err != nil {
		t.Fatal(err)
	}
	return number
}

func valueOfExpiry(t *testing.T, raw string) domain.ExpiryDate {
	t.Helper()
	date, err := domain.NewExpiryDate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func loadBatch(t *testing.T, stores stockStores, id domain.InboundBatchID) (domain.InboundBatch, bool, error) {
	t.Helper()
	var batch domain.InboundBatch
	var found bool
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var loadErr error
		batch, found, loadErr = stores.ledger.LoadInboundBatch(txCtx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), id)
		return loadErr
	})
	return batch, found, err
}

func balanceOf(t *testing.T, stores stockStores, cell domain.StockCell) (int64, error) {
	t.Helper()
	return stores.ledger.CellBalance(t.Context(),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		cell)
}

func onHand(t *testing.T, stores stockStores, goods string) (bool, error) {
	t.Helper()
	var onHand bool
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var handErr error
		onHand, handErr = stores.ledger.HasOnHandStock(txCtx,
			valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
			valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
			valueOf(t, domain.NewGoodsCode, goods))
		return handErr
	})
	return onHand, err
}

type stockCounts struct {
	movements int
	batches   int
	lines     int
}

func stockCountsOf(t *testing.T, stores stockStores) stockCounts {
	t.Helper()
	movements, batches, lines := stockRowCounts(t, stores.pool)
	return stockCounts{movements: movements, batches: batches, lines: lines}
}
