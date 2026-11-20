package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RecordWarehouseMeasurementOutcome 是一次仓测登记的业务结果（UC-WF-002 结果语义契约），平铺。
// 未落册的那几种不占事实身份。
type RecordWarehouseMeasurementOutcome uint8

const (
	RecordMeasurementOutcomeInvalid RecordWarehouseMeasurementOutcome = iota
	RecordMeasurementRecorded
	RecordMeasurementExistingResult
	RecordMeasurementContentConflict
	RecordMeasurementWarehouseNotFound
	RecordMeasurementWarehouseNotAccepting
	RecordMeasurementNotAccepted
)

func (outcome RecordWarehouseMeasurementOutcome) String() string {
	switch outcome {
	case RecordMeasurementRecorded:
		return "RECORDED"
	case RecordMeasurementExistingResult:
		return "EXISTING_RESULT"
	case RecordMeasurementContentConflict:
		return "CONTENT_CONFLICT"
	case RecordMeasurementWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case RecordMeasurementWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case RecordMeasurementNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// RecordWarehouseMeasurementResult 携带已落册的仓测（RECORDED、EXISTING_RESULT）、仓不受理原因，或 NOT_ACCEPTED 上全部不合的格。
type RecordWarehouseMeasurementResult struct {
	outcome     RecordWarehouseMeasurementOutcome
	measurement domain.WarehouseMeasurement
	has         bool
	reason      WarehouseNotAcceptingReason
	fields      []domain.MeasurementField
}

func (result RecordWarehouseMeasurementResult) Outcome() RecordWarehouseMeasurementOutcome {
	return result.outcome
}

func (result RecordWarehouseMeasurementResult) Measurement() (domain.WarehouseMeasurement, bool) {
	return result.measurement, result.has
}

func (result RecordWarehouseMeasurementResult) NotAcceptingReason() (WarehouseNotAcceptingReason, bool) {
	return result.reason, result.outcome == RecordMeasurementWarehouseNotAccepting
}

func (result RecordWarehouseMeasurementResult) InvalidFields() []domain.MeasurementField {
	return slices.Clone(result.fields)
}

// RecordWarehouseMeasurementCommand 是一次仓测登记。租户、操作者与设备取自信封，不在载荷里。
type RecordWarehouseMeasurementCommand struct {
	Tenant         domain.TenantID
	FactID         domain.MeasurementFactID
	Device         domain.MeasurementDeviceID
	Operator       domain.MeasurementOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      domain.WarehouseCode
	Target         domain.MeasurementTarget
	Dimensions     domain.DeclaredDimensions
	Weight         domain.DeclaredWeight
}

func (command RecordWarehouseMeasurementCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.FactID.String() == "":
		return errors.New("缺事实身份")
	case command.Device.String() == "":
		return errors.New("缺设备")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	case command.OccurredAt.IsZero():
		return errors.New("缺发生时刻")
	case command.DeviceSequence < 1:
		return errors.New("设备序号须为正")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.Target.Kind() == domain.MeasurementTargetKindInvalid:
		return errors.New("缺测量对象")
	case command.Dimensions.Unit().String() == "":
		return errors.New("缺尺寸")
	case command.Weight.Unit().String() == "":
		return errors.New("缺重量")
	default:
		return nil
	}
}

func (command RecordWarehouseMeasurementCommand) submission() domain.WarehouseMeasurementSubmission {
	return domain.WarehouseMeasurementSubmission{
		Device:         command.Device,
		Operator:       command.Operator,
		OccurredAt:     command.OccurredAt,
		DeviceSequence: command.DeviceSequence,
		Warehouse:      command.Warehouse,
		Target:         command.Target,
		Dimensions:     command.Dimensions,
		Weight:         command.Weight,
	}
}

// RecordWarehouseMeasurementHandler 把一次仓测尺寸重量追加落册（UC-WF-002）。
// 事务边界在调用方：HoldFact 与 Save 在没有环境事务时拒绝。
type RecordWarehouseMeasurementHandler struct {
	measurements ports.WarehouseMeasurementStore
	warehouses   ports.WarehouseLookup
	goods        ports.StockGoodsLookup
	cartons      ports.CartonOnHandLookup
	clock        ports.Clock
}

func NewRecordWarehouseMeasurementHandler(
	measurements ports.WarehouseMeasurementStore,
	warehouses ports.WarehouseLookup,
	goods ports.StockGoodsLookup,
	cartons ports.CartonOnHandLookup,
	clock ports.Clock,
) *RecordWarehouseMeasurementHandler {
	return &RecordWarehouseMeasurementHandler{
		measurements: measurements,
		warehouses:   warehouses,
		goods:        goods,
		cartons:      cartons,
		clock:        clock,
	}
}

// Record 执行仓测登记。判定次序（UC-WF-002「应用流程」与「一致性、幂等与并发」）：
//  1. 装配校验：缺件交 error，不是业务答复。
//  2. 先拿事实身份锁再按事实身份认重放：同内容答 EXISTING_RESULT，异内容答 CONTENT_CONFLICT、在册的不变。
//  3. 核指名的仓在本租户已登记且是自营仓：没登记（含本租户一个仓都没登记）答 WAREHOUSE_NOT_FOUND；
//     代理仓答 WAREHOUSE_NOT_ACCEPTING；停用的自营仓照记。未通过的不占事实身份。
//  4. 核对象、数值与单位，一次收齐全部不合的格，任一处不合答 NOT_ACCEPTED 并逐格指出，不占事实身份：
//     - 按（商品，包装层级）：读落册这一刻商品的当前修订，没登记指 goodsCode，包装层级未声明指 packagingLevel；
//     - 按箱：在事实到达服务端这一刻核该箱在这个货主这个仓的整箱账上是否在库，不在库指 cartonNumber；
//     - 长、宽、高、重量须为正，尺寸单位须是内置长度类目录码，重量单位须是内置重量类目录码。
//  5. 追加落册，答 RECORDED；当前那一版按发生时间与设备序号定序，迟到的旧事实不覆盖更晚发生的那一版。
func (handler *RecordWarehouseMeasurementHandler) Record(
	ctx context.Context,
	command RecordWarehouseMeasurementCommand,
) (RecordWarehouseMeasurementResult, error) {
	if err := command.assemblyError(); err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	if err := handler.measurements.HoldFact(ctx, command.Tenant, command.FactID); err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	existing, found, err := handler.measurements.LoadByFact(ctx, command.Tenant, command.FactID)
	if err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	if found {
		if existing.SameContent(command.submission()) {
			return RecordWarehouseMeasurementResult{
				outcome: RecordMeasurementExistingResult, measurement: existing, has: true,
			}, nil
		}
		return RecordWarehouseMeasurementResult{outcome: RecordMeasurementContentConflict}, nil
	}

	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	if !found {
		return RecordWarehouseMeasurementResult{outcome: RecordMeasurementWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return RecordWarehouseMeasurementResult{
			outcome: RecordMeasurementWarehouseNotAccepting,
			reason:  WarehouseNotAcceptingAgent,
		}, nil
	}

	fields, err := handler.screen(ctx, command)
	if err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	if len(fields) > 0 {
		return RecordWarehouseMeasurementResult{
			outcome: RecordMeasurementNotAccepted,
			fields:  fields,
		}, nil
	}

	recorded, err := domain.NewWarehouseMeasurement(domain.WarehouseMeasurementSpec{
		Tenant:         command.Tenant,
		FactID:         command.FactID,
		Device:         command.Device,
		Operator:       command.Operator,
		OccurredAt:     command.OccurredAt,
		DeviceSequence: command.DeviceSequence,
		ReceivedAt:     handler.clock.Now(),
		Warehouse:      command.Warehouse,
		Target:         command.Target,
		Dimensions:     command.Dimensions,
		Weight:         command.Weight,
	})
	if err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	inserted, err := handler.measurements.Save(ctx, recorded)
	if err != nil {
		return RecordWarehouseMeasurementResult{}, fmt.Errorf("record warehouse measurement: %w", err)
	}
	if !inserted {
		return RecordWarehouseMeasurementResult{}, errors.New("record warehouse measurement: 事实身份在锁内仍被占")
	}
	return RecordWarehouseMeasurementResult{
		outcome: RecordMeasurementRecorded, measurement: recorded, has: true,
	}, nil
}

func (handler *RecordWarehouseMeasurementHandler) screen(
	ctx context.Context,
	command RecordWarehouseMeasurementCommand,
) ([]domain.MeasurementField, error) {
	var fields []domain.MeasurementField
	switch command.Target.Kind() {
	case domain.MeasurementTargetGoodsLevel:
		revision, found, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Target.Owner(), command.Target.Goods())
		if err != nil {
			return nil, err
		}
		if !found {
			fields = append(fields, domain.MeasurementFieldGoodsCode)
		} else if _, declared := unitFactor(revision, command.Target.Level()); !declared {
			fields = append(fields, domain.MeasurementFieldPackagingLevel)
		}
	case domain.MeasurementTargetCarton:
		if handler.cartons == nil {
			return nil, errors.New("整箱在库读口没有接上")
		}
		onHand, err := handler.cartons.CartonOnHand(
			ctx, command.Tenant, command.Target.Owner(), command.Warehouse, command.Target.Carton(),
		)
		if err != nil {
			return nil, err
		}
		if !onHand {
			fields = append(fields, domain.MeasurementFieldCartonNumber)
		}
	default:
		return nil, errors.New("未知的测量对象种类")
	}

	if command.Dimensions.Length().IsZero() {
		fields = append(fields, domain.MeasurementFieldDimensionsLength)
	}
	if command.Dimensions.Width().IsZero() {
		fields = append(fields, domain.MeasurementFieldDimensionsWidth)
	}
	if command.Dimensions.Height().IsZero() {
		fields = append(fields, domain.MeasurementFieldDimensionsHeight)
	}
	if !isUnitOfCategory(command.Dimensions.Unit().String(), refdata.UnitCategoryLength) {
		fields = append(fields, domain.MeasurementFieldDimensionsUnit)
	}
	if command.Weight.Value().IsZero() {
		fields = append(fields, domain.MeasurementFieldWeightValue)
	}
	if !isUnitOfCategory(command.Weight.Unit().String(), refdata.UnitCategoryWeight) {
		fields = append(fields, domain.MeasurementFieldWeightUnit)
	}
	return fields, nil
}
