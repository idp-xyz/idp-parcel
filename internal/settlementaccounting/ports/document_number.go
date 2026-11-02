package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// DocumentNumberAssigner 是本上下文取单据号的消费侧端口（ADR-0025；ADR-0355 决定三、四）。组合根把编号组件的取号
// 编排接上来；编排在开立事务的 ctx 里调它，取号与单据落库同一事务，开立回滚号就释放。
type DocumentNumberAssigner interface {
	AssignDocumentNumber(ctx context.Context, request DocumentNumberRequest) (DocumentNumberAssignment, error)
}

// DocumentNumberRequest 是一次取号。Type 用本上下文 domain 里的类型声明（如 domain.StatementDisputeDocumentNumberType），
// 不另写字符串。
type DocumentNumberRequest struct {
	Tenant domain.TenantID
	Type   docnumber.TypeCode
	// IssuerTimeZone 是开立方结算账户对账安排的业务时区（IANA 名）。那个账户没登对账安排时留空，由编号组件退到租户
	// 的单据编号时区。
	IssuerTimeZone string
	// IsUsed 在同一事务里答一个渲染出的号在本类单据表上是否已用（ADR-0355 决定八）。
	IsUsed func(ctx context.Context, number string) (bool, error)
}

// DocumentNumberOutcome 是取号的答案，三格封闭。
type DocumentNumberOutcome uint8

const (
	DocumentNumberOutcomeInvalid DocumentNumberOutcome = iota
	// DocumentNumberAssigned：取到了号，随单落库。
	DocumentNumberAssigned
	// DocumentNumberTimeZoneUnconfigured：开立方没交业务时区，租户也没登单据编号时区。单据照常开立、不带号，记无号
	// 原因（ADR-0355 决定七）；序列一个值也没动。
	DocumentNumberTimeZoneUnconfigured
	// DocumentNumberRuleCollision：跳过到上限仍撞上已用的号（决定八）。这一次开立不落单，编排答「编号规则撞号」，
	// 开立事务必须回滚——取过的流水只有回滚才释放，提交就全成了空号。
	DocumentNumberRuleCollision
)

// DocumentNumberAssignment 是取号的结果。Number 只在 DocumentNumberAssigned 时非零。
type DocumentNumberAssignment struct {
	Outcome DocumentNumberOutcome
	Number  string
}
