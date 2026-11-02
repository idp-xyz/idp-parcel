package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sadocnumber "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/documentnumber"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件在真库上证对账单、费用争议与审核应付的单据号（ADR-0355 决定三、七、八、九；票 document-numbering/05）：编排、本上下文
// 的册与编号组件的取号编排接在同一个库上，号在开立事务里取——同一开立意图重放答原号、并发开立不撞号、按租户隔离、按结算账户
// 业务时区出日期并按月重置、开立回滚号就释放、撞号不落单。编号组件自己的证据（加扰、跳号上限、换模板接着数）在 docnumberpg。

type numberingClock struct {
	mu sync.Mutex
	at time.Time
}

func (clock *numberingClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.at
}

func (clock *numberingClock) set(at time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.at = at
}

// zoneDouble 是对账安排读口的替身：只答账户的业务时区。对账安排册本身的读写由 statement_arrangement_test 钉。
type zoneDouble map[string]string

func (zones zoneDouble) LoadStatementArrangement(
	_ context.Context,
	_ domain.TenantID,
	account domain.SettlementAccountID,
) (domain.StatementArrangement, bool, error) {
	zone, found := zones[account.String()]
	if !found {
		return domain.StatementArrangement{}, false, nil
	}
	arrangement, err := domain.NewStatementArrangement(account, zone, 18*60)
	return arrangement, err == nil, err
}

type quietStatementHandoff struct{}

func (quietStatementHandoff) HandOffStatement(context.Context, ports.StatementIntent) error {
	return nil
}

type quietBillHandoff struct{}

func (quietBillHandoff) HandOffSupplierBill(context.Context, ports.SupplierBillHandoffIntent) error {
	return nil
}

// registeredPayableInstances 是审核授权、应付账户与越权上限三册的替身：三册各自的读写有自己的真库用例。
type registeredPayableInstances struct{}

func (registeredPayableInstances) LoadSupplierAuditAuthority(
	context.Context, domain.TenantID, domain.SupplierPartyReference, domain.LegalEntityReference,
) (domain.AuditorReference, bool, error) {
	auditor, err := domain.NewAuditorReference("auditor-1")
	return auditor, true, err
}

func (registeredPayableInstances) LoadSupplierPayableAccount(
	context.Context, domain.TenantID, domain.SupplierPartyReference, domain.LegalEntityReference, domain.CurrencyCode,
) (domain.SettlementAccountID, bool, error) {
	account, err := domain.NewSettlementAccountID("supplier-account-1")
	return account, true, err
}

func (registeredPayableInstances) LoadAuditEscalationCeiling(
	context.Context, domain.TenantID, domain.SupplierPartyReference, domain.LegalEntityReference, domain.CurrencyCode,
) (domain.AuditEscalationCeiling, bool, error) {
	ceiling, err := domain.NewAuditEscalationCeiling(1_000_000)
	return ceiling, true, err
}

type numberingStack struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	clock      *numberingClock
	registrar  *docnumber.Registrar
	statements *adapter.CustomerStatements
	disputes   *adapter.StatementDisputes
	payables   *adapter.AuditedPayables
	receptions *adapter.BillReceptions
	charges    *adapter.CustomerCharges
	cutoff     *application.CutOffPublishStatementHandler
	supplier   *application.ReceiveSupplierBillHandler
}

// 2026-11-02T02:00Z：上海是 11 月 2 日上午，月份 11。
var numberingNow = time.Date(2026, time.November, 2, 2, 0, 0, 0, time.UTC)

func newNumberingStack(t *testing.T) numberingStack {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	stack := numberingStack{pool: pool, db: db, clock: &numberingClock{at: numberingNow}}
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
	if err != nil {
		t.Fatal(err)
	}
	book, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: docnumber.SecretTable{}, Clock: stack.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stack.registrar, err = docnumber.NewRegistrar(docnumber.RegistrarDeps{
		Catalogue: catalogue, Book: book, Secrets: docnumber.SecretTable{}, Clock: stack.clock,
	}); err != nil {
		t.Fatal(err)
	}
	numbers, err := sadocnumber.NewAssigner(issuer)
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	stack.statements, err = adapter.NewCustomerStatements(db)
	must(err)
	stack.disputes, err = adapter.NewStatementDisputes(db)
	must(err)
	stack.payables, err = adapter.NewAuditedPayables(db)
	must(err)
	stack.receptions, err = adapter.NewBillReceptions(db)
	must(err)
	stack.charges, err = adapter.NewCustomerCharges(db)
	must(err)
	forms, err := adapter.NewChargeAttributions(db)
	must(err)
	adjustments, err := adapter.NewChargeAdjustments(db)
	must(err)
	inclusions, err := adapter.NewSubsequentInclusions(db)
	must(err)
	zones := zoneDouble{"account-1": "Asia/Shanghai", "ACCOUNT/syn-1": "Asia/Shanghai"}
	attributions, err := application.NewJudgeChargeAttributionHandler(forms, zones)
	must(err)
	stack.cutoff, err = application.NewCutOffPublishStatementHandler(application.CutOffPublishStatementDeps{
		Statements: stack.statements, Charges: stack.charges, Billing: stack.statements, Attributions: attributions,
		AttributionDates: stack.charges, Adjustments: adjustments, Inclusions: inclusions, Disputes: stack.disputes,
		Numbers: numbers, Arrangements: zones, Downstream: quietStatementHandoff{}, Clock: stack.clock,
	})
	must(err)
	instances := registeredPayableInstances{}
	stack.supplier = application.NewReceiveSupplierBillHandler(application.ReceiveSupplierBillDeps{
		Receptions: stack.receptions, Authority: instances, Accounts: instances, Escalation: instances, Payables: stack.payables,
		Numbers: numbers, Arrangements: zones, Downstream: quietBillHandoff{}, Clock: stack.clock,
	})
	return stack
}

// errRolledBack 让一笔开立事务在编排答完之后回滚：撞号与「开立回滚释放号码」两格都靠它，与 parcel-api 的事务壳同一个做法。
var errRolledBack = errors.New("the issuing transaction rolls back")

// within 在一笔事务里跑 fn；rollBack 为真或编排答撞号时整笔回滚，答复照样交回。
func (stack numberingStack) within(t *testing.T, rollBack bool, fn func(context.Context) (string, error)) string {
	t.Helper()
	var outcome string
	err := stack.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		answered, err := fn(txCtx)
		outcome = answered
		if err == nil && (rollBack || answered == "NUMBERING_RULE_COLLISION") {
			return errRolledBack
		}
		return err
	})
	if err != nil && !errors.Is(err, errRolledBack) {
		t.Fatal(err)
	}
	return outcome
}

func (stack numberingStack) openDispute(t *testing.T, tenant, id string, rollBack bool) (string, ports.DisputeRecord) {
	t.Helper()
	var record ports.DisputeRecord
	outcome := stack.within(t, rollBack, func(ctx context.Context) (string, error) {
		result, err := stack.cutoff.OpenDispute(ctx, application.OpenDisputeCommand{
			TenantID: saTenant(t, tenant), Dispute: id, Number: "STMT-A", ChargeID: "charge-1", DisputedMinor: 300,
			Reason: "weight", OpenedAt: time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC),
		})
		record, _ = result.Dispute()
		return result.Outcome().String(), err
	})
	return outcome, record
}

func documentNumberOf(numbering domain.DocumentNumbering) string {
	number, _ := numbering.Number()
	return number.String()
}

// numberedDocument 造一格有号的单据号，给直写册的夹具用。
func numberedDocument(t *testing.T, raw string) domain.DocumentNumbering {
	t.Helper()
	numbering, err := domain.NumberedDocument(saValue(t, domain.NewDocumentNumber, raw))
	if err != nil {
		t.Fatal(err)
	}
	return numbering
}

// Covers: `AT-SA-230`（真库半边）——争议号在登记事务里取：重放答原号不再取号、开立回滚释放号码、并发登记不撞号、同一标识的
// 并发登记只取一个号、租户隔离、按被争对账单那个账户的业务时区出日期并按月重置、撞号不登记、库上（租户，争议号）唯一。
func TestADisputeNumberIsIssuedInTheRegistrationTransaction(t *testing.T) {
	stack := newNumberingStack(t)
	for _, tenant := range []string{"tenant-1", "tenant-2", "tenant-3"} {
		record := statementRecord(t, tenant, "STMT-A", "digest-"+tenant)
		saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
			_, err := stack.statements.Save(txCtx, record)
			return err
		})
	}

	outcome, first := stack.openDispute(t, "tenant-1", "dispute-1", false)
	if outcome != "DISPUTE_OPENED" || documentNumberOf(first.Numbering) != "DSP-202611-00001" {
		t.Fatalf("first = %s %+v", outcome, first.Numbering)
	}
	stored, found, err := stack.disputes.FindByKey(t.Context(), first.Key)
	if err != nil || !found || stored.Numbering != first.Numbering {
		t.Fatalf("争议号没随争议落库：%+v %v %v", stored.Numbering, found, err)
	}
	if outcome, replay := stack.openDispute(t, "tenant-1", "dispute-1", false); outcome != "EXISTING_DISPUTE" || replay.Numbering != first.Numbering {
		t.Fatalf("replay = %s %+v", outcome, replay.Numbering)
	}

	if outcome, rolled := stack.openDispute(t, "tenant-1", "dispute-rolled-back", true); outcome != "DISPUTE_OPENED" ||
		documentNumberOf(rolled.Numbering) != "DSP-202611-00002" {
		t.Fatalf("rolled back = %s %+v", outcome, rolled.Numbering)
	}
	if _, next := stack.openDispute(t, "tenant-1", "dispute-2", false); documentNumberOf(next.Numbering) != "DSP-202611-00002" {
		t.Fatalf("回滚的那一笔没释放号码：重放之后的下一项 = %+v（重放也不该取号）", next.Numbering)
	}

	t.Run("concurrent registrations never collide", func(t *testing.T) {
		const parallel = 8
		numbers := make([]string, parallel)
		var wait sync.WaitGroup
		for index := range parallel {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, record := stack.openDispute(t, "tenant-1", fmt.Sprintf("dispute-parallel-%d", index), false)
				numbers[index] = documentNumberOf(record.Numbering)
			}()
		}
		wait.Wait()
		sort.Strings(numbers)
		for index, number := range numbers {
			if want := fmt.Sprintf("DSP-202611-%05d", index+3); number != want {
				t.Fatalf("numbers = %v", numbers)
			}
		}
	})

	t.Run("two registrations of one dispute take one number", func(t *testing.T) {
		outcomes := make([]string, 2)
		records := make([]ports.DisputeRecord, 2)
		var wait sync.WaitGroup
		for index := range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				outcomes[index], records[index] = stack.openDispute(t, "tenant-1", "dispute-twice", false)
			}()
		}
		wait.Wait()
		sort.Strings(outcomes)
		if outcomes[0] != "DISPUTE_OPENED" || outcomes[1] != "EXISTING_DISPUTE" || records[0].Numbering != records[1].Numbering {
			t.Fatalf("outcomes = %v numbers = %+v %+v", outcomes, records[0].Numbering, records[1].Numbering)
		}
		if _, next := stack.openDispute(t, "tenant-1", "dispute-after-twice", false); documentNumberOf(next.Numbering) != "DSP-202611-00012" {
			t.Fatalf("同一标识的两份登记取了两个号：下一项 = %+v", next.Numbering)
		}
	})

	t.Run("tenants number separately", func(t *testing.T) {
		if _, other := stack.openDispute(t, "tenant-2", "dispute-1", false); documentNumberOf(other.Numbering) != "DSP-202611-00001" {
			t.Fatalf("tenant-2 = %+v", other.Numbering)
		}
	})

	t.Run("the month resets in the account business time zone", func(t *testing.T) {
		// UTC 还在 11 月 30 日，上海已是 12 月 1 日。
		stack.clock.set(time.Date(2026, time.November, 30, 16, 30, 0, 0, time.UTC))
		defer stack.clock.set(numberingNow)
		if _, december := stack.openDispute(t, "tenant-1", "dispute-december", false); documentNumberOf(december.Numbering) != "DSP-202612-00001" {
			t.Fatalf("december = %+v", december.Numbering)
		}
	})

	t.Run("a collision registers nothing and releases the sequence", func(t *testing.T) {
		// tenant-3 跳号上限（docnumber.MaxSkippedNumbers）以内的号都已被用掉（换格式前的旧号）：跳到上限仍撞，答撞号、不登记，整笔回滚。
		for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
			record := formedDisputeRecord(t, "tenant-3", fmt.Sprintf("dispute-old-%d", index), false)
			record.Numbering = numberedDocument(t, fmt.Sprintf("DSP-202611-%05d", index))
			saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
				_, err := stack.disputes.Save(txCtx, record)
				return err
			})
		}
		outcome, _ := stack.openDispute(t, "tenant-3", "dispute-collided", false)
		if outcome != "NUMBERING_RULE_COLLISION" {
			t.Fatalf("outcome = %s", outcome)
		}
		if _, found, err := stack.disputes.FindByKey(t.Context(), ports.DisputeKey{
			TenantID: saTenant(t, "tenant-3"), Dispute: saValue(t, domain.NewDisputeID, "dispute-collided"),
		}); err != nil || found {
			t.Fatalf("撞号还登记了争议：%v %v", found, err)
		}
		if _, err := stack.pool.Exec(t.Context(), `DELETE FROM settlement_accounting.statement_dispute
			WHERE tenant_id = 'tenant-3' AND document_number = 'DSP-202611-00001'`); err != nil {
			t.Fatal(err)
		}
		if _, freed := stack.openDispute(t, "tenant-3", "dispute-after-collision", false); documentNumberOf(freed.Numbering) != "DSP-202611-00001" {
			t.Fatalf("撞号回滚之后序列应从头再取：%+v", freed.Numbering)
		}
	})

	t.Run("the statement register shows each dispute's number", func(t *testing.T) {
		catalogue, err := adapter.NewStatementCatalogue(stack.db)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := catalogue.ListCustomerStatements(t.Context(), saTenant(t, "tenant-1"), 10)
		if err != nil || len(rows) != 1 {
			t.Fatalf("rows = %d err = %v", len(rows), err)
		}
		for _, entry := range rows[0].Disputes {
			if entry.Dispute == "dispute-1" && (entry.DocumentNumber != "DSP-202611-00001" || entry.DocumentNumberAbsence != "") {
				t.Fatalf("对账单册上的争议号 = %+v", entry)
			}
			if entry.DocumentNumber == "" && entry.DocumentNumberAbsence == "" {
				t.Fatalf("争议 %s 既没号也没无号原因", entry.Dispute)
			}
		}
	})

	t.Run("the table keeps one dispute per number", func(t *testing.T) {
		twin := formedDisputeRecord(t, "tenant-1", "dispute-twin", false)
		twin.Numbering = first.Numbering
		var outcome ports.DisputeSaveOutcome
		saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
			var err error
			outcome, err = stack.disputes.Save(txCtx, twin)
			return err
		})
		if outcome != ports.DisputeAlreadyOpened {
			t.Fatalf("同号的第二项 = %d，库上的（租户，争议号）唯一没挡住", outcome)
		}
	})
}

// Covers: ADR-0355 决定三、七、十一（存储半边）——无号原因随争议落库、读回；本决定之前开立的旧行两列都空，读作上线前开立；无号原因
// 只收一个词，有号就不能再带原因。
func TestADisputeWithoutANumberKeepsTheReason(t *testing.T) {
	stack := newNumberingStack(t)
	record := formedDisputeRecord(t, "tenant-1", "dispute-unnumbered", false)
	numbering, err := domain.UnnumberedDocument(domain.NumberingTimeZoneUnconfigured)
	if err != nil {
		t.Fatal(err)
	}
	record.Numbering = numbering
	saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := stack.disputes.Save(txCtx, record)
		return err
	})
	read, _, err := stack.disputes.FindByKey(t.Context(), record.Key)
	if absence, absent := read.Numbering.Absence(); err != nil || !absent || absence != domain.NumberingTimeZoneUnconfigured {
		t.Fatalf("无号原因往返 = %+v %v", read.Numbering, err)
	}

	if _, err := stack.pool.Exec(t.Context(), `UPDATE settlement_accounting.statement_dispute SET document_number_absence = NULL
		WHERE tenant_id = 'tenant-1' AND dispute_id = 'dispute-unnumbered'`); err != nil {
		t.Fatal(err)
	}
	read, _, err = stack.disputes.FindByKey(t.Context(), record.Key)
	if absence, absent := read.Numbering.Absence(); err != nil || !absent || absence != domain.IssuedBeforeNumbering {
		t.Fatalf("两列都空的旧行 = %+v %v，要读作上线前开立", read.Numbering, err)
	}

	for name, statement := range map[string]string{
		"有号又带原因":  `UPDATE settlement_accounting.statement_dispute SET document_number = 'DSP-X', document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' WHERE dispute_id = 'dispute-unnumbered'`,
		"原因写成别的词": `UPDATE settlement_accounting.statement_dispute SET document_number_absence = 'ISSUED_BEFORE_NUMBERING' WHERE dispute_id = 'dispute-unnumbered'`,
	} {
		if _, err := stack.pool.Exec(t.Context(), statement); err == nil {
			t.Fatalf("%s：库上的约束没挡住", name)
		}
	}

	t.Run("a new dispute without a numbering is refused", func(t *testing.T) {
		blank := formedDisputeRecord(t, "tenant-1", "dispute-blank", false)
		blank.Numbering = domain.DocumentNumbering{}
		err := stack.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, err := stack.disputes.Save(txCtx, blank)
			return err
		})
		if err == nil {
			t.Fatal("漏接取号的争议落成了一张「上线前开立」的单")
		}
	})
}

// Covers: `AT-SA-222`（真库半边）——对账单号在发布事务里按编号规则取，日期按结算账户业务时区；勾稽不平不取号；同一发布意图重放答
// 原单原号、不再取号，内容不同答发布意图冲突、不取号；同一意图的两笔并发发布只出一张单、只取一个号；换截止版本是新意图、取新号。
func TestAStatementNumberIsIssuedInThePublicationTransaction(t *testing.T) {
	stack := newNumberingStack(t)
	tenant := saTenant(t, "tenant-1")
	for _, id := range []string{"charge-1", "charge-2", "charge-3"} {
		charge, err := confirmedCharge(t, id, "DELIVERY_FINALIZED/"+id).WithAttributionDate(domain.AttributionDate{Year: 2026, Month: time.October, Day: 30})
		if err != nil {
			t.Fatal(err)
		}
		saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
			_, err := stack.charges.SaveConfirmed(txCtx, tenant, charge)
			return err
		})
	}
	command := func(version string, total int64, charges ...string) application.PublishStatementCommand {
		return application.PublishStatementCommand{
			TenantID: tenant, Account: "ACCOUNT/syn-1", Period: "2026-10", Version: version, Currency: "CNY",
			ChargeIDs: charges, LastAttributionDate: "2026-10-31", DeclaredTotalMinor: total,
			CutOffAt: time.Date(2026, time.October, 31, 10, 0, 0, 0, time.UTC), PublishedAt: numberingNow,
		}
	}
	publish := func(command application.PublishStatementCommand) (string, string) {
		var number string
		outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
			result, err := stack.cutoff.Publish(ctx, command)
			if record, has := result.Statement(); has {
				number = record.Key.Number.String()
			}
			return result.Outcome().String(), err
		})
		return outcome, number
	}

	if outcome, _ := publish(command("v1", 1, "charge-1")); outcome != "STATEMENT_IMBALANCE" {
		t.Fatalf("imbalance = %s", outcome)
	}
	outcome, number := publish(command("v1", 45600, "charge-1"))
	if outcome != "STATEMENT_PUBLISHED" || number != "STMT-202611-00001" {
		t.Fatalf("publish = %s %s（勾稽不平那一次不该先取号）", outcome, number)
	}
	if outcome, again := publish(command("v1", 45600, "charge-1")); outcome != "EXISTING_STATEMENT" || again != number {
		t.Fatalf("replay = %s %s", outcome, again)
	}
	if outcome, _ := publish(command("v1", 45600, "charge-2")); outcome != "PUBLICATION_INTENT_CONFLICT" {
		t.Fatalf("同一意图换内容 = %s", outcome)
	}
	stored, found, err := stack.statements.FindByIntent(t.Context(), ports.StatementPublicationIntent{
		TenantID: tenant, Account: saValue(t, domain.NewSettlementAccountID, "ACCOUNT/syn-1"),
		Period: saValue(t, domain.NewBillingPeriodReference, "2026-10"), Version: saValue(t, domain.NewStatementDraftVersion, "v1"),
	})
	if err != nil || !found || stored.Key.Number.String() != number {
		t.Fatalf("按意图读回 = %+v %v %v", stored.Key, found, err)
	}

	outcomes := make([]string, 2)
	numbers := make([]string, 2)
	var wait sync.WaitGroup
	for index := range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			outcomes[index], numbers[index] = publish(command("v2", 45600, "charge-2"))
		}()
	}
	wait.Wait()
	sort.Strings(outcomes)
	if outcomes[0] != "EXISTING_STATEMENT" || outcomes[1] != "STATEMENT_PUBLISHED" || numbers[0] != numbers[1] || numbers[0] != "STMT-202611-00002" {
		t.Fatalf("同一意图并发 = %v %v", outcomes, numbers)
	}
	if outcome, third := publish(command("v3", 45600, "charge-3")); outcome != "STATEMENT_PUBLISHED" || third != "STMT-202611-00003" {
		t.Fatalf("新意图 = %s %s（并发那两笔只该取一个号）", outcome, third)
	}
}

// Covers: `AT-SA-231`（真库半边）——应付号在审核事务里取：应付账户没登对账安排、租户也没登单据编号时区时照常形成、记无号原因；
// 登了单据编号时区之后按它出号；重放答原号；并发审核不撞号；号随应付落库、读回。
func TestAnAuditedPayableNumberIsIssuedInTheAuditTransaction(t *testing.T) {
	stack := newNumberingStack(t)
	for index := 1; index <= 5; index++ {
		mustSaveBill(t, stack.db.Transactor(), t.Context(), stack.receptions,
			billRecord(t, "tenant-1", fmt.Sprintf("claim-%d", index), "v1", fmt.Sprintf("digest-%d", index)))
	}
	audit := func(claim, payable string) (string, ports.AuditedPayableRecord) {
		var record ports.AuditedPayableRecord
		outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
			result, err := stack.supplier.Audit(ctx, application.AuditBillLineCommand{
				TenantID: saTenant(t, "tenant-1"), Claim: saValue(t, domain.NewBillClaimID, claim),
				Version: saValue(t, domain.NewBillClaimVersion, "v1"), Line: saValue(t, domain.NewBillLineReference, "L1"),
				Payable: saValue(t, domain.NewPayableID, payable),
			})
			record, _ = result.Payable()
			return result.Outcome().String(), err
		})
		return outcome, record
	}

	outcome, unnumbered := audit("claim-1", "payable-1")
	if absence, absent := unnumbered.Numbering.Absence(); outcome != "PAYABLE_FORMED" || !absent || absence != domain.NumberingTimeZoneUnconfigured {
		t.Fatalf("没有时区 = %s %+v，应付照常形成、不带号", outcome, unnumbered.Numbering)
	}

	saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := stack.registrar.RegisterTimeZone(txCtx, docnumber.RegisterTimeZoneCommand{
			Tenant: "tenant-1", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
	outcome, formed := audit("claim-2", "payable-2")
	if outcome != "PAYABLE_FORMED" || documentNumberOf(formed.Numbering) != "AP-202611-00001" {
		t.Fatalf("formed = %s %+v", outcome, formed.Numbering)
	}
	if outcome, replay := audit("claim-2", "payable-2"); outcome != "EXISTING_PAYABLE" || replay.Numbering != formed.Numbering {
		t.Fatalf("replay = %s %+v", outcome, replay.Numbering)
	}
	stored, found, err := stack.payables.FindByKey(t.Context(), formed.Key)
	if err != nil || !found || stored.Numbering != formed.Numbering {
		t.Fatalf("应付号没随应付落库：%+v %v %v", stored.Numbering, found, err)
	}

	numbers := make([]string, 3)
	var wait sync.WaitGroup
	for index := range 3 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, record := audit(fmt.Sprintf("claim-%d", index+3), fmt.Sprintf("payable-%d", index+3))
			numbers[index] = documentNumberOf(record.Numbering)
		}()
	}
	wait.Wait()
	sort.Strings(numbers)
	if numbers[0] != "AP-202611-00002" || numbers[1] != "AP-202611-00003" || numbers[2] != "AP-202611-00004" {
		t.Fatalf("并发审核 = %v", numbers)
	}
}

// Covers: ADR-0355 决定九（存储半边）——对账单的发布意图只认一张单：同意图的第二张答已发布；没记截止版本的新单拒写；按意图找、按
// 账期找没记版本的旧单、单号已用，三个读法各认各的。
func TestTheStatementStoreKeepsOneStatementPerIntent(t *testing.T) {
	stack := newNumberingStack(t)
	first := statementRecord(t, "tenant-1", "STMT-1", "digest-1")
	second := statementRecord(t, "tenant-1", "STMT-2", "digest-2")
	second.DraftVersion = first.DraftVersion
	var outcome ports.StatementSaveOutcome
	saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := stack.statements.Save(txCtx, first); err != nil {
			return err
		}
		var err error
		outcome, err = stack.statements.Save(txCtx, second)
		return err
	})
	if outcome != ports.StatementAlreadyPublished {
		t.Fatalf("同一意图第二张 = %d", outcome)
	}

	unversioned := statementRecord(t, "tenant-1", "STMT-3", "digest-3")
	unversioned.DraftVersion = domain.StatementDraftVersion{}
	if err := stack.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := stack.statements.Save(txCtx, unversioned)
		return err
	}); err == nil {
		t.Fatal("没记截止版本的新单写进了库，会躲过同一意图的查重")
	}

	// 本决定之前手工发布的单：截止版本列为空。
	if _, err := stack.pool.Exec(t.Context(), `UPDATE settlement_accounting.customer_statement SET draft_version = NULL
		WHERE tenant_id = 'tenant-1' AND statement_number = 'STMT-1'`); err != nil {
		t.Fatal(err)
	}
	legacy, err := stack.statements.FindUnversionedInPeriod(t.Context(), saTenant(t, "tenant-1"),
		first.Statement.Account(), first.Statement.Period())
	if err != nil || len(legacy) != 1 || legacy[0].Key.Number.String() != "STMT-1" || legacy[0].DraftVersion.String() != "" {
		t.Fatalf("没记版本的旧单 = %+v %v", legacy, err)
	}
	if _, found, err := stack.statements.FindByIntent(t.Context(), ports.StatementPublicationIntent{
		TenantID: saTenant(t, "tenant-1"), Account: first.Statement.Account(), Period: first.Statement.Period(), Version: first.DraftVersion,
	}); err != nil || found {
		t.Fatalf("没记版本的旧单不该按版本查到：%v %v", found, err)
	}
	var used, unused bool
	saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		if used, err = stack.statements.StatementNumberUsed(txCtx, saTenant(t, "tenant-1"), "STMT-1"); err != nil {
			return err
		}
		unused, err = stack.statements.StatementNumberUsed(txCtx, saTenant(t, "tenant-2"), "STMT-1")
		return err
	})
	if !used || unused {
		t.Fatalf("单号已用：本租户 %v、他租户 %v", used, unused)
	}
}

// 三把排队锁都是事务级咨询锁：离开事务就锁不到提交那一刻，排队等于没排，所以事务外一律拒。
func TestIssuanceLocksRefuseToRunOutsideATransaction(t *testing.T) {
	stack := newNumberingStack(t)
	tenant := saTenant(t, "tenant-1")
	if err := stack.statements.LockPublicationIntent(t.Context(), ports.StatementPublicationIntent{
		TenantID: tenant, Account: saValue(t, domain.NewSettlementAccountID, "account-1"),
		Period: saValue(t, domain.NewBillingPeriodReference, "2026-09"), Version: saValue(t, domain.NewStatementDraftVersion, "v1"),
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 LockPublicationIntent 应答 ErrTransactionRequired，实得：%v", err)
	}
	if err := stack.disputes.LockRegistration(t.Context(), ports.DisputeKey{
		TenantID: tenant, Dispute: saValue(t, domain.NewDisputeID, "dispute-1"),
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 LockRegistration 应答 ErrTransactionRequired，实得：%v", err)
	}
	if err := stack.payables.LockIssuance(t.Context(), ports.AuditedPayableKey{
		TenantID: tenant, Payable: saValue(t, domain.NewPayableID, "payable-1"),
	}, saValue(t, domain.NewBillClaimID, "claim-1"), saValue(t, domain.NewBillLineReference, "L1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 LockIssuance 应答 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 迁移 settlement_accounting/0053 施加在有旧单的库上——自动截单截出的旧单按截止记录认、补上 v1；手工发布的旧单留空，
// 单号写成 STMT/<账户>/<日> 的样子、账期也是 CYCLE/<日> 的，截止记录上没有它，同样留空；旧争议、旧应付两列都空（不回填）。
// 从空库按序施加本上下文的迁移，停在 0053 之前落旧形状的行，再施加 0053。
func TestTheDocumentNumberMigrationMarksOnlyCycleCutStatements(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA settlement_accounting`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.SettlementAccounting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0053_document_numbers_and_statement_publication_intent"
	var numbering *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			numbering = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if numbering == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency, settlement_policy_id, responsibility_basis, registered_at)
		VALUES ('SYN-TENANT-01', 'SYN-ACC-1', 'SYN-LE-01', 'SYN-PARTY-1', 'RECEIVABLE', 'CNY', 'SYN-POLICY-1', 'SYN-CONTRACT-1', now());
		INSERT INTO settlement_accounting.statement_cycle (tenant_id, account_id, cycle_kind, since_date, registered_at)
		VALUES ('SYN-TENANT-01', 'SYN-ACC-1', 'DAILY', '2026-10-01', now());
		INSERT INTO settlement_accounting.statement_cycle_cut (tenant_id, account_id, closing_date, outcome, statement_number, cut_at)
		VALUES ('SYN-TENANT-01', 'SYN-ACC-1', '2026-10-30', 'STATEMENT_PUBLISHED', 'STMT/SYN-ACC-1/20261030', now()),
		       ('SYN-TENANT-01', 'SYN-ACC-1', '2026-10-31', 'NO_CHARGE', NULL, now());
		INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor, content_digest, lines, adjustment_lines, published_at, recorded_at)
		VALUES ('SYN-TENANT-01', 'STMT/SYN-ACC-1/20261030', 'SYN-ACC-1', 'CYCLE/2026-10-30', 'CNY', 100, 'd1', '[{"charge":"C1","amount_minor":100}]', '[]', now(), now()),
		       ('SYN-TENANT-01', 'STMT/SYN-ACC-1/20261029', 'SYN-ACC-1', 'CYCLE/2026-10-29', 'CNY', 100, 'd2', '[{"charge":"C2","amount_minor":100}]', '[]', now(), now()),
		       ('SYN-TENANT-01', 'SYN-STMT-MANUAL-1', 'SYN-ACC-1', '2026-10', 'CNY', 100, 'd3', '[{"charge":"C3","amount_minor":100}]', '[]', now(), now());
		INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor, reason_ref, opened_at, content_digest, recorded_at)
		VALUES ('SYN-TENANT-01', 'SYN-DSP-OLD', 'SYN-STMT-MANUAL-1', 'C3', 50, 'weight', now(), 'dd', now());
		INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id, currency, amount_minor, auditor_ref, audited_at, content_digest, recorded_at)
		VALUES ('SYN-TENANT-01', 'SYN-PAYABLE-OLD', 'SYN-BILL-1', 'L1', 'SYN-COST-1', 'SYN-LE-01', 'SYN-ACC-2', 'CNY', 100, 'SYN-AUDITOR-1', now(), 'pd', now())`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, numbering.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}

	versions := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT statement_number, coalesce(draft_version, '') FROM settlement_accounting.customer_statement`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var number, version string
		if err := rows.Scan(&number, &version); err != nil {
			t.Fatal(err)
		}
		versions[number] = version
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if versions["STMT/SYN-ACC-1/20261030"] != "v1" || versions["STMT/SYN-ACC-1/20261029"] != "" || versions["SYN-STMT-MANUAL-1"] != "" {
		t.Fatalf("截止版本 = %v，want 只有截止记录上那一张补 v1", versions)
	}
	var unnumbered int
	if err := conn.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM settlement_accounting.statement_dispute WHERE document_number IS NULL AND document_number_absence IS NULL) +
		(SELECT count(*) FROM settlement_accounting.audited_payable WHERE document_number IS NULL AND document_number_absence IS NULL)`).Scan(&unnumbered); err != nil {
		t.Fatal(err)
	}
	if unnumbered != 2 {
		t.Fatalf("旧争议与旧应付两列皆空的 = %d，want 2（不回填）", unnumbered)
	}
}

// Covers: `AT-SA-222`（旧单那一格，真库半边）——本决定之前手工发布、没记下截止版本的单占着它的账户与账期：同一账户同一账期换了
// 内容再发，答发布意图冲突、不落单不取号；同内容重放答原单原号；那张旧单作废之后不再挡，替代单取新号。
func TestAManualStatementBeforeNumberingHoldsItsPeriod(t *testing.T) {
	stack := newNumberingStack(t)
	tenant := saTenant(t, "tenant-1")
	for _, id := range []string{"charge-1", "charge-2"} {
		charge, err := confirmedCharge(t, id, "DELIVERY_FINALIZED/"+id).WithAttributionDate(domain.AttributionDate{Year: 2026, Month: time.October, Day: 30})
		if err != nil {
			t.Fatal(err)
		}
		saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
			_, err := stack.charges.SaveConfirmed(txCtx, tenant, charge)
			return err
		})
	}
	command := func(version string, charges ...string) application.PublishStatementCommand {
		return application.PublishStatementCommand{
			TenantID: tenant, Account: "ACCOUNT/syn-1", Period: "2026-10", Version: version, Currency: "CNY",
			ChargeIDs: charges, LastAttributionDate: "2026-10-31", DeclaredTotalMinor: 45600,
			CutOffAt: time.Date(2026, time.October, 31, 10, 0, 0, 0, time.UTC), PublishedAt: numberingNow,
		}
	}
	publish := func(command application.PublishStatementCommand) (string, string) {
		var number string
		outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
			result, err := stack.cutoff.Publish(ctx, command)
			if record, has := result.Statement(); has {
				number = record.Key.Number.String()
			}
			return result.Outcome().String(), err
		})
		return outcome, number
	}
	if outcome, _ := publish(command("v1", "charge-1")); outcome != "STATEMENT_PUBLISHED" {
		t.Fatalf("publish = %s", outcome)
	}
	// 改成升级前手工发布的样子：人填的单号、没记截止版本；序列也回到没取过号。
	if _, err := stack.pool.Exec(t.Context(), `UPDATE settlement_accounting.customer_statement
		SET statement_number = 'SYN-STMT-MANUAL-1', draft_version = NULL WHERE tenant_id = 'tenant-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := stack.pool.Exec(t.Context(), `DELETE FROM platform.numbering_sequence WHERE scope = 'tenant-1'`); err != nil {
		t.Fatal(err)
	}

	for _, version := range []string{"v1", "v2"} {
		if outcome, _ := publish(command(version, "charge-2")); outcome != "PUBLICATION_INTENT_CONFLICT" {
			t.Fatalf("%s 换内容 = %s，同账期还有未作废的旧单就答意图冲突", version, outcome)
		}
	}
	if outcome, number := publish(command("v1", "charge-1")); outcome != "EXISTING_STATEMENT" || number != "SYN-STMT-MANUAL-1" {
		t.Fatalf("同内容重放 = %s %s", outcome, number)
	}
	var taken int
	if err := stack.pool.QueryRow(t.Context(), `SELECT count(*) FROM platform.numbering_sequence WHERE scope = 'tenant-1'`).Scan(&taken); err != nil || taken != 0 {
		t.Fatalf("意图冲突与重放取了号：序列行 %d err %v", taken, err)
	}

	if outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
		result, err := stack.cutoff.Void(ctx, application.VoidStatementCommand{
			TenantID: tenant, Number: "SYN-STMT-MANUAL-1", Basis: "void-basis-1", VoidedAt: numberingNow,
		})
		return result.Outcome().String(), err
	}); outcome != "STATEMENT_VOIDED" {
		t.Fatalf("void = %s", outcome)
	}
	if outcome, number := publish(command("v2", "charge-2")); outcome != "STATEMENT_PUBLISHED" || number != "STMT-202611-00001" {
		t.Fatalf("旧单作废之后 = %s %s", outcome, number)
	}
}
