package nrinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证第二个消费方向的消费门四条。与接受决定那条各写各的：
// 两个消费者的 inbox 键前缀不同（消费者名不同），账本天然分家，因此同一份投递在两条
// 线上各处理一次不是重复处理。

type networkIntakeHandlerDouble struct {
	calls []nrinbox.AdoptedNetworkIntake
	err   error
}

func (double *networkIntakeHandlerDouble) HandleAdoptedNetworkIntake(
	_ context.Context,
	intake nrinbox.AdoptedNetworkIntake,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, intake)
	return nil
}

func newNetworkIntakeFixture(t *testing.T) (*nrinbox.NetworkIntakeConsumer, *networkIntakeHandlerDouble) {
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
	handler := &networkIntakeHandlerDouble{}
	consumer, err := nrinbox.NewNetworkIntakeConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func networkIntakeEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"parcel":   "parcel-1",
		"kind":     "NODE_INTAKE",
		"version":  "intake-result/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         nrinbox.AdoptedNetworkIntakeEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1",
		PartitionKey: "tenant-a/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

// TestANetworkIntakeDeliveryIsProcessedExactlyOnce 证恰一次处理：首投处理并入账，
// 重复投递幂等跳过——处理方不会被调第二次。
func TestANetworkIntakeDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newNetworkIntakeFixture(t)
	ctx := t.Context()
	envelope := networkIntakeEnvelope(t, "adoption-1")

	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("首投消费：%v", err)
	}
	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}

	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	// 四维要一路带到处理方：处理方按它们整行取回采用结果，少一维就取不着那一行，
	// 而复核的业务输入是采用记录本体而不是这四个字符串。
	got := handler.calls[0]
	if got.TenantID != "tenant-a" || got.Parcel != "parcel-1" ||
		got.Kind != "NODE_INTAKE" || got.Version != "intake-result/v1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// TestANetworkIntakeEnvelopeMissingAnyKeyDimensionIsPoison 证采用键四维缺一即毒丸：
// 处理方按整键取回采用结果，缺任一维都取不着——重投同样内容不会长出字段来。
//
// 尤其不能给 kind 或 version 留缺省：来源类型决定控制依据译成哪一格（节点收寄 vs 权威
// 运输交接），来源版本进触发指纹，猜任一个都会让复核落在另一份事实上。
func TestANetworkIntakeEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"parcel":"parcel-1","kind":"NODE_INTAKE","version":"intake-result/v1"}`,
		"缺 parcel":   `{"tenantId":"tenant-a","kind":"NODE_INTAKE","version":"intake-result/v1"}`,
		"缺 kind":     `{"tenantId":"tenant-a","parcel":"parcel-1","version":"intake-result/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","parcel":"parcel-1","kind":"NODE_INTAKE"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newNetworkIntakeFixture(t)

			poison := networkIntakeEnvelope(t, "adoption-"+name)
			poison.Payload = json.RawMessage(payload)

			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0——取不回采用记录的信封不该到达处理方", len(handler.calls))
			}
		})
	}
}

// TestAFailedNetworkIntakeHandlerRollsBackAndTheRedeliveryRetries 证处理失败整体
// 回滚：inbox 无痕，重投可以再试并成功——失败不吃掉投递。复核的未决就走这一格。
func TestAFailedNetworkIntakeHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newNetworkIntakeFixture(t)
	ctx := t.Context()
	envelope := networkIntakeEnvelope(t, "adoption-1")

	handler.err = errors.New("reassessment undecided")
	if err := consumer.Consume(ctx, envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——静默吞掉等于丢投递")
	}

	handler.err = nil
	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("重投消费：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1——失败那次不算处理", len(handler.calls))
	}
}

// TestAPoisonNetworkIntakeEnvelopeIsRejectedOnceAndStaysRejected 证毒丸显式拒收：
// 拒收也是账，重投不再处理也不再报错。
func TestAPoisonNetworkIntakeEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newNetworkIntakeFixture(t)
	ctx := t.Context()

	poison := networkIntakeEnvelope(t, "adoption-poison")
	poison.Payload = json.RawMessage(`{"tenantId":""}`)

	if err := consumer.Consume(ctx, poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
	}
	if err := consumer.Consume(ctx, poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0——毒丸不该到达处理方", len(handler.calls))
	}
}

// TestAForeignTypeIsLoudOnTheNetworkIntakeConsumer 证认不得的类型响亮报错不入账。
// 这里特意拿另一个真实存在的类型（接受决定）：路由表把两类都投给本进程，订阅面配置
// 串了线时错的不是内容而是收件人，拒收会把接受决定记进复核的账。
func TestAForeignTypeIsLoudOnTheNetworkIntakeConsumer(t *testing.T) {
	consumer, handler := newNetworkIntakeFixture(t)
	ctx := t.Context()

	foreign := networkIntakeEnvelope(t, "adoption-foreign")
	foreign.Type = nrinbox.AcceptedDecisionEventType

	if err := consumer.Consume(ctx, foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
