package bentocontract

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件取 `PBC-03`：业务 Repository 与 Outbox 使用同一个事务——成功时二者同时可见，
// 回滚时二者都不可见。
//
// 受测的是真实的委托仓储与框架 Outbox Store 装在**同一个** Transactor 事务里，跑在
// 真实 PostgreSQL 16 上。信封按简报「事件信封基线」构造（`委托已提交`）；它是取证
// 夹具——生产发射器（UC-PS-001 步骤 6 的同事务入队编排）今天还不存在，本文件证的是
// 两个真实端口在同一事务边界下的同生共死，不宣称生产编排已接上这条缝。
func TestRepositoryAndOutboxCommitTogether(t *testing.T) {
	db, _ := parcelDB(t)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	transactor := db.Transactor()
	ctx := context.Background()

	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}
	request := submittedContractRequest(t, scope, "tx-key-1", "tx-request-1")
	envelope := submittedEnvelopeFixture(t, "tx-event-1", scope, request)

	var inserted ports.ShipmentRequestInsertOutcome
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		inserted, err = requests.Insert(txCtx, requestSourceIdentity(request), request)
		if err != nil {
			return err
		}
		return store.Enqueue(txCtx, envelope)
	})
	if err != nil {
		t.Fatalf("同事务写入：%v", err)
	}
	if inserted != ports.ShipmentRequestInserted {
		t.Fatalf("建单结果 = %s, want INSERTED", inserted)
	}

	// 提交后两侧同时可见：委托读得回，信封领得到。领取时刻要晚于信封的 RecordedAt——
	// 框架入队把 available_at 取为 recorded_at，早于它领取会误判成「不可见」。
	claimAt := contractInstant.Add(5 * time.Minute)
	if _, found, err := requests.FindBySourceIdentity(ctx, requestSourceIdentity(request)); err != nil || !found {
		t.Fatalf("提交后委托不可见：found=%v err=%v", found, err)
	}
	deliveries, err := store.Claim(ctx, eventing.OutboxClaim{
		Now: claimAt, Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("领取投递：%v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Envelope.ID != envelope.ID {
		t.Fatalf("提交后的投递 = %#v，want 恰一条 %s", deliveries, envelope.ID)
	}
	if err := store.MarkPublished(ctx, deliveries[0].Ref, claimAt); err != nil {
		t.Fatalf("完结投递：%v", err)
	}
}

func TestRepositoryAndOutboxRollBackTogether(t *testing.T) {
	db, _ := parcelDB(t)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	transactor := db.Transactor()
	ctx := context.Background()

	scope := contractScope{tenant: "tenant-a", customer: "customer-a"}
	request := submittedContractRequest(t, scope, "tx-key-2", "tx-request-2")
	envelope := submittedEnvelopeFixture(t, "tx-event-2", scope, request)

	rollback := errors.New("contract rollback")
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := requests.Insert(txCtx, requestSourceIdentity(request), request); err != nil {
			return err
		}
		if err := store.Enqueue(txCtx, envelope); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("回滚错误 = %v", err)
	}

	// 回滚后两侧同时不可见：委托读不回，信封领不到。只剩一侧可见就是两账分岔——
	// 要么发出了没落库的事件，要么落了库却永远不发。领取时刻与提交侧用例同款地晚于
	// RecordedAt：这样「领不到」只能归因于回滚，不是还没到可领时刻。
	if _, found, err := requests.FindBySourceIdentity(ctx, requestSourceIdentity(request)); err != nil || found {
		t.Fatalf("回滚后委托仍可见：found=%v err=%v", found, err)
	}
	deliveries, err := store.Claim(ctx, eventing.OutboxClaim{
		Now: contractInstant.Add(5 * time.Minute), Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5,
	})
	if err != nil {
		t.Fatalf("领取投递：%v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("回滚后仍领到投递：%#v", deliveries)
	}
}

// submittedEnvelopeFixture 按简报「事件信封基线」构造一份`委托已提交`信封：
// Type/Version/Source 逐字取基线，Scope 是租户与客户账户复合标识，Subject 是委托标识，
// PartitionKey 是（租户, 客户账户, 委托）稳定复合键，Payload 只带委托、批次、版本与
// 声明包裹标识——不含地址、联系人、货物或申报明文。
//
// EventID 由调用方给：基线要求它在首次命令处理中生成并随业务结果保存、重复请求复用
// 原值，那半机制属生产发射器，本夹具不伪造。
func submittedEnvelopeFixture(
	t *testing.T,
	id eventing.EventID,
	scope contractScope,
	request domain.ShipmentRequest,
) eventing.Envelope {
	t.Helper()

	current := request.CurrentSubmissionVersion()
	payload := `{"shipmentRequestId":"` + request.ShipmentRequestID().String() +
		`","submissionBatchId":"` + request.BatchID().String() +
		`","submissionVersionId":"` + current.VersionID().String() + `","declaredParcelIds":[`
	for index, parcel := range current.DeclaredParcelIDs() {
		if index > 0 {
			payload += ","
		}
		payload += `"` + parcel.String() + `"`
	}
	payload += `]}`

	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersionV1,
		ID:           id,
		Source:       "go.idp.xyz/idp-parcel/parcel-shipment",
		Type:         "idp.parcel.shipment-request.submitted",
		Version:      1,
		Scope:        scope.tenant + "/" + scope.customer,
		Subject:      request.ShipmentRequestID().String(),
		PartitionKey: scope.tenant + "/" + scope.customer + "/" + request.ShipmentRequestID().String(),
		// 记录时间与领域发生时间分别填写（基线明令，不得用当前时间覆盖来源发生时间）。
		OccurredAt:  request.SubmittedAt(),
		RecordedAt:  contractInstant.Add(time.Minute),
		ContentType: eventing.JSONContentType,
		Payload:     []byte(payload),
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("基线信封立不起来：%v", err)
	}
	return envelope
}
