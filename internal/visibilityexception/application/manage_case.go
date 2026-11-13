package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrUnexpectedCaseAdvance 说明案件库交回了封闭集合以外的推进结果。
var ErrUnexpectedCaseAdvance = errors.New("visibility exception: unexpected case advance outcome")

// ManageCaseOutcome 是案件接单、关闭与归并请求的应用处理结果。
type ManageCaseOutcome uint8

const (
	ManageCaseOutcomeInvalid ManageCaseOutcome = iota
	CaseTakenUp
	CaseClosedWithConclusion
	CaseMerged
	// CaseAlreadyInProgress：案件已经被接了，这次接单不改首次响应时间与接单人。
	CaseAlreadyInProgress
	// CaseAlreadyClosed：案件已经关闭（含已归并），这次决定不改它的结论。
	CaseAlreadyClosed
	CaseNotFound
	// CaseMainNotEligible：指名的主案件不在册、已经关闭，或就是这份案件自己——重复案件并不进一份不再处置的案件。
	CaseMainNotEligible
	CaseNotAccepted
	// CaseUndecided：案件册读写不动，没有形成答案。
	CaseUndecided
)

// caseMovedUnderDecision 只在本文件里流转：关闭或归并落库时发现案件已被接单。它不是答复，出不了编排。
const caseMovedUnderDecision ManageCaseOutcome = 255

// settleMoved 把重做一次之后仍然撞上的那一格收成未形成答案。
func settleMoved(result ManageCaseResult) ManageCaseResult {
	if result.outcome == caseMovedUnderDecision {
		return ManageCaseResult{outcome: CaseUndecided}
	}
	return result
}

func (outcome ManageCaseOutcome) String() string {
	switch outcome {
	case CaseTakenUp:
		return "TAKEN_UP"
	case CaseClosedWithConclusion:
		return "CLOSED"
	case CaseMerged:
		return "MERGED"
	case CaseAlreadyInProgress:
		return "ALREADY_IN_PROGRESS"
	case CaseAlreadyClosed:
		return "ALREADY_CLOSED"
	case CaseNotFound:
		return "NOT_FOUND"
	case CaseMainNotEligible:
		return "MAIN_CASE_NOT_ELIGIBLE"
	case CaseNotAccepted:
		return "NOT_ACCEPTED"
	case CaseUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// TakeUpCaseCommand 是一次接单请求。接单时刻取服务端时钟；接单人是认证出来的操作者。
type TakeUpCaseCommand struct {
	TenantID domain.TenantID
	Case     domain.CaseID
	Operator string
}

// CloseCaseCommand 是一次以明确结论关闭案件的请求。结论是异常运营写下的业务结论原文，产品不规定它的取值。
type CloseCaseCommand struct {
	TenantID   domain.TenantID
	Case       domain.CaseID
	Conclusion string
	Operator   string
}

// MergeCaseCommand 是把一份重复案件并进主案件的请求。
type MergeCaseCommand struct {
	TenantID domain.TenantID
	Case     domain.CaseID
	Main     domain.CaseID
	Operator string
}

// ManageCaseResult 交回这次决定之后在册的案件。
type ManageCaseResult struct {
	outcome       ManageCaseOutcome
	exceptionCase *domain.ExceptionCase
}

func (result ManageCaseResult) Outcome() ManageCaseOutcome { return result.outcome }

// Case 在答复带着案件时交回它：形成了推进，或案件已在这次决定想去的那一格之后。
func (result ManageCaseResult) Case() (*domain.ExceptionCase, bool) {
	return result.exceptionCase, result.exceptionCase != nil
}

type ManageCaseDeps struct {
	Cases ports.ExceptionCaseStore
	Clock ports.Clock
}

// ManageCaseHandler 推进一份已建立的异常案件（UC-VE-005 的案件生命周期半边）：接单、关闭、归并。案件的建立不在这里——
// 自动建案只来自分诊规则命中。处置请求、响应目标与受控重开不在本编排：处置请求今天没有生产入口，所以关闭前「未完成的
// 处置请求都已移交或完成取消核对」那一条此刻没有可核的对象。
type ManageCaseHandler struct {
	deps ManageCaseDeps
}

func NewManageCaseHandler(deps ManageCaseDeps) (*ManageCaseHandler, error) {
	if deps.Cases == nil || deps.Clock == nil {
		return nil, errors.New("visibility exception: manage case handler needs a case store and a clock")
	}
	return &ManageCaseHandler{deps: deps}, nil
}

// TakeUp 接单：待响应 → 处理中，首次响应时间在这一刻形成。已在处理中的案件再接一次答已在处理中，不改首次响应时间——
// 响应周期的绩效按第一次接单算。
func (handler *ManageCaseHandler) TakeUp(ctx context.Context, command TakeUpCaseCommand) (ManageCaseResult, error) {
	exceptionCase, outcome := handler.load(ctx, command.TenantID, command.Case, command.Operator)
	if outcome != ManageCaseOutcomeInvalid {
		return ManageCaseResult{outcome: outcome}, nil
	}
	switch exceptionCase.Phase() {
	case domain.CaseInProgress:
		return ManageCaseResult{outcome: CaseAlreadyInProgress, exceptionCase: exceptionCase}, nil
	case domain.CaseClosed:
		return ManageCaseResult{outcome: CaseAlreadyClosed, exceptionCase: exceptionCase}, nil
	}
	from := exceptionCase.Phase()
	if err := exceptionCase.TakeUp(handler.deps.Clock.Now().UTC()); err != nil {
		return ManageCaseResult{outcome: CaseNotAccepted}, nil
	}
	result, err := handler.advance(ctx, command.TenantID, exceptionCase, from, command.Operator, CaseTakenUp)
	// 接单没有「照现在的案件重做一次」这回事：案件动了就按它现在的样子作答。内部的那一格在这里收掉，出不了编排。
	return settleMoved(result), err
}

// Close 以明确结论关闭案件。待响应的案件可以不经接单直接关闭（判定不成立）。关闭不关闭源案件或外部义务。
func (handler *ManageCaseHandler) Close(ctx context.Context, command CloseCaseCommand) (ManageCaseResult, error) {
	result, err := handler.close(ctx, command)
	if err == nil && result.outcome == caseMovedUnderDecision {
		// 读到待响应之后被别人接成了处理中：关闭的决定仍然成立，照现在的案件再做一次。再撞一次就不追了。
		result, err = handler.close(ctx, command)
	}
	return settleMoved(result), err
}

func (handler *ManageCaseHandler) close(ctx context.Context, command CloseCaseCommand) (ManageCaseResult, error) {
	conclusion := strings.TrimSpace(command.Conclusion)
	if conclusion == "" || conclusion == domain.CaseConclusionMerged {
		// 「已归并」这个结论只由归并形成：单写这个词关闭，案件就成了一份没有主案件的已归并案件。
		return ManageCaseResult{outcome: CaseNotAccepted}, nil
	}
	exceptionCase, outcome := handler.load(ctx, command.TenantID, command.Case, command.Operator)
	if outcome != ManageCaseOutcomeInvalid {
		return ManageCaseResult{outcome: outcome}, nil
	}
	if exceptionCase.Phase() == domain.CaseClosed {
		return ManageCaseResult{outcome: CaseAlreadyClosed, exceptionCase: exceptionCase}, nil
	}
	from := exceptionCase.Phase()
	if err := exceptionCase.Close(conclusion, handler.deps.Clock.Now().UTC()); err != nil {
		return ManageCaseResult{outcome: CaseNotAccepted}, nil
	}
	return handler.advance(ctx, command.TenantID, exceptionCase, from, command.Operator, CaseClosedWithConclusion)
}

// Merge 把重复案件并进主案件：本案件以「已归并」结论关闭并指向主案件，两边各自的单据号与历史都不动。主案件必须在册、
// 仍在处置中、且不是这份案件自己。
//
// 两份案件先在本事务里一起锁住、再读：归并读两份、只写一份，不锁的话「甲并进乙」与「乙并进甲」同时到达会各自读到对方
// 仍在处置、各自提交，两份都成了已归并而没有一份还在处置；主案件在读过之后被别人关闭也是同一个窗口。锁住之后这些
// 决定排成先后：后到的那一笔读到的是先到那一笔落下之后的案件。
func (handler *ManageCaseHandler) Merge(ctx context.Context, command MergeCaseCommand) (ManageCaseResult, error) {
	result, err := handler.merge(ctx, command)
	if err == nil && result.outcome == caseMovedUnderDecision {
		result, err = handler.merge(ctx, command)
	}
	return settleMoved(result), err
}

func (handler *ManageCaseHandler) merge(ctx context.Context, command MergeCaseCommand) (ManageCaseResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.Case.String()) == "" || strings.TrimSpace(command.Operator) == "" {
		return ManageCaseResult{outcome: CaseNotAccepted}, nil
	}
	if strings.TrimSpace(command.Main.String()) == "" || command.Main == command.Case {
		return ManageCaseResult{outcome: CaseMainNotEligible}, nil
	}
	if err := handler.deps.Cases.LockCases(ctx, command.TenantID, command.Case, command.Main); err != nil {
		return ManageCaseResult{outcome: CaseUndecided}, nil
	}
	exceptionCase, outcome := handler.load(ctx, command.TenantID, command.Case, command.Operator)
	if outcome != ManageCaseOutcomeInvalid {
		return ManageCaseResult{outcome: outcome}, nil
	}
	if exceptionCase.Phase() == domain.CaseClosed {
		return ManageCaseResult{outcome: CaseAlreadyClosed, exceptionCase: exceptionCase}, nil
	}
	main, found, err := handler.deps.Cases.FindCase(ctx, command.TenantID, command.Main)
	if err != nil {
		return ManageCaseResult{outcome: CaseUndecided}, nil
	}
	if !found || main.Phase() == domain.CaseClosed {
		return ManageCaseResult{outcome: CaseMainNotEligible}, nil
	}
	from := exceptionCase.Phase()
	if err := exceptionCase.MergeInto(command.Main, handler.deps.Clock.Now().UTC()); err != nil {
		return ManageCaseResult{outcome: CaseNotAccepted}, nil
	}
	return handler.advance(ctx, command.TenantID, exceptionCase, from, command.Operator, CaseMerged)
}

// load 读回案件。交回的 outcome 为零值表示读到了；否则是该直接作答的那一格。
func (handler *ManageCaseHandler) load(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CaseID,
	operator string,
) (*domain.ExceptionCase, ManageCaseOutcome) {
	if strings.TrimSpace(tenant.String()) == "" || strings.TrimSpace(id.String()) == "" || strings.TrimSpace(operator) == "" {
		return nil, CaseNotAccepted
	}
	exceptionCase, found, err := handler.deps.Cases.FindCase(ctx, tenant, id)
	if err != nil {
		return nil, CaseUndecided
	}
	if !found {
		return nil, CaseNotFound
	}
	return exceptionCase, ManageCaseOutcomeInvalid
}

// advance 落这次推进。册上已不是读到时的那一格，说明另一笔决定先落了：重读一遍，按案件现在的样子作答，不盖掉先落的。
func (handler *ManageCaseHandler) advance(
	ctx context.Context,
	tenant domain.TenantID,
	exceptionCase *domain.ExceptionCase,
	from domain.CasePhase,
	operator string,
	formed ManageCaseOutcome,
) (ManageCaseResult, error) {
	advanced, err := handler.deps.Cases.AdvanceCase(ctx, ports.CaseAdvance{
		Tenant: tenant, Case: exceptionCase, From: from, Operator: strings.TrimSpace(operator),
	})
	if err != nil {
		return ManageCaseResult{outcome: CaseUndecided}, nil
	}
	switch advanced {
	case ports.CaseAdvanced:
		return ManageCaseResult{outcome: formed, exceptionCase: exceptionCase}, nil
	case ports.CasePhaseMoved:
		current, found, err := handler.deps.Cases.FindCase(ctx, tenant, exceptionCase.ID())
		if err != nil || !found {
			return ManageCaseResult{outcome: CaseUndecided}, nil
		}
		if current.Phase() == domain.CaseClosed {
			return ManageCaseResult{outcome: CaseAlreadyClosed, exceptionCase: current}, nil
		}
		if formed == CaseTakenUp && current.Phase() == domain.CaseInProgress {
			return ManageCaseResult{outcome: CaseAlreadyInProgress, exceptionCase: current}, nil
		}
		// 案件从待响应被别人接成了处理中，而这一笔是关闭或归并：它的决定仍然成立，只是基于的那一格过时了。交回内部的
		// 「案件动了」，由 Close 与 Merge 照现在的案件再做一次。
		return ManageCaseResult{outcome: caseMovedUnderDecision}, nil
	default:
		return ManageCaseResult{}, fmt.Errorf("%w: %d", ErrUnexpectedCaseAdvance, advanced)
	}
}
