// Package ports 声明 network-routing 自有的语义边界。这些只是接口：它们的 PostgreSQL
// 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试用替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// NetworkEvidence 是一次判断所需的版本化网络事实与它们共同来自的那个视图修订。
//
// 端口只取事实不做评估（ADR-0046）：区域明确排除折成淘汰、地址缺信息折成资料不足，这些
// 是领域拥有的评估规则，落在端口后面就落到了适配器手里，而适配器只翻译不判断。事实与
// 修订一次取回而不分多次调用：两次取回之间视图一变，事实与它标的修订就不再来自同一版。
//
// 后续增量按同一形状扩字段（日历/截单、硬约束），不另开第二个取数端口。
type NetworkEvidence struct {
	ServiceAreas      []domain.ServiceAreaResolution
	RouteRequirements []domain.RouteRequirement
	ViewRevision      domain.NetworkViewRevision
}

// NetworkEvidenceView 为一次判断取回版本化网络事实。
//
// 它只回业务事实。调不通、超时、配置读不到都要作为错误返回，由应用层形成`未形成判断`——
// 把技术故障装扮成一个证据缺口，会让它进入`资料不足`统计，而用例明写这两者不能混。
type NetworkEvidenceView interface {
	LoadNetworkEvidence(
		ctx context.Context,
		key domain.ReachabilityJudgmentKey,
	) (NetworkEvidence, error)
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
//
// ViewRevision 同在记录而不在结论里：它是证据出处不是三值判断的一部分，留在记录上供
// 消费方比对「判断形成后视图有没有换代」（CONTEXT「保留……当前修订标识」）。
type ReachabilityJudgmentRecord struct {
	Key          domain.ReachabilityJudgmentKey
	Finding      domain.ReachabilityFinding
	JudgedAt     time.Time
	ViewRevision domain.NetworkViewRevision
}

// ReachabilityJudgmentSaveOutcome 是保存一次判断的封闭写入结果。error 只留给「答不出」，
// 「已有记录」是一个业务答案（ADR-0031 的同一裁决）：`AT-NR-028` 只允许一个结果版本越过
// 提交边界，第二个写入方要按它读回赢家——而一个 error 分不出「库坏了」与「有人先到」，
// 前者该重试，后者重试一万次也还是有人先到。
type ReachabilityJudgmentSaveOutcome uint8

const (
	ReachabilityJudgmentSaveOutcomeInvalid ReachabilityJudgmentSaveOutcome = iota
	ReachabilityJudgmentSaved
	ReachabilityJudgmentAlreadyRecorded
)

func (outcome ReachabilityJudgmentSaveOutcome) String() string {
	switch outcome {
	case ReachabilityJudgmentSaved:
		return "SAVED"
	case ReachabilityJudgmentAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// ReachabilityJudgmentStore 按请求关联找回并保存判断。租户是显式入参、不从 context 里
// 补，因为按 ADR-0003 运营集团租户是最高数据隔离边界，跨越它必须在签名上看得见。
//
// Save 对同一关联只接纳第一份记录；再来的写入答`已有记录`而不覆盖——迟到结果不按到达
// 顺序覆盖原判断（`AT-NR-028`）。
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
	) (ReachabilityJudgmentSaveOutcome, error)
}

// ReachabilityJudgmentHandoffIntent 是一次已提交判断交给发起方一侧适用下游的那份引用。
// 意图由请求关联认领：同一判断无论交几次都是同一份，不是第二份。
type ReachabilityJudgmentHandoffIntent struct {
	Correlation  domain.RequestCorrelationID
	Key          domain.ReachabilityJudgmentKey
	Finding      domain.ReachabilityFinding
	JudgedAt     time.Time
	ViewRevision domain.NetworkViewRevision
}

// ReachabilityJudgmentHandoff 把一份已提交的三值判断交给适用下游——`AT-NR-030` 的意图
// 半边，发布意图这条缝的第三个样本（ADR-0043）。
//
// 本上下文不记意图完没完成：那份状态要与判断同一事务落库才算数，而事务与 outbox 仍阻断于
// ADR-0017 的 Bento 闸门。在那之前重放一律重发同一意图，由下游按请求关联认领。它今天没有
// 实现，唯一的实现是测试用的确定性替身。
type ReachabilityJudgmentHandoff interface {
	HandOffReachabilityJudgment(ctx context.Context, intent ReachabilityJudgmentHandoffIntent) error
}

type Clock interface {
	Now() time.Time
}
