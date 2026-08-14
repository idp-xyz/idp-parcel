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

// 本文件对真实 PostgreSQL 16 证账单接收库与对账单库的行为：整行往返后经领域重建门
// 复验（五值匹配矩阵、勾稽、作废留痕）、同键撞写答`已有记录`且事务保持可用
// （ADR-0031）、作废只写留痕两列且不二废、留痕一致性由库内 CHECK 钉住、租户隔离、
// 无事务拒、回滚无痕。

var (
	billReceivedAt  = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	billMatchedAt   = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	statementCutAt  = time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC)
	statementVoidAt = time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	billRecordedAt  = time.Date(2026, 9, 10, 9, 5, 0, 0, time.UTC)
)

func TestABillReceptionRoundTripsWithItsMatches(t *testing.T) {
	bills, _, transactor, _ := newBillStores(t)
	ctx := t.Context()

	record := billRecord(t, "tenant-1", "claim-1", "v1", "digest-1")
	mustSaveBill(t, transactor, ctx, bills, record)

	found, present, err := bills.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("按键读回：present=%v err=%v", present, err)
	}
	if found.ContentDigest != "digest-1" || !found.AuditAuthorityConfigured {
		t.Fatalf("记录头部往返变形：%+v", found)
	}
	claim := found.Claim
	if claim.Supplier().String() != "supplier-1" ||
		claim.Period().String() != "2026-09" ||
		claim.Currency().String() != "CNY" ||
		claim.TotalClaimedMinor() != 1500 ||
		!claim.ReceivedAt().Equal(billReceivedAt) {
		t.Fatalf("主张往返变形：%+v", claim)
	}
	if supplements, present := claim.Supplements(); !present || supplements.String() != "claim-0" {
		t.Fatal("追加主张的回指没有随主张往返")
	}
	matches := found.Matches
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3", len(matches))
	}
	if matches[0].Classification() != domain.LineMatched || matches[0].VarianceMinor() != 0 {
		t.Fatal("已匹配格往返变形")
	}
	if matches[1].Classification() != domain.PriceVariance || matches[1].VarianceMinor() != 200 {
		t.Fatal("价差格（含差额方向）往返变形")
	}
	basis, hasBasis := matches[1].Basis()
	if !hasBasis || basis.String() != "agreed-price-v3" {
		t.Fatal("差异依据没有随匹配往返")
	}
	if matches[2].Classification() != domain.NoMatchingOccurrence {
		t.Fatal("无匹配发生项格往返变形")
	}
	if _, hasExpected := matches[2].Expected(); hasExpected {
		t.Fatal("无匹配发生项长出了预期成本")
	}
}

// TestReceivingTheSameClaimTwiceKeepsTheFirstReception 证账单接收的写入代数：同幂等
// 三维第二份（哪怕内容不同）答`已有记录`，撞键后同事务读回的仍是先到者——版本冲突的
// 判定材料（content_digest）由读回交给编排。
func TestReceivingTheSameClaimTwiceKeepsTheFirstReception(t *testing.T) {
	bills, _, transactor, _ := newBillStores(t)
	ctx := t.Context()

	first := billRecord(t, "tenant-1", "claim-1", "v1", "digest-1")
	mustSaveBill(t, transactor, ctx, bills, first)

	var forgedOutcome ports.BillSaveOutcome
	var billWinner ports.BillReceptionRecord
	var billWinnerPresent bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		forged := billRecord(t, "tenant-1", "claim-1", "v1", "digest-forged")
		var err error
		if forgedOutcome, err = bills.Save(txCtx, forged); err != nil {
			return err
		}
		billWinner, billWinnerPresent, err = bills.FindByKey(txCtx, first.Key)
		return err
	})
	if forgedOutcome != ports.BillAlreadyRecorded {
		t.Fatalf("replay outcome = %d, want ALREADY_RECORDED", forgedOutcome)
	}
	if !billWinnerPresent {
		t.Fatal("撞键后同事务读回失败")
	}
	if billWinner.ContentDigest != "digest-1" {
		t.Fatal("迟到的版本覆盖了先到的接收")
	}
}

func TestBillScopesAreInvisibleToEachOther(t *testing.T) {
	bills, _, transactor, _ := newBillStores(t)
	ctx := t.Context()

	record := billRecord(t, "tenant-1", "claim-1", "v1", "digest-1")
	mustSaveBill(t, transactor, ctx, bills, record)

	otherKey := record.Key
	otherKey.TenantID = saTenant(t, "tenant-b")
	if _, present, err := bills.FindByKey(ctx, otherKey); err != nil || present {
		t.Errorf("他租户按同名键读到了本租户的接收：present=%v err=%v", present, err)
	}
}

func TestAPublishedStatementRoundTripsWholly(t *testing.T) {
	_, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()

	record := statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1")
	mustSaveStatement(t, transactor, ctx, statements, record)

	found, present, err := statements.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("按键读回：present=%v err=%v", present, err)
	}
	statement := found.Statement
	if statement.Account().String() != "account-1" ||
		statement.Period().String() != "2026-09" ||
		statement.Currency().String() != "CNY" ||
		statement.TotalMinor() != 1400 ||
		!statement.PublishedAt().Equal(statementCutAt.Add(time.Hour)) {
		t.Fatalf("对账单头部往返变形：%+v", statement)
	}
	if lines := statement.Lines(); len(lines) != 2 || lines[0].AmountMinor != 1000 {
		t.Fatal("费用行没有随对账单往返")
	}
	adjustments := statement.AdjustmentLines()
	if len(adjustments) != 2 ||
		adjustments[0].Direction != domain.AdjustmentDebit ||
		adjustments[1].Direction != domain.AdjustmentCredit {
		t.Fatal("调整行（含借贷方向）没有随对账单往返")
	}
	if _, _, voided := statement.Voided(); voided {
		t.Fatal("新发布的对账单读回成了已作废")
	}
}

// TestPublishingTheSameNumberTwiceKeepsTheFirst 证同单号第二次发布答`已发布`且不覆盖
// ——同号异文是冲突还是重放由编排比 content_digest，库只保证先到者不动。
func TestPublishingTheSameNumberTwiceKeepsTheFirst(t *testing.T) {
	_, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()

	mustSaveStatement(t, transactor, ctx, statements, statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1"))

	var forgedOutcome ports.StatementSaveOutcome
	var statementWinner ports.StatementRecord
	var statementWinnerPresent bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		forgedOutcome, err = statements.Save(
			txCtx, statementRecord(t, "tenant-1", "STMT-2026-09", "digest-forged"))
		if err != nil {
			return err
		}
		statementWinner, statementWinnerPresent, err = statements.FindByKey(txCtx, ports.StatementKey{
			TenantID: saTenant(t, "tenant-1"),
			Number:   saValue(t, domain.NewStatementNumber, "STMT-2026-09"),
		})
		return err
	})
	if forgedOutcome != ports.StatementAlreadyPublished {
		t.Fatalf("replay outcome = %d, want ALREADY_PUBLISHED", forgedOutcome)
	}
	if !statementWinnerPresent {
		t.Fatal("撞键后同事务读回失败")
	}
	if statementWinner.ContentDigest != "digest-1" {
		t.Fatal("迟到的发布覆盖了先到的快照")
	}
}

// TestVoidLeavesATraceWithoutErasingContent 证作废留痕的三面：Replace 后依据与时刻
// 可读而行与总额原样；已作废的行再 Replace 答 false（不二废）；未作废的快照拿来
// Replace 是编排缺陷，响亮报错。
func TestVoidLeavesATraceWithoutErasingContent(t *testing.T) {
	_, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()

	record := statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1")
	mustSaveStatement(t, transactor, ctx, statements, record)

	var unvoidedReplaceErr error
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		_, unvoidedReplaceErr = statements.Replace(txCtx, record)
		return nil
	})
	if unvoidedReplaceErr == nil {
		t.Fatal("未作废的快照被 Replace 接受了")
	}

	voided, err := record.Statement.Void(
		saValue(t, domain.NewStatementVoidBasisReference, "billing-error-42"), statementVoidAt)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	record.Statement = voided
	record.RecordedAt = statementVoidAt

	var firstVoid bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		firstVoid, err = statements.Replace(txCtx, record)
		return err
	})
	if !firstVoid {
		t.Fatal("首次作废应换值成功")
	}

	found, present, err := statements.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("读回：present=%v err=%v", present, err)
	}
	basis, voidedAt, isVoided := found.Statement.Voided()
	if !isVoided || basis.String() != "billing-error-42" || !voidedAt.Equal(statementVoidAt) {
		t.Fatal("作废留痕没有读回")
	}
	if found.Statement.TotalMinor() != 1400 || len(found.Statement.Lines()) != 2 {
		t.Fatal("作废把内容也废了——留痕不是删内容")
	}

	var secondVoid bool
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		secondVoid, err = statements.Replace(txCtx, record)
		return err
	})
	if secondVoid {
		t.Fatal("已作废的行被第二次作废改写了")
	}
}

// TestVoidTraceIsPinnedInTheDatabase 证留痕一致性的库面：绕过适配器直插「有时刻无
// 依据」的行被 CHECK 拒。
func TestVoidTraceIsPinnedInTheDatabase(t *testing.T) {
	_, _, _, pool := newBillStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency,
			 total_minor, content_digest, lines, adjustment_lines,
			 published_at, void_basis, voided_at, recorded_at)
		 VALUES ('tenant-1', 'STMT-X', 'account-1', '2026-09', 'CNY',
		         100, 'digest-x', '[{"charge":"c1","amount_minor":100}]', '[]',
		         now(), NULL, now(), now())`); err == nil {
		t.Fatal("一行「有作废时刻无作废依据」进了对账单库")
	}
}

func TestStatementScopesAreInvisibleToEachOther(t *testing.T) {
	_, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()

	mustSaveStatement(t, transactor, ctx, statements, statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1"))

	if _, present, err := statements.FindByKey(ctx, ports.StatementKey{
		TenantID: saTenant(t, "tenant-b"),
		Number:   saValue(t, domain.NewStatementNumber, "STMT-2026-09"),
	}); err != nil || present {
		t.Errorf("他租户按同名单号读到了本租户的对账单：present=%v err=%v", present, err)
	}
}

func TestBillAndStatementWritesRefuseToRunOutsideATransaction(t *testing.T) {
	bills, statements, _, _ := newBillStores(t)
	ctx := t.Context()

	if _, err := bills.Save(ctx, billRecord(t, "tenant-1", "claim-1", "v1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存接收应返回 ErrTransactionRequired，实得：%v", err)
	}
	record := statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1")
	if _, err := statements.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务发布应返回 ErrTransactionRequired，实得：%v", err)
	}
	voided, err := record.Statement.Void(
		saValue(t, domain.NewStatementVoidBasisReference, "billing-error-42"), statementVoidAt)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	record.Statement = voided
	if _, err := statements.Replace(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务作废应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestBillAndStatementRollbackLeavesNothingBehind(t *testing.T) {
	bills, statements, transactor, _ := newBillStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	bill := billRecord(t, "tenant-1", "claim-1", "v1", "digest-1")
	statement := statementRecord(t, "tenant-1", "STMT-2026-09", "digest-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := bills.Save(txCtx, bill); err != nil {
			return err
		}
		if _, err := statements.Save(txCtx, statement); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, present, err := bills.FindByKey(ctx, bill.Key); err != nil || present {
		t.Errorf("回滚后接收仍在：present=%v err=%v", present, err)
	}
	if _, present, err := statements.FindByKey(ctx, statement.Key); err != nil || present {
		t.Errorf("回滚后对账单仍在：present=%v err=%v", present, err)
	}
}

// ---- 夹具 ----

func newBillStores(t *testing.T) (*adapter.BillReceptions, *adapter.CustomerStatements, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	bills, err := adapter.NewBillReceptions(db)
	if err != nil {
		t.Fatalf("构造接收库：%v", err)
	}
	statements, err := adapter.NewCustomerStatements(db)
	if err != nil {
		t.Fatalf("构造对账单库：%v", err)
	}
	return bills, statements, db.Transactor(), pool
}

func saWithin(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveBill(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	bills *adapter.BillReceptions,
	record ports.BillReceptionRecord,
) {
	t.Helper()
	var outcome ports.BillSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = bills.Save(txCtx, record)
		return err
	})
	if outcome != ports.BillSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

func mustSaveStatement(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	statements *adapter.CustomerStatements,
	record ports.StatementRecord,
) {
	t.Helper()
	var outcome ports.StatementSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = statements.Save(txCtx, record)
		return err
	})
	if outcome != ports.StatementSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

// billRecord 造一份三格齐备的接收：已匹配、价差（带依据与 +200 差额）、无匹配发生项
// ——五值矩阵里形状最不同的三格，往返要保住的每一类内容它都有。匹配经
// RehydrateBillLineMatch 构造：重建门的校验正是「什么算合法」的单一权威。
func billRecord(t *testing.T, tenant, claimID, version, digest string) ports.BillReceptionRecord {
	t.Helper()

	key := ports.BillReceptionKey{
		TenantID: saTenant(t, tenant),
		Claim:    saValue(t, domain.NewBillClaimID, claimID),
		Version:  saValue(t, domain.NewBillClaimVersion, version),
	}
	claim, err := domain.ReceiveSupplierBillClaim(domain.SupplierBillClaimSpec{
		Claim:       key.Claim,
		Version:     key.Version,
		Supplier:    saValue(t, domain.NewSupplierPartyReference, "supplier-1"),
		LegalEntity: saValue(t, domain.NewLegalEntityReference, "entity-1"),
		Period:      saValue(t, domain.NewBillingPeriodReference, "2026-09"),
		Currency:    saValue(t, domain.NewCurrencyCode, "CNY"),
		Lines: []domain.BillLine{
			{Line: saValue(t, domain.NewBillLineReference, "L1"), FeeItem: saValue(t, domain.NewFeeItemReference, "fee-a"), ClaimedMinor: 1000},
			{Line: saValue(t, domain.NewBillLineReference, "L2"), FeeItem: saValue(t, domain.NewFeeItemReference, "fee-b"), ClaimedMinor: 500},
		},
		ReceivedAt:       billReceivedAt,
		SupplementsClaim: saValue(t, domain.NewBillClaimID, "claim-0"),
	})
	if err != nil {
		t.Fatalf("构造主张：%v", err)
	}

	match := func(spec domain.RehydrateBillLineMatchSpec) domain.BillLineMatch {
		spec.Claim = key.Claim
		spec.Currency = claim.Currency()
		spec.MatchedAt = billMatchedAt
		formed, err := domain.RehydrateBillLineMatch(spec)
		if err != nil {
			t.Fatalf("构造匹配：%v", err)
		}
		return formed
	}
	return ports.BillReceptionRecord{
		Key:           key,
		ContentDigest: digest,
		Claim:         claim,
		Matches: []domain.BillLineMatch{
			match(domain.RehydrateBillLineMatchSpec{
				Line:           saValue(t, domain.NewBillLineReference, "L1"),
				Classification: domain.LineMatched,
				Expected:       saValue(t, domain.NewSupplierCostVersionID, "cost-v1"),
				ClaimedMinor:   1000,
				ExpectedMinor:  1000,
			}),
			match(domain.RehydrateBillLineMatchSpec{
				Line:           saValue(t, domain.NewBillLineReference, "L2"),
				Classification: domain.PriceVariance,
				Expected:       saValue(t, domain.NewSupplierCostVersionID, "cost-v2"),
				ClaimedMinor:   500,
				ExpectedMinor:  300,
				Basis:          saValue(t, domain.NewMatchBasisReference, "agreed-price-v3"),
			}),
			match(domain.RehydrateBillLineMatchSpec{
				Line:           saValue(t, domain.NewBillLineReference, "L2"),
				Classification: domain.NoMatchingOccurrence,
				ClaimedMinor:   500,
				Basis:          saValue(t, domain.NewMatchBasisReference, "occurrence-search-2026-09"),
			}),
		},
		AuditAuthorityConfigured: true,
		RecordedAt:               billRecordedAt,
	}
}

// statementRecord 造一份两行两调整（借+贷）的发布快照，总额 1400 与行勾稽。快照经
// RehydratePublishedStatement 构造——夹具要的是一份合法的已发布对账单，而重建门的
// 校验正是「什么算合法」的单一权威。
func statementRecord(t *testing.T, tenant, number, digest string) ports.StatementRecord {
	t.Helper()

	statement, err := domain.RehydratePublishedStatement(domain.RehydratePublishedStatementSpec{
		Number:   saValue(t, domain.NewStatementNumber, number),
		Account:  saValue(t, domain.NewSettlementAccountID, "account-1"),
		Period:   saValue(t, domain.NewBillingPeriodReference, "2026-09"),
		Currency: saValue(t, domain.NewCurrencyCode, "CNY"),
		Lines: []domain.StatementLine{
			{Charge: saValue(t, domain.NewCustomerChargeID, "charge-1"), AmountMinor: 1000},
			{Charge: saValue(t, domain.NewCustomerChargeID, "charge-2"), AmountMinor: 500},
		},
		Adjustments: []domain.StatementAdjustmentLine{
			{
				Adjustment:  saValue(t, domain.NewChargeAdjustmentID, "adj-1"),
				Charge:      saValue(t, domain.NewCustomerChargeID, "charge-1"),
				Direction:   domain.AdjustmentDebit,
				AmountMinor: 200,
			},
			{
				Adjustment:  saValue(t, domain.NewChargeAdjustmentID, "adj-2"),
				Charge:      saValue(t, domain.NewCustomerChargeID, "charge-2"),
				Direction:   domain.AdjustmentCredit,
				AmountMinor: 300,
			},
		},
		TotalMinor:  1400,
		PublishedAt: statementCutAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("构造对账单：%v", err)
	}
	return ports.StatementRecord{
		Key: ports.StatementKey{
			TenantID: saTenant(t, tenant),
			Number:   saValue(t, domain.NewStatementNumber, number),
		},
		ContentDigest: digest,
		Statement:     statement,
		RecordedAt:    statementCutAt.Add(time.Hour),
	}
}
