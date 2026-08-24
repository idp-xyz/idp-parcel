package bentocontract

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件取 `PBC-05`:Envelope v1 校验、Payload 最小化、同委托分区顺序和 At-Least-Once
// 重投符合框架合同。
//
// 四边各自的证法——v1 校验拿真实信封过框架 Validate;载荷最小化按「恰好这套键、一个不多」
// 断言(DisallowUnknownFields 证无缺,键数证无多);分区键证「同一委托稳定、不同委托各自
// 成区」——同分区内的 FIFO 与终结语义由框架 Outbox 合同承担(PBC-06 已过),本文件证的是
// Parcel 把自己的事件放进正确的分区;At-Least-Once 用真实信封走租约过期,重投回来的必须
// 是同一份、一个字节都不变。

// submittedEnvelopePayload 镜像适配器载荷的键集。只在断言里用:DisallowUnknownFields
// 拿它当格子,信封里多出任何一个键都当场失败。
type submittedEnvelopePayload struct {
	TenantID            string   `json:"tenantId"`
	CustomerAccountID   string   `json:"customerAccountId"`
	Source              string   `json:"source"`
	SourceRequestKey    string   `json:"sourceRequestKey"`
	ShipmentRequestID   string   `json:"shipmentRequestId"`
	SubmissionBatchID   string   `json:"submissionBatchId"`
	SubmissionVersionID string   `json:"submissionVersionId"`
	DeclaredParcelIDs   []string `json:"declaredParcelIds"`
}

func TestSubmittedEnvelopeConformsToTheFrameworkContract(t *testing.T) {
	fixture := newSubmitFlowFixture(t, nil)
	ctx := t.Context()
	first := contractIdentity(t, "tenant-1", "pbc05-key-1")
	second := contractIdentity(t, "tenant-1", "pbc05-key-2")

	for _, submission := range []struct {
		identity  domain.SourceIdentity
		requestID string
	}{
		{identity: first, requestID: "PBC05-REQ-01"},
		{identity: second, requestID: "PBC05-REQ-02"},
	} {
		result, err := fixture.handler.Handle(ctx,
			fixture.submitCommand(t, submission.identity, submission.requestID, "sha256:pbc05", submitFlowSubmittedAt.Add(-time.Minute)))
		if err != nil || result.Outcome() != psapplication.OutcomeSubmitted {
			t.Fatalf("提交 %s = %s, err=%v", submission.requestID, result.Outcome(), err)
		}
	}

	claimAt := time.Now().UTC()
	deliveries, err := fixture.store.Claim(ctx, eventing.OutboxClaim{
		Now: claimAt, Limit: 10, LeaseFor: time.Minute, MaxAttempts: 10,
	})
	if err != nil {
		t.Fatalf("领取:%v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("可领 %d 件, want 2", len(deliveries))
	}
	byID := make(map[string]eventing.Delivery, len(deliveries))
	for _, delivery := range deliveries {
		byID[string(delivery.Envelope.ID)] = delivery
	}
	firstDelivery, ok := byID[submittedFlowEventID(first)]
	if !ok {
		t.Fatalf("第一份委托的信封不在可领集合里:%v", byID)
	}
	secondDelivery, ok := byID[submittedFlowEventID(second)]
	if !ok {
		t.Fatalf("第二份委托的信封不在可领集合里:%v", byID)
	}

	// —— Envelope v1 校验:真实信封必须原样通过框架校验,各字段是登记的那一个。——
	envelope := firstDelivery.Envelope
	if err := envelope.Validate(); err != nil {
		t.Fatalf("信封没过框架校验:%v", err)
	}
	if envelope.SpecVersion != eventing.SpecVersionV1 {
		t.Fatalf("SpecVersion = %q, want v1", envelope.SpecVersion)
	}
	if string(envelope.Type) != submittedFlowEventType {
		t.Fatalf("Type = %q, want %q", envelope.Type, submittedFlowEventType)
	}
	if envelope.Source != "idp-parcel/parcel-shipment" {
		t.Fatalf("Source = %q, 不是本上下文的稳定名", envelope.Source)
	}
	if envelope.Version != 1 {
		t.Fatalf("Version = %d, want 1", envelope.Version)
	}
	if envelope.Scope != "tenant-1/customer-1" {
		t.Fatalf("Scope = %q, want 租户/客户账户复合标识", envelope.Scope)
	}
	if envelope.Subject != "PBC05-REQ-01" {
		t.Fatalf("Subject = %q, want 委托标识", envelope.Subject)
	}
	if envelope.PartitionKey != "tenant-1/customer-1/PBC05-REQ-01" {
		t.Fatalf("PartitionKey = %q, want 租户/客户账户/委托", envelope.PartitionKey)
	}
	// 领域发生时间与记录时间分别填写:提交时刻来自领域,入队时刻来自适配器时钟,
	// 两个夹具值故意错开,填串位就会在这里现形。
	if !envelope.OccurredAt.Equal(submitFlowSubmittedAt) {
		t.Fatalf("OccurredAt = %v, want 提交时刻 %v", envelope.OccurredAt, submitFlowSubmittedAt)
	}
	if !envelope.RecordedAt.Equal(submitFlowRecordedAt) {
		t.Fatalf("RecordedAt = %v, want 入队时刻 %v", envelope.RecordedAt, submitFlowRecordedAt)
	}

	// —— Payload 最小化:恰好这套标识键,无缺无多,不带任何地址、联系人、货物或申报明文。——
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	var payload submittedEnvelopePayload
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("载荷里有登记键集之外的内容:%v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &keys); err != nil {
		t.Fatalf("载荷不是 JSON 对象:%v", err)
	}
	if len(keys) != 8 {
		t.Fatalf("载荷有 %d 个键, want 恰好 8 个——少一个是漏关联,多一个是漏内容", len(keys))
	}
	if payload.TenantID != "tenant-1" || payload.CustomerAccountID != "customer-1" ||
		payload.Source != "portal" || payload.SourceRequestKey != "pbc05-key-1" {
		t.Fatalf("载荷的来源身份四维不对:%+v", payload)
	}
	if payload.ShipmentRequestID != "PBC05-REQ-01" ||
		payload.SubmissionBatchID != "PBC-BATCH-01" ||
		payload.SubmissionVersionID == "" {
		t.Fatalf("载荷的委托关联不对:%+v", payload)
	}
	if len(payload.DeclaredParcelIDs) != 2 ||
		payload.DeclaredParcelIDs[0] != "PBC-PARCEL-01" ||
		payload.DeclaredParcelIDs[1] != "PBC-PARCEL-02" {
		t.Fatalf("载荷的成员声明不对:%v", payload.DeclaredParcelIDs)
	}

	// —— 分区:同一委托的分区键是确定性的稳定复合键,不同委托各自成区、互不阻塞;
	// 可见性边界(Scope)相同。同分区内的先后顺序由框架 Outbox 合同承担(PBC-06)。——
	if firstDelivery.Envelope.PartitionKey == secondDelivery.Envelope.PartitionKey {
		t.Fatalf("两份委托共用分区 %q——一份的失败会拖住另一份", firstDelivery.Envelope.PartitionKey)
	}
	if firstDelivery.Envelope.Scope != secondDelivery.Envelope.Scope {
		t.Fatalf("同一客户的两份委托可见性边界不同:%q 与 %q",
			firstDelivery.Envelope.Scope, secondDelivery.Envelope.Scope)
	}

	// —— At-Least-Once 重投:不终结、等租约过期,再领必须拿回同一份,信封一个字节不变。——
	redelivered, err := fixture.store.Claim(ctx, eventing.OutboxClaim{
		Now: claimAt.Add(2 * time.Minute), Limit: 10, LeaseFor: time.Minute, MaxAttempts: 10,
	})
	if err != nil {
		t.Fatalf("重领:%v", err)
	}
	if len(redelivered) != 2 {
		t.Fatalf("租约过期后可领 %d 件, want 2——未终结的投递必须重投", len(redelivered))
	}
	for _, delivery := range redelivered {
		original, known := byID[string(delivery.Envelope.ID)]
		if !known {
			t.Fatalf("重投出现首轮没有的信封:%s", delivery.Envelope.ID)
		}
		if delivery.Attempt != original.Attempt+1 {
			t.Fatalf("%s 的重投尝试 = %d, want %d", delivery.Envelope.ID, delivery.Attempt, original.Attempt+1)
		}
		if !bytes.Equal(delivery.Envelope.Payload, original.Envelope.Payload) ||
			delivery.Envelope.PartitionKey != original.Envelope.PartitionKey ||
			delivery.Envelope.Type != original.Envelope.Type {
			t.Fatalf("%s 重投后内容变了", delivery.Envelope.ID)
		}
	}

	// —— 重放不产出第二个 EventID:同键同摘要再来一次,意图仍是两份委托各一。——
	replay, err := fixture.handler.Handle(ctx,
		fixture.submitCommand(t, first, "PBC05-REQ-01", "sha256:pbc05", submitFlowSubmittedAt.Add(time.Minute)))
	if err != nil || replay.Outcome() != psapplication.OutcomeExistingResult {
		t.Fatalf("重放 = %s, err=%v", replay.Outcome(), err)
	}
	if intents := fixture.submittedIntentCount(t); intents != 2 {
		t.Fatalf("意图行数 = %d, want 2——重放不得造新信封", intents)
	}
}
