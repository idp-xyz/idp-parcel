package registrationjson

import (
	"encoding/json"
	"io"
	"strconv"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// locationDocument 是一笔库位修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`warehouseCode`、`zoneCode`、
// `locationTypeCode`），登记方照答复里的格改报文，不必再做一次对照。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。库位类型编码是指针：
// 键缺失与显式 null 都是「没有」。优先级用原文承接：它要认「整数、32 位有符号」，带小数、指数写法与字符串都得拒，交给标准库的
// 整型解码就会在三种错上给出三种说法，也认不出 null 之外的几种写法，所以自己按 JSON 整数的文法判。
type locationDocument struct {
	carriedIdentity
	WarehouseCode    *string         `json:"warehouseCode"`
	LocationCode     *string         `json:"locationCode"`
	Revision         *int            `json:"revision"`
	ZoneCode         *string         `json:"zoneCode"`
	LocationTypeCode *string         `json:"locationTypeCode"`
	Priority         json.RawMessage `json:"priority"`
	Deactivated      *bool           `json:"deactivated"`
}

// LocationCommand 把一笔库位修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象，规矩同 WarehouseCommand。
//
// 仓编码、库位编码、所属库区编码与库位类型编码逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。库位类型与优先级可缺，
// 缺失与显式 null 都译成没有，不取任何默认；键给了却是空白的库位类型编码不当作没给——那是写错了，不是没有。优先级是 JSON 整数，
// 可以为负，范围是 32 位有符号整数，带小数、写成字符串或超出范围都是坏载荷。「指名的仓、库区与库位类型在不在册」不在这里判：
// 它们是应用层在判过重放与冲突之后另判的事，译装提前拒掉，同一份内容在「原样重投」时就会先于「已登记（重放）」被拒。是否停用
// 缺失即不停用。
func LocationCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterLocationCommand, error) {
	var none application.RegisterLocationCommand
	document, err := decodeDocument[locationDocument](reader, "location revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}

	if document.WarehouseCode == nil {
		return none, malformed("warehouseCode is required")
	}
	if document.LocationCode == nil {
		return none, malformed("locationCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.ZoneCode == nil {
		return none, malformed("zoneCode is required")
	}

	warehouse, err := textCell("warehouseCode", *document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	code, err := textCell("locationCode", *document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	zone, err := textCell("zoneCode", *document.ZoneCode, domain.NewZoneCode)
	if err != nil {
		return none, err
	}

	spec := domain.LocationContentSpec{ZoneCode: zone}
	if document.LocationTypeCode != nil {
		if spec.LocationType, err = textCell("locationTypeCode", *document.LocationTypeCode, domain.NewLocationTypeCode); err != nil {
			return none, err
		}
	}
	if spec.Priority, err = priorityOf(document.Priority); err != nil {
		return none, err
	}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewLocationContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the location content is incomplete")
	}
	return application.RegisterLocationCommand{
		Tenant:     tenant,
		Warehouse:  warehouse,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}

// priorityOf 译优先级这一格：键缺失与显式 null 是没有；否则必须是 JSON 整数（可带负号，不带小数点、指数与前导零），并且装得进
// 32 位有符号整数。理由只点名格与规则，不回显取值。
func priorityOf(raw json.RawMessage) (domain.LocationPriority, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return domain.LocationPriority{}, nil
	}
	const reason = "priority must be an integer that fits a 32-bit signed integer"
	if !isJSONInteger(raw) {
		return domain.LocationPriority{}, malformed(reason)
	}
	value, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil {
		return domain.LocationPriority{}, malformed(reason)
	}
	return domain.NewLocationPriority(int32(value)), nil
}

// isJSONInteger 按 JSON 的数文法判一段原文是不是整数：可选负号，其后是 0 或不以 0 起头的数字串。带小数点或指数的写法（含 `1.0`、
// `1e2`）一律不算，即使它们的数值恰是整数——登记方交的是整数，写成别的样子就是写错了。
func isJSONInteger(raw []byte) bool {
	digits := raw
	if len(digits) > 0 && digits[0] == '-' {
		digits = digits[1:]
	}
	if len(digits) == 0 {
		return false
	}
	for _, character := range digits {
		if character < '0' || character > '9' {
			return false
		}
	}
	return len(digits) == 1 || digits[0] != '0'
}
