package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// deliveryAttemptArrivedAt 是种入派送尝试的到场时刻；结果与更正都取它之后，
// 守住「结果不早于到场、更正不早于交付」两道领域门。
var deliveryAttemptArrivedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

// Covers: `/transport-fulfillment/deliveries` 与 `/transport-fulfillment/delivery-proof-corrections`
// 的第二参是真编排——尝试读面、交付登记库、结果版本签发与 Outbox 意图交付在真实
// PostgreSQL 上装得起来；指名不存在的尝试如实答未受理（真视图查过了，指错是提交矛盾，
// 不是未决，更不造一笔生效）；首登真实落库且意图经真 Outbox 同笔交出（重放走已有版本，
// 证首笔事务真的提交了）；POD 更正经 Supersede 翻旧插新（证 Correct 入口的事务边界同样
// 成立——写口按框架合同无事务即拒）；失败的尝试结果登不出生效交付（领域硬句经真读面
// 照样成立）。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredDeliveryAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}

	absent, err := delivery.Register(t.Context(), registerDeliveryCommand(t, "SYN-ATTEMPT-ABSENT", "SYN-PARCEL-1"))
	if err != nil {
		t.Fatalf("指名不存在的尝试：%v", err)
	}
	if got := absent.Outcome(); got != tfapp.DeliveryNotAccepted {
		t.Fatalf("outcome = %v, want SOURCE_NOT_ACCEPTED——尝试不存在是提交矛盾，不是未决", got)
	}

	seedDeliveryAttemptRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-1", []string{"SYN-PARCEL-1", "SYN-PARCEL-2"})
	seedDeliveryResultRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-1", "SYN-PARCEL-1", "DELIVERED", nil)
	failureBasis := "SYN-NO-ONE-HOME"
	seedDeliveryResultRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-1", "SYN-PARCEL-2", "NO_ONE_TO_RECEIVE", &failureBasis)

	command := registerDeliveryCommand(t, "SYN-ATTEMPT-1", "SYN-PARCEL-1")
	registered, err := delivery.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("首登交付生效：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.DeliveryRegistered {
		t.Fatalf("outcome = %v, want DELIVERY_REGISTERED", got)
	}
	if _, has := registered.Record(); !has {
		t.Fatal("首登成功却没带回登记")
	}
	if ref := registered.DeliveryHandoffReference(); ref != "" {
		t.Fatalf("意图交付走真 Outbox 应当成功，却留了续办引用 %q", ref)
	}

	replay, err := delivery.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份首登：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.DeliveryExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务的落库没有提交", got)
	}

	corrected, err := delivery.Correct(t.Context(), tfapp.CorrectDeliveryProofCommand{
		TenantID:    mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Attempt:     "SYN-ATTEMPT-1",
		Object:      "SYN-PARCEL-1",
		NewProof:    "SYN-POD-CORRECTED-1",
		CorrectedAt: deliveryAttemptArrivedAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("POD 更正：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.DeliveryCorrected {
		t.Fatalf("outcome = %v, want DELIVERY_CORRECTED——更正没走 Supersede 翻旧插新", got)
	}

	notEffective, err := delivery.Register(t.Context(), registerDeliveryCommand(t, "SYN-ATTEMPT-1", "SYN-PARCEL-2"))
	if err != nil {
		t.Fatalf("对失败结果首登：%v", err)
	}
	if got := notEffective.Outcome(); got != tfapp.DeliveryNotEffective {
		t.Fatalf("outcome = %v, want NOT_EFFECTIVE——失败的尝试结果登不出生效交付", got)
	}
}

func registerDeliveryCommand(t *testing.T, attempt, object string) tfapp.RegisterEffectiveDeliveryCommand {
	t.Helper()
	return tfapp.RegisterEffectiveDeliveryCommand{
		TenantID:  mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Attempt:   attempt,
		Object:    object,
		Method:    "SYN-METHOD-SIGNATURE",
		Recipient: "SYN-RECIPIENT-1",
		Proof:     "SYN-POD-1",
	}
}

// seedDeliveryAttemptRow 直插派送尝试行。尝试读面只读，本仓今天还没有派送尝试的登记
// 入口（揽收侧有 PerformOffsitePickup，派送侧的对应用例尚未落地），所以夹具从库这一层
// 造事实——本测试证的是装配对那张权威表的消费，不是某个写入方的行为。
func seedDeliveryAttemptRow(t *testing.T, pool *pgxpool.Pool, tenant, attempt string, objects []string) {
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
		 VALUES ($1, $2, 'SYN-TASK-1', 'SYN-COURIER-1', 'SYN-DOOR-1',
		         $3, $4, $5, 'SYN-SCAN-1', NULL, $6::jsonb, $5)`,
		tenant, attempt,
		deliveryAttemptArrivedAt.Add(-time.Hour),
		deliveryAttemptArrivedAt.Add(time.Hour),
		deliveryAttemptArrivedAt,
		payload,
	); err != nil {
		t.Fatalf("植入派送尝试：%v", err)
	}
}

func seedDeliveryResultRow(t *testing.T, pool *pgxpool.Pool, tenant, attempt, object, outcome string, basis *string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant, attempt, object, outcome, basis,
		deliveryAttemptArrivedAt.Add(15*time.Minute),
	); err != nil {
		t.Fatalf("植入派送结果：%v", err)
	}
}
