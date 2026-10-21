package veinbox_test

import (
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证交接信封上补派生那一路的消费门（ADR-0224）：与交接投影同一封信、各记各的账，恰一次，毒丸照拒。

// Covers: AT-VE-178 消费门半边——同一封交接信封，投影与补派生两路各处理一次；补派生那一路重投按 inbox 跳过。两路共一本账时，
// 先到的一路会把另一路当重复跳过，补派生就永远不跑。
func TestTheBackfillLaneKeepsItsOwnLedgerOnTheHandoverEnvelope(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	projectionHandler := &veRegisteredHandoverHandlerDouble{}
	projection, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, projectionHandler)
	if err != nil {
		t.Fatalf("构造交接投影消费者：%v", err)
	}
	backfillHandler := &veRegisteredHandoverHandlerDouble{}
	backfill, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), store, backfillHandler)
	if err != nil {
		t.Fatalf("构造补派生消费者：%v", err)
	}

	envelope := veTransportHandoverEnvelope(t, "tenant-a/parcel-1/scope-1/handover-result/v1")
	if err := projection.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("投影投递：%v", err)
	}
	for range 2 {
		if err := backfill.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("补派生投递：%v", err)
		}
	}
	if len(projectionHandler.calls) != 1 || len(backfillHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：投影 = %d，补派生 = %d", len(projectionHandler.calls), len(backfillHandler.calls))
	}
	if got := backfillHandler.calls[0]; got.TenantID != "tenant-a" || got.Object != "parcel-1" ||
		got.Scope != "scope-1" || got.Version != "handover-result/v1" {
		t.Fatalf("补派生那一路译码 = %+v，want 与投影同四维", got)
	}
}

// Covers: AT-VE-181 消费门半边——缺键维的交接信封在补派生那一路同样是毒丸：拒收入账、交回 nil、不调处理方。
func TestAPoisonHandoverEnvelopeIsRejectedOnTheBackfillLane(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &veRegisteredHandoverHandlerDouble{}
	consumer, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造补派生消费者：%v", err)
	}
	poison := veTransportHandoverEnvelope(t, "poison-1")
	poison.Payload = []byte(`{"tenantId":"tenant-a","object":"parcel-1","scope":"scope-1"}`)
	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸应拒收入账：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("毒丸调了处理方 %d 次", len(handler.calls))
	}
}

func TestTheBackfillConsumerRefusesMissingDependencies(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	if _, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(nil, store, &veRegisteredHandoverHandlerDouble{}); err == nil {
		t.Fatal("缺事务器构造成功了")
	}
	if _, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), nil, &veRegisteredHandoverHandlerDouble{}); err == nil {
		t.Fatal("缺 inbox 构造成功了")
	}
	if _, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("缺处理方构造成功了")
	}
}
