package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// WarehouseRegistrationIntake 把一次已认证的接入请求翻译成仓修订的登记命令。
//
// 它是接口而不是解析代码：租户与登记责任方只能来自认证结果，采信请求自称的租户会穿透 ADR-0003 的隔离边界。认证先于请求体——
// 令牌不过的请求，请求体一个字节都不读。真实现是操作者渠道的 OperatorRegistryIntake（ADR-0100 决定四），渠道未配置时装配点
// 挂 UnconfiguredIntake。
type WarehouseRegistrationIntake interface {
	IntakeWarehouseRegistration(ctx context.Context, request *http.Request) (application.RegisterWarehouseCommand, error)
}

// ZoneRegistrationIntake 把一次已认证的接入请求翻译成库区修订的登记命令。理由同 WarehouseRegistrationIntake：租户与登记责任方
// 只能来自认证结果，认证先于请求体。各册各有一个这样的接口，因为命令的类型各不相同；认证与译装的做法各册共用同一份
// （OperatorRegistryIntake），渠道未配置时同样挂 UnconfiguredIntake。
type ZoneRegistrationIntake interface {
	IntakeZoneRegistration(ctx context.Context, request *http.Request) (application.RegisterZoneCommand, error)
}

// LocationTypeRegistrationIntake 把一次已认证的接入请求翻译成库位类型修订的登记命令。理由同 WarehouseRegistrationIntake：租户与登记责任方
// 只能来自认证结果，认证先于请求体；认证与译装的做法各册共用同一份（OperatorRegistryIntake），渠道未配置时同样挂 UnconfiguredIntake。
type LocationTypeRegistrationIntake interface {
	IntakeLocationTypeRegistration(ctx context.Context, request *http.Request) (application.RegisterLocationTypeCommand, error)
}

// PackagingMaterialRegistrationIntake 把一次已认证的接入请求翻译成包材修订的登记命令。理由同 WarehouseRegistrationIntake：租户与登记责任方
// 只能来自认证结果，认证先于请求体；认证与译装的做法各册共用同一份（OperatorRegistryIntake），渠道未配置时同样挂 UnconfiguredIntake。
type PackagingMaterialRegistrationIntake interface {
	IntakePackagingMaterialRegistration(ctx context.Context, request *http.Request) (application.RegisterPackagingMaterialCommand, error)
}

// CatalogueQuery 是一次已授权的仓册查阅：租户来自认证结果、不取自查询串，页大小由接入面按渠道契约裁决、不采信调用方自报。
type CatalogueQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// CatalogueIntake 把一次已认证的查阅请求翻译成查询。仓的各读口（各仓当前修订、一个仓的修订史）同属主数据与运营查阅，
// 共用一个 Intake：分设只会让装配点看起来能只配一半。
type CatalogueIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}
