package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// RegistrationNumberTypeSaveOutcome 是一笔注册号类型修订在持久化面的落点（ADR-0031 同款）：
// `已登记`是重放（同键同内容），`内容冲突`是同修订号携带不同内容。判据同 PartyRegistrySaveOutcome，
// 但目录条目不是身份，两册不共用一个落点类型。
type RegistrationNumberTypeSaveOutcome uint8

const (
	RegistrationNumberTypeSaveOutcomeInvalid RegistrationNumberTypeSaveOutcome = iota
	RegistrationNumberTypeSaved
	RegistrationNumberTypeAlreadyRegistered
	RegistrationNumberTypeContentConflict
)

func (outcome RegistrationNumberTypeSaveOutcome) String() string {
	switch outcome {
	case RegistrationNumberTypeSaved:
		return "SAVED"
	case RegistrationNumberTypeAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case RegistrationNumberTypeContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// RegistrationNumberTypeRegistry 是注册号类型目录登记册的持久化面：键=租户+注册国家 / 地区+类型
// 代码+修订，修订不可覆盖。
//
// LoadLatestRegistrationNumberType 取某类型的**最新修订**：写入用例靠它做修订连续性检查、钉住
// 所属层，停用用例靠它取要停用的那笔。found=false = 从未登记；读取失败走 error，不得折成
// found=false（判据同 PartyIdentityRegistry 的 Load*）。
type RegistrationNumberTypeRegistry interface {
	SaveRegistrationNumberType(
		ctx context.Context,
		registration domain.RegistrationNumberTypeRegistration,
	) (RegistrationNumberTypeSaveOutcome, error)
	LoadLatestRegistrationNumberType(
		ctx context.Context,
		tenant domain.TenantID,
		country domain.RegistrationCountryCode,
		code domain.RegistrationNumberTypeCode,
	) (domain.RegistrationNumberTypeRegistration, bool, error)
}

// RegistrationNumberTypeLookup 是按目录判号的读口：身份登记与法人资料的写入用例按租户与注册国家 /
// 地区取回整份目录，再用 domain.RegistrationNumberTypeCatalogue.Check 判号。
//
// 它不并进 RegistrationNumberTypeRegistry：判号的用例不该持有目录的写口。也不并进目录上列读口：
// 那边是给管理台翻的列表、有页大小，这边是一次判断要的整份国家 / 地区目录。国家 / 地区在目录里
// 一笔都没有时交回空目录（Check 据此答`未登记`）；读取失败走 error——不得把一次该重试的故障答成
// 「未登记」，那会让登记方去补一个本来就在的目录条目。
type RegistrationNumberTypeLookup interface {
	LoadRegistrationNumberTypeCatalogue(
		ctx context.Context,
		tenant domain.TenantID,
		country domain.RegistrationCountryCode,
	) (domain.RegistrationNumberTypeCatalogue, error)
}

// RegistrationNumberTypeRow 是注册号类型目录上列的一行：一个类型的最新登记修订。
//
// Status 是装载时点对生命周期事实的导出（REGISTERED / EFFECTIVE / DEACTIVATED），判据同
// GroupLegalEntityRow；停用两件只在 HasDeactivation 为真时有意义，零时刻不兼作「没停用」。
// FormatPattern 是登记原文，按整串匹配的语义在领域门里，页面照原文显示。
type RegistrationNumberTypeRow struct {
	TenantID          string
	CountryCode       string
	TypeCode          string
	Revision          int
	TypeName          string
	Layer             string
	FormatPattern     string
	Basis             string
	Status            string
	EffectiveFrom     time.Time
	DeactivatedAt     time.Time
	DeactivationBasis string
	HasDeactivation   bool
	RegisteredAt      time.Time
}

// RegistrationNumberTypeCatalogueRead 是注册号类型目录的伴生列表读端口（ADR-0077）。上列对象是
// 各类型的**最新修订**；修订史是登记册的证据面，不是目录的行。租户在签名上、Limit 非正拒、空册
// 答空列表，判据同 ServiceProductCatalogueRead。
type RegistrationNumberTypeCatalogueRead interface {
	ListRegistrationNumberTypes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]RegistrationNumberTypeRow, error)
}
