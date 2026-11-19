package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// rawMeasurementOccurredAt 是 writeRawMeasurement 写下的那一行的发生时间，与 savedMeasurement 的取法一致。
var rawMeasurementOccurredAt = time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)

// writeRawMeasurement 在一个隔离库里绕开领域直接写一行测量，只有重量数值与摘要由用例给。其余列与 savedMeasurement 同款：
// 对象 unit-1、重量单位 KG、尺寸 20×15×10 CM，所以摘要对得上的那一行就用同款测量形成时的摘要。
func writeRawMeasurement(t *testing.T, sourceID, weightValue, digest string) (*bentopg.DB, ports.ActualMeasurementKey) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	err = db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		if _, err = executor.Exec(txCtx, `INSERT INTO node_operations.measurement_source_identity (tenant_id, source_id, kind)
			VALUES ('tenant-a', $1, 'MEASUREMENT')`, sourceID); err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `INSERT INTO node_operations.actual_measurement
			(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
			 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
			 performed_by, evidence, occurred_at, recorded_at)
			VALUES ('tenant-a', $1, $2, 'HANDLING_UNIT', 'unit-1', 'node-origin', 'bench-2', 'intake-1',
			        $3, 'KG', '20', '15', '10', 'CM', 'scale-7', 'WEIGH/7', $4, $4)`,
			sourceID, digest, weightValue, rawMeasurementOccurredAt)
		return err
	})
	if err != nil {
		t.Fatalf("绕开领域写 %s：%v", sourceID, err)
	}
	return db, ports.ActualMeasurementKey{TenantID: ref(t, domain.NewTenantID, "tenant-a"), SourceID: sourceID}
}

func genuineMeasurementDigest(t *testing.T, sourceID string) string {
	t.Helper()
	return savedMeasurement(t, sourceID, "2.10", [3]string{"20", "15", "10"}, rawMeasurementOccurredAt).ContentDigest
}

// Covers: 读回时按行上的值重建测量并重算摘要比对。绕开领域直接写进来的行，重量不是正的十进制数，或摘要与行上的值对不上，
// 按来源身份读与按对象列仍有效清单都响亮报错，不把它当一条普通测量交出去；形状与摘要都对的同款行读得回来，证明夹具本身没毛病。
// 每一格各用一个隔离库：坏行会让它所在对象的整张清单读不出，不能互相牵连。夹具为隔离合成，只记 `S`。
func TestARowWrittenPastTheDomainDoesNotReadBackAsAMeasurement(t *testing.T) {
	ctx := context.Background()
	open := func(t *testing.T, db *bentopg.DB) *adapter.ActualMeasurements {
		t.Helper()
		measurements, err := adapter.NewActualMeasurements(db)
		if err != nil {
			t.Fatalf("构造测量登记册：%v", err)
		}
		return measurements
	}

	t.Run("形状与摘要都对的行读得回来", func(t *testing.T) {
		db, key := writeRawMeasurement(t, "raw-genuine", "2.10", genuineMeasurementDigest(t, "raw-genuine"))
		measurements := open(t, db)
		if _, present, err := measurements.FindMeasurement(ctx, key); err != nil || !present {
			t.Fatalf("按来源身份读 = present %v, err %v，want 读得回来", present, err)
		}
		listed, err := measurements.ListStillValid(ctx, key.TenantID, domain.HandlingUnitObject, "unit-1")
		if err != nil || len(listed) != 1 {
			t.Fatalf("仍有效清单 = %d 条, err %v，want 1 条", len(listed), err)
		}
	})

	for name, row := range map[string]struct{ weight, digest string }{
		"重量不是数":  {"2.1x", ""},
		"重量是零":   {"0", ""},
		"重量带正号":  {"+2.10", ""},
		"摘要对不上":  {"2.10", "NOC-1:00"},
		"重量写法不同": {"2.100", ""},
	} {
		t.Run(name, func(t *testing.T) {
			digest := row.digest
			if digest == "" {
				// 摘要取「重量是 2.10」那一条的：行上的值与它对不上，错就错在行上的值而不在摘要。
				digest = genuineMeasurementDigest(t, "raw-bad")
			}
			db, key := writeRawMeasurement(t, "raw-bad", row.weight, digest)
			measurements := open(t, db)
			if found, present, err := measurements.FindMeasurement(ctx, key); err == nil || present {
				t.Fatalf("按来源身份读 = %+v present %v err %v，want 报错", found, present, err)
			}
			if listed, err := measurements.ListStillValid(ctx, key.TenantID, domain.HandlingUnitObject, "unit-1"); err == nil {
				t.Fatalf("仍有效清单 = %d 条，want 报错", len(listed))
			}
		})
	}
}

// Covers: 查阅册上列的测量行经两个值对象重建：绕开领域写进来、重量不是正的十进制数的行，查阅册整页报错，不照抄给查阅人。
// 这与按来源身份读、仍有效清单同一条规矩；形状对的行照常列出。夹具为隔离合成，只记 `S`。
func TestTheMeasurementCatalogueDoesNotTranscribeARowWrittenPastTheDomain(t *testing.T) {
	ctx := context.Background()
	for name, weight := range map[string]string{"重量是零": "0", "重量不是数": "2.1x"} {
		t.Run(name, func(t *testing.T) {
			db, key := writeRawMeasurement(t, "raw-bad", weight, genuineMeasurementDigest(t, "raw-bad"))
			catalogue, err := adapter.NewReviewCatalogue(db)
			if err != nil {
				t.Fatalf("构造查阅册：%v", err)
			}
			page, err := catalogue.ListActualMeasurements(ctx, key.TenantID, 10, firstPage(t, ports.ActualMeasurementPages))
			if err == nil {
				t.Fatalf("查阅册列出 %d 行，want 报错", len(page.Rows))
			}
		})
	}
	t.Run("形状对的行照常列出", func(t *testing.T) {
		db, key := writeRawMeasurement(t, "raw-genuine", "2.10", genuineMeasurementDigest(t, "raw-genuine"))
		catalogue, err := adapter.NewReviewCatalogue(db)
		if err != nil {
			t.Fatalf("构造查阅册：%v", err)
		}
		page, err := catalogue.ListActualMeasurements(ctx, key.TenantID, 10, firstPage(t, ports.ActualMeasurementPages))
		if err != nil || len(page.Rows) != 1 || page.Rows[0].SourceID != "raw-genuine" {
			t.Fatalf("查阅册 = %+v, err %v，want 列出那一行", page.Rows, err)
		}
	})
}
