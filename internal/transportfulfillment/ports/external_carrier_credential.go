package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 外部承运凭证登记册（label-channel/18）。CONTEXT「外部承运凭证」要登记的五件事——分配方、真实
// 标识对象、适用范围、版本、替代关系——由 domain.ExternalCarrierCredential 承载；这里是它越过提交
// 边界的形状。ExternalCarrierCredentialResolver（external_tracking_fact.go）由这本册子的适配器实现。

// ExternalCarrierCredentialKey 是一份凭证某一版的幂等键。版本在键里：作废、失效、替代都是新版本，
// 原版本不被改写。
type ExternalCarrierCredentialKey struct {
	TenantID   domain.TenantID
	Credential domain.ExternalCarrierCredentialReference
	Version    domain.ExternalCarrierCredentialVersion
}

// ExternalCarrierCredentialRecord 是一个版本越过提交边界留下的东西。RecordedAt 是登记落库的时刻，
// 与凭证上的适用起点、改变时间都是业务时间，三者不合并。
type ExternalCarrierCredentialRecord struct {
	Key        ExternalCarrierCredentialKey
	Credential domain.ExternalCarrierCredential
	RecordedAt time.Time
}

// ExternalCarrierCredentialSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）；
// 撞键之后是重放还是改内容，由调用方读回既有版本比对——写口只答「这一键已经有了」。
type ExternalCarrierCredentialSaveOutcome uint8

const (
	ExternalCarrierCredentialSaveOutcomeInvalid ExternalCarrierCredentialSaveOutcome = iota
	ExternalCarrierCredentialSaved
	ExternalCarrierCredentialAlreadyRegistered
)

func (outcome ExternalCarrierCredentialSaveOutcome) String() string {
	switch outcome {
	case ExternalCarrierCredentialSaved:
		return "SAVED"
	case ExternalCarrierCredentialAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// ExternalCarrierCredentialRegistry 按键找回并登记凭证版本。只插不改。
//
// FindCurrent 交回一份凭证此刻未被任何版本回指的那一版——「当前」是按回指派生的问答，不是存下来
// 的标记。ListVersions 按登记先后交回全部版本，给读面与冲突核对用。
type ExternalCarrierCredentialRegistry interface {
	FindByKey(ctx context.Context, key ExternalCarrierCredentialKey) (ExternalCarrierCredentialRecord, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		credential domain.ExternalCarrierCredentialReference,
	) (ExternalCarrierCredentialRecord, bool, error)
	ListVersions(
		ctx context.Context,
		tenant domain.TenantID,
		credential domain.ExternalCarrierCredentialReference,
	) ([]ExternalCarrierCredentialRecord, error)
	Save(ctx context.Context, record ExternalCarrierCredentialRecord) (ExternalCarrierCredentialSaveOutcome, error)
}
