package main

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL、经生产装配的同一组构造函数（buildDeliveryPreAdviceOrchestration、buildDeliveryHandoverNoteOrchestration）证
// 送站预报与送站交接单的单据号（ADR-0355 决定三、七、八；票 document-numbering/08「每类单据都要做到」第 8 条）：同一开立重放答原号、
// 并发开立不撞号也不为重投多取号、租户隔离、周期重置、开立回滚释放号码、撞号答案；日期按开立网点的业务时区（网络目录节点版本），
// 网点没有时退到租户的单据编号时区，都没有照常开立不带号。取号编排换一只可拨的时钟——生产上它就是服务端时钟（docnumbering.Build），
// 拨钟只为在一个用例里跨日。合成租户、网点与号码只记 `S`。

// steppedClock 是可拨的服务端时钟。
type steppedClock struct {
	mu sync.Mutex
	at time.Time
}

func (clock *steppedClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.at
}

func (clock *steppedClock) set(at time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.at = at
}

// testDocumentNumberAssigner 交回组合根同一个装配（docnumbering.Build）装出的取号编排：别的装配用例要装编排，就用生产那一只。
func testDocumentNumberAssigner(t *testing.T, db *bentopg.DB) *docnumber.Assigner {
	t.Helper()
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	return numbers.Assigner
}

// clockedDocumentNumberAssigner 与 docnumbering.Build 同一份类型汇总、同一本册、同一组序列，只把时钟换成 clock。
func clockedDocumentNumberAssigner(t *testing.T, db *bentopg.DB, clock docnumber.Clock) *docnumber.Assigner {
	t.Helper()
	catalogue, err := docnumbering.Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	store, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: store, Sequences: sequences, Transactions: store, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	return assigner
}

type documentNumberRig struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	clock      *steppedClock
	preAdvices nodeopshttp.DeliveryPreAdviceHandler
	notes      nodeopshttp.DeliveryHandoverNoteHandler
}

func newDocumentNumberRig(t *testing.T) *documentNumberRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	clock := &steppedClock{}
	assigner := clockedDocumentNumberAssigner(t, db, clock)
	preAdvices, err := buildDeliveryPreAdviceOrchestration(db, assigner)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := buildDeliveryHandoverNoteOrchestration(db, assigner)
	if err != nil {
		t.Fatal(err)
	}
	return &documentNumberRig{pool: pool, db: db, clock: clock, preAdvices: preAdvices, notes: notes}
}

// registerNode 在网络目录登一个网点版本：业务时区 zone，自 2000 年起生效。
func (rig *documentNumberRig) registerNode(t *testing.T, tenant, node, zone string) {
	t.Helper()
	catalog, err := nrpostgres.NewNetworkCatalog(rig.db)
	if err != nil {
		t.Fatal(err)
	}
	nrTenant, err := nrdomain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if err := rig.db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
		return catalog.RegisterNodeVersion(ctx, nrTenant, nrports.NodeDefinitionVersion{
			Code: node, Version: 1, BusinessTimezone: zone, EffectiveFrom: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}); err != nil {
		t.Fatalf("登网点 %s：%v", node, err)
	}
}

// registerNumberingTimeZone 给租户登一版单据编号时区，自 2000 年起生效。
func (rig *documentNumberRig) registerNumberingTimeZone(t *testing.T, tenant, zone string) {
	t.Helper()
	numbers, err := docnumbering.Build(rig.db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := numbers.Registrations.RegisterTimeZone(context.Background(), docnumber.RegisterTimeZoneCommand{
		Tenant: tenant, TimeZone: zone, EffectiveFrom: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	})
	if err != nil || registered.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registered, err)
	}
}

func (rig *documentNumberRig) submit(t *testing.T, tenant, node, requestKey string, numbers ...string) (nodeopsapp.DeliveryPreAdviceSubmissionResult, error) {
	t.Helper()
	command := nodeopsapp.SubmitDeliveryPreAdviceCommand{
		TenantID:          mustValue(t, nodomain.NewTenantID, tenant),
		CustomerAccount:   mustValue(t, nodomain.NewCustomerAccountReference, "SYN-ACCOUNT-DN-01"),
		Channel:           "https://id.syn.example/dex#syn-customer-client-dn",
		RequestKey:        requestKey,
		Node:              mustValue(t, nodomain.NewNodeReference, node),
		ExpectedArrivalAt: time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC),
	}
	for _, number := range numbers {
		command.Numbers = append(command.Numbers, mustValue(t, nodomain.NewPreAdvisedParcelNumber, number))
	}
	return rig.preAdvices.Submit(context.Background(), command)
}

func (rig *documentNumberRig) open(t *testing.T, tenant, node, noteID string, openedAt time.Time) (nodeopsapp.DeliveryHandoverResult, error) {
	t.Helper()
	return rig.notes.Open(context.Background(), nodeopsapp.OpenDeliveryHandoverNoteCommand{
		TenantID:        mustValue(t, nodomain.NewTenantID, tenant),
		Node:            mustValue(t, nodomain.NewNodeReference, node),
		Note:            mustValue(t, nodomain.NewDeliveryHandoverNoteID, noteID),
		DeliveringParty: mustValue(t, nodomain.NewDeliveringPartyReference, "SYN-DRIVER-DN"),
		ClaimedAccounts: []nodomain.CustomerAccountReference{mustValue(t, nodomain.NewCustomerAccountReference, "SYN-ACCOUNT-DN-01")},
		OpenedAt:        openedAt,
	})
}

// sequenceValue 读平台序列表上（租户，类型，周期）那一行最后取出的值（平台序列表的 next_val 存的就是它）；没有那一行答 0。
func (rig *documentNumberRig) sequenceValue(t *testing.T, tenant, docType, period string) int64 {
	t.Helper()
	var next int64
	err := rig.pool.QueryRow(context.Background(),
		`SELECT coalesce((SELECT next_val FROM platform.numbering_sequence WHERE scope = $1 AND doc_type = $2 AND period = $3), 0)`,
		tenant, docType, period,
	).Scan(&next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func preAdviceNumberOf(result nodeopsapp.DeliveryPreAdviceSubmissionResult) string {
	number, _ := result.PreAdvice().DocumentNumbering().Number()
	return number.String()
}

func noteNumberOf(result nodeopsapp.DeliveryHandoverResult) string {
	number, _ := result.Note().DocumentNumbering().Number()
	return number.String()
}

// Covers: AT-NO-135（真库）——预报进入已登记时取单据号：日期按预报指名的网点此刻的业务时区（上海已过零点、纽约与 UTC 还在前一日，号
// 是上海那一日的），网点不在网络目录时退到租户的单据编号时区，两者都没有照常登记、记单据编号时区未配置；同一接入请求原样重投答原号；
// 跨日从 1 起；另一租户同一时刻同一网点从 1 起；并发十二份各得一号、互不相同、连成一段，同一接入请求并发五次只登一份、只取一个号。
func TestPreAdviceDocumentNumbersOnARealDatabase(t *testing.T) {
	rig := newDocumentNumberRig(t)
	const tenant, other, bare = "SYN-TENANT-DN-1", "SYN-TENANT-DN-2", "SYN-TENANT-DN-3"
	rig.registerNode(t, tenant, "SYN-NODE-SHA-HUB", "Asia/Shanghai")
	rig.registerNode(t, other, "SYN-NODE-SHA-HUB", "Asia/Shanghai")
	rig.registerNumberingTimeZone(t, tenant, "America/New_York")

	rig.clock.set(time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC))
	first, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-01", "SYN-PCL-DN-01")
	if err != nil || first.Outcome() != nodeopsapp.DeliveryPreAdviceRegistered || preAdviceNumberOf(first) != "PA-20261103-00001" {
		t.Fatalf("首份 = %s %q %v，want 上海那一日的 PA-20261103-00001", first.Outcome(), preAdviceNumberOf(first), err)
	}
	replay, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-01", "SYN-PCL-DN-01")
	if err != nil || replay.Outcome() != nodeopsapp.DeliveryPreAdviceExistingResult || preAdviceNumberOf(replay) != "PA-20261103-00001" ||
		replay.PreAdvice().ID() != first.PreAdvice().ID() {
		t.Fatalf("原样重投 = %s %q %v", replay.Outcome(), preAdviceNumberOf(replay), err)
	}
	if taken := rig.sequenceValue(t, tenant, "DELIVERY_PRE_ADVICE", "20261103"); taken != 1 {
		t.Fatalf("重投之后序列取到 %d，want 1（重投不取号）", taken)
	}
	isolated, err := rig.submit(t, other, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-01", "SYN-PCL-DN-01")
	if err != nil || preAdviceNumberOf(isolated) != "PA-20261103-00001" {
		t.Fatalf("另一租户 = %q %v，want 从 1 起", preAdviceNumberOf(isolated), err)
	}
	fallback, err := rig.submit(t, tenant, "SYN-NODE-NOWHERE", "SYN-REQ-DN-02", "SYN-PCL-DN-02")
	if err != nil || preAdviceNumberOf(fallback) != "PA-20261102-00001" {
		t.Fatalf("网点不在网络目录 = %q %v，want 纽约那一日的 PA-20261102-00001", preAdviceNumberOf(fallback), err)
	}
	withheld, err := rig.submit(t, bare, "SYN-NODE-NOWHERE", "SYN-REQ-DN-03", "SYN-PCL-DN-03")
	if absence, absent := withheld.PreAdvice().DocumentNumbering().Absence(); err != nil || withheld.Outcome() != nodeopsapp.DeliveryPreAdviceRegistered ||
		!absent || absence != nodomain.NumberingTimeZoneNotConfigured {
		t.Fatalf("两者都没有 = %s %v %v %v", withheld.Outcome(), absence, absent, err)
	}

	rig.clock.set(time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC))
	nextDay, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-04", "SYN-PCL-DN-04")
	if err != nil || preAdviceNumberOf(nextDay) != "PA-20261104-00001" {
		t.Fatalf("跨日 = %q %v，want 从 1 起", preAdviceNumberOf(nextDay), err)
	}

	rig.clock.set(time.Date(2026, 11, 4, 17, 0, 0, 0, time.UTC))
	const concurrent = 12
	numbers := make([]string, concurrent)
	failures := make([]error, concurrent)
	var wait sync.WaitGroup
	for index := range concurrent {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", fmt.Sprintf("SYN-REQ-DN-PAR-%02d", index), "SYN-PCL-DN-PAR")
			numbers[index], failures[index] = preAdviceNumberOf(result), err
		}()
	}
	wait.Wait()
	slices.Sort(numbers)
	for index, number := range numbers {
		if failures[index] != nil {
			t.Fatalf("并发第 %d 份：%v", index, failures[index])
		}
		if want := fmt.Sprintf("PA-20261105-%05d", index+1); number != want {
			t.Fatalf("并发取到的号 = %v，want 00001 起连成一段", numbers)
		}
	}

	outcomes := make([]nodeopsapp.DeliveryPreAdviceOutcome, 5)
	replayed := make([]string, 5)
	for index := range outcomes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-SAME", "SYN-PCL-DN-SAME")
			if err == nil {
				outcomes[index], replayed[index] = result.Outcome(), preAdviceNumberOf(result)
			}
		}()
	}
	wait.Wait()
	registered := 0
	for index, outcome := range outcomes {
		if outcome == nodeopsapp.DeliveryPreAdviceRegistered {
			registered++
		}
		if replayed[index] != "PA-20261105-00013" {
			t.Fatalf("同一接入请求并发重投答的号 = %v", replayed)
		}
	}
	if taken := rig.sequenceValue(t, tenant, "DELIVERY_PRE_ADVICE", "20261105"); registered != 1 || taken != concurrent+1 {
		t.Fatalf("同一接入请求并发五次：登了 %d 份，序列取到 %d，want 一份、%d", registered, taken, concurrent+1)
	}
}

// Covers: AT-NO-135 的撞号一格（真库）——当日的前二十个号都已被别的写入占了，提交答编号规则撞号：册上没有这份预报，整笔回滚，序列
// 上取过的二十个值一个也不留；规则册换了格式之后（这里是让出一个号）同一接入请求重交即登记，拿到的是那一个空着的号。
func TestAPreAdviceNumberingCollisionRollsBackOnARealDatabase(t *testing.T) {
	rig := newDocumentNumberRig(t)
	const tenant = "SYN-TENANT-DN-1"
	rig.registerNode(t, tenant, "SYN-NODE-SHA-HUB", "Asia/Shanghai")
	rig.clock.set(time.Date(2026, 11, 9, 17, 0, 0, 0, time.UTC))
	for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
		if _, err := rig.pool.Exec(context.Background(),
			`INSERT INTO node_operations.delivery_pre_advice
				(tenant_id, pre_advice_id, customer_account_id, node_ref, expected_arrival_at, parcel_numbers,
				 source_channel, source_request_key, content_digest, formed_at, document_number)
			 VALUES ($1, $2, 'SYN-ACCOUNT-DN-01', 'SYN-NODE-SHA-HUB', now(), ARRAY['SYN-PCL-OLD'], 'SYN-CH-OLD', $2, 'sha256:old', now(), $3)`,
			tenant, fmt.Sprintf("SYN-PREADV-OLD-%02d", index), fmt.Sprintf("PA-20261110-%05d", index),
		); err != nil {
			t.Fatal(err)
		}
	}

	collided, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-CLASH", "SYN-PCL-DN-CLASH")
	if err != nil || collided.Outcome() != nodeopsapp.DeliveryPreAdviceNumberingRuleCollision || collided.PreAdvice().ID().String() != "" {
		t.Fatalf("撞号 = %s %v", collided.Outcome(), err)
	}
	if taken := rig.sequenceValue(t, tenant, "DELIVERY_PRE_ADVICE", "20261110"); taken != 0 {
		t.Fatalf("撞号回滚之后序列上还留着 %d", taken)
	}
	var landed bool
	if err := rig.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM node_operations.delivery_pre_advice WHERE tenant_id = $1 AND source_request_key = 'SYN-REQ-DN-CLASH')`, tenant,
	).Scan(&landed); err != nil || landed {
		t.Fatalf("撞号也落了预报：%v %v", landed, err)
	}

	if _, err := rig.pool.Exec(context.Background(),
		`DELETE FROM node_operations.delivery_pre_advice WHERE tenant_id = $1 AND document_number = 'PA-20261110-00007'`, tenant); err != nil {
		t.Fatal(err)
	}
	retried, err := rig.submit(t, tenant, "SYN-NODE-SHA-HUB", "SYN-REQ-DN-CLASH", "SYN-PCL-DN-CLASH")
	if err != nil || retried.Outcome() != nodeopsapp.DeliveryPreAdviceRegistered || preAdviceNumberOf(retried) != "PA-20261110-00007" {
		t.Fatalf("让出一个号之后重交 = %s %q %v", retried.Outcome(), preAdviceNumberOf(retried), err)
	}
}

// Covers: AT-NO-136（真库）——服务端收下开立时取交接单单据号：设备声明的开立时刻是前一日，号按服务端此刻、开立网点的业务时区出；
// 标识仍是设备签的那一个；同一开立重投答原号、不再取号；并发重投同一张只开一张、只取一个号，并发开八张各得一号、连成一段；当日号段
// 被占满时答编号规则撞号、交接单不落、序列回滚。
func TestHandoverNoteDocumentNumbersOnARealDatabase(t *testing.T) {
	rig := newDocumentNumberRig(t)
	const tenant = "SYN-TENANT-DN-1"
	rig.registerNode(t, tenant, "SYN-NODE-SHA-HUB", "Asia/Shanghai")
	rig.clock.set(time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC))
	offlineOpenedAt := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

	opened, err := rig.open(t, tenant, "SYN-NODE-SHA-HUB", "SYN-HANDOVER-DN-01", offlineOpenedAt)
	if err != nil || opened.Outcome() != nodeopsapp.DeliveryHandoverNoteOpened || noteNumberOf(opened) != "HO-20261103-00001" ||
		opened.Note().ID().String() != "SYN-HANDOVER-DN-01" || !opened.Note().OpenedAt().Equal(offlineOpenedAt) {
		t.Fatalf("开立 = %s %q %v", opened.Outcome(), noteNumberOf(opened), err)
	}
	replay, err := rig.open(t, tenant, "SYN-NODE-SHA-HUB", "SYN-HANDOVER-DN-01", offlineOpenedAt)
	if err != nil || replay.Outcome() != nodeopsapp.DeliveryHandoverExistingResult || noteNumberOf(replay) != "HO-20261103-00001" {
		t.Fatalf("原样重投 = %s %q %v", replay.Outcome(), noteNumberOf(replay), err)
	}

	var wait sync.WaitGroup
	outcomes := make([]nodeopsapp.DeliveryHandoverOutcome, 5)
	for index := range outcomes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if result, err := rig.open(t, tenant, "SYN-NODE-SHA-HUB", "SYN-HANDOVER-DN-SAME", offlineOpenedAt); err == nil {
				outcomes[index] = result.Outcome()
			}
		}()
	}
	numbers := make([]string, 8)
	for index := range numbers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if result, err := rig.open(t, tenant, "SYN-NODE-SHA-HUB", fmt.Sprintf("SYN-HANDOVER-DN-PAR-%02d", index), offlineOpenedAt); err == nil {
				numbers[index] = noteNumberOf(result)
			}
		}()
	}
	wait.Wait()
	openedOnce, replayedRest := 0, 0
	for _, outcome := range outcomes {
		switch outcome {
		case nodeopsapp.DeliveryHandoverNoteOpened:
			openedOnce++
		case nodeopsapp.DeliveryHandoverExistingResult:
			replayedRest++
		}
	}
	if openedOnce != 1 || replayedRest != len(outcomes)-1 {
		t.Fatalf("同一张并发重投的答案 = %v，want 一张开立、其余已有结果", outcomes)
	}
	slices.Sort(numbers)
	if len(slices.Compact(slices.Clone(numbers))) != len(numbers) || numbers[0] == "" {
		t.Fatalf("并发开立的号 = %v，want 各得一号、互不相同", numbers)
	}
	if taken := rig.sequenceValue(t, tenant, "DELIVERY_HANDOVER_NOTE", "20261103"); taken != 10 {
		t.Fatalf("一张、同一张并发五次、另八张之后序列取到 %d，want 10", taken)
	}

	rig.clock.set(time.Date(2026, 11, 9, 17, 0, 0, 0, time.UTC))
	for index := 1; index <= docnumber.MaxSkippedNumbers; index++ {
		if _, err := rig.pool.Exec(context.Background(),
			`INSERT INTO node_operations.delivery_handover_note
				(tenant_id, note_id, node_ref, delivering_party, claimed_account_ids, opened_at, document_number)
			 VALUES ($1, $2, 'SYN-NODE-SHA-HUB', 'SYN-DRIVER-OLD', ARRAY['SYN-ACCOUNT-DN-01'], now(), $3)`,
			tenant, fmt.Sprintf("SYN-HANDOVER-OLD-%02d", index), fmt.Sprintf("HO-20261110-%05d", index),
		); err != nil {
			t.Fatal(err)
		}
	}
	collided, err := rig.open(t, tenant, "SYN-NODE-SHA-HUB", "SYN-HANDOVER-DN-CLASH", offlineOpenedAt)
	if err != nil || collided.Outcome() != nodeopsapp.DeliveryHandoverNumberingRuleCollision || collided.Note().ID().String() != "" {
		t.Fatalf("撞号 = %s %v", collided.Outcome(), err)
	}
	if taken := rig.sequenceValue(t, tenant, "DELIVERY_HANDOVER_NOTE", "20261110"); taken != 0 {
		t.Fatalf("撞号回滚之后序列上还留着 %d", taken)
	}
	var landed bool
	if err := rig.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM node_operations.delivery_handover_note WHERE tenant_id = $1 AND note_id = 'SYN-HANDOVER-DN-CLASH')`, tenant,
	).Scan(&landed); err != nil || landed {
		t.Fatalf("撞号也落了交接单：%v %v", landed, err)
	}
}
