package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件是申报的线下递交标记（dox/decisions.md「产品形态」）：没有对接报关行或单一窗口的租户，关务人员把申报包导出、
// 在线下递交之后，回来记一笔「已递交」。它落的是与系统发送同一种发送事实，守的是同一组门（尝试仍待发送、还没有发送
// 事实、单元此刻仍就绪且授权仍有效）；差别只在没有通道、没有同步答复——结果恒为`待确认`，海关的受理与放行随后经
// 外部结果的录入口进来。

// ManualLodgementOutcome 是一次线下递交标记的答案。
type ManualLodgementOutcome uint8

const (
	ManualLodgementOutcomeInvalid ManualLodgementOutcome = iota
	// ManualLodgementRecorded：发送事实与递交留痕都记下了。
	ManualLodgementRecorded
	// ManualLodgementAlreadySent：这一份尝试已有发送事实（系统发过，或已经标记过），或尝试本身不是待发送。
	ManualLodgementAlreadySent
	// ManualLodgementHeld：登记之后单元不再就绪或授权已失效，不记。重新取得之后再来。
	ManualLodgementHeld
	// ManualLodgementNotAccepted：提交版本不在册，或命令不成形（没有递交人、没有原件、递交时刻在未来）。
	ManualLodgementNotAccepted
	// ManualLodgementReferenceTaken：填的受理号已经记在另一份尝试上。
	ManualLodgementReferenceTaken
	// ManualLodgementUndecided：某一本册读写不动，重投即可。
	ManualLodgementUndecided
)

func (outcome ManualLodgementOutcome) String() string {
	switch outcome {
	case ManualLodgementRecorded:
		return "LODGEMENT_RECORDED"
	case ManualLodgementAlreadySent:
		return "DECLARATION_ALREADY_SENT"
	case ManualLodgementHeld:
		return "DECLARATION_SEND_HELD"
	case ManualLodgementNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ManualLodgementReferenceTaken:
		return "LODGEMENT_REFERENCE_TAKEN"
	case ManualLodgementUndecided:
		return "LODGEMENT_UNDECIDED"
	default:
		return ""
	}
}

// RecordManualLodgementCommand 指名哪一版提交在线下递交了。LodgedAt 是实际递交的时刻，由递交的人给（记的是已经发生
// 的事，不是录入这一刻），不得晚于现在；不给取现在。Reference 是对方给的受理号，可缺。Original 是递交凭据的编号或
// 存档引用，必给。LodgedBy 只取认证结果。
type RecordManualLodgementCommand struct {
	TenantID  domain.TenantID
	Version   string
	LodgedAt  time.Time
	Reference string
	Original  string
	LodgedBy  string
}

type RecordManualLodgementResult struct {
	outcome ManualLodgementOutcome
	fact    domain.SubmissionSendFact
	hasFact bool
}

func (result RecordManualLodgementResult) Outcome() ManualLodgementOutcome { return result.outcome }

// Fact 在`已记下`与`已发过`（发送事实读得回时）给出。
func (result RecordManualLodgementResult) Fact() (domain.SubmissionSendFact, bool) {
	return result.fact, result.hasFact
}

type RecordManualLodgementDeps struct {
	Submissions ports.DeclarationSubmissionStore
	SendFacts   ports.SubmissionSendFactStore
	Readiness   ports.ReadinessView
	Authority   ports.SubmissionAuthorityView
	Lodgements  ports.ManualLodgementLedger
	Clock       ports.Clock
}

type RecordManualLodgementHandler struct {
	deps RecordManualLodgementDeps
}

func NewRecordManualLodgementHandler(deps RecordManualLodgementDeps) (*RecordManualLodgementHandler, error) {
	if deps.Submissions == nil || deps.SendFacts == nil || deps.Readiness == nil || deps.Authority == nil ||
		deps.Lodgements == nil || deps.Clock == nil {
		return nil, fmt.Errorf("customs compliance: record manual lodgement dependency is nil")
	}
	return &RecordManualLodgementHandler{deps: deps}, nil
}

// manualLodgementResponse 记在发送事实的答复摘要上：这一笔没有通道答复，摘要如实写它是线下递交。
const manualLodgementResponse = "MANUAL_LODGEMENT"

// Record 记一笔线下递交。调用方给环境事务：发送事实与递交留痕同生同灭。
func (handler *RecordManualLodgementHandler) Record(ctx context.Context, command RecordManualLodgementCommand) (RecordManualLodgementResult, error) {
	now := handler.deps.Clock.Now()
	lodgedAt := command.LodgedAt
	if lodgedAt.IsZero() {
		lodgedAt = now
	}
	version, err := domain.NewSubmissionVersionID(command.Version)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.LodgedBy) == "" ||
		strings.TrimSpace(command.Original) == "" || lodgedAt.After(now) {
		return RecordManualLodgementResult{outcome: ManualLodgementNotAccepted}, nil
	}
	undecided := RecordManualLodgementResult{outcome: ManualLodgementUndecided}

	record, found, err := handler.deps.Submissions.FindByVersion(ctx, command.TenantID, version)
	if err != nil {
		return undecided, nil
	}
	if !found {
		return RecordManualLodgementResult{outcome: ManualLodgementNotAccepted}, nil
	}
	attempt := record.Attempt
	if attempt.Result() != domain.AttemptRegisteredPendingSend {
		return RecordManualLodgementResult{outcome: ManualLodgementAlreadySent}, nil
	}
	if fact, sent, err := handler.deps.SendFacts.FindSendFact(ctx, command.TenantID, version, attempt.Sequence()); err != nil {
		return undecided, nil
	} else if sent {
		return RecordManualLodgementResult{outcome: ManualLodgementAlreadySent, fact: fact, hasFact: true}, nil
	}
	if held, err := unitLapsed(ctx, handler.deps.Readiness, handler.deps.Authority, command.TenantID, record.Version.Unit()); err != nil {
		return undecided, nil
	} else if held {
		return RecordManualLodgementResult{outcome: ManualLodgementHeld}, nil
	}

	// 截到微秒：库里的时刻只到微秒，重投同一份时读回的那一份才对得上。
	lodgedAt = lodgedAt.UTC().Truncate(time.Microsecond)
	fact, err := domain.RecordSubmissionSend(version, attempt.Sequence(), lodgedAt, domain.AttemptPendingConfirmation,
		command.Reference, manualLodgementResponse)
	if err != nil {
		// 受理号不成形（过长、带 NUL）：是调用方给的值的问题。
		return RecordManualLodgementResult{outcome: ManualLodgementNotAccepted}, nil
	}
	saved, err := handler.deps.SendFacts.SaveSendFact(ctx, command.TenantID, fact, now)
	if err != nil {
		return undecided, nil
	}
	switch saved {
	case ports.SubmissionSendRegistered:
	case ports.SubmissionSendReplayed:
		return RecordManualLodgementResult{outcome: ManualLodgementAlreadySent, fact: fact, hasFact: true}, nil
	default:
		// 没落成：要么另一方刚给这份尝试记了发送事实，要么受理号已经记在另一份尝试上。
		if stored, found, err := handler.deps.SendFacts.FindSendFact(ctx, command.TenantID, version, attempt.Sequence()); err != nil {
			return undecided, nil
		} else if found {
			return RecordManualLodgementResult{outcome: ManualLodgementAlreadySent, fact: stored, hasFact: true}, nil
		}
		return RecordManualLodgementResult{outcome: ManualLodgementReferenceTaken}, nil
	}
	if err := handler.deps.Lodgements.RecordManualLodgement(ctx, ports.ManualLodgement{
		Tenant: command.TenantID, Version: version, Sequence: attempt.Sequence(),
		LodgedBy: strings.TrimSpace(command.LodgedBy), Original: strings.TrimSpace(command.Original), RecordedAt: now,
	}); err != nil {
		// 发送事实落了而留痕没落：答未决，装配点整笔回滚——不留一笔看不出是线下递交的发送事实。
		return undecided, nil
	}
	return RecordManualLodgementResult{outcome: ManualLodgementRecorded, fact: fact, hasFact: true}, nil
}
