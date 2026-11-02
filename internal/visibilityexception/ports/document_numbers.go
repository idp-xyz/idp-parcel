package ports

import (
	"context"

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

// DocumentNumberIssue 是一次取号的结果。Number 只在取到号时非空。
type DocumentNumberIssue struct {
	Outcome DocumentNumberIssueOutcome
	Number  string
}

// CaseNumberIssuer 为一件要建立的异常案件取单据号（类型 `EXCEPTION_CASE`，日期按租户的单据编号时区）。
//
// 只许在开立事务里、在确定要落案件的那一步调（ADR-0355 决定三）。答撞号时，实现已把这一次从序列取过的值全部退回——
// 开立事务里同时落着别的东西（发作期派生、投影），不能为一件案件撞号就让整笔回滚，所以退回放在取号这一步自己身上；
// 调用方只需不落案件。交回错误时调用方必须回滚开立事务。
type CaseNumberIssuer interface {
	IssueCaseNumber(ctx context.Context, tenant domain.TenantID) (DocumentNumberIssue, error)
}

// ClaimNumberIssuer 为一项要记录的客户索赔取单据号（类型 `CLAIM`，日期按租户的单据编号时区）。契约同 CaseNumberIssuer。
type ClaimNumberIssuer interface {
	IssueClaimNumber(ctx context.Context, tenant domain.TenantID) (DocumentNumberIssue, error)
}
