// Package warehouseswitch 是仓配履约部署开关在组合根一侧的唯一读法与启动日志（ADR-0375 决定四）。
//
// 它只能落在组合根一侧：开关是部署方的断言——这个部署提供仓配——不是租户取值，领域包不读环境。放在 cmd/internal
// 而不是写进某一个进程：`parcel-api` 与 `parcel-dispatch` 要读同一个变量、按同一条规则读，各写一遍就是两份会各自走样
// 的读法，而同一个部署里两个进程对「仓配开没开」答得不一样，比哪一边都没开更难查。
package warehouseswitch

import (
	"log/slog"
	"strings"
)

// EnabledEnv 是仓配履约的部署开关。产品给的缺省是关；这个名字是部署清单与运维文档照着写的外部契约，改名要连同它们一起改。
const EnabledEnv = "IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED"

// Enabled 读开关，规则照 ADR-0352 决定二：去首尾空白后恰为 `true` 才开。缺省、空、大小写不同、`1`、`yes` 都按关读——
// 开错的代价（没买仓配的部署里装进了仓配的路）远大于没开（改过开关再重启即可，库表不随开关变）。
//
// 各进程在启动时读一次，值经装配交给各特性；特性自己不再读环境。
func Enabled(getenv func(string) string) bool {
	return strings.TrimSpace(getenv(EnabledEnv)) == "true"
}

// LogStartup 把开关的取值写进启动日志，开与关都出声：关着时仓配的路与端点都没有，看上去与「开着而没人用」一样安静，
// 启动日志是分辨两者的第一处；开着则是部署方断言了这个部署提供仓配，事后要查得出哪一次启动开的。
//
// logger 为 nil 即不出声，调用方不必先判空。
func LogStartup(logger *slog.Logger, enabled bool) {
	if logger == nil {
		return
	}
	if enabled {
		logger.Info("Warehouse fulfillment is enabled (ADR-0375): this deployment provides the optional capability",
			"env", EnabledEnv)
		return
	}
	logger.Info("Warehouse fulfillment is not enabled (ADR-0375): this deployment registers none of it until the switch is set to true",
		"env", EnabledEnv)
}
