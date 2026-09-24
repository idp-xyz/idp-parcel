// Package governancehttp 是 pilot-governance 的 HTTP 入站适配器——只有查阅面
// （票 admin-skeleton-closure-batch/02，键形依 ADR-0083）。命令面刻意不存在：治理
// 登记走受控 CLI `parcel-governance-register`（syn-wall-door-audit 票 12——登记主体
// 是租户运营方自己，信任边界是运维边界而不是商业渠道），本包永不长出登记端点。
//
// 按 ADR-0022 把结果映射成响应：状态码只回答服务端有没有形成答案，业务判别一律进
// 响应体的 `outcome`。包名与目录名不一致与 shipmenthttp 同理：目录按 ADR-0018 叫
// `adapters/http`，包名叫 `http` 会遮住标准库。
package governancehttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// RegistryQuery 是一次已授权的治理登记册运营查阅。作用域只带登记册实有的维度
// （作用域引用，ADR-0083 Decision 三）——治理册没有租户列，作用域也就没有租户维；
// 页大小由接入面按渠道契约裁决——都不采信调用方自报。
type RegistryQuery struct {
	Scope domain.OperationsQueryScope
	Limit int
}

// RegistryQueryIntake 把一次已认证的运营查阅请求翻译成查询。
//
// 它是接口而非解析代码：请求方身份必须核对，运营接入面的认证归操作者渠道
// （ADR-0100），其真 Intake 未就位；未决期间本包不带任何采信自报身份的实现，包括「开发用」版本。今天在场的
// 两个实现是一对：未配置（如实拒）与隔离读注入（ADR-0078 + ADR-0083 Decision 三）。
type RegistryQueryIntake interface {
	IntakeRegistryQuery(ctx context.Context, request *http.Request) (RegistryQuery, error)
}
