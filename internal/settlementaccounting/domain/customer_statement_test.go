package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var (
	statementCutOffAt    = time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC)
	statementPublishedAt = time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)
)

func confirmedCharge(t *testing.T, id string, amountMinor int64) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 settlementValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            settlementValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:         settlementValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		OriginalCurrency:   settlementValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      amountMinor,
		SettlementCurrency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    amountMinor,
		Stage:              domain.ChargeProvisional,
		FormedAt:           statementCutOffAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form charge: %v", err)
	}
	confirmed, err := charge.Confirm(
		confirmationFacts(t),
		settlementValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-"+id),
		statementCutOffAt.Add(-48*time.Hour),
	)
	if err != nil {
		t.Fatalf("confirm charge: %v", err)
	}
	return confirmed
}

func estimatedStatementCharge(t *testing.T, id string) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 settlementValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            settlementValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:         settlementValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		OriginalCurrency:   settlementValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      5000,
		SettlementCurrency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    5000,
		Stage:              domain.ChargeEstimated,
		FormedAt:           statementCutOffAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form estimated charge: %v", err)
	}
	return charge
}

func creditAdjustment(t *testing.T, id, chargeID string, amountMinor int64) domain.ChargeAdjustment {
	t.Helper()
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:                 settlementValue(t, domain.NewChargeAdjustmentID, id),
		Charge:             settlementValue(t, domain.NewCustomerChargeID, chargeID),
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentCredit,
		Evaluation:         settlementValue(t, domain.NewSellEvaluationReference, "correction-evaluation-1"),
		OriginalCurrency:   settlementValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      amountMinor,
		SettlementCurrency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    amountMinor,
		FormedAt:           statementCutOffAt.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form adjustment: %v", err)
	}
	return adjustment
}

func cutDraft(t *testing.T) domain.StatementDraft {
	t.Helper()
	draft, err := domain.CutStatementDraft(domain.StatementDraftSpec{
		Account:  settlementValue(t, domain.NewSettlementAccountID, "account-1"),
		Period:   settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
		Version:  settlementValue(t, domain.NewStatementDraftVersion, "draft/v1"),
		Currency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		CutOffAt: statementCutOffAt,
		Charges: []domain.CustomerCharge{
			confirmedCharge(t, "charge-1", 10000),
			confirmedCharge(t, "charge-2", 6000),
		},
		Adjustments: []domain.ChargeAdjustment{
			creditAdjustment(t, "adjustment-1", "charge-1", 1000),
		},
	})
	if err != nil {
		t.Fatalf("cut draft: %v", err)
	}
	return draft
}

func publishedStatement(t *testing.T) domain.PublishedStatement {
	t.Helper()
	statement, err := domain.PublishStatement(cutDraft(t),
		settlementValue(t, domain.NewStatementNumber, "statement-2026-08-001"),
		15000, statementPublishedAt)
	if err != nil {
		t.Fatalf("publish statement: %v", err)
	}
	return statement
}

// Covers: `AT-SA-059`「确认费用进入草稿并形成可勾稽范围」、`AT-SA-060`「预估费用不进入
// 已发布对账单」与 `AT-SA-063`「发布前收到新确认费用 → 可重算草稿并保存新范围/版本」。
func TestACutOffDraftAdmitsOnlyConfirmedCharges(t *testing.T) {
	draft := cutDraft(t)
	// 计数锚定本夹具：10000+6000−贷 1000 = 15000（USD，截于 2026-08-31T16:00Z）。
	if draft.NetTotalMinor() != 15000 {
		t.Fatalf("net total = %d, want 15000", draft.NetTotalMinor())
	}

	t.Run("an estimated charge is refused", func(t *testing.T) {
		if _, err := domain.CutStatementDraft(domain.StatementDraftSpec{
			Account:  settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			Period:   settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
			Version:  settlementValue(t, domain.NewStatementDraftVersion, "draft/v1"),
			Currency: settlementValue(t, domain.NewCurrencyCode, "USD"),
			CutOffAt: statementCutOffAt,
			Charges:  []domain.CustomerCharge{estimatedStatementCharge(t, "charge-9")},
		}); !errors.Is(err, domain.ErrChargeNotConfirmed) {
			t.Fatalf("error = %v, want ErrChargeNotConfirmed", err)
		}
	})

	t.Run("an adjustment for a charge outside the draft is refused", func(t *testing.T) {
		if _, err := domain.CutStatementDraft(domain.StatementDraftSpec{
			Account:  settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			Period:   settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
			Version:  settlementValue(t, domain.NewStatementDraftVersion, "draft/v1"),
			Currency: settlementValue(t, domain.NewCurrencyCode, "USD"),
			CutOffAt: statementCutOffAt,
			Charges:  []domain.CustomerCharge{confirmedCharge(t, "charge-1", 10000)},
			Adjustments: []domain.ChargeAdjustment{
				creditAdjustment(t, "adjustment-x", "charge-9", 500),
			},
		}); !errors.Is(err, domain.ErrInvalidStatementDraft) {
			t.Fatalf("error = %v; 别的费用的调整被记进了这个账户", err)
		}
	})

	t.Run("a recut forms a new version without touching the original", func(t *testing.T) {
		recut, err := draft.Recut(
			[]domain.CustomerCharge{
				confirmedCharge(t, "charge-1", 10000),
				confirmedCharge(t, "charge-2", 6000),
				confirmedCharge(t, "charge-3", 2000),
			},
			nil,
			settlementValue(t, domain.NewStatementDraftVersion, "draft/v2"),
			statementCutOffAt.Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("recut: %v", err)
		}
		if recut.NetTotalMinor() != 18000 {
			t.Fatalf("recut net total = %d, want 18000", recut.NetTotalMinor())
		}
		if draft.NetTotalMinor() != 15000 || len(draft.Charges()) != 2 {
			t.Fatal("重算改写了原草稿")
		}
		if _, err := draft.Recut(draft.Charges(), draft.Adjustments(), draft.Version(), statementCutOffAt); !errors.Is(err, domain.ErrInvalidStatementDraft) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})
}

// Covers: `AT-SA-067`「固定单号、账户、周期、费用范围和金额快照」、`AT-SA-065`「行金额
// 与总额不一致 → 发布被阻断，不修正为约等于」与 `AT-SA-069`「整单无效 → 原单保留并
// 形成作废依据；替代单使用新身份」——已发布快照类型上没有任何增删行或改额方法。
func TestAPublishedStatementIsASealedSnapshot(t *testing.T) {
	statementType := reflect.TypeOf(domain.PublishedStatement{})
	for index := 0; index < statementType.NumMethod(); index++ {
		name := strings.ToLower(statementType.Method(index).Name)
		for _, banned := range []string{"add", "append", "include", "amend", "replace", "update", "remove"} {
			if strings.Contains(name, banned) {
				t.Fatalf("PublishedStatement 带方法 %q——已发布快照就有了被扩张或改写的入口", name)
			}
		}
	}

	statement := publishedStatement(t)
	if len(statement.Lines()) != 2 || len(statement.AdjustmentLines()) != 1 {
		t.Fatalf("lines = %d adjustments = %d, want 2/1", len(statement.Lines()), len(statement.AdjustmentLines()))
	}
	if statement.TotalMinor() != 15000 {
		t.Fatalf("total = %d, want 15000", statement.TotalMinor())
	}

	t.Run("an imbalanced declared total blocks publication", func(t *testing.T) {
		if _, err := domain.PublishStatement(cutDraft(t),
			settlementValue(t, domain.NewStatementNumber, "statement-2026-08-002"),
			15001, statementPublishedAt); !errors.Is(err, domain.ErrStatementImbalance) {
			t.Fatalf("error = %v, want ErrStatementImbalance", err)
		}
	})

	t.Run("voiding keeps the content and happens once", func(t *testing.T) {
		voided, err := statement.Void(
			settlementValue(t, domain.NewStatementVoidBasisReference, "void-basis-1"),
			statementPublishedAt.Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("void: %v", err)
		}
		if _, _, ok := voided.Voided(); !ok {
			t.Fatal("作废没有登记")
		}
		if len(voided.Lines()) != 2 || voided.TotalMinor() != 15000 {
			t.Fatal("作废删掉了内容——历史派生断了")
		}
		if _, err := voided.Void(
			settlementValue(t, domain.NewStatementVoidBasisReference, "void-basis-2"),
			statementPublishedAt.Add(2*time.Hour),
		); !errors.Is(err, domain.ErrStatementVoided) {
			t.Fatalf("error = %v, want ErrStatementVoided", err)
		}
		if _, _, ok := statement.Voided(); ok {
			t.Fatal("作废改写了原单值——值语义破了")
		}
	})
}

// Covers: 硬句「截单快照不可扩张——后到费用归后续账期不回填」与「对账只纳入既有调整」
// （`AT-SA-064`/`AT-SA-076`/`AT-SA-077`）——纳入关系只持身份引用，类型上没有金额字段；
// 纳入原周期被拒；迟到费用不豁免确认条件。
func TestLateChargesAndAdjustmentsGoToSubsequentPeriods(t *testing.T) {
	inclusionType := reflect.TypeOf(domain.SubsequentInclusion{})
	for index := 0; index < inclusionType.NumField(); index++ {
		name := strings.ToLower(inclusionType.Field(index).Name)
		if strings.Contains(name, "minor") || strings.Contains(name, "amount") {
			t.Fatalf("SubsequentInclusion 携带 %q——纳入关系就能自造金额", inclusionType.Field(index).Name)
		}
	}

	statement := publishedStatement(t)
	subsequent := settlementValue(t, domain.NewBillingPeriodReference, "period-2026-09")

	t.Run("an existing adjustment joins the next period", func(t *testing.T) {
		inclusion, err := domain.IncludeAdjustmentInSubsequentPeriod(
			statement,
			creditAdjustment(t, "adjustment-2", "charge-1", 800),
			settlementValue(t, domain.NewInclusionReference, "inclusion-1"),
			subsequent,
			statementPublishedAt.Add(72*time.Hour),
		)
		if err != nil {
			t.Fatalf("include adjustment: %v", err)
		}
		if inclusion.Kind() != domain.IncludedAdjustment {
			t.Fatalf("kind = %q", inclusion.Kind())
		}
		if inclusion.OriginalPeriod() == inclusion.SubsequentPeriod() {
			t.Fatal("纳入没有换到后续账期")
		}
		if _, present := inclusion.Adjustment(); !present {
			t.Fatal("纳入丢了调整身份")
		}
	})

	t.Run("backfilling the original period is refused", func(t *testing.T) {
		if _, err := domain.IncludeAdjustmentInSubsequentPeriod(
			statement,
			creditAdjustment(t, "adjustment-2", "charge-1", 800),
			settlementValue(t, domain.NewInclusionReference, "inclusion-x"),
			statement.Period(),
			statementPublishedAt.Add(72*time.Hour),
		); !errors.Is(err, domain.ErrInclusionBackfillsPeriod) {
			t.Fatalf("error = %v, want ErrInclusionBackfillsPeriod（回填就是改写已发布快照）", err)
		}
	})

	t.Run("an adjustment for a foreign charge is refused", func(t *testing.T) {
		if _, err := domain.IncludeAdjustmentInSubsequentPeriod(
			statement,
			creditAdjustment(t, "adjustment-3", "charge-9", 500),
			settlementValue(t, domain.NewInclusionReference, "inclusion-y"),
			subsequent,
			statementPublishedAt.Add(72*time.Hour),
		); !errors.Is(err, domain.ErrInvalidInclusion) {
			t.Fatalf("error = %v; 别的账单的调整被关联了进来", err)
		}
	})

	t.Run("a late confirmed charge joins the next period", func(t *testing.T) {
		inclusion, err := domain.IncludeLateChargeInSubsequentPeriod(
			statement,
			confirmedCharge(t, "charge-late-1", 2500),
			settlementValue(t, domain.NewInclusionReference, "inclusion-2"),
			subsequent,
			statementPublishedAt.Add(96*time.Hour),
		)
		if err != nil {
			t.Fatalf("include late charge: %v", err)
		}
		if inclusion.Kind() != domain.IncludedLateCharge {
			t.Fatalf("kind = %q", inclusion.Kind())
		}
	})

	t.Run("a late estimated charge is still refused", func(t *testing.T) {
		if _, err := domain.IncludeLateChargeInSubsequentPeriod(
			statement,
			estimatedStatementCharge(t, "charge-late-9"),
			settlementValue(t, domain.NewInclusionReference, "inclusion-z"),
			subsequent,
			statementPublishedAt.Add(96*time.Hour),
		); !errors.Is(err, domain.ErrChargeNotConfirmed) {
			t.Fatalf("error = %v; 迟到不豁免确认条件", err)
		}
	})
}

// Covers: `AT-SA-070`「只锁定争议金额」、`AT-SA-071`「未指明费用 → 要求补充明确范围」
// 与 `AT-SA-072`/`AT-SA-073`「接受只产生复核请求依据；拒绝保留原费用责任」——异议是
// 独立对象：只持引用、没有任何造调整的入口，原对账单不被触碰。
func TestADisputeIsIndependentAndScoped(t *testing.T) {
	disputeType := reflect.TypeOf(domain.StatementDispute{})
	for index := 0; index < disputeType.NumMethod(); index++ {
		name := strings.ToLower(disputeType.Method(index).Name)
		for _, banned := range []string{"adjust", "credit", "debit", "void", "amend"} {
			if strings.Contains(name, banned) {
				t.Fatalf("StatementDispute 带方法 %q——异议就有了改写对账单或自造金额的入口", name)
			}
		}
	}

	statement := publishedStatement(t)
	charge1 := settlementValue(t, domain.NewCustomerChargeID, "charge-1")

	dispute, err := domain.OpenStatementDispute(
		statement, charge1, 4000,
		settlementValue(t, domain.NewDisputeBasisReference, "customer-claim-1"),
		settlementValue(t, domain.NewDisputeID, "dispute-1"),
		statementPublishedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("open dispute: %v", err)
	}
	if dispute.DisputedMinor() != 4000 {
		t.Fatalf("disputed = %d, want 4000（只锁定争议金额）", dispute.DisputedMinor())
	}

	t.Run("a dispute beyond the line amount is refused", func(t *testing.T) {
		if _, err := domain.OpenStatementDispute(
			statement, charge1, 10001,
			settlementValue(t, domain.NewDisputeBasisReference, "customer-claim-2"),
			settlementValue(t, domain.NewDisputeID, "dispute-x"),
			statementPublishedAt.Add(24*time.Hour),
		); !errors.Is(err, domain.ErrInvalidDispute) {
			t.Fatalf("error = %v; 争议金额超过了该行", err)
		}
	})

	t.Run("a dispute without a named charge is refused", func(t *testing.T) {
		if _, err := domain.OpenStatementDispute(
			statement, settlementValue(t, domain.NewCustomerChargeID, "charge-9"), 100,
			settlementValue(t, domain.NewDisputeBasisReference, "customer-claim-3"),
			settlementValue(t, domain.NewDisputeID, "dispute-y"),
			statementPublishedAt.Add(24*time.Hour),
		); !errors.Is(err, domain.ErrInvalidDispute) {
			t.Fatalf("error = %v; 整单笼统异议要求补充明确范围", err)
		}
	})

	t.Run("a pending review may resolve again, a terminal resolution may not", func(t *testing.T) {
		pending, err := dispute.Resolve(domain.DisputePendingReview,
			settlementValue(t, domain.NewDisputeBasisReference, "review-entry-1"),
			statementPublishedAt.Add(48*time.Hour))
		if err != nil {
			t.Fatalf("resolve pending: %v", err)
		}
		accepted, err := pending.Resolve(domain.DisputeAccepted,
			settlementValue(t, domain.NewDisputeBasisReference, "review-request-1"),
			statementPublishedAt.Add(72*time.Hour))
		if err != nil {
			t.Fatalf("resolve accepted after pending: %v", err)
		}
		kind, basis, _, resolved := accepted.Resolution()
		if !resolved || kind != domain.DisputeAccepted || basis.String() != "review-request-1" {
			t.Fatalf("resolution = %q/%q resolved=%v", kind, basis, resolved)
		}
		if _, err := accepted.Resolve(domain.DisputeRejected,
			settlementValue(t, domain.NewDisputeBasisReference, "late-flip"),
			statementPublishedAt.Add(96*time.Hour)); !errors.Is(err, domain.ErrDisputeResolved) {
			t.Fatalf("error = %v; 终局处理被改写", err)
		}
	})

	t.Run("the resolution kind set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, kind := range []domain.DisputeResolutionKind{
			domain.DisputeAccepted, domain.DisputePartiallyAccepted,
			domain.DisputeRejected, domain.DisputePendingReview,
		} {
			label := kind.String()
			if label == "" {
				t.Fatalf("kind %d has no label", kind)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.DisputeResolutionKind(len(labels)+1).String() != "" {
			t.Fatal("第五个处理取值带了标签——封闭集合被悄悄放开")
		}
	})
}

func TestRehydrateDisputeDoesNotNeedTheStatement(t *testing.T) {
	opened := publishedStatement(t)
	formed, err := domain.OpenStatementDispute(
		opened,
		settlementValue(t, domain.NewCustomerChargeID, "charge-1"),
		4000,
		settlementValue(t, domain.NewDisputeBasisReference, "weight-mismatch"),
		settlementValue(t, domain.NewDisputeID, "dispute-1"),
		statementPublishedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	resolved, err := formed.Resolve(
		domain.DisputeAccepted,
		settlementValue(t, domain.NewDisputeBasisReference, "review-request-1"),
		statementPublishedAt.Add(48*time.Hour),
	)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	restored, err := domain.RehydrateStatementDispute(domain.RehydrateStatementDisputeSpec{
		Dispute:       resolved.Dispute(),
		Statement:     resolved.Statement(),
		Charge:        resolved.Charge(),
		DisputedMinor: resolved.DisputedMinor(),
		Reason:        resolved.Reason(),
		OpenedAt:      resolved.OpenedAt(),
		Resolution:    domain.DisputeAccepted,
		ResolutionRef: settlementValue(t, domain.NewDisputeBasisReference, "review-request-1"),
		ResolvedAt:    statementPublishedAt.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	kind, _, _, ok := restored.Resolution()
	if !ok || kind != domain.DisputeAccepted {
		t.Fatal("裁定没有随重建回来")
	}

	if _, err := domain.RehydrateSubsequentInclusion(domain.RehydrateSubsequentInclusionSpec{
		Inclusion:        settlementValue(t, domain.NewInclusionReference, "inclusion-1"),
		Kind:             domain.IncludedLateCharge,
		Statement:        opened.Number(),
		OriginalPeriod:   opened.Period(),
		SubsequentPeriod: opened.Period(),
		Charge:           settlementValue(t, domain.NewCustomerChargeID, "charge-1"),
		IncludedAt:       statementPublishedAt.Add(24 * time.Hour),
	}); !errors.Is(err, domain.ErrInclusionBackfillsPeriod) {
		t.Fatalf("error = %v; 回填原周期从重建门溜过", err)
	}
}
