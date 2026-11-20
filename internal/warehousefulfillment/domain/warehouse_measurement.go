package domain

import (
	"errors"
	"slices"
	"time"
)

// ErrInvalidWarehouseMeasurement 是仓测事实缺了必填格、对象形状不自洽，或序号不为正。
var ErrInvalidWarehouseMeasurement = errors.New("warehouse fulfillment: invalid warehouse measurement")

// MeasurementFactID 是仓测作业事实的身份，由设备签发，本上下文不另签一个（ADR-0023）。
type MeasurementFactID struct{ requiredValue }

func NewMeasurementFactID(value string) (MeasurementFactID, error) {
	required, err := newRequiredValue("measurement fact ID", value)
	return MeasurementFactID{required}, err
}

// MeasurementDeviceID 是签发这条仓测事实的设备，逐字保存。
type MeasurementDeviceID struct{ requiredValue }

func NewMeasurementDeviceID(value string) (MeasurementDeviceID, error) {
	required, err := newRequiredValue("measurement device", value)
	return MeasurementDeviceID{required}, err
}

// MeasurementOperatorID 是登记这次仓测的操作者，逐字保存。
type MeasurementOperatorID struct{ requiredValue }

func NewMeasurementOperatorID(value string) (MeasurementOperatorID, error) {
	required, err := newRequiredValue("measurement operator", value)
	return MeasurementOperatorID{required}, err
}

// MeasurementTargetKind 是仓测指向的两种对象：按（商品，包装层级），或按在库的一箱。
type MeasurementTargetKind uint8

const (
	MeasurementTargetKindInvalid MeasurementTargetKind = iota
	MeasurementTargetGoodsLevel
	MeasurementTargetCarton
)

func (kind MeasurementTargetKind) String() string {
	switch kind {
	case MeasurementTargetGoodsLevel:
		return "GOODS_LEVEL"
	case MeasurementTargetCarton:
		return "CARTON"
	default:
		return ""
	}
}

func NewMeasurementTargetKind(word string) (MeasurementTargetKind, error) {
	switch word {
	case MeasurementTargetGoodsLevel.String():
		return MeasurementTargetGoodsLevel, nil
	case MeasurementTargetCarton.String():
		return MeasurementTargetCarton, nil
	default:
		return MeasurementTargetKindInvalid, ErrInvalidWarehouseMeasurement
	}
}

// MeasurementField 指出仓测未受理时差在哪一格。键路径与请求体、答复体一致。
type MeasurementField uint8

const (
	MeasurementFieldInvalid MeasurementField = iota
	MeasurementFieldGoodsCode
	MeasurementFieldPackagingLevel
	MeasurementFieldCartonNumber
	MeasurementFieldDimensionsLength
	MeasurementFieldDimensionsWidth
	MeasurementFieldDimensionsHeight
	MeasurementFieldDimensionsUnit
	MeasurementFieldWeightValue
	MeasurementFieldWeightUnit
)

func (field MeasurementField) String() string {
	switch field {
	case MeasurementFieldGoodsCode:
		return "goodsCode"
	case MeasurementFieldPackagingLevel:
		return "packagingLevel"
	case MeasurementFieldCartonNumber:
		return "cartonNumber"
	case MeasurementFieldDimensionsLength:
		return "dimensions.length"
	case MeasurementFieldDimensionsWidth:
		return "dimensions.width"
	case MeasurementFieldDimensionsHeight:
		return "dimensions.height"
	case MeasurementFieldDimensionsUnit:
		return "dimensions.unit"
	case MeasurementFieldWeightValue:
		return "weight.value"
	case MeasurementFieldWeightUnit:
		return "weight.unit"
	default:
		return ""
	}
}

// MeasurementTarget 是一次仓测所指向的对象：要么是（货主，商品，包装层级），要么是（货主，箱号）。
// 按箱测的仓在事实本身的 Warehouse 格上。两类对象在构造器里互斥，不留半残值。
type MeasurementTarget struct {
	kind   MeasurementTargetKind
	owner  OwnerAccountID
	goods  GoodsCode
	level  GoodsUnitName
	carton CartonNumber
}

func NewGoodsLevelMeasurementTarget(
	owner OwnerAccountID,
	goods GoodsCode,
	level GoodsUnitName,
) (MeasurementTarget, error) {
	if !owner.valid() || !goods.valid() || !level.valid() {
		return MeasurementTarget{}, ErrInvalidWarehouseMeasurement
	}
	return MeasurementTarget{
		kind:  MeasurementTargetGoodsLevel,
		owner: owner,
		goods: goods,
		level: level,
	}, nil
}

func NewCartonMeasurementTarget(
	owner OwnerAccountID,
	carton CartonNumber,
) (MeasurementTarget, error) {
	if !owner.valid() || !carton.valid() {
		return MeasurementTarget{}, ErrInvalidWarehouseMeasurement
	}
	return MeasurementTarget{
		kind:   MeasurementTargetCarton,
		owner:  owner,
		carton: carton,
	}, nil
}

func (target MeasurementTarget) Kind() MeasurementTargetKind { return target.kind }
func (target MeasurementTarget) Owner() OwnerAccountID       { return target.owner }
func (target MeasurementTarget) Goods() GoodsCode            { return target.goods }
func (target MeasurementTarget) Level() GoodsUnitName        { return target.level }
func (target MeasurementTarget) Carton() CartonNumber        { return target.carton }

func (target MeasurementTarget) valid() bool {
	switch target.kind {
	case MeasurementTargetGoodsLevel:
		return target.owner.valid() && target.goods.valid() && target.level.valid() && !target.carton.valid()
	case MeasurementTargetCarton:
		return target.owner.valid() && !target.goods.valid() && !target.level.valid() && target.carton.valid()
	default:
		return false
	}
}

// WarehouseMeasurementSubmission 是仓测判重放与内容冲突要比的那一份内容。
// 不含接收时刻：重放总在更晚的时刻到达，不该因此判成内容冲突。
type WarehouseMeasurementSubmission struct {
	Device         MeasurementDeviceID
	Operator       MeasurementOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      WarehouseCode
	Target         MeasurementTarget
	Dimensions     DeclaredDimensions
	Weight         DeclaredWeight
}

// WarehouseMeasurementSpec 构造一版已落册的仓测记录。
type WarehouseMeasurementSpec struct {
	Tenant         TenantID
	FactID         MeasurementFactID
	Device         MeasurementDeviceID
	Operator       MeasurementOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	ReceivedAt     time.Time
	Warehouse      WarehouseCode
	Target         MeasurementTarget
	Dimensions     DeclaredDimensions
	Weight         DeclaredWeight
}

// WarehouseMeasurement 是一版不可改写的仓测尺寸重量。长宽高与重量同在，单位逐字保存、不折算。
// 发生时刻与接收时刻按 UTC 微秒归一，保证跨 Postgres 读写后重放比对一致。
type WarehouseMeasurement struct {
	tenant         TenantID
	factID         MeasurementFactID
	device         MeasurementDeviceID
	operator       MeasurementOperatorID
	occurredAt     time.Time
	deviceSequence int64
	receivedAt     time.Time
	warehouse      WarehouseCode
	target         MeasurementTarget
	dimensions     DeclaredDimensions
	weight         DeclaredWeight
}

func NewWarehouseMeasurement(spec WarehouseMeasurementSpec) (WarehouseMeasurement, error) {
	measurement := WarehouseMeasurement{
		tenant:         spec.Tenant,
		factID:         spec.FactID,
		device:         spec.Device,
		operator:       spec.Operator,
		occurredAt:     normalizedInstant(spec.OccurredAt),
		deviceSequence: spec.DeviceSequence,
		receivedAt:     normalizedInstant(spec.ReceivedAt),
		warehouse:      spec.Warehouse,
		target:         spec.Target,
		dimensions:     spec.Dimensions,
		weight:         spec.Weight,
	}
	if !measurement.tenant.valid() || !measurement.factID.valid() || !measurement.device.valid() ||
		!measurement.operator.valid() || measurement.occurredAt.IsZero() || measurement.deviceSequence < 1 ||
		measurement.receivedAt.IsZero() || !measurement.warehouse.valid() || !measurement.target.valid() ||
		!dimensionsValid(measurement.dimensions) || !weightValid(measurement.weight) {
		return WarehouseMeasurement{}, ErrInvalidWarehouseMeasurement
	}
	return measurement, nil
}

func dimensionsValid(dimensions DeclaredDimensions) bool {
	return dimensions.length.valid() && dimensions.width.valid() && dimensions.height.valid() && dimensions.unit.valid()
}

func weightValid(weight DeclaredWeight) bool {
	return weight.value.valid() && weight.unit.valid()
}

func (measurement WarehouseMeasurement) Tenant() TenantID                { return measurement.tenant }
func (measurement WarehouseMeasurement) FactID() MeasurementFactID       { return measurement.factID }
func (measurement WarehouseMeasurement) Device() MeasurementDeviceID     { return measurement.device }
func (measurement WarehouseMeasurement) Operator() MeasurementOperatorID { return measurement.operator }
func (measurement WarehouseMeasurement) OccurredAt() time.Time           { return measurement.occurredAt }
func (measurement WarehouseMeasurement) DeviceSequence() int64           { return measurement.deviceSequence }
func (measurement WarehouseMeasurement) ReceivedAt() time.Time           { return measurement.receivedAt }
func (measurement WarehouseMeasurement) Warehouse() WarehouseCode        { return measurement.warehouse }
func (measurement WarehouseMeasurement) Target() MeasurementTarget       { return measurement.target }
func (measurement WarehouseMeasurement) Dimensions() DeclaredDimensions {
	return measurement.dimensions
}
func (measurement WarehouseMeasurement) Weight() DeclaredWeight { return measurement.weight }

// SameContent 回答一次提交与这版在册仓测是不是同一份内容。数值经 Measure 规范化，12.5 与 12.50 算同一内容；
// 单位按码逐字比，不折算；时间按微秒归一。
func (measurement WarehouseMeasurement) SameContent(submission WarehouseMeasurementSubmission) bool {
	return measurement.device == submission.Device &&
		measurement.operator == submission.Operator &&
		measurement.occurredAt.Equal(normalizedInstant(submission.OccurredAt)) &&
		measurement.deviceSequence == submission.DeviceSequence &&
		measurement.warehouse == submission.Warehouse &&
		measurement.target == submission.Target &&
		measurement.dimensions == submission.Dimensions &&
		measurement.weight == submission.Weight
}

// CompareWarehouseMeasurementOrder 按作业事实的发生次序定序（UC-WF-002、ADR-0023）：
// 先比发生时间，再比设备单调序号；两台不同设备偶发同时同序号时，按接收时刻再按事实身份打破平局，保证全序确定。
func CompareWarehouseMeasurementOrder(left, right WarehouseMeasurement) int {
	switch {
	case left.occurredAt.Before(right.occurredAt):
		return -1
	case left.occurredAt.After(right.occurredAt):
		return 1
	case left.deviceSequence < right.deviceSequence:
		return -1
	case left.deviceSequence > right.deviceSequence:
		return 1
	case left.receivedAt.Before(right.receivedAt):
		return -1
	case left.receivedAt.After(right.receivedAt):
		return 1
	case left.factID.String() < right.factID.String():
		return -1
	case left.factID.String() > right.factID.String():
		return 1
	default:
		return 0
	}
}

// SortWarehouseMeasurements 按发生次序从早到晚排出一份新切片，不改调用方的切片。
func SortWarehouseMeasurements(versions []WarehouseMeasurement) []WarehouseMeasurement {
	ordered := slices.Clone(versions)
	slices.SortFunc(ordered, CompareWarehouseMeasurementOrder)
	return ordered
}

// CurrentWarehouseMeasurement 从同一个对象的各版仓测里取出当前那一版（按发生次序最后一版，不按到达先后）。
func CurrentWarehouseMeasurement(versions []WarehouseMeasurement) (WarehouseMeasurement, bool) {
	if len(versions) == 0 {
		return WarehouseMeasurement{}, false
	}
	latest := versions[0]
	for _, candidate := range versions[1:] {
		if CompareWarehouseMeasurementOrder(candidate, latest) > 0 {
			latest = candidate
		}
	}
	return latest, true
}
