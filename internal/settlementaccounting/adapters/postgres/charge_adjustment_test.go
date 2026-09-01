package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var adjustmentFormedAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

func newChargeAdjustments(t *testing.T) (*adapter.ChargeAdjustments, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	adjustments, err := adapter.NewChargeAdjustments(db)
	if err != nil {
		t.Fatalf("构造调整册：%v", err)
	}
	return adjustments, db.Transactor(), pool
}

func correctionAdjustment(t *testing.T, id, charge string, at time.Time) domain.ChargeAdjustment {
	t.Helper()
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:                 saValue(t, domain.NewChargeAdjustmentID, id),
		Charge:             saValue(t, domain.NewCustomerChargeID, charge),
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "sell-eval/re-"+id),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      900,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    900,
		FormedAt:           at,
	})
	if err != nil {
		t.Fatalf("形成纠错调整：%v", err)
	}
	return adjustment
}

func adjustmentKey(t *testing.T, tenant, id string) ports.ChargeAdjustmentKey {
	t.Helper()
	return ports.ChargeAdjustmentKey{
		TenantID:   saValue(t, domain.NewTenantID, tenant),
		Adjustment: saValue(t, domain.NewChargeAdjustmentID, id),
	}
}

// Covers: ADR-0087 决定二——调整独立成册，不必先进对账单才存在；册只追加，同标识重放
// 答`已登记`且不改写先到者。
func TestAChargeAdjustmentRoundTripsWithoutAStatement(t *testing.T) {
	adjustments, transactor, _ := newChargeAdjustments(t)
	ctx := t.Context()

	key := adjustmentKey(t, "tenant-a", "adj-1")
	record := ports.ChargeAdjustmentRecord{
		Key:        key,
		Adjustment: correctionAdjustment(t, "adj-1", "charge-1", adjustmentFormedAt),
		RecordedAt: adjustmentFormedAt.Add(time.Minute),
	}
	var outcome ports.ChargeAdjustmentSaveOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = adjustments.Save(txCtx, record)
		return err
	}); err != nil {
		t.Fatalf("登记调整：%v", err)
	}
	if outcome != ports.ChargeAdjustmentSaved {
		t.Fatalf("outcome = %d, want ChargeAdjustmentSaved", outcome)
	}

	// 读得回来，且没有任何对账单参与：调整先成立，纳入是后来的事。
	found, present, err := adjustments.FindByKey(ctx, key)
	if err != nil || !present {
		t.Fatalf("找回调整：err=%v present=%v", err, present)
	}
	if found.Adjustment.Kind() != domain.PricingCorrection ||
		found.Adjustment.Direction() != domain.AdjustmentDebit {
		t.Fatalf("种类或方向变形：%#v", found.Adjustment)
	}
	if found.Adjustment.Charge().String() != "charge-1" {
		t.Fatalf("被调整的费用变了：%s", found.Adjustment.Charge())
	}
	if evaluation, ok := found.Adjustment.Evaluation(); !ok || evaluation.String() != "sell-eval/re-adj-1" {
		t.Fatal("纠错类读回丢了评价引用")
	}
	if _, ok := found.Adjustment.Authorization(); ok {
		t.Fatal("纠错类读回带了商业授权——依据分格在读侧没守住")
	}
	if !found.Adjustment.FormedAt().Equal(adjustmentFormedAt) {
		t.Fatalf("形成时点变了：%s", found.Adjustment.FormedAt())
	}

	// 同标识重放：答`已登记`，先到者不被改写。
	replay := record
	replay.Adjustment = correctionAdjustment(t, "adj-1", "charge-9", adjustmentFormedAt.Add(time.Hour))
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = adjustments.Save(txCtx, replay)
		return err
	}); err != nil {
		t.Fatalf("重放登记：%v", err)
	}
	if outcome != ports.ChargeAdjustmentAlreadyRecorded {
		t.Fatalf("outcome = %d, want ChargeAdjustmentAlreadyRecorded", outcome)
	}
	after, _, err := adjustments.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("重放后找回：%v", err)
	}
	if after.Adjustment.Charge().String() != "charge-1" {
		t.Fatal("重放改写了先到者——册不是只追加的")
	}
}

// Covers: 同一笔费用上的调整是追加序列，按形成时点读回；一笔没被调整过的费用答空集
// 而不是错——「没有调整」与「费用不存在」不是同一件事，后者由费用库回答。
func TestAdjustmentsOnOneChargeComeBackAsAnAppendedSequence(t *testing.T) {
	adjustments, transactor, _ := newChargeAdjustments(t)
	ctx := t.Context()

	tenant := saValue(t, domain.NewTenantID, "tenant-a")
	charge := saValue(t, domain.NewCustomerChargeID, "charge-seq")
	// 故意按倒序登记：读回顺序由形成时点决定，不由登记先后决定。
	for _, seed := range []struct {
		id string
		at time.Time
	}{
		{"adj-late", adjustmentFormedAt.Add(2 * time.Hour)},
		{"adj-early", adjustmentFormedAt},
	} {
		record := ports.ChargeAdjustmentRecord{
			Key:        adjustmentKey(t, "tenant-a", seed.id),
			Adjustment: correctionAdjustment(t, seed.id, "charge-seq", seed.at),
			RecordedAt: seed.at,
		}
		if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := adjustments.Save(txCtx, record)
			return err
		}); err != nil {
			t.Fatalf("登记 %s：%v", seed.id, err)
		}
	}

	listed, err := adjustments.ListByCharge(ctx, tenant, charge)
	if err != nil {
		t.Fatalf("按费用上列：%v", err)
	}
	if len(listed) != 2 ||
		listed[0].Key.Adjustment.String() != "adj-early" ||
		listed[1].Key.Adjustment.String() != "adj-late" {
		t.Fatalf("上列顺序不是形成序：%#v", listed)
	}

	untouched, err := adjustments.ListByCharge(
		ctx, tenant, saValue(t, domain.NewCustomerChargeID, "charge-untouched"))
	if err != nil {
		t.Fatalf("未被调整过的费用上列：%v", err)
	}
	if len(untouched) != 0 {
		t.Fatalf("凭空多出 %d 笔调整", len(untouched))
	}

	other := saValue(t, domain.NewTenantID, "tenant-b")
	crossTenant, err := adjustments.ListByCharge(ctx, other, charge)
	if err != nil {
		t.Fatalf("跨租户上列：%v", err)
	}
	if len(crossTenant) != 0 {
		t.Fatal("另一个租户看得见这笔费用的调整")
	}
}

func TestChargeAdjustmentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	adjustments, _, _ := newChargeAdjustments(t)
	record := ports.ChargeAdjustmentRecord{
		Key:        adjustmentKey(t, "tenant-a", "adj-no-tx"),
		Adjustment: correctionAdjustment(t, "adj-no-tx", "charge-1", adjustmentFormedAt),
		RecordedAt: adjustmentFormedAt,
	}
	// 断言的是 ErrTransactionRequired 本身而不是「有错就行」：后者对任何一种失败都成立，
	// 包括写口压根没跑到 RequireExecutor 那一步的那些。
	if _, err := adjustments.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Save：%v", err)
	}
}

// Covers: 库面判据与 FormChargeAdjustment 是同一组判据的两份。逐格钉住**是哪条约束**
// 拒的：只断言 err != nil 时，一个写错的 INSERT 与一条真正生效的 CHECK 长着同一张脸。
func TestChargeAdjustmentCheckConstraintsMirrorTheFormingGate(t *testing.T) {
	_, _, pool := newChargeAdjustments(t)
	ctx := t.Context()

	insert := `INSERT INTO settlement_accounting.charge_adjustment
			(tenant_id, adjustment_id, charge_id, kind, direction,
			 evaluation_ref, authorization_ref, original_currency, original_minor,
			 settlement_currency, settlement_minor, conversion_ref, formed_at, recorded_at)
		 VALUES ('tenant-a', $1, 'charge-1', $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())`

	// constraints 允许多名的场合只有一种：一行同时违反了多条，而 Postgres 报哪条不由
	// 我们决定。未知种类就是这样——basis_slotted_by_kind 的两个分支各自点名一个种类，
	// 于是它连同 kind_closed 一起被违反。列全两条仍然守得住：拒它的必须是这两条带种类
	// 判据的之一，不能是某个 NOT NULL 之类的失手。
	for _, refusal := range []struct {
		name        string
		args        []any
		constraints []string
		complaint   string
	}{
		{
			name:        "adj-bad-kind",
			args:        []any{"CLAIM_REFUND", "DEBIT", "eval-1", nil, "CNY", 100, "CNY", 100, nil},
			constraints: []string{"charge_adjustment_kind_closed", "charge_adjustment_basis_slotted_by_kind"},
			complaint:   "索赔退款进了普通客户费用调整册——那类归 UC-SA-007 与它自己的册",
		},
		{
			name:        "adj-void",
			args:        []any{"", "DEBIT", "eval-1", nil, "CNY", 100, "CNY", 100, nil},
			constraints: []string{"charge_adjustment_kind_closed", "charge_adjustment_basis_slotted_by_kind"},
			complaint:   "一笔无语义的「冲销」溜进了调整册",
		},
		{
			name:        "adj-wrong-slot",
			args:        []any{"PRICING_CORRECTION", "DEBIT", nil, "authz-1", "CNY", 100, "CNY", 100, nil},
			constraints: []string{"charge_adjustment_basis_slotted_by_kind"},
			complaint:   "纠错挂着商业授权溜了进去——Kind 之外没有第二个维分辨依据",
		},
		{
			name:        "adj-both-slots",
			args:        []any{"COMMERCIAL_CONCESSION", "CREDIT", "eval-1", "authz-1", "CNY", 100, "CNY", 100, nil},
			constraints: []string{"charge_adjustment_basis_slotted_by_kind"},
			complaint:   "两格齐填溜了进去",
		},
		{
			name:        "adj-no-conversion",
			args:        []any{"PRICING_CORRECTION", "DEBIT", "eval-1", nil, "USD", 100, "CNY", 700, nil},
			constraints: []string{"charge_adjustment_conversion_present"},
			complaint:   "跨币种没有换算依据溜了进去——第二个数只能是自行取汇率补算的",
		},
		{
			name:        "adj-amounts-disagree",
			args:        []any{"PRICING_CORRECTION", "DEBIT", "eval-1", nil, "CNY", 100, "CNY", 90, nil},
			constraints: []string{"charge_adjustment_same_currency_amounts_agree"},
			complaint:   "同币种两额不等溜了进去",
		},
		{
			name:        "adj-zero-amount",
			args:        []any{"PRICING_CORRECTION", "DEBIT", "eval-1", nil, "CNY", 0, "CNY", 0, nil},
			constraints: []string{"charge_adjustment_amounts_positive"},
			complaint:   "零金额的调整溜了进去",
		},
	} {
		_, err := pool.Exec(ctx, insert, append([]any{refusal.name}, refusal.args...)...)
		if err == nil {
			t.Fatal(refusal.complaint)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || !slices.Contains(refusal.constraints, pgErr.ConstraintName) {
			t.Fatalf("%s 被拒了，但不是 %v 之一拒的：%v", refusal.name, refusal.constraints, err)
		}
	}
}
