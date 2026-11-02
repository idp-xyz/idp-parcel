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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证异常案件与客户索赔项的单据号（ADR-0355 决定三、七、八；document-numbering/06）：号由编号
// 组件的真取号编排在开立事务里出、随单落库；重放答原号、并发不撞号、按租户各数各的、按周期重置、开立回滚释放号码、
// 撞号答案连同它取过的流水一并退回；表上（租户，单据号）唯一与无号原因的取值域由迁移把关。测试输入是隔离合成，只记 S。

var numberingNow = time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC) // 上海 2026-11-02 11:00

// movableClock 是取号与登记共用的服务端时钟，周期重置那一条要拨它。
type movableClock struct {
	mu sync.Mutex
	at time.Time
}

func (clock *movableClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.at
}

func (clock *movableClock) set(at time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.at = at
}

type numberingFixture struct {
	pool       *pgxpool.Pool
	transactor bentoapp.Transactor
	clock      *movableClock
	registrar  *docnumber.Registrar
	numbers    *adapter.DocumentNumbers
	claims     *adapter.Claims
	episodes   *adapter.SignalEpisodes
	review     *adapter.CaseReview
	receipts   *application.HandleClaimHandler
}

// newNumberingFixture 照 cmd/internal/docnumbering.Build 的装法在同一个 DB 上装规则册、序列与取号编排（那个包只许 cmd 导入，
// 这里按同一组公开构造现装），类型声明只取本上下文自己的（domain.DocumentNumberTypes）。
func newNumberingFixture(t *testing.T) *numberingFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
	if err != nil {
		t.Fatalf("汇总类型声明：%v", err)
	}
	book, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := &movableClock{at: numberingNow}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{Catalogue: catalogue, Book: book, Secrets: docnumber.SecretTable{}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := adapter.NewDocumentNumbers(db, assigner)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := adapter.NewClaims(db)
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := adapter.NewSignalEpisodes(db)
	if err != nil {
		t.Fatal(err)
	}
	review, err := adapter.NewCaseReview(db)
	if err != nil {
		t.Fatal(err)
	}
	// 受理只用到索赔库与取号两样依赖。
	receipts := application.NewHandleClaimHandler(application.HandleClaimDeps{Claims: claims, ClaimNumbers: numbers})
	return &numberingFixture{
		pool: pool, transactor: db.Transactor(), clock: clock, registrar: registrar,
		numbers: numbers, claims: claims, episodes: episodes, review: review, receipts: receipts,
	}
}

func (fixture *numberingFixture) registerTimeZone(t *testing.T, tenant string) {
	t.Helper()
	var registration docnumber.TimeZoneRegistration
	err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		registration, err = fixture.registrar.RegisterTimeZone(ctx, docnumber.RegisterTimeZoneCommand{
			Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
	if err != nil || registration.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registration, err)
	}
}

func (fixture *numberingFixture) registerClaimRule(t *testing.T, tenant, template string) {
	t.Helper()
	var registration docnumber.RuleRegistration
	err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		registration, err = fixture.registrar.RegisterRule(ctx, docnumber.RegisterRuleCommand{
			Tenant: tenant, Type: domain.ClaimDocumentNumberType.Code, Template: template,
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
	if err != nil || registration.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登索赔规则：%+v %v", registration, err)
	}
}

var errReceiptDiscarded = errors.New("the receipt rolls back")

// receive 照 parcel-api 的受理事务办：编排交回错误或说取了号没落单（DiscardIssuing）就回滚；rollBack 为真时在编排答完
// 之后再强行回滚一次，模拟开立事务在落单之后才失败。
func (fixture *numberingFixture) receive(t *testing.T, tenant, item string, rollBack bool) (application.HandleClaimResult, error) {
	t.Helper()
	var result application.HandleClaimResult
	err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		handled, err := fixture.receipts.ReceiveClaim(ctx, numberingClaimCommand(t, tenant, item))
		if err != nil {
			return err
		}
		result = handled
		if handled.DiscardIssuing() || rollBack {
			return errReceiptDiscarded
		}
		return nil
	})
	if errors.Is(err, errReceiptDiscarded) {
		err = nil
	}
	return result, err
}

func numberingClaimCommand(t *testing.T, tenant, item string) application.ReceiveClaimCommand {
	t.Helper()
	return application.ReceiveClaimCommand{
		TenantID:    claimValue(t, domain.NewTenantID, tenant),
		Batch:       claimValue(t, domain.NewClaimBatchReference, "SYN-BATCH-1"),
		Item:        claimValue(t, domain.NewClaimItemID, item),
		Customer:    claimValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Applicant:   claimValue(t, domain.NewApplicantReference, "SYN-APPLICANT-01"),
		Contract:    claimValue(t, domain.NewContractScopeReference, "SYN-CONTRACT-01"),
		Target:      claimValue(t, domain.NewRequestScopeReference, "SYN-PARCEL-"+item),
		Kind:        claimValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: numberingNow,
	}
}

func numberOfReceipt(t *testing.T, result application.HandleClaimResult) string {
	t.Helper()
	claim, present := result.Claim()
	if !present {
		t.Fatalf("受理没交回索赔：outcome = %q", result.Outcome())
	}
	number, _ := claim.DocumentNumber().Number()
	return number
}

// Covers: AT-VE-229 真库半边、CONTEXT「客户索赔项」——记录提交时在同一事务内取得索赔单据号（按产品默认 `CLM-{yyyy}{mm}-{seq:5}`、租户
// 单据编号时区下的服务端日期），号随项落库、按（批次+项）读回带着号；同一（批次+项）重放答原号，不再取号——下一项接着数。
func TestAReceivedClaimTakesItsNumberInTheReceivingTransactionAndAReplayAnswersIt(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")

	first, err := fixture.receive(t, "SYN-TENANT-01", "item-1", false)
	if err != nil || first.Outcome() != application.ClaimReceived || numberOfReceipt(t, first) != "CLM-202611-00001" {
		t.Fatalf("首次受理：%v %q %q", err, first.Outcome(), numberOfReceipt(t, first))
	}
	stored := fixture.loadClaim(t, "SYN-TENANT-01", "item-1")
	if number, _ := stored.DocumentNumber().Number(); number != "CLM-202611-00001" {
		t.Fatalf("库里读回的号 = %q", number)
	}

	replay, err := fixture.receive(t, "SYN-TENANT-01", "item-1", false)
	if err != nil || replay.Outcome() != application.ClaimExistingResult || numberOfReceipt(t, replay) != "CLM-202611-00001" {
		t.Fatalf("重放：%v %q", err, replay.Outcome())
	}
	next, err := fixture.receive(t, "SYN-TENANT-01", "item-2", false)
	if err != nil || numberOfReceipt(t, next) != "CLM-202611-00002" {
		t.Fatalf("重放占了号：下一项 = %q %v", numberOfReceipt(t, next), err)
	}
}

func (fixture *numberingFixture) loadClaim(t *testing.T, tenant, item string) *domain.ClaimItem {
	t.Helper()
	claim, found, err := fixture.claims.FindByBatchItem(t.Context(),
		claimValue(t, domain.NewTenantID, tenant),
		claimValue(t, domain.NewClaimBatchReference, "SYN-BATCH-1"),
		claimValue(t, domain.NewClaimItemID, item))
	if err != nil || !found {
		t.Fatalf("读回索赔 %s/%s：%v found=%v", tenant, item, err, found)
	}
	return claim
}

// Covers: AT-VE-229 真库半边、ADR-0355 决定三——并发开立不撞号：同一租户同一周期里并发受理的各项各拿一个号，互不相同、连成一段，不留空号。
func TestConcurrentReceiptsNeverShareANumber(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")

	const receipts = 12
	numbers := make([]string, receipts)
	errs := make([]error, receipts)
	var wait sync.WaitGroup
	for index := range receipts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			var result application.HandleClaimResult
			errs[index] = fixture.transactor.WithinTransaction(context.Background(), func(ctx context.Context) error {
				handled, err := fixture.receipts.ReceiveClaim(ctx, numberingClaimCommand(t, "SYN-TENANT-01", fmt.Sprintf("item-%02d", index)))
				result = handled
				return err
			})
			if claim, present := result.Claim(); present {
				numbers[index], _ = claim.DocumentNumber().Number()
			}
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 份受理：%v", index, err)
		}
	}
	sort.Strings(numbers)
	for index, number := range numbers {
		if want := fmt.Sprintf("CLM-202611-%05d", index+1); number != want {
			t.Fatalf("并发受理的号 = %v，want CLM-202611-00001…%05d 各一个", numbers, receipts)
		}
	}
}

// Covers: AT-VE-229 真库半边、ADR-0355 决定三——同一（批次+项）并发受理：只落一项、只占一个号；输掉建行竞争的那一方答赢家与赢家的号，
// 它取过的号随它的事务回滚退回（DiscardIssuing），下一项接着数、不留空号。
func TestConcurrentReceiptsOfOneItemLandOnceAndLeaveNoGap(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")

	const racers = 6
	results := make([]application.HandleClaimResult, racers)
	errs := make([]error, racers)
	var wait sync.WaitGroup
	for index := range racers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs[index] = fixture.transactor.WithinTransaction(context.Background(), func(ctx context.Context) error {
				handled, err := fixture.receipts.ReceiveClaim(ctx, numberingClaimCommand(t, "SYN-TENANT-01", "item-1"))
				results[index] = handled
				if err == nil && handled.DiscardIssuing() {
					return errReceiptDiscarded
				}
				return err
			})
		}()
	}
	wait.Wait()
	received := 0
	for index, result := range results {
		if errs[index] != nil && !errors.Is(errs[index], errReceiptDiscarded) {
			t.Fatalf("第 %d 份受理：%v", index, errs[index])
		}
		switch result.Outcome() {
		case application.ClaimReceived:
			received++
		case application.ClaimExistingResult:
		default:
			t.Fatalf("第 %d 份受理答 %q", index, result.Outcome())
		}
		if number := numberOfReceipt(t, result); number != "CLM-202611-00001" {
			t.Fatalf("第 %d 份受理交回的号 = %q，want 赢家那一个", index, number)
		}
	}
	if received != 1 {
		t.Fatalf("同一项落了 %d 次", received)
	}
	if next, err := fixture.receive(t, "SYN-TENANT-01", "item-2", false); err != nil || numberOfReceipt(t, next) != "CLM-202611-00002" {
		t.Fatalf("竞争之后下一项拿到 %q（%v），want 紧接着的号", numberOfReceipt(t, next), err)
	}
}

// Covers: AT-VE-229、AT-VE-230 真库半边、ADR-0355 决定五（序列键含租户）与 ADR-0003——两个租户各数各的：都从 00001 起，另一个租户用过同一个号不算已用；
// 没登单据编号时区的租户照常受理、不带号。
func TestNumbersAreCountedPerTenant(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	fixture.registerTimeZone(t, "SYN-TENANT-02")

	for _, tenant := range []string{"SYN-TENANT-01", "SYN-TENANT-02"} {
		result, err := fixture.receive(t, tenant, "item-1", false)
		if err != nil || numberOfReceipt(t, result) != "CLM-202611-00001" {
			t.Fatalf("%s 的首号 = %q %v", tenant, numberOfReceipt(t, result), err)
		}
	}
	unconfigured, err := fixture.receive(t, "SYN-TENANT-03", "item-1", false)
	if err != nil || unconfigured.Outcome() != application.ClaimReceived {
		t.Fatalf("没登时区的租户：%v %q", err, unconfigured.Outcome())
	}
	claim, _ := unconfigured.Claim()
	if _, numbered := claim.DocumentNumber().Number(); numbered ||
		fixture.loadClaim(t, "SYN-TENANT-03", "item-1").DocumentNumber().NumberlessReason() != domain.NumberingTimeZoneNotConfigured {
		t.Fatal("没登单据编号时区的租户的索赔没记下无号原因")
	}
}

// Covers: AT-VE-227、AT-VE-229 真库半边、ADR-0355 决定五——重置周期随模板最细的日期部件：案件按日、索赔按月，周期一换流水从 1 起；周期用租户单据编号时区
// 下的服务端日期算（UTC 前一天 16:00 之后已是上海的次日）。
func TestTheSequenceResetsWithThePeriodInTheTenantsTimeZone(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	tenant := claimValue(t, domain.NewTenantID, "SYN-TENANT-01")

	issueCase := func() string {
		t.Helper()
		var issue ports.DocumentNumberIssue
		if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			var err error
			issue, err = fixture.numbers.IssueCaseNumber(ctx, tenant)
			return err
		}); err != nil || issue.Outcome != ports.DocumentNumberAssigned {
			t.Fatalf("取案件号：%+v %v", issue, err)
		}
		return issue.Number
	}
	steps := []struct {
		at   time.Time
		want string
	}{
		{numberingNow, "EX-20261102-0001"},
		{numberingNow.Add(time.Hour), "EX-20261102-0002"},
		{time.Date(2026, 11, 2, 16, 0, 0, 0, time.UTC), "EX-20261103-0001"},
		{time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC), "EX-20261103-0002"},
	}
	for _, step := range steps {
		fixture.clock.set(step.at)
		if got := issueCase(); got != step.want {
			t.Fatalf("%s 取到 %q，want %q", step.at, got, step.want)
		}
	}

	fixture.clock.set(numberingNow)
	if first, _ := fixture.receive(t, "SYN-TENANT-01", "item-1", false); numberOfReceipt(t, first) != "CLM-202611-00001" {
		t.Fatalf("十一月首号 = %q", numberOfReceipt(t, first))
	}
	fixture.clock.set(time.Date(2026, 11, 30, 16, 30, 0, 0, time.UTC)) // 上海已是十二月一日
	if december, _ := fixture.receive(t, "SYN-TENANT-01", "item-2", false); numberOfReceipt(t, december) != "CLM-202612-00001" {
		t.Fatalf("十二月首号 = %q", numberOfReceipt(t, december))
	}
}

// Covers: AT-VE-230 真库半边、ADR-0355 决定三——开立回滚释放号码：取了号、落了单的事务回滚，号随之退回，下一次开立拿到的还是它，库里没有那一项。
func TestARolledBackReceiptReleasesItsNumber(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")

	if rolled, err := fixture.receive(t, "SYN-TENANT-01", "item-1", true); err != nil || numberOfReceipt(t, rolled) != "CLM-202611-00001" {
		t.Fatalf("回滚那一次：%v", err)
	}
	if _, found, err := fixture.claims.FindByBatchItem(t.Context(),
		claimValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		claimValue(t, domain.NewClaimBatchReference, "SYN-BATCH-1"),
		claimValue(t, domain.NewClaimItemID, "item-1")); err != nil || found {
		t.Fatalf("回滚的那一项留在了库里：%v %v", found, err)
	}
	if next, err := fixture.receive(t, "SYN-TENANT-01", "item-2", false); err != nil || numberOfReceipt(t, next) != "CLM-202611-00001" {
		t.Fatalf("回滚后下一次开立拿到 %q，want 退回的那个号", numberOfReceipt(t, next))
	}
}

// Covers: AT-VE-228、AT-VE-230 真库半边、ADR-0355 决定八——撞号：租户换的格式渲染出的号连着撞上已用的号到了跳号上限，这一次提交不记下、答「编号规则撞号」；
// 取过的流水在取号那一步自己的保存点里退回，同一笔开立事务照常提交也不留空号——腾出一个旧号之后，下一次开立拿到的正是
// 第一个流水。案件那一类同理：撞号时同一事务里别的写入照常提交。
func TestACollisionAnswersAndReleasesEverythingItTook(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	fixture.registerClaimRule(t, "SYN-TENANT-01", "SYN-{seq:5}")
	for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.claim_item
				(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref, submitted_at, document_number)
			 VALUES ('SYN-TENANT-01', 'SYN-BATCH-OLD', $1, 'SYN-ACCOUNT-01', 'SYN-CONTRACT-01', 'SYN-PARCEL-OLD', 'LOSS', $2, $3)`,
			fmt.Sprintf("old-%02d", index), numberingNow.Add(-time.Hour), fmt.Sprintf("SYN-%05d", index),
		); err != nil {
			t.Fatalf("摆旧号：%v", err)
		}
	}

	collided, err := fixture.receive(t, "SYN-TENANT-01", "item-1", false)
	if err != nil || collided.Outcome() != application.ClaimNumberingRuleCollision || collided.DiscardIssuing() {
		t.Fatalf("撞号：%v %q discard=%v", err, collided.Outcome(), collided.DiscardIssuing())
	}
	if _, present := collided.Claim(); present {
		t.Fatal("撞号的提交交回了索赔")
	}
	var landed int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.claim_item WHERE tenant_id = 'SYN-TENANT-01' AND item_id = 'item-1'`,
	).Scan(&landed); err != nil || landed != 0 {
		t.Fatalf("撞号的提交落了单：%d %v", landed, err)
	}

	if _, err := fixture.pool.Exec(t.Context(),
		`DELETE FROM visibility_exception.claim_item WHERE tenant_id = 'SYN-TENANT-01' AND document_number = 'SYN-00001'`); err != nil {
		t.Fatal(err)
	}
	freed, err := fixture.receive(t, "SYN-TENANT-01", "item-1", false)
	if err != nil || numberOfReceipt(t, freed) != "SYN-00001" {
		t.Fatalf("撞号取过的流水没退回：下一次拿到 %q %v", numberOfReceipt(t, freed), err)
	}

	// 案件那一类：撞号退回只退取号那一段，同一事务里先落的那一行照常提交。
	tenant := claimValue(t, domain.NewTenantID, "SYN-TENANT-01")
	var registerErr error
	_ = fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, registerErr = fixture.registrar.RegisterRule(ctx, docnumber.RegisterRuleCommand{
			Tenant: "SYN-TENANT-01", Type: domain.ExceptionCaseDocumentNumberType.Code, Template: "SYN-{seq:5}",
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return registerErr
	})
	if registerErr != nil {
		t.Fatalf("登案件规则：%v", registerErr)
	}
	for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at, document_number)
			 VALUES ('SYN-TENANT-01', $1, 'SYN-PARCEL-OLD', 'SYN-PARCEL-OLD', 'SYN-TEAM', 'AWAITING_RESPONSE', $2, $3)`,
			fmt.Sprintf("old-case-%02d", index), numberingNow.Add(-time.Hour), fmt.Sprintf("SYN-%05d", index),
		); err != nil {
			t.Fatalf("摆旧案件号：%v", err)
		}
	}
	bystander, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:             claimValue(t, domain.NewClaimItemID, "bystander"),
		Batch:          claimValue(t, domain.NewClaimBatchReference, "SYN-BATCH-1"),
		DocumentNumber: domain.NumberlessForUnconfiguredTimeZone(),
		Customer:       claimValue(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Applicant:      claimValue(t, domain.NewApplicantReference, "SYN-APPLICANT-01"),
		Contract:       claimValue(t, domain.NewContractScopeReference, "SYN-CONTRACT-01"),
		Target:         claimValue(t, domain.NewRequestScopeReference, "SYN-PARCEL-BYSTANDER"),
		Kind:           claimValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt:    numberingNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	var issue ports.DocumentNumberIssue
	if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := fixture.claims.Save(ctx, tenant, bystander); err != nil {
			return err
		}
		var err error
		issue, err = fixture.numbers.IssueCaseNumber(ctx, tenant)
		return err
	}); err != nil {
		t.Fatalf("撞号那一笔事务没能照常提交：%v", err)
	}
	if issue.Outcome != ports.DocumentNumberingRuleCollision {
		t.Fatalf("案件撞号答 %+v", issue)
	}
	fixture.loadClaim(t, "SYN-TENANT-01", "bystander")
	if _, err := fixture.pool.Exec(t.Context(),
		`DELETE FROM visibility_exception.exception_case WHERE tenant_id = 'SYN-TENANT-01' AND document_number = 'SYN-00001'`); err != nil {
		t.Fatal(err)
	}
	if err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		issue, err = fixture.numbers.IssueCaseNumber(ctx, tenant)
		return err
	}); err != nil || issue.Number != "SYN-00001" {
		t.Fatalf("案件撞号取过的流水没退回：%+v %v", issue, err)
	}
}

// Covers: AT-VE-227 真库半边、CONTEXT「异常案件」——案件单据号与案件同一条记录落库、同生共死：事务回滚，号与案件一起不在，下一件拿到
// 退回的那个号。
func TestACaseNumberLandsWithItsCaseAndRollsBackWithIt(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	tenant := claimValue(t, domain.NewTenantID, "SYN-TENANT-01")

	raise := func(episodeID string, rollBack bool) (string, error) {
		t.Helper()
		var number string
		err := fixture.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
			issue, err := fixture.numbers.IssueCaseNumber(ctx, tenant)
			if err != nil {
				return err
			}
			documentNumber, err := domain.NewDocumentNumber(issue.Number)
			if err != nil {
				return err
			}
			number = issue.Number
			episode := openedEpisode(t, episodeID, "SYN-PARCEL-"+episodeID, "STALLED", factBaseAt)
			record := raisedRecord(t, episode, "SYN-TENANT-01", "SYN-PARCEL-"+episodeID, "STALLED", domain.ManualReviewRequired)
			record.Conclusion, err = domain.ConcludeTriage(episode.ID(), domain.AutoEstablishCase,
				factValue(t, domain.NewSignalRuleVersionReference, "triage-rule/v1"), factBaseAt.Add(5*time.Minute))
			if err != nil {
				return err
			}
			record.Case, err = domain.EstablishCase(
				factValue(t, domain.NewCaseID, "case-"+episodeID), documentNumber,
				factValue(t, domain.NewTrackedParcelReference, "SYN-PARCEL-"+episodeID),
				factValue(t, domain.NewImpactScopeReference, "SYN-PARCEL-"+episodeID),
				factValue(t, domain.NewResponsibleTeamReference, "SYN-TEAM"), factBaseAt.Add(5*time.Minute))
			if err != nil {
				return err
			}
			if err := fixture.episodes.SaveRaised(ctx, record); err != nil {
				return err
			}
			if rollBack {
				return errReceiptDiscarded
			}
			return nil
		})
		if errors.Is(err, errReceiptDiscarded) {
			err = nil
		}
		return number, err
	}
	if number, err := raise("ep-rolled", true); err != nil || number != "EX-20261102-0001" {
		t.Fatalf("回滚那一件：%q %v", number, err)
	}
	if number, err := raise("ep-kept", false); err != nil || number != "EX-20261102-0001" {
		t.Fatalf("回滚之后下一件拿到 %q（%v），want 退回的那个号", number, err)
	}
	rows, err := fixture.review.ListExceptionCases(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 || rows[0].CaseID != "case-ep-kept" || rows[0].DocumentNumber != "EX-20261102-0001" || rows[0].NumberlessReason != "" {
		t.Fatalf("案件册 = %+v %v", rows, err)
	}
}

// Covers: ADR-0355 决定三——取号只在开立事务里：两类单据的取号在事务外都被 RequireExecutor 拒（ErrTransactionRequired），
// 不改用连接池、不动序列。
func TestDocumentNumberIssuesRefuseToRunOutsideATransaction(t *testing.T) {
	fixture := newNumberingFixture(t)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	tenant := claimValue(t, domain.NewTenantID, "SYN-TENANT-01")
	if _, err := fixture.numbers.IssueCaseNumber(t.Context(), tenant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("事务外取案件号应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := fixture.numbers.IssueClaimNumber(t.Context(), tenant); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("事务外取索赔号应返回 ErrTransactionRequired，实得：%v", err)
	}
	if first, err := fixture.receive(t, "SYN-TENANT-01", "item-1", false); err != nil || numberOfReceipt(t, first) != "CLM-202611-00001" {
		t.Fatalf("事务外的两次被拒之后首号 = %q %v", numberOfReceipt(t, first), err)
	}
}

// Covers: ADR-0355 决定八（最后一道保险）与 0033 的取值域——同一租户同一个号只许一行（另一个租户可以同号），没有号的行
// 不参与唯一；无号原因只认「单据编号时区未配置」且不与号同在。两张表同一套。
func TestTheTablesKeepNumbersUniquePerTenantAndReasonsClosed(t *testing.T) {
	fixture := newNumberingFixture(t)
	insertCase := func(tenant, id string, number, reason any) error {
		_, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at, document_number, numberless_reason)
			 VALUES ($1, $2, 'SYN-PARCEL', 'SYN-PARCEL', 'SYN-TEAM', 'AWAITING_RESPONSE', $3, $4, $5)`,
			tenant, id, numberingNow, number, reason)
		return err
	}
	insertClaim := func(tenant, item string, number, reason any) error {
		_, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.claim_item
				(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref, submitted_at, document_number, numberless_reason)
			 VALUES ($1, 'SYN-BATCH-1', $2, 'SYN-ACCOUNT-01', 'SYN-CONTRACT-01', 'SYN-PARCEL', 'LOSS', $3, $4, $5)`,
			tenant, item, numberingNow, number, reason)
		return err
	}
	for name, insert := range map[string]func(tenant, id string, number, reason any) error{"exception_case": insertCase, "claim_item": insertClaim} {
		mustInsert := func(tenant, id string, number, reason any) {
			t.Helper()
			if err := insert(tenant, id, number, reason); err != nil {
				t.Fatalf("%s：(%s, %s, %v, %v) 该收没收：%v", name, tenant, id, number, reason, err)
			}
		}
		mustRefuse := func(code, tenant, id string, number, reason any) {
			t.Helper()
			var pgErr *pgconn.PgError
			if err := insert(tenant, id, number, reason); !errors.As(err, &pgErr) || pgErr.Code != code {
				t.Fatalf("%s：(%s, %s, %v, %v) 该拒（%s）却答 %v", name, tenant, id, number, reason, code, err)
			}
		}
		mustInsert("SYN-TENANT-01", "a", "N-1", nil)
		mustInsert("SYN-TENANT-02", "a", "N-1", nil)
		mustInsert("SYN-TENANT-01", "b", nil, nil)
		mustInsert("SYN-TENANT-01", "c", nil, nil)
		mustInsert("SYN-TENANT-01", "d", nil, "NUMBERING_TIME_ZONE_NOT_CONFIGURED")
		mustRefuse("23505", "SYN-TENANT-01", "e", "N-1", nil)
		mustRefuse("23514", "SYN-TENANT-01", "f", "N-2", "NUMBERING_TIME_ZONE_NOT_CONFIGURED")
		mustRefuse("23514", "SYN-TENANT-01", "g", nil, "ISSUED_BEFORE_NUMBERING")
		mustRefuse("23514", "SYN-TENANT-01", "h", " ", nil)
	}
}

// Covers: AT-VE-227、AT-VE-229 读面半边——案件侧查阅页读面的单据号——案件册与索赔册按行带号或无号原因（两格都空的旧行读成「上线前开立」），处置请求册与
// 追偿册带所属案件的单据号，案件没号就缺这一格、不拿案件标识顶。
func TestCaseReviewCarriesDocumentNumbersAndTheirAbsence(t *testing.T) {
	fixture := newCaseReviewFixture(t)
	ctx := t.Context()
	tenant := reviewValue(t, domain.NewTenantID, "tenant-a")

	fixture.insertReviewCase(t, "tenant-a", "case-1", "AWAITING_RESPONSE", caseBaseAt, nil, nil, nil, nil)
	fixture.insertReviewCase(t, "tenant-a", "case-2", "AWAITING_RESPONSE", caseBaseAt.Add(time.Hour), nil, nil, nil, nil)
	fixture.insertReviewCase(t, "tenant-a", "case-3", "AWAITING_RESPONSE", caseBaseAt.Add(2*time.Hour), nil, nil, nil, nil)
	if _, err := fixture.pool.Exec(ctx, `UPDATE visibility_exception.exception_case SET document_number = 'EX-20260302-0001'
		 WHERE tenant_id = 'tenant-a' AND case_id = 'case-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(ctx, `UPDATE visibility_exception.exception_case SET numberless_reason = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
		 WHERE tenant_id = 'tenant-a' AND case_id = 'case-2'`); err != nil {
		t.Fatal(err)
	}
	cases, err := fixture.review.ListExceptionCases(ctx, tenant, 10)
	if err != nil || len(cases) != 3 {
		t.Fatalf("案件册：%+v %v", cases, err)
	}
	byID := map[string]ports.ExceptionCaseCatalogueRow{}
	for _, row := range cases {
		byID[row.CaseID] = row
	}
	if byID["case-1"].DocumentNumber != "EX-20260302-0001" || byID["case-1"].NumberlessReason != "" ||
		byID["case-2"].DocumentNumber != "" || byID["case-2"].NumberlessReason != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" ||
		byID["case-3"].DocumentNumber != "" || byID["case-3"].NumberlessReason != "ISSUED_BEFORE_NUMBERING" {
		t.Fatalf("案件册的单据号两格：%+v", byID)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, tenant, sentRequest(t, "request-1", 1)); err != nil {
			return err
		}
		if _, err := fixture.recoveries.Save(txCtx, tenant, openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")); err != nil {
			return err
		}
		_, err := fixture.recoveries.Save(txCtx, tenant, openedMatter(t, "recovery-2", "case-3", "supplier-1", "parcel-3/loss"))
		return err
	})
	requests, err := fixture.review.ListDispositionRequests(ctx, tenant, 10)
	if err != nil || len(requests) != 1 || requests[0].CaseID != "case-1" || requests[0].CaseDocumentNumber != "EX-20260302-0001" {
		t.Fatalf("处置请求册：%+v %v", requests, err)
	}
	matters, err := fixture.review.ListRecoveryMatters(ctx, tenant, 10)
	if err != nil || len(matters) != 2 {
		t.Fatalf("追偿册：%+v %v", matters, err)
	}
	for _, matter := range matters {
		want := map[string]string{"case-1": "EX-20260302-0001", "case-3": ""}[matter.CaseID]
		if matter.CaseDocumentNumber != want {
			t.Fatalf("追偿 %s 的案件单据号 = %q，want %q", matter.MatterID, matter.CaseDocumentNumber, want)
		}
	}

	numbered, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:             reviewValue(t, domain.NewClaimItemID, "item-1"),
		Batch:          reviewValue(t, domain.NewClaimBatchReference, "batch-1"),
		DocumentNumber: reviewValue(t, domain.NewDocumentNumber, "CLM-202603-00001"),
		Customer:       reviewValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:      reviewValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:       reviewValue(t, domain.NewContractScopeReference, "contract-1"),
		Target:         reviewValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Kind:           reviewValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt:    caseBaseAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.claims.Save(txCtx, tenant, numbered)
		return err
	})
	items, err := fixture.review.ListClaimItems(ctx, tenant, 10)
	if err != nil || len(items) != 1 || items[0].DocumentNumber != "CLM-202603-00001" || items[0].NumberlessReason != "" ||
		items[0].Batch != "batch-1" || items[0].ItemID != "item-1" {
		t.Fatalf("索赔册：%+v %v", items, err)
	}
}

// Covers: 0033 在有数据的库上施加——施加之前落下的案件与索赔行一行不改：两格都空（读成「上线前开立」），不回填、不补号。
func TestTheDocumentNumberMigrationLeavesExistingRowsUnnumbered(t *testing.T) {
	const documentNumbersMigration = "0033_exception_case_and_claim_document_numbers"
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.WithoutCancel(ctx)) })
	if _, err := conn.Exec(ctx, `CREATE SCHEMA visibility_exception`); err != nil {
		t.Fatal(err)
	}
	assets, err := migrations.VisibilityException()
	if err != nil {
		t.Fatal(err)
	}
	var migration *migrations.Asset
	for index := range assets {
		if assets[index].Name == documentNumbersMigration {
			migration = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if migration == nil {
		t.Fatalf("没有找到迁移 %s", documentNumbersMigration)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.exception_case
			(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at)
		 VALUES ('SYN-TENANT-01', 'case-old', 'SYN-PARCEL', 'SYN-PARCEL', 'SYN-TEAM', 'AWAITING_RESPONSE', '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatalf("落旧形状的案件：%v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref, submitted_at)
		 VALUES ('SYN-TENANT-01', 'SYN-BATCH-OLD', 'item-old', 'SYN-ACCOUNT-01', 'SYN-CONTRACT-01', 'SYN-PARCEL', 'LOSS', '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatalf("落旧形状的索赔：%v", err)
	}
	if _, err := conn.Exec(ctx, migration.SQL); err != nil {
		t.Fatalf("在有数据的库上施加 %s：%v", documentNumbersMigration, err)
	}
	for _, table := range []string{"exception_case", "claim_item"} {
		var numbered, reasoned int
		if err := conn.QueryRow(ctx, `SELECT count(document_number), count(numberless_reason) FROM visibility_exception.`+table).
			Scan(&numbered, &reasoned); err != nil || numbered != 0 || reasoned != 0 {
			t.Fatalf("%s 的旧行被回填了：号 %d 行、原因 %d 行 %v", table, numbered, reasoned, err)
		}
	}
}
