package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// adjustmentViewDouble 只答「这笔调整长什么样」。它刻意没有 Save：UC-SA-003 对调整只有
// 纳入关系的所有权，替身也不该长出一条本用例不该有的路。
type adjustmentViewDouble struct {
	records map[string]ports.ChargeAdjustmentRecord
	err     error
}

func newAdjustmentView() *adjustmentViewDouble {
	return &adjustmentViewDouble{records: map[string]ports.ChargeAdjustmentRecord{}}
}

func (double *adjustmentViewDouble) FindByKey(
	_ context.Context,
	key ports.ChargeAdjustmentKey,
) (ports.ChargeAdjustmentRecord, bool, error) {
	if double.err != nil {
		return ports.ChargeAdjustmentRecord{}, false, double.err
	}
	record, found := double.records[key.TenantID.String()+"|"+key.Adjustment.String()]
	return record, found, nil
}

// pricingCorrectionOn 造一笔已由 UC-SA-002 形成的计价纠错借项——本用例只读它，从不造它。
func pricingCorrectionOn(t *testing.T, id, chargeID string, formedAt time.Time) ports.ChargeAdjustmentRecord {
	t.Helper()
	tenant := billValue(t, domain.NewTenantID, "tenant-1")
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:                 billValue(t, domain.NewChargeAdjustmentID, id),
		Charge:             billValue(t, domain.NewCustomerChargeID, chargeID),
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         billValue(t, domain.NewSellEvaluationReference, "sell-evaluation-2"),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      700,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    700,
		FormedAt:           formedAt,
	})
	if err != nil {
		t.Fatalf("form adjustment: %v", err)
	}
	return ports.ChargeAdjustmentRecord{
		Key:        ports.ChargeAdjustmentKey{TenantID: tenant, Adjustment: adjustment.ID()},
		Adjustment: adjustment,
		RecordedAt: formedAt,
	}
}

// Covers: `AT-SA-076`「迟到测量导致费用增加 → UC-SA-002 先形成计价纠错借项；本用例只把
// 该既有借项纳入后续周期并关联原费用/账单」与 UC-SA-003 步 7 的调整半边——纳入原周期
// → INCLUSION_BACKFILLS_PERIOD；调整挂在单外费用上 → 未受理；纳入幂等分重放/冲突并交意图。
func TestAnExistingAdjustmentIsIncludedInASubsequentPeriod(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	tenant := billValue(t, domain.NewTenantID, "tenant-1")
	formedAt := statementPublishAt.Add(36 * time.Hour)
	fixture.adjustments.records["tenant-1|adj-1"] = pricingCorrectionOn(t, "adj-1", "charge-A", formedAt)
	fixture.adjustments.records["tenant-1|adj-outside"] = pricingCorrectionOn(t, "adj-outside", "charge-late", formedAt)

	include := application.IncludeAdjustmentCommand{
		TenantID:         tenant,
		Inclusion:        "inclusion-adj-1",
		Number:           "STMT-2026-08-001",
		AdjustmentID:     "adj-1",
		SubsequentPeriod: "2026-09",
		IncludedAt:       statementPublishAt.Add(48 * time.Hour),
	}

	included, err := fixture.handler.IncludeAdjustment(context.Background(), include)
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	if included.Outcome() != application.AdjustmentIncluded {
		t.Fatalf("outcome = %q, want ADJUSTMENT_INCLUDED", included.Outcome())
	}
	record, present := included.Inclusion()
	if !present {
		t.Fatal("no inclusion returned")
	}
	inclusion := record.Inclusion
	if inclusion.Kind() != domain.IncludedAdjustment {
		t.Fatalf("kind = %q, want ADJUSTMENT", inclusion.Kind())
	}
	adjustmentID, hasAdjustment := inclusion.Adjustment()
	if !hasAdjustment || adjustmentID.String() != "adj-1" {
		t.Fatalf("adjustment = %q/%v，纳入关系没有指回既有调整", adjustmentID, hasAdjustment)
	}
	if inclusion.Charge().String() != "charge-A" ||
		inclusion.Statement().String() != "STMT-2026-08-001" ||
		inclusion.OriginalPeriod().String() != "2026-08" ||
		inclusion.SubsequentPeriod().String() != "2026-09" {
		t.Fatalf("纳入关系没有同时关联原账单、原费用与后续周期：%+v", inclusion)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（纳入也交下游）", len(fixture.handoff.intents))
	}
	if handed := fixture.handoff.intents[1]; handed.Inclusion.Key.Inclusion.String() != "inclusion-adj-1" {
		t.Fatalf("交出去的意图不是这次纳入：%+v", handed)
	}

	t.Run("backfilling the original period is refused", func(t *testing.T) {
		backfill := include
		backfill.Inclusion = "inclusion-adj-2"
		backfill.SubsequentPeriod = "2026-08"
		result, err := fixture.handler.IncludeAdjustment(context.Background(), backfill)
		if err != nil {
			t.Fatalf("backfill: %v", err)
		}
		if result.Outcome() != application.InclusionBackfillsOutcome {
			t.Fatalf("outcome = %q, want INCLUSION_BACKFILLS_PERIOD（已发布快照不回填）", result.Outcome())
		}
		if len(fixture.inclusions.records) != 1 {
			t.Fatal("回填的纳入落了库")
		}
	})

	t.Run("an adjustment on a charge outside the statement is not accepted", func(t *testing.T) {
		outside := include
		outside.Inclusion = "inclusion-adj-3"
		outside.AdjustmentID = "adj-outside"
		result, err := fixture.handler.IncludeAdjustment(context.Background(), outside)
		if err != nil {
			t.Fatalf("include outside: %v", err)
		}
		if result.Outcome() != application.StatementNotAccepted {
			t.Fatalf("outcome = %q（别的费用的调整进来就是把别人的数记到这个账户头上）", result.Outcome())
		}
	})

	t.Run("an unknown adjustment is not accepted", func(t *testing.T) {
		unknown := include
		unknown.Inclusion = "inclusion-adj-4"
		unknown.AdjustmentID = "adj-9"
		result, err := fixture.handler.IncludeAdjustment(context.Background(), unknown)
		if err != nil {
			t.Fatalf("include unknown: %v", err)
		}
		if result.Outcome() != application.StatementNotAccepted {
			t.Fatalf("outcome = %q（指名了不存在的调整是提交矛盾）", result.Outcome())
		}
	})

	t.Run("a replay returns the original inclusion", func(t *testing.T) {
		replay, err := fixture.handler.IncludeAdjustment(context.Background(), include)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.InclusionExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
	})

	t.Run("a different period under the same inclusion identity is a conflict", func(t *testing.T) {
		flipped := include
		flipped.SubsequentPeriod = "2026-10"
		result, err := fixture.handler.IncludeAdjustment(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict include: %v", err)
		}
		if result.Outcome() != application.InclusionConflict {
			t.Fatalf("outcome = %q, want INCLUSION_CONFLICT", result.Outcome())
		}
	})
}

// Covers: ADR-0029 在调整纳入这一步的恢复面——调整读口故障归未决并指名等谁；以及
// UC-SA-003「不得创建金额调整」的结构面：本编排拿到的调整口只有读法，没有任何写法。
func TestAdjustmentInclusionOnlyReadsAdjustments(t *testing.T) {
	t.Run("an unavailable adjustment view is undecided", func(t *testing.T) {
		fixture := newStatementFixture(t)
		if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
			t.Fatalf("publish: %v", err)
		}
		fixture.adjustments.err = errors.New("view down")
		result, err := fixture.handler.IncludeAdjustment(context.Background(), application.IncludeAdjustmentCommand{
			TenantID:         billValue(t, domain.NewTenantID, "tenant-1"),
			Inclusion:        "inclusion-adj-1",
			Number:           "STMT-2026-08-001",
			AdjustmentID:     "adj-1",
			SubsequentPeriod: "2026-09",
			IncludedAt:       statementPublishAt.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("include: %v", err)
		}
		if result.Outcome() != application.StatementUndecided ||
			result.UndecidedReason() != application.AdjustmentViewUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("the adjustment dependency has no write method", func(t *testing.T) {
		field, found := reflect.TypeOf(application.CutOffPublishStatementDeps{}).FieldByName("Adjustments")
		if !found {
			t.Fatal("编排没有调整读口")
		}
		for index := 0; index < field.Type.NumMethod(); index++ {
			name := field.Type.Method(index).Name
			if name != "FindByKey" {
				t.Fatalf("调整口长出了 %q——UC-SA-003 只拥有纳入关系，连一条能写调整的路都不该有", name)
			}
		}
	})
}
