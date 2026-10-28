package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 技术回执接收（ADR-0342 决定四；ADR-0242 决定一至三）：通道异步送来的回执按它给原发送的消息号关联回那一次尝试，逐份追加；尝试
// 此刻的技术判断由发送事实与全部回执折出。接收纪律照 UC-CC-006：同一来源身份同内容答已有，异内容是来源响应冲突、不覆盖，
// 关联不上答待关联、什么都不记。

type ReceiveTechnicalReceiptCommand struct {
	TenantID       domain.TenantID
	Source         string
	ChannelMessage string
	Outcome        domain.AttemptResult
	OccurredAt     time.Time
	ReceivedAt     time.Time
	Digest         string
}

type TechnicalReceiptOutcome uint8

const (
	TechnicalReceiptOutcomeInvalid TechnicalReceiptOutcome = iota
	TechnicalReceiptRecorded
	TechnicalReceiptExisting
	TechnicalReceiptConflict
	// TechnicalReceiptUnattributable：消息号指不回任何一份已发出的尝试，待关联。
	TechnicalReceiptUnattributable
	TechnicalReceiptNotAccepted
	TechnicalReceiptUndecided
)

func (outcome TechnicalReceiptOutcome) String() string {
	switch outcome {
	case TechnicalReceiptRecorded:
		return "RECEIPT_RECORDED"
	case TechnicalReceiptExisting:
		return "EXISTING_RECEIPT"
	case TechnicalReceiptConflict:
		return "RECEIPT_CONFLICT"
	case TechnicalReceiptUnattributable:
		return "UNATTRIBUTABLE"
	case TechnicalReceiptNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case TechnicalReceiptUndecided:
		return "RECEIPT_UNDECIDED"
	default:
		return ""
	}
}

type TechnicalReceiptResult struct {
	outcome  TechnicalReceiptOutcome
	version  domain.SubmissionVersionID
	sequence int
	current  domain.AttemptResult
}

func (result TechnicalReceiptResult) Outcome() TechnicalReceiptOutcome { return result.outcome }

// Attempt 是回执关联到的那一份尝试；待关联与未受理时为零。
func (result TechnicalReceiptResult) Attempt() (domain.SubmissionVersionID, int) {
	return result.version, result.sequence
}

// Current 是记下（或读回）这份回执之后尝试此刻的技术判断。
func (result TechnicalReceiptResult) Current() domain.AttemptResult { return result.current }

type ReceiveTechnicalReceiptDeps struct {
	SendFacts ports.SubmissionSendFactStore
	Receipts  ports.TechnicalReceiptStore
	Clock     ports.Clock
}

type ReceiveTechnicalReceiptHandler struct {
	deps ReceiveTechnicalReceiptDeps
}

func NewReceiveTechnicalReceiptHandler(deps ReceiveTechnicalReceiptDeps) (*ReceiveTechnicalReceiptHandler, error) {
	if deps.SendFacts == nil || deps.Receipts == nil || deps.Clock == nil {
		return nil, fmt.Errorf("customs compliance: receive technical receipt dependency is nil")
	}
	return &ReceiveTechnicalReceiptHandler{deps: deps}, nil
}

// Handle 关联 → 记一份 → 折出当前判断。调用方给环境事务。
func (handler *ReceiveTechnicalReceiptHandler) Handle(
	ctx context.Context,
	command ReceiveTechnicalReceiptCommand,
) (TechnicalReceiptResult, error) {
	if blankTenant(command.TenantID) || strings.TrimSpace(command.ChannelMessage) == "" {
		return TechnicalReceiptResult{outcome: TechnicalReceiptNotAccepted}, nil
	}
	send, found, err := handler.deps.SendFacts.FindSendFactByChannelMessage(ctx, command.TenantID, command.ChannelMessage)
	if err != nil {
		return TechnicalReceiptResult{outcome: TechnicalReceiptUndecided}, nil
	}
	if !found {
		return TechnicalReceiptResult{outcome: TechnicalReceiptUnattributable}, nil
	}
	receipt, err := domain.ReceiveTechnicalReceipt(command.Source, send.Version(), send.Sequence(), command.Outcome,
		command.OccurredAt, command.ReceivedAt, command.Digest)
	if err != nil {
		return TechnicalReceiptResult{outcome: TechnicalReceiptNotAccepted}, nil
	}
	saved, err := handler.deps.Receipts.SaveTechnicalReceipt(ctx, command.TenantID, receipt, handler.deps.Clock.Now())
	if err != nil {
		return TechnicalReceiptResult{outcome: TechnicalReceiptUndecided}, nil
	}
	outcome := TechnicalReceiptRecorded
	switch saved {
	case ports.SubmissionSendReplayed:
		outcome = TechnicalReceiptExisting
	case ports.SubmissionSendConflict:
		return TechnicalReceiptResult{outcome: TechnicalReceiptConflict, version: send.Version(), sequence: send.Sequence()}, nil
	}
	current, err := handler.currentJudgment(ctx, command.TenantID, send)
	if err != nil {
		return TechnicalReceiptResult{outcome: TechnicalReceiptUndecided}, nil
	}
	return TechnicalReceiptResult{outcome: outcome, version: send.Version(), sequence: send.Sequence(), current: current}, nil
}

func (handler *ReceiveTechnicalReceiptHandler) currentJudgment(
	ctx context.Context,
	tenant domain.TenantID,
	send domain.SubmissionSendFact,
) (domain.AttemptResult, error) {
	receipts, err := handler.deps.Receipts.ListTechnicalReceipts(ctx, tenant, send.Version(), send.Sequence())
	if err != nil {
		return domain.AttemptResultInvalid, err
	}
	return domain.TechnicalJudgmentAfterSend(send, receipts)
}
