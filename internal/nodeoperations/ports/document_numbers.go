package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// 本文件是送站预报与送站交接单取单据号的两个消费侧端口（ADR-0025；ADR-0355 决定三、七、八；票 document-numbering/08）：取号编排
// 与开立网点的业务时区。取号编排是编号技术组件（internal/platform/docnumber），网点的业务时区归 network-routing 的网络目录；两者都
// 经本上下文的适配器接进来，编排只认这里的词。

// DocumentNumberRequest 是一次取号。IsUsed 在同一事务里查本类单据表（租户，单据号）是否已用——撞号只可能撞上换格式之前的旧号，
// 判撞要读的是本上下文自己的表（ADR-0355 决定八）。
type DocumentNumberRequest struct {
	Tenant domain.TenantID
	Type   docnumber.TypeCode
	// IssuerTimeZone 是开立网点此刻生效的业务时区（IANA 名）；网点在网络目录里没有生效版本时留空，取号退到租户的单据编号时区。
	IssuerTimeZone string
	IsUsed         func(ctx context.Context, number string) (bool, error)
}

// DocumentNumberIssueOutcome 是取号的封闭答案。
type DocumentNumberIssueOutcome uint8

const (
	DocumentNumberIssueInvalid DocumentNumberIssueOutcome = iota
	// DocumentNumberIssued：取到了号，随单落库。
	DocumentNumberIssued
	// DocumentNumberTimeZoneNotConfigured：网点没有业务时区、租户也没登单据编号时区。单据照常开立、不带号，记无号原因。
	DocumentNumberTimeZoneNotConfigured
	// DocumentNumberRuleCollision：跳过到上限仍撞上已用的号。这一次开立不落单，开立事务必须回滚——取过的流水只有回滚才释放
	// （docnumber.NumberingRuleCollision 的注释）。
	DocumentNumberRuleCollision
)

// DocumentNumberIssue 是一次取号的结果；Number 只在 DocumentNumberIssued 时在场。
type DocumentNumberIssue struct {
	Outcome DocumentNumberIssueOutcome
	Number  domain.DocumentNumber
}

// DocumentNumberIssuer 是取号编排。必须在开立事务的 ctx 里调，且那笔事务与单据落库同一个库：事务外或另一个库的事务里调一律是错误。
// 交回 error 时调用方回滚开立事务，与撞号同办。
type DocumentNumberIssuer interface {
	IssueDocumentNumber(ctx context.Context, request DocumentNumberRequest) (DocumentNumberIssue, error)
}

// IssuingNodeTimeZones 答网点在 at 时刻生效的业务时区（IANA 名）。found=false 即网络目录里此刻没有这个网点的生效版本——单据号的
// 日期因此退到租户的单据编号时区（ADR-0355 决定七「开立方没有业务时区（或没登）的，用租户在规则册登记的单据编号时区」），不拿别
// 的网点、别的版本或进程时区顶。读不动交回 error：此刻判断不了，开立随之不落。
type IssuingNodeTimeZones interface {
	NodeBusinessTimeZoneAt(ctx context.Context, tenant domain.TenantID, node domain.NodeReference, at time.Time) (string, bool, error)
}
