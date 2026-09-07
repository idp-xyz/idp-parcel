package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 总单登记册（ADR-0113；票 tf-carrier-master-document-register/01）。CONTEXT「总单」要登记的东西——签发方、
// 主运输凭证范围、可缺的运输委托 / 订舱引用、关联对象集、版本与替代关系——由 domain.MasterDocument 承载；
// 这里是它越过提交边界的形状。

// MasterDocumentKey 是一份总单某一版的幂等键。版本在键里：撤销、替代、关联重述都是新版本，原版本不被改写。
type MasterDocumentKey struct {
	TenantID domain.TenantID
	Document domain.MasterDocumentReference
	Version  domain.MasterDocumentVersion
}

// MasterDocumentRecord 是一个版本越过提交边界留下的东西。RecordedAt 是登记落库的时刻，与总单上的改变时间
// 是两个时间，不合并。
type MasterDocumentRecord struct {
	Key        MasterDocumentKey
	Document   domain.MasterDocument
	RecordedAt time.Time
}

// MasterDocumentSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）；撞的是主键还是链的
// 两道部分唯一索引（第二个首版、同一前版被回指两次）写口不分——它只答「这一版没落进去」，是重放、冲突
// 还是链已被别人推进，由调用方读回既有版本比对。
type MasterDocumentSaveOutcome uint8

const (
	MasterDocumentSaveOutcomeInvalid MasterDocumentSaveOutcome = iota
	MasterDocumentSaved
	MasterDocumentAlreadyRegistered
)

func (outcome MasterDocumentSaveOutcome) String() string {
	switch outcome {
	case MasterDocumentSaved:
		return "SAVED"
	case MasterDocumentAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// MasterDocumentRegistry 按键找回并登记总单版本。只插不改。
//
// FindCurrent 交回一份总单此刻未被任何版本回指的那一版——「当前」是按回指派生的问答，不是存下来的标记。
// ListVersions 按登记先后交回全部版本，给读面与冲突核对用。
type MasterDocumentRegistry interface {
	FindByKey(ctx context.Context, key MasterDocumentKey) (MasterDocumentRecord, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		document domain.MasterDocumentReference,
	) (MasterDocumentRecord, bool, error)
	ListVersions(
		ctx context.Context,
		tenant domain.TenantID,
		document domain.MasterDocumentReference,
	) ([]MasterDocumentRecord, error)
	Save(ctx context.Context, record MasterDocumentRecord) (MasterDocumentSaveOutcome, error)
}
