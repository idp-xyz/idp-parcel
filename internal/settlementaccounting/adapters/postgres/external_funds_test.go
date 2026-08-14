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
	fundsOccurredAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
	fundsMappedAt   = time.Date(2026, 8, 12, 11, 30, 0, 0, time.UTC)
	fundsAppliedAt  = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	fundsReversedAt = time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
)

func TestAnExternalFundsFactRoundTripsAndSecondAdoptKeepsTheWinner(t *testing.T) {
	facts, _, _, transactor, _ := newFundsStores(t)
	ctx := t.Context()

	record := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := facts.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.FundsFactSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := facts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if _, amount := found.Fact.Amount(); amount != 8000 || found.Fact.Kind() != domain.FundsReceiptConfirmed {
		t.Fatal("资金事实往返变形")
	}

	corrected, err := domain.RehydrateExternalFundsFact(domain.RehydrateExternalFundsFactSpec{
		Fact:        saValue(t, domain.NewFundsFactReference, "bank-fact-2"),
		Source:      saValue(t, domain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Kind:        domain.FundsReceiptConfirmed,
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 7500,
		Version:     saValue(t, domain.NewFundsFactVersion, "bank-fact/v2"),
		OccurredAt:  fundsOccurredAt,
		Corrects:    saValue(t, domain.NewFundsFactVersion, "bank-fact/v1"),
		CorrectedAt: fundsOccurredAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("rehydrate corrected: %v", err)
	}
	correctedRecord := ports.FundsFactRecord{
		Key:           ports.FundsFactKey{TenantID: saTenant(t, "tenant-a"), Fact: corrected.Fact()},
		ContentDigest: "digest-corrected",
		Fact:          corrected,
		RecordedAt:    fundsOccurredAt.Add(time.Hour),
	}
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := facts.Save(txCtx, correctedRecord)
		if err != nil {
			return err
		}
		if outcome != ports.FundsFactSaved {
			t.Fatalf("corrected save = %d", outcome)
		}
		return nil
	})
	foundCorrected, exists, err := facts.FindByKey(ctx, correctedRecord.Key)
	if err != nil || !exists {
		t.Fatalf("更正读回失败：exists=%v err=%v", exists, err)
	}
	predecessor, present := foundCorrected.Fact.Corrects()
	if !present || predecessor.String() != "bank-fact/v1" {
		t.Fatal("更正回指没有往返")
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := facts.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := facts.FindByKey(txCtx, record.Key)
		if err != nil || !found || winner.ContentDigest != record.ContentDigest {
			t.Fatalf("同事务读回赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if outcome != ports.FundsFactAlreadyAdopted {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestAMappingAndApplicationRoundTripAndReplaceOnlyWritesReversal(t *testing.T) {
	facts, mappings, applications, transactor, _ := newFundsStores(t)
	ctx := t.Context()

	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	payable := mappingRecord(t, "tenant-a", "mapping-1", fact.Fact, domain.TargetPayable, "payable-1")
	credit := mappingRecord(t, "tenant-a", "mapping-2", fact.Fact, domain.TargetCreditNote, "credit-note-1")
	application := applicationRecord(t, "tenant-a", "application-1", fact.Fact, payable.Mapping, credit.Mapping)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := facts.Save(txCtx, fact); err != nil {
			return err
		}
		if _, err := mappings.Save(txCtx, payable); err != nil {
			return err
		}
		if _, err := mappings.Save(txCtx, credit); err != nil {
			return err
		}
		outcome, err := applications.Save(txCtx, application)
		if err != nil {
			return err
		}
		if outcome != ports.SettlementApplicationSaved {
			t.Fatalf("application save = %d", outcome)
		}
		return nil
	})

	foundMapping, exists, err := mappings.FindByKey(ctx, payable.Key)
	if err != nil || !exists || foundMapping.Mapping.TargetKind() != domain.TargetPayable {
		t.Fatalf("映射往返失败：exists=%v err=%v", exists, err)
	}

	found, exists, err := applications.FindByKey(ctx, application.Key)
	if err != nil || !exists {
		t.Fatalf("核销读回失败：exists=%v err=%v", exists, err)
	}
	if found.Application.AppliedMinor() != 8000 || found.Application.RemainderMinor() != 0 {
		t.Fatalf("applied=%d remainder=%d", found.Application.AppliedMinor(), found.Application.RemainderMinor())
	}
	if len(found.Application.Allocations()) != 2 {
		t.Fatalf("allocations = %d", len(found.Application.Allocations()))
	}

	reversed, err := found.Application.Reverse(saValue(t, domain.NewApplicationBasisReference, "reversal-1"), fundsReversedAt)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	replaced := found
	replaced.Application = reversed
	replaced.RecordedAt = fundsReversedAt
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := applications.Replace(txCtx, replaced)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("撤销 Replace 答 false")
		}
		return nil
	})

	after, _, err := applications.FindByKey(ctx, application.Key)
	if err != nil {
		t.Fatalf("撤销后读回：%v", err)
	}
	if _, _, ok := after.Application.Reversed(); !ok {
		t.Fatal("撤销没有落库")
	}
	if after.Application.AppliedMinor() != 8000 || len(after.Application.Allocations()) != 2 {
		t.Fatal("Replace 改写了分配")
	}

	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := applications.Replace(txCtx, replaced)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("已撤销的核销又被撤了一次")
		}
		return nil
	})

	secondMapping := payable
	secondMapping.ContentDigest = "digest-other"
	var mappingOutcome ports.FundsMappingSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := mappings.Save(txCtx, secondMapping)
		if err != nil {
			return err
		}
		mappingOutcome = saved
		winner, found, err := mappings.FindByKey(txCtx, payable.Key)
		if err != nil || !found || winner.ContentDigest != payable.ContentDigest {
			t.Fatalf("同事务读回映射赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if mappingOutcome != ports.FundsMappingAlreadyRecorded {
		t.Fatalf("第二份映射结果 = %d", mappingOutcome)
	}
}

func TestFundsRecordsAreInvisibleAcrossTenants(t *testing.T) {
	facts, mappings, applications, transactor, _ := newFundsStores(t)
	ctx := t.Context()

	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-shared", domain.FundsReceiptConfirmed, 8000)
	mapping := mappingRecord(t, "tenant-a", "mapping-shared", fact.Fact, domain.TargetPayable, "payable-1")
	application := applicationRecord(t, "tenant-a", "application-shared", fact.Fact, mapping.Mapping)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := facts.Save(txCtx, fact); err != nil {
			return err
		}
		if _, err := mappings.Save(txCtx, mapping); err != nil {
			return err
		}
		_, err := applications.Save(txCtx, application)
		return err
	})

	other := saTenant(t, "tenant-b")
	if _, exists, err := facts.FindByKey(ctx, ports.FundsFactKey{TenantID: other, Fact: fact.Key.Fact}); err != nil || exists {
		t.Errorf("另一个租户读到了资金事实：exists=%v err=%v", exists, err)
	}
	if _, exists, err := mappings.FindByKey(ctx, ports.FundsMappingKey{TenantID: other, Mapping: mapping.Key.Mapping}); err != nil || exists {
		t.Errorf("另一个租户读到了映射：exists=%v err=%v", exists, err)
	}
	if _, exists, err := applications.FindByKey(ctx, ports.SettlementApplicationKey{TenantID: other, Application: application.Key.Application}); err != nil || exists {
		t.Errorf("另一个租户读到了核销：exists=%v err=%v", exists, err)
	}
}

func TestFundsWritesRefuseToRunOutsideATransaction(t *testing.T) {
	facts, mappings, applications, _, _ := newFundsStores(t)
	ctx := t.Context()

	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	if _, err := facts.Save(ctx, fact); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 事实：%v", err)
	}
	mapping := mappingRecord(t, "tenant-a", "mapping-1", fact.Fact, domain.TargetPayable, "payable-1")
	if _, err := mappings.Save(ctx, mapping); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 映射：%v", err)
	}
	application := applicationRecord(t, "tenant-a", "application-1", fact.Fact, mapping.Mapping)
	if _, err := applications.Save(ctx, application); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 核销：%v", err)
	}
	if _, err := applications.Replace(ctx, application); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Replace 核销：%v", err)
	}
}

func TestFundsRollbackLeavesNothingBehind(t *testing.T) {
	facts, mappings, applications, transactor, _ := newFundsStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	mapping := mappingRecord(t, "tenant-a", "mapping-1", fact.Fact, domain.TargetPayable, "payable-1")
	application := applicationRecord(t, "tenant-a", "application-1", fact.Fact, mapping.Mapping)

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := facts.Save(txCtx, fact); err != nil {
			return err
		}
		if _, err := mappings.Save(txCtx, mapping); err != nil {
			return err
		}
		if _, err := applications.Save(txCtx, application); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := facts.FindByKey(ctx, fact.Key); err != nil || exists {
		t.Errorf("回滚后事实仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := mappings.FindByKey(ctx, mapping.Key); err != nil || exists {
		t.Errorf("回滚后映射仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := applications.FindByKey(ctx, application.Key); err != nil || exists {
		t.Errorf("回滚后核销仍在：exists=%v err=%v", exists, err)
	}
}

func TestFundsCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, pool := newFundsStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, corrects, corrected_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'f-bad-1', 'src', 'RECEIPT_CONFIRMED', 'USD', 100, 'v1',
		         now(), 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「更正版本等于当前版本」溜进了事实库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, corrects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'f-bad-2', 'src', 'RECEIPT_CONFIRMED', 'USD', 100, 'v2',
		         now(), 'v1', 'd', now())`); err == nil {
		t.Fatal("一行「更正却没有时刻」溜进了事实库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.funds_mapping
			(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis,
			 mapped_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'm-bad-1', 'f-1', 'PAYABLE', 'p-1', '   ', now(), 'd', now())`); err == nil {
		t.Fatal("一行「空依据」溜进了映射库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-1', 'f-1', 'USD', 100, 100, '[]', 'b', now(), 'd', now())`); err == nil {
		t.Fatal("一行「空分配」溜进了核销库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-2', 'f-1', 'USD', 100, 200,
		         '[{"mapping":"m","targetKind":"PAYABLE","target":"p","direction":"DEBIT","amountMinor":200}]',
		         'b', now(), 'd', now())`); err == nil {
		t.Fatal("一行「核销超过事实金额」溜进了核销库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, reversal_basis, reversed_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-3', 'f-1', 'USD', 100, 100,
		         '[{"mapping":"m","targetKind":"PAYABLE","target":"p","direction":"DEBIT","amountMinor":100}]',
		         'b', now(), NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「撤销却没有依据」溜进了核销库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-4', 'f-1', 'USD', 100, 100, NULL, 'b', now(), 'd', now())`); err == nil {
		t.Fatal("一行「分配列为 NULL」按 jsonb 三值缝溜进了核销库")
	}
}

func newFundsStores(t *testing.T) (
	*adapter.ExternalFundsFacts,
	*adapter.FundsMappings,
	*adapter.SettlementApplications,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("构造资金事实库：%v", err)
	}
	mappings, err := adapter.NewFundsMappings(db)
	if err != nil {
		t.Fatalf("构造映射库：%v", err)
	}
	applications, err := adapter.NewSettlementApplications(db)
	if err != nil {
		t.Fatalf("构造核销库：%v", err)
	}
	return facts, mappings, applications, db.Transactor(), pool
}

func adoptedFactRecord(t *testing.T, tenant, id string, kind domain.FundsFactKind, amount int64) ports.FundsFactRecord {
	t.Helper()
	fact, err := domain.AdoptExternalFundsFact(domain.ExternalFundsFactSpec{
		Fact:        saValue(t, domain.NewFundsFactReference, id),
		Source:      saValue(t, domain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Kind:        kind,
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: amount,
		Version:     saValue(t, domain.NewFundsFactVersion, "bank-fact/v1"),
		OccurredAt:  fundsOccurredAt,
	})
	if err != nil {
		t.Fatalf("构造资金事实：%v", err)
	}
	return ports.FundsFactRecord{
		Key:           ports.FundsFactKey{TenantID: saTenant(t, tenant), Fact: fact.Fact()},
		ContentDigest: "digest-" + id,
		Fact:          fact,
		RecordedAt:    fundsOccurredAt,
	}
}

func mappingRecord(
	t *testing.T,
	tenant, id string,
	fact domain.ExternalFundsFact,
	kind domain.SettlementTargetKind,
	target string,
) ports.FundsMappingRecord {
	t.Helper()
	mapping, err := domain.MapFundsToTarget(
		fact,
		saValue(t, domain.NewMappingReference, id),
		kind,
		saValue(t, domain.NewSettlementTargetReference, target),
		saValue(t, domain.NewMappingBasisReference, "payment-instruction-1"),
		fundsMappedAt,
	)
	if err != nil {
		t.Fatalf("构造映射：%v", err)
	}
	return ports.FundsMappingRecord{
		Key:           ports.FundsMappingKey{TenantID: saTenant(t, tenant), Mapping: mapping.Mapping()},
		ContentDigest: "digest-" + id,
		Mapping:       mapping,
		RecordedAt:    fundsMappedAt,
	}
}

func applicationRecord(
	t *testing.T,
	tenant, id string,
	fact domain.ExternalFundsFact,
	mappings ...domain.FundsMapping,
) ports.SettlementApplicationRecord {
	t.Helper()
	allocations := make([]domain.SettlementAllocation, 0, len(mappings))
	remaining := int64(8000)
	for index, mapping := range mappings {
		amount := remaining
		direction := domain.AllocationDebit
		if mapping.TargetKind() == domain.TargetCreditNote {
			amount = 2000
			direction = domain.AllocationCredit
			remaining += 2000
		} else if index == 0 && len(mappings) > 1 {
			amount = 10000
			remaining = 0
		}
		allocations = append(allocations, domain.SettlementAllocation{
			Mapping:     mapping.Mapping(),
			TargetKind:  mapping.TargetKind(),
			Target:      mapping.Target(),
			Direction:   direction,
			AmountMinor: amount,
		})
	}
	application, err := domain.ApplySettlement(
		fact,
		mappings,
		allocations,
		saValue(t, domain.NewApplicationReference, id),
		saValue(t, domain.NewApplicationBasisReference, "offset-authority-1"),
		fundsAppliedAt,
	)
	if err != nil {
		t.Fatalf("构造核销：%v", err)
	}
	return ports.SettlementApplicationRecord{
		Key:           ports.SettlementApplicationKey{TenantID: saTenant(t, tenant), Application: application.Application()},
		ContentDigest: "digest-" + id,
		Application:   application,
		RecordedAt:    fundsAppliedAt,
	}
}
