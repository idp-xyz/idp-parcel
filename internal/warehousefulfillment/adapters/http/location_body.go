package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// locationBody 是一笔库位修订的答复形状，登记口回显在册那一笔与各读口的行共用。键名与登记报文、与登记口答复里 `field` 点名的格
// 同一套；是否停用是显式布尔。库位类型与优先级可缺：没有时那个键整键缺席（不是空串、null 或 0）——「没有」与「有」是不同的内容，
// 优先级 0 要出现。
type locationBody struct {
	WarehouseCode    string  `json:"warehouseCode"`
	LocationCode     string  `json:"locationCode"`
	Revision         int     `json:"revision"`
	ZoneCode         string  `json:"zoneCode"`
	LocationTypeCode *string `json:"locationTypeCode,omitempty"`
	Priority         *int32  `json:"priority,omitempty"`
	Deactivated      bool    `json:"deactivated"`
	Registrant       string  `json:"registrant"`
	RegisteredAt     string  `json:"registeredAt"`
}

func locationBodyOfRevision(revision domain.LocationRevision) locationBody {
	content := revision.Content()
	body := locationBody{
		WarehouseCode: revision.Warehouse().String(),
		LocationCode:  revision.Code().String(),
		Revision:      revision.Revision(),
		ZoneCode:      content.ZoneCode().String(),
		Deactivated:   content.Deactivated(),
		Registrant:    revision.Registrant().String(),
		RegisteredAt:  rfc3339(revision.RegisteredAt()),
	}
	if code, has := content.LocationType(); has {
		text := code.String()
		body.LocationTypeCode = &text
	}
	if content.Priority().Given() {
		value := content.Priority().Value()
		body.Priority = &value
	}
	return body
}

func locationBodiesOfRows(rows []ports.LocationRow) []locationBody {
	// 空册是空数组，不是 null：调用侧按数组读，不必再判一遍缺席。
	bodies := make([]locationBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, locationBody{
			WarehouseCode:    row.WarehouseCode,
			LocationCode:     row.LocationCode,
			Revision:         row.Revision,
			ZoneCode:         row.ZoneCode,
			LocationTypeCode: row.LocationTypeCode,
			Priority:         row.Priority,
			Deactivated:      row.Deactivated,
			Registrant:       row.Registrant,
			RegisteredAt:     rfc3339(row.RegisteredAt),
		})
	}
	return bodies
}
