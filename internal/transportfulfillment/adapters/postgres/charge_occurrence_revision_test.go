package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真库证发生项修订入口要的几样（ADR-0232 决定四、五；迁移 TF 0048）：修订者随修订版往返；一版至多被修订一次；按范围列链头
// 不认被替代与失效的版本；范围锁不在事务里取由实现拒。

// invalidatedRecord 让 first 那一版失效，交回 V2 的落册记录。
func invalidatedRecord(t *testing.T, first ports.ChargeOccurrenceRecord, basis string) ports.ChargeOccurrenceRecord {
	t.Helper()
	invalidated, err := domain.InvalidateChargeOccurrence(first.Occurrence,
		occurrenceRef(t, domain.NewOccurrenceBasisReference, basis), occurredAtFixture.Add(24*time.Hour),
		occurrenceRef(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01"))
	if err != nil {
		t.Fatal(err)
	}
	return ports.ChargeOccurrenceRecord{
		Key:        ports.ChargeOccurrenceKey{TenantID: first.Key.TenantID, Occurrence: first.Key.Occurrence, Validity: invalidated.Validity()},
		Occurrence: invalidated,
		RecordedAt: occurredAtFixture.Add(24 * time.Hour),
	}
}

func TestAnInvalidationRoundTripsWithItsReviserAndAVersionIsRevisedOnlyOnce(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()
	first := occurrenceRecord(t, "OCC-REV-01", "V1", "parcel-1")
	mustSaveOccurrence(t, transactor, ctx, repository, first)
	second := invalidatedRecord(t, first, "SYN-EVIDENCE-01")
	mustSaveOccurrence(t, transactor, ctx, repository, second)

	read, found, err := repository.FindByKey(ctx, second.Key)
	if err != nil || !found {
		t.Fatalf("失效版读不回：%v %v", found, err)
	}
	reviser, present := read.Occurrence.RevisedBy()
	if !present || reviser.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" || !read.Occurrence.Invalidated() {
		t.Fatalf("修订者没往返：%+v", read.Occurrence)
	}

	// 另起一个号回指同一版：库面的「一版至多被修订一次」挡下，答已登记。
	fork, err := first.Occurrence.ReviseValidity(domain.OccurrenceSuperseded,
		occurrenceRef(t, domain.NewOccurrenceValidityVersion, "V9"),
		occurrenceRef(t, domain.NewOccurrenceBasisReference, "CORRECTION/fork"), occurredAtFixture.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var outcome ports.ChargeOccurrenceSaveOutcome
	mustWithinOccurrenceTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = repository.Save(txCtx, ports.ChargeOccurrenceRecord{
			Key:        ports.ChargeOccurrenceKey{TenantID: first.Key.TenantID, Occurrence: first.Key.Occurrence, Validity: fork.Validity()},
			Occurrence: fork,
			RecordedAt: occurredAtFixture,
		})
		return saveErr
	})
	if outcome != ports.ChargeOccurrenceAlreadyRegistered {
		t.Fatalf("分叉修订 = %d, want 已登记", outcome)
	}
}

func TestListingAScopeReturnsOnlyTheLiveHeads(t *testing.T) {
	repository, transactor, _ := newChargeOccurrences(t)
	ctx := t.Context()
	live := occurrenceRecord(t, "OCC-LIVE-01", "V1", "parcel-1")
	gone := occurrenceRecord(t, "OCC-GONE-01", "V1", "parcel-2")
	mustSaveOccurrence(t, transactor, ctx, repository, live)
	mustSaveOccurrence(t, transactor, ctx, repository, gone)
	mustSaveOccurrence(t, transactor, ctx, repository, invalidatedRecord(t, gone, "SYN-EVIDENCE-02"))

	heads, err := repository.ListCurrentOnScope(ctx, live.Key.TenantID, live.Occurrence.Reason(), live.Occurrence.Scope())
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 1 || heads[0].Key != live.Key {
		t.Fatalf("链头 = %+v, want 只有仍有效的那一项", heads)
	}
	other, err := repository.ListCurrentOnScope(ctx, live.Key.TenantID, domain.ActualFulfillmentOccurrence, live.Occurrence.Scope())
	if err != nil || len(other) != 0 {
		t.Fatalf("另一个原因上列出了 %d 项，%v", len(other), err)
	}
}

func TestTheScopeHoldRefusesToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newChargeOccurrences(t)
	err := repository.HoldOccurrenceScope(t.Context(), occurrenceRef(t, domain.NewTenantID, "tenant-1"),
		domain.FailedAttemptOccurrence, occurrenceRef(t, domain.NewOccurrenceScopeReference, "scope-1"))
	// 锁随事务释放：事务外取的锁在语句结束即放，等于没取。
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务取范围锁应返回 ErrTransactionRequired，实得：%v", err)
	}
}
