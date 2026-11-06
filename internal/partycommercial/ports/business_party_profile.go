package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// BusinessPartyProfileSaveOutcome 是一笔参与方资料修订在持久化面的落点，判据同 LegalEntityProfileSaveOutcome（ADR-0031）：
// `已登记`是同键同内容的重放，`内容冲突`是同修订号携带不同内容。两本资料册各引各的主体，不共用一个落点类型。
type BusinessPartyProfileSaveOutcome uint8

const (
	BusinessPartyProfileSaveOutcomeInvalid BusinessPartyProfileSaveOutcome = iota
	BusinessPartyProfileSaved
	BusinessPartyProfileAlreadyRegistered
	BusinessPartyProfileContentConflict
)

// BusinessPartyProfileRegistry 是参与方资料登记册的写入面（ADR-0369）：键 = 租户 + 往来单位 + 修订，修订不可覆盖。
// LoadLatestBusinessPartyProfile 取最新修订给写入用例做修订连续性检查；从未登记答 found=false，读取失败走 error。
type BusinessPartyProfileRegistry interface {
	SaveBusinessPartyProfile(
		ctx context.Context,
		revision domain.BusinessPartyProfileRevision,
	) (BusinessPartyProfileSaveOutcome, error)
	LoadLatestBusinessPartyProfile(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) (domain.BusinessPartyProfileRevision, bool, error)
}

// RecordedBusinessPartyProfileRevision 是读回的一笔修订连同它落册的时刻：修订本身没有「此刻的状态」，哪一笔此刻有效由
// domain.EffectiveBusinessPartyProfileRevision 回答。
type RecordedBusinessPartyProfileRevision struct {
	Revision   domain.BusinessPartyProfileRevision
	RecordedAt time.Time
}

// BusinessPartyProfileHistoryRead 按单个往来单位交回登记册上的全部修订，按修订号升序。不在册与跨租户同形：零行、无错。
// 一条修订链就是要全部交出——截掉一段，追溯生效的修订就可能恰好落在截掉的那段里。
type BusinessPartyProfileHistoryRead interface {
	ListBusinessPartyProfileRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) ([]RecordedBusinessPartyProfileRevision, error)
}
