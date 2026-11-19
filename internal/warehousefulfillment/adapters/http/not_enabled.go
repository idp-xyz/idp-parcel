package warehousehttp

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// CodeWarehouseFulfillmentNotEnabled 是部署没打开仓配开关时仓配各口答的码（ADR-0375 决定四，ADR-0384 决定七）。
//
// 它与 ACCESS_CHANNEL_NOT_CONFIGURED 各是一格，因为恢复动作不同：那一格要接入方去配接入渠道，这一格要部署方打开开关；沿用前者会
// 把人指到错的地方。状态码取 403、不取 404：404 与「产品没有这个能力」不可分辨，而能力在、只是这个部署没开。
const CodeWarehouseFulfillmentNotEnabled = "WAREHOUSE_FULFILLMENT_NOT_ENABLED"

// NewNotEnabledEndpoint 交回开关关着时仓配一口的固定处理器：方法对的请求一律答 403 问题详情、码
// WAREHOUSE_FULFILLMENT_NOT_ENABLED、不带 `outcome`。它在认证之前答：不读令牌、不读请求体、不读查询串、不读册，对任何调用方、
// 任何请求内容都是同一份——要不然一个有凭据的调用方就能从答复的差别里探出这个部署开着还是关着之外的东西。
//
// 方法门照仓库各端点的做法自守：方法不对仍答 405 并带 Allow，403 不越过它抢答（ADR-0055 对未配置同此）。允许的方法由装配点按
// 那一行声明的方法传入，所以同一路径上方法对错的答复与开关开着时同一份（405 写本包的旧形状问题体，不随开关变）。
//
// 只有这一格的 403 写 RFC 9457 问题详情（ADR-0140 决定三）：它是新立的一格，没有旧形状要对齐；本包别的问题体沿用操作者各族的
// 旧形状（见 problem.go）。
func NewNotEnabledEndpoint(allowedMethod string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, allowedMethod) {
			return
		}
		httpapi.WriteProblem(response, request, http.StatusForbidden, CodeWarehouseFulfillmentNotEnabled)
	})
}
