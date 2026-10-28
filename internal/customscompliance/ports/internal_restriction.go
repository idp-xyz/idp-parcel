package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件声明内部合规限制册的出入口（ADR-0334）。另立文件而不进 ports.go：内部合规限制与监管限制是两种限制、两本册
// （ADR-0241），读口、写口都不混在 RestrictionStore 里。

// InternalRestrictionScopeView 是准入判断读内部合规限制的那一口：按范围盘出覆盖该范围的全部限制，每条带它全部已有
// 的决定（含已解除的覆盖项——动作时点早于解除的动作仍读到解除之前的状态，由领域按时点判）。空册是如实的答案，
// 不是未决；依赖调不通作为错误返回。
type InternalRestrictionScopeView interface {
	ListInternalByScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.DecisionScopeReference,
	) ([]domain.InternalRestriction, error)
}

type InternalRestrictionSaveOutcome uint8

const (
	InternalRestrictionSaveOutcomeInvalid InternalRestrictionSaveOutcome = iota
	InternalRestrictionSaved
	InternalRestrictionAlreadyRecorded
)

// InternalRestrictionStore 是内部合规限制册的写口加读回口。写入跑在调用方的事务里。
//
// 限制头建立后不改写，所以 Save 是 ON CONFLICT DO NOTHING 代数（ADR-0031）：同标识已有记录答`已有记录`，内容是否同一份
// 由编排读回自己比。决定的追加不走 Save：FindForDecision 先对限制头行取行锁再读回限制与它全部已有决定，编排在锁下判、
// 再 AppendDecision，同一限制上的两个决定因此串行。
type InternalRestrictionStore interface {
	InternalRestrictionScopeView
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		restriction domain.InternalRestriction,
	) (InternalRestrictionSaveOutcome, error)
	// FindForDecision 对限制头取 FOR UPDATE 行锁（只在事务里成立）并读回限制与按接受次序重演的全部决定。
	FindForDecision(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.InternalRestrictionID,
	) (domain.InternalRestriction, bool, error)
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.InternalRestrictionID,
	) (domain.InternalRestriction, bool, error)
	// AppendDecision 追加一条决定及其作用的覆盖项。库里的部分唯一索引守同一覆盖项至多一次确认、至多一次解除。
	AppendDecision(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.InternalRestrictionID,
		decision domain.RestrictionDecision,
	) error
}
