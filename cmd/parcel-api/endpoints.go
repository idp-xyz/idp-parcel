package main

import (
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。七个处理器已在各上下文的
// adapters/http 成型（PS 提交/撤回、NO 收寄登记、TF 交付登记双端点、VE 视图查询与
// 索赔受理、CC 外部结果接收），它们的构造函数都以 Intake 接口为第一参——来源信封
// 只能来自认证结果，而真实接入渠道的认证方式属 `PAR-INT-01` 待提供。
//
// 因此本函数现在交回空清单：这不是缺口的另一种写法，而是把「未接线」固化成「装配缝
// 已开、逐端点等 Intake」——每个 Intake 实现就位时在此追加一行 httpapi.BusinessEndpoint，
// 不再动路由层或处理器。红线同各 Intake 注释：未决期间不得出现任何「开发用」的采信
// 头部实现。
func assembleBusinessEndpoints() []httpapi.BusinessEndpoint {
	return nil
}
