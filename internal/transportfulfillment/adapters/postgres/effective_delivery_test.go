package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证交付登记库的行为：当前版往返、首登幂等由部分唯一
// 索引拦住且事务保持可用、更正翻旧插新历史行只增不删、POD 必备入库内 CHECK、
// 作用域隔离、无事务拒、回滚无痕。

var deliveredAtFixture = time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)

func TestADeliveryRoundTripsItsCurrentVersion(t *testing.T) {
	repository, transactor, _ := newEffectiveDeliveries(t)
	ctx := t.Context()

	record := deliveryRecord(t, "pod-1", "delivery/v1")
	mustSaveDelivery(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, deliveryKeyFixture(t, "tenant-1"))
	if err != nil {
		t.Fatalf("取回交付：%v", err)
	}
	if !exists {
		t.Fatal("已登记的交付读不回来")
	}
	if found.Delivery.Proof().String() != "pod-1" ||
		found.Delivery.Version().String() != "delivery/v1" ||
		found.Delivery.Method().String() != "signature" ||
		found.Delivery.Recipient().String() != "recipient-1" ||
		!found.Delivery.OccurredAt().Equal(deliveredAtFixture) ||
		found.ContentDigest != record.ContentDigest {
		t.Fatalf("交付往返变形：%+v", found.Delivery)
	}
	if _, corrected := found.Delivery.Corrects(); corrected {
		t.Fatal("首登读回却带了前版引用")
	}
}

// TestSavingTwiceKeepsTheFirstRegistration 证首登幂等：撞当前版唯一译已登记（业务
// 答案不是 error），撞键后同一事务立刻读回先到者——事务保持可用是这条代数的另一半。
func TestSavingTwiceKeepsTheFirstRegistration(t *testing.T) {
	repository, transactor, _ := newEffectiveDeliveries(t)
	ctx := t.Context()

	mustSaveDelivery(t, transactor, ctx, repository, deliveryRecord(t, "pod-1", "delivery/v1"))

	second := deliveryRecord(t, "pod-late", "delivery/v9")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, second)
		if err != nil {
			return err
		}
		if outcome != ports.DeliveryAlreadyRegistered {
			t.Fatalf("outcome = %d, want ALREADY_REGISTERED", outcome)
		}
		found, exists, err := repository.FindByKey(txCtx, deliveryKeyFixture(t, "tenant-1"))
		if err != nil || !exists {
			t.Fatalf("撞键后同事务读回失败：%v exists=%v", err, exists)
		}
		if found.Delivery.Proof().String() != "pod-1" {
			t.Fatal("后到者覆盖了先到者的登记")
		}
		return nil
	})
}

// TestACorrectionAppendsANewRowPointingBack 证更正是新键新行指回前版：历史行只增
// 不删（两行在库）、当前版翻到新行、前版引用与更正时间随行保全。
func TestACorrectionAppendsANewRowPointingBack(t *testing.T) {
	repository, transactor, pool := newEffectiveDeliveries(t)
	ctx := t.Context()

	first := deliveryRecord(t, "pod-1", "delivery/v1")
	mustSaveDelivery(t, transactor, ctx, repository, first)

	corrected, err := first.Delivery.CorrectProof(
		deliveryValue(t, domain.NewDeliveryProofReference, "pod-2"),
		deliveryValue(t, domain.NewDeliveryResultVersion, "delivery/v2"),
		deliveredAtFixture.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	record := first
	record.Delivery = corrected
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		superseded, err := repository.Supersede(txCtx, record)
		if err != nil {
			return err
		}
		if !superseded {
			t.Fatal("有登记却顶替失败")
		}
		return nil
	})

	found, exists, err := repository.FindByKey(ctx, deliveryKeyFixture(t, "tenant-1"))
	if err != nil || !exists {
		t.Fatalf("读回当前版：%v exists=%v", err, exists)
	}
	predecessor, corrects := found.Delivery.Corrects()
	if !corrects || predecessor.String() != "delivery/v1" {
		t.Fatalf("前版引用没有随行保全：%+v", found.Delivery)
	}
	if found.Delivery.Proof().String() != "pod-2" {
		t.Fatalf("proof = %q", found.Delivery.Proof())
	}

	var total, current int
	if err := pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE is_current)
		   FROM transport_fulfillment.effective_delivery`).Scan(&total, &current); err != nil {
		t.Fatalf("统计版本行：%v", err)
	}
	if total != 2 || current != 1 {
		t.Fatalf("rows = %d current = %d, want 2/1（历史行只增不删、当前版唯一）", total, current)
	}

	t.Run("superseding an absent registration reports false", func(t *testing.T) {
		absent := deliveryRecord(t, "pod-x", "delivery/v9")
		absent.Key.Object = deliveryValue(t, domain.NewCarriedObjectReference, "parcel-9")
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			superseded, err := repository.Supersede(txCtx, absent)
			if err != nil {
				return err
			}
			if superseded {
				t.Fatal("没有登记却宣称顶替成功")
			}
			return nil
		})
	})
}

// TestDeliveryScopesAreInvisibleToEachOther 证否定结果不泄露其他租户是否存在该交付。
func TestDeliveryScopesAreInvisibleToEachOther(t *testing.T) {
	repository, transactor, _ := newEffectiveDeliveries(t)
	ctx := t.Context()

	mustSaveDelivery(t, transactor, ctx, repository, deliveryRecord(t, "pod-1", "delivery/v1"))

	_, exists, err := repository.FindByKey(ctx, deliveryKeyFixture(t, "tenant-b"))
	if err != nil {
		t.Fatalf("他租户查询出错：%v", err)
	}
	if exists {
		t.Error("他租户读到了本租户的交付")
	}
}

// TestPODRequiredIsPinnedInTheDatabase 证 POD 必备入库内 CHECK：绕过领域直插一行
// 空证明的「交付」被数据库拒（领域重建门是第二道，两道互补不互替）。
func TestPODRequiredIsPinnedInTheDatabase(t *testing.T) {
	_, _, pool := newEffectiveDeliveries(t)
	ctx := t.Context()

	_, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.effective_delivery
			(tenant_id, object_ref, attempt_ref, delivery_version,
			 place_ref, method_ref, recipient_ref, proof_ref,
			 corrects_version, corrected_at, occurred_at, content_digest, is_current)
		 VALUES ('tenant-1', 'parcel-1', 'attempt-1', 'delivery/v1',
		         'door-1', 'signature', 'recipient-1', '   ',
		         NULL, NULL, now(), 'digest-1', true)`)
	if err == nil {
		t.Fatal("一行没有交付证明的「交付」进了库")
	}
}

// TestDeliveryWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
func TestDeliveryWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newEffectiveDeliveries(t)
	ctx := t.Context()

	record := deliveryRecord(t, "pod-1", "delivery/v1")
	if _, err := repository.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务首登应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.Supersede(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务顶替应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestDeliveryRollbackLeavesNothingBehind 证首登与它所在的事务同生共死。
func TestDeliveryRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newEffectiveDeliveries(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, deliveryRecord(t, "pod-1", "delivery/v1")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, deliveryKeyFixture(t, "tenant-1"))
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后登记仍在")
	}
}

// ---- 夹具 ----

func newEffectiveDeliveries(t *testing.T) (*adapter.EffectiveDeliveries, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewEffectiveDeliveries(db)
	if err != nil {
		t.Fatalf("构造交付登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinDeliveryTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveDelivery(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.EffectiveDeliveries,
	record ports.EffectiveDeliveryRecord,
) {
	t.Helper()
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.DeliverySaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})
}

func deliveryValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func deliveryKeyFixture(t *testing.T, tenant string) ports.EffectiveDeliveryKey {
	t.Helper()
	return ports.EffectiveDeliveryKey{
		TenantID: deliveryValue(t, domain.NewTenantID, tenant),
		Object:   deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:  deliveryValue(t, domain.NewAttemptReference, "attempt-1"),
	}
}

func deliveryRecord(t *testing.T, proof, version string) ports.EffectiveDeliveryRecord {
	t.Helper()
	delivery, err := domain.RehydrateEffectiveDelivery(domain.RehydrateEffectiveDeliverySpec{
		TenantID:   deliveryValue(t, domain.NewTenantID, "tenant-1"),
		Object:     deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:    deliveryValue(t, domain.NewAttemptReference, "attempt-1"),
		Place:      deliveryValue(t, domain.NewAttemptPlaceReference, "door-1"),
		Method:     deliveryValue(t, domain.NewDeliveryMethodReference, "signature"),
		Recipient:  deliveryValue(t, domain.NewReceivingPartyReference, "recipient-1"),
		Proof:      deliveryValue(t, domain.NewDeliveryProofReference, proof),
		Version:    deliveryValue(t, domain.NewDeliveryResultVersion, version),
		OccurredAt: deliveredAtFixture,
	})
	if err != nil {
		t.Fatalf("重建交付夹具：%v", err)
	}
	return ports.EffectiveDeliveryRecord{
		Key:           deliveryKeyFixture(t, "tenant-1"),
		ContentDigest: "digest-" + proof,
		Delivery:      delivery,
		RecordedAt:    deliveredAtFixture,
	}
}
