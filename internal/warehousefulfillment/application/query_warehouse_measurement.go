package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// OwnerDeclaredMeasurementSource 是未仓测时附带货主声明尺寸重量的来源词（AT-WF-023）。
const OwnerDeclaredMeasurementSource = "OWNER_DECLARED"

// GoodsLevelMeasurementReading 是一个（货主，商品，包装层级）的仓测查阅结果。
// Found 为假表示商品没登记、属于别的货主，或该包装层级未在这个商品的当前修订上声明。
// Measured 为假且货主声明过尺寸重量时，Declared 非空并带来源词；两样都没有时 Current 与 Declared 皆为空。
type GoodsLevelMeasurementReading struct {
	Found    bool
	Measured bool
	Current  *ports.WarehouseMeasurementRow
	Declared *ports.GoodsDeclaredReferenceRow
	Versions []ports.WarehouseMeasurementRow
}

// CartonMeasurementReading 是一箱在一个仓的仓测查阅结果。还没测过时 Measured 为假、Current 为空、Versions 为空。
type CartonMeasurementReading struct {
	Measured bool
	Current  *ports.WarehouseMeasurementRow
	Versions []ports.WarehouseMeasurementRow
}

// QueryWarehouseMeasurementsHandler 查商品某一包装层级与在库一箱的仓测各版和当前那一版。
// 读不动原样上抛，不折成未测量。
type QueryWarehouseMeasurementsHandler struct {
	goods        ports.GoodsCurrentRead
	measurements ports.WarehouseMeasurementRead
}

func NewQueryWarehouseMeasurementsHandler(
	goods ports.GoodsCurrentRead,
	measurements ports.WarehouseMeasurementRead,
) *QueryWarehouseMeasurementsHandler {
	return &QueryWarehouseMeasurementsHandler{goods: goods, measurements: measurements}
}

// GoodsLevelMeasurements 查一个商品某一包装层级的仓测各版与当前那一版。
// 找不到商品或包装层级未声明时 Found 为假，不泄露别的货主的数据。
func (handler *QueryWarehouseMeasurementsHandler) GoodsLevelMeasurements(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
) (GoodsLevelMeasurementReading, error) {
	row, found, err := handler.goods.GetCurrentGoods(ctx, tenant, owner, goods)
	if err != nil {
		return GoodsLevelMeasurementReading{}, fmt.Errorf("query goods measurements: %w", err)
	}
	if !found || !hasPackagingLevel(row, level.String()) {
		return GoodsLevelMeasurementReading{Found: false}, nil
	}
	recorded, err := handler.measurements.ListForGoodsLevel(ctx, tenant, owner, goods, level)
	if err != nil {
		return GoodsLevelMeasurementReading{}, fmt.Errorf("query goods measurements: %w", err)
	}
	ordered := domain.SortWarehouseMeasurements(recorded)
	versions := warehouseMeasurementRowsOf(ordered)
	current, hasCurrent := domain.CurrentWarehouseMeasurement(ordered)
	if hasCurrent {
		curRow := warehouseMeasurementRowOf(current)
		return GoodsLevelMeasurementReading{
			Found: true, Measured: true, Current: &curRow, Versions: versions,
		}, nil
	}
	return GoodsLevelMeasurementReading{
		Found:    true,
		Measured: false,
		Declared: declaredReferenceForLevel(row.DeclaredMeasurements, level.String()),
		Versions: versions,
	}, nil
}

// CartonMeasurements 查一箱在一个仓的仓测各版与当前那一版。
func (handler *QueryWarehouseMeasurementsHandler) CartonMeasurements(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	carton domain.CartonNumber,
) (CartonMeasurementReading, error) {
	recorded, err := handler.measurements.ListForCarton(ctx, tenant, owner, warehouse, carton)
	if err != nil {
		return CartonMeasurementReading{}, fmt.Errorf("query carton measurements: %w", err)
	}
	ordered := domain.SortWarehouseMeasurements(recorded)
	versions := warehouseMeasurementRowsOf(ordered)
	current, hasCurrent := domain.CurrentWarehouseMeasurement(ordered)
	if !hasCurrent {
		return CartonMeasurementReading{Measured: false, Versions: versions}, nil
	}
	curRow := warehouseMeasurementRowOf(current)
	return CartonMeasurementReading{
		Measured: true, Current: &curRow, Versions: versions,
	}, nil
}

func hasPackagingLevel(row ports.GoodsRow, level string) bool {
	for _, item := range row.PackagingLevels {
		if item.Name == level {
			return true
		}
	}
	return false
}

func declaredReferenceForLevel(declared []ports.GoodsMeasurementRow, level string) *ports.GoodsDeclaredReferenceRow {
	for _, item := range declared {
		if item.Level != level {
			continue
		}
		ref := &ports.GoodsDeclaredReferenceRow{Source: OwnerDeclaredMeasurementSource}
		if item.Dimensions != nil {
			copyDims := *item.Dimensions
			ref.Dimensions = &copyDims
		}
		if item.Weight != nil {
			copyWeight := *item.Weight
			ref.Weight = &copyWeight
		}
		return ref
	}
	return nil
}

func warehouseMeasurementRowsOf(items []domain.WarehouseMeasurement) []ports.WarehouseMeasurementRow {
	rows := make([]ports.WarehouseMeasurementRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, warehouseMeasurementRowOf(item))
	}
	return rows
}

func warehouseMeasurementRowOf(item domain.WarehouseMeasurement) ports.WarehouseMeasurementRow {
	dims := item.Dimensions()
	weight := item.Weight()
	return ports.WarehouseMeasurementRow{
		FactID:         item.FactID().String(),
		WarehouseCode:  item.Warehouse().String(),
		Device:         item.Device().String(),
		Operator:       item.Operator().String(),
		OccurredAt:     item.OccurredAt(),
		DeviceSequence: item.DeviceSequence(),
		ReceivedAt:     item.ReceivedAt(),
		Dimensions: ports.GoodsDimensionsRow{
			Length: dims.Length().String(),
			Width:  dims.Width().String(),
			Height: dims.Height().String(),
			Unit:   dims.Unit().String(),
		},
		Weight: ports.GoodsWeightRow{
			Value: weight.Value().String(),
			Unit:  weight.Unit().String(),
		},
	}
}

// buildGoodsLevelMeasurements 按商品当前修订声明的包装层级逐层组装仓测视图（AT-WF-023）。
// 有仓测的带按发生次序的当前那一版；没有仓测的如实答未测量，货主声明过的另附来源标注。
func buildGoodsLevelMeasurements(
	row ports.GoodsRow,
	recorded []domain.WarehouseMeasurement,
) []ports.GoodsLevelMeasurementRow {
	byLevel := make(map[string][]domain.WarehouseMeasurement)
	for _, item := range recorded {
		if item.Target().Kind() != domain.MeasurementTargetGoodsLevel {
			continue
		}
		level := item.Target().Level().String()
		byLevel[level] = append(byLevel[level], item)
	}
	result := make([]ports.GoodsLevelMeasurementRow, 0, len(row.PackagingLevels))
	for _, level := range row.PackagingLevels {
		entry := ports.GoodsLevelMeasurementRow{Level: level.Name}
		if current, has := domain.CurrentWarehouseMeasurement(byLevel[level.Name]); has {
			curRow := warehouseMeasurementRowOf(current)
			entry.Measured = true
			entry.Current = &curRow
		} else {
			entry.Declared = declaredReferenceForLevel(row.DeclaredMeasurements, level.Name)
		}
		result = append(result, entry)
	}
	return result
}
