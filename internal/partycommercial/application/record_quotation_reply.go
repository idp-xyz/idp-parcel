package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 运营代录一条报价答复。代录人与登记时刻由调用方交已经取定的信封和时钟，载荷里的自报值到不了这里。
// 已有答复先于过期和撤下：同一条再交来要交回原条，即使所引草稿后来撤下。过期与撤下只挡还没有答复的那一次。
// 过期按客户答复时刻，用发出记录上的有效期，起点算在内、终点算在外。不引入业务时区。
// 读不动发出册或价格草稿册是错误，不是业务拒绝。已登记的价卡不读撤下这一格。

// RecordQuotationReplyOutcome 是代录的业务结论。RECORDED 才新落一行。
type RecordQuotationReplyOutcome uint8

const (
	RecordQuotationReplyOutcomeInvalid RecordQuotationReplyOutcome = iota
	QuotationReplyRecorded
	QuotationReplyReplay
	QuotationReplyAlreadyReplied
	QuotationReplyExpired
	QuotationReplyWithdrawn
	QuotationReplyContentChanged
	QuotationReplyNotAccepted
	QuotationReplyNotFound
)

func (outcome RecordQuotationReplyOutcome) String() string {
	switch outcome {
	case QuotationReplyRecorded:
		return "RECORDED"
	case QuotationReplyReplay:
		return "ALREADY_RECORDED"
	case QuotationReplyAlreadyReplied:
		return "ALREADY_REPLIED"
	case QuotationReplyExpired:
		return "EXPIRED"
	case QuotationReplyWithdrawn:
		return "WITHDRAWN"
	case QuotationReplyContentChanged:
		return "CONTENT_CHANGED"
	case QuotationReplyNotAccepted:
		return "NOT_ACCEPTED"
	case QuotationReplyNotFound:
		return "NOT_FOUND"
	default:
		return ""
	}
}

// RecordQuotationReplyCommand 是一次代录。客户时刻、证据索引与决定由代录人交来。
type RecordQuotationReplyCommand struct {
	Tenant          domain.TenantID
	QuotationID     domain.QuotationID
	Decision        domain.QuotationReplyDecision
	RepliedAt       time.Time
	EvidenceIndex   string
	RejectionReason string
	ContentDigest   string
	Recorder        domain.OperatorSubjectReference
	RecordedAt      time.Time
}

// RecordQuotationReplyResult 是代录的答案。重放时 Reply 是原先记下的那一条。
type RecordQuotationReplyResult struct {
	Outcome    RecordQuotationReplyOutcome
	Conclusion string
	Reply      domain.QuotationReply
}

// RecordQuotationReplyHandler 编排发出记录、所引草稿与答复册。
type RecordQuotationReplyHandler struct {
	sends   ports.QuotationSendStore
	replies ports.QuotationReplyStore
	prices  ports.QuotedPriceReader
}

func NewRecordQuotationReplyHandler(
	sends ports.QuotationSendStore,
	replies ports.QuotationReplyStore,
	prices ports.QuotedPriceReader,
) (*RecordQuotationReplyHandler, error) {
	if sends == nil || replies == nil || prices == nil {
		return nil, fmt.Errorf("party commercial: quotation reply needs the send store, the reply store and the price reader")
	}
	return &RecordQuotationReplyHandler{sends: sends, replies: replies, prices: prices}, nil
}

func (handler *RecordQuotationReplyHandler) Handle(ctx context.Context, command RecordQuotationReplyCommand) (RecordQuotationReplyResult, error) {
	var none RecordQuotationReplyResult
	if command.Tenant.String() == "" || command.QuotationID.String() == "" || command.Decision.String() == "" ||
		command.ContentDigest == "" || command.Recorder.String() == "" || command.RecordedAt.IsZero() {
		return none, fmt.Errorf("party commercial: quotation reply needs tenant, quotation, decision, digest, recorder and recorded time")
	}
	if _, err := domain.NewQuotationEvidenceIndex(command.EvidenceIndex); err != nil {
		return replyRefused(QuotationReplyNotAccepted, "未受理：证据索引去空白后不能空，也不能含控制字符"), nil
	}
	if _, err := domain.NewQuotationRejectionReason(command.RejectionReason); err != nil {
		return replyRefused(QuotationReplyNotAccepted, "未受理：拒绝原因不含控制字符"), nil
	}
	if command.Decision == domain.QuotationReplyAccepted && strings.TrimSpace(command.RejectionReason) != "" {
		return replyRefused(QuotationReplyNotAccepted, "未受理：接受不收原因"), nil
	}
	if command.RepliedAt.IsZero() {
		return replyRefused(QuotationReplyNotAccepted, "未受理：客户答复时刻要有"), nil
	}

	send, found, err := handler.sends.GetQuotationSend(ctx, command.Tenant, command.QuotationID)
	if err != nil {
		return none, err
	}
	if !found {
		return replyRefused(QuotationReplyNotFound, "没有这一条发出记录"), nil
	}
	if send.ContentDigest() != command.ContentDigest {
		return replyRefused(QuotationReplyContentChanged, "内容已变，与发出记录上的摘要不同"), nil
	}

	candidate, err := domain.NewQuotationReply(domain.QuotationReplySpec{
		QuotationID: command.QuotationID, Tenant: command.Tenant, Decision: command.Decision,
		RepliedAt: command.RepliedAt, EvidenceIndex: command.EvidenceIndex, RejectionReason: command.RejectionReason,
		RecordedBy: command.Recorder, RecordedAt: command.RecordedAt,
	})
	if err != nil {
		return none, err
	}
	existing, hasReply, err := handler.replies.GetQuotationReply(ctx, command.Tenant, command.QuotationID)
	if err != nil {
		return none, err
	}
	if hasReply {
		if existing.SameFact(candidate) {
			return RecordQuotationReplyResult{Outcome: QuotationReplyReplay, Conclusion: "已经记下", Reply: existing}, nil
		}
		return replyRefused(QuotationReplyAlreadyReplied, "已有答复"), nil
	}
	if !send.Validity().Contains(command.RepliedAt) {
		return replyRefused(QuotationReplyExpired, "已过期"), nil
	}
	if send.Lane() == domain.QuotationPriceFromApprovedDraft {
		withdrawn, judgeErr := quotedDraftWithdrawn(ctx, handler.prices, send)
		if judgeErr != nil {
			return none, judgeErr
		}
		if withdrawn {
			return replyRefused(QuotationReplyWithdrawn, "已撤下"), nil
		}
	}

	outcome, err := handler.replies.AppendQuotationReply(ctx, candidate)
	if err != nil {
		return none, err
	}
	switch outcome {
	case ports.QuotationReplyAppended:
		return RecordQuotationReplyResult{Outcome: QuotationReplyRecorded, Conclusion: "已记下", Reply: candidate}, nil
	case ports.QuotationReplyAppendReplay:
		stored, found, loadErr := handler.replies.GetQuotationReply(ctx, command.Tenant, command.QuotationID)
		if loadErr != nil {
			return none, loadErr
		}
		if !found {
			return none, fmt.Errorf("party commercial: quotation reply replay did not read back")
		}
		return RecordQuotationReplyResult{Outcome: QuotationReplyReplay, Conclusion: "已经记下", Reply: stored}, nil
	case ports.QuotationReplyAppendConflict:
		return replyRefused(QuotationReplyAlreadyReplied, "已有答复"), nil
	default:
		return none, fmt.Errorf("party commercial: quotation reply append outcome %s", outcome)
	}
}

func replyRefused(outcome RecordQuotationReplyOutcome, conclusion string) RecordQuotationReplyResult {
	return RecordQuotationReplyResult{Outcome: outcome, Conclusion: conclusion}
}
