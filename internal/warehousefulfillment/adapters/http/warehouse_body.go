package warehousehttp

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// warehouseBody 是一笔仓修订的答复形状，登记口回显在册那一笔与各读口的行共用。键名与登记报文、与登记口答复里 `field`
// 点名的格同一套。业务时区与同址物流节点缺席即没登记、没指名，不拿空串兼作「没有」；是否停用是显式布尔。
type warehouseBody struct {
	WarehouseCode    string      `json:"warehouseCode"`
	Revision         int         `json:"revision"`
	Name             string      `json:"name"`
	Kind             string      `json:"kind"`
	Address          addressBody `json:"address"`
	BusinessTimeZone string      `json:"businessTimeZone,omitempty"`
	ColocatedNode    string      `json:"colocatedNode,omitempty"`
	Deactivated      bool        `json:"deactivated"`
	Registrant       string      `json:"registrant"`
	RegisteredAt     string      `json:"registeredAt"`
}

type addressBody struct {
	Country string `json:"country"`
	Text    string `json:"text"`
}

func warehouseBodyOfRevision(revision domain.WarehouseRevision) warehouseBody {
	content := revision.Content()
	body := warehouseBody{
		WarehouseCode: revision.Code().String(),
		Revision:      revision.Revision(),
		Name:          content.Name().String(),
		Kind:          content.Kind().String(),
		Address: addressBody{
			Country: content.Address().Country().String(),
			Text:    content.Address().Text().String(),
		},
		Deactivated:  content.Deactivated(),
		Registrant:   revision.Registrant().String(),
		RegisteredAt: rfc3339(revision.RegisteredAt()),
	}
	if content.BusinessTimeZone().Registered() {
		body.BusinessTimeZone = content.BusinessTimeZone().String()
	}
	if content.ColocatedNode().Named() {
		body.ColocatedNode = content.ColocatedNode().String()
	}
	return body
}

func warehouseBodyOfRow(row ports.WarehouseRow) warehouseBody {
	body := warehouseBody{
		WarehouseCode: row.WarehouseCode,
		Revision:      row.Revision,
		Name:          row.Name,
		Kind:          row.Kind,
		Address:       addressBody{Country: row.CountryCode, Text: row.AddressText},
		Deactivated:   row.Deactivated,
		Registrant:    row.Registrant,
		RegisteredAt:  rfc3339(row.RegisteredAt),
	}
	if row.HasBusinessTimeZone {
		body.BusinessTimeZone = row.BusinessTimeZone
	}
	if row.HasColocatedNode {
		body.ColocatedNode = row.ColocatedNode
	}
	return body
}

// customerWarehouseBody 是货主读仓当前修订的形状。与运营读口同一本册、同一套内容键，不带登记责任方。
type customerWarehouseBody struct {
	WarehouseCode    string      `json:"warehouseCode"`
	Revision         int         `json:"revision"`
	Name             string      `json:"name"`
	Kind             string      `json:"kind"`
	Address          addressBody `json:"address"`
	BusinessTimeZone string      `json:"businessTimeZone,omitempty"`
	ColocatedNode    string      `json:"colocatedNode,omitempty"`
	Deactivated      bool        `json:"deactivated"`
	RegisteredAt     string      `json:"registeredAt"`
}

func customerWarehouseBodyOfRow(row ports.WarehouseRow) customerWarehouseBody {
	body := customerWarehouseBody{
		WarehouseCode: row.WarehouseCode,
		Revision:      row.Revision,
		Name:          row.Name,
		Kind:          row.Kind,
		Address:       addressBody{Country: row.CountryCode, Text: row.AddressText},
		Deactivated:   row.Deactivated,
		RegisteredAt:  rfc3339(row.RegisteredAt),
	}
	if row.HasBusinessTimeZone {
		body.BusinessTimeZone = row.BusinessTimeZone
	}
	if row.HasColocatedNode {
		body.ColocatedNode = row.ColocatedNode
	}
	return body
}

func customerWarehouseBodiesOfRows(rows []ports.WarehouseRow) []customerWarehouseBody {
	bodies := make([]customerWarehouseBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, customerWarehouseBodyOfRow(row))
	}
	return bodies
}

func warehouseBodiesOfRows(rows []ports.WarehouseRow) []warehouseBody {
	// 空册是空数组，不是 null：调用侧按数组读，不必再判一遍缺席。
	bodies := make([]warehouseBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, warehouseBodyOfRow(row))
	}
	return bodies
}

func rfc3339(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}
