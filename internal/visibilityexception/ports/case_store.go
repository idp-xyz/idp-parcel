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
	FindCase(ctx context.Context, tenant domain.TenantID, id domain.CaseID) (*domain.ExceptionCase, bool, error)
	AdvanceCase(ctx context.Context, advance CaseAdvance) (CaseAdvanceOutcome, error)
}
