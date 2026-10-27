package veinbox_test

import (
	"encoding/json"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证节点收寄形成信封上移动事实补派生的消费门（ADR-0299）：与同一封信上的投影、parcel-shipment 的来源采用各记各的
// 账，恰一次，毒丸照拒。信封与译码同投影那一路，载荷没有为补派生加任何字段。

// Covers: AT-VE-221 消费门半边——同一封节点收寄形成信封，VE 投影、补派生与 PS 来源采用三路各处理一次，补派生那一路重投按 inbox 跳过；
// 共一本账时先到的一路会把另一路当重复跳过，补派生就永远不跑。三路认的是同一份载荷（租户、来源标识）。
func TestTheBackfillLaneKeepsItsOwnLedgerOnTheNodeIntakeEnvelope(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	projectionHandler := &formedHandlerDouble{}
	projection, err := veinbox.NewNodeIntakeConsumer(db.Transactor(), store, projectionHandler)
	if err != nil {
		t.Fatalf("构造投影消费者：%v", err)
	}
	backfillHandler := &formedHandlerDouble{}
	backfill, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(db.Transactor(), store, backfillHandler)
	if err != nil {
		t.Fatalf("构造补派生消费者：%v", err)
	}
	psHandler := &psFormedHandlerDouble{}
	adoption, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), store, psHandler)
	if err != nil {
		t.Fatalf("构造 PS 采用消费者：%v", err)
	}

	envelope := nodeIntakeEnvelope(t, "tenant-a/source-1")
	if err := projection.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("投影投递：%v", err)
	}
	for range 2 {
		if err := backfill.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("补派生投递：%v", err)
		}
	}
	if err := adoption.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("采用投递：%v", err)
	}
	if len(projectionHandler.calls) != 1 || len(backfillHandler.calls) != 1 || len(psHandler.calls) != 1 {
		t.Fatalf("三本账应各处理一次：投影 = %d，补派生 = %d，采用 = %d",
			len(projectionHandler.calls), len(backfillHandler.calls), len(psHandler.calls))
	}
	if got := backfillHandler.calls[0]; got != projectionHandler.calls[0] || got.TenantID != "tenant-a" || got.SourceID != "source-1" {
		t.Fatalf("补派生那一路译码 = %+v，want 与投影同两维 %+v", got, projectionHandler.calls[0])
	}
}

// Covers: AT-VE-221 消费门半边——缺键维的节点收寄信封在补派生那一路同样是毒丸：拒收入账、交回 nil、不调处理方；认不得的类型响亮报错。
func TestAPoisonOrForeignEnvelopeIsRejectedOnTheNodeIntakeBackfillLane(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &formedHandlerDouble{}
	consumer, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造补派生消费者：%v", err)
	}
	for name, payload := range map[string]string{
		"缺 tenantId": `{"sourceId":"source-1"}`,
		"缺 sourceId": `{"tenantId":"tenant-a"}`,
	} {
		poison := nodeIntakeEnvelope(t, "poison-"+name)
		poison.Payload = json.RawMessage(payload)
		if err := consumer.Consume(t.Context(), poison); err != nil {
			t.Fatalf("%s：毒丸应拒收入账：%v", name, err)
		}
	}
	foreign := nodeIntakeEnvelope(t, "foreign")
	foreign.Type = "parcel-shipment.acceptance-decision.formed"
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("毒丸与外来类型调了处理方 %d 次", len(handler.calls))
	}
}

func TestTheNodeIntakeBackfillConsumerRefusesMissingDependencies(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	if _, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(nil, store, &formedHandlerDouble{}); err == nil {
		t.Fatal("缺事务器构造成功了")
	}
	if _, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(db.Transactor(), nil, &formedHandlerDouble{}); err == nil {
		t.Fatal("缺 inbox 构造成功了")
	}
	if _, err := veinbox.NewMovementFactBackfillOnNodeIntakeConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("缺处理方构造成功了")
	}
}
