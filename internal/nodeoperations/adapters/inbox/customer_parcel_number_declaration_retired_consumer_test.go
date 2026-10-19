package noinbox_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证重核消费者的声明撤除那一扇门（ADR-0211 决定三，票 no-reception-identity/07）：PS 的声明撤除门铃投到
// 这里，恰处理一次；载荷缺租户或号码即毒丸；与关联已形成那一扇门同用一个消费者名，两种门铃各记各的账。夹具全为合成（S 级）。

type retiredDeclarationHandlerDouble struct {
	calls []noinbox.RetiredCustomerParcelNumberDeclaration
}

func (double *retiredDeclarationHandlerDouble) HandleRetiredCustomerParcelNumberDeclaration(
	_ context.Context, retired noinbox.RetiredCustomerParcelNumberDeclaration,
) error {
	double.calls = append(double.calls, retired)
	return nil
}

func retiredDeclarationEnvelope(t *testing.T, eventID string, payload map[string]string) eventing.Envelope {
	t.Helper()
	envelope := formedRelationEnvelope(t, eventID, payload)
	envelope.Type = noinbox.CustomerParcelNumberDeclarationRetiredEventType
	return envelope
}

// Covers: 一封声明撤除门铃恰处理一次，号码原样转交；缺租户或号码即毒丸，拒收入账、不转交、不报错。同一个事件 ID 的关联已形成
// 门铃另记一笔账：两扇门同用一个消费者名，inbox 键靠事件 ID 分开，PS 的两只口 ID 各带口名前缀。
func TestARetiredDeclarationDoorbellIsHandledExactlyOnce(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &retiredDeclarationHandlerDouble{}
	consumer, err := noinbox.NewCustomerParcelNumberDeclarationRetiredConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费门：%v", err)
	}
	envelope := retiredDeclarationEnvelope(t, "retired-1", syntheticFormedRelation)
	for range 2 {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递：%v", err)
		}
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "SYN-TENANT-NRS03" || got.Number != " SYN-MARK-NRS03\t" {
		t.Fatalf("译码结果 = %+v, want 租户与号码原样", got)
	}

	for name, payload := range map[string]map[string]string{
		"缺 tenantId": {"number": "SYN-MARK-NRI07"},
		"缺 number":   {"tenantId": "SYN-TENANT-NRI07"},
	} {
		if err := consumer.Consume(t.Context(), retiredDeclarationEnvelope(t, "poison-"+name, payload)); err != nil {
			t.Fatalf("%s：毒丸应拒收入账而不是报错：%v", name, err)
		}
	}
	if len(handler.calls) != 1 {
		t.Fatalf("毒丸被转交了：处理次数 = %d", len(handler.calls))
	}

	formed := &formedRelationHandlerDouble{}
	formedGate, err := noinbox.NewExternalIdentifierRelationFormedConsumer(db.Transactor(), store, formed)
	if err != nil {
		t.Fatalf("构造关联已形成那一扇门：%v", err)
	}
	if err := formedGate.Consume(t.Context(), formedRelationEnvelope(t, "retired-1-formed", syntheticFormedRelation)); err != nil {
		t.Fatalf("关联已形成门铃：%v", err)
	}
	if len(formed.calls) != 1 {
		t.Fatalf("关联已形成门铃处理次数 = %d, want 1", len(formed.calls))
	}

	foreign := retiredDeclarationEnvelope(t, "foreign", syntheticFormedRelation)
	foreign.Type = noinbox.ExternalIdentifierRelationFormedEventType
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 1 {
		t.Fatalf("认不得的类型被转交了：处理次数 = %d", len(handler.calls))
	}
}
