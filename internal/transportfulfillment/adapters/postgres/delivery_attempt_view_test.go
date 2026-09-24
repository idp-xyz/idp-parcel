package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 证派送尝试视图：尝试与对象结果一次取回并经领域构造门
// 复验、指错的尝试或对象只回 found=false（不是 error）、作用域隔离，以及「妥投不得
// 带依据／失败必带原因」由库内 CHECK 守住。

var deliveryArrivedAtFixture = time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)

func TestADeliveryResultLoadsWithItsAttempt(t *testing.T) {
	view, pool := newDeliveryAttempts(t)
	ctx := t.Context()

	seedDeliveryAttempt(t, pool, "tenant-1", "attempt-1", []string{"parcel-1", "parcel-2"})
	seedDeliveryResult(t, pool, "tenant-1", "attempt-1", "parcel-1", "DELIVERED", nil)

	attempt, result, found, err := view.LoadDeliveryResult(ctx,
		deliveryViewValue(t, domain.NewTenantID, "tenant-1"),
		deliveryViewValue(t, domain.NewAttemptReference, "attempt-1"),
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if err != nil {
		t.Fatalf("取回派送结果：%v", err)
	}
	if !found {
		t.Fatal("已登记的派送结果读不回来")
	}
	if attempt.Attempt().String() != "attempt-1" ||
		attempt.Task().String() != "task-1" ||
		attempt.ExecutedBy().String() != "courier-1" ||
		attempt.Place().String() != "door-1" ||
		!attempt.ArrivedAt().Equal(deliveryArrivedAtFixture) ||
		!attempt.Covers(deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-2")) {
		t.Fatalf("尝试往返变形：%+v", attempt)
	}
	if result.Outcome() != domain.ObjectDelivered ||
		result.Object().String() != "parcel-1" ||
		result.Attempt().String() != "attempt-1" {
		t.Fatalf("对象结果往返变形：%+v", result)
	}
	if _, hasBasis := result.Basis(); hasBasis {
		t.Fatal("妥投读回却带了失败依据")
	}
}

// TestAFailedDeliveryResultKeepsItsBasis 证失败那一格：原因依据随行——没有原因的
// 失败与数据丢失无从分辨。
func TestAFailedDeliveryResultKeepsItsBasis(t *testing.T) {
	view, pool := newDeliveryAttempts(t)
	ctx := t.Context()

	basis := "no-one-home"
	seedDeliveryAttempt(t, pool, "tenant-1", "attempt-1", []string{"parcel-1"})
	seedDeliveryResult(t, pool, "tenant-1", "attempt-1", "parcel-1", "NO_ONE_TO_RECEIVE", &basis)

	_, result, found, err := view.LoadDeliveryResult(ctx,
		deliveryViewValue(t, domain.NewTenantID, "tenant-1"),
		deliveryViewValue(t, domain.NewAttemptReference, "attempt-1"),
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if err != nil || !found {
		t.Fatalf("取回失败结果：%v found=%v", err, found)
	}
	if result.Outcome() != domain.NoOneToReceive {
		t.Fatalf("outcome = %v", result.Outcome())
	}
	carried, hasBasis := result.Basis()
	if !hasBasis || carried.String() != basis {
		t.Fatal("失败原因依据没有随行保全")
	}
}

// TestAnAbsentAttemptOrObjectIsNotAnError 证 found=false 与 error 的分界：指名的尝试
// 或对象结果不存在是提交矛盾（编排据此答未受理），不是「等谁」。
func TestAnAbsentAttemptOrObjectIsNotAnError(t *testing.T) {
	view, pool := newDeliveryAttempts(t)
	ctx := t.Context()

	seedDeliveryAttempt(t, pool, "tenant-1", "attempt-1", []string{"parcel-1"})
	seedDeliveryResult(t, pool, "tenant-1", "attempt-1", "parcel-1", "DELIVERED", nil)

	cases := map[string]struct{ tenant, attempt, object string }{
		"尝试不存在":   {"tenant-1", "attempt-absent", "parcel-1"},
		"对象不在结果里": {"tenant-1", "attempt-1", "parcel-absent"},
		"另一个租户":   {"tenant-b", "attempt-1", "parcel-1"},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, found, err := view.LoadDeliveryResult(ctx,
				deliveryViewValue(t, domain.NewTenantID, value.tenant),
				deliveryViewValue(t, domain.NewAttemptReference, value.attempt),
				deliveryViewValue(t, domain.NewCarriedObjectReference, value.object))
			if err != nil {
				t.Fatalf("指错不该报错，实得：%v", err)
			}
			if found {
				t.Fatal("指名的尝试/对象不存在却答了 found=true")
			}
		})
	}
}

// TestDeliveryResultBasisCouplingIsPinnedInTheDatabase 证妥投与失败的依据耦合入库内
// CHECK：妥投带依据、失败缺依据两种坏行都被数据库拒。
func TestDeliveryResultBasisCouplingIsPinnedInTheDatabase(t *testing.T) {
	_, pool := newDeliveryAttempts(t)
	ctx := t.Context()

	seedDeliveryAttempt(t, pool, "tenant-1", "attempt-1", []string{"parcel-1"})

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-1', 'attempt-1', 'parcel-1', 'DELIVERED', 'some-basis', now())`,
	); err == nil {
		t.Fatal("一行带失败依据的「妥投」进了库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ('tenant-1', 'attempt-1', 'parcel-1', 'REFUSED', NULL, now())`,
	); err == nil {
		t.Fatal("一行没有原因的「拒收」进了库")
	}
}

// TestAResultOutsideTheAttemptScopeIsRefusedOnRead 证跨表不变量由领域重建门守住：
// 对象不在尝试范围内是库内 CHECK 表达不了的，读回时 FormDeliveryAttemptResult 拒。
func TestAResultOutsideTheAttemptScopeIsRefusedOnRead(t *testing.T) {
	view, pool := newDeliveryAttempts(t)
	ctx := t.Context()

	seedDeliveryAttempt(t, pool, "tenant-1", "attempt-1", []string{"parcel-1"})
	seedDeliveryResult(t, pool, "tenant-1", "attempt-1", "parcel-outside", "DELIVERED", nil)

	_, _, _, err := view.LoadDeliveryResult(ctx,
		deliveryViewValue(t, domain.NewTenantID, "tenant-1"),
		deliveryViewValue(t, domain.NewAttemptReference, "attempt-1"),
		deliveryViewValue(t, domain.NewCarriedObjectReference, "parcel-outside"))
	if err == nil {
		t.Fatal("范围外的对象结果被当成一份合法结果交了出去")
	}
}

// ---- 夹具 ----

func newDeliveryAttempts(t *testing.T) (*adapter.DeliveryAttempts, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	view, err := adapter.NewDeliveryAttempts(db)
	if err != nil {
		t.Fatalf("构造派送尝试视图：%v", err)
	}
	return view, pool
}

func deliveryViewValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// seedDeliveryAttempt 直插派送尝试行：本文件证的是视图对那张权威表的翻译，不是写入方
// （DeliveryAttempts.Save）的行为，夹具因此从库这一层造事实，绕开写口的复核。
func seedDeliveryAttempt(t *testing.T, pool *pgxpool.Pool, tenant, attempt string, objects []string) {
	t.Helper()
	payload := "["
	for index, object := range objects {
		if index > 0 {
			payload += ","
		}
		payload += `"` + object + `"`
	}
	payload += "]"

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO transport_fulfillment.delivery_attempt
			(tenant_id, attempt_ref, task_ref, executed_by, place_ref,
			 planned_from, planned_to, arrived_at, evidence_ref, rescheduled_from,
			 objects, recorded_at)
		 VALUES ($1, $2, 'task-1', 'courier-1', 'door-1',
		         $3, $4, $5, 'scan-1', NULL, $6::jsonb, $5)`,
		tenant, attempt,
		deliveryArrivedAtFixture.Add(-time.Hour),
		deliveryArrivedAtFixture.Add(time.Hour),
		deliveryArrivedAtFixture,
		payload,
	); err != nil {
		t.Fatalf("植入派送尝试：%v", err)
	}
}

func seedDeliveryResult(t *testing.T, pool *pgxpool.Pool, tenant, attempt, object, outcome string, basis *string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant, attempt, object, outcome, basis,
		deliveryArrivedAtFixture.Add(15*time.Minute),
	); err != nil {
		t.Fatalf("植入派送结果：%v", err)
	}
}
