package registrationjson

import (
	"io"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// zoneDocument 是一笔库区修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`usage`、`warehouseCode`），登记方照答复里的
// 格改报文，不必再做一次对照。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。
type zoneDocument struct {
	carriedIdentity
	WarehouseCode *string `json:"warehouseCode"`
	ZoneCode      *string `json:"zoneCode"`
	Revision      *int    `json:"revision"`
	Name          *string `json:"name"`
	Usage         *string `json:"usage"`
	Deactivated   *bool   `json:"deactivated"`
}

// ZoneCommand 把一笔库区修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象，规矩同 WarehouseCommand。
//
// 仓编码、库区编码与名称逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。用途须是封闭的六种之一，集外的词是坏载荷，
// 不是未受理。「指名的仓在不在册」不在这里判：它是应用层在判过重放与冲突之后另判的事，译装提前拒掉，同一份内容在「原样重投」
// 时就会先于「已登记（重放）」被拒。是否停用缺失即不停用，不取任何默认。
func ZoneCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterZoneCommand, error) {
	var none application.RegisterZoneCommand
	document, err := decodeDocument[zoneDocument](reader, "zone revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}

	if document.WarehouseCode == nil {
		return none, malformed("warehouseCode is required")
	}
	if document.ZoneCode == nil {
		return none, malformed("zoneCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.Name == nil {
		return none, malformed("name is required")
	}
	if document.Usage == nil {
		return none, malformed("usage is required")
	}

	warehouse, err := textCell("warehouseCode", *document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	code, err := textCell("zoneCode", *document.ZoneCode, domain.NewZoneCode)
	if err != nil {
		return none, err
	}
	name, err := textCell("name", *document.Name, domain.NewZoneName)
	if err != nil {
		return none, err
	}
	usage, known := domain.ZoneUsageNamed(*document.Usage)
	if !known {
		return none, malformed("usage must be one of %s", zoneUsageWords())
	}

	spec := domain.ZoneContentSpec{Name: name, Usage: usage}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewZoneContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the zone content is incomplete")
	}
	return application.RegisterZoneCommand{
		Tenant:     tenant,
		Warehouse:  warehouse,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}

// zoneUsageWords 把封闭集的字面量写成理由里的一句，次序同封闭集。
func zoneUsageWords() string {
	usages := domain.ZoneUsages()
	words := make([]string, len(usages))
	for index, usage := range usages {
		words[index] = usage.String()
	}
	return strings.Join(words, ", ")
}
