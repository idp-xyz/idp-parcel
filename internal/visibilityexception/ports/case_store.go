package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

type CaseAdvanceOutcome uint8

const (
	CaseAdvanceOutcomeInvalid CaseAdvanceOutcome = iota
	CaseAdvanced
	// CasePhaseMoved：案件在册上已不在这次推进所基于的那一格——另一笔接单、关闭或归并先落了，或这一行不在了。
	CasePhaseMoved
)

// CaseAdvance 是一次要落库的案件推进：推进之后的案件、它推进之前所在的那一格，以及作出这次决定的操作者。
type CaseAdvance struct {
	Tenant domain.TenantID
	Case   *domain.ExceptionCase
	// From 是读到案件时它所在的主状态。落库只在册上仍是这一格时才成：两个人同时接同一份案件、一个接单一个关闭，后落
	// 的那一笔不盖掉先落的。
	From domain.CasePhase
	// Operator 是作出这次决定的操作者（「发行方#sub」），记成接单人或关闭人。
	Operator string
}

// ExceptionCaseStore 读回一份异常案件并落它的推进。案件的建立不在这里：建立与分诊结论同一提交，走 SignalEpisodeStore。
// 租户进签名（ADR-0003）：另一个租户拿着同名案件标识读不到也推不动这一行。
type ExceptionCaseStore interface {
	// LockCases 在本事务里锁住指名的几份案件，直到事务结束；不在册的标识略过。归并读两份案件、只写其中一份：不锁的话
	// 「甲并进乙」与「乙并进甲」各自读到对方仍在处置、各自提交，两份都成了已归并、没有一份还在处置。锁按案件标识的
	// 固定顺序取，两笔对向的归并排成先后而不是互相等死。
	LockCases(ctx context.Context, tenant domain.TenantID, ids ...domain.CaseID) error
	FindCase(ctx context.Context, tenant domain.TenantID, id domain.CaseID) (*domain.ExceptionCase, bool, error)
	AdvanceCase(ctx context.Context, advance CaseAdvance) (CaseAdvanceOutcome, error)
}
