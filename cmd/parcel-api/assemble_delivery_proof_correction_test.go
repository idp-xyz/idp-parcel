package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 operator-channel/41 裁定 (a) 在生产装配的交付编排与真库上成立——POD 更正指名前版：原样重发答已有版本、
// 交回原版本，交付行与意图都不再加；迟到的旧更正答冲突，当前 POD 不被改回。测试输入是隔离合成，只记 `S`。
func TestTheWiredPODCorrectionTellsReplayFromConflictAgainstARealDatabase(t *testing.T) {
	pool, delivery := wiredDeliveryWithAttempt(t, []string{"SYN-PARCEL-41"})
	first := registeredDeliveryVersion(t, delivery, "SYN-PARCEL-41")
	at := deliveryAttemptArrivedAt.Add(2 * time.Hour)

	corrected := correctPOD(t, delivery, "SYN-PARCEL-41", first, "SYN-POD-41-B", at)
	if got := corrected.Outcome(); got != tfapp.DeliveryCorrected {
		t.Fatalf("outcome = %v, want DELIVERY_CORRECTED", got)
	}
	correctedRecord, _ := corrected.Record()
	second := correctedRecord.Delivery.Version().String()
	rows, intents := deliveryRowsAndIntents(t, pool)

	replay := correctPOD(t, delivery, "SYN-PARCEL-41", first, "SYN-POD-41-B", at)
	if got := replay.Outcome(); got != tfapp.DeliveryExistingVersion {
		t.Fatalf("原样重发 outcome = %v, want EXISTING_VERSION", got)
	}
	if record, _ := replay.Record(); record.Delivery.Version().String() != second {
		t.Fatalf("原样重发交回 %q，want 原版本 %q", record.Delivery.Version(), second)
	}
	if gotRows, gotIntents := deliveryRowsAndIntents(t, pool); gotRows != rows || gotIntents != intents {
		t.Fatalf("原样重发后交付行 %d→%d、意图 %d→%d，want 都不变（同一版本的意图按事件标识去重）", rows, gotRows, intents, gotIntents)
	}

	third := correctPOD(t, delivery, "SYN-PARCEL-41", second, "SYN-POD-41-C", at.Add(time.Hour))
	if got := third.Outcome(); got != tfapp.DeliveryCorrected {
		t.Fatalf("对当前版再更正 outcome = %v, want DELIVERY_CORRECTED", got)
	}
	late := correctPOD(t, delivery, "SYN-PARCEL-41", first, "SYN-POD-41-A", at.Add(-time.Hour))
	if got := late.Outcome(); got != tfapp.DeliveryRegistrationConflict {
		t.Fatalf("迟到的旧更正 outcome = %v, want SOURCE_CONFLICT", got)
	}
	var proof string
	if err := pool.QueryRow(t.Context(),
		`SELECT proof_ref FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = 'SYN-TENANT-1' AND object_ref = 'SYN-PARCEL-41' AND is_current`).Scan(&proof); err != nil {
		t.Fatalf("读当前版：%v", err)
	}
	if proof != "SYN-POD-41-C" {
		t.Fatalf("当前 POD = %q，want SYN-POD-41-C（迟到的旧更正不得改回）", proof)
	}
}

// Covers: 票 operator-channel/41 附录 F-6「读旧当前版时版本链分叉」——同一前版上两份更正并发，多轮之后每一版至多被
// 回指一次，没有翻成历史却无后继的版本；输家答冲突或已有版本，不答新更正。
func TestConcurrentPODCorrectionsOfOnePredecessorKeepTheChainLinearAgainstARealDatabase(t *testing.T) {
	const rounds = 6
	objects := make([]string, rounds)
	for round := range objects {
		objects[round] = fmt.Sprintf("SYN-PARCEL-41-R%d", round)
	}
	pool, delivery := wiredDeliveryWithAttempt(t, objects)
	at := deliveryAttemptArrivedAt.Add(2 * time.Hour)

	for _, object := range objects {
		first := registeredDeliveryVersion(t, delivery, object)
		outcomes := make([]tfapp.DeliveryRegistrationOutcome, 2)
		var wait sync.WaitGroup
		for racer := range outcomes {
			wait.Add(1)
			go func() {
				defer wait.Done()
				result, err := delivery.Correct(context.Background(), podCorrectionCommand(t, object, first, fmt.Sprintf("SYN-POD-RACE-%d", racer), at))
				if err != nil {
					t.Errorf("%s 第 %d 份更正：%v", object, racer, err)
					return
				}
				outcomes[racer] = result.Outcome()
			}()
		}
		wait.Wait()
		corrected := 0
		for _, outcome := range outcomes {
			switch outcome {
			case tfapp.DeliveryCorrected:
				corrected++
			case tfapp.DeliveryRegistrationConflict, tfapp.DeliveryExistingVersion, tfapp.DeliveryUndecided:
			default:
				t.Fatalf("%s 并发更正答了 %v", object, outcome)
			}
		}
		if corrected > 1 {
			t.Fatalf("%s 同一前版被更正了 %d 次", object, corrected)
		}
	}

	var forked, orphaned int
	if err := pool.QueryRow(t.Context(),
		`SELECT
		    (SELECT count(*) FROM (
		        SELECT 1 FROM transport_fulfillment.effective_delivery
		         WHERE corrects_version IS NOT NULL
		         GROUP BY tenant_id, object_ref, attempt_ref, corrects_version
		        HAVING count(*) > 1) AS forks),
		    (SELECT count(*) FROM transport_fulfillment.effective_delivery AS old
		      WHERE NOT old.is_current
		        AND NOT EXISTS (
		            SELECT 1 FROM transport_fulfillment.effective_delivery AS successor
		             WHERE successor.tenant_id = old.tenant_id
		               AND successor.object_ref = old.object_ref
		               AND successor.attempt_ref = old.attempt_ref
		               AND successor.corrects_version = old.delivery_version))`).Scan(&forked, &orphaned); err != nil {
		t.Fatalf("核版本链：%v", err)
	}
	if forked != 0 || orphaned != 0 {
		t.Fatalf("分叉的前版 %d 个、翻成历史却无后继的版本 %d 个，want 0/0", forked, orphaned)
	}
}

func wiredDeliveryWithAttempt(t *testing.T, objects []string) (*pgxpool.Pool, tfhttp.DeliveryHandler) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	seedDeliveryAttemptRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-1", objects)
	for _, object := range objects {
		seedDeliveryResultRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-1", object, "DELIVERED", nil)
	}
	return pool, delivery
}

func registeredDeliveryVersion(t *testing.T, delivery tfhttp.DeliveryHandler, object string) string {
	t.Helper()
	registered, err := delivery.Register(t.Context(), registerDeliveryCommand(t, "SYN-ATTEMPT-1", object))
	if err != nil {
		t.Fatalf("首登 %s：%v", object, err)
	}
	record, has := registered.Record()
	if registered.Outcome() != tfapp.DeliveryRegistered || !has {
		t.Fatalf("首登 %s outcome = %v", object, registered.Outcome())
	}
	return record.Delivery.Version().String()
}

func podCorrectionCommand(t *testing.T, object, predecessor, proof string, at time.Time) tfapp.CorrectDeliveryProofCommand {
	return tfapp.CorrectDeliveryProofCommand{
		TenantID:           mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Attempt:            "SYN-ATTEMPT-1",
		Object:             object,
		PredecessorVersion: predecessor,
		NewProof:           proof,
		CorrectedAt:        at,
	}
}

func correctPOD(t *testing.T, delivery tfhttp.DeliveryHandler, object, predecessor, proof string, at time.Time) tfapp.RegisterEffectiveDeliveryResult {
	t.Helper()
	result, err := delivery.Correct(t.Context(), podCorrectionCommand(t, object, predecessor, proof, at))
	if err != nil {
		t.Fatalf("POD 更正：%v", err)
	}
	return result
}

func deliveryRowsAndIntents(t *testing.T, pool *pgxpool.Pool) (int, int) {
	t.Helper()
	var rows, intents int
	if err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM transport_fulfillment.effective_delivery),
		        (SELECT count(*) FROM `+migrate.SchemaBento+`.outbox)`).Scan(&rows, &intents); err != nil {
		t.Fatalf("统计交付行与意图：%v", err)
	}
	return rows, intents
}
