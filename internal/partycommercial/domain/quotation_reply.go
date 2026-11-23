package domain

import (
	"strings"
	"time"
	"unicode"
)

// 答复指名一条已经发出的报价，只增不改。它记下接受或拒绝、客户答复的时刻、证据索引、
// 代录人与登记时刻。没有状态列：待答复、已接受、已拒绝、已过期、所引价格已撤下都在读的时候派生。
// 证据只存索引，文件字节不进这条记录。不复用签署证据：那一套挂在合同或协议版本旁，还要文件名和哈希。

// ErrInvalidQuotationReply 是一条答复的身份立不住：缺发出、缺租户、缺代录人或登记时刻。
// 证据、原因、客户时刻，以及带首尾空白的代录人，立不住走 ErrQuotationNotAccepted。那是未受理，不是这条记录的其余事实。
var ErrInvalidQuotationReply = errQuotation("invalid reply")

// QuotationReplyDecision 是客户的答复。零值无效。接受与拒绝都登记，没有第三格。
type QuotationReplyDecision uint8

const (
	QuotationReplyDecisionInvalid QuotationReplyDecision = iota
	// QuotationReplyAccepted 是客户接受这一条发出。
	QuotationReplyAccepted
	// QuotationReplyRejected 是客户拒绝这一条发出。原因可以缺。
	QuotationReplyRejected
)

func (decision QuotationReplyDecision) String() string {
	switch decision {
	case QuotationReplyAccepted:
		return "ACCEPTED"
	case QuotationReplyRejected:
		return "REJECTED"
	default:
		return ""
	}
}

// QuotationReplyDecisionNamed 按名字认接受或拒绝。
func QuotationReplyDecisionNamed(name string) (QuotationReplyDecision, bool) {
	switch name {
	case QuotationReplyAccepted.String():
		return QuotationReplyAccepted, true
	case QuotationReplyRejected.String():
		return QuotationReplyRejected, true
	default:
		return QuotationReplyDecisionInvalid, false
	}
}

// QuotationEvidenceIndex 是客户书面确认在受限证据库里的索引。去首尾空白后必须非空，不含控制字符。
type QuotationEvidenceIndex struct {
	text string
}

func NewQuotationEvidenceIndex(text string) (QuotationEvidenceIndex, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
		return QuotationEvidenceIndex{}, ErrQuotationNotAccepted
	}
	return QuotationEvidenceIndex{text: trimmed}, nil
}

func (index QuotationEvidenceIndex) Text() string { return index.text }

func (index QuotationEvidenceIndex) valid() bool {
	return index.text != "" && index.text == strings.TrimSpace(index.text) &&
		strings.IndexFunc(index.text, unicode.IsControl) < 0
}

// QuotationRejectionReason 是拒绝原因。去首尾空白后可以为空；非空时不含控制字符。接受不使用它。
type QuotationRejectionReason struct {
	text string
}

func NewQuotationRejectionReason(text string) (QuotationRejectionReason, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" && strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
		return QuotationRejectionReason{}, ErrQuotationNotAccepted
	}
	return QuotationRejectionReason{text: trimmed}, nil
}

func (reason QuotationRejectionReason) Text() string { return reason.text }

// QuotationReplySpec 是立一条答复要的全部事实。代录人与登记时刻由调用方交服务端已经取定的值。
type QuotationReplySpec struct {
	QuotationID     QuotationID
	Tenant          TenantID
	Decision        QuotationReplyDecision
	RepliedAt       time.Time
	EvidenceIndex   string
	RejectionReason string
	RecordedBy      OperatorSubjectReference
	RecordedAt      time.Time
}

// QuotationReply 是一条已经登记的答复。值类型，没有改写方法。
type QuotationReply struct {
	quotationID     QuotationID
	tenant          TenantID
	decision        QuotationReplyDecision
	repliedAt       time.Time
	evidence        QuotationEvidenceIndex
	rejectionReason QuotationRejectionReason
	recordedBy      OperatorSubjectReference
	recordedAt      time.Time
}

func NewQuotationReply(spec QuotationReplySpec) (QuotationReply, error) {
	evidence, err := NewQuotationEvidenceIndex(spec.EvidenceIndex)
	if err != nil {
		return QuotationReply{}, err
	}
	reason, err := NewQuotationRejectionReason(spec.RejectionReason)
	if err != nil {
		return QuotationReply{}, err
	}
	if spec.Decision == QuotationReplyAccepted && reason.Text() != "" {
		return QuotationReply{}, ErrQuotationNotAccepted
	}
	repliedAt := quotationClock(spec.RepliedAt)
	recordedAt := quotationClock(spec.RecordedAt)
	if repliedAt.IsZero() {
		return QuotationReply{}, ErrQuotationNotAccepted
	}
	// 带首尾空白的代录人不去掉，也不换成去掉空白后的另一个人。库要求原文就等于去掉空白后的自己。
	if spec.RecordedBy.String() != strings.TrimSpace(spec.RecordedBy.String()) {
		return QuotationReply{}, ErrQuotationNotAccepted
	}
	reply := QuotationReply{
		quotationID:     spec.QuotationID,
		tenant:          spec.Tenant,
		decision:        spec.Decision,
		repliedAt:       repliedAt,
		evidence:        evidence,
		rejectionReason: reason,
		recordedBy:      spec.RecordedBy,
		recordedAt:      recordedAt,
	}
	if !reply.valid() {
		return QuotationReply{}, ErrInvalidQuotationReply
	}
	return reply, nil
}

func (reply QuotationReply) valid() bool {
	return reply.quotationID.valid() && reply.tenant.valid() && reply.decision.String() != "" &&
		!reply.repliedAt.IsZero() && reply.evidence.valid() && reply.recordedBy.valid() &&
		reply.recordedBy.String() == strings.TrimSpace(reply.recordedBy.String()) &&
		!reply.recordedAt.IsZero() &&
		(reply.decision != QuotationReplyAccepted || reply.rejectionReason.Text() == "")
}

func (reply QuotationReply) QuotationID() QuotationID              { return reply.quotationID }
func (reply QuotationReply) Tenant() TenantID                      { return reply.tenant }
func (reply QuotationReply) Decision() QuotationReplyDecision      { return reply.decision }
func (reply QuotationReply) RepliedAt() time.Time                  { return reply.repliedAt }
func (reply QuotationReply) EvidenceIndex() QuotationEvidenceIndex { return reply.evidence }
func (reply QuotationReply) RejectionReason() QuotationRejectionReason {
	return reply.rejectionReason
}
func (reply QuotationReply) RecordedBy() OperatorSubjectReference { return reply.recordedBy }
func (reply QuotationReply) RecordedAt() time.Time                { return reply.recordedAt }

// SameFact 认的是决定、客户时刻、证据索引与拒绝原因。代录人与登记时刻是这一次登记的痕迹，
// 再交来不相同也不另算一条。
func (reply QuotationReply) SameFact(other QuotationReply) bool {
	return reply.quotationID == other.quotationID && reply.tenant == other.tenant &&
		reply.decision == other.decision && reply.repliedAt.Equal(other.repliedAt) &&
		reply.evidence == other.evidence && reply.rejectionReason == other.rejectionReason
}

// QuotationReadingStatus 是一条发出此刻的读法。零值无效。不入库。
type QuotationReadingStatus uint8

const (
	QuotationReadingStatusInvalid QuotationReadingStatus = iota
	// QuotationReadingPending 还没有答复，所引草稿没撤下，服务端此刻仍在有效期内。
	QuotationReadingPending
	// QuotationReadingAccepted 已经记下接受。之后草稿撤下也不改这一格。
	QuotationReadingAccepted
	// QuotationReadingRejected 已经记下拒绝。
	QuotationReadingRejected
	// QuotationReadingExpired 没有答复，服务端此刻不在有效期内。
	QuotationReadingExpired
	// QuotationReadingPriceWithdrawn 没有答复，所引的已批准草稿此刻已撤下。
	QuotationReadingPriceWithdrawn
)

func (status QuotationReadingStatus) String() string {
	switch status {
	case QuotationReadingPending:
		return "PENDING"
	case QuotationReadingAccepted:
		return "ACCEPTED"
	case QuotationReadingRejected:
		return "REJECTED"
	case QuotationReadingExpired:
		return "EXPIRED"
	case QuotationReadingPriceWithdrawn:
		return "REFERENCED_PRICE_WITHDRAWN"
	default:
		return ""
	}
}

// QuotationReading 是一次读派生出来的结论。ReferencedDraftWithdrawn 只在已接受、
// 且所引草稿此刻已经撤下时为真：状态仍是已接受，不把答复擦掉。
type QuotationReading struct {
	Status                   QuotationReadingStatus
	ReferencedDraftWithdrawn bool
}

// ReadQuotation 按已经取到的事实派生读法。不读时钟、不读册，也不改发出记录。
// 先后：已有接受、已有拒绝、没有答复且所引草稿已撤下、没有答复且此刻不在有效期内、其余待答复。
// 已登记的价卡没有撤下这一格，draftWithdrawnNow 对那一路无效。now 是服务端此刻，不是客户时刻。
func ReadQuotation(send QuotationSend, reply QuotationReply, hasReply, draftWithdrawnNow bool, now time.Time) (QuotationReading, error) {
	if !send.valid() || now.IsZero() {
		return QuotationReading{}, ErrInvalidQuotationReply
	}
	if hasReply {
		if !reply.valid() || reply.quotationID != send.id || reply.tenant != send.tenant {
			return QuotationReading{}, ErrInvalidQuotationReply
		}
		switch reply.decision {
		case QuotationReplyAccepted:
			return QuotationReading{
				Status:                   QuotationReadingAccepted,
				ReferencedDraftWithdrawn: send.lane == QuotationPriceFromApprovedDraft && draftWithdrawnNow,
			}, nil
		case QuotationReplyRejected:
			return QuotationReading{Status: QuotationReadingRejected}, nil
		default:
			return QuotationReading{}, ErrInvalidQuotationReply
		}
	}
	if send.lane == QuotationPriceFromApprovedDraft && draftWithdrawnNow {
		return QuotationReading{Status: QuotationReadingPriceWithdrawn}, nil
	}
	if !send.validity.Contains(now) {
		return QuotationReading{Status: QuotationReadingExpired}, nil
	}
	return QuotationReading{Status: QuotationReadingPending}, nil
}
