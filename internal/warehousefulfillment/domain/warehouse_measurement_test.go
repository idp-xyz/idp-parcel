package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func measurementFixture(
	t *testing.T,
	factID string,
	occurredAt time.Time,
	sequence int64,
	receivedAt time.Time,
	lengthText string,
) domain.WarehouseMeasurement {
	t.Helper()
	length, err := domain.NewMeasure(lengthText)
	if err != nil {
		t.Fatal(err)
	}
	width, err := domain.NewMeasure("20")
	if err != nil {
		t.Fatal(err)
	}
	height, err := domain.NewMeasure("10")
	if err != nil {
		t.Fatal(err)
	}
	cm, err := domain.NewUnitCode("CM")
	if err != nil {
		t.Fatal(err)
	}
	weightVal, err := domain.NewMeasure("1.25")
	if err != nil {
		t.Fatal(err)
	}
	kg, err := domain.NewUnitCode("KG")
	if err != nil {
		t.Fatal(err)
	}
	dimensions, err := domain.NewDeclaredDimensions(length, width, height, cm)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewDeclaredWeight(weightVal, kg)
	if err != nil {
		t.Fatal(err)
	}
	target, err := domain.NewGoodsLevelMeasurementTarget(
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		valueOf(t, domain.NewGoodsUnitName, "件"),
	)
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewWarehouseMeasurement(domain.WarehouseMeasurementSpec{
		Tenant:         valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		FactID:         valueOf(t, domain.NewMeasurementFactID, factID),
		Device:         valueOf(t, domain.NewMeasurementDeviceID, "SYN-DEVICE-01"),
		Operator:       valueOf(t, domain.NewMeasurementOperatorID, "https://id.syn.example/dex#SYN-OP-1"),
		OccurredAt:     occurredAt,
		DeviceSequence: sequence,
		ReceivedAt:     receivedAt,
		Warehouse:      valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Target:         target,
		Dimensions:     dimensions,
		Weight:         weight,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: AT-WF-021（仓测领域）——当前那一版按发生时间与设备序号定序，不按到达先后；迟到的旧仓测不顶掉更新发生的那一版。
func TestCurrentWarehouseMeasurementFollowsOccurrenceOrderNotArrivalOrder(t *testing.T) {
	base := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	newer := measurementFixture(t, "SYN-FACT-NEWER", base.Add(10*time.Minute), 1, base.Add(11*time.Minute), "30")
	lateOlder := measurementFixture(t, "SYN-FACT-LATE-OLD", base.Add(5*time.Minute), 99, base.Add(20*time.Minute), "28")
	sameTimeHigherSeq := measurementFixture(t, "SYN-FACT-SEQ-2", base.Add(10*time.Minute), 2, base.Add(12*time.Minute), "31")

	current, ok := domain.CurrentWarehouseMeasurement([]domain.WarehouseMeasurement{newer, lateOlder})
	if !ok || current.FactID().String() != "SYN-FACT-NEWER" {
		t.Fatalf("迟到旧事实后到达：%s (%v)，want SYN-FACT-NEWER", current.FactID(), ok)
	}
	current, ok = domain.CurrentWarehouseMeasurement([]domain.WarehouseMeasurement{newer, lateOlder, sameTimeHigherSeq})
	if !ok || current.FactID().String() != "SYN-FACT-SEQ-2" {
		t.Fatalf("同发生时刻按序号：%s (%v)，want SYN-FACT-SEQ-2", current.FactID(), ok)
	}
	ordered := domain.SortWarehouseMeasurements([]domain.WarehouseMeasurement{sameTimeHigherSeq, newer, lateOlder})
	if ordered[0].FactID().String() != "SYN-FACT-LATE-OLD" || ordered[1].FactID().String() != "SYN-FACT-NEWER" || ordered[2].FactID().String() != "SYN-FACT-SEQ-2" {
		t.Fatalf("排序次序 = [%s %s %s]", ordered[0].FactID(), ordered[1].FactID(), ordered[2].FactID())
	}
}

// Covers: AT-WF-021（仓测领域）——重放比对忽略接收时刻、按数值归一（30 与 30.00 相同）、按微秒归一时间；换数值判内容冲突。
func TestWarehouseMeasurementSameContentNormalizesMeasureAndIgnoresReceivedAt(t *testing.T) {
	base := time.Date(2026, 11, 20, 8, 0, 0, 123456789, time.UTC)
	recorded := measurementFixture(t, "SYN-FACT-1", base, 1, base.Add(time.Minute), "30")

	sameLength, _ := domain.NewMeasure("30.00")
	sameDims, _ := domain.NewDeclaredDimensions(sameLength, recorded.Dimensions().Width(), recorded.Dimensions().Height(), recorded.Dimensions().Unit())
	sameSubmission := domain.WarehouseMeasurementSubmission{
		Device: recorded.Device(), Operator: recorded.Operator(), OccurredAt: base, DeviceSequence: 1,
		Warehouse: recorded.Warehouse(), Target: recorded.Target(), Dimensions: sameDims, Weight: recorded.Weight(),
	}
	if !recorded.SameContent(sameSubmission) {
		t.Fatal("30 与 30.00 应算同一内容")
	}

	diffLength, _ := domain.NewMeasure("30.5")
	diffDims, _ := domain.NewDeclaredDimensions(diffLength, recorded.Dimensions().Width(), recorded.Dimensions().Height(), recorded.Dimensions().Unit())
	diffSubmission := sameSubmission
	diffSubmission.Dimensions = diffDims
	if recorded.SameContent(diffSubmission) {
		t.Fatal("长度变了却判成同一内容")
	}
}
