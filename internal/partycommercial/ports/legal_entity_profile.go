package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// LegalEntityProfileSaveOutcome 是一笔法人资料修订在持久化面的落点（ADR-0031 同款）：`已登记`是重放（同键
// 同内容），`内容冲突`是同修订号携带不同内容。判据同 PartyRegistrySaveOutcome，但资料不是身份，两册不共用
// 一个落点类型。
type LegalEntityProfileSaveOutcome uint8

const (
	LegalEntityProfileSaveOutcomeInvalid LegalEntityProfileSaveOutcome = iota
	LegalEntityProfileRegistrySaved
	LegalEntityProfileRegistryAlreadyRegistered
	LegalEntityProfileRegistryContentConflict
)

func (outcome LegalEntityProfileSaveOutcome) String() string {
	switch outcome {
	case LegalEntityProfileRegistrySaved:
		return "SAVED"
	case LegalEntityProfileRegistryAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case LegalEntityProfileRegistryContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// LegalEntityProfileRegistry 是法人资料登记册的写入面：键=租户+责任法人+修订，修订不可覆盖。
//
// LoadLatestLegalEntityProfile 取某法人资料的**最新修订**，写入用例靠它做修订连续性检查。found=false = 从未
// 登记；读取失败走 error，不得折成 found=false（判据同 PartyIdentityRegistry 的 Load*）。
type LegalEntityProfileRegistry interface {
	SaveLegalEntityProfile(
		ctx context.Context,
		revision domain.LegalEntityProfileRevision,
	) (LegalEntityProfileSaveOutcome, error)
	LoadLatestLegalEntityProfile(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) (domain.LegalEntityProfileRevision, bool, error)
}

// LegalEntityProfileChainRead 取一个法人资料的全部修订，给按时点解析用（domain.ResolveLegalEntityProfile）。
//
// 它不并进 LegalEntityProfileRegistry：解析的用例不该持有登记册的写口。一个法人的修订链就是要全部交出——
// 截掉哪一段，追溯生效的修订就可能恰好落在截掉的那段里。从未登记答空链；读取失败走 error，不得答成空链，
// 那会把一次该重试的故障变成一次`资料不全`。
type LegalEntityProfileChainRead interface {
	LoadLegalEntityProfileChain(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) ([]domain.LegalEntityProfileRevision, error)
}

// LegalEntityRegistrationLookup 取责任法人身份的最新修订：法人资料的写入与解析都要看法人在不在册、有没有
// 停用、身份上登的注册国家 / 地区。PartyIdentityRegistry 带同名方法；单列成口是为了资料的用例拿不到身份
// 登记册的写口。
type LegalEntityRegistrationLookup interface {
	LoadLatestLegalEntity(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) (domain.LegalEntityRegistration, bool, error)
}

// LegalEntityProfileRevisionRow 是法人资料修订链上的一笔，照册上那一行转写。与 LegalEntityRevisionRow 同一个
// 判据：修订本身，没有「此刻的状态」——哪一笔在某个时点有效是解析的事，给每一笔各算一格会让已被取代的
// 修订也显出一格状态。
type LegalEntityProfileRevisionRow struct {
	TenantID       string
	LegalEntityID  string
	Revision       int
	Basis          string
	EffectiveFrom  time.Time
	AddressCountry string
	AddressLines   []string
	TaxNumbers     []TaxRegistrationNumberRow
	// HasInvoicing 为假即这笔修订没带开票资料；InvoiceTitle 只在它为真时有意义。
	HasInvoicing bool
	InvoiceTitle string
	Contacts     []LegalEntityContactRow
	RegisteredAt time.Time
}

type TaxRegistrationNumberRow struct {
	TypeCode string
	Number   string
}

// LegalEntityContactRow 的邮箱与电话空串即未登记。
type LegalEntityContactRow struct {
	Name  string
	Email string
	Phone string
}

// LegalEntityProfileRevisionHistoryRead 是法人资料修订历史的读端口：按单个法人展开登记册上的全部修订，按
// 修订号升序。判据同 LegalEntityRevisionHistoryRead：不在册与跨租户同形——都是零行、无错，传输层答 200 +
// 空数组，不分「没有」与「别家的」。
type LegalEntityProfileRevisionHistoryRead interface {
	ListLegalEntityProfileRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) ([]LegalEntityProfileRevisionRow, error)
}
