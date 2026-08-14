package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	allocatedAt = time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC)
	derivedAsOf = time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC)
)

func TestACostAllocationRoundTripsAndReplaceRewritesVersionNotSource(t *testing.T) {
	allocations, _, transactor, _ := newOperatingStores(t)
	ctx := t.Context()

	record := formedAllocationRecord(t, "tenant-a", "allocation-1")
	var savedAllocation ports.AllocationSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedAllocation, err = allocations.Save(txCtx, record)
		return err
	})
	if savedAllocation != ports.AllocationSaved {
		t.Fatalf("save outcome = %d", savedAllocation)
	}

	found, exists, err := allocations.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Allocation.UnallocatedMinor() != 1 || len(found.Allocation.Portions()) != 2 {
		t.Fatalf("分摊往返变形：unallocated=%d portions=%d", found.Allocation.UnallocatedMinor(), len(found.Allocation.Portions()))
	}

	reallocated, err := found.Allocation.Reallocate(
		saValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule/v2"),
		[]domain.AllocationPortion{{
			Target:      saValue(t, domain.NewAllocationTargetReference, "parcel-1"),
			AmountMinor: 10000,
		}},
		saValue(t, domain.NewAllocationVersion, "allocation/v2"),
		allocatedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("reallocate: %v", err)
	}
	replaced := found
	replaced.Allocation = reallocated
	replaced.ContentDigest = "digest-reallocated"
	replaced.RecordedAt = allocatedAt.Add(time.Hour)
	var allocationReplaced bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		allocationReplaced, err = allocations.Replace(txCtx, replaced)
		return err
	})
	if !allocationReplaced {
		t.Fatal("Replace 答 false")
	}

	after, _, err := allocations.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("重分摊后读回：%v", err)
	}
	predecessor, ok := after.Allocation.Corrects()
	if !ok || predecessor.String() != "allocation/v1" || after.Allocation.UnallocatedMinor() != 0 {
		t.Fatal("版本链或份额没有落库")
	}
	_, source := after.Allocation.SourceAmount()
	if source != 10000 {
		t.Fatal("Replace 改写了来源金额")
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.AllocationSaveOutcome
	var allocationWinner ports.AllocationRecord
	var allocationWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = allocations.Save(txCtx, second); err != nil {
			return err
		}
		allocationWinner, allocationWinnerFound, err = allocations.FindByKey(txCtx, record.Key)
		return err
	})
	if !allocationWinnerFound || allocationWinner.ContentDigest != "digest-reallocated" {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q",
			allocationWinnerFound, allocationWinner.ContentDigest)
	}
	if outcome != ports.AllocationAlreadyFormed {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestAnOperatingResultRoundTripsAndReplaceRewritesVersionNotScope(t *testing.T) {
	_, results, transactor, _ := newOperatingStores(t)
	ctx := t.Context()

	record := derivedResultRecord(t, "tenant-a", domain.ConfirmedBasis)
	var savedResult ports.OperatingResultSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedResult, err = results.Save(txCtx, record)
		return err
	})
	if savedResult != ports.OperatingResultSaved {
		t.Fatalf("save outcome = %d", savedResult)
	}

	found, exists, err := results.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if _, margin := found.Result.Margin(); margin != 7000 {
		t.Fatalf("margin = %d, want 7000", margin)
	}

	rederived, err := found.Result.Rederive(
		[]domain.ResultComponent{{
			Source:      saValue(t, domain.NewComponentSourceReference, "charge-2"),
			Effect:      domain.IncreasesResult,
			AmountMinor: 5000,
		}},
		saValue(t, domain.NewOperatingResultVersion, "result/v2"),
		derivedAsOf.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("rederive: %v", err)
	}
	replaced := found
	replaced.Result = rederived
	replaced.ContentDigest = "digest-rederived"
	replaced.RecordedAt = derivedAsOf.Add(24 * time.Hour)
	var resultReplaced bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		resultReplaced, err = results.Replace(txCtx, replaced)
		return err
	})
	if !resultReplaced {
		t.Fatal("Replace 答 false")
	}

	after, _, err := results.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("重派生后读回：%v", err)
	}
	predecessor, ok := after.Result.Corrects()
	if !ok || predecessor.String() != "result/v1" {
		t.Fatal("版本链没有落库")
	}
	if after.Result.Basis() != domain.ConfirmedBasis {
		t.Fatal("Replace 改写了口径")
	}

	missing := derivedResultRecord(t, "tenant-a", domain.EstimatedBasis)
	var missingReplaced bool
	var missingErr error
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		missingReplaced, missingErr = results.Replace(txCtx, missing)
		return nil
	})
	if missingErr == nil && missingReplaced {
		t.Fatal("没有可转换的快照却答 true")
	}
}

func TestOperatingRecordsAreInvisibleAcrossTenants(t *testing.T) {
	allocations, results, transactor, _ := newOperatingStores(t)
	ctx := t.Context()

	allocation := formedAllocationRecord(t, "tenant-a", "allocation-shared")
	result := derivedResultRecord(t, "tenant-a", domain.ConfirmedBasis)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := allocations.Save(txCtx, allocation); err != nil {
			return err
		}
		_, err := results.Save(txCtx, result)
		return err
	})

	other := saTenant(t, "tenant-b")
	if _, exists, err := allocations.FindByKey(ctx, ports.AllocationKey{TenantID: other, Allocation: allocation.Key.Allocation}); err != nil || exists {
		t.Errorf("另一个租户读到了分摊：exists=%v err=%v", exists, err)
	}
	if _, exists, err := results.FindByKey(ctx, ports.OperatingResultKey{
		TenantID: other, Scope: result.Key.Scope, Period: result.Key.Period, Basis: result.Key.Basis,
	}); err != nil || exists {
		t.Errorf("另一个租户读到了经营结果：exists=%v err=%v", exists, err)
	}
}

func TestOperatingWritesRefuseToRunOutsideATransaction(t *testing.T) {
	allocations, results, _, _ := newOperatingStores(t)
	ctx := t.Context()

	allocation := formedAllocationRecord(t, "tenant-a", "allocation-1")
	if _, err := allocations.Save(ctx, allocation); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 分摊：%v", err)
	}
	if _, err := allocations.Replace(ctx, allocation); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Replace 分摊：%v", err)
	}
	result := derivedResultRecord(t, "tenant-a", domain.ConfirmedBasis)
	if _, err := results.Save(ctx, result); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 经营结果：%v", err)
	}
	if _, err := results.Replace(ctx, result); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Replace 经营结果：%v", err)
	}
}

func TestOperatingRollbackLeavesNothingBehind(t *testing.T) {
	allocations, results, transactor, _ := newOperatingStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	allocation := formedAllocationRecord(t, "tenant-a", "allocation-1")
	result := derivedResultRecord(t, "tenant-a", domain.ConfirmedBasis)

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := allocations.Save(txCtx, allocation); err != nil {
			return err
		}
		if _, err := results.Save(txCtx, result); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := allocations.FindByKey(ctx, allocation.Key); err != nil || exists {
		t.Errorf("回滚后分摊仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := results.FindByKey(ctx, result.Key); err != nil || exists {
		t.Errorf("回滚后经营结果仍在：exists=%v err=%v", exists, err)
	}
}

func TestOperatingCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, pool := newOperatingStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.cost_allocation
			(tenant_id, allocation_id, source_ref, source_minor, currency, rule_ref,
			 portions, unallocated_minor, version, allocated_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-1', 'src', 100, 'USD', 'rule', '[]', 200, 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「未分摊超过来源」溜进了分摊库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.cost_allocation
			(tenant_id, allocation_id, source_ref, source_minor, currency, rule_ref,
			 portions, unallocated_minor, version, allocated_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-2', 'src', 100, 'USD', 'rule', NULL, 0, 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「份额列为 NULL」按 jsonb 三值缝溜进了分摊库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.operating_result
			(tenant_id, scope_ref, period_ref, basis, currency, components, margin_minor,
			 version, as_of, content_digest, recorded_at)
		 VALUES ('tenant-a', 'scope-1', '2026-08', 'CONFIRMED', 'USD', '[]', 0, 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「空组成」溜进了经营结果库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.operating_result
			(tenant_id, scope_ref, period_ref, basis, currency, components, margin_minor,
			 version, as_of, corrects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'scope-1', '2026-08', 'CONFIRMED', 'USD',
		         '[{"source":"c1","effect":"INCREASES","amountMinor":1}]', 1, 'v1', now(), 'v1', 'd', now())`); err == nil {
		t.Fatal("一行「回指等于当前版本」溜进了经营结果库")
	}
}

func newOperatingStores(t *testing.T) (
	*adapter.CostAllocations,
	*adapter.OperatingResults,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	allocations, err := adapter.NewCostAllocations(db)
	if err != nil {
		t.Fatalf("构造分摊库：%v", err)
	}
	results, err := adapter.NewOperatingResults(db)
	if err != nil {
		t.Fatalf("构造经营结果库：%v", err)
	}
	return allocations, results, db.Transactor(), pool
}

func formedAllocationRecord(t *testing.T, tenant, id string) ports.AllocationRecord {
	t.Helper()
	allocation, err := domain.FormCostAllocation(domain.CostAllocationSpec{
		ID:          saValue(t, domain.NewAllocationID, id),
		Source:      saValue(t, domain.NewAllocationSourceReference, "payable-1"),
		SourceMinor: 10000,
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		Rule:        saValue(t, domain.NewAllocationRuleVersionReference, "allocation-rule/v1"),
		Portions: []domain.AllocationPortion{
			{Target: saValue(t, domain.NewAllocationTargetReference, "parcel-1"), AmountMinor: 6000},
			{Target: saValue(t, domain.NewAllocationTargetReference, "parcel-2"), AmountMinor: 3999},
		},
		Version:     saValue(t, domain.NewAllocationVersion, "allocation/v1"),
		AllocatedAt: allocatedAt,
	})
	if err != nil {
		t.Fatalf("构造分摊：%v", err)
	}
	return ports.AllocationRecord{
		Key:           ports.AllocationKey{TenantID: saTenant(t, tenant), Allocation: allocation.ID()},
		ContentDigest: "digest-" + id,
		Allocation:    allocation,
		RecordedAt:    allocatedAt,
	}
}

func derivedResultRecord(t *testing.T, tenant string, basis domain.OperatingBasis) ports.OperatingResultRecord {
	t.Helper()
	result, err := domain.DeriveOperatingResult(
		saValue(t, domain.NewOperatingScopeReference, "customer-1"),
		saValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
		basis,
		saValue(t, domain.NewCurrencyCode, "USD"),
		[]domain.ResultComponent{
			{Source: saValue(t, domain.NewComponentSourceReference, "charge-1"), Effect: domain.IncreasesResult, AmountMinor: 10000},
			{Source: saValue(t, domain.NewComponentSourceReference, "payable-1"), Effect: domain.DecreasesResult, AmountMinor: 3000},
		},
		saValue(t, domain.NewOperatingResultVersion, "result/v1"),
		derivedAsOf,
	)
	if err != nil {
		t.Fatalf("构造经营结果：%v", err)
	}
	return ports.OperatingResultRecord{
		Key: ports.OperatingResultKey{
			TenantID: saTenant(t, tenant),
			Scope:    result.Scope(),
			Period:   result.Period(),
			Basis:    result.Basis(),
		},
		ContentDigest: "digest-" + basis.String(),
		Result:        result,
		RecordedAt:    derivedAsOf,
	}
}
