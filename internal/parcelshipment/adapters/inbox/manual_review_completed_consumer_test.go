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

// 本文件对真实 PostgreSQL 16 证「复核已完成 → 接受判断链再驱一拍」这扇续办门自己的
// 消费纪律（ADR-0086 Decision 二）。译码与折法与提交门共用同一份包级函数，那半的逐格
// 取证不在此重列（毒丸分维、集合外结果、编排错误上抛见提交门的用例）；这里钉的是本门
// 自己的三件事：自己的类型认领、自己的 inbox 账本名下译出同一个命令、发布侧类型串。
//
// 「等待人工复核按处理完毕入账」那格在这里同样证不了：ManualReviewPending 的
// AdvanceAcceptanceChainResult 只有编排自己造得出。它由 cmd/parcel-dispatch 的
// manual_review_resume_loop_test 按真链取证。

func newResumeFixture(t *testing.T) (*psinbox.ManualReviewCompletedConsumer, *chainAdvancerDouble) {
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
	consumer, err := psinbox.NewManualReviewCompletedConsumer(db.Transactor(), store, advancer)
	if err != nil {
		t.Fatalf("构造续办门：%v", err)
	}
	return consumer, advancer
}

// resumePayload 是发布侧 OutboxManualReviewCompletedHandoff 实际写出的形状：与「委托
// 已提交」同名同义、少批次维。少批次不该让本门毒丸——译码只取链要的那几维。
const resumePayload = `{
	"tenantId": "tenant-a",
	"customerAccountId": "customer-1",
	"source": "source-a",
	"sourceRequestKey": "key-1",
	"shipmentRequestId": "request-1",
	"submissionVersionId": "version-1",
	"declaredParcelIds": ["parcel-1", "parcel-2"]
}`

func resumeEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	now := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.ManualReviewCompletedEventType,
		Version:      1,
		Scope:        "tenant-a/customer-1",
		Subject:      "request-1",
		PartitionKey: "tenant-a/customer-1/request-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(resumePayload),
	}
}

// Covers: 续办信封的载荷形状——四维来源身份、委托、复核针对的提交版本与声明成员都译进
// **同一个**推进命令（与提交门同一命令类型）。版本那维尤其要紧：续办签回的必须是复核
// 针对的那份版本，漂开会让链拿新版本的任务去接旧版本的复核。
func TestAResumeEnvelopeTranslatesIntoTheChainCommand(t *testing.T) {
	consumer, advancer := newResumeFixture(t)

	if err := consumer.Consume(t.Context(), resumeEnvelope(t, "resume-1")); err == nil {
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
	if command.SubmissionVersion.String() != "version-1" {
		t.Fatalf("提交版本 = %q", command.SubmissionVersion)
	}
	if len(command.DeclaredParcelIDs) != 2 ||
		command.DeclaredParcelIDs[0].String() != "parcel-1" ||
		command.DeclaredParcelIDs[1].String() != "parcel-2" {
		t.Fatalf("声明成员 = %v", command.DeclaredParcelIDs)
	}
}

// Covers: ADR-0049 消费门毒丸一次判定在本门下同样成立——缺维的续办信封拒收入账，重投
// 不再推链。分维逐格见提交门（同一份译码）；这里留一格烟测，防的是本门把译码接错。
func TestAPoisonResumeEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, advancer := newResumeFixture(t)
	poison := resumeEnvelope(t, "resume-poison")
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

// Covers: ADR-0049 装配错误要响亮——路由表把别的类型投到本门是接线错了，不是业务缺件。
// 两扇门转交同一条链，最容易的错接正是把提交信封投进续办门。
func TestAForeignTypeIsLoudOnTheResumeGate(t *testing.T) {
	consumer, advancer := newResumeFixture(t)
	foreign := resumeEnvelope(t, "resume-foreign")
	foreign.Type = psinbox.ShipmentRequestSubmittedEventType

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(advancer.commands) != 0 {
		t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
	}
}

// Covers: 消费方与发布方各写各的事件类型串——两串漂开时路由表会把发布侧那一封投成
// dispatch.no_subscriber。这里钉住消费侧认的那一串，发布侧由 cmd/parcel-api 的铸封
// 译码用例钉住，两处对不上时至少有一处红。
func TestTheResumeGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "parcel-shipment.shipment-request.manual-review-completed"
	if got := string(psinbox.ManualReviewCompletedEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	if !strings.HasPrefix(published, "parcel-shipment.") {
		t.Fatalf("%q 不在本上下文的事件家族里", published)
	}
}
