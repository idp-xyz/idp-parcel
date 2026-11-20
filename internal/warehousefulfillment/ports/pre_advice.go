package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// PreAdviceNumberOutcome 是一次取入库预报单据号的答案。三格都是已形成的答案；读不动另走 error。
type PreAdviceNumberOutcome uint8

const (
	PreAdviceNumberOutcomeInvalid PreAdviceNumberOutcome = iota
	// PreAdviceNumberAssigned 是取到了号。
	PreAdviceNumberAssigned
	// PreAdviceNumberTimeZoneNotConfigured 是仓没交业务时区、租户也没有单据编号时区。预报照常受理、不带号。
	PreAdviceNumberTimeZoneNotConfigured
	// PreAdviceNumberRuleCollision 是规则撞号。这一次不落预报。
	PreAdviceNumberRuleCollision
)

func (outcome PreAdviceNumberOutcome) String() string {
	switch outcome {
	case PreAdviceNumberAssigned:
		return "NUMBER_ASSIGNED"
	case PreAdviceNumberTimeZoneNotConfigured:
		return "NUMBERING_TIME_ZONE_NOT_CONFIGURED"
	case PreAdviceNumberRuleCollision:
		return "NUMBERING_RULE_COLLISION"
	default:
		return ""
	}
}

// PreAdviceNumberIssue 是取号的结果。Number 只在取到号时有。
type PreAdviceNumberIssue struct {
	Outcome PreAdviceNumberOutcome
	Number  domain.DocumentNumber
}

// PreAdviceNumberIssuer 在调用方的受理事务里取入库预报的单据号。issuerZone 是仓的业务时区，没登记就是零值。
//
// 只许在确定要落预报的那一步调。答撞号时实现已把这一次从序列取过的值退回，调用方不落预报。
// 答时区未配置时一个序列值也没动。交回错误时调用方必须回滚受理事务。
type PreAdviceNumberIssuer interface {
	IssuePreAdviceNumber(
		ctx context.Context,
		tenant domain.TenantID,
		issuerZone domain.BusinessTimeZone,
	) (PreAdviceNumberIssue, error)
}

// PreAdviceIdentities 签发预报标识。只在决定落册时取，重放不取。
type PreAdviceIdentities interface {
	NextPreAdviceID() (domain.PreAdviceID, error)
}

// PreAdviceWriteOutcome 是落册的结果。键被占是并发下另一笔先写下了同一个判重位置，调用方再读一次按内容作答。
type PreAdviceWriteOutcome uint8

const (
	PreAdviceWriteOutcomeInvalid PreAdviceWriteOutcome = iota
	PreAdviceWritten
	PreAdviceWriteKeyTaken
)

func (outcome PreAdviceWriteOutcome) String() string {
	switch outcome {
	case PreAdviceWritten:
		return "WRITTEN"
	case PreAdviceWriteKeyTaken:
		return "KEY_TAKEN"
	default:
		return ""
	}
}

// PreAdviceStore 是入库预报的册。读用环境里的读执行器；写与两把锁在没有环境事务时拒绝。事务是 READ COMMITTED。
//
// 提交只取判重位置上的锁，告知、撤回、收货缝与完结只取这份预报上的锁，两把不嵌套。
// 读不动走 error，不折成查无。
type PreAdviceStore interface {
	HoldSubmission(ctx context.Context, tenant domain.TenantID, submission domain.PreAdviceSubmission) error
	HoldPreAdvice(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error

	FindBySubmission(
		ctx context.Context,
		tenant domain.TenantID,
		submission domain.PreAdviceSubmission,
	) (domain.PreAdvice, bool, error)
	FindNoticeBySubmission(
		ctx context.Context,
		tenant domain.TenantID,
		submission domain.PreAdviceSubmission,
	) (domain.PreAdviceID, domain.PreAdviceNotice, bool, error)
	FindWithdrawalBySubmission(
		ctx context.Context,
		tenant domain.TenantID,
		submission domain.PreAdviceSubmission,
	) (domain.PreAdviceID, bool, error)
	FindForOwner(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		id domain.PreAdviceID,
	) (domain.PreAdvice, bool, error)
	Find(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) (domain.PreAdvice, bool, error)

	Insert(ctx context.Context, advice domain.PreAdvice) (PreAdviceWriteOutcome, error)
	AppendNotice(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.PreAdviceID,
		notice domain.PreAdviceNotice,
	) (PreAdviceWriteOutcome, error)
	Cancel(ctx context.Context, advice domain.PreAdvice) (PreAdviceWriteOutcome, error)
	MarkReceiving(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error
	MarkClosed(ctx context.Context, tenant domain.TenantID, id domain.PreAdviceID) error
}
