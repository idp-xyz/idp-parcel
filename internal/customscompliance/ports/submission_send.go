package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 申报发送通道、发送事实与技术回执的读写口（ADR-0342）。

// SubmissionSendEffect 是登一份通道、发送事实或技术回执的落点：新登、同内容重放、同身份异内容冲突。冲突不覆盖。
type SubmissionSendEffect uint8

const (
	SubmissionSendEffectInvalid SubmissionSendEffect = iota
	SubmissionSendRegistered
	SubmissionSendReplayed
	SubmissionSendConflict
)

// DeclarationChannelRegistry 是申报通道册。
type DeclarationChannelRegistry interface {
	SaveDeclarationChannel(ctx context.Context, tenant domain.TenantID, channel domain.DeclarationChannel, at time.Time) (SubmissionSendEffect, error)
	LoadDeclarationChannel(ctx context.Context, tenant domain.TenantID, target string) (domain.DeclarationChannel, bool, error)
}

// SubmissionSendFactStore 是发送事实册。FindByChannelMessage 按通道给的消息号找回那一份尝试的发送事实，给异步回执关联用。
type SubmissionSendFactStore interface {
	SaveSendFact(ctx context.Context, tenant domain.TenantID, fact domain.SubmissionSendFact, at time.Time) (SubmissionSendEffect, error)
	FindSendFact(ctx context.Context, tenant domain.TenantID, version domain.SubmissionVersionID, sequence int) (domain.SubmissionSendFact, bool, error)
	FindSendFactByChannelMessage(ctx context.Context, tenant domain.TenantID, message string) (domain.SubmissionSendFact, bool, error)
}

// ManualLodgement 是一条线下递交留痕：哪一份尝试是谁在线下递交的、凭的是哪份递交凭据。
type ManualLodgement struct {
	Tenant     domain.TenantID
	Version    domain.SubmissionVersionID
	Sequence   int
	LodgedBy   string
	Original   string
	RecordedAt time.Time
}

// ManualLodgementLedger 记线下递交的留痕，与发送事实同一事务落。系统发送的尝试没有这一条。
type ManualLodgementLedger interface {
	RecordManualLodgement(ctx context.Context, lodgement ManualLodgement) error
}

// TechnicalReceiptStore 是技术回执册。
type TechnicalReceiptStore interface {
	SaveTechnicalReceipt(ctx context.Context, tenant domain.TenantID, receipt domain.TechnicalReceipt, at time.Time) (SubmissionSendEffect, error)
	ListTechnicalReceipts(ctx context.Context, tenant domain.TenantID, version domain.SubmissionVersionID, sequence int) ([]domain.TechnicalReceipt, error)
}
