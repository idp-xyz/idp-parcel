package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真库证发生项修订驱动的纠错要的几样（迁移 SA 0044，ADR-0232 决定七、八）：失效纠错版本带着 withdrawn 往返；一版至多被纠正一次；
// 成本状态在锁下交链头与已失效的那几版；两口不在事务里调用由实现拒。

func withdrawnOccurrence(t *testing.T, id string) domain.TransportChargeOccurrence {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		saValue(t, domain.NewChargeOccurrenceID, id),
		saValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_LEG"),
		saValue(t, domain.NewOccurrenceVersion, "occurrence-version-2"),
		expectedCostOccurredAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}

func TestAWithdrawalVersionRoundTripsAndAVersionIsCorrectedOnlyOnce(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	tenant := saTenant(t, "tenant-1")
	first := formedExpectedCost(t, "cost-v1", "occurrence-1")
	mustSaveExpectedCost(t, transactor, repository, tenant, first)
	withdrawn, err := first.Withdraw(withdrawnOccurrence(t, "occurrence-1"))
	if err != nil {
		t.Fatal(err)
	}
	mustSaveExpectedCost(t, transactor, repository, tenant, withdrawn)

	read, found, err := repository.LoadExpectedCost(t.Context(), tenant, withdrawn.Version())
	if err != nil || !found || !read.Withdrawn() {
		t.Fatalf("失效纠错版本读回 = %v %v withdrawn=%v", found, err, read.Withdrawn())
	}

	// 另一版也回指首版：「一版至多被纠正一次」挡下，答已登记。
	fork, err := first.AppendCorrection(domain.CostCorrectionSpec{
		Version: saValue(t, domain.NewSupplierCostVersionID, "cost-fork"), Occurrence: withdrawnOccurrence(t, "occurrence-1"),
		RuleVersion: first.RuleVersion(), Agreement: first.Agreement(), Evaluation: first.Evaluation(),
		OriginalCurrency: saValue(t, domain.NewCurrencyCode, "USD"), OriginalMinor: 4200, SettlementMinor: 4200,
		Reason: domain.OccurrenceSupersededCorrection(),
	})
	if err != nil {
		t.Fatal(err)
	}
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, tenant, fork, expectedCostRecordedAt)
		if outcome != ports.ExpectedCostAlreadyRecorded {
			t.Errorf("分叉纠错 = %s, want ALREADY_RECORDED", outcome)
		}
		return err
	})
}

func TestTheOccurrenceCostStateListsHeadsAndInvalidatedVersionsUnderItsLock(t *testing.T) {
	repository, transactor := newExpectedCosts(t)
	tenant := saTenant(t, "tenant-1")
	first := formedExpectedCost(t, "cost-v1", "occurrence-1")
	mustSaveExpectedCost(t, transactor, repository, tenant, first)
	withdrawn, err := first.Withdraw(withdrawnOccurrence(t, "occurrence-1"))
	if err != nil {
		t.Fatal(err)
	}
	mustSaveExpectedCost(t, transactor, repository, tenant, withdrawn)
	mustSaveExpectedCost(t, transactor, repository, tenant, formedExpectedCost(t, "cost-other", "occurrence-2"))

	var state ports.OccurrenceCostState
	saWithin(t, transactor, t.Context(), func(txCtx context.Context) error {
		if err := repository.RecordOccurrenceInvalidation(txCtx, tenant, first.Occurrence().ID(), first.Occurrence().Version(),
			withdrawn.Occurrence().Version(), expectedCostRecordedAt); err != nil {
			return err
		}
		// 重记落回同一行。
		if err := repository.RecordOccurrenceInvalidation(txCtx, tenant, first.Occurrence().ID(), first.Occurrence().Version(),
			withdrawn.Occurrence().Version(), expectedCostRecordedAt); err != nil {
			return err
		}
		var err error
		state, err = repository.LoadOccurrenceCostState(txCtx, tenant, first.Occurrence().ID())
		return err
	})
	if len(state.Heads) != 1 || state.Heads[0].Version() != withdrawn.Version() || !state.Heads[0].Withdrawn() {
		t.Fatalf("链头 = %+v, want 只有失效纠错那一版", state.Heads)
	}
	if !state.Invalidates(first.Occurrence().Version()) || len(state.Invalidated) != 1 {
		t.Fatalf("已失效 = %v", state.Invalidated)
	}
}

func TestTheOccurrenceCostStateAndInvalidationRecordRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newExpectedCosts(t)
	tenant := saTenant(t, "tenant-1")
	occurrence := saValue(t, domain.NewChargeOccurrenceID, "occurrence-1")
	// 锁随事务释放：事务外取的锁在语句结束即放，等于没取。
	if _, err := repository.LoadOccurrenceCostState(t.Context(), tenant, occurrence); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务读成本状态应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := repository.RecordOccurrenceInvalidation(t.Context(), tenant, occurrence,
		saValue(t, domain.NewOccurrenceVersion, "v1"), saValue(t, domain.NewOccurrenceVersion, "v2"), expectedCostRecordedAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记失效应返回 ErrTransactionRequired，实得：%v", err)
	}
}
