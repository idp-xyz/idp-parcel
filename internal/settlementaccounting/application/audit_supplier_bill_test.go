package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var billAuditedAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

type payableStoreDouble struct {
	records map[string]ports.AuditedPayableRecord
	findErr error
	saveErr error
	saves   int
}

func newPayableStore() *payableStoreDouble {
	return &payableStoreDouble{records: map[string]ports.AuditedPayableRecord{}}
}

func payableStoreKey(key ports.AuditedPayableKey) string {
	return key.TenantID.String() + "|" + key.Payable.String()
}

func (double *payableStoreDouble) FindByKey(
	_ context.Context,
	key ports.AuditedPayableKey,
) (ports.AuditedPayableRecord, bool, error) {
	if double.findErr != nil {
		return ports.AuditedPayableRecord{}, false, double.findErr
	}
	record, found := double.records[payableStoreKey(key)]
	return record, found, nil
}

func (double *payableStoreDouble) FindByLine(
	_ context.Context,
	tenant domain.TenantID,
	claim domain.BillClaimID,
	line domain.BillLineReference,
) (ports.AuditedPayableRecord, bool, error) {
	if double.findErr != nil {
		return ports.AuditedPayableRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Payable.Claim() == claim && record.Payable.Line() == line {
			return record, true, nil
		}
	}
	return ports.AuditedPayableRecord{}, false, nil
}

func (double *payableStoreDouble) Save(
	_ context.Context,
	record ports.AuditedPayableRecord,
) (ports.AuditedPayableSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.AuditedPayableSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[payableStoreKey(record.Key)]; exists {
		return ports.AuditedPayableAlreadyRecorded, nil
	}
	for _, existing := range double.records {
		// 同一行第二份应付：库上的唯一约束答`已有记录`，让编排读回赢家。
		if existing.Payable.Claim() == record.Payable.Claim() && existing.Payable.Line() == record.Payable.Line() {
			return ports.AuditedPayableAlreadyRecorded, nil
		}
	}
	double.records[payableStoreKey(record.Key)] = record
	return ports.AuditedPayableSaved, nil
}

type accountViewDouble struct {
	configured bool
	err        error
}

func (double *accountViewDouble) LoadSupplierPayableAccount(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SupplierPartyReference,
	_ domain.LegalEntityReference,
	_ domain.CurrencyCode,
) (domain.SettlementAccountID, bool, error) {
	if double.err != nil {
		return domain.SettlementAccountID{}, false, double.err
	}
	if !double.configured {
		return domain.SettlementAccountID{}, false, nil
	}
	account, err := domain.NewSettlementAccountID("supplier-account-1")
	return account, true, err
}

type creditNoteStoreDouble struct {
	records map[string]ports.SupplierCreditNoteRecord
	findErr error
	saveErr error
	saves   int
}

func newCreditNoteStore() *creditNoteStoreDouble {
	return &creditNoteStoreDouble{records: map[string]ports.SupplierCreditNoteRecord{}}
}

func creditNoteStoreKey(key ports.SupplierCreditNoteKey) string {
	return key.TenantID.String() + "|" + key.Note.String() + "|" + key.Version.String()
}

func (double *creditNoteStoreDouble) FindByKey(
	_ context.Context,
	key ports.SupplierCreditNoteKey,
) (ports.SupplierCreditNoteRecord, bool, error) {
	if double.findErr != nil {
		return ports.SupplierCreditNoteRecord{}, false, double.findErr
	}
	record, found := double.records[creditNoteStoreKey(key)]
	return record, found, nil
}

func (double *creditNoteStoreDouble) Save(
	_ context.Context,
	record ports.SupplierCreditNoteRecord,
) (ports.SupplierCreditNoteSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.SupplierCreditNoteSaveOutcomeInvalid, double.saveErr
	}
	if _, exists := double.records[creditNoteStoreKey(record.Key)]; exists {
		return ports.SupplierCreditNoteAlreadyRecorded, nil
	}
	double.records[creditNoteStoreKey(record.Key)] = record
	return ports.SupplierCreditNoteSaved, nil
}

func auditCommand(t *testing.T, claimID, line, payable string) application.AuditBillLineCommand {
	t.Helper()
	return application.AuditBillLineCommand{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"),
		Claim:    billValue(t, domain.NewBillClaimID, claimID),
		Version:  billValue(t, domain.NewBillClaimVersion, claimID+"/v1"),
		Line:     billValue(t, domain.NewBillLineReference, line),
		Payable:  billValue(t, domain.NewPayableID, payable),
	}
}

func creditCommand(t *testing.T, note, payable string, amount int64) application.FormSupplierCreditNoteCommand {
	t.Helper()
	return application.FormSupplierCreditNoteCommand{
		TenantID:    billValue(t, domain.NewTenantID, "tenant-1"),
		Note:        billValue(t, domain.NewCreditNoteID, note),
		Version:     billValue(t, domain.NewCreditNoteVersion, note+"/v1"),
		Payable:     billValue(t, domain.NewPayableID, payable),
		AmountMinor: amount,
		Reason:      billValue(t, domain.NewCreditReasonReference, "supplier-correction-1"),
		IssuedAt:    billAuditedAt.Add(24 * time.Hour),
	}
}

// receivedFixture 先把一份账单推进到接收与逐行匹配——审核只能从已提交的匹配开始，不从命令
// 里的金额开始。
func receivedFixture(t *testing.T) *billFixture {
	t.Helper()
	fixture := newBillFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	return fixture
}

// Covers: `AT-SA-089`「审核通过部分账单 → 只对通过金额形成审核应付，其他范围保持原状态」与
// `AT-SA-087`「无争议范围可先审核，争议范围独立保留」的编排面（UC-SA-004 步 5）：`已匹配`
// 行形成应付，金额与币种从匹配取；价差行拒审（走争议，不被顺手接受）；应付按身份幂等、同一
// 行只成立一份应付；步 7 的发布随之交出。
func TestAMatchedLineFormsAnAuditedPayable(t *testing.T) {
	recordType := reflect.TypeOf(ports.AuditedPayableRecord{})
	for index := 0; index < recordType.NumField(); index++ {
		name := strings.ToLower(recordType.Field(index).Name)
		if strings.Contains(name, "payment") || strings.Contains(name, "paid") || strings.Contains(name, "net") {
			t.Fatalf("AuditedPayableRecord 携带 %q——审核应付就能被读成已付款或净额", recordType.Field(index).Name)
		}
	}

	fixture := receivedFixture(t)
	result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if result.Outcome() != application.PayableFormed {
		t.Fatalf("outcome = %q, want PAYABLE_FORMED", result.Outcome())
	}
	record, present := result.Payable()
	if !present {
		t.Fatal("no payable returned")
	}
	payable := record.Payable
	// 金额锚定本夹具：line-1 已匹配 12000 USD，引预期成本 cost/v1。
	if currency, minor := payable.Amount(); currency.String() != "USD" || minor != 12000 {
		t.Fatalf("amount = %s %d, want USD 12000（审核改不了数字）", currency, minor)
	}
	if payable.Claim().String() != "bill-1" || payable.Line().String() != "line-1" ||
		payable.Expected().String() != "cost/v1" {
		t.Fatalf("应付没有指回主张、行与预期成本：%+v", payable)
	}
	if payable.LegalEntity().String() != "legal-1" || payable.Account().String() != "supplier-account-1" ||
		payable.Auditor().String() != "auditor-1" {
		t.Fatalf("责任法人/结算账户/审核人没有从已登记实例取：%+v", payable)
	}
	if !payable.AuditedAt().Equal(billRecordedAt) {
		t.Fatalf("auditedAt = %s（审核时点由时钟给出，不由调用方声称）", payable.AuditedAt())
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（接收一份、应付一份）", len(fixture.handoff.intents))
	}
	if handed := fixture.handoff.intents[1]; handed.Payable.Key.Payable.String() != "payable-1" || handed.Record.Key.Claim.String() != "" {
		t.Fatalf("应付的发布意图形状不对：%+v", handed)
	}

	t.Run("a variance line cannot be audited into a payable", func(t *testing.T) {
		result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-2", "payable-2"))
		if err != nil {
			t.Fatalf("audit variance: %v", err)
		}
		if result.Outcome() != application.LineNotAuditable {
			t.Fatalf("outcome = %q, want LINE_NOT_AUDITABLE（价差先走争议，不得顺手接受）", result.Outcome())
		}
		if len(fixture.payables.records) != 1 {
			t.Fatal("价差行落成了应付")
		}
	})

	t.Run("a replay returns the original payable without a second save", func(t *testing.T) {
		saves := fixture.payables.saves
		replay, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.PayableExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_PAYABLE", replay.Outcome())
		}
		if fixture.payables.saves != saves {
			t.Fatal("重放重复提交了应付")
		}
	})

	t.Run("a second payable identity for the same line answers with the existing payable", func(t *testing.T) {
		again, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-9"))
		if err != nil {
			t.Fatalf("second identity: %v", err)
		}
		if again.Outcome() != application.PayableExistingResult {
			t.Fatalf("outcome = %q（同一行不成立两份应付）", again.Outcome())
		}
		record, _ := again.Payable()
		if record.Key.Payable.String() != "payable-1" {
			t.Fatalf("交回的不是先到的应付：%s", record.Key.Payable)
		}
	})

	t.Run("the same payable identity over another line is a conflict", func(t *testing.T) {
		if _, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-2")); err != nil {
			t.Fatalf("receive bill-2: %v", err)
		}
		result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-2", "line-1", "payable-1"))
		if err != nil {
			t.Fatalf("conflict audit: %v", err)
		}
		if result.Outcome() != application.PayableConflict {
			t.Fatalf("outcome = %q, want PAYABLE_CONFLICT（同一身份携带不同内容）", result.Outcome())
		}
	})

	t.Run("an unknown claim or line is not accepted", func(t *testing.T) {
		for name, command := range map[string]application.AuditBillLineCommand{
			"unknown claim": auditCommand(t, "bill-9", "line-1", "payable-3"),
			"unknown line":  auditCommand(t, "bill-1", "line-9", "payable-3"),
		} {
			result, err := fixture.handler.Audit(context.Background(), command)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if result.Outcome() != application.AuditNotAccepted {
				t.Fatalf("%s: outcome = %q, want SOURCE_NOT_ACCEPTED", name, result.Outcome())
			}
		}
	})
}

// Covers: 实例半边纪律与 ADR-0029 分格在审核这一步——审核授权未配置（AT-SA-088 的等配置那
// 一格）与供应商应付结算账户未配置（PAR-SET-01）都停在未决并指名等谁，不虚构授权人、不
// 默认账户；依赖故障归未决；未决原因集封闭；投递失败不翻结果（AT-SA-098）。
func TestAuditStopsOnUnconfiguredInstanceHalves(t *testing.T) {
	stops := map[string]struct {
		arrange func(*billFixture)
		reason  application.AuditUndecidedReason
	}{
		"authority unconfigured":   {func(f *billFixture) { f.authority.configured = false }, application.AuditAuthorityUnconfigured},
		"authority unavailable":    {func(f *billFixture) { f.authority.err = errors.New("authority down") }, application.AuditAuthorityUnavailable},
		"account unconfigured":     {func(f *billFixture) { f.accounts.configured = false }, application.PayableAccountUnconfigured},
		"account unavailable":      {func(f *billFixture) { f.accounts.err = errors.New("accounts down") }, application.PayableAccountUnavailable},
		"reception store down":     {func(f *billFixture) { f.store.findErr = errors.New("store down") }, application.AuditBillStoreUnavailable},
		"payable store save fails": {func(f *billFixture) { f.payables.saveErr = errors.New("payables down") }, application.PayableStoreUnavailable},
	}
	for name, stop := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := receivedFixture(t)
			stop.arrange(fixture)
			result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
			if err != nil {
				t.Fatalf("audit: %v", err)
			}
			if result.Outcome() != application.AuditUndecided || result.UndecidedReason() != stop.reason {
				t.Fatalf("outcome = %q reason = %q, want AUDIT_UNDECIDED/%q", result.Outcome(), result.UndecidedReason(), stop.reason)
			}
			if len(fixture.payables.records) != 0 {
				t.Fatal("未决的审核落成了应付")
			}
		})
	}

	t.Run("a handoff failure keeps the payable and leaves a continuation", func(t *testing.T) {
		fixture := receivedFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
		if err != nil {
			t.Fatalf("audit: %v", err)
		}
		if result.Outcome() != application.PayableFormed || result.PayableHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", result.Outcome(), result.PayableHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.AuditUndecidedReason{
			application.AuditBillStoreUnavailable, application.AuditAuthorityUnavailable, application.AuditAuthorityUnconfigured,
			application.PayableAccountUnavailable, application.PayableAccountUnconfigured, application.PayableStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 6 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.AuditUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第七个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-SA-090`「供应商贷项更正已审核金额 → 原主张/应付保留，追加具有稳定身份、结算
// 账户、借贷方向和原应付关系的贷项，并分别发布下游引用；不另建净额审核应付」的编排面
// （UC-SA-004 步 6–7）：账户、主张与币种从原应付取，原应付一字不动；贷项按（身份+版本）
// 幂等；指名不存在的应付是提交矛盾。
func TestASupplierCreditNoteAppendsToItsPayable(t *testing.T) {
	fixture := receivedFixture(t)
	if _, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1")); err != nil {
		t.Fatalf("audit: %v", err)
	}
	intentsBefore := len(fixture.handoff.intents)

	result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-1", "payable-1", 2000))
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if result.Outcome() != application.CreditNoteFormed {
		t.Fatalf("outcome = %q, want CREDIT_NOTE_FORMED", result.Outcome())
	}
	record, present := result.CreditNote()
	if !present {
		t.Fatal("no credit note returned")
	}
	note := record.Note
	if note.Payable().String() != "payable-1" || note.Claim().String() != "bill-1" ||
		note.Account().String() != "supplier-account-1" {
		t.Fatalf("贷项没有回指原应付/原主张/同一结算账户：%+v", note)
	}
	if currency, minor := note.Amount(); currency.String() != "USD" || minor != 2000 {
		t.Fatalf("amount = %s %d, want USD 2000", currency, minor)
	}
	if note.Reason().String() != "supplier-correction-1" || !note.IssuedAt().Equal(billAuditedAt.Add(24*time.Hour)) {
		t.Fatalf("原因或出具时点变形：%+v", note)
	}
	kept, _, _ := fixture.payables.FindByKey(context.Background(), ports.AuditedPayableKey{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"),
		Payable:  billValue(t, domain.NewPayableID, "payable-1"),
	})
	if _, minor := kept.Payable.Amount(); minor != 12000 {
		t.Fatalf("原应付被贷项改写成 %d——贷项不静默净入原应付", minor)
	}
	if len(fixture.handoff.intents) != intentsBefore+1 {
		t.Fatalf("intents = %d, want %d（贷项引用单独发布）", len(fixture.handoff.intents), intentsBefore+1)
	}
	if handed := fixture.handoff.intents[len(fixture.handoff.intents)-1]; handed.CreditNote.Key.Note.String() != "note-1" || handed.Payable.Key.Payable.String() != "" {
		t.Fatalf("贷项的发布意图形状不对：%+v", handed)
	}

	t.Run("a replay returns the original note", func(t *testing.T) {
		replay, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-1", "payable-1", 2000))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.CreditNoteExistingResult {
			t.Fatalf("outcome = %q, want EXISTING_CREDIT_NOTE", replay.Outcome())
		}
	})

	t.Run("a different amount under the same note identity is a conflict", func(t *testing.T) {
		result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-1", "payable-1", 2500))
		if err != nil {
			t.Fatalf("conflict credit: %v", err)
		}
		if result.Outcome() != application.CreditNoteConflict {
			t.Fatalf("outcome = %q, want CREDIT_NOTE_CONFLICT", result.Outcome())
		}
	})

	t.Run("a note against an unknown payable is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-2", "payable-9", 100))
		if err != nil {
			t.Fatalf("unknown payable: %v", err)
		}
		if result.Outcome() != application.CreditNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a non-positive amount is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-3", "payable-1", 0))
		if err != nil {
			t.Fatalf("zero credit: %v", err)
		}
		if result.Outcome() != application.CreditNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture.payables.findErr = errors.New("payables down")
		result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-4", "payable-1", 100))
		if err != nil {
			t.Fatalf("credit: %v", err)
		}
		if result.Outcome() != application.CreditUndecided || result.UndecidedReason() != application.CreditPayableStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		fixture.payables.findErr = nil

		fixture.creditNotes.saveErr = errors.New("notes down")
		result, err = fixture.handler.Credit(context.Background(), creditCommand(t, "note-4", "payable-1", 100))
		if err != nil {
			t.Fatalf("credit: %v", err)
		}
		if result.Outcome() != application.CreditUndecided || result.UndecidedReason() != application.CreditNoteStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		fixture.creditNotes.saveErr = nil
	})

	t.Run("a handoff failure keeps the note and leaves a continuation", func(t *testing.T) {
		fixture.handoff.err = errors.New("downstream unavailable")
		result, err := fixture.handler.Credit(context.Background(), creditCommand(t, "note-5", "payable-1", 100))
		if err != nil {
			t.Fatalf("credit: %v", err)
		}
		if result.Outcome() != application.CreditNoteFormed || result.CreditNoteHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果，只恢复贷项自己的发布意图）", result.Outcome(), result.CreditNoteHandoffReference())
		}
		fixture.handoff.err = nil
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.CreditUndecidedReason{
			application.CreditPayableStoreUnavailable, application.CreditNoteStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.CreditUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
