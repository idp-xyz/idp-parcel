package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DocumentNumberIssuer 是本上下文取单据号的消费侧端口（ADR-0025；ADR-0355 决定三、四）。编号是公司级技术组件的能力，
// 本上下文只经这一口要号，号随委托、订舱落在本上下文自己的表里。
//
// 必须在开立事务的 ctx 里调：取号与单据落库同一笔，回滚即释放号码。答编号规则撞号或交回错误时，调用方必须让开立事务
// 回滚——撞号时已从序列取过的值只有回滚才释放。
type DocumentNumberIssuer interface {
	IssueDocumentNumber(ctx context.Context, request DocumentNumberRequest) (DocumentNumberIssue, error)
}

// DocumentNumberRequest 是一次取号：哪个租户、哪一类单据，以及在同一事务里查本类单据表「这个号用过没有」的那一问。
// 委托与订舱的开立方都没有业务时区（domain.TransportCommissionDocumentNumberType 与 domain.BookingDocumentNumberType 的
// 声明），所以这里没有业务时区那一格。
type DocumentNumberRequest struct {
	TenantID domain.TenantID
	Type     docnumber.TypeCode
	IsUsed   func(ctx context.Context, number domain.DocumentNumber) (bool, error)
}

// DocumentNumberIssueOutcome 是取号的答案。
type DocumentNumberIssueOutcome uint8

const (
	DocumentNumberIssueInvalid DocumentNumberIssueOutcome = iota
	// DocumentNumberIssued：取到了号。
	DocumentNumberIssued
	// DocumentNumberTimeZoneNotConfigured：租户没有生效的单据编号时区，单据照常开立、不带号。
	DocumentNumberTimeZoneNotConfigured
	// DocumentNumberRuleCollision：跳过已用号到上限仍撞，这一次开立不落单。
	DocumentNumberRuleCollision
)

func (outcome DocumentNumberIssueOutcome) String() string {
	switch outcome {
	case DocumentNumberIssued:
		return "NUMBER_ASSIGNED"
	case DocumentNumberTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	case DocumentNumberRuleCollision:
		return "NUMBERING_RULE_COLLISION"
	default:
		return ""
	}
}

// DocumentNumberIssue 是一次取号的结果；Number 只在取到号时成立。
type DocumentNumberIssue struct {
	Outcome DocumentNumberIssueOutcome
	Number  domain.DocumentNumber
}

// TransportCommissionNumbers 是委托表上单据号那两列的读写：查（租户，单据号）是否已用，以及给刚落库的那份委托记下号或
// 无号原因。只写这两列——委托的快照、成员与转换各列不经这里。
type TransportCommissionNumbers interface {
	CommissionDocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number domain.DocumentNumber) (bool, error)
	RecordCommissionDocumentNumber(ctx context.Context, key TransportCommissionKey, numbering domain.DocumentNumbering) error
}

// BookingNumbers 是订舱表上单据号那两列的读写，理由同 TransportCommissionNumbers。
type BookingNumbers interface {
	BookingDocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number domain.DocumentNumber) (bool, error)
	RecordBookingDocumentNumber(ctx context.Context, key BookingKey, numbering domain.DocumentNumbering) error
}
