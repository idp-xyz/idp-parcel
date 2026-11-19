package domain

import "strings"

// GrossWeight 是称重量方里的毛重：数值与它的单位成对，拆开就没有意义。单位是现场报的原词，这里不换算、不改写数字；
// 对产品内置单位目录的核对在入口做。零值不是一份毛重。
type GrossWeight struct {
	value string
	unit  string
}

// NewGrossWeight 收下一份毛重。数值按正的十进制数收，单位去掉首尾空白后必须还在；两头的空白都不算内容。
func NewGrossWeight(value, unit string) (GrossWeight, error) {
	trimmedUnit := strings.TrimSpace(unit)
	if trimmedUnit == "" {
		return GrossWeight{}, ErrInvalidActualMeasurement
	}
	quantity, err := newMeasurementQuantity(value)
	if err != nil {
		return GrossWeight{}, err
	}
	return GrossWeight{value: quantity, unit: trimmedUnit}, nil
}

func (weight GrossWeight) Value() string { return weight.value }

func (weight GrossWeight) Unit() string { return weight.unit }

func (weight GrossWeight) valid() bool { return weight.value != "" && weight.unit != "" }

// OuterDimensions 是称重量方里的外廓尺寸：长、宽、高三边共用一个单位。三边与单位成套，缺一格就算不出体积重量。
// 单位是现场报的原词，这里不换算。零值不是一份外廓尺寸。
type OuterDimensions struct {
	length string
	width  string
	height string
	unit   string
}

// NewOuterDimensions 收下一份外廓尺寸。三边各按正的十进制数收，单位去掉首尾空白后必须还在。
func NewOuterDimensions(length, width, height, unit string) (OuterDimensions, error) {
	trimmedUnit := strings.TrimSpace(unit)
	if trimmedUnit == "" {
		return OuterDimensions{}, ErrInvalidActualMeasurement
	}
	dimensions := OuterDimensions{unit: trimmedUnit}
	for _, side := range []struct {
		raw  string
		into *string
	}{
		{length, &dimensions.length},
		{width, &dimensions.width},
		{height, &dimensions.height},
	} {
		quantity, err := newMeasurementQuantity(side.raw)
		if err != nil {
			return OuterDimensions{}, err
		}
		*side.into = quantity
	}
	return dimensions, nil
}

func (dimensions OuterDimensions) Length() string { return dimensions.length }

func (dimensions OuterDimensions) Width() string { return dimensions.width }

func (dimensions OuterDimensions) Height() string { return dimensions.height }

func (dimensions OuterDimensions) Unit() string { return dimensions.unit }

func (dimensions OuterDimensions) valid() bool {
	return dimensions.length != "" && dimensions.width != "" && dimensions.height != "" && dimensions.unit != ""
}
