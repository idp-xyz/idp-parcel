package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证交付门禁读数（ADR-0213 决定五，TF 迁移 0043）：每一版交付都带一份，按键、按版本、更正、查阅册
// 各条读路都带着；缺读数、明细形状不对、问了关务的那一格对不上这一行的对象都被 CHECK 拒；存量回填那一形读回未决。另证交付
// 门禁读数找进段交接的那一口按版本找回`已交接`的各版。节点与对象都是合成串，证据只记 `S`。

func askedDeliveryGateReading(t *testing.T, object string, answer domain.GateAnswer) domain.DeliveryGateReading {
	t.Helper()
	finding, err := domain.NewGateFinding(deliveryValue(t, domain.NewCarriedObjectReference, object), answer, "AADJ-"+object,
		[]domain.GateBasis{{Action: "FINAL_DELIVERY", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"SYN-RESTRICTION-01"}}})
	if err != nil {
		t.Fatalf("那一格：%v", err)
	}
	reading, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{
		Node:    deliveryValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SIN-HUB"),
		Finding: finding,
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	return reading
}

func deliveryRecordReading(t *testing.T, proof, version string, reading domain.DeliveryGateReading) ports.EffectiveDeliveryRecord {
	t.Helper()
	record := deliveryRecord(t, proof, version)
	gated, err := record.Delivery.WithGateReading(reading)
	if err != nil {
		t.Fatalf("挂交付门禁读数：%v", err)
	}
	record.Delivery = gated
	return record
}

// Covers: `AT-TF-256`、`AT-TF-263`、`AT-TF-265` 的库面——问了关务的读数连同 at、关务判断标识与依据原文落进明细、整格另成一列，
// 按键、按版本原样装回；POD 更正那一行带着承继来的同一份，被翻成历史的前版仍带着自己的；查阅册列出整格。
func TestDeliveryGateReadingsRoundTripThroughEveryReadPath(t *testing.T) {
	repository, transactor, _ := newEffectiveDeliveries(t)
	ctx := t.Context()
	blocked := askedDeliveryGateReading(t, "parcel-1", domain.GateBlocked)
	first := deliveryRecordReading(t, "pod-1", "delivery/v1", blocked)
	mustSaveDelivery(t, transactor, ctx, repository, first)

	expect := func(label string, record ports.EffectiveDeliveryRecord) {
		t.Helper()
		reading, read := record.Delivery.GateReading()
		if !read || reading.Answer() != domain.GateBlocked {
			t.Fatalf("%s：读数 = %s (%v)", label, reading.Answer(), read)
		}
		node, asked := reading.Node()
		finding, _ := reading.Finding()
		judgement, _ := finding.Judgement()
		bases := finding.Bases()
		if !asked || node.String() != "SYN-NODE-SIN-HUB" || finding.Object().String() != "parcel-1" ||
			judgement != "AADJ-parcel-1" || len(bases) != 1 || bases[0].Sources[0] != "SYN-RESTRICTION-01" {
			t.Fatalf("%s：at=%q 那一格=%+v 依据=%+v", label, node, finding, bases)
		}
	}
	current, found, err := repository.FindByKey(ctx, first.Key)
	if err != nil || !found {
		t.Fatalf("按键读回：%v found=%v", err, found)
	}
	expect("按键", current)

	corrected, err := current.Delivery.CorrectProof(
		deliveryValue(t, domain.NewDeliveryProofReference, "pod-2"),
		deliveryValue(t, domain.NewDeliveryResultVersion, "delivery/v2"),
		deliveredAtFixture.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		superseded, err := repository.Supersede(txCtx, ports.EffectiveDeliveryRecord{
			Key: first.Key, ContentDigest: first.ContentDigest, Delivery: corrected, RecordedAt: deliveredAtFixture.Add(time.Hour),
		})
		if err == nil && !superseded {
			t.Fatal("更正没落下")
		}
		return err
	})
	current, _, err = repository.FindByKey(ctx, first.Key)
	if err != nil {
		t.Fatalf("更正后按键读回：%v", err)
	}
	if current.Delivery.Version().String() != "delivery/v2" {
		t.Fatalf("当前版 = %s", current.Delivery.Version())
	}
	expect("更正版", current)
	earlier, found, err := repository.FindByKeyAndVersion(ctx, first.Key, deliveryValue(t, domain.NewDeliveryResultVersion, "delivery/v1"))
	if err != nil || !found {
		t.Fatalf("按版本读回前版：%v found=%v", err, found)
	}
	expect("按版本", earlier)

	stores := newReviewCatalogueStores(t)
	stopped, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: domain.DeliveryEnteredByPickup})
	if err != nil {
		t.Fatalf("停点读数：%v", err)
	}
	record := deliveryRecordReading(t, "pod-1", "delivery/v1", stopped)
	if err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := stores.deliveries.Save(txCtx, record)
		return err
	}); err != nil {
		t.Fatalf("登一份停在本上下文这一侧的交付：%v", err)
	}
	rows, err := stores.catalogue.ListEffectiveDeliveries(ctx, record.Key.TenantID, 10)
	if err != nil || len(rows) != 1 || rows[0].GateReading != "UNDECIDED" {
		t.Fatalf("查阅册 = %+v %v，want 列出整格 UNDECIDED", rows, err)
	}
	stored, _, err := stores.deliveries.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("读回停点那一份：%v", err)
	}
	reading, _ := stored.Delivery.GateReading()
	if stop, _ := reading.Stop(); stop != domain.DeliveryEnteredByPickup {
		t.Fatalf("读回的停点 = %s", stop)
	}
}

// Covers: `AT-TF-264`、`AT-TF-265` 的库面半边——迁移 0043 的库面：缺读数、整格越出封闭集、明细停点与答复都在或都不在、问了关务
// 却没有 at、停在本上下文这一侧却带着 at、问了关务的那一格不是这一行的对象，都被拒；断言落到 SQLSTATE 与约束名上。没带
// 读数的那一版在发 SQL 之前就被适配器拒。存量回填那一形读回成形；整格与明细对不上的一行库面不核，重建门拒装回。
func TestTheDatabaseGuardsTheDeliveryGateReadingShape(t *testing.T) {
	repository, transactor, pool := newEffectiveDeliveries(t)
	ctx := t.Context()
	insert := func(attempt string, answer, detail any) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO transport_fulfillment.effective_delivery
				(tenant_id, object_ref, attempt_ref, delivery_version,
				 place_ref, method_ref, recipient_ref, proof_ref,
				 corrects_version, corrected_at, occurred_at, content_digest, is_current,
				 gate_reading, gate_reading_detail)
			 VALUES ('tenant-1', 'parcel-1', $1, 'delivery/v1',
			         'door-1', 'signature', 'recipient-1', 'pod-1',
			         NULL, NULL, $2, 'digest-1', true,
			         $3, $4::jsonb)`,
			attempt, deliveredAtFixture, answer, detail)
		return err
	}
	finding := func(object string) string {
		return `{"object": "` + object + `", "answer": "NOT_GUARDED", "judgement": "AADJ-1", "bases": [{"action": "FINAL_DELIVERY", "cause": "OUTSIDE_CUSTOMS_BOOKS", "sources": []}]}`
	}
	const backfill = `{"node": null, "stop": "RECORDED_BEFORE_GATE_READING", "finding": null}`
	for name, tc := range map[string]struct {
		answer, detail any
		code           string
		constraint     string
	}{
		"no reading":                   {nil, nil, "23502", ""},
		"answer out of the closed set": {"MAYBE", backfill, "23514", "effective_delivery_gate_reading_closed"},
		"neither stop nor finding":     {"UNDECIDED", `{"node": null, "stop": null, "finding": null}`, "23514", "effective_delivery_gate_reading_detail_shape"},
		"both stop and finding":        {"UNDECIDED", `{"node": "SYN-NODE-SIN-HUB", "stop": "ENTERED_BY_PICKUP", "finding": ` + finding("parcel-1") + `}`, "23514", "effective_delivery_gate_reading_detail_shape"},
		"asked without a node":         {"NOT_GUARDED", `{"node": null, "stop": null, "finding": ` + finding("parcel-1") + `}`, "23514", "effective_delivery_gate_reading_detail_shape"},
		"stopped with a node":          {"UNDECIDED", `{"node": "SYN-NODE-SIN-HUB", "stop": "ENTERED_BY_PICKUP", "finding": null}`, "23514", "effective_delivery_gate_reading_detail_shape"},
		"keys missing":                 {"UNDECIDED", `{}`, "23514", "effective_delivery_gate_reading_detail_shape"},
		"asked about another object":   {"NOT_GUARDED", `{"node": "SYN-NODE-SIN-HUB", "stop": null, "finding": ` + finding("parcel-2") + `}`, "23514", "effective_delivery_gate_reading_matches_delivery"},
	} {
		t.Run(name, func(t *testing.T) {
			err := insert("attempt-check-"+strings.ReplaceAll(name, " ", "-"), tc.answer, tc.detail)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code || pgErr.ConstraintName != tc.constraint {
				t.Fatalf("err = %v，want %s / %s", err, tc.code, tc.constraint)
			}
		})
	}
	if err := insert("attempt-asked", "NOT_GUARDED", `{"node": "SYN-NODE-SIN-HUB", "stop": null, "finding": `+finding("parcel-1")+`}`); err != nil {
		t.Fatalf("对得上的问询读数进不了库：%v", err)
	}

	bare := deliveryRecord(t, "pod-1", "delivery/v1")
	bare.Key.Attempt = deliveryValue(t, domain.NewAttemptReference, "attempt-bare")
	bare.Delivery = formedWithoutGateReading(t, "attempt-bare")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, bare)
		return err
	}); err == nil {
		t.Fatal("没带读数的那一版落册了")
	}

	if err := insert("attempt-backfill", "UNDECIDED", backfill); err != nil {
		t.Fatalf("存量回填那一形进不了库：%v", err)
	}
	key := deliveryKeyFixture(t, "tenant-1")
	key.Attempt = deliveryValue(t, domain.NewAttemptReference, "attempt-backfill")
	found, exists, err := repository.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("读回回填行：%v exists=%v", err, exists)
	}
	reading, _ := found.Delivery.GateReading()
	if stop, _ := reading.Stop(); reading.Answer() != domain.GateUndecided || stop != domain.DeliveryRecordedBeforeGateReading {
		t.Fatalf("回填行读数 = %s %s", reading.Answer(), stop)
	}

	if err := insert("attempt-mismatch", "BLOCKED", backfill); err != nil {
		t.Fatalf("整格与明细对不上的一行（库面不核，领域重建门核）：%v", err)
	}
	key.Attempt = deliveryValue(t, domain.NewAttemptReference, "attempt-mismatch")
	if _, _, err := repository.FindByKey(ctx, key); !errors.Is(err, domain.ErrInvalidDeliveryGateReading) {
		t.Fatalf("整格与停点对不上的一行装回了：%v", err)
	}
}

// formedWithoutGateReading 交回 FormEffectiveDelivery 刚形成、编排还没挂读数的那一份。
func formedWithoutGateReading(t *testing.T, attempt string) domain.EffectiveDelivery {
	t.Helper()
	object := deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1")
	formed, err := domain.FormFulfillmentAttempt(domain.FulfillmentAttemptSpec{
		TenantID:    deliveryValue(t, domain.NewTenantID, "tenant-1"),
		Attempt:     deliveryValue(t, domain.NewAttemptReference, attempt),
		Task:        deliveryValue(t, domain.NewDispatchTaskReference, "task-1"),
		ExecutedBy:  deliveryValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Place:       deliveryValue(t, domain.NewAttemptPlaceReference, "door-1"),
		PlannedFrom: deliveredAtFixture.Add(-time.Hour),
		PlannedTo:   deliveredAtFixture.Add(time.Hour),
		ArrivedAt:   deliveredAtFixture,
		Objects:     []domain.CarriedObjectReference{object},
		Evidence:    deliveryValue(t, domain.NewAttemptEvidenceReference, "evidence-1"),
	})
	if err != nil {
		t.Fatalf("尝试：%v", err)
	}
	result, err := domain.FormDeliveryAttemptResult(formed, object, domain.ObjectDelivered, domain.AttemptResultBasisReference{}, deliveredAtFixture)
	if err != nil {
		t.Fatalf("对象结果：%v", err)
	}
	delivery, err := domain.FormEffectiveDelivery(formed, result, domain.EffectiveDeliverySpec{
		Method:    deliveryValue(t, domain.NewDeliveryMethodReference, "signature"),
		Recipient: deliveryValue(t, domain.NewReceivingPartyReference, "recipient-1"),
		Proof:     deliveryValue(t, domain.NewDeliveryProofReference, "pod-1"),
		Version:   deliveryValue(t, domain.NewDeliveryResultVersion, "delivery/v1"),
	})
	if err != nil {
		t.Fatalf("交付：%v", err)
	}
	return delivery
}

// Covers: `AT-TF-265` 的库面半边——迁移 0043 的存量回填：迁移之前登下的有效交付（含更正翻成历史的前版）一律回填为未决「登记于门禁读数之前」，读回
// 成形。回填语句取迁移文件原文，不在这里另抄一份。
func TestTheMigrationBackfillsEarlierDeliveriesAsRecordedBeforeTheGateReading(t *testing.T) {
	repository, _, pool := newEffectiveDeliveries(t)
	ctx := t.Context()

	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	var backfill string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0043_") {
			start := strings.Index(asset.SQL, "UPDATE transport_fulfillment.effective_delivery")
			if start < 0 {
				t.Fatal("迁移 0043 里找不到回填那一句")
			}
			end := strings.Index(asset.SQL[start:], ";")
			if end < 0 {
				t.Fatal("迁移 0043 的回填那一句没有收尾")
			}
			backfill = asset.SQL[start : start+end]
		}
	}
	if backfill == "" {
		t.Fatal("没有迁移 0043")
	}

	for _, statement := range []string{
		`ALTER TABLE transport_fulfillment.effective_delivery ALTER COLUMN gate_reading DROP NOT NULL`,
		`ALTER TABLE transport_fulfillment.effective_delivery ALTER COLUMN gate_reading_detail DROP NOT NULL`,
		`ALTER TABLE transport_fulfillment.effective_delivery DROP CONSTRAINT effective_delivery_gate_reading_detail_shape`,
		`INSERT INTO transport_fulfillment.effective_delivery
		     (tenant_id, object_ref, attempt_ref, delivery_version, place_ref, method_ref, recipient_ref, proof_ref,
		      corrects_version, corrected_at, occurred_at, content_digest, is_current)
		 VALUES ('tenant-1', 'parcel-1', 'attempt-1', 'delivery/v1', 'door-1', 'signature', 'recipient-1', 'pod-1',
		         NULL, NULL, now() - interval '2 hours', 'digest-1', false),
		        ('tenant-1', 'parcel-1', 'attempt-1', 'delivery/v2', 'door-1', 'signature', 'recipient-1', 'pod-2',
		         'delivery/v1', now() - interval '1 hour', now() - interval '2 hours', 'digest-1', true)`,
		backfill,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("执行 %q：%v", statement, err)
		}
	}

	key := deliveryKeyFixture(t, "tenant-1")
	for _, version := range []string{"delivery/v1", "delivery/v2"} {
		found, exists, err := repository.FindByKeyAndVersion(ctx, key, deliveryValue(t, domain.NewDeliveryResultVersion, version))
		if err != nil || !exists {
			t.Fatalf("回填后的 %s 装不回：%v exists=%v", version, err, exists)
		}
		reading, read := found.Delivery.GateReading()
		if stop, _ := reading.Stop(); !read || reading.Answer() != domain.GateUndecided || stop != domain.DeliveryRecordedBeforeGateReading {
			t.Fatalf("%s 回填读数 = %s %s", version, reading.Answer(), stop)
		}
	}
}

// Covers: `AT-TF-256`、`AT-TF-262` 的库面半边——交付门禁读数找进段交接的那一口：按（租户、对象、版本）交回`已交接`的各版，
// 同一版本号在两个交接范围各一版时都交回，由领域认；拒收不交回，别的对象、别的租户不交回。
func TestTheEntryHandoverIsFoundByVersionAcrossScopes(t *testing.T) {
	repository, transactor, _ := newTransportHandovers(t)
	ctx := t.Context()
	for _, record := range []ports.TransportHandoverRecord{
		scopedRecord(t, "tenant-1", "parcel-1", "scope-1", "HRV-1"),
		scopedRecord(t, "tenant-1", "parcel-1", "scope-2", "HRV-1"),
		scopedRecord(t, "tenant-1", "parcel-2", "scope-1", "HRV-1"),
		scopedRecord(t, "tenant-2", "parcel-1", "scope-1", "HRV-1"),
		refusedScopedRecord(t, "scope-3", "HRV-1"),
	} {
		mustSaveHandover(t, transactor, ctx, repository, record)
	}

	found, err := repository.FindHandedOverByVersion(ctx,
		handoverValue(t, domain.NewTenantID, "tenant-1"),
		handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		handoverValue(t, domain.NewHandoverResultVersion, "HRV-1"))
	if err != nil {
		t.Fatalf("按版本找：%v", err)
	}
	if len(found) != 2 || found[0].Scope().String() != "scope-1" || found[1].Scope().String() != "scope-2" {
		t.Fatalf("找回 %d 版：%+v", len(found), found)
	}
	for _, handover := range found {
		if !handover.TransfersControl() {
			t.Fatalf("交回了不是`已交接`的一版：%s", handover.Verdict())
		}
		if _, read := handover.GateReading(); !read {
			t.Fatal("交回的`已交接`没装回交出门禁读数")
		}
	}

	none, err := repository.FindHandedOverByVersion(ctx,
		handoverValue(t, domain.NewTenantID, "tenant-1"),
		handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		handoverValue(t, domain.NewHandoverResultVersion, "HRV-NONE"))
	if err != nil || len(none) != 0 {
		t.Fatalf("不在册的版本：%v %d", err, len(none))
	}
}

func refusedScopedRecord(t *testing.T, scope, version string) ports.TransportHandoverRecord {
	t.Helper()
	handover, err := domain.FormTransportHandover(domain.TransportHandoverSpec{
		TenantID:   handoverValue(t, domain.NewTenantID, "tenant-1"),
		Object:     handoverValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Scope:      handoverValue(t, domain.NewHandoverScopeReference, scope),
		ReleasedBy: handoverValue(t, domain.NewHandoverPartyReference, "node-1"),
		ReceivedBy: handoverValue(t, domain.NewHandoverPartyReference, "carrier-1"),
		Verdict:    domain.HandoverRefused,
		Basis:      handoverValue(t, domain.NewHandoverBasisReference, "seal-broken"),
		Version:    handoverValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:   handoverJudgedAtFixture,
	})
	if err != nil {
		t.Fatalf("拒收夹具：%v", err)
	}
	return ports.TransportHandoverRecord{
		Key: ports.TransportHandoverKey{
			TenantID: handover.TenantID(), Object: handover.Object(), Scope: handover.Scope(), Version: handover.Version(),
		},
		ContentDigest: "digest-refused-" + version,
		Handover:      handover,
		RecordedAt:    handoverJudgedAtFixture,
	}
}
