package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// locationTypeDocument 是一笔库位类型修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`name`），登记方照答复里的格改报文，
// 不必再做一次对照。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。
type locationTypeDocument struct {
	carriedIdentity
	LocationTypeCode *string `json:"locationTypeCode"`
	Revision         *int    `json:"revision"`
	Name             *string `json:"name"`
	Deactivated      *bool   `json:"deactivated"`
}

// LocationTypeCommand 把一笔库位类型修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象，规矩同 WarehouseCommand。
//
// 库位类型编码与名称逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。是否停用缺失即不停用，不取任何默认。
// 「本租户有没有登记过仓」不在这里判：它是应用层在判过重放与冲突之后另判的事，译装提前拒掉，同一份内容在「原样重投」时就会先于
// 「已登记（重放）」被拒。
func LocationTypeCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterLocationTypeCommand, error) {
	var none application.RegisterLocationTypeCommand
	document, err := decodeDocument[locationTypeDocument](reader, "location type revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}

	if document.LocationTypeCode == nil {
		return none, malformed("locationTypeCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.Name == nil {
		return none, malformed("name is required")
	}

	code, err := textCell("locationTypeCode", *document.LocationTypeCode, domain.NewLocationTypeCode)
	if err != nil {
		return none, err
	}
	name, err := textCell("name", *document.Name, domain.NewLocationTypeName)
	if err != nil {
		return none, err
	}

	spec := domain.LocationTypeContentSpec{Name: name}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewLocationTypeContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the location type content is incomplete")
	}
	return application.RegisterLocationTypeCommand{
		Tenant:     tenant,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}
