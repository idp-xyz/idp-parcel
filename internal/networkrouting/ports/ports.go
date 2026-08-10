// Package ports 声明 network-routing 自有的语义边界。这些只是接口：它们的 PostgreSQL
// 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试用替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// NetworkEvidenceView 为一次判断装配候选空间与业务证据缺口。
//
// 候选与缺口一次取回而不分两次调用：用例要求`可达`判断也保留其他候选的缺口，两次取回
// 之间视图一变，结论所依据的候选与它记录的缺口就不再来自同一份证据。
//
// 它只回业务事实。调不通、超时、配置读不到都要作为错误返回，由应用层形成`未形成判断`——
// 把技术故障装扮成一个证据缺口，会让它进入`资料不足`统计，而用例明写这两者不能混。
type NetworkEvidenceView interface {
	AssembleCandidates(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
	) ([]domain.RouteCandidate, []domain.EvidenceGap, error)
}

// CommercialEligibilityView 取商业侧对「这个服务要不要判断网络可达性」的回答，覆盖用例
// 步骤 4 的产品形态、责任法人、合同约束与网络使用资格。
//
// network-routing 只消费这个回答，绝不自行推导一个：产品、合同与责任法人都属
// party-commercial，在这里判一次就成了第二处定义，而两处口径迟早会分叉。
//
// 依赖调不通要作为错误返回，由应用层形成`未形成判断`。把它读成「不要求」会让一次商业侧
// 故障变成`不适用`，而用例明写不得以`不适用`代替其他结果，也不得虚构运营网络。
type CommercialEligibilityView interface {
	AssessNetworkEligibility(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
	) (domain.NetworkEligibility, error)
}

// ReachabilityJudgmentRecord 是一次判断越过提交边界后留下的东西。判断时间不在
// `ReachabilityFinding` 里，因为它不是领域结论的一部分——`asOf` 决定按哪一刻的网络证据
// 评估，判断时间只说明这次判断何时作出，压成一个会让重放看起来像新判断。
type ReachabilityJudgmentRecord struct {
	Key      domain.ReachabilityJudgmentKey
	Finding  domain.ReachabilityFinding
	JudgedAt time.Time
}

// ReachabilityJudgmentStore 按请求关联找回并保存判断。租户是显式入参、不从 context 里
// 补，因为按 ADR-0003 运营集团租户是最高数据隔离边界，跨越它必须在签名上看得见。
type ReachabilityJudgmentStore interface {
	FindByCorrelation(
		ctx context.Context,
		tenant domain.TenantID,
		correlation domain.RequestCorrelationID,
	) (ReachabilityJudgmentRecord, bool, error)
	Save(
		ctx context.Context,
		correlation domain.RequestCorrelationID,
		record ReachabilityJudgmentRecord,
	) error
}

type Clock interface {
	Now() time.Time
}
