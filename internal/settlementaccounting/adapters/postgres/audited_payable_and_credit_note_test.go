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

// 本文件对真实 PostgreSQL 16 证 UC-SA-004 步 5–7 的两张判断库：审核应付整行往返后经重建门
// 复验、一行主张只成立一份应付（库内唯一约束）、同身份撞写答`已有记录`且事务保持可用
// （ADR-0031）；供应商费用贷项按（身份+版本）追加、回指原应付而不改它；两者租户隔离、
// 无事务拒、回滚无痕、金额与依据由库内 CHECK 钉住。

var (
	payableAuditedAt   = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	payableRecordedAt  = time.Date(2026, 9, 11, 10, 5, 0, 0, time.UTC)
	creditNoteIssuedAt = time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
)

func TestAnAuditedPayableRoundTripsAndOneLineFormsOnePayable(t *testing.T) {
	payables, _, transactor, _ := newPayableStores(t)
	ctx := t.Context()

	record := payableRecord(t, "tenant-1", "payable-1", "claim-1", "L1", "digest-1")
	mustSavePayable(t, transactor, ctx, payables, record)

	found, present, err := payables.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("按键读回：present=%v err=%v", present, err)
	}
	payable := found.Payable
	if found.ContentDigest != "digest-1" ||
		payable.Claim().String() != "claim-1" || payable.Line().String() != "L1" ||
		payable.Expected().String() != "cost-v1" ||
		payable.LegalEntity().String() != "entity-1" || payable.Account().String() != "supplier-account-1" ||
		payable.Auditor().String() != "auditor-1" ||
		!payable.AuditedAt().Equal(payableAuditedAt) {
		t.Fatalf("应付往返变形：%+v", payable)
	}
	if currency, minor := payable.Amount(); currency.String() != "CNY" || minor != 1000 {
		t.Fatalf("金额往返变形：%s %d", currency, minor)
	}

	byLine, present, err := payables.FindByLine(ctx, record.Key.TenantID,
		saValue(t, domain.NewBillClaimID, "claim-1"), saValue(t, domain.NewBillLineReference, "L1"))
	if err != nil || !present || byLine.Key.Payable.String() != "payable-1" {
		t.Fatalf("按行读回：present=%v err=%v key=%s", present, err, byLine.Key.Payable)
	}

	// 同一行第二份应付：库内唯一约束拦下，答`已有记录`，先到者不动，撞键后同事务仍可读。
	var secondOutcome ports.AuditedPayableSaveOutcome
	var lineWinner ports.AuditedPayableRecord
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		secondOutcome, err = payables.Save(txCtx, payableRecord(t, "tenant-1", "payable-2", "claim-1", "L1", "digest-2"))
		if err != nil {
			return err
		}
		lineWinner, _, err = payables.FindByLine(txCtx, record.Key.TenantID,
			saValue(t, domain.NewBillClaimID, "claim-1"), saValue(t, domain.NewBillLineReference, "L1"))
		return err
	})
	if secondOutcome != ports.AuditedPayableAlreadyRecorded {
		t.Fatalf("second payable outcome = %d, want ALREADY_RECORDED（一行只成立一份应付）", secondOutcome)
	}
	if lineWinner.Key.Payable.String() != "payable-1" {
		t.Fatal("同一行的第二份应付顶替了先到者")
	}

	// 同身份异内容：同样答`已有记录`，冲突由编排比 content_digest。
	var replayOutcome ports.AuditedPayableSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		replayOutcome, err = payables.Save(txCtx, payableRecord(t, "tenant-1", "payable-1", "claim-1", "L2", "digest-forged"))
		return err
	})
	if replayOutcome != ports.AuditedPayableAlreadyRecorded {
		t.Fatalf("replay outcome = %d", replayOutcome)
	}
	kept, _, _ := payables.FindByKey(ctx, record.Key)
	if kept.ContentDigest != "digest-1" || kept.Payable.Line().String() != "L1" {
		t.Fatal("同身份的第二份内容覆盖了先到的应付")
	}
}

func TestASupplierCreditNoteRoundTripsAndKeepsItsPayableLink(t *testing.T) {
	payables, notes, transactor, _ := newPayableStores(t)
	ctx := t.Context()

	payable := payableRecord(t, "tenant-1", "payable-1", "claim-1", "L1", "digest-1")
	mustSavePayable(t, transactor, ctx, payables, payable)
	note := creditNoteRecord(t, "tenant-1", "note-1", "v1", "payable-1", "claim-1", 300, "digest-n1")
	mustSaveCreditNote(t, transactor, ctx, notes, note)

	found, present, err := notes.FindByKey(ctx, note.Key)
	if err != nil || !present {
		t.Fatalf("按键读回：present=%v err=%v", present, err)
	}
	got := found.Note
	if got.Payable().String() != "payable-1" || got.Claim().String() != "claim-1" ||
		got.Account().String() != "supplier-account-1" || got.Reason().String() != "supplier-correction-7" ||
		!got.IssuedAt().Equal(creditNoteIssuedAt) {
		t.Fatalf("贷项往返变形：%+v", got)
	}
	if currency, minor := got.Amount(); currency.String() != "CNY" || minor != 300 {
		t.Fatalf("贷记金额往返变形：%s %d", currency, minor)
	}

	// 贷项落库后原应付一字不动——贷项不静默净入原应付。
	kept, _, _ := payables.FindByKey(ctx, payable.Key)
	if _, minor := kept.Payable.Amount(); minor != 1000 {
		t.Fatalf("原应付金额变成了 %d", minor)
	}

	// 同（身份+版本）第二份：答`已登记`，先到者不动；新版本另成一行。
	var replayOutcome, versionOutcome ports.SupplierCreditNoteSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if replayOutcome, err = notes.Save(txCtx, creditNoteRecord(t, "tenant-1", "note-1", "v1", "payable-1", "claim-1", 999, "digest-forged")); err != nil {
			return err
		}
		versionOutcome, err = notes.Save(txCtx, creditNoteRecord(t, "tenant-1", "note-1", "v2", "payable-1", "claim-1", 250, "digest-n1v2"))
		return err
	})
	if replayOutcome != ports.SupplierCreditNoteAlreadyRecorded {
		t.Fatalf("replay outcome = %d, want ALREADY_RECORDED", replayOutcome)
	}
	if versionOutcome != ports.SupplierCreditNoteSaved {
		t.Fatalf("version outcome = %d, want SAVED（贷项的更正是新版本）", versionOutcome)
	}
	after, _, _ := notes.FindByKey(ctx, note.Key)
	if _, minor := after.Note.Amount(); minor != 300 {
		t.Fatal("同身份同版本的第二份覆盖了先到的贷项")
	}
}

func TestPayableAndCreditNoteScopesAreInvisibleToEachOther(t *testing.T) {
	payables, notes, transactor, _ := newPayableStores(t)
	ctx := t.Context()

	mustSavePayable(t, transactor, ctx, payables, payableRecord(t, "tenant-1", "payable-1", "claim-1", "L1", "digest-1"))
	mustSaveCreditNote(t, transactor, ctx, notes, creditNoteRecord(t, "tenant-1", "note-1", "v1", "payable-1", "claim-1", 300, "digest-n1"))

	if _, present, err := payables.FindByKey(ctx, ports.AuditedPayableKey{
		TenantID: saTenant(t, "tenant-b"), Payable: saValue(t, domain.NewPayableID, "payable-1"),
	}); err != nil || present {
		t.Errorf("他租户按同名键读到了本租户的应付：present=%v err=%v", present, err)
	}
	if _, present, err := payables.FindByLine(ctx, saTenant(t, "tenant-b"),
		saValue(t, domain.NewBillClaimID, "claim-1"), saValue(t, domain.NewBillLineReference, "L1")); err != nil || present {
		t.Errorf("他租户按同一行读到了本租户的应付：present=%v err=%v", present, err)
	}
	if _, present, err := notes.FindByKey(ctx, ports.SupplierCreditNoteKey{
		TenantID: saTenant(t, "tenant-b"),
		Note:     saValue(t, domain.NewCreditNoteID, "note-1"),
		Version:  saValue(t, domain.NewCreditNoteVersion, "v1"),
	}); err != nil || present {
		t.Errorf("他租户按同名键读到了本租户的贷项：present=%v err=%v", present, err)
	}
}

func TestPayableAndCreditNoteWritesRefuseToRunOutsideATransaction(t *testing.T) {
	payables, notes, _, _ := newPayableStores(t)
	ctx := t.Context()

	if _, err := payables.Save(ctx, payableRecord(t, "tenant-1", "payable-1", "claim-1", "L1", "digest-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应付应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := notes.Save(ctx, creditNoteRecord(t, "tenant-1", "note-1", "v1", "payable-1", "claim-1", 300, "digest-n1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存贷项应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestPayableAndCreditNoteRollbackLeavesNothingBehind(t *testing.T) {
	payables, notes, transactor, _ := newPayableStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	payable := payableRecord(t, "tenant-1", "payable-1", "claim-1", "L1", "digest-1")
	note := creditNoteRecord(t, "tenant-1", "note-1", "v1", "payable-1", "claim-1", 300, "digest-n1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := payables.Save(txCtx, payable); err != nil {
			return err
		}
		if _, err := notes.Save(txCtx, note); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, present, err := payables.FindByKey(ctx, payable.Key); err != nil || present {
		t.Errorf("回滚后应付仍在：present=%v err=%v", present, err)
	}
	if _, present, err := notes.FindByKey(ctx, note.Key); err != nil || present {
		t.Errorf("回滚后贷项仍在：present=%v err=%v", present, err)
	}
}

// TestPayableAndCreditNoteCheckConstraintsRejectImpossibleRows 证库面与领域形成门是同一组
// 判据的两份：绕过适配器直插零额应付、空原因贷项都被 CHECK 拒。
func TestPayableAndCreditNoteCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, pool := newPayableStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity,
			 account_id, currency, amount_minor, auditor_ref, audited_at, content_digest, recorded_at)
		 VALUES ('tenant-x', 'payable-x', 'claim-x', 'L1', 'cost-x', 'entity-x',
		         'account-x', 'CNY', 0, 'auditor-x', now(), 'digest-x', now())`); err == nil {
		t.Fatal("一行零额应付进了库")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_credit_note
			(tenant_id, note_id, note_version, payable_id, claim_id, account_id,
			 currency, amount_minor, reason_ref, issued_at, content_digest, recorded_at)
		 VALUES ('tenant-x', 'note-x', 'v1', 'payable-x', 'claim-x', 'account-x',
		         'CNY', 100, '   ', now(), 'digest-x', now())`); err == nil {
		t.Fatal("一行没有原因的贷项进了库")
	}
}

// TestPayableAndCreditNoteEnvelopesShareTheClaimPartition 证步 7 的两份发布各自成封（ID 各
// 带自己的身份，互不吞）且与接收信封同分区（排队主体是账单主张：应付与贷项必须排在它们
// 所审、所贷的那份主张之后）；三种事件类型各不相同。
func TestPayableAndCreditNoteEnvelopesShareTheClaimPartition(t *testing.T) {
	handoff, db, pool := newSupplierBillHandoffFixture(t)
	ctx := t.Context()

	payable := payableRecord(t, "tenant-a", "payable-1", "claim-p", "L1", "digest-1")
	note := creditNoteRecord(t, "tenant-a", "note-1", "v1", "payable-1", "claim-p", 300, "digest-n1")
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{
			Record: billRecord(t, "tenant-a", "claim-p", "v1", "digest-r"),
		}); err != nil {
			return err
		}
		if err := handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{Payable: payable}); err != nil {
			return err
		}
		return handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{CreditNote: note})
	})

	receptionID := "tenant-a/bill/claim-p/v1"
	payableID := "tenant-a/bill/claim-p/payable/payable-1"
	noteID := "tenant-a/bill/claim-p/credit-note/note-1/v1"
	for _, eventID := range []string{receptionID, payableID, noteID} {
		if count := countSAIntents(t, pool, eventID); count != 1 {
			t.Fatalf("%s 信封 = %d, want 1", eventID, count)
		}
		if got := partitionKeyOf(t, pool, eventID); got != "tenant-a/bill/claim-p" {
			t.Fatalf("%s 分区键 = %q，应当与接收同在主张分区", eventID, got)
		}
	}
	if got := settlementApplicationIntentType(t, pool, payableID); got != "settlement-accounting.supplier-bill.payable-audited" {
		t.Fatalf("应付事件类型 = %q", got)
	}
	if got := settlementApplicationIntentType(t, pool, noteID); got != "settlement-accounting.supplier-bill.credit-note-formed" {
		t.Fatalf("贷项事件类型 = %q", got)
	}

	t.Run("an intent carrying two records is refused", func(t *testing.T) {
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffSupplierBill(txCtx, ports.SupplierBillHandoffIntent{Payable: payable, CreditNote: note})
		})
		if err == nil {
			t.Fatal("一封同时装着应付与贷项的意图入了队——两者必须分别发布")
		}
	})
}

// ---- 夹具 ----

func newPayableStores(t *testing.T) (*adapter.AuditedPayables, *adapter.SupplierCreditNotes, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	payables, err := adapter.NewAuditedPayables(db)
	if err != nil {
		t.Fatalf("构造应付库：%v", err)
	}
	notes, err := adapter.NewSupplierCreditNotes(db)
	if err != nil {
		t.Fatalf("构造贷项库：%v", err)
	}
	return payables, notes, db.Transactor(), pool
}

func mustSavePayable(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	payables *adapter.AuditedPayables,
	record ports.AuditedPayableRecord,
) {
	t.Helper()
	var outcome ports.AuditedPayableSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = payables.Save(txCtx, record)
		return err
	})
	if outcome != ports.AuditedPayableSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

func mustSaveCreditNote(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	notes *adapter.SupplierCreditNotes,
	record ports.SupplierCreditNoteRecord,
) {
	t.Helper()
	var outcome ports.SupplierCreditNoteSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = notes.Save(txCtx, record)
		return err
	})
	if outcome != ports.SupplierCreditNoteSaved {
		t.Fatalf("save outcome = %d", outcome)
	}
}

// payableRecord 造一份合法的审核应付：经 RehydrateAuditedPayable 构造——重建门的校验正是
// 「什么算合法」的单一权威（形成门要一份匹配聚合本体，夹具不必重演匹配）。
func payableRecord(t *testing.T, tenant, payableID, claimID, line, digest string) ports.AuditedPayableRecord {
	t.Helper()
	payable, err := domain.RehydrateAuditedPayable(domain.RehydrateAuditedPayableSpec{
		Payable:     saValue(t, domain.NewPayableID, payableID),
		Claim:       saValue(t, domain.NewBillClaimID, claimID),
		Line:        saValue(t, domain.NewBillLineReference, line),
		Expected:    saValue(t, domain.NewSupplierCostVersionID, "cost-v1"),
		LegalEntity: saValue(t, domain.NewLegalEntityReference, "entity-1"),
		Account:     saValue(t, domain.NewSettlementAccountID, "supplier-account-1"),
		Currency:    saValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: 1000,
		Auditor:     saValue(t, domain.NewAuditorReference, "auditor-1"),
		AuditedAt:   payableAuditedAt,
	})
	if err != nil {
		t.Fatalf("构造应付：%v", err)
	}
	return ports.AuditedPayableRecord{
		Key:           ports.AuditedPayableKey{TenantID: saTenant(t, tenant), Payable: payable.Payable()},
		ContentDigest: digest,
		Payable:       payable,
		RecordedAt:    payableRecordedAt,
	}
}

func creditNoteRecord(t *testing.T, tenant, noteID, version, payableID, claimID string, amount int64, digest string) ports.SupplierCreditNoteRecord {
	t.Helper()
	note, err := domain.FormSupplierCreditNote(domain.SupplierCreditNoteSpec{
		Note:        saValue(t, domain.NewCreditNoteID, noteID),
		Version:     saValue(t, domain.NewCreditNoteVersion, version),
		Payable:     saValue(t, domain.NewPayableID, payableID),
		Claim:       saValue(t, domain.NewBillClaimID, claimID),
		Account:     saValue(t, domain.NewSettlementAccountID, "supplier-account-1"),
		Currency:    saValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: amount,
		Reason:      saValue(t, domain.NewCreditReasonReference, "supplier-correction-7"),
		IssuedAt:    creditNoteIssuedAt,
	})
	if err != nil {
		t.Fatalf("构造贷项：%v", err)
	}
	return ports.SupplierCreditNoteRecord{
		Key: ports.SupplierCreditNoteKey{
			TenantID: saTenant(t, tenant), Note: note.Note(), Version: note.Version(),
		},
		ContentDigest: digest,
		Note:          note,
		RecordedAt:    creditNoteIssuedAt.Add(time.Minute),
	}
}
