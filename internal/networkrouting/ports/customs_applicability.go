package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// CustomsCandidateLeg 是交给关务来源的一段：网络连接身份与它在判断时点的起讫节点。
type CustomsCandidateLeg struct {
	Connection string
	FromNode   string
	ToNode     string
}

// CustomsCandidate 是交给关务来源作答的一条路由候选：候选标识、随请求携带的两端国家/
// 地区（寄件/收件国，ADR-0148 决定二的地理解析投影派生）与它的段链（按序）。缺国家码
// 如实交缺席格——CC 按「证据不足」答状态未知（routing-first-cut/12 分诊裁定二）。
type CustomsCandidate struct {
	Candidate      domain.CandidateID
	Origin         string
	HasOrigin      bool
	Destination    string
	HasDestination bool
	Legs           []CustomsCandidateLeg
}

// CustomsApplicabilityQuery 是一次关务适用性询问：哪个租户、哪一刻、哪些候选。不带判断键——可达性与初始路由
// 共用同一个来源，关务一侧要的只是租户、时点与候选段链。
type CustomsApplicabilityQuery struct {
	Tenant     domain.TenantID
	AsOf       time.Time
	Candidates []CustomsCandidate
}

// CustomsApplicabilityAssessment 是关务来源一次交回的完整答复：逐候选硬约束事实加逐候
// 选出处（ADR-0148 决定一「端口取回的事实带出处」）。出处是判断标识与所用目录版本引用，
// 随判断记录留痕；状态未知与淘汰的事实也各自带出处。
type CustomsApplicabilityAssessment struct {
	Findings  []domain.HardConstraintFinding
	Citations []domain.CustomsApplicabilityCitation
}

// CustomsApplicabilitySource 取路由候选的关务适用性（ADR-0148 决定一「经端口取」、决定三）。关务区域、口岸与
// 申报路径是否合规可用是 customs-compliance 的判断，本上下文只在它答过的候选里选，不自行推断候选是否跨关务区域。
//
// 逐候选作答：每条候选至少一条硬约束事实——满足、适用限制（指名限制来源）或状态未知（缺什么、何时再判）。
// 漏答一条不等于满足；来源未接时如实答状态未知，不答满足。error 只表示依赖调不通。出处随答复交回：已接的
// 来源逐候选带出处；来源未接时出处缺席是如实形态，不代拟一份。
type CustomsApplicabilitySource interface {
	AssessCustomsApplicability(ctx context.Context, query CustomsApplicabilityQuery) (CustomsApplicabilityAssessment, error)
}
