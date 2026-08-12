package domain

import (
	"errors"
	"strings"
)

var ErrInvalidDeclaredMeasurement = errors.New("parcel shipment: invalid declared measurement")

// MeasurementValue 是客户申报数字的保真快照（ADR-0048）：合法正十进制字符串，原样保全。
// 不折成浮点也不换算——本上下文只回答「客户报了什么」，不回答「等于多少」；后者是
// parcel-pricing 规范化的事。
type MeasurementValue struct {
	raw string
}

// NewMeasurementValue 只做形状校验：非空、合法十进制、大于零。零与负数在申报语义下
// 不是一个测量；格式花样（指数、正负号、多点）一律拒——保真不等于什么都收，收下一个
// 读不出数的字符串，下游译计价输入时才炸。
func NewMeasurementValue(raw string) (MeasurementValue, error) {
	trimmed := strings.TrimSpace(raw)
	if !decimalShape(trimmed) || !positiveDecimal(trimmed) {
		return MeasurementValue{}, ErrInvalidDeclaredMeasurement
	}
	return MeasurementValue{raw: trimmed}, nil
}

func (value MeasurementValue) String() string {
	return value.raw
}

func (value MeasurementValue) valid() bool {
	return value.raw != ""
}

// decimalShape 认「数字串」或「数字串.数字串」两种形状，别的都不是申报数字。
func decimalShape(raw string) bool {
	if raw == "" {
		return false
	}
	whole, fraction, dotted := strings.Cut(raw, ".")
	if !digits(whole) {
		return false
	}
	if dotted && !digits(fraction) {
		return false
	}
	return true
}

func digits(raw string) bool {
	if raw == "" {
		return false
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// positiveDecimal 判至少一位非零数字：形状合法的 "0"、"0.00" 仍不是一个测量。
func positiveDecimal(raw string) bool {
	for _, character := range raw {
		if character >= '1' && character <= '9' {
			return true
		}
	}
	return false
}

// MeasurementUnitReference 指名申报所用的计量单位。真实单位目录属实例半边
// （PAR-INT-01 的接入契约），机制只要求单位被指名——没有单位的数字读不出任何测量。
type MeasurementUnitReference struct{ requiredValue }

func NewMeasurementUnitReference(value string) (MeasurementUnitReference, error) {
	required, err := newRequiredValue("measurement unit reference", value)
	return MeasurementUnitReference{required}, err
}

// DeclaredWeight 是客户申报的毛重：数值加单位，二者缺一读不出重量。
type DeclaredWeight struct {
	value MeasurementValue
	unit  MeasurementUnitReference
}

func NewDeclaredWeight(value MeasurementValue, unit MeasurementUnitReference) (DeclaredWeight, error) {
	if !value.valid() || !unit.valid() {
		return DeclaredWeight{}, ErrInvalidDeclaredMeasurement
	}
	return DeclaredWeight{value: value, unit: unit}, nil
}

func (weight DeclaredWeight) Value() MeasurementValue {
	return weight.value
}

func (weight DeclaredWeight) Unit() MeasurementUnitReference {
	return weight.unit
}

func (weight DeclaredWeight) declared() bool {
	return weight.value.valid()
}

// DeclaredDimensions 是客户申报的外廓尺寸：三边共用一个单位。允许整体缺席（小包申报
// 常只报重量），但报了就三边齐——两边加一个洞不是一个外廓。
type DeclaredDimensions struct {
	length MeasurementValue
	width  MeasurementValue
	height MeasurementValue
	unit   MeasurementUnitReference
}

func NewDeclaredDimensions(
	length, width, height MeasurementValue,
	unit MeasurementUnitReference,
) (DeclaredDimensions, error) {
	if !length.valid() || !width.valid() || !height.valid() || !unit.valid() {
		return DeclaredDimensions{}, ErrInvalidDeclaredMeasurement
	}
	return DeclaredDimensions{length: length, width: width, height: height, unit: unit}, nil
}

func (dimensions DeclaredDimensions) Length() MeasurementValue {
	return dimensions.length
}

func (dimensions DeclaredDimensions) Width() MeasurementValue {
	return dimensions.width
}

func (dimensions DeclaredDimensions) Height() MeasurementValue {
	return dimensions.height
}

func (dimensions DeclaredDimensions) Unit() MeasurementUnitReference {
	return dimensions.unit
}

func (dimensions DeclaredDimensions) declared() bool {
	return dimensions.length.valid()
}

// DeclaredMeasurement 是一个声明成员的测量快照：毛重必备（没有重量的测量装配不出任何
// 估价输入），外廓可缺。它是客户话语不是事实判断——与节点实测的比对属 PN-03 的复核链，
// 不在这里预演。
type DeclaredMeasurement struct {
	weight     DeclaredWeight
	dimensions DeclaredDimensions
}

// NewDeclaredMeasurement 的外廓参数允许零值（缺席）；半截外廓在 NewDeclaredDimensions
// 构造期就死，这里到不了。
func NewDeclaredMeasurement(weight DeclaredWeight, dimensions DeclaredDimensions) (DeclaredMeasurement, error) {
	if !weight.declared() {
		return DeclaredMeasurement{}, ErrInvalidDeclaredMeasurement
	}
	return DeclaredMeasurement{weight: weight, dimensions: dimensions}, nil
}

func (measurement DeclaredMeasurement) Weight() DeclaredWeight {
	return measurement.weight
}

// Dimensions 报告外廓及其是否被申报。缺席是真话：客户没报，读取方自己决定缺席的后果。
func (measurement DeclaredMeasurement) Dimensions() (DeclaredDimensions, bool) {
	return measurement.dimensions, measurement.dimensions.declared()
}

func (measurement DeclaredMeasurement) declared() bool {
	return measurement.weight.declared()
}

// DeclaredParcelProfile 把一个声明成员与其测量配对（ADR-0048 的画像）。测量必填与否由
// 真实产品定，画像整体缺席合法——但立起来的画像必须完整指名成员与测量。
type DeclaredParcelProfile struct {
	parcel      DeclaredParcelID
	measurement DeclaredMeasurement
}

func NewDeclaredParcelProfile(
	parcel DeclaredParcelID,
	measurement DeclaredMeasurement,
) (DeclaredParcelProfile, error) {
	if !parcel.valid() || !measurement.declared() {
		return DeclaredParcelProfile{}, ErrInvalidDeclaredMeasurement
	}
	return DeclaredParcelProfile{parcel: parcel, measurement: measurement}, nil
}

func (profile DeclaredParcelProfile) Parcel() DeclaredParcelID {
	return profile.parcel
}

func (profile DeclaredParcelProfile) Measurement() DeclaredMeasurement {
	return profile.measurement
}
