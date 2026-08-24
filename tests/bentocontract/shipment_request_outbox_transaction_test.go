package bentocontract

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
	"go.idp.xyz/idp-bento-go/testkit"

	psadapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件取 `PBC-03`:业务 Repository 与 Outbox 使用同一个事务——成功时二者同时可见,回滚时
// 二者都不可见。
//
// 业务侧用真实的委托聚合仓储(不是为合同造的样品仓储):它与 Outbox Store 共用同一个
// `bentopg.DB` 与同一个 Transactor,这正是生产装配的形状。先验回滚半边再验提交半边,
// 使两次可见性检查互不借位——回滚检查时库里一无所有,提交检查时恰好各有一件。

func TestShipmentRequestAndOutboxShareOneTransaction(t *testing.T) {
	db := frameworkDB(t)
	requests, err := psadapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储:%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store:%v", err)
	}
	transactor := db.Transactor()
	ctx := context.Background()
	now := time.Now().UTC()

	rolledBackKey := contractIdentity(t, "tenant-1", "key-rolled-back")
	rolledBackRequest := rehydrated(t, submittedSpec(t, rolledBackKey, "request-rb", "version-1", "task-1"))
	rolledBackEnvelope := testkit.ValidEnvelope()
	rolledBackEnvelope.ID = "pbc03-event-0001"

	committedKey := contractIdentity(t, "tenant-1", "key-committed")
	committedRequest := rehydrated(t, submittedSpec(t, committedKey, "request-ok", "version-1", "task-1"))
	committedEnvelope := testkit.ValidEnvelope()
	committedEnvelope.ID = "pbc03-event-0002"
	committedEnvelope.Subject = "pbc03-committed"
	committedEnvelope.PartitionKey = "test-scope/pbc03-committed"

	// 回滚半边:两笔写入都成功后人为退出——错误必须原样透出,且两侧都像没发生过。
	// 闭包只做 IO 并回 error(事务闭包门禁):非预期结果译成错误透出,由闭包外的
	// errors.Is 判定当场失败,不在闭包里断言。
	rollback := errors.New("pbc-03 rollback")
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := requests.Insert(txCtx, rolledBackKey, rolledBackRequest)
		if err != nil {
			return err
		}
		if outcome != ports.ShipmentRequestInserted {
			return fmt.Errorf("回滚事务里的建单结果 = %d, want Inserted", outcome)
		}
		if err := store.Enqueue(txCtx, rolledBackEnvelope); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("回滚错误 = %v, want pbc-03 rollback", err)
	}

	if _, found, err := requests.FindBySourceIdentity(ctx, rolledBackKey); err != nil || found {
		t.Fatalf("回滚后委托仍可见:found=%v err=%v", found, err)
	}
	deliveries, err := store.Claim(ctx, eventing.OutboxClaim{
		Now: now, Limit: 10, LeaseFor: time.Minute, MaxAttempts: 10,
	})
	if err != nil {
		t.Fatalf("回滚后领取:%v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("回滚后 Outbox 仍有 %d 件可领,首件 %s", len(deliveries), deliveries[0].Envelope.ID)
	}

	// 提交半边:同一事务里建单 + 入队,提交后两侧同时可见。
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := requests.Insert(txCtx, committedKey, committedRequest)
		if err != nil {
			return err
		}
		if outcome != ports.ShipmentRequestInserted {
			return fmt.Errorf("提交事务里的建单结果 = %d, want Inserted", outcome)
		}
		return store.Enqueue(txCtx, committedEnvelope)
	})
	if err != nil {
		t.Fatalf("提交事务:%v", err)
	}

	loaded, found, err := requests.FindBySourceIdentity(ctx, committedKey)
	if err != nil || !found {
		t.Fatalf("提交后委托读不回:found=%v err=%v", found, err)
	}
	if loaded.Revision() != 1 || loaded.ShipmentRequestID() != committedRequest.ShipmentRequestID() {
		t.Fatalf("提交后读回 = %s revision %d", loaded.ShipmentRequestID(), loaded.Revision())
	}
	deliveries, err = store.Claim(ctx, eventing.OutboxClaim{
		Now: now, Limit: 10, LeaseFor: time.Minute, MaxAttempts: 10,
	})
	if err != nil {
		t.Fatalf("提交后领取:%v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Envelope.ID != committedEnvelope.ID {
		t.Fatalf("提交后 Outbox 可领 = %+v, want 恰好 %s", deliveries, committedEnvelope.ID)
	}
}
