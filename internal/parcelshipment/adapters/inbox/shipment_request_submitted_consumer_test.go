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
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「委托已提交 → 接受判断链」这一路的消费门与译码。
//
// 「形成决定即入账」那半在这里证不了：AdvanceAcceptanceChainResult 的取值只有编排自己
// 造得出，替身能交回的只有零值。那半由 cmd/parcel-dispatch 的装配用例按真编排证——那里
// 接的是生产依赖图，未决与形成决定都是真答出来的，比在这里塞一个能任意作答的替身更实。

// errAdvancerProbe 是探针错误：本文件多数用例只关心「译出了什么」与「哪些格根本不该走到
// 编排」，让编排一律失败可以把断言集中在译码上，不必先造出一份能走通的接受链。
var errAdvancerProbe = errors.New("advancer probe")

type chainAdvancerDouble struct {
	commands []psapplication.AdvanceAcceptanceChainCommand
	err      error
}

func (double *chainAdvancerDouble) Handle(
	_ context.Context,
	command psapplication.AdvanceAcceptanceChainCommand,
) (psapplication.AdvanceAcceptanceChainResult, error) {
	double.commands = append(double.commands, command)
	return psapplication.AdvanceAcceptanceChainResult{}, double.err
}

func newSubmittedFixture(t *testing.T) (*psinbox.ShipmentRequestSubmittedConsumer, *chainAdvancerDouble) {
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
	consumer, err := psinbox.NewShipmentRequestSubmittedConsumer(db.Transactor(), store, advancer)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, advancer
}

// submittedPayload 是发布侧 OutboxShipmentRequestSubmittedHandoff 实际写出的形状。
// submissionBatchId 留在这里而不是删掉：本路不译它，而「带着它也照样译得出」正是要证的
// ——载荷多一维不该让消费方毒丸。
const submittedPayload = `{
	"tenantId": "tenant-a",
	"customerAccountId": "customer-1",
	"source": "source-a",
	"sourceRequestKey": "key-1",
	"shipmentRequestId": "request-1",
	"submissionBatchId": "batch-1",
	"submissionVersionId": "version-1",
	"declaredParcelIds": ["parcel-1", "parcel-2"]
}`

func submittedEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	now := time.Date(2026, 8, 27, 9, 15, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.ShipmentRequestSubmittedEventType,
		Version:      1,
		Scope:        "tenant-a/customer-1",
		Subject:      "request-1",
		PartitionKey: "tenant-a/customer-1/request-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(submittedPayload),
	}
}

// Covers: 简报「事件信封基线」的载荷形状 — 来源身份四维、委托、当前提交版本与声明成员
// 都译进命令。漏一维不会有任何东西变红：接受链会拿一个零值标识去问权威，而权威只会答
// 「这个范围没有适用依据」，看起来与实例半边还没配置一模一样。
func TestASubmittedEnvelopeTranslatesIntoTheChainCommand(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)

	if err := consumer.Consume(t.Context(), submittedEnvelope(t, "tenant-a/customer-1/source-a/key-1")); err == nil {
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

// Covers: ADR-0049 消费门毒丸那一格 — 信封缺维或成员清单为空都是发布侧铸信封时就该齐的
// 东西，重投同一份内容不会长出字段来，因此拒收入账而不是回滚重投。
//
// 成员清单为空单列一格：`已提交`委托必有声明成员，空清单当「没有成员要判」会让接受链
// 直接走到形成决定，而那份委托根本不成立。
func TestASubmittedEnvelopeMissingAnyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"载荷不是 JSON":             `{`,
		"缺 tenantId":            `{"customerAccountId":"c","source":"s","sourceRequestKey":"k","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":["p"]}`,
		"缺 customerAccountId":   `{"tenantId":"t","source":"s","sourceRequestKey":"k","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":["p"]}`,
		"缺 source":              `{"tenantId":"t","customerAccountId":"c","sourceRequestKey":"k","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":["p"]}`,
		"缺 sourceRequestKey":    `{"tenantId":"t","customerAccountId":"c","source":"s","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":["p"]}`,
		"缺 shipmentRequestId":   `{"tenantId":"t","customerAccountId":"c","source":"s","sourceRequestKey":"k","submissionVersionId":"v","declaredParcelIds":["p"]}`,
		"缺 submissionVersionId": `{"tenantId":"t","customerAccountId":"c","source":"s","sourceRequestKey":"k","shipmentRequestId":"r","declaredParcelIds":["p"]}`,
		"成员清单为空":                `{"tenantId":"t","customerAccountId":"c","source":"s","sourceRequestKey":"k","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":[]}`,
		"成员里有一个空串":              `{"tenantId":"t","customerAccountId":"c","source":"s","sourceRequestKey":"k","shipmentRequestId":"r","submissionVersionId":"v","declaredParcelIds":["p",""]}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, advancer := newSubmittedFixture(t)
			poison := submittedEnvelope(t, "poison-"+name)
			poison.Payload = json.RawMessage(payload)

			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
			}
			if len(advancer.commands) != 0 {
				t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
			}
		})
	}
}

// Covers: ADR-0049 毒丸一次判定 — 拒收入账之后重投仍然不推进接受链。
func TestAPoisonSubmittedEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)
	poison := submittedEnvelope(t, "poison")
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

// Covers: ADR-0049 装配错误要响亮 — 路由表把别的类型投到本消费者是接线错了，不是业务缺件。
func TestAForeignTypeIsLoudOnTheSubmittedConsumer(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)
	foreign := submittedEnvelope(t, "foreign")
	foreign.Type = "parcel-shipment.acceptance-decision.formed"

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(advancer.commands) != 0 {
		t.Fatalf("推进次数 = %d, want 0", len(advancer.commands))
	}
}

// Covers: adoptconsume 那条同款纪律「静默入账等于替编排作判断」— 封闭集合以外的结果不
// 折成未决。折进去会让这一封一路重投到失败预算耗尽，而日志上看起来像「一直在等某个依赖」。
func TestAnOutOfSetChainOutcomeIsNotFoldedIntoUndecided(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)
	advancer.err = nil

	err := consumer.Consume(t.Context(), submittedEnvelope(t, "out-of-set"))
	if !errors.Is(err, psinbox.ErrUnexpectedAcceptanceChainOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedAcceptanceChainOutcome", err)
	}
	if errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatal("集合外的结果挂上了未决哨兵——它会被登记成「等依赖」而无休止重投")
	}
}

// Covers: UC-PS-001 结果语义契约 — 编排上抛的错误原样上抛，不挂未决哨兵。那是端口交回了
// 集合外的答复或装配缺件，重投改不了它；挂上哨兵，运维会去查一个并不存在的依赖。
func TestAnOrchestrationErrorIsNotDressedAsUndecided(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)
	advancer.err = psapplication.ErrAcceptanceChainNotAssembled

	err := consumer.Consume(t.Context(), submittedEnvelope(t, "unassembled"))
	if !errors.Is(err, psapplication.ErrAcceptanceChainNotAssembled) {
		t.Fatalf("err = %v, want ErrAcceptanceChainNotAssembled", err)
	}
	if errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatal("装配缺件被报成了未决——等多久也不会长出一个没接上的编排")
	}
}

// Covers: 消费门回滚重投那一格 — 处理失败让消费报错，重投时再走一遍完整译码与推进。
func TestAFailedAdvanceRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, advancer := newSubmittedFixture(t)
	envelope := submittedEnvelope(t, "tenant-a/customer-1/source-a/key-1")

	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——业务缺件不落毒丸账")
	}
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("重投仍失败时同样报错")
	}
	if len(advancer.commands) != 2 {
		t.Fatalf("推进次数 = %d, want 2——回滚之后重投没有重跑", len(advancer.commands))
	}
}

// Covers: 消费方与发布方各写各的事件类型串（本文件顶部注释）— 两串漂开时路由表会把发布
// 侧那一封投成 dispatch.no_subscriber。这里钉住消费侧认的那一串，发布侧由
// adapters/postgres 的交接用例钉住，两处对不上时至少有一处红。
func TestTheConsumerAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "parcel-shipment.shipment-request.submitted"
	if got := string(psinbox.ShipmentRequestSubmittedEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	if !strings.HasPrefix(published, "parcel-shipment.") {
		t.Fatalf("%q 不在本上下文的事件家族里", published)
	}
}
