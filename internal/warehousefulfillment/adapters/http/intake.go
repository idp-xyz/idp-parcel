package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
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

// LocationRegistrationIntake 把一次已认证的接入请求翻译成库位修订的登记命令。理由同 WarehouseRegistrationIntake：租户与登记责任方
// 只能来自认证结果，认证先于请求体；认证与译装的做法各册共用同一份（OperatorRegistryIntake），渠道未配置时同样挂 UnconfiguredIntake。
type LocationRegistrationIntake interface {
	IntakeLocationRegistration(ctx context.Context, request *http.Request) (application.RegisterLocationCommand, error)
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
	// Warehouses 为零值时不切仓。操作者渠道要放进信封上的范围：没授是 Restricted 且 Codes 为空。
	Warehouses ports.WarehouseScope
}

// CatalogueIntake 把一次已认证的查阅请求翻译成查询。仓的各读口（各仓当前修订、一个仓的修订史）同属主数据与运营查阅，
// 共用一个 Intake：分设只会让装配点看起来能只配一半。商品的运营查阅也用这一只。
type CatalogueIntake interface {
	IntakeCatalogueQuery(ctx context.Context, request *http.Request) (CatalogueQuery, error)
}

// GoodsOnBehalfAuthenticator 认证一次商品代录：核「商品代录」授予与准入范围，交回租户与登记责任方。失败只交回本包的格。
type GoodsOnBehalfAuthenticator interface {
	AuthenticateGoodsOnBehalf(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// GoodsOnBehalfIntake 把一次已认证的代录请求翻译成商品登记命令。货主客户账户、决定标识与依据引用是载荷里的业务内容；
// 租户与登记责任方只来自认证结果。认证先于请求体。
type GoodsOnBehalfIntake interface {
	IntakeGoodsOnBehalf(ctx context.Context, request *http.Request) (application.RegisterGoodsCommand, error)
}

// PreAdviceOnBehalfAuthenticator 认证一次入库预报代录：核「入库预报代录」授予与准入范围，交回租户与登记责任方。
type PreAdviceOnBehalfAuthenticator interface {
	AuthenticatePreAdviceOnBehalf(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// PreAdviceCloseAuthenticator 认证一次入库预报完结：核「入库预报完结」授予与准入范围，交回租户与操作者。
type PreAdviceCloseAuthenticator interface {
	AuthenticatePreAdviceClose(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// ClosePreAdviceIntake 把一次已认证的完结请求翻译成命令。认证先于请求体。载荷只有预报标识，说明可缺。
type ClosePreAdviceIntake interface {
	IntakeClosePreAdvice(ctx context.Context, request *http.Request) (application.ClosePreAdviceCommand, error)
}

// PreAdviceCloser 是完结口转交的编排。事务边界在编排侧给出。
type PreAdviceCloser interface {
	ClosePreAdvice(ctx context.Context, command application.ClosePreAdviceCommand) (application.ClosePreAdviceResult, error)
}

// PreAdviceOnBehalfIntake 把一次已认证的代录请求翻译成预报的提交、告知更新或撤回。三个写口共用这一只，认证先于请求体。
// 代录撤回的键由预报标识构造，不从载荷收决定标识。
type PreAdviceOnBehalfIntake interface {
	IntakePreAdvice(ctx context.Context, request *http.Request) (application.AcceptPreAdviceCommand, error)
	IntakePreAdviceNotice(ctx context.Context, request *http.Request) (application.UpdatePreAdviceNoticeCommand, error)
	IntakePreAdviceWithdrawal(ctx context.Context, request *http.Request) (application.WithdrawPreAdviceCommand, error)
}
