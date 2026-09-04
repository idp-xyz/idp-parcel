package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// ReferenceSeriesLatestVersionLoader 读一条序列最近登记的一版，供来源连接器作整版重述的前一版。
//
// 「最近」按登记时刻，同刻按版本号字典序——不按复核（在用版本是另一问，归
// ReferenceSeriesInForceResolver）：重述要接的是册上最完整的那一版，而每一版都整版重述自起点
// 以来的全部期次（CONTEXT），最近登记的一版因此就是最完整的一版，无论它复核了没有。
//
// 另立接口而不扩 ReferenceSeriesVersionLoader，理由同 reference_series_register.go 那一条：
// 扩既有接口会拆全部替身。没有任何版本答 false 不答 error——那就是首版。
type ReferenceSeriesLatestVersionLoader interface {
	LoadLatestVersion(ctx context.Context, tenant domain.TenantID, seriesID string) (domain.ReferenceSeriesRegistration, bool, error)
}
