package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是本上下文取单据号的消费侧端口（ADR-0025；ADR-0355 决定三、七、八）。号归单据所属的上下文、随单落在本上下文
// 自己的表里；取号编排是编号技术组件的，经适配器接进来。异常案件与客户索赔项各一个端口：两类单据的开立编排各只该看见
// 自己那一类，共用一个按类型分派的入口就让分诊编排也能替索赔取号。

// DocumentNumberIssueOutcome 是一次取号的答案。
type DocumentNumberIssueOutcome uint8

const (
	DocumentNumberIssueOutcomeInvalid DocumentNumberIssueOutcome = iota
	// DocumentNumberAssigned：取到了号，随单落库。
	DocumentNumberAssigned
	// DocumentNumberingTimeZoneNotConfigured：开立方没有业务时区、租户也没有生效的单据编号时区。单据照常开立、不带号，
	// 记无号原因；没有动任何序列。
	DocumentNumberingTimeZoneNotConfigured
	// DocumentNumberingRuleCollision：规则渲染出的号连着撞上已用的号，到了跳号上限。这一次开立不落单。
	DocumentNumberingRuleCollision
)

func (outcome DocumentNumberIssueOutcome) String() string {
	switch outcome {
	case DocumentNumberAssigned:
		return "NUMBER_ASSIGNED"
	case DocumentNumberingTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	case DocumentNumberingRuleCollision:
		return "NUMBERING_RULE_COLLISION"
	default:
		return ""
	}
}

// DocumentNumberIssue 是一次取号的结果。Number 只在取到号时非空；Template 是这一次所用的模板原文，撞号时据它说清是
// 哪一份格式撞上了旧号。
type DocumentNumberIssue struct {
	Outcome  DocumentNumberIssueOutcome
	Number   string
	Template string
}

// CaseNumberIssuer 为一件要建立的异常案件取单据号（类型 `EXCEPTION_CASE`，日期按租户的单据编号时区）。
//
// 只许在开立事务里、在确定要落案件的那一步调（ADR-0355 决定三）。答撞号时，实现已把这一次从序列取过的值全部退回——
// 开立事务里同时落着别的东西（发作期派生、投影），不能为一件案件撞号就让整笔回滚，所以退回放在取号这一步自己身上；
// 调用方只需不落案件、把命中记进案件撞号册（CaseNumberingCollisionRecorder）。交回错误时调用方必须回滚开立事务。
type CaseNumberIssuer interface {
	IssueCaseNumber(ctx context.Context, tenant domain.TenantID) (DocumentNumberIssue, error)
}

// ClaimNumberIssuer 为一项要记录的客户索赔取单据号（类型 `CLAIM`，日期按租户的单据编号时区）。契约同 CaseNumberIssuer。
type ClaimNumberIssuer interface {
	IssueClaimNumber(ctx context.Context, tenant domain.TenantID) (DocumentNumberIssue, error)
}

// CaseNumberingCollision 是一次因案件单据号撞号而没建成案件的分诊命中（ADR-0355 决定八）。撞号时发作期、结论与案件一样
// 都不落，命中却不能丢：信号多半只来一次（一次违规出发、一处分叉），没有谁会再把它送进来，所以本上下文把它记下，等规则册
// 换了格式（或周期换了、旧号让出）再按原样重新分诊。Hit 是这一原因第一次撞号的那次命中——同一原因再撞不改写它，重驱按它
// 开期（CONTEXT「原样重新分诊」）；Template 是最近一次撞上的那份格式，给看日志、查册的人指认。
type CaseNumberingCollision struct {
	Tenant   domain.TenantID
	Hit      domain.SignalHit
	Template string
	// At 是撞号那一刻的服务端时刻。
	At time.Time
}

// CaseNumberingCollisionKey 是案件撞号册的键：与发作期定位同一把（租户、对象、类型、因果条件）——同一原因再撞一次是同一
// 件事，不另起一行。
type CaseNumberingCollisionKey struct {
	Tenant domain.TenantID
	Parcel domain.TrackedParcelReference
	Kind   domain.ExceptionSignalKindReference
	Cause  domain.CausalConditionReference
}

// Key 交回这次撞号在册上的键。
func (collision CaseNumberingCollision) Key() CaseNumberingCollisionKey {
	return CaseNumberingCollisionKey{Tenant: collision.Tenant, Parcel: collision.Hit.Parcel, Kind: collision.Hit.Kind, Cause: collision.Hit.Cause}
}

// CaseNumberingCollisionRecorder 是分诊编排看见的案件撞号册。
//
// HoldCaseNumberingCollision 在分诊事务开头锁住这一原因在册上的那一行（若有；没有就什么都不锁）：重驱与另一份正常命中都会
// 对同一原因开期，谁先锁到谁走，后到的等先到的提交——先到的建了案、同笔销了那一行，后到的再读发作期就看见它，只记一次
// 命中，不另开一期、不另立一案。必须排在找发作期之前，否则后到的已按「没有发作期」作了判断。
//
// RecordCaseNumberingCollision 在开立事务里记下一次撞号：同键再撞只加撞号次数、换成最近撞上的格式并让它马上到期，首次命中
// 的各样不动；撞着的那一刻下一次重驱就再试一回，有人看得见。写库失败按端口惯例交回错误。
type CaseNumberingCollisionRecorder interface {
	HoldCaseNumberingCollision(ctx context.Context, key CaseNumberingCollisionKey) error
	RecordCaseNumberingCollision(ctx context.Context, collision CaseNumberingCollision) error
}

// CaseNumberingCollisionQueue 是重驱那一侧看见的案件撞号册：交出到期的、按时推后、重驱成了就销掉。到期判据只看服务端
// 时刻，跨租户取——重驱在派发进程里统一跑，每一行自带租户。
//
// ClaimDueCaseNumberingCollision 在重驱事务里抢这一行：锁得到、仍在册、仍到期才交回（交回的是此刻库里那一行，不是取页时
// 那一份）；别的派发进程正锁着、或已被销掉推后，答 false、不等。滚动发布时新旧派发进程短暂并存，同一页会被两边各取一份，
// 靠这一抢只有一边真去分诊。
type CaseNumberingCollisionQueue interface {
	DueCaseNumberingCollisions(ctx context.Context, at time.Time, limit int) ([]CaseNumberingCollision, error)
	ClaimDueCaseNumberingCollision(ctx context.Context, key CaseNumberingCollisionKey, at time.Time) (CaseNumberingCollision, bool, error)
	PostponeCaseNumberingCollision(ctx context.Context, key CaseNumberingCollisionKey, until time.Time) error
	ResolveCaseNumberingCollision(ctx context.Context, key CaseNumberingCollisionKey) error
}
