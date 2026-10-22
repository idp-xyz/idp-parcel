package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestRecordAttributionDateAndLockChargeBillingRefuseToRunOutsideATransaction(t *testing.T) {
	charges, _, _, _, _ := newChargeAdvanceStores(t)
	_, statements, _, _ := newBillStores(t)
	tenant := saTenant(t, "tenant-a")
	id := saValue(t, domain.NewCustomerChargeID, "charge-1")
	if _, err := charges.RecordAttributionDate(t.Context(), tenant, id, domain.AttributionDate{Year: 2026, Month: time.August, Day: 31}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordAttributionDate 应答 ErrTransactionRequired，实得：%v", err)
	}
	if _, _, err := statements.LockChargeBilling(t.Context(), tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 LockChargeBilling 应答 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0269 决定三——来源发生时点随形成落库，归属日随确认同笔落库、读回原值；截单当场判出的归属日只落
// 在还没有归属日的已确认行上，已有的不改、交回册上那一天；预估行上落不了归属日（库上的约束也挡）。
func TestTheAttributionDateIsStoredOnceOnAConfirmedCharge(t *testing.T) {
	charges, _, _, transactor, pool := newChargeAdvanceStores(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")
	occurred := time.Date(2026, 8, 9, 22, 0, 0, 0, time.UTC)
	day := domain.AttributionDate{Year: 2026, Month: time.August, Day: 12}

	estimated := chargeWithSourceInstant(t, "charge-estimated", occurred)
	attributed, err := chargeWithSourceInstant(t, "charge-attributed", occurred).Confirm(
		saConfirmationFacts(t), saValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/final-1"), chargeConfirmedAt)
	if err != nil {
		t.Fatal(err)
	}
	if attributed, err = attributed.WithAttributionDate(day); err != nil {
		t.Fatal(err)
	}
	unattributed := confirmedCharge(t, "charge-unattributed", "DELIVERY_FINALIZED/final-2")
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := charges.SaveEstimated(txCtx, tenant, estimated); err != nil {
			return err
		}
		if _, err := charges.SaveConfirmed(txCtx, tenant, attributed); err != nil {
			return err
		}
		_, err := charges.SaveConfirmed(txCtx, tenant, unattributed)
		return err
	})

	found, _, err := charges.FindByID(ctx, tenant, attributed.ID())
	if err != nil {
		t.Fatal(err)
	}
	if date, ok := found.AttributionDate(); !ok || date != day {
		t.Fatalf("归属日往返 = %s ok=%v", date, ok)
	}
	if at, ok := found.SourceOccurredAt(); !ok || !at.Equal(occurred) {
		t.Fatalf("来源发生时点往返 = %v ok=%v", at, ok)
	}
	if found, _, err := charges.FindByID(ctx, tenant, estimated.ID()); err != nil {
		t.Fatal(err)
	} else if at, ok := found.SourceOccurredAt(); !ok || !at.Equal(occurred) {
		t.Fatalf("预估行的来源发生时点往返 = %v ok=%v", at, ok)
	}

	record := func(id domain.CustomerChargeID, date domain.AttributionDate) (domain.AttributionDate, error) {
		var stored domain.AttributionDate
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var recordErr error
			stored, recordErr = charges.RecordAttributionDate(txCtx, tenant, id, date)
			return recordErr
		})
		return stored, err
	}
	later := domain.AttributionDate{Year: 2026, Month: time.September, Day: 1}
	if stored, err := record(unattributed.ID(), later); err != nil || stored != later {
		t.Fatalf("当场判出的归属日 = %s err = %v", stored, err)
	}
	if stored, err := record(unattributed.ID(), day); err != nil || stored != later {
		t.Fatalf("已有归属日被改成了 %s err = %v", stored, err)
	}
	if stored, err := record(attributed.ID(), later); err != nil || stored != day {
		t.Fatalf("确认时形成的归属日被改成了 %s err = %v", stored, err)
	}
	if _, err := record(estimated.ID(), later); err == nil {
		t.Fatal("预估费用落上了归属日")
	}
	if _, err := record(saValue(t, domain.NewCustomerChargeID, "charge-absent"), later); err == nil {
		t.Fatal("册上没有的费用落上了归属日")
	}
	if _, err := pool.Exec(ctx, `UPDATE settlement_accounting.customer_charge SET attribution_date = '2026-08-31'
		WHERE tenant_id = $1 AND charge_id = $2`, tenant.String(), estimated.ID().String()); err == nil {
		t.Fatal("库上的约束放进了一行带归属日的预估费用")
	}

	// 生产确认路径：预估行先在，确认是对它的改写，归属日与来源发生时点都要随这一次改写留在行上。
	promoted, err := estimated.Confirm(saConfirmationFacts(t),
		saValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/final-3"), chargeConfirmedAt)
	if err != nil {
		t.Fatal(err)
	}
	if promoted, err = promoted.WithAttributionDate(day); err != nil {
		t.Fatal(err)
	}
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := charges.SaveConfirmed(txCtx, tenant, promoted)
		if err != nil || saved != ports.ChargeSaved {
			return errors.Join(err, errors.New("改写预估行的确认没落下"))
		}
		return nil
	})
	found, _, err = charges.FindByID(ctx, tenant, estimated.ID())
	if err != nil {
		t.Fatal(err)
	}
	if date, ok := found.AttributionDate(); !ok || date != day {
		t.Fatalf("改写预估行的确认丢了归属日：%s ok=%v", date, ok)
	}
	if at, ok := found.SourceOccurredAt(); !ok || !at.Equal(occurred) {
		t.Fatalf("改写预估行的确认丢了来源发生时点：%v ok=%v", at, ok)
	}
}

// Covers: UC-SA-003「已出账的不再纳入」的读法——费用在一张未作废的已发布对账单上就答已出账并交回单号；整单作废后
// 那张单上的费用不算已出账；别的租户看不到。
func TestLockChargeBillingFindsTheUnvoidedStatementCarryingTheCharge(t *testing.T) {
	_, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()
	mustSaveStatement(t, transactor, ctx, statements, statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1"))
	lock := func(tenant string, charge string) (domain.StatementNumber, bool) {
		var number domain.StatementNumber
		var billed bool
		saWithin(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			number, billed, err = statements.LockChargeBilling(txCtx, saTenant(t, tenant), saValue(t, domain.NewCustomerChargeID, charge))
			return err
		})
		return number, billed
	}
	if number, billed := lock("tenant-1", "charge-2"); !billed || number.String() != "STMT-2026-09" {
		t.Fatalf("charge-2 billed=%v on %q", billed, number)
	}
	if _, billed := lock("tenant-1", "charge-9"); billed {
		t.Fatal("不在任何单上的费用答了已出账")
	}
	if _, billed := lock("tenant-2", "charge-2"); billed {
		t.Fatal("别的租户的单让这件费用成了已出账")
	}

	record, _, err := statements.FindByKey(ctx, ports.StatementKey{TenantID: saTenant(t, "tenant-1"), Number: saValue(t, domain.NewStatementNumber, "STMT-2026-09")})
	if err != nil {
		t.Fatal(err)
	}
	voided, err := record.Statement.Void(saValue(t, domain.NewStatementVoidBasisReference, "void-1"), statementCutAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	record.Statement = voided
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := statements.Replace(txCtx, record)
		return err
	})
	if _, billed := lock("tenant-1", "charge-2"); billed {
		t.Fatal("作废单上的费用仍答已出账")
	}
}

// Covers: UC-SA-003「同一费用被两个账户或周期同时纳入」在出账判断上的串行：先到的截单锁住费用、写下对账单之前，
// 后到的截单问同一件费用要等；先到者提交之后它读到的是已出账，不会两张单各收一次。锁与写在同一事务里。
func TestTwoCutsListingTheSameChargeAreSerializedByTheBillingLock(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	statements, err := adapter.NewCustomerStatements(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	charge := saValue(t, domain.NewCustomerChargeID, "charge-1")
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		_, err := charges.SaveConfirmed(txCtx, tenant, confirmedCharge(t, "charge-1", "DELIVERY_FINALIZED/final-1"))
		return err
	})
	record := statementRecord(t, "tenant-1", "STMT-FIRST", "digest-first")

	firstLocked, releaseFirst := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	// 断言失败时也放开先到者：它的事务不结束，库就收不了尾，用例挂到超时而不是报红。
	t.Cleanup(release)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if _, billed, err := statements.LockChargeBilling(txCtx, tenant, charge); err != nil || billed {
				return errors.Join(err, errors.New("先到的截单读到了已出账"))
			}
			close(firstLocked)
			<-releaseFirst
			_, err := statements.Save(txCtx, record)
			return err
		})
	}()
	<-firstLocked

	type answer struct {
		number domain.StatementNumber
		billed bool
		err    error
	}
	secondDone := make(chan answer, 1)
	go func() {
		var got answer
		got.err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			got.number, got.billed, err = statements.LockChargeBilling(txCtx, tenant, charge)
			return err
		})
		secondDone <- got
	}()
	select {
	case got := <-secondDone:
		t.Fatalf("后到的截单没等先到者就答了：%+v", got)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	got := <-secondDone
	if got.err != nil || !got.billed || got.number.String() != "STMT-FIRST" {
		t.Fatalf("后到的截单 billed=%v on %q err=%v，want 已出账在 STMT-FIRST", got.billed, got.number, got.err)
	}
}

func chargeWithSourceInstant(t *testing.T, id string, occurred time.Time) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 saValue(t, domain.NewCustomerChargeID, id),
		FeeItem:            saValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         saValue(t, domain.NewSellEvaluationReference, "evaluation-sell-1"),
		OriginalCurrency:   saValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      45600,
		SettlementCurrency: saValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    45600,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
		SourceOccurredAt:   occurred,
	})
	if err != nil {
		t.Fatal(err)
	}
	return charge
}
