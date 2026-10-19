package domain

import (
	"errors"
	"fmt"
	"slices"
)

// ErrInvalidCapacityDeclaration：逐维容量声明不成立——维度不在封闭词里、单位缺、硬上限不为正，或同一维度声明了两次。
var ErrInvalidCapacityDeclaration = errors.New("transport fulfillment: invalid capacity declaration")

// CapacityDimension 是容量池的维度。封闭词就是下面这几个常量，取自 CONTEXT「容量池」所举的维度（舱位写作 CARGO_SLOTS）。
//
// 封闭而不收自由串：准备评估要按维度把需求量配到池上（ADR-0201 决定八只认重量、体积、件数三维有来源），自由串
// 一拼错就成了一个永远配不上需求的维度，评估只能答`容量待确认`，而登记时没有任何一处会响。要第七种维度是产品
// 改动，不是租户取值（ADR-0201 补记「容量维度封闭词」）。
type CapacityDimension uint8

const (
	CapacityDimensionInvalid CapacityDimension = iota
	CapacityWeight
	CapacityVolume
	CapacityPieces
	CapacityPalletPositions
	CapacityBagPositions
	CapacityCargoSlots
)

func (dimension CapacityDimension) String() string {
	switch dimension {
	case CapacityWeight:
		return "WEIGHT"
	case CapacityVolume:
		return "VOLUME"
	case CapacityPieces:
		return "PIECES"
	case CapacityPalletPositions:
		return "PALLET_POSITIONS"
	case CapacityBagPositions:
		return "BAG_POSITIONS"
	case CapacityCargoSlots:
		return "CARGO_SLOTS"
	default:
		return ""
	}
}

func (dimension CapacityDimension) valid() bool {
	return dimension.String() != ""
}

// ParseCapacityDimension 把 String 写出的词认回来。大小写不折：同一维度只有一种写法，否则池身份（由维度派生）会有两种。
func ParseCapacityDimension(word string) (CapacityDimension, error) {
	for dimension := CapacityWeight; dimension <= CapacityCargoSlots; dimension++ {
		if dimension.String() == word {
			return dimension, nil
		}
	}
	return CapacityDimensionInvalid, fmt.Errorf("%w: dimension %q", ErrInvalidCapacityDeclaration, word)
}

// CapacityDeclaration 是周期安排上一个维度的容量声明：维度、单位与硬上限（ADR-0201 决定三的可选逐维容量）。按日期
// 物化班次时，每一条声明在那一班上建一个池。单位只被引用、不被换算——同维度内怎样按量纲换算归准备评估（决定八）。
type CapacityDeclaration struct {
	Dimension CapacityDimension
	Unit      QuantityUnitReference
	Capacity  int64
}

func (declaration CapacityDeclaration) valid() bool {
	return declaration.Dimension.valid() && declaration.Unit.valid() && declaration.Capacity > 0
}

// normalizedCapacityDeclarations 逐条校验并按维度在封闭词里的次序排。同一维度声明两次拒，不取其一：两条落到同一班次上是同一个
// 池身份，取哪一条都是替登记方选。排序让同一组声明只有一种写法，摘要与库面因此不随登记方的列举顺序变。
func normalizedCapacityDeclarations(declarations []CapacityDeclaration) ([]CapacityDeclaration, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	seen := map[CapacityDimension]bool{}
	normalized := make([]CapacityDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		if !declaration.valid() || seen[declaration.Dimension] {
			return nil, ErrInvalidCapacityDeclaration
		}
		seen[declaration.Dimension] = true
		normalized = append(normalized, declaration)
	}
	slices.SortFunc(normalized, func(left, right CapacityDeclaration) int {
		return int(left.Dimension) - int(right.Dimension)
	})
	return normalized, nil
}
