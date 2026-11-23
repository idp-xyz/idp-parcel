package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 报价答复的端口。一条发出至多一条，只增不改。读口不要求事务。

// QuotationReplyAppendOutcome 是写入落点。重放不改行，已有不同答复也不改行。
type QuotationReplyAppendOutcome uint8

const (
	QuotationReplyAppendOutcomeInvalid QuotationReplyAppendOutcome = iota
	QuotationReplyAppended
	QuotationReplyAppendReplay
	QuotationReplyAppendConflict
)

func (outcome QuotationReplyAppendOutcome) String() string {
	switch outcome {
	case QuotationReplyAppended:
		return "APPENDED"
	case QuotationReplyAppendReplay:
		return "REPLAY"
	case QuotationReplyAppendConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// QuotationReplyStore 是答复册。Append 无事务即拒。
type QuotationReplyStore interface {
	AppendQuotationReply(ctx context.Context, reply domain.QuotationReply) (QuotationReplyAppendOutcome, error)
	GetQuotationReply(ctx context.Context, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationReply, bool, error)
}
