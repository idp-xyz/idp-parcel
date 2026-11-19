package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// locationTypeBody 是一笔库位类型修订的答复形状，登记口回显在册那一笔与各读口的行共用。键名与登记报文、与登记口答复里 `field`
// 点名的格同一套；是否停用是显式布尔。
type locationTypeBody struct {
	LocationTypeCode string `json:"locationTypeCode"`
	Revision         int    `json:"revision"`
	Name             string `json:"name"`
	Deactivated      bool   `json:"deactivated"`
	Registrant       string `json:"registrant"`
	RegisteredAt     string `json:"registeredAt"`
}

func locationTypeBodyOfRevision(revision domain.LocationTypeRevision) locationTypeBody {
	content := revision.Content()
	return locationTypeBody{
		LocationTypeCode: revision.Code().String(),
		Revision:         revision.Revision(),
		Name:             content.Name().String(),
		Deactivated:      content.Deactivated(),
		Registrant:       revision.Registrant().String(),
		RegisteredAt:     rfc3339(revision.RegisteredAt()),
	}
}

func locationTypeBodiesOfRows(rows []ports.LocationTypeRow) []locationTypeBody {
	// 空册是空数组，不是 null：调用侧按数组读，不必再判一遍缺席。
	bodies := make([]locationTypeBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, locationTypeBody{
			LocationTypeCode: row.LocationTypeCode,
			Revision:         row.Revision,
			Name:             row.Name,
			Deactivated:      row.Deactivated,
			Registrant:       row.Registrant,
			RegisteredAt:     rfc3339(row.RegisteredAt),
		})
	}
	return bodies
}
