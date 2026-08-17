package main

import (
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。七个处理器已在各上下文的
// adapters/http 成型（PS 提交/撤回、NO 收寄登记、TF 交付登记双端点、VE 视图查询与
// 索赔受理、CC 外部结果接收），它们的构造函数都以 Intake 接口为第一参——来源信封
// 只能来自认证结果，而真实接入渠道的认证方式属 `PAR-INT-01` 待提供。
//
// 按 ADR-0055，本函数不再以空清单等 `PAR-INT-01`：七个端点各以「未配置即拒」的 Intake
// 进装配——不读业务内容、不铸来源信封、不出命令，对每个请求如实答「接入渠道未配置」
// （403 + ACCESS_CHANNEL_NOT_CONFIGURED）。当前的空清单是该批装配落地前的过渡态，落地
// 随评审 081701 排序的「最小业务 API」一步进行；真渠道 Intake 就位时在此逐端点替换，
// 不再动路由层或处理器。红线同各 Intake 注释：不得出现任何「开发用」的采信头部实现；
// 未配置即拒不是那种默认实现——分界同 ADR-0052：「读一个空登记册并如实答未配置不是
// 默认实现，恰恰是它想保护的东西」。
func assembleBusinessEndpoints() []httpapi.BusinessEndpoint {
	return nil
}
