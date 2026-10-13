package psinbox_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「结算账户已登记」那一份登记续办门自己的几件事（票 routing-first-cut/19）：认
// settlement-accounting 写的那一串类型、在自己的账本名下记账、不收 party-commercial 那一类。重驱的纪律与
// party-commercial 那一份同一份代码，逐份事务、翻页、毒丸那几格由 operator_registration_completed_consumer_test 守。

const settlementAccountResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-settlement-account-registration"

func newSettlementAccountGateFixture(
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
	consumer, err := psinbox.NewSettlementAccountRegisteredConsumer(db.Transactor(), store, queue, advancer, 50)
	if err != nil {
		t.Fatalf("构造结算账户续办门：%v", err)
	}
	return consumer, db
}

// settlementAccountEnvelope 是发布侧 settlement-accounting 的 OutboxSettlementAccountRegisteredHandoff 实际写出的
// 形状：载荷同为两键，种类是 SETTLEMENT_ACCOUNT。
func settlementAccountEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	envelope := registrationEnvelope(t, eventID)
	envelope.Source = "idp-parcel/settlement-accounting"
	envelope.Type = psinbox.SettlementAccountRegisteredEventType
	envelope.Subject = "SETTLEMENT_ACCOUNT/SYN-ACCT-1"
	envelope.Payload = json.RawMessage(`{"tenantId": "tenant-a", "registrationKind": "SETTLEMENT_ACCOUNT"}`)
	return envelope
}

// Covers: 消费方与发布方各写各的事件类型串——钉住本门认的那一串是 settlement-accounting 那边写的字面；载荷两键由
// cmd/parcel-dispatch 那条用真 SA 适配器铸封的往返用例守。
func TestTheSettlementAccountGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "settlement-accounting.settlement-account.registered"
	if got := string(psinbox.SettlementAccountRegisteredEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	if !strings.HasPrefix(published, "settlement-accounting.") {
		t.Fatalf("%q 不在 settlement-accounting 的事件家族里——本门收的是跨上下文的信", published)
	}
	if psinbox.SettlementAccountRegisteredEventType == psinbox.OperatorRegistrationCompletedEventType {
		t.Fatal("两个发布方共用一串类型——路由表分不开两扇门")
	}
}

// Covers: 一封信译出租户去问同一张`等待运营登记`队列，队列上的每一份都被驱到；账本记在本门自己的名下，
// party-commercial 那一份的名下无痕。
func TestASettlementAccountEnvelopeRedrivesTheTenantQueueUnderItsOwnLedgerName(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, _ := newSettlementAccountGateFixture(t, queue, advancer)

	err := consumer.Consume(t.Context(), settlementAccountEnvelope(t, "settlement-account-1"))
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("两份都停在会自愈的依赖上，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if len(queue.tenants) == 0 || queue.tenants[0] != "tenant-a" {
		t.Fatalf("队列问的是 %v，要 tenant-a", queue.tenants)
	}
	if len(advancer.driven) != 2 {
		t.Fatalf("被驱的委托 = %v，要两份都到", advancer.driven)
	}

	empty := &queueDouble{}
	settled, db := newSettlementAccountGateFixture(t, empty, &perRequestAdvancer{answers: map[string]error{}})
	if err := settled.Consume(t.Context(), settlementAccountEnvelope(t, "settlement-account-2")); err != nil {
		t.Fatalf("空队列应入账：%v", err)
	}
	if n := countInboxRows(t, db, settlementAccountResumeConsumer, "settlement-account-2"); n != 1 {
		t.Fatalf("本门名下 inbox 行数 = %d，要 1", n)
	}
	if n := countInboxRows(t, db, "parcel-shipment/advance-acceptance-chain-on-operator-registration", "settlement-account-2"); n != 0 {
		t.Fatalf("party-commercial 那一份的名下记了 %d 行——两扇门共了账本名", n)
	}
}

// Covers: 把 party-commercial 那一类投到本门是接线错了，不是业务缺件——响亮报错，不问队列。
func TestTheSettlementAccountGateIsLoudOnThePartyCommercialType(t *testing.T) {
	queue := &queueDouble{}
	consumer, _ := newSettlementAccountGateFixture(t, queue, &perRequestAdvancer{answers: map[string]error{}})

	if err := consumer.Consume(t.Context(), registrationEnvelope(t, "registration-on-settlement-gate")); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if queue.calls != 0 {
		t.Fatalf("认不得的类型不该问队列，实际问了 %d 次", queue.calls)
	}
}
