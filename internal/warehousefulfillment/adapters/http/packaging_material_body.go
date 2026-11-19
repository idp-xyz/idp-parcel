package warehousehttp

import (
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// packagingMaterialBody 是一笔包材修订的答复形状，登记口回显在册那一笔与各读口的行共用。键名与登记报文、与登记口答复里 `field` 点名的格同一套：
// 外廓尺寸在 `dimensions` 下，重量在 `weight` 下；四个数值是 JSON 字符串，写法是去掉多余前导零与尾零的规范写法（12.50 回显 12.5，12.00 回显 12），
// 与请求体里数值的类型一致；是否停用是显式布尔。
type packagingMaterialBody struct {
	PackagingMaterialCode string                  `json:"packagingMaterialCode"`
	Revision              int                     `json:"revision"`
	Name                  string                  `json:"name"`
	Dimensions            packagingDimensionsBody `json:"dimensions"`
	Weight                packagingWeightBody     `json:"weight"`
	Deactivated           bool                    `json:"deactivated"`
	Registrant            string                  `json:"registrant"`
	RegisteredAt          string                  `json:"registeredAt"`
}

type packagingDimensionsBody struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type packagingWeightBody struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

func packagingMaterialBodyOfRevision(revision domain.PackagingMaterialRevision) packagingMaterialBody {
	content := revision.Content()
	return packagingMaterialBody{
		PackagingMaterialCode: revision.Code().String(),
		Revision:              revision.Revision(),
		Name:                  content.Name().String(),
		Dimensions: packagingDimensionsBody{
			Length: content.Length().String(),
			Width:  content.Width().String(),
			Height: content.Height().String(),
			Unit:   content.DimensionsUnit().String(),
		},
		Weight: packagingWeightBody{
			Value: content.Weight().String(),
			Unit:  content.WeightUnit().String(),
		},
		Deactivated:  content.Deactivated(),
		Registrant:   revision.Registrant().String(),
		RegisteredAt: rfc3339(revision.RegisteredAt()),
	}
}

func packagingMaterialBodiesOfRows(rows []ports.PackagingMaterialRow) []packagingMaterialBody {
	// 空册是空数组，不是 null：调用侧按数组读，不必再判一遍缺席。
	bodies := make([]packagingMaterialBody, 0, len(rows))
	for _, row := range rows {
		bodies = append(bodies, packagingMaterialBody{
			PackagingMaterialCode: row.PackagingMaterialCode,
			Revision:              row.Revision,
			Name:                  row.Name,
			Dimensions: packagingDimensionsBody{
				Length: row.Length,
				Width:  row.Width,
				Height: row.Height,
				Unit:   row.DimensionsUnit,
			},
			Weight: packagingWeightBody{
				Value: row.Weight,
				Unit:  row.WeightUnit,
			},
			Deactivated:  row.Deactivated,
			Registrant:   row.Registrant,
			RegisteredAt: rfc3339(row.RegisteredAt),
		})
	}
	return bodies
}
