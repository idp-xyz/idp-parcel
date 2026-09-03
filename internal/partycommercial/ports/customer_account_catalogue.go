package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CustomerAccountRow 是客户与合同页「客户账户」签上列的一行：一个货主客户账户的最新登记
// 修订，连同其客户参与方身份的名称转写（票 admin-write-faces/04）。
//
// 它与 GroupLegalEntityRow 同形而不与 BusinessPartyRow 同形：账户与法人一样**钉在**一个
// 参与方身份上（CONTEXT「货主客户账户登记必须显式关联其客户参与方」），名称在参与方册，
// 这里只登引用——所以有 HasPartyName 那一格，参与方册查无此人是写入用例把门失败才会出现
// 的悬空引用，目录如实上列不遮掩。
//
// Status 是装载时点对生命周期事实的导出（domain.IdentityLifecycle.StatusAt 的 SQL 镜像）：
// REGISTERED / EFFECTIVE / DEACTIVATED。停用两件只在 HasDeactivation 为真时有意义——零时刻
// 是合法时刻，不拿零值兼作「没停用」。
type CustomerAccountRow struct {
	TenantID          string
	AccountID         string
	CustomerPartyID   string
	CustomerPartyName string
	HasPartyName      bool
	Status            string
	Revision          int
	Basis             string
	EffectiveFrom     time.Time
	DeactivatedAt     time.Time
	DeactivationBasis string
	HasDeactivation   bool
	RegisteredAt      time.Time
}

// CustomerAccountCatalogueRead 是货主客户账户目录的伴生列表读端口（ADR-0077）：管理台
// party-contracts 页「客户账户」签的供数面。
//
// 它是参与方身份三册里的第三册，却不并进 PartyIdentityCatalogueRead 那三个方法。两条理由，
// 一条是页面所有权，一条是共享树上的改动形状：
//
//   - 那个端口自述是 group-legal-entities 与 business-parties 两页的供数面，而本册按 CONTEXT
//     落在客户与合同页——账户「面向一个货主客户建立」、引用一个参与方身份而不是那个角色中立
//     的身份本体（票 04 的落点裁定）。一页一入口，读口跟着页走。
//   - 给既有接口加一个方法要同笔改所有实现方与替身（cmd/parcel-api 的 unwired 占位、http 的
//     测试替身），在多会话共写的树上是一次跨包红窗。独立端口是三步法的 expand 步，任何一刻
//     停下来都编得过；要不要在收口时并回去，由那时的树状态定，不在这里预支。
//
// 上列对象是**最新修订**：目录回答「这个租户今天有哪些账户、各处哪格」，修订史是登记册的
// 证据面。租户在签名上、Limit 非正拒、空册答空列表，判据同 PartyIdentityCatalogueRead。
type CustomerAccountCatalogueRead interface {
	ListCustomerAccounts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CustomerAccountRow, error)
}
