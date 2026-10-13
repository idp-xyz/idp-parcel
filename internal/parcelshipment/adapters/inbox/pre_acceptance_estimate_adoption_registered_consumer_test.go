package psinbox_test

import (
	"encoding/json"
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「受理前估价采用已登记」那一份登记续办门自己的几件事（票 product-strategy-boundary/25）：认
// settlement-accounting 写的那一串类型、在自己的账本名下记账、不收另两份门的类型。重驱纪律与另两份同一份代码。

const estimateAdoptionResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-pre-acceptance-estimate-adoption"

func newEstimateAdoptionGateFixture(
	t *testing.T,
	queue *queueDouble,
	advancer *perRequestAdvancer,
) (*psinbox.OperatorRegistrationCompletedConsumer, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := psinbox.NewPreAcceptanceEstimateAdoptionRegisteredConsumer(db.Transactor(), store, queue, advancer, 50)
	if err != nil {
		t.Fatalf("构造受理前估价采用续办门：%v", err)
	}
	return consumer, db
}

// estimateAdoptionEnvelope 是发布侧 OutboxPreAcceptanceEstimateAdoptionRegisteredHandoff 实际写出的形状：载荷两键，种类是
// PRE_ACCEPTANCE_ESTIMATE_ADOPTION。
func estimateAdoptionEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	envelope := registrationEnvelope(t, eventID)
	envelope.Source = "idp-parcel/settlement-accounting"
	envelope.Type = psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType
	envelope.Subject = "PRE_ACCEPTANCE_ESTIMATE_ADOPTION/SYN-ACCT-1"
	envelope.Payload = json.RawMessage(`{"tenantId": "tenant-a", "registrationKind": "PRE_ACCEPTANCE_ESTIMATE_ADOPTION"}`)
	return envelope
}

// Covers: 消费方与发布方各写各的事件类型串——钉住本门认的那一串是 settlement-accounting 那边写的字面，且与另两份门不共串。
func TestTheEstimateAdoptionGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "settlement-accounting.pre-acceptance-estimate-adoption.registered"
	if got := string(psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	for _, other := range []eventing.EventType{psinbox.OperatorRegistrationCompletedEventType, psinbox.SettlementAccountRegisteredEventType} {
		if psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType == other {
			t.Fatalf("与 %q 共用一串类型——路由表分不开几扇门", other)
		}
	}
}

// Covers: 一封信译出租户去问同一张`等待运营登记`队列，队列上的每一份都被驱到；账本记在本门自己的名下，另两份的名下无痕；
// 别的门的类型投到这里响亮报错、不问队列。
func TestAnEstimateAdoptionEnvelopeRedrivesTheTenantQueueUnderItsOwnLedgerName(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, _ := newEstimateAdoptionGateFixture(t, queue, advancer)
	err := consumer.Consume(t.Context(), estimateAdoptionEnvelope(t, "estimate-adoption-1"))
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("两份都停在会自愈的依赖上，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if len(queue.tenants) == 0 || queue.tenants[0] != "tenant-a" || len(advancer.driven) != 2 {
		t.Fatalf("队列问的是 %v、被驱的是 %v，要 tenant-a 且两份都到", queue.tenants, advancer.driven)
	}

	settled, db := newEstimateAdoptionGateFixture(t, &queueDouble{}, &perRequestAdvancer{answers: map[string]error{}})
	if err := settled.Consume(t.Context(), estimateAdoptionEnvelope(t, "estimate-adoption-2")); err != nil {
		t.Fatalf("空队列应入账：%v", err)
	}
	if n := countInboxRows(t, db, estimateAdoptionResumeConsumer, "estimate-adoption-2"); n != 1 {
		t.Fatalf("本门名下 inbox 行数 = %d，要 1", n)
	}
	for _, other := range []string{
		"parcel-shipment/advance-acceptance-chain-on-operator-registration",
		"parcel-shipment/advance-acceptance-chain-on-settlement-account-registration",
	} {
		if n := countInboxRows(t, db, other, "estimate-adoption-2"); n != 0 {
			t.Fatalf("%s 名下记了 %d 行——几扇门共了账本名", other, n)
		}
	}

	untouched := &queueDouble{}
	loud, _ := newEstimateAdoptionGateFixture(t, untouched, &perRequestAdvancer{answers: map[string]error{}})
	if err := loud.Consume(t.Context(), settlementAccountEnvelope(t, "settlement-account-on-estimate-gate")); err == nil || untouched.calls != 0 {
		t.Fatalf("别的门的类型 err = %v、问了队列 %d 次，要响亮报错且不问", err, untouched.calls)
	}
}
