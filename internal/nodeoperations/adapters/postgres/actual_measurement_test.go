package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newActualMeasurements(t *testing.T) (*adapter.ActualMeasurements, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	measurements, err := adapter.NewActualMeasurements(db)
	if err != nil {
		t.Fatalf("构造测量登记册：%v", err)
	}
	return measurements, db.Transactor()
}

// savedWeight 是一条完整的测量：毛重由用例给，尺寸 20×15×10 CM，发生在两点。
func savedWeight(t *testing.T, sourceID, value string) ports.ActualMeasurementRecord {
	t.Helper()
	return savedMeasurement(t, sourceID, value, [3]string{"20", "15", "10"}, time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
}

func savedMeasurement(t *testing.T, sourceID, weight string, sides [3]string, occurred time.Time) ports.ActualMeasurementRecord {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		ref(t, domain.NewPerformingPartyReference, "scale-7"),
		ref(t, domain.NewExecutionEvidenceReference, "WEIGH/7"),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	measurement, err := domain.NewActualMeasurement(domain.ActualMeasurementSpec{
		Source: source, ObjectKind: domain.HandlingUnitObject, ObjectID: "unit-1",
		Node: ref(t, domain.NewNodeReference, "node-origin"), Location: ref(t, domain.NewWorkLocationReference, "bench-2"),
		Basis:       ref(t, domain.NewWorkBasisReference, "intake-1"),
		WeightValue: weight, WeightUnit: "KG",
		Length: sides[0], Width: sides[1], Height: sides[2], DimensionUnit: "CM",
	})
	if err != nil {
		t.Fatalf("测量：%v", err)
	}
	tenant := ref(t, domain.NewTenantID, "tenant-a")
	return ports.ActualMeasurementRecord{
		Key:           ports.ActualMeasurementKey{TenantID: tenant, SourceID: sourceID},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
}

func TestActualMeasurementWritesRefuseToRunOutsideATransaction(t *testing.T) {
	measurements, _ := newActualMeasurements(t)
	record := savedWeight(t, "measure-1", "2.10")
	if _, err := measurements.SaveMeasurement(context.Background(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Save 应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := measurements.SaveInvalidation(context.Background(), ports.MeasurementInvalidationRecord{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务作废应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestAVoidedMeasurementLeavesTheStillValidList(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	ctx := context.Background()
	first := savedWeight(t, "measure-1", "2.10")
	second := savedWeight(t, "measure-2", "2.40")
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := measurements.SaveMeasurement(txCtx, first)
		if err != nil {
			return err
		}
		if outcome != ports.ActualMeasurementSaved {
			return fmt.Errorf("第一条 %s", outcome)
		}
		outcome, err = measurements.SaveMeasurement(txCtx, second)
		if err != nil {
			return err
		}
		if outcome != ports.ActualMeasurementSaved {
			return fmt.Errorf("第二条 %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记：%v", err)
	}

	occurred := time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)
	source, err := domain.NewWorkFactSource(
		"void-1",
		ref(t, domain.NewPerformingPartyReference, "scale-7"),
		ref(t, domain.NewExecutionEvidenceReference, "WEIGH/8"),
		occurred,
	)
	if err != nil {
		t.Fatalf("作废来源：%v", err)
	}
	invalidation, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: "measure-1", Reason: "称错件",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	voidRecord := ports.MeasurementInvalidationRecord{
		Key:           ports.ActualMeasurementKey{TenantID: first.Key.TenantID, SourceID: "void-1"},
		Invalidation:  invalidation,
		ContentDigest: invalidation.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := measurements.SaveInvalidation(txCtx, voidRecord)
		if err != nil {
			return err
		}
		if outcome != ports.MeasurementInvalidationSaved {
			return fmt.Errorf("作废写入 %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("作废事务：%v", err)
	}

	still, err := measurements.ListStillValid(ctx, first.Key.TenantID, domain.HandlingUnitObject, "unit-1")
	if err != nil {
		t.Fatalf("仍有效：%v", err)
	}
	if len(still) != 1 || still[0].Key.SourceID != "measure-2" {
		t.Fatalf("仍有效 = %v", still)
	}
	if weight, unit := still[0].Measurement.Weight(); weight != "2.40" || unit != "KG" {
		t.Fatalf("留下的重量 = %s %s", weight, unit)
	}
	if length, width, height, unit := still[0].Measurement.Dimensions(); length != "20" || width != "15" || height != "10" || unit != "CM" {
		t.Fatalf("留下的尺寸 = %s×%s×%s %s，重量与尺寸要随同一行读回", length, width, height, unit)
	}
	if !still[0].Measurement.Source().OccurredAt().Equal(second.Measurement.Source().OccurredAt()) {
		t.Fatal("发生时间被记录时刻顶替了")
	}
	if still[0].RecordedAt.Equal(still[0].Measurement.Source().OccurredAt()) {
		t.Fatal("记录时刻与发生时间合成了一列")
	}

	other := ref(t, domain.NewTenantID, "tenant-b")
	foreign, err := measurements.ListStillValid(ctx, other, domain.HandlingUnitObject, "unit-1")
	if err != nil {
		t.Fatalf("他租户：%v", err)
	}
	if len(foreign) != 0 {
		t.Fatalf("他租户看见了 %d 条", len(foreign))
	}
}

func TestTheMeasurementCatalogueKeepsVoidedRows(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	measurements, err := adapter.NewActualMeasurements(db)
	if err != nil {
		t.Fatalf("构造测量登记册：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅册：%v", err)
	}
	ctx := context.Background()
	// 先测一条，再复测一条：复测是另一条完整的测量，旧的那条随后作废。
	weight := savedWeight(t, "cat-first", "2.10")
	size := savedMeasurement(t, "cat-remeasure", "2.10", [3]string{"30", "25", "20"}, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := measurements.SaveMeasurement(txCtx, weight); err != nil {
			return err
		}
		_, err := measurements.SaveMeasurement(txCtx, size)
		return err
	})
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	occurred := time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)
	source, err := domain.NewWorkFactSource(
		"cat-void",
		ref(t, domain.NewPerformingPartyReference, "scale-7"),
		ref(t, domain.NewExecutionEvidenceReference, "WEIGH/void"),
		occurred,
	)
	if err != nil {
		t.Fatalf("作废来源：%v", err)
	}
	invalidation, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: "cat-first", Reason: "称错件",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := measurements.SaveInvalidation(txCtx, ports.MeasurementInvalidationRecord{
			Key:           ports.ActualMeasurementKey{TenantID: weight.Key.TenantID, SourceID: "cat-void"},
			Invalidation:  invalidation,
			ContentDigest: invalidation.ContentDigest(),
			RecordedAt:    occurred.Add(time.Minute),
		})
		return err
	})
	if err != nil {
		t.Fatalf("作废事务：%v", err)
	}

	page, err := catalogue.ListActualMeasurements(ctx, weight.Key.TenantID, 10, firstPage(t, ports.ActualMeasurementPages))
	if err != nil {
		t.Fatalf("全册：%v", err)
	}
	if page.Total != 2 || len(page.Rows) != 2 {
		t.Fatalf("全册 = total %d 行 %d", page.Total, len(page.Rows))
	}
	remeasured, first := page.Rows[0], page.Rows[1]
	if remeasured.SourceID != "cat-remeasure" || remeasured.Voided ||
		remeasured.WeightValue != "2.10" || remeasured.WeightUnit != "KG" ||
		remeasured.Length != "30" || remeasured.Width != "25" || remeasured.Height != "20" || remeasured.DimensionUnit != "CM" {
		t.Fatalf("复测行 = %+v，want 一行上同时有重量与尺寸", remeasured)
	}
	if first.SourceID != "cat-first" || !first.Voided ||
		first.WeightValue != "2.10" || first.Length != "20" || first.Width != "15" || first.Height != "10" {
		t.Fatalf("被作废的那一行 = %+v，want 数字原样留着", first)
	}

	voidedOnly, err := catalogue.ListActualMeasurements(ctx, weight.Key.TenantID, 10, pageQuery(t, ports.ActualMeasurementPages, url.Values{
		"voided": {"true"},
	}))
	if err != nil {
		t.Fatalf("筛选：%v", err)
	}
	if voidedOnly.Total != 1 || len(voidedOnly.Rows) != 1 || voidedOnly.Rows[0].SourceID != "cat-first" {
		t.Fatalf("已作废 = %+v total %d", voidedOnly.Rows, voidedOnly.Total)
	}

	other := ref(t, domain.NewTenantID, "tenant-b")
	foreign, err := catalogue.ListActualMeasurements(ctx, other, 10, firstPage(t, ports.ActualMeasurementPages))
	if err != nil {
		t.Fatalf("他租户：%v", err)
	}
	if foreign.Total != 0 || len(foreign.Rows) != 0 {
		t.Fatalf("他租户看见了 %d 条", foreign.Total)
	}
}

// Covers: 重量与尺寸在库上都是必填。绕过领域直接写一行只有重量的测量，表自己拒：缺列被非空约束拦，空白被检查约束拦。
func TestTheRegisterRefusesAMeasurementWithoutItsDimensions(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	insert := func(sourceID string, length any) error {
		return db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			executor, err := db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			_, err = executor.Exec(txCtx, `INSERT INTO node_operations.actual_measurement
				(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
				 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
				 performed_by, evidence, occurred_at, recorded_at)
				VALUES ('tenant-a', $1, 'digest', 'HANDLING_UNIT', 'unit-1', 'node-origin', 'bench-2', 'intake-1',
				        '2.10', 'KG', $2, '15', '10', 'CM', 'scale-7', 'WEIGH/7', now(), now())`, sourceID, length)
			return err
		})
	}
	var pgErr *pgconn.PgError
	if err := insert("raw-null", nil); !errors.As(err, &pgErr) || pgErr.Code != "23502" || pgErr.ColumnName != "length_value" {
		t.Fatalf("缺长度的那一行没被非空约束拒：%v", err)
	}
	if err := insert("raw-blank", " "); !errors.As(err, &pgErr) || pgErr.ConstraintName != "actual_measurement_weight_and_dimensions_present" {
		t.Fatalf("长度空白的那一行没被检查约束拒：%v", err)
	}
	if err := insert("raw-complete", "20"); err != nil {
		t.Fatalf("重量与尺寸齐全的那一行应当写得进：%v", err)
	}
}
