package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrInvalidSourceFileIdentity 表示源文件身份放不进证据索引：名称为空或 SHA-256
	// 不是 64 位小写十六进制。身份保证强度低于哈希匹配的情形必须显式登记为断言，
	// 不能靠格式不合的哈希混进登记册（Source integrity gate）。
	ErrInvalidSourceFileIdentity = errors.New("parcel pricing: invalid source file identity")
	// ErrInvalidPriceCardRegistration 表示价卡登记缺治理件：方案立不住、租户为空、
	// 方向授权引用不是 party-commercial 的授权工件，或没有发布批准责任方。
	ErrInvalidPriceCardRegistration = errors.New("parcel pricing: invalid price card registration")
	// ErrPriceCardRegistrationSnapshotInvalid 表示登记快照解不出一份立得住的登记。
	ErrPriceCardRegistrationSnapshotInvalid = errors.New("parcel pricing: invalid price card registration snapshot")
)

// sha256HexLength 是 SHA-256 摘要的十六进制长度。仓库只登脱敏标识与证据索引，真实
// 价卡文件外置（ADR-0008），哈希是登记册指回受限证据库的那根手指。
const sha256HexLength = 64

// SourceFileIdentity 是一张价卡的源文件身份：文件名与其 SHA-256。票面要求版本仓储
// 携带它，因为没有源身份的价卡在争议中无从对账——CONTEXT 的价卡治理案例条目把来源
// 不完整的资料整体降为不可作生产金额证据。
type SourceFileIdentity struct {
	name   string
	sha256 string
}

func NewSourceFileIdentity(name, sha256Hex string) (SourceFileIdentity, error) {
	identity := SourceFileIdentity{name: name, sha256: sha256Hex}
	if !identity.valid() {
		return SourceFileIdentity{}, ErrInvalidSourceFileIdentity
	}
	return identity, nil
}

func (identity SourceFileIdentity) Name() string   { return identity.name }
func (identity SourceFileIdentity) SHA256() string { return identity.sha256 }

func (identity SourceFileIdentity) valid() bool {
	if !trimmed(identity.name) {
		return false
	}
	if len(identity.sha256) != sha256HexLength {
		return false
	}
	for _, character := range identity.sha256 {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// PriceCardRegistration 是一次价卡发布登记：租户、可执行方案版本、源文件身份，加上
// 两件治理引用——价格方向授权（party-commercial 签发，这里只引用）与发布批准责任方
// （生命周期「已校验 → 已批准」的责任归属）。登记内容全属实例半边，本类型只定形状，
// 不携带任何默认取值。
type PriceCardRegistration struct {
	tenant                 TenantID
	plan                   PricingPlanVersion
	source                 SourceFileIdentity
	directionAuthorization VersionReference
	publicationApprover    string
}

func NewPriceCardRegistration(
	tenant TenantID,
	plan PricingPlanVersion,
	source SourceFileIdentity,
	directionAuthorization VersionReference,
	publicationApprover string,
) (PriceCardRegistration, error) {
	registration := PriceCardRegistration{
		tenant:                 tenant,
		plan:                   plan,
		source:                 source,
		directionAuthorization: directionAuthorization,
		publicationApprover:    publicationApprover,
	}
	if !registration.valid() {
		return PriceCardRegistration{}, ErrInvalidPriceCardRegistration
	}
	return registration, nil
}

func (registration PriceCardRegistration) Tenant() TenantID         { return registration.tenant }
func (registration PriceCardRegistration) Plan() PricingPlanVersion { return registration.plan }
func (registration PriceCardRegistration) SourceFile() SourceFileIdentity {
	return registration.source
}
func (registration PriceCardRegistration) DirectionAuthorization() VersionReference {
	return registration.directionAuthorization
}
func (registration PriceCardRegistration) PublicationApprover() string {
	return registration.publicationApprover
}

func (registration PriceCardRegistration) valid() bool {
	return registration.tenant.valid() &&
		registration.plan.valid() &&
		registration.source.valid() &&
		registration.directionAuthorization.kind == ArtifactCommercialAuthorization &&
		registration.directionAuthorization.valid() &&
		trimmed(registration.publicationApprover)
}

type priceCardRegistrationSnapshot struct {
	Tenant                 string                   `json:"tenant"`
	SourceFileName         string                   `json:"sourceFileName"`
	SourceFileSHA256       string                   `json:"sourceFileSha256"`
	DirectionAuthorization versionReferenceSnapshot `json:"directionAuthorization"`
	PublicationApprover    string                   `json:"publicationApprover"`
	Plan                   pricingPlanSnapshot      `json:"plan"`
}

// MarshalPriceCardRegistration 把一次价卡登记折成持久化快照。方案与治理元数据折在
// 同一份文档里，读回经同一道整图重验——列面只是比对，权威内容在快照。
func MarshalPriceCardRegistration(registration PriceCardRegistration) ([]byte, error) {
	if !registration.valid() {
		return nil, ErrPriceCardRegistrationSnapshotInvalid
	}
	return json.Marshal(priceCardRegistrationSnapshot{
		Tenant:                 registration.tenant.String(),
		SourceFileName:         registration.source.name,
		SourceFileSHA256:       registration.source.sha256,
		DirectionAuthorization: versionReferenceOf(registration.directionAuthorization),
		PublicationApprover:    registration.publicationApprover,
		Plan:                   pricingPlanDocumentOf(registration.plan),
	})
}

// RehydratePriceCardRegistration 从快照重建价卡登记并整图重验。方案部分沿用方案
// 快照的规范化门：规范化版本不被当前构建支持时拒绝重建，不是内容冲突。
func RehydratePriceCardRegistration(raw []byte) (PriceCardRegistration, error) {
	var document priceCardRegistrationSnapshot
	if err := json.Unmarshal(raw, &document); err != nil {
		return PriceCardRegistration{}, fmt.Errorf("%w: %v", ErrPriceCardRegistrationSnapshotInvalid, err)
	}
	if document.Plan.Canonicalization != canonicalizationVersion {
		return PriceCardRegistration{}, fmt.Errorf("%w: snapshot records %q, this build canonicalizes %q",
			ErrCanonicalizationVersionUnsupported, document.Plan.Canonicalization, canonicalizationVersion)
	}
	registration := PriceCardRegistration{
		tenant: TenantID{identifier{value: document.Tenant}},
		plan:   pricingPlanFrom(document.Plan),
		source: SourceFileIdentity{
			name:   document.SourceFileName,
			sha256: document.SourceFileSHA256,
		},
		directionAuthorization: versionReferenceFrom(document.DirectionAuthorization),
		publicationApprover:    document.PublicationApprover,
	}
	if !registration.valid() {
		return PriceCardRegistration{}, ErrPriceCardRegistrationSnapshotInvalid
	}
	return registration, nil
}
