package warehousehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信任何自报身份、不构造命令或查询，一律交回
// ErrAccessChannelNotConfigured（ADR-0055）。它不是被禁的「开发用」采信实现：那条红线禁的是采信自报租户，本类型恰是其反面。
//
// 仓配的部署开关打开、而操作者渠道的发行方参数没设时，装配点挂它。开关关着时不挂它：那时答的是
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，恢复动作是部署方打开开关，不是去配接入渠道。
type UnconfiguredIntake struct{}

var (
	_ WarehouseRegistrationIntake = UnconfiguredIntake{}
	_ ZoneRegistrationIntake      = UnconfiguredIntake{}
	_ CatalogueIntake             = UnconfiguredIntake{}
)

// IntakeWarehouseRegistration 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeWarehouseRegistration(
	context.Context, *http.Request,
) (application.RegisterWarehouseCommand, error) {
	return application.RegisterWarehouseCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeZoneRegistration(context.Context, *http.Request) (application.RegisterZoneCommand, error) {
	return application.RegisterZoneCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}
