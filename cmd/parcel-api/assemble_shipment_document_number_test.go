package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	psdocnumbering "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/docnumbering"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pshandoff "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/productionhandoff"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真库上证委托的单据号（ADR-0355 决定三、七、八；票 document-numbering/04）：提交编排、续办守卫、取号编排、平台序列与
// 委托表全是生产件，只有归属读口换放行替身、取号时钟换可拨的替身——周期重置与按时区出日期要拨到指定时刻才看得见。

// documentAssignerForTest 装一套与生产同款的取号编排（docnumbering.Build，服务端时钟），给只要「提交能开立单据号」的装配用例。
func documentAssignerForTest(t *testing.T, db *bentopg.DB) *docnumber.Assigner {
	t.Helper()
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatalf("装配编号组件：%v", err)
	}
	return numbers.Assigner
}

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
	pool          *pgxpool.Pool
	clock         *movableClock
	handler       *shipmentapp.SubmitShipmentRequestHandler
	registrations docnumbering.Registrations
}

// newNumberingFixture 装提交编排，取号编排用可拨时钟；wrapRecorder 非空时包住记号那一口，好在记号之后注入失败，看守卫那一笔
// 整笔回滚时号码是否随之释放。
func newNumberingFixture(t *testing.T, wrapRecorder func(ports.ShipmentDocumentNumberRecorder) ports.ShipmentDocumentNumberRecorder) *numberingFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := docnumbering.Catalogue()
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
	clock := &movableClock{at: time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}

	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	submitClock := fixedClock{at: envelopeProofAnchor}
	handoff, err := pspostgres.NewOutboxShipmentRequestSubmittedHandoff(db, store, submitClock)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		t.Fatal(err)
	}
	answers, err := pspostgres.NewSubmissionAnswers(db)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := psdocnumbering.NewIssuer(assigner, requests)
	if err != nil {
		t.Fatal(err)
	}
	var recorder ports.ShipmentDocumentNumberRecorder = requests
	if wrapRecorder != nil {
		recorder = wrapRecorder(requests)
	}
	handler := shipmentapp.NewSubmitShipmentRequestHandler(
		preservationBoundary{transactor: db.Transactor(), inner: sources},
		submissionBoundary{transactor: db.Transactor(), inner: requests, handoff: handoff},
		permittingOwnership{anchor: envelopeProofAnchor},
		submissionAnswerBoundary{transactor: db.Transactor(), inner: answers},
		submissionContinuationGuard{transactor: db.Transactor(), locks: answers},
		pshandoff.UnconfiguredOtherProductionAuthorityChannel{},
		identities,
		submitClock,
		issuer,
		recorder,
	)
	return &numberingFixture{pool: pool, clock: clock, handler: handler, registrations: numbers.Registrations}
}

func (fixture *numberingFixture) registerTimeZone(t *testing.T, tenant string) {
	t.Helper()
	registered, err := fixture.registrations.RegisterTimeZone(t.Context(), docnumber.RegisterTimeZoneCommand{
		Tenant: tenant, TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	})
	if err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登记单据编号时区：%+v %v", registered, err)
	}
}

func (fixture *numberingFixture) registerRule(t *testing.T, tenant, template string, effectiveFrom time.Time) {
	t.Helper()
	registered, err := fixture.registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: tenant, Type: domain.ShipmentDocumentNumberType.Code, Template: template, EffectiveFrom: effectiveFrom, RegisteredBy: "SYN-TEST",
	})
	if err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登记规则 %s：%+v %v", template, registered, err)
	}
}

func (fixture *numberingFixture) submit(t *testing.T, tenant, key string) shipmentapp.SubmitShipmentRequestResult {
	t.Helper()
	result, err := fixture.handler.Handle(t.Context(), numberingCommand(t, tenant, key))
	if err != nil {
		t.Fatalf("提交 %s/%s：%v", tenant, key, err)
	}
	return result
}

// stored 读委托行上的两列：单据号与无号原因。found 为假即这一键下没有委托行。
func (fixture *numberingFixture) stored(t *testing.T, tenant, key string) (number, absence string, found bool) {
	t.Helper()
	var numberCell, absenceCell *string
	err := fixture.pool.QueryRow(t.Context(),
		`SELECT document_number, document_number_absence
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1 AND source_request_key = $2`, tenant, key).Scan(&numberCell, &absenceCell)
	if err != nil {
		return "", "", false
	}
	if numberCell != nil {
		number = *numberCell
	}
	if absenceCell != nil {
		absence = *absenceCell
	}
	return number, absence, true
}

func numberOf(t *testing.T, result shipmentapp.SubmitShipmentRequestResult) string {
	t.Helper()
	number, numbered := result.DocumentNumbering().Number()
	if !numbered {
		t.Fatalf("%s 没有带单据号：%+v", result.Outcome(), result.DocumentNumbering())
	}
	return number.String()
}

// numberingCommand 与 submissionCommand 同一配方，租户与来源请求键可变：取号按租户分序列，各笔提交要各是一份委托。
func numberingCommand(t *testing.T, tenant, key string) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	command := submissionCommand(t)
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, tenant),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Identity = identity
	command.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-"+key)
	command.DeclaredParcelIDs = []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-"+key)}
	return command
}

// Covers: PS CONTEXT「委托」草稿 → 已提交时在同一提交事务内取得单据号，同一提交重放答原号（ADR-0355 决定三）；日期按租户的单据
// 编号时区、按日重置（决定五、七）；序列按租户分（ADR-0003）。重放不取号：下一份委托接着上一个流水。
//
// Covers: `AT-PS-177`。
func TestASubmittedShipmentTakesItsDocumentNumberAndAReplayAnswersTheOriginal(t *testing.T) {
	fixture := newNumberingFixture(t, nil)
	fixture.registerTimeZone(t, "SYN-TENANT-A")
	fixture.registerTimeZone(t, "SYN-TENANT-B")

	first := fixture.submit(t, "SYN-TENANT-A", "KEY-1")
	if first.Outcome() != shipmentapp.OutcomeSubmitted || numberOf(t, first) != "P20261102000001" {
		t.Fatalf("首次提交：%s %+v", first.Outcome(), first.DocumentNumbering())
	}
	if number, absence, _ := fixture.stored(t, "SYN-TENANT-A", "KEY-1"); number != "P20261102000001" || absence != "" {
		t.Fatalf("委托行上的号 = %q / %q", number, absence)
	}

	replay := fixture.submit(t, "SYN-TENANT-A", "KEY-1")
	if replay.Outcome() != shipmentapp.OutcomeExistingResult || numberOf(t, replay) != "P20261102000001" {
		t.Fatalf("重放：%s %+v", replay.Outcome(), replay.DocumentNumbering())
	}
	if second := fixture.submit(t, "SYN-TENANT-A", "KEY-2"); numberOf(t, second) != "P20261102000002" {
		t.Fatalf("第二份委托 = %s；重放不该占号", numberOf(t, second))
	}
	if other := fixture.submit(t, "SYN-TENANT-B", "KEY-1"); numberOf(t, other) != "P20261102000001" {
		t.Fatalf("另一租户的首份委托 = %s，序列按租户分", numberOf(t, other))
	}

	// 上海时区跨了午夜：UTC 仍是 11 月 2 日，号上的日期是 11 月 3 日，流水从 1 重数。
	fixture.clock.set(time.Date(2026, 11, 2, 16, 30, 0, 0, time.UTC))
	if nextDay := fixture.submit(t, "SYN-TENANT-A", "KEY-3"); numberOf(t, nextDay) != "P20261103000001" {
		t.Fatalf("次日首份委托 = %s", numberOf(t, nextDay))
	}
}

// Covers: ADR-0355 决定七——开立方没有业务时区、租户也没登单据编号时区：委托照常建成，不带单据号，记下「单据编号时区未配置」，
// 序列一个值也不动；之后补登也不补号。
//
// Covers: `AT-PS-178`。
func TestAShipmentWithoutANumberingTimeZoneIsSubmittedWithoutANumber(t *testing.T) {
	fixture := newNumberingFixture(t, nil)

	result := fixture.submit(t, "SYN-TENANT-C", "KEY-1")
	if result.Outcome() != shipmentapp.OutcomeSubmitted {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if reason, absent := result.DocumentNumbering().Absence(); !absent || reason != domain.DocumentNumberingTimeZoneNotConfigured {
		t.Fatalf("无号原因 = %v（%v）", reason, absent)
	}
	if number, absence, _ := fixture.stored(t, "SYN-TENANT-C", "KEY-1"); number != "" || absence != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("委托行 = %q / %q", number, absence)
	}
	var sequences int
	if err := fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM platform.numbering_sequence WHERE scope = 'SYN-TENANT-C'`).Scan(&sequences); err != nil || sequences != 0 {
		t.Fatalf("时区未配置却动了序列：%d %v", sequences, err)
	}

	fixture.registerTimeZone(t, "SYN-TENANT-C")
	replay := fixture.submit(t, "SYN-TENANT-C", "KEY-1")
	if _, numbered := replay.DocumentNumbering().Number(); numbered {
		t.Fatal("补登单据编号时区之后，旧委托的重放被补上了号")
	}
	if fresh := fixture.submit(t, "SYN-TENANT-C", "KEY-2"); numberOf(t, fresh) != "P20261102000001" {
		t.Fatalf("补登之后的新委托 = %s", numberOf(t, fresh))
	}
}

// failingOnceRecorder 在真记号之后报一次错，扮演同一笔事务里号已取、已记而事务终究回滚。
type failingOnceRecorder struct {
	inner  ports.ShipmentDocumentNumberRecorder
	failed bool
}

var errRecorderInjected = errors.New("injected failure after the number was recorded")

func (recorder *failingOnceRecorder) RecordShipmentDocumentNumbering(ctx context.Context, identity domain.SourceIdentity, request domain.ShipmentRequest) error {
	if err := recorder.inner.RecordShipmentDocumentNumbering(ctx, identity, request); err != nil {
		return err
	}
	if !recorder.failed {
		recorder.failed = true
		return errRecorderInjected
	}
	return nil
}

// Covers: ADR-0355 决定三「开立回滚，号码随之释放」——号已取、已记在委托行上，守卫那一笔终究回滚：委托不在、号不占，重发接着办，
// 拿到的仍是那个号。
//
// Covers: `AT-PS-177`。
func TestARolledBackSubmissionReleasesItsDocumentNumber(t *testing.T) {
	fixture := newNumberingFixture(t, func(inner ports.ShipmentDocumentNumberRecorder) ports.ShipmentDocumentNumberRecorder {
		return &failingOnceRecorder{inner: inner}
	})
	fixture.registerTimeZone(t, "SYN-TENANT-A")

	if _, err := fixture.handler.Handle(t.Context(), numberingCommand(t, "SYN-TENANT-A", "KEY-1")); !errors.Is(err, errRecorderInjected) {
		t.Fatalf("注入的失败没有上抛：%v", err)
	}
	if _, _, found := fixture.stored(t, "SYN-TENANT-A", "KEY-1"); found {
		t.Fatal("回滚的那一笔留下了委托行")
	}
	retried := fixture.submit(t, "SYN-TENANT-A", "KEY-1")
	if retried.Outcome() != shipmentapp.OutcomeSubmitted || numberOf(t, retried) != "P20261102000001" {
		t.Fatalf("重发：%s %s——回滚该把流水 1 一起释放", retried.Outcome(), numberOf(t, retried))
	}
}

// Covers: ADR-0355 决定八「同一序列分出的号互不相同」——同一租户同一天并发提交，各得其号、不撞，流水连续不空。
//
// Covers: `AT-PS-177`。
func TestConcurrentSubmissionsTakeDistinctDocumentNumbers(t *testing.T) {
	fixture := newNumberingFixture(t, nil)
	fixture.registerTimeZone(t, "SYN-TENANT-A")

	const submissions = 12
	commands := make([]shipmentapp.SubmitShipmentRequestCommand, submissions)
	for index := range submissions {
		commands[index] = numberingCommand(t, "SYN-TENANT-A", fmt.Sprintf("KEY-%02d", index))
	}
	numbers := make([]string, submissions)
	errs := make([]error, submissions)
	var wait sync.WaitGroup
	for index := range submissions {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := fixture.handler.Handle(context.Background(), commands[index])
			if err != nil {
				errs[index] = err
				return
			}
			number, numbered := result.DocumentNumbering().Number()
			if !numbered {
				errs[index] = fmt.Errorf("%s 没带号", result.Outcome())
				return
			}
			numbers[index] = number.String()
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 份：%v", index, err)
		}
	}
	sort.Strings(numbers)
	for index, number := range numbers {
		if want := fmt.Sprintf("P20261102%06d", index+1); number != want {
			t.Fatalf("并发取号 = %v，第 %d 个应为 %s", numbers, index, want)
		}
	}
}

// Covers: ADR-0355 决定八——租户换了格式，新号整段撞上已用的旧号：跳过到上限仍撞，这一次不建单，答「编号规则撞号」；守卫那一笔整笔
// 回滚，取过的流水一并释放（换了不撞的格式后第一个号从 1 起），答案不记、重发接着办。
//
// Covers: `AT-PS-179`。
func TestANumberingRuleCollisionRollsTheSubmissionBack(t *testing.T) {
	fixture := newNumberingFixture(t, nil)
	fixture.registerTimeZone(t, "SYN-TENANT-A")

	for index := range docnumber.MaxSkippedNumbers {
		fixture.submit(t, "SYN-TENANT-A", fmt.Sprintf("OLD-%02d", index))
	}
	// 不分周期的新格式从流水 1 起，渲染出的前二十个号正是默认格式当天已用的那二十个。
	switchAt := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	fixture.registerRule(t, "SYN-TENANT-A", "P20261102{seq:6}", switchAt)
	fixture.clock.set(switchAt.Add(time.Minute))

	collided := fixture.submit(t, "SYN-TENANT-A", "NEW-1")
	if collided.Outcome() != shipmentapp.OutcomeNumberingRuleCollision {
		t.Fatalf("outcome = %s, want NUMBERING_RULE_COLLISION", collided.Outcome())
	}
	if _, named := collided.ShipmentRequestID(); named {
		t.Fatal("撞号答复指名了一份被回滚的委托")
	}
	if _, _, found := fixture.stored(t, "SYN-TENANT-A", "NEW-1"); found {
		t.Fatal("撞号那一笔留下了委托行")
	}
	if again := fixture.submit(t, "SYN-TENANT-A", "NEW-1"); again.Outcome() != shipmentapp.OutcomeNumberingRuleCollision {
		t.Fatalf("重发 = %s；答案没记，规则册没换之前重发仍撞", again.Outcome())
	}

	fixedAt := switchAt.Add(time.Hour)
	fixture.registerRule(t, "SYN-TENANT-A", "Q{seq:6}", fixedAt)
	fixture.clock.set(fixedAt.Add(time.Minute))
	resumed := fixture.submit(t, "SYN-TENANT-A", "NEW-1")
	if resumed.Outcome() != shipmentapp.OutcomeSubmitted || numberOf(t, resumed) != "Q000001" {
		t.Fatalf("换格式后重发：%s %s——撞号两次取过的流水都该随回滚释放", resumed.Outcome(), numberOf(t, resumed))
	}
}
