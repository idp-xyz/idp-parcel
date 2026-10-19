package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidCapacityDemand：预占或释放声明的需求量不成立——数量不为正、单位缺、来源种类不在封闭词里或来源引用缺。
	ErrInvalidCapacityDemand = errors.New("transport fulfillment: invalid capacity demand")
	// ErrCapacityUnitMismatch：声明的单位不是池的单位。池上不换算（CONTEXT「只有明确业务规则存在时才能形成可追溯
	// 换算」），声明方要按池的单位重述数量；按字面比，不归一大小写——单位拼写归登记池的一方（ADR-0201 补记「单位只存不判」）。
	ErrCapacityUnitMismatch = errors.New("transport fulfillment: the declared unit is not the pool's unit")
	// ErrDuplicateRelease：同一池里这个释放引用已经被另一次释放占了。释放引用是重放的认领键，一个引用只认一次释放。
	ErrDuplicateRelease = errors.New("transport fulfillment: the release reference is already taken")
)

// CapacityDemandSourceKind 说一份需求量从哪来（ADR-0203 决定三）。封闭词就是下面两种：运营人员声明的一版需求，或运输
// 准备结果固定下来的需求快照。
//
// 封闭而不收自由串：CONTEXT 要每次容量申请固定「采用的测量/数量版本」，来源种类是读的人回头核这个量的入口——自由串
// 拼错一次，这份预占就再也对不回它的依据。要第三种来源是产品改动，不是租户取值。
type CapacityDemandSourceKind uint8

const (
	CapacityDemandSourceInvalid CapacityDemandSourceKind = iota
	CapacityDemandOperatorDeclaration
	CapacityDemandPreparationSnapshot
)

func (kind CapacityDemandSourceKind) String() string {
	switch kind {
	case CapacityDemandOperatorDeclaration:
		return "OPERATOR_DECLARATION"
	case CapacityDemandPreparationSnapshot:
		return "PREPARATION_DEMAND_SNAPSHOT"
	default:
		return ""
	}
}

func (kind CapacityDemandSourceKind) valid() bool {
	return kind.String() != ""
}

// ParseCapacityDemandSourceKind 把 String 写出的词认回来，大小写不折：同一种来源只有一种写法。
func ParseCapacityDemandSourceKind(word string) (CapacityDemandSourceKind, error) {
	for kind := CapacityDemandOperatorDeclaration; kind <= CapacityDemandPreparationSnapshot; kind++ {
		if kind.String() == word {
			return kind, nil
		}
	}
	return CapacityDemandSourceInvalid, fmt.Errorf("%w: source kind %q", ErrInvalidCapacityDemand, word)
}

// CapacityDemandSourceReference 指名需求量依据的那一份：运营声明的版本，或运输准备结果的需求快照。本上下文原样保存、
// 不解析；准备结果那一种此刻也不回读核对（ADR-0203 越权风险点）。
type CapacityDemandSourceReference struct{ requiredValue }

func NewCapacityDemandSourceReference(value string) (CapacityDemandSourceReference, error) {
	required, err := newRequiredValue("capacity demand source reference", value)
	return CapacityDemandSourceReference{required}, err
}

// CapacityDemandSource 是一份需求量的来源：种类加引用。
type CapacityDemandSource struct {
	kind      CapacityDemandSourceKind
	reference CapacityDemandSourceReference
}

func NewCapacityDemandSource(kind CapacityDemandSourceKind, reference CapacityDemandSourceReference) (CapacityDemandSource, error) {
	source := CapacityDemandSource{kind: kind, reference: reference}
	if !source.valid() {
		return CapacityDemandSource{}, ErrInvalidCapacityDemand
	}
	return source, nil
}

func (source CapacityDemandSource) Kind() CapacityDemandSourceKind {
	return source.kind
}

func (source CapacityDemandSource) Reference() CapacityDemandSourceReference {
	return source.reference
}

func (source CapacityDemandSource) valid() bool {
	return source.kind.valid() && source.reference.valid()
}

// CapacityDemand 是预占或释放声明的需求量：数量、单位与来源（CONTEXT「每次容量申请必须固定……各容量维数量、单位」）。
// 维度不在这里——一个池只有一维，声明落到哪个池就是哪一维。单位要声明出来再与池比，不从池上默认：把「5」默认成池的
// 单位，声明方心里的吨就被当成了千克，而这一格没有任何一处会响。
type CapacityDemand struct {
	quantity int64
	unit     QuantityUnitReference
	source   CapacityDemandSource
}

func NewCapacityDemand(quantity int64, unit QuantityUnitReference, source CapacityDemandSource) (CapacityDemand, error) {
	demand := CapacityDemand{quantity: quantity, unit: unit, source: source}
	if !demand.valid() {
		return CapacityDemand{}, ErrInvalidCapacityDemand
	}
	return demand, nil
}

func (demand CapacityDemand) Quantity() int64 {
	return demand.quantity
}

func (demand CapacityDemand) Unit() QuantityUnitReference {
	return demand.unit
}

func (demand CapacityDemand) Source() CapacityDemandSource {
	return demand.source
}

func (demand CapacityDemand) valid() bool {
	return demand.quantity > 0 && demand.unit.valid() && demand.source.valid()
}

// CapacityReleaseReference 指名一次显式释放。释放有自己的身份，是为了重放：同一次释放原样重交要认得出来，否则每交一次
// 就多还一份容量。
type CapacityReleaseReference struct{ requiredValue }

func NewCapacityReleaseReference(value string) (CapacityReleaseReference, error) {
	required, err := newRequiredValue("capacity release reference", value)
	return CapacityReleaseReference{required}, err
}
