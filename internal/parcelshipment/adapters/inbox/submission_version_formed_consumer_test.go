package psinbox_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「新提交版本已形成 → 接受判断链再驱一拍」这扇续办门自己的
// 消费纪律（ADR-0106 Decision 三）。译码与折法与提交门、复核续办门共用同一份包级函数，逐格
// 取证不在此重列；这里钉的是本门自己的三件事：自己的类型认领、自己的 inbox 账本名下译出同一个
// 命令且**签回的是新版本**、发布侧类型串。
//
// 「等待受控补充按处理完毕入账」那格在这里证不了（结果只有编排造得出），由 cmd/parcel-dispatch
// 的 customer_supplement_resume_loop_test 按真链取证。

func newSupplementResumeFixture(t *testing.T) (*psinbox.SubmissionVersionFormedConsumer, *chainAdvancerDouble) {
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
	advancer := &chainAdvancerDouble{err: errAdvancerProbe}
	consumer, err := psinbox.NewSubmissionVersionFormedConsumer(db.Transactor(), store, advancer)
	if err != nil {
		t.Fatalf("构造受控补充续办门：%v", err)
	}
	return consumer, advancer
}

// supplementResumePayload 是发布侧 OutboxSubmissionVersionFormedHandoff 实际写出的形状：与
// 「复核已完成」同名同义，提交版本那维填的是**新**版本——链要拿新版本的任务重跑。
const supplementResumePayload = `{
	"tenantId": "tenant-a",
	"customerAccountId": "customer-1",
	"source": "source-a",
	"sourceRequestKey": "key-1",
	"shipmentRequestId": "request-1",
	"submissionVersionId": "version-2",
	"declaredParcelIds": ["parcel-1", "parcel-2"]
}`

func supplementResumeEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	now := time.Date(2026, 9, 4, 19, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.SubmissionVersionFormedEventType,
		Version:      1,
		Scope:        "tenant-a/customer-1",
		Subject:      "request-1",
		PartitionKey: "tenant-a/customer-1/request-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(supplementResumePayload),
	}
}

// Covers: 续办信封译进**同一个**推进命令，且提交版本是新版本——续办拿旧版本重跑正是 ADR-0106
// Context 第 3 条说的「白烧或落到换代原因」，链必须按新版本的任务走。
func TestASupplementResumeEnvelopeTranslatesIntoTheChainCommandOnTheNewVersion(t *testing.T) {
	consumer, advancer := newSupplementResumeFixture(t)

	if err := consumer.Consume(t.Context(), supplementResumeEnvelope(t, "supplement-resume-1")); err == nil {
		t.Fatal("探针错误必须让消费报错——业务缺件不落毒丸账")
	}
	if len(advancer.commands) != 1 {
		t.Fatalf("推进次数 = %d, want 1", len(advancer.commands))
	}

	command := advancer.commands[0]
	identity := command.Identity
	if identity.TenantID().String() != "tenant-a" ||
		identity.CustomerAccountID().String() != "customer-1" ||
		identity.Source().String() != "source-a" ||
		identity.RequestKey().String() != "key-1" {
		t.Fatalf("来源身份 = %+v", identity)
	}
	if command.ShipmentRequestID.String() != "request-1" {
		t.Fatalf("委托 = %q", command.ShipmentRequestID)
	}
	if command.SubmissionVersion.String() != "version-2" {
		t.Fatalf("提交版本 = %q, want 新版本 version-2", command.SubmissionVersion)
	}
	if len(command.DeclaredParcelIDs) != 2 {
		t.Fatalf("声明成员 = %v", command.DeclaredParcelIDs)
	}
}

// Covers: 毒丸一次判定在本门下成立（同一份译码，留一格烟测防接错）。
func TestAPoisonSupplementResumeEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, advancer := newSupplementResumeFixture(t)
	poison := supplementResumeEnvelope(t, "supplement-resume-poison")
	poison.Payload = json.RawMessage(`{"tenantId":""}`)

	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账：%v", err)
	}
	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(advancer.commands) != 0 {
		t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
	}
}

// Covers: 装配错误要响亮——几扇门转交同一条链，最容易的错接是把复核续办信封投进本门。
func TestAForeignTypeIsLoudOnTheSupplementResumeGate(t *testing.T) {
	consumer, advancer := newSupplementResumeFixture(t)
	foreign := supplementResumeEnvelope(t, "supplement-resume-foreign")
	foreign.Type = psinbox.ManualReviewCompletedEventType

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(advancer.commands) != 0 {
		t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
	}
}

// Covers: 消费方与发布方各写各的事件类型串；这里钉消费侧那一串，发布侧由 postgres 交接适配器
// 的用例钉，两处对不上时至少有一处红。词取 ADR-0045「新提交版本」。
func TestTheSupplementResumeGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "parcel-shipment.shipment-request.submission-version-formed"
	if got := string(psinbox.SubmissionVersionFormedEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	if !strings.HasPrefix(published, "parcel-shipment.") {
		t.Fatalf("%q 不在本上下文的事件家族里", published)
	}
}
