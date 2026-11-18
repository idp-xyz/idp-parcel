package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	// ErrBlankValue 是逐字保存的取值给成了空串或只有空白。
	ErrBlankValue = errors.New("warehouse fulfillment: blank value")
	// ErrUnstorableValue 是取值带了存不下的字节（NUL、非 UTF-8）。逐字保存的前提是存得下：到库里才失败，
	// 登记方看到的就是一次技术失败而不是一句「这个值不成立」。
	ErrUnstorableValue = errors.New("warehouse fulfillment: value cannot be stored verbatim")
)

// requiredValue 是逐字保存的非空取值：只拒空白与存不下的串，其余一个字节都不动——不折大小写，不去首尾与内部空白
// （CONTEXT「主数据登记」：货主给的码与租户登记的编码逐字保存）。因此 `WH-1` 与 `wh-1`、` WH-1` 是三个不同的值。
type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrUnstorableValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

// valid 只回答「是不是经构造器得来的」：构造器不放行空串，零值因此就是没构造过。
func (value requiredValue) valid() bool {
	return value.value != ""
}

// TenantID 取自接入信封，从不取自载荷。
type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// WarehouseCode 是仓的编码，与租户合成仓的身份，登记后不改。
type WarehouseCode struct{ requiredValue }

func NewWarehouseCode(value string) (WarehouseCode, error) {
	required, err := newRequiredValue("warehouse code", value)
	return WarehouseCode{required}, err
}

type WarehouseName struct{ requiredValue }

func NewWarehouseName(value string) (WarehouseName, error) {
	required, err := newRequiredValue("warehouse name", value)
	return WarehouseName{required}, err
}

// CountryCode 是仓的地址所在国家 / 地区码。构造器只管「给了且存得下」；是不是产品内置目录里的码，由登记口在判过
// 重放与冲突之后另判（ADR-0345 决定五），读回的在册行因此不受目录更新牵连。
type CountryCode struct{ requiredValue }

func NewCountryCode(value string) (CountryCode, error) {
	required, err := newRequiredValue("country code", value)
	return CountryCode{required}, err
}

type AddressText struct{ requiredValue }

func NewAddressText(value string) (AddressText, error) {
	required, err := newRequiredValue("address text", value)
	return AddressText{required}, err
}

// BusinessTimeZone 是仓的业务时区名。零值就是没登记：不取租户、部署或地址所在国的默认（CONTEXT「身份与边界」按日计的
// 各项取仓的业务时区，没登记时答未配置）。认不认得出这个名字由登记口判，不在构造器里读环境。
type BusinessTimeZone struct{ requiredValue }

func NewBusinessTimeZone(value string) (BusinessTimeZone, error) {
	required, err := newRequiredValue("business time zone", value)
	return BusinessTimeZone{required}, err
}

// Registered 回答这一格有没有登记。
func (zone BusinessTimeZone) Registered() bool {
	return zone.valid()
}

// ColocatedNode 是仓指名的同址物流节点标识，逐字保存，只作指名：仓与物流节点各是各的身份，不向
// network-routing 核对（ADR-0368 决定三）。零值就是没指名。
type ColocatedNode struct{ requiredValue }

func NewColocatedNode(value string) (ColocatedNode, error) {
	required, err := newRequiredValue("colocated node", value)
	return ColocatedNode{required}, err
}

// Named 回答这一格有没有指名。
func (node ColocatedNode) Named() bool {
	return node.valid()
}

// Registrant 是登记责任方，取自操作者信封里的主体引用，不取自载荷。
type Registrant struct{ requiredValue }

func NewRegistrant(value string) (Registrant, error) {
	required, err := newRequiredValue("registrant", value)
	return Registrant{required}, err
}
