package psinbox_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「释放拒收已处理 → 接受判断链再驱一拍」这扇续办门自己的消费纪律（ADR-0293 决定四）。译码与折法与提交门、复核续办门、
// 受控补充续办门共用同一份包级函数，逐格取证不在此重列；这里钉本门自己的几件事：自己的类型认领、译出同一个命令且**签回的是委托此刻的当前版本**
// （不是被拒收的那一版）、载荷里多出的两个说明字段不让译码失败、发布侧类型串。真链往返由 cmd/parcel-dispatch 的 control_release_refusal_resume_test 取证。

func newRefusalHandledResumeFixture(t *testing.T) (*psinbox.ControlReleaseRefusalHandledConsumer, *chainAdvancerDouble) {
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
	consumer, err := psinbox.NewControlReleaseRefusalHandledConsumer(db.Transactor(), store, advancer)
	if err != nil {
		t.Fatalf("构造释放拒收处理续办门：%v", err)
	}
	return consumer, advancer
}

// refusalHandledPayload 是发布侧 OutboxControlReleaseRefusalHandledHandoff 实际写出的形状：来源身份四维、委托、提交版本与成员清单与「复核已完成」同名同义，提交版本那维是委托
// 此刻的当前版本；handled 开头的两个字段说明因哪一笔占用的处理而发，本门不读。
const refusalHandledPayload = `{
	"tenantId": "tenant-a",
	"customerAccountId": "customer-1",
	"source": "source-a",
	"sourceRequestKey": "key-1",
	"shipmentRequestId": "request-1",
	"submissionVersionId": "version-2",
	"declaredParcelIds": ["parcel-1", "parcel-2"],
	"handledSubmissionVersionId": "version-1",
	"handledControlResultId": "request-1/version-1"
}`

func refusalHandledEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	now := time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.ControlReleaseRefusalHandledEventType,
		Version:      1,
		Scope:        "tenant-a/customer-1",
		Subject:      "request-1",
		PartitionKey: "tenant-a/customer-1/request-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(refusalHandledPayload),
	}
}

// Covers: 续办信封译进同一个推进命令，提交版本是委托此刻的当前版本（version-2），不是被拒收的那一版（version-1）——链只判当前版本，拿旧版本
// 驱会落到「版本已换代」白烧一拍。
func TestARefusalHandledEnvelopeTranslatesIntoTheChainCommandOnTheCurrentVersion(t *testing.T) {
	consumer, advancer := newRefusalHandledResumeFixture(t)

	if err := consumer.Consume(t.Context(), refusalHandledEnvelope(t, "refusal-handled-1")); err == nil {
		t.Fatal("探针错误必须让消费报错——业务缺件不落毒丸账")
	}
	if len(advancer.commands) != 1 {
		t.Fatalf("推进次数 = %d, want 1", len(advancer.commands))
	}
	command := advancer.commands[0]
	identity := command.Identity
	if identity.TenantID().String() != "tenant-a" || identity.CustomerAccountID().String() != "customer-1" ||
		identity.Source().String() != "source-a" || identity.RequestKey().String() != "key-1" {
		t.Fatalf("来源身份 = %+v", identity)
	}
	if command.ShipmentRequestID.String() != "request-1" || command.SubmissionVersion.String() != "version-2" || len(command.DeclaredParcelIDs) != 2 {
		t.Fatalf("命令 = 委托 %q 版本 %q 成员 %v, want request-1 / 当前版本 version-2 / 两个成员",
			command.ShipmentRequestID, command.SubmissionVersion, command.DeclaredParcelIDs)
	}
}

// Covers: 毒丸一次判定在本门下成立（同一份译码，留一格烟测防接错）。
func TestAPoisonRefusalHandledEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, advancer := newRefusalHandledResumeFixture(t)
	poison := refusalHandledEnvelope(t, "refusal-handled-poison")
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

// Covers: 装配错误要响亮——几扇门转交同一条链，最容易的错接是把别的续办信封投进本门。
func TestAForeignTypeIsLoudOnTheRefusalHandledGate(t *testing.T) {
	consumer, advancer := newRefusalHandledResumeFixture(t)
	for name, foreignType := range map[string]eventing.EventType{
		"复核已完成":    psinbox.ManualReviewCompletedEventType,
		"新提交版本已形成": psinbox.SubmissionVersionFormedEventType,
	} {
		foreign := refusalHandledEnvelope(t, "refusal-handled-foreign-"+name)
		foreign.Type = foreignType
		if err := consumer.Consume(t.Context(), foreign); err == nil {
			t.Errorf("%s：认不得的类型必须响亮报错", name)
		}
	}
	if len(advancer.commands) != 0 {
		t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
	}
}

// Covers: 消费方与发布方各写各的事件类型串；这里钉消费侧那一串，发布侧由 postgres 交接适配器的用例钉，两处对不上时至少有一处红。
func TestTheRefusalHandledGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "parcel-shipment.shipment-request.control-release-refusal-handled"
	if got := string(psinbox.ControlReleaseRefusalHandledEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
}
