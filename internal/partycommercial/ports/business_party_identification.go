package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// BusinessPartyIdentificationRow 是业务参与方一笔修订上的识别信息（ADR-0367），目录行与修订历史行共用。
// HasIdentityLayer 为假即这笔修订没带注册国家与终身注册号——可能是这几格落地之前登的，也可能是这家单位就没登号，
// 两种读出来一样，读口不分；IdentityCorrectionBasis 只在 HasIdentityCorrection 为真时有意义。
type BusinessPartyIdentificationRow struct {
	HasIdentityLayer        bool
	RegistrationCountry     string
	LifetimeNumbers         []LifetimeRegistrationNumberRow
	HasIdentityCorrection   bool
	IdentityCorrectionBasis string
	ExternalReferences      []ExternalReferenceRow
}

// ExternalReferenceRow 是一个外部系统编码：系统、用途（CUSTOMER / SUPPLIER / PARTY）与码。
type ExternalReferenceRow struct {
	System  string
	Purpose string
	Code    string
}

// IdentificationHolderKind 说持有方是哪本身份册上的身份。
type IdentificationHolderKind uint8

const (
	IdentificationHolderInvalid IdentificationHolderKind = iota
	IdentificationHeldByBusinessParty
	IdentificationHeldByLegalEntity
)

// RegistrationNumberHolder 是一个终身注册号的在用持有方：往来单位按参与方标识点名，责任法人按法人标识点名，
// 另带它所钉的参与方，拒绝理由里可以同时说出公司主体与它的往来单位。
type RegistrationNumberHolder struct {
	Kind     IdentificationHolderKind
	ID       string
	Party    string
	TypeCode string
	Number   string
}

// ExternalReferenceHolder 是一个外部系统编码的在用持有方（只有往来单位持有外部编码）。
type ExternalReferenceHolder struct {
	Party   string
	System  string
	Purpose string
	Code    string
}

// IdentificationHolderLookup 回答「这些号、这些编码此刻被谁持有」（ADR-0367 决定二、三）。在用指该身份最新修订
// 未停用。实现必须在调用方事务内先按每个键取事务级咨询锁再查：两笔同号登记并发时，后到的那一笔要等前一笔提交、
// 读得到它，才答得出撞号——先查后锁或不锁，两笔都会看见「没人持有」。调用方不在事务里时实现答技术失败，不退化成
// 不加锁的查询。
type IdentificationHolderLookup interface {
	FindRegistrationNumberHolders(
		ctx context.Context,
		tenant domain.TenantID,
		identity domain.LegalEntityIdentityLayer,
	) ([]RegistrationNumberHolder, error)
	FindExternalReferenceHolders(
		ctx context.Context,
		tenant domain.TenantID,
		references []domain.ExternalReference,
	) ([]ExternalReferenceHolder, error)
	// FindLegalEntitiesPinning 交回最新修订钉着这个参与方的责任法人标识（不论是否已停用）：被钉着的参与方是我方
	// 公司，它的号只登在公司主体上。实现同样先按（租户，参与方）取事务级咨询锁：给参与方登号与拿公司主体钉它两条路径
	// 都先调它，互斥才在并发下成立；公司主体那条路径取锁之后要重读参与方的最新修订。
	FindLegalEntitiesPinning(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) ([]string, error)
	// HasBusinessPartyProfile 回答这个参与方最新一版参与方资料是不是带着内容（ADR-0369 决定二）：拿公司主体钉它之前要问——
	// 我方公司的资料只在法人资料上。从未登记或最新一版是空的都答 false。调用方须先取钉住锁（FindLegalEntitiesPinning）。
	HasBusinessPartyProfile(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) (bool, error)
}
