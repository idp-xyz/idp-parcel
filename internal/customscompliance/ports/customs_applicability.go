package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件声明「关务适用性判断」这一口与外部的关系（票 routing-first-cut/12）：消费方
// （network-routing 的 adapters/customscompliance）按候选交来投影，判断服务交回逐候选
// 作答；两本目录的全量快照是判断的输入。判断词形在 domain，这里只声明出入形状与读口。

// CustomsApplicabilityCandidate 是交来作答的一条候选：候选引用与两端国家/地区码。段链
// 不带进本口——首版候选级作答不看段链（分诊裁定一/二），段级口岸匹配等区域维建模后再
// 议；不带用不上的投影，也免得本口替提供方再存一份段链词形。
type CustomsApplicabilityCandidate struct {
	Candidate      domain.RouteCandidateReference
	Origin         string
	HasOrigin      bool
	Destination    string
	HasDestination bool
}

// CustomsApplicabilityQuery 是一次关务适用性询问：哪个租户、哪一刻、哪些候选。
type CustomsApplicabilityQuery struct {
	Tenant     domain.TenantID
	AsOf       time.Time
	Candidates []CustomsApplicabilityCandidate
}

// PortsPathsSnapshot 是两本目录在册的全部已登记行（不过滤时点、不按点读）。全量而不
// 只答生效行的理由：`口岸未登记`需要查到特定口岸**任何时点**都没有行，`口岸未生效`需
// 要它在册而行不覆盖判断时点，两本皆空是`目录为空`——三种答案续办动作不同，读口只交
// 全量快照，判断服务才折得开。
type PortsPathsSnapshot struct {
	Ports []CandidatePortEntry
	Paths []DeclarationPathEntry
}

// PortsPathsSnapshotView 取某个租户两本目录的全量快照。空快照是「该租户什么都没登记」
// 的如实答案，不是未决；依赖调不通作为错误返回——判断服务把它折成状态未知（目录读不
// 到），不冒充「查无记录」。
type PortsPathsSnapshotView interface {
	LoadPortsPathsSnapshot(
		ctx context.Context,
		tenant domain.TenantID,
	) (PortsPathsSnapshot, error)
}
