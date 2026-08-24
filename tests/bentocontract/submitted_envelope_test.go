package bentocontract

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
)

// 本文件取 `PBC-05` 今天可取证的三面：`委托已提交`基线信封过框架 v1 校验、Payload
// 最小化（字段集恰为委托/批次/版本/声明包裹标识，无地址、联系人、货物或申报明文）、
// 同委托分区顺序与 At-Least-Once 重投经真实 Store 成立。
//
// 边界（如实记录）：生产发射器不存在——`idp.parcel.shipment-request.submitted` 在
// internal/ 下零出现，事件仍未被任何生产编排发出。本文件证的是简报「事件信封基线」
// 所登记形状的合同符合性；「生产代码所发信封符合合同」那半要等发射器落地后按同一
// 夹具复证，本文件不宣称它已成立。

// TestSubmittedEnvelopePassesValidationWithMinimalPayload 证基线信封过 Validate 且
// Payload 字段集**恰好**是四个标识字段——恰好等于既禁多也禁少：多出的任何键都可能
// 携带明文，少掉的键会让下游拿不到必要关联。
func TestSubmittedEnvelopePassesValidationWithMinimalPayload(t *testing.T) {
	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}
	request := submittedContractRequest(t, scope, "env-key-1", "env-request-1")
	envelope := submittedEnvelopeFixture(t, "env-event-1", scope, request)

	if err := envelope.Validate(); err != nil {
		t.Fatalf("基线信封没过 v1 校验：%v", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("解 Payload：%v", err)
	}
	expected := []string{"shipmentRequestId", "submissionBatchId", "submissionVersionId", "declaredParcelIds"}
	if len(payload) != len(expected) {
		t.Fatalf("Payload 有 %d 个键，基线登记 %d 个：%v", len(payload), len(expected), payload)
	}
	for _, key := range expected {
		if _, present := payload[key]; !present {
			t.Errorf("Payload 缺基线键 %q", key)
		}
	}

	// 序列化往返保持同一份信封：信封要进 Outbox 行再被发布进程读回，Marshal/Unmarshal
	// 不守恒的话，落库的就不是发出的那份。
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("信封序列化：%v", err)
	}
	var roundTripped eventing.Envelope
	if err := json.Unmarshal(raw, &roundTripped); err != nil {
		t.Fatalf("信封反序列化：%v", err)
	}
	if roundTripped.ID != envelope.ID ||
		roundTripped.Type != envelope.Type ||
		roundTripped.Version != envelope.Version ||
		roundTripped.Scope != envelope.Scope ||
		roundTripped.Subject != envelope.Subject ||
		roundTripped.PartitionKey != envelope.PartitionKey ||
		!roundTripped.OccurredAt.Equal(envelope.OccurredAt) ||
		!roundTripped.RecordedAt.Equal(envelope.RecordedAt) ||
		!bytes.Equal(bytes.TrimSpace(roundTripped.Payload), bytes.TrimSpace(envelope.Payload)) {
		t.Fatalf("信封往返变形：%#v", roundTripped)
	}
}

// TestSubmittedEnvelopesKeepPartitionOrderAndRedeliver 证同委托分区顺序与 At-Least-Once
// 重投在真实 Store 上对 Parcel 形状的信封成立：队首未完结时后继不可领、他委托独立可领、
// 租约过期后重投的是同一份信封（同 ID 同 Payload、尝试计数推进）。
//
// 同一委托今天只有`已提交`一种事件类型；同分区后继用第二个事件标识站位表示「同一委托
// 的下一份信封」，不宣称存在第二次提交事件——顺序性质按分区键成立，与事件类型无关。
func TestSubmittedEnvelopesKeepPartitionOrderAndRedeliver(t *testing.T) {
	db, _ := parcelDB(t)
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	transactor := db.Transactor()
	ctx := context.Background()

	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}
	request := submittedContractRequest(t, scope, "order-key-1", "order-request-1")
	otherRequest := submittedContractRequest(t, scope, "order-key-2", "order-request-2")

	head := submittedEnvelopeFixture(t, "order-event-1", scope, request)
	successor := submittedEnvelopeFixture(t, "order-event-2", scope, request)
	successor.RecordedAt = head.RecordedAt.Add(time.Second)
	otherPartition := submittedEnvelopeFixture(t, "order-event-3", scope, otherRequest)

	if head.PartitionKey != successor.PartitionKey {
		t.Fatal("同一委托的两份信封分区键不同——稳定复合键坏了")
	}
	if head.PartitionKey == otherPartition.PartitionKey {
		t.Fatal("两份委托撞进了同一个分区")
	}

	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, envelope := range []eventing.Envelope{head, successor, otherPartition} {
			if err := store.Enqueue(txCtx, envelope); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("入队：%v", err)
	}

	lease := time.Minute
	claim := func(now time.Time) []eventing.Delivery {
		t.Helper()
		deliveries, err := store.Claim(ctx, eventing.OutboxClaim{
			Now: now, Limit: 10, LeaseFor: lease, MaxAttempts: 5,
		})
		if err != nil {
			t.Fatalf("领取投递：%v", err)
		}
		return deliveries
	}
	byID := func(deliveries []eventing.Delivery, id eventing.EventID) (eventing.Delivery, bool) {
		for _, delivery := range deliveries {
			if delivery.Envelope.ID == id {
				return delivery, true
			}
		}
		return eventing.Delivery{}, false
	}

	// 队首未完结：同分区后继不可领，他委托独立可领。领取从全部信封的 RecordedAt 之后
	// 开始——框架入队把 available_at 取为 recorded_at。
	claimStart := contractInstant.Add(5 * time.Minute)
	first := claim(claimStart)
	headDelivery, ok := byID(first, head.ID)
	if !ok {
		t.Fatalf("队首没被领到：%#v", first)
	}
	if headDelivery.Attempt != 1 {
		t.Fatalf("首领尝试计数 = %d, want 1", headDelivery.Attempt)
	}
	if _, ok := byID(first, successor.ID); ok {
		t.Fatal("队首未完结，同委托后继就被领走——分区顺序破了")
	}
	if _, ok := byID(first, otherPartition.ID); !ok {
		t.Fatal("另一委托的分区没有独立可领")
	}

	// 租约过期不完结：重投的是同一份信封（At-Least-Once），尝试计数如实推进。
	redeliverAt := claimStart.Add(lease).Add(time.Second)
	redelivered := claim(redeliverAt)
	second, ok := byID(redelivered, head.ID)
	if !ok {
		t.Fatalf("租约过期后队首没有重投：%#v", redelivered)
	}
	if second.Attempt != 2 {
		t.Fatalf("重投尝试计数 = %d, want 2", second.Attempt)
	}
	// payload 列是 jsonb，键序与空白经库归一，字节比较要对同为库内形态的两次投递做；
	// 与入队原件的比较按解构后的值做——语义相同即同一份。
	if !bytes.Equal(second.Envelope.Payload, headDelivery.Envelope.Payload) {
		t.Fatal("重投的信封 Payload 与首领不同——重投不是同一份")
	}
	var redeliveredPayload, enqueuedPayload map[string]any
	if err := json.Unmarshal(second.Envelope.Payload, &redeliveredPayload); err != nil {
		t.Fatalf("解重投 Payload：%v", err)
	}
	if err := json.Unmarshal(head.Payload, &enqueuedPayload); err != nil {
		t.Fatalf("解入队 Payload：%v", err)
	}
	if !reflect.DeepEqual(redeliveredPayload, enqueuedPayload) {
		t.Fatalf("重投 Payload 语义 = %v，入队原件 = %v", redeliveredPayload, enqueuedPayload)
	}

	// 完结队首后，同分区后继才可领。
	if err := store.MarkPublished(ctx, second.Ref, redeliverAt); err != nil {
		t.Fatalf("完结队首：%v", err)
	}
	next := claim(redeliverAt.Add(time.Second))
	if _, ok := byID(next, successor.ID); !ok {
		t.Fatalf("队首完结后后继仍不可领：%#v", next)
	}
}
