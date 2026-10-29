package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// ReferenceEnablementSaveOutcome 是一行启用范围修订在持久化面的落点（ADR-0031 同款）：`已登记`是重放（同键同内容），
// `内容冲突`是同修订号携带不同内容。
type ReferenceEnablementSaveOutcome uint8

const (
	ReferenceEnablementSaveOutcomeInvalid ReferenceEnablementSaveOutcome = iota
	ReferenceEnablementSaved
	ReferenceEnablementAlreadyRegistered
	ReferenceEnablementContentConflict
)

func (outcome ReferenceEnablementSaveOutcome) String() string {
	switch outcome {
	case ReferenceEnablementSaved:
		return "SAVED"
	case ReferenceEnablementAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ReferenceEnablementContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ReferenceEnablementRegistry 是启用范围登记册的持久化面：键=租户+种类+码+修订，修订不可覆盖。
//
// LoadLatestReferenceEnablement 取某个码的**最新修订**（按修订号，不按生效时点）：写入用例靠它做修订连续性检查。
// found=false = 从未登记；读取失败走 error，不得折成 found=false（折了，登记方会对着一本其实有行的册去补首笔）。
//
// ListReferenceEnablements 取某租户某种类下全部码的**全部修订**，按码、修订升序：读面据此导出每个码此刻的状态并带出
// 历史。册上的码数以内置目录为界（国家 / 地区 249、币种 152），每个码的修订数是人工登记的次数，不分页。
type ReferenceEnablementRegistry interface {
	SaveReferenceEnablement(
		ctx context.Context,
		registration domain.ReferenceEnablementRegistration,
	) (ReferenceEnablementSaveOutcome, error)
	LoadLatestReferenceEnablement(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.ReferenceKind,
		code domain.ReferenceCode,
	) (domain.ReferenceEnablementRegistration, bool, error)
	ListReferenceEnablements(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.ReferenceKind,
	) ([]domain.ReferenceEnablementRegistration, error)
}
