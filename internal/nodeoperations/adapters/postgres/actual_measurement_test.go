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
	gross, err := domain.NewGrossWeight(weight, "KG")
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	outer, err := domain.NewOuterDimensions(sides[0], sides[1], sides[2], "CM")
	if err != nil {
		t.Fatalf("外廓尺寸：%v", err)
	}
	measurement, err := domain.NewActualMeasurement(domain.ActualMeasurementSpec{
		Source: source, ObjectKind: domain.HandlingUnitObject, ObjectID: "unit-1",
		Node: ref(t, domain.NewNodeReference, "node-origin"), Location: ref(t, domain.NewWorkLocationReference, "bench-2"),
		Basis:  ref(t, domain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
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
	if weight := still[0].Measurement.Weight(); weight.Value() != "2.40" || weight.Unit() != "KG" {
		t.Fatalf("留下的重量 = %s %s", weight.Value(), weight.Unit())
	}
	if size := still[0].Measurement.Dimensions(); size.Length() != "20" || size.Width() != "15" || size.Height() != "10" || size.Unit() != "CM" {
		t.Fatalf("留下的尺寸 = %s×%s×%s %s，重量与尺寸要随同一行读回", size.Length(), size.Width(), size.Height(), size.Unit())
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
		remeasured.Weight.Value() != "2.10" || remeasured.Weight.Unit() != "KG" ||
		remeasured.Dimensions.Length() != "30" || remeasured.Dimensions.Width() != "25" || remeasured.Dimensions.Height() != "20" || remeasured.Dimensions.Unit() != "CM" {
		t.Fatalf("复测行 = %+v，want 一行上同时有重量与尺寸", remeasured)
	}
	if first.SourceID != "cat-first" || !first.Voided ||
		first.Weight.Value() != "2.10" || first.Dimensions.Length() != "20" || first.Dimensions.Width() != "15" || first.Dimensions.Height() != "10" {
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
			// 事实行要先在来源身份登记表占位才写得进；这条用例证的是尺寸必填，占位只是让齐全的那一行不被登记表拦下。
			if _, err = executor.Exec(txCtx, `INSERT INTO node_operations.measurement_source_identity (tenant_id, source_id, kind)
				VALUES ('tenant-a', $1, 'MEASUREMENT')`, sourceID); err != nil {
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

// savedInvalidation 是一条作废：指回 target，来源与发生时间由用例给。
func savedInvalidation(t *testing.T, sourceID, target string, occurred time.Time) ports.MeasurementInvalidationRecord {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		ref(t, domain.NewPerformingPartyReference, "scale-7"),
		ref(t, domain.NewExecutionEvidenceReference, "WEIGH/8"),
		occurred,
	)
	if err != nil {
		t.Fatalf("作废来源：%v", err)
	}
	invalidation, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: target, Reason: "称错件",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	return ports.MeasurementInvalidationRecord{
		Key:           ports.ActualMeasurementKey{TenantID: ref(t, domain.NewTenantID, "tenant-a"), SourceID: sourceID},
		Invalidation:  invalidation,
		ContentDigest: invalidation.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
}

// Covers: 发生时间带纳秒的称重量方，登记之后读得回来。库列只存到微秒，读回时用行上的值重算摘要并与存下的比对；形成处不截到
// 微秒，这一条登记成功后就再也读不回来，同一件实物上其余测量的仍有效清单也一并读不出。
func TestAMeasurementWithNanosecondsReadsBackAndIsListedAsStillValid(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	ctx := context.Background()
	occurred := time.Date(2026, 11, 17, 2, 0, 0, 123456789, time.UTC)
	nanos := savedMeasurement(t, "nanos-1", "2.10", [3]string{"20", "15", "10"}, occurred)
	whole := savedMeasurement(t, "whole-1", "2.40", [3]string{"20", "15", "10"}, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ActualMeasurementRecord{nanos, whole} {
			outcome, err := measurements.SaveMeasurement(txCtx, record)
			if err != nil {
				return err
			}
			if outcome != ports.ActualMeasurementSaved {
				return fmt.Errorf("%s 写入 %s", record.Key.SourceID, outcome)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记：%v", err)
	}

	// 三问各自成一格：读回、清单、重放分别失败时，输出里各自可见。
	t.Run("按来源身份读回", func(t *testing.T) {
		found, present, err := measurements.FindMeasurement(ctx, nanos.Key)
		if err != nil || !present {
			t.Fatalf("按来源身份读回 = present %v, err %v", present, err)
		}
		if found.ContentDigest != nanos.ContentDigest {
			t.Fatalf("读回的摘要 %s ≠ 形成时的 %s", found.ContentDigest, nanos.ContentDigest)
		}
		readBack := found.Measurement.Source().OccurredAt()
		if !readBack.Equal(nanos.Measurement.Source().OccurredAt()) || !readBack.Equal(occurred.Truncate(time.Microsecond)) {
			t.Fatalf("读回的发生时间 = %s，册上与形成时应是同一个截到微秒的时刻", readBack.Format(time.RFC3339Nano))
		}
	})

	t.Run("仍有效清单", func(t *testing.T) {
		still, err := measurements.ListStillValid(ctx, nanos.Key.TenantID, domain.HandlingUnitObject, "unit-1")
		if err != nil {
			t.Fatalf("仍有效清单：%v", err)
		}
		if len(still) != 2 || still[0].Key.SourceID != "nanos-1" || still[1].Key.SourceID != "whole-1" {
			t.Fatalf("仍有效清单 = %v，want 带纳秒的那条与整秒的那条都在", still)
		}
	})

	// 重放：同来源身份、同内容再报一次，落册答已登记，读回的摘要与这次形成的摘要相同，编排据此答已有结果而不是冲突。
	t.Run("重放", func(t *testing.T) {
		replay := savedMeasurement(t, "nanos-1", "2.10", [3]string{"20", "15", "10"}, occurred)
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			outcome, err := measurements.SaveMeasurement(txCtx, replay)
			if err != nil {
				return err
			}
			if outcome != ports.ActualMeasurementAlreadyRecorded {
				return fmt.Errorf("重放写入 %s", outcome)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("重放：%v", err)
		}
		again, present, err := measurements.FindMeasurement(ctx, nanos.Key)
		if err != nil || !present || again.ContentDigest != replay.ContentDigest {
			t.Fatalf("重放之后读回 = present %v, err %v, 摘要 %s ≠ 重放形成的 %s", present, err, again.ContentDigest, replay.ContentDigest)
		}
	})
}

// Covers: 发生时间带纳秒的作废，登记之后读得回来；被作废的那条（发生时间也带纳秒）离开仍有效清单，仍可按来源身份读到，
// 同一条作废再报一次答已登记。
func TestAnInvalidationWithNanosecondsReadsBackAndTakesItsTargetOutOfTheStillValidList(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	ctx := context.Background()
	target := savedMeasurement(t, "nanos-1", "2.10", [3]string{"20", "15", "10"}, time.Date(2026, 11, 17, 2, 0, 0, 123456789, time.UTC))
	kept := savedMeasurement(t, "whole-1", "2.40", [3]string{"20", "15", "10"}, time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	voidedAt := time.Date(2026, 11, 17, 4, 0, 0, 987654321, time.UTC)
	void := savedInvalidation(t, "void-1", "nanos-1", voidedAt)
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ActualMeasurementRecord{target, kept} {
			if _, err := measurements.SaveMeasurement(txCtx, record); err != nil {
				return err
			}
		}
		outcome, err := measurements.SaveInvalidation(txCtx, void)
		if err != nil {
			return err
		}
		if outcome != ports.MeasurementInvalidationSaved {
			return fmt.Errorf("作废写入 %s", outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记与作废：%v", err)
	}

	t.Run("作废读回", func(t *testing.T) {
		found, present, err := measurements.FindInvalidation(ctx, void.Key)
		if err != nil || !present {
			t.Fatalf("作废读回 = present %v, err %v", present, err)
		}
		if found.ContentDigest != void.ContentDigest {
			t.Fatalf("读回的作废摘要 %s ≠ 形成时的 %s", found.ContentDigest, void.ContentDigest)
		}
		readBack := found.Invalidation.Source().OccurredAt()
		if !readBack.Equal(void.Invalidation.Source().OccurredAt()) || !readBack.Equal(voidedAt.Truncate(time.Microsecond)) {
			t.Fatalf("读回的作废发生时间 = %s，册上与形成时应是同一个截到微秒的时刻", readBack.Format(time.RFC3339Nano))
		}
	})

	t.Run("被作废的那条仍读得到", func(t *testing.T) {
		if _, present, err := measurements.FindMeasurement(ctx, target.Key); err != nil || !present {
			t.Fatalf("被作废的那条仍应按来源身份读得到 = present %v, err %v", present, err)
		}
	})

	t.Run("仍有效清单", func(t *testing.T) {
		still, err := measurements.ListStillValid(ctx, target.Key.TenantID, domain.HandlingUnitObject, "unit-1")
		if err != nil {
			t.Fatalf("仍有效清单：%v", err)
		}
		if len(still) != 1 || still[0].Key.SourceID != "whole-1" {
			t.Fatalf("仍有效清单 = %v，want 只剩整秒的那条", still)
		}
	})

	t.Run("重放", func(t *testing.T) {
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			outcome, err := measurements.SaveInvalidation(txCtx, savedInvalidation(t, "void-1", "nanos-1", voidedAt))
			if err != nil {
				return err
			}
			if outcome != ports.MeasurementInvalidationAlreadyRecorded {
				return fmt.Errorf("重放写入 %s", outcome)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("作废重放：%v", err)
		}
	})
}
