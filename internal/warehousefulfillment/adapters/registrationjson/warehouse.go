package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// warehouseDocument 是一笔仓修订的线格式。键名与登记口答复里 `field` 点名的格同一套（`address.country`、`businessTimeZone`），
// 登记方照答复里的格改报文，不必再做一次对照。
//
// 必填格用指针，缺失与零值才分得开：修订号 0 是应用层答修订错位的内容，不是缺失；空串是空白，不是缺失。
type warehouseDocument struct {
	carriedIdentity
	WarehouseCode    *string          `json:"warehouseCode"`
	Revision         *int             `json:"revision"`
	Name             *string          `json:"name"`
	Kind             *string          `json:"kind"`
	Address          *addressDocument `json:"address"`
	BusinessTimeZone *string          `json:"businessTimeZone"`
	ColocatedNode    *string          `json:"colocatedNode"`
	Deactivated      *bool            `json:"deactivated"`
}

type addressDocument struct {
	Country *string `json:"country"`
	Text    *string `json:"text"`
}

// WarehouseCommand 把一笔仓修订的 JSON 译成登记命令。tenant 与 registrant 由调用方给（取自认证结果），报文自带即坏载荷。
// 一份载荷只许一个对象：尾随的第二个 JSON 值也拒，放过它就是无声丢掉一段输入。
//
// 仓编码、地址正文、同址物流节点逐字交给领域：不去空白、不折大小写（CONTEXT「主数据登记」）。可缺的业务时区、同址物流节点与
// 是否停用缺失即没登记、没指名、不停用，不取任何默认。键给了却是空白的，不当作没给——那是写错了，不是没有。
func WarehouseCommand(
	reader io.Reader,
	tenant domain.TenantID,
	registrant domain.Registrant,
) (application.RegisterWarehouseCommand, error) {
	var none application.RegisterWarehouseCommand
	document, err := decodeDocument[warehouseDocument](reader, "warehouse revision")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}

	if document.WarehouseCode == nil {
		return none, malformed("warehouseCode is required")
	}
	if document.Revision == nil {
		return none, malformed("revision is required")
	}
	if document.Name == nil {
		return none, malformed("name is required")
	}
	if document.Kind == nil {
		return none, malformed("kind is required")
	}
	if document.Address == nil {
		return none, malformed("address is required")
	}
	if document.Address.Country == nil {
		return none, malformed("address.country is required")
	}
	if document.Address.Text == nil {
		return none, malformed("address.text is required")
	}

	code, err := textCell("warehouseCode", *document.WarehouseCode, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	name, err := textCell("name", *document.Name, domain.NewWarehouseName)
	if err != nil {
		return none, err
	}
	kind, known := domain.WarehouseKindNamed(*document.Kind)
	if !known {
		return none, malformed("kind must be one of %s, %s", domain.WarehouseKindSelfOperated, domain.WarehouseKindAgent)
	}
	country, err := textCell("address.country", *document.Address.Country, domain.NewCountryCode)
	if err != nil {
		return none, err
	}
	addressText, err := textCell("address.text", *document.Address.Text, domain.NewAddressText)
	if err != nil {
		return none, err
	}

	spec := domain.WarehouseContentSpec{
		Name:    name,
		Kind:    kind,
		Address: domain.NewWarehouseAddress(country, addressText),
	}
	if document.BusinessTimeZone != nil {
		if spec.BusinessTimeZone, err = textCell("businessTimeZone", *document.BusinessTimeZone, domain.NewBusinessTimeZone); err != nil {
			return none, err
		}
	}
	if document.ColocatedNode != nil {
		if spec.ColocatedNode, err = textCell("colocatedNode", *document.ColocatedNode, domain.NewColocatedNode); err != nil {
			return none, err
		}
	}
	if document.Deactivated != nil {
		spec.Deactivated = *document.Deactivated
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		// 各必填格上面都已过了领域构造门，走到这里是领域与译装对「必填」的理解漂开了。
		return none, malformed("the warehouse content is incomplete")
	}
	return application.RegisterWarehouseCommand{
		Tenant:     tenant,
		Code:       code,
		Revision:   *document.Revision,
		Content:    content,
		Registrant: registrant,
	}, nil
}
