package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证 UC-SA-003 步 7 调整半边的库面：一笔由唯一创建用例登进
// 册的调整，经**只读口** ports.ChargeAdjustmentView 读回后能直接交给
// IncludeAdjustmentInSubsequentPeriod 形成纳入并落库，纳入行回指该调整；只读口与册是同一
// 个类型、同一份行模型，读回的调整与登进去的一字不差。

// Covers: `AT-SA-076`——UC-SA-002 形成的计价纠错借项经读口取回、纳入后续账期并关联原账单；
// 纳入行经 SubsequentInclusions 往返后仍指回那笔调整。
func TestAnAdjustmentReadThroughTheViewIsIncludedInASubsequentPeriod(t *testing.T) {
	adjustments, transactor, _ := newChargeAdjustments(t)
	// 两个夹具各自一份 DB，事务归属也各自一份：纳入用它自己那份事务写，混用会被写入
	// 执行器的归属检查拒掉——那道拒是框架在守，不是这里要证的东西。
	inclusions, _, _, _, inclusionTransactor, _ := newCloseoutStores(t)
	ctx := t.Context()

	key := adjustmentKey(t, "tenant-a", "adj-view-1")
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := adjustments.Save(txCtx, ports.ChargeAdjustmentRecord{
			Key:        key,
			Adjustment: correctionAdjustment(t, "adj-view-1", "charge-1", adjustmentFormedAt),
			RecordedAt: adjustmentFormedAt.Add(time.Minute),
		})
		return err
	})

	// 截单编排拿到的是这个接口，不是册：这里就按接口读，读得回来才算缝接上了。
	var view ports.ChargeAdjustmentView = adjustments
	record, present, err := view.FindByKey(ctx, key)
	if err != nil || !present {
		t.Fatalf("经只读口找回调整：err=%v present=%v", err, present)
	}
	if record.Adjustment.ID() != key.Adjustment || record.Adjustment.Charge().String() != "charge-1" {
		t.Fatalf("只读口读回的不是登进去的那笔：%#v", record.Adjustment)
	}

	statement := statementRecord(t, "tenant-a", "STMT-VIEW-2026-09", "digest-view").Statement
	inclusion, err := domain.IncludeAdjustmentInSubsequentPeriod(
		statement,
		record.Adjustment,
		saValue(t, domain.NewInclusionReference, "inclusion-view-1"),
		saValue(t, domain.NewBillingPeriodReference, "2026-10"),
		inclusionAt,
	)
	if err != nil {
		t.Fatalf("以读回的调整形成纳入：%v", err)
	}
	inclusionRecord := ports.InclusionRecord{
		Key:           ports.InclusionKey{TenantID: key.TenantID, Inclusion: inclusion.Inclusion()},
		ContentDigest: "digest-inclusion-view-1",
		Inclusion:     inclusion,
		RecordedAt:    inclusionAt,
	}
	var saved ports.InclusionSaveOutcome
	saWithin(t, inclusionTransactor, ctx, func(txCtx context.Context) error {
		var err error
		saved, err = inclusions.Save(txCtx, inclusionRecord)
		return err
	})
	if saved != ports.InclusionSaved {
		t.Fatalf("save outcome = %d, want InclusionSaved", saved)
	}

	found, present, err := inclusions.FindByKey(ctx, inclusionRecord.Key)
	if err != nil || !present {
		t.Fatalf("读回纳入：err=%v present=%v", err, present)
	}
	adjustmentID, hasAdjustment := found.Inclusion.Adjustment()
	if found.Inclusion.Kind() != domain.IncludedAdjustment || !hasAdjustment || adjustmentID != key.Adjustment {
		t.Fatalf("纳入行没有指回那笔调整：kind=%s adjustment=%s/%v", found.Inclusion.Kind(), adjustmentID, hasAdjustment)
	}
	if found.Inclusion.Statement() != statement.Number() ||
		found.Inclusion.OriginalPeriod() != statement.Period() ||
		found.Inclusion.SubsequentPeriod().String() != "2026-10" {
		t.Fatalf("原账单、原周期与后续周期没有一起往返：%+v", found.Inclusion)
	}
}
