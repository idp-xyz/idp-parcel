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
	var savedFact ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedFact, err = facts.Save(txCtx, record)
		return err
	})
	if savedFact != ports.FundsFactSaved {
		t.Fatalf("save outcome = %d", savedFact)
	}

	found, exists, err := facts.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if _, amount := found.Fact.Amount(); amount != 8000 || found.Fact.Kind() != domain.FundsReceiptConfirmed {
		t.Fatal("资金事实往返变形")
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.FundsFactSaveOutcome
	var factWinner ports.FundsFactRecord
	var factWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if outcome, err = facts.Save(txCtx, second); err != nil {
			return err
		}
		factWinner, factWinnerFound, err = facts.FindByKey(txCtx, record.Key)
		return err
	})
	if !factWinnerFound || factWinner.ContentDigest != record.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", factWinnerFound, factWinner.ContentDigest)
	}
	if outcome != ports.FundsFactAlreadyAdopted {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

// Covers: 票 sa-cc/20 裁决 1「版本子表」与完成判据 (1)(2)——更正版本 v2（回指 v1）落第二行、身份行不变；
// FindByKey 交回链头 v2；FindVersion(v1) 仍原样、FindVersion(v2) 带回指；同 v2 重放答 FundsFactAlreadyAdopted
// 且不顶替先到者；AdoptedFundsFactView 两版各答各的。
func TestAFundsFactCorrectionVersionAccruesAsASecondRowAndFindByKeyAnswersTheChainHead(t *testing.T) {
	facts, _, _, transactor, pool := newFundsStores(t)
	ctx := t.Context()

	original := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := facts.Save(txCtx, original)
		return err
	})

	correctedFact, err := original.Fact.CorrectAmount(7500,
		saValue(t, domain.NewFundsFactVersion, "bank-fact/v2"), fundsOccurredAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成更正版本：%v", err)
	}
	corrected := ports.FundsFactRecord{
		Key:           original.Key,
		ContentDigest: "digest-bank-fact-1-v2",
		Fact:          correctedFact,
		RecordedAt:    fundsOccurredAt.Add(time.Hour),
	}
	var savedCorrected ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		savedCorrected, err = facts.Save(txCtx, corrected)
		return err
	})
	if savedCorrected != ports.FundsFactSaved {
		t.Fatalf("更正版本 save = %d, want FundsFactSaved——身份已在、版本是新的，这正是要开的口", savedCorrected)
	}

	var identityRows, versionRows int
	if err := pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM settlement_accounting.external_funds_fact WHERE tenant_id = $1 AND fact_id = $2),
		        (SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = $1 AND fact_id = $2)`,
		"tenant-a", "bank-fact-1").Scan(&identityRows, &versionRows); err != nil {
		t.Fatalf("直读两表：%v", err)
	}
	if identityRows != 1 || versionRows != 2 {
		t.Fatalf("身份行 = %d 版本行 = %d, want 1 / 2", identityRows, versionRows)
	}

	head, found, err := facts.FindByKey(ctx, original.Key)
	if err != nil || !found {
		t.Fatalf("读链头：found=%v err=%v", found, err)
	}
	if head.Fact.Version().String() != "bank-fact/v2" || head.ContentDigest != corrected.ContentDigest {
		t.Fatalf("链头 = %q（摘要 %q），want v2", head.Fact.Version(), head.ContentDigest)
	}
	if predecessor, present := head.Fact.Corrects(); !present || predecessor != original.Fact.Version() {
		t.Fatalf("链头回指 = (%q, %v), want v1", predecessor, present)
	}
	if _, amount := head.Fact.Amount(); amount != 7500 {
		t.Fatalf("链头金额 = %d, want 7500", amount)
	}

	first, found, err := facts.FindVersion(ctx, original.Key, original.Fact.Version())
	if err != nil || !found {
		t.Fatalf("按版本读 v1：found=%v err=%v", found, err)
	}
	if first.Fact != original.Fact || first.ContentDigest != original.ContentDigest {
		t.Fatalf("v1 读回 = %#v, want 与采用时同值——更正不改写前版", first.Fact)
	}
	if _, found, err := facts.FindVersion(ctx, original.Key, saValue(t, domain.NewFundsFactVersion, "bank-fact/v3")); err != nil || found {
		t.Fatalf("从未采用的版本：found=%v err=%v, want false", found, err)
	}

	replay := corrected
	replay.ContentDigest = "digest-other"
	var replayed ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		replayed, err = facts.Save(txCtx, replay)
		return err
	})
	if replayed != ports.FundsFactAlreadyAdopted {
		t.Fatalf("同版本重放 save = %d, want FundsFactAlreadyAdopted", replayed)
	}
	if winner, _, _ := facts.FindVersion(ctx, original.Key, correctedFact.Version()); winner.ContentDigest != corrected.ContentDigest {
		t.Fatalf("重放顶替了先到的 v2：摘要 = %q", winner.ContentDigest)
	}

	view, err := adapter.NewAdoptedFundsFactView(mustDB(t, pool))
	if err != nil {
		t.Fatalf("构造只读视图：%v", err)
	}
	if loaded, found, err := view.LoadAdoptedFundsFact(ctx, original.Key.TenantID, original.Key.Fact, original.Fact.Version()); err != nil || !found || loaded != original.Fact {
		t.Fatalf("视图读 v1：found=%v err=%v loaded=%#v", found, err, loaded)
	}
	if loaded, found, err := view.LoadAdoptedFundsFact(ctx, original.Key.TenantID, original.Key.Fact, correctedFact.Version()); err != nil || !found || loaded != correctedFact {
		t.Fatalf("视图读 v2：found=%v err=%v loaded=%#v", found, err, loaded)
	}
}

// Covers: 0021 头注「校验版本链的形」——本上下文是铸造方，链必须线性：第二个首版与同一前版的第二次更正
// 都落不进去、折成 FundsFactAlreadyAdopted（编排照竞态输家那样读回链头）；回指一个不存在的版本是外键错。
// 三样之后链头仍唯一且仍是 v2，FindByKey 因此不需要标记列。
func TestTheDatabaseKeepsAFundsFactVersionChainLinear(t *testing.T) {
	facts, _, _, transactor, pool := newFundsStores(t)
	ctx := t.Context()

	original := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := facts.Save(txCtx, original)
		return err
	})

	secondFirstVersion := original
	secondFirstVersion.ContentDigest = "digest-v1-bis"
	secondFirstVersion.Fact = correctedFactFrom(t, original.Fact, 8000, "bank-fact/v1-bis", true)
	var secondFirstOutcome ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		secondFirstOutcome, err = facts.Save(txCtx, secondFirstVersion)
		return err
	})
	if secondFirstOutcome != ports.FundsFactAlreadyAdopted {
		t.Fatalf("同一事实的第二个首版 save = %d, want FundsFactAlreadyAdopted（首版已采用）", secondFirstOutcome)
	}

	corrected := original
	corrected.ContentDigest = "digest-v2"
	corrected.Fact = correctedFactFrom(t, original.Fact, 7500, "bank-fact/v2", false)
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := facts.Save(txCtx, corrected)
		return err
	})
	fork := original
	fork.ContentDigest = "digest-v2-fork"
	fork.Fact = correctedFactFrom(t, original.Fact, 7000, "bank-fact/v2-fork", false)
	var forkOutcome ports.FundsFactSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		forkOutcome, err = facts.Save(txCtx, fork)
		return err
	})
	if forkOutcome != ports.FundsFactAlreadyAdopted {
		t.Fatalf("同一前版的第二次更正 save = %d, want FundsFactAlreadyAdopted（那一版的更正已采用）", forkOutcome)
	}

	dangling := original
	dangling.ContentDigest = "digest-v10"
	dangling.Fact = correctedFactFrom(t, corrected.Fact, 7000, "bank-fact/v9", false)
	dangling.Fact = correctedFactFrom(t, dangling.Fact, 6000, "bank-fact/v10", false)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := facts.Save(txCtx, dangling)
		return err
	}); err == nil {
		t.Fatal("回指一个从未采用的版本的更正溜进了版本表")
	}

	var versionRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = $1 AND fact_id = $2`,
		"tenant-a", "bank-fact-1").Scan(&versionRows); err != nil {
		t.Fatalf("直读版本表：%v", err)
	}
	if versionRows != 2 {
		t.Fatalf("版本行 = %d, want 2——三样被拒的一行都不该落", versionRows)
	}
	head, found, err := facts.FindByKey(ctx, original.Key)
	if err != nil || !found || head.Fact.Version().String() != "bank-fact/v2" {
		t.Fatalf("三次被拒之后链头该仍是 v2：found=%v err=%v got=%q", found, err, head.Fact.Version())
	}
}

// correctedFactFrom 从 base 形成一个新版本；asFirstVersion 为 true 时把它重铸成没有回指的首版（借 Rehydrate 走
// 读回门），用来造「第二个首版」这种只在库门口才该被拒的形。
func correctedFactFrom(t *testing.T, base domain.ExternalFundsFact, amount int64, version string, asFirstVersion bool) domain.ExternalFundsFact {
	t.Helper()
	if asFirstVersion {
		currency, _ := base.Amount()
		fact, err := domain.RehydrateExternalFundsFact(domain.RehydrateExternalFundsFactSpec{
			Fact:        base.Fact(),
			Source:      base.Source(),
			Kind:        base.Kind(),
			Currency:    currency,
			AmountMinor: amount,
			Version:     saValue(t, domain.NewFundsFactVersion, version),
			OccurredAt:  base.OccurredAt(),
		})
		if err != nil {
			t.Fatalf("重铸首版：%v", err)
		}
		return fact
	}
	fact, err := base.CorrectAmount(amount, saValue(t, domain.NewFundsFactVersion, version), base.OccurredAt().Add(time.Hour))
	if err != nil {
		t.Fatalf("形成更正版本：%v", err)
	}
	return fact
}

func mustDB(t *testing.T, pool *pgxpool.Pool) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

func TestAMappingAndApplicationRoundTripAndReplaceOnlyWritesReversal(t *testing.T) {
	facts, mappings, applications, transactor, _ := newFundsStores(t)
	ctx := t.Context()

	fact := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	payable := mappingRecord(t, "tenant-a", "mapping-1", fact.Fact, domain.TargetPayable, "payable-1")
	credit := mappingRecord(t, "tenant-a", "mapping-2", fact.Fact, domain.TargetCreditNote, "credit-note-1")
	application := applicationRecord(t, "tenant-a", "application-1", fact.Fact, payable.Mapping, credit.Mapping)
	var savedApplication ports.SettlementApplicationSaveOutcome
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
		var err error
		savedApplication, err = applications.Save(txCtx, application)
		return err
	})
	if savedApplication != ports.SettlementApplicationSaved {
		t.Fatalf("application save = %d", savedApplication)
	}

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
	var reversalApplied bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		reversalApplied, err = applications.Replace(txCtx, replaced)
		return err
	})
	if !reversalApplied {
		t.Fatal("撤销 Replace 答 false")
	}

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

	var secondReversalApplied bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		secondReversalApplied, err = applications.Replace(txCtx, replaced)
		return err
	})
	if secondReversalApplied {
		t.Fatal("已撤销的核销又被撤了一次")
	}

	secondMapping := payable
	secondMapping.ContentDigest = "digest-other"
	var mappingOutcome ports.FundsMappingSaveOutcome
	var mappingWinner ports.FundsMappingRecord
	var mappingWinnerFound bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if mappingOutcome, err = mappings.Save(txCtx, secondMapping); err != nil {
			return err
		}
		mappingWinner, mappingWinnerFound, err = mappings.FindByKey(txCtx, payable.Key)
		return err
	})
	if !mappingWinnerFound || mappingWinner.ContentDigest != payable.ContentDigest {
		t.Fatalf("同事务读回映射赢家失败：found=%v digest=%q",
			mappingWinnerFound, mappingWinner.ContentDigest)
	}
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

	// 身份行先落（0021 起内容在版本子表），下面两行坏的是版本行自己的形，不是缺身份。
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact (tenant_id, fact_id, recorded_at)
		 VALUES ('tenant-a', 'f-bad-1', now()), ('tenant-a', 'f-bad-2', now())`); err != nil {
		t.Fatalf("预铺身份行：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, corrects, corrected_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'f-bad-1', 'src', 'RECEIPT_CONFIRMED', 'USD', 100, 'v1',
		         now(), 'v1', now(), 'd', now())`); err == nil {
		t.Fatal("一行「更正版本等于当前版本」溜进了事实库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, corrects, content_digest, recorded_at)
		 VALUES ('tenant-a', 'f-bad-2', 'src', 'RECEIPT_CONFIRMED', 'USD', 100, 'v2',
		         now(), 'v1', 'd', now())`); err == nil {
		t.Fatal("一行「更正却没有时刻」溜进了事实库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, source_ref, kind, currency, amount_minor, version,
			 occurred_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'f-orphan', 'src', 'RECEIPT_CONFIRMED', 'USD', 100, 'v1',
		         now(), 'd', now())`); err == nil {
		t.Fatal("一行没有身份行的版本溜进了版本表")
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
