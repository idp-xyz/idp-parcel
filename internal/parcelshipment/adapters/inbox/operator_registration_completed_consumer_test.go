package psinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「参数已登记 → 按租户重驱接受判断链」这扇门自己的消费纪律
// （ADR-0094 Decision 四）。链的折法与另两扇门共用同一份包级函数，不在此重列；这里钉的是本门
// 自己的几件事：认自己的类型与发布侧的字面、译出租户去问队列、一封信对多份委托时**逐份各自
// 一笔事务、一份失败不拖累其余**、全部落定才入账、毒丸拒收一次。
//
// 「落定即入账、停在会自愈的依赖上即回滚重投、已决的不再被驱」那几格要真链才走得到——
// AdvanceAcceptanceChainResult 只有编排自己造得出——由 cmd/parcel-dispatch 的
// operator_registration_resume_loop_test 按真链取证。

// queueDouble 是 ports.OperatorRegistrationQueue 的替身：按调用次数逐页交回预置名单，并记下
// 每次要的租户与页大小。
type queueDouble struct {
	pages   [][]ports.OperatorRegistrationQueueRecord
	calls   int
	tenants []string
	limits  []int
}

func (double *queueDouble) ListWaitingOnOperatorRegistration(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.OperatorRegistrationQueueRecord, error) {
	double.tenants = append(double.tenants, tenant.String())
	double.limits = append(double.limits, limit)
	index := double.calls
	double.calls++
	if index >= len(double.pages) {
		return nil, nil
	}
	return double.pages[index], nil
}

// perRequestAdvancer 按委托标识决定推进那一步答什么：nil 项交回未决哨兵（链自己在停在会自愈
// 的依赖上时就是这么答的），其余交回预置错误。它记下被驱的顺序。
type perRequestAdvancer struct {
	answers map[string]error
	driven  []string
}

func (double *perRequestAdvancer) Handle(
	_ context.Context,
	command psapplication.AdvanceAcceptanceChainCommand,
) (psapplication.AdvanceAcceptanceChainResult, error) {
	id := command.ShipmentRequestID.String()
	double.driven = append(double.driven, id)
	if err, known := double.answers[id]; known && err != nil {
		return psapplication.AdvanceAcceptanceChainResult{}, err
	}
	return psapplication.AdvanceAcceptanceChainResult{}, psinbox.ErrAcceptanceChainUndecided
}

func waitingRecord(t *testing.T, requestID string) ports.OperatorRegistrationQueueRecord {
	t.Helper()
	tenant := mustDomain(t, domain.NewTenantID, "tenant-a")
	account := mustDomain(t, domain.NewCustomerAccountID, "customer-1")
	source := mustDomain(t, domain.NewSource, "source-a")
	key := mustDomain(t, domain.NewSourceRequestKey, "key-"+requestID)
	identity, err := domain.NewSourceIdentity(tenant, account, source, key)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return ports.OperatorRegistrationQueueRecord{
		Identity:          identity,
		ShipmentRequestID: mustDomain(t, domain.NewShipmentRequestID, requestID),
		SubmissionVersion: mustDomain(t, domain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustDomain(t, domain.NewDeclaredParcelID, "parcel-1")},
		SubmittedAt:       time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC),
	}
}

func mustDomain[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func newRegistrationFixture(
	t *testing.T,
	queue *queueDouble,
	advancer *perRequestAdvancer,
	pageSize int,
) (*psinbox.OperatorRegistrationCompletedConsumer, *bentopg.DB) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := psinbox.NewOperatorRegistrationCompletedConsumer(db.Transactor(), store, queue, advancer, pageSize)
	if err != nil {
		t.Fatalf("构造登记续办门：%v", err)
	}
	return consumer, db
}

// registrationPayload 是发布侧 party-commercial 的 OutboxOperatorRegistrationCompletedHandoff 实际
// 写出的形状：只有两键。
const registrationPayload = `{"tenantId": "tenant-a", "registrationKind": "AS_OF_POLICY"}`

func registrationEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	now := time.Date(2026, 9, 4, 9, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/party-commercial",
		Type:         psinbox.OperatorRegistrationCompletedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "ACCEPTANCE_RULE_PACKAGE/rules-1@v1",
		PartitionKey: "tenant-a",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(registrationPayload),
	}
}

func countInboxRows(t *testing.T, db *bentopg.DB, consumer, eventID string) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM bento.inbox WHERE consumer = $1 AND event_id = $2`,
		consumer, eventID).Scan(&count); err != nil {
		t.Fatalf("数 inbox：%v", err)
	}
	return count
}

// Covers: 一封信译出租户去问队列（页大小照构造参数），队列上的每一份都被驱到，命令四维
// 逐字来自队列记录；全部停在会自愈的依赖上时整封交回未决哨兵（账本无痕，等重投），且
// **第一份停住不挡第二份被驱**。
func TestARegistrationEnvelopeRedrivesEveryWaitingRequestIndependently(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, db := newRegistrationFixture(t, queue, advancer, 50)

	envelope := registrationEnvelope(t, "registration-1")
	err := consumer.Consume(t.Context(), envelope)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("两份都停在会自愈的依赖上，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if !strings.Contains(err.Error(), "2 of 2") {
		t.Fatalf("未决报文要带计数：%v", err)
	}
	if len(queue.tenants) == 0 || queue.tenants[0] != "tenant-a" || queue.limits[0] != 50 {
		t.Fatalf("队列问的是 %v / %v，要 tenant-a / 50", queue.tenants, queue.limits)
	}
	if len(advancer.driven) != 2 || advancer.driven[0] != "request-1" || advancer.driven[1] != "request-2" {
		t.Fatalf("被驱的委托 = %v，要按队列顺序两份都到", advancer.driven)
	}
	if n := countInboxRows(t, db, "parcel-shipment/advance-acceptance-chain-on-operator-registration", "registration-1"); n != 0 {
		t.Fatalf("未决必须让账本无痕，实际 inbox 行数 = %d", n)
	}
}

// Covers: 硬错误（集合外结果、装配缺件）不折进未决，也不挡其余委托：第一份硬错，第二份照驱；
// 整封以硬错误上抛（不是未决哨兵），账本无痕。
func TestAHardFailureOnOneRequestIsLoudAndDoesNotStopTheOthers(t *testing.T) {
	probe := errors.New("probe: assembly is missing a step")
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{"request-1": probe}}
	consumer, db := newRegistrationFixture(t, queue, advancer, 50)

	err := consumer.Consume(t.Context(), registrationEnvelope(t, "registration-2"))
	if !errors.Is(err, probe) {
		t.Fatalf("硬错误要原样上抛，实际 err = %v", err)
	}
	if errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatal("硬错误挂上了未决哨兵——会被登记成「等依赖」而重投到预算耗尽")
	}
	if len(advancer.driven) != 2 {
		t.Fatalf("第一份硬错后第二份也要被驱，实际被驱 %v", advancer.driven)
	}
	if n := countInboxRows(t, db, "parcel-shipment/advance-acceptance-chain-on-operator-registration", "registration-2"); n != 0 {
		t.Fatalf("硬错误必须让账本无痕，实际 inbox 行数 = %d", n)
	}
}

// Covers: 队列为空时本封处理完毕入账——没有谁在等，这封信的工作就是零；重投由账本跳过，
// 队列不再被问。翻页终止：只有停住的旧面孔的一页不再翻。
func TestAnEmptyQueueSettlesTheEnvelopeAndReplayIsSkipped(t *testing.T) {
	queue := &queueDouble{}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, db := newRegistrationFixture(t, queue, advancer, 50)

	envelope := registrationEnvelope(t, "registration-3")
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("空队列应入账：%v", err)
	}
	if n := countInboxRows(t, db, "parcel-shipment/advance-acceptance-chain-on-operator-registration", "registration-3"); n != 1 {
		t.Fatalf("inbox 行数 = %d，要 1", n)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投已入账的投递：%v", err)
	}
	if queue.calls != 1 {
		t.Fatalf("重投后队列被问了 %d 次，要仍为 1——账本跳过", queue.calls)
	}
}

// Covers: 翻页——第一页满且都是新面孔就再取一页；第二页只剩第一页停住的旧面孔时到底，不再
// 无限翻。三份各被驱恰好一次。
func TestPagingStopsWhenAPageBringsNoNewRequests(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-3")},
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-3")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, _ := newRegistrationFixture(t, queue, advancer, 2)

	err := consumer.Consume(t.Context(), registrationEnvelope(t, "registration-4"))
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("三份都停住应交回未决哨兵，实际 err = %v", err)
	}
	if queue.calls != 3 {
		t.Fatalf("翻页次数 = %d，要 3（第三页无新面孔即止）", queue.calls)
	}
	if len(advancer.driven) != 3 || advancer.driven[0] != "request-1" || advancer.driven[1] != "request-2" || advancer.driven[2] != "request-3" {
		t.Fatalf("被驱的委托 = %v，要三份各一次", advancer.driven)
	}
}

// Covers: ADR-0049 毒丸一次判定——缺租户或缺登记种类的信封拒收入账，重投不问队列。
func TestAPoisonRegistrationEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{{waitingRecord(t, "request-1")}}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, db := newRegistrationFixture(t, queue, advancer, 50)

	for _, spec := range []struct{ id, payload string }{
		{"registration-poison-tenant", `{"tenantId":"","registrationKind":"AS_OF_POLICY"}`},
		{"registration-poison-kind", `{"tenantId":"tenant-a"}`},
	} {
		poison := registrationEnvelope(t, spec.id)
		poison.Payload = json.RawMessage(spec.payload)
		if err := consumer.Consume(t.Context(), poison); err != nil {
			t.Fatalf("%s 首投应拒收入账：%v", spec.id, err)
		}
		if err := consumer.Consume(t.Context(), poison); err != nil {
			t.Fatalf("%s 重投：%v", spec.id, err)
		}
		if n := countInboxRows(t, db, "parcel-shipment/advance-acceptance-chain-on-operator-registration", spec.id); n != 1 {
			t.Fatalf("%s inbox 行数 = %d，要 1（拒收入账）", spec.id, n)
		}
	}
	if queue.calls != 0 || len(advancer.driven) != 0 {
		t.Fatalf("毒丸不该问队列也不该驱链：队列 %d 次，驱 %v", queue.calls, advancer.driven)
	}
}

// Covers: ADR-0049 装配错误要响亮——把别的类型投到本门是接线错了，不是业务缺件。
func TestAForeignTypeIsLoudOnTheRegistrationGate(t *testing.T) {
	queue := &queueDouble{}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, _ := newRegistrationFixture(t, queue, advancer, 50)

	foreign := registrationEnvelope(t, "registration-foreign")
	foreign.Type = psinbox.ManualReviewCompletedEventType
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if queue.calls != 0 {
		t.Fatalf("认不得的类型不该问队列，实际问了 %d 次", queue.calls)
	}
}

// Covers: 消费方与发布方各写各的事件类型串——钉住消费侧认的那一串是 party-commercial 那边写
// 的字面（契约见票 first-tenant-runway/07 Comments）；载荷两键由 cmd/parcel-dispatch 那条用
// 真 PC 适配器铸封的往返用例守。
func TestTheRegistrationGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "party-commercial.commercial-authority.operator-registration-completed"
	if got := string(psinbox.OperatorRegistrationCompletedEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	if !strings.HasPrefix(published, "party-commercial.") {
		t.Fatalf("%q 不在 party-commercial 的事件家族里——本门收的是跨上下文的信", published)
	}
}

// Covers: 页大小非正是装配缺陷，构造期就拒。
func TestARegistrationGateRefusesANonPositivePageSize(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	if _, err := psinbox.NewOperatorRegistrationCompletedConsumer(db.Transactor(), store, &queueDouble{}, &perRequestAdvancer{}, 0); err == nil {
		t.Fatal("页大小 0 应在构造期被拒")
	}
}
