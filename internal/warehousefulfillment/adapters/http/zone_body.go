package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// zoneBody 是一笔库区修订的答复形状，登记口回显在册那一笔与各读口的行共用。键名与登记报文、与登记口答复里 `field` 点名的格
// 同一套；是否停用是显式布尔。
type zoneBody struct {
	WarehouseCode string `json:"warehouseCode"`
	ZoneCode      string `json:"zoneCode"`
	Revision      int    `json:"revision"`
	Name          string `json:"name"`
	Usage         string `json:"usage"`
	Deactivated   bool   `json:"deactivated"`
	Registrant    string `json:"registrant"`
	RegisteredAt  string `json:"registeredAt"`
}

func zoneBodyOfRevision(revision domain.ZoneRevision) zoneBody {
	content := revision.Content()
	return zoneBody{
		WarehouseCode: revision.Warehouse().String(),
		ZoneCode:      revision.Code().String(),
		Revision:      revision.Revision(),
		Name:          content.Name().String(),
		Usage:         content.Usage().String(),
		Deactivated:   content.Deactivated(),
		Registrant:    revision.Registrant().String(),
		RegisteredAt:  rfc3339(revision.RegisteredAt()),
	}
}

func zoneBodiesOfRows(rows []ports.ZoneRow) []zoneBody {
	// 空册是空数组，不是 null：调用侧按数组读，不必再判一遍缺席。
	bodies := make([]zoneBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, zoneBody{
			WarehouseCode: row.WarehouseCode,
			ZoneCode:      row.ZoneCode,
			Revision:      row.Revision,
			Name:          row.Name,
			Usage:         row.Usage,
			Deactivated:   row.Deactivated,
			Registrant:    row.Registrant,
			RegisteredAt:  rfc3339(row.RegisteredAt),
		})
	}
	return bodies
}
