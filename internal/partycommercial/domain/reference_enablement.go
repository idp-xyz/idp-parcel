package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

var (
	ErrInvalidReferenceEnablement = errors.New("party commercial: invalid reference enablement registration")
	// ErrReferenceCodeNotInCatalogue：码不在产品内置的基础数据目录里。只由登记新修订的构造门抛出，
	// 读回的重建不经它（见 RehydrateReferenceEnablementRegistration）。
	ErrReferenceCodeNotInCatalogue = errors.New("party commercial: reference code is not in the built-in catalogue")
	// ErrReferenceEnablementEffectiveBeforePrior：新修订的生效时点早于前一修订的。
	ErrReferenceEnablementEffectiveBeforePrior = errors.New("party commercial: reference enablement takes effect before the prior revision")
	// ErrReferenceEnablementOutOfSequence：修订号接不上册面（首笔不是 1，或不是最新修订加一）。
	ErrReferenceEnablementOutOfSequence = errors.New("party commercial: reference enablement revision is out of sequence")
)

// referenceEnablementSourceLimit 是来源单据的字数上限，与迁移里的 CHECK 同一个数。来源单据是登记者写下的凭据
// 引用（单据号、会议纪要号），不是正文。
const referenceEnablementSourceLimit = 200

// ReferenceKind 是启用范围登记册的种类，封闭两词：国家 / 地区与币种。种类决定码去哪份内置目录里查。
type ReferenceKind uint8

const (
	ReferenceKindInvalid ReferenceKind = iota
	ReferenceKindCountry
	ReferenceKindCurrency
)

func (kind ReferenceKind) String() string {
	switch kind {
	case ReferenceKindCountry:
		return "COUNTRY"
	case ReferenceKindCurrency:
		return "CURRENCY"
	default:
		return ""
	}
}

func (kind ReferenceKind) valid() bool {
	return kind.String() != ""
}

// ReferenceKindNamed 把原词反查回封闭集；空串与集外都答不认识。
func ReferenceKindNamed(name string) (ReferenceKind, bool) {
	return closedCodeNamed(ReferenceKind.valid, ReferenceKind.String, name)
}

// ReferenceEnablementStatus 是登记册上一行说的话：这个租户启用这个码，或停用这个码。没有登记过的码不是第三个值，
// 它是册上没有行——“未配置”由缺席表达（ADR-0146：租户取值留空，不套默认）。
type ReferenceEnablementStatus uint8

const (
	ReferenceEnablementStatusInvalid ReferenceEnablementStatus = iota
	ReferenceEnablementEnabled
	ReferenceEnablementDisabled
)

func (status ReferenceEnablementStatus) String() string {
	switch status {
	case ReferenceEnablementEnabled:
		return "ENABLED"
	case ReferenceEnablementDisabled:
		return "DISABLED"
	default:
		return ""
	}
}

func (status ReferenceEnablementStatus) valid() bool {
	return status.String() != ""
}

// ReferenceEnablementStatusNamed 把原词反查回封闭集；空串与集外都答不认识。
func ReferenceEnablementStatusNamed(name string) (ReferenceEnablementStatus, bool) {
	return closedCodeNamed(ReferenceEnablementStatus.valid, ReferenceEnablementStatus.String, name)
}

// ReferenceCode 是国家 / 地区码或币种码。这里只收非空；码在不在目录里要连种类一起看，由登记修订的构造门判。
type ReferenceCode struct{ requiredValue }

func NewReferenceCode(value string) (ReferenceCode, error) {
	required, err := newRequiredValue("reference code", value)
	return ReferenceCode{required}, err
}

// ReferenceEnablementSource 是登记者写下的来源单据：凭什么启用 / 停用。可以不写——登记册上每一行都能答出是谁在何时登的，
// 凭据是加分项而不是门槛；不写就是空，不兼作“无”的另一种写法。
type ReferenceEnablementSource struct{ value string }

func NewReferenceEnablementSource(value string) (ReferenceEnablementSource, error) {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) > referenceEnablementSourceLimit {
		return ReferenceEnablementSource{}, fmt.Errorf(
			"%w: source reference longer than %d characters", ErrInvalidReferenceEnablement, referenceEnablementSourceLimit)
	}
	return ReferenceEnablementSource{value: trimmed}, nil
}

func (source ReferenceEnablementSource) String() string {
	return source.value
}

// ReferenceEnablementRegistration 是启用范围登记册上的一行：某租户从某个时点起启用或停用某个国家 / 地区或币种。
//
// 登记册只追加：键=租户+种类+码+修订，修订从 1 起连续递增，启用与停用都是新修订，绝不改写、不删除。当前是启用还是
// 停用，由“生效时点不晚于此刻的最新一行”导出（EnablementStandingAt），不存状态列。
//
// 登记责任方（registrant）与登记时间（recordedAt）是审计事实：登记责任方取自认证信封，不取自载荷；两样都不进重放比对
// （ReferenceEnablementRegistration.SameContent），同一个意思由另一位操作者重放，在册的仍是先到的那一行。
type ReferenceEnablementRegistration struct {
	tenant        TenantID
	kind          ReferenceKind
	code          ReferenceCode
	revision      int
	status        ReferenceEnablementStatus
	effectiveFrom time.Time
	source        ReferenceEnablementSource
	registrant    OperatorSubjectReference
	recordedAt    time.Time
}

// ReferenceEnablementSpec 是一行的正文：说了什么、从何时起、凭什么。
type ReferenceEnablementSpec struct {
	Status        ReferenceEnablementStatus
	EffectiveFrom time.Time
	Source        ReferenceEnablementSource
}

// NewReferenceEnablementRegistration 建一行新登记的修订。码必须在内置目录里：登记的是一个租户取值，但它只能在产品认得的
// 码里选，目录外的码在这一道门上答 ErrReferenceCodeNotInCatalogue，而不是落一行日后谁也答不出名字的册面。
func NewReferenceEnablementRegistration(
	tenant TenantID,
	kind ReferenceKind,
	code ReferenceCode,
	revision int,
	spec ReferenceEnablementSpec,
	registrant OperatorSubjectReference,
	recordedAt time.Time,
) (ReferenceEnablementRegistration, error) {
	registration, err := rehydrateReferenceEnablementRegistration(tenant, kind, code, revision, spec, registrant, recordedAt)
	if err != nil {
		return ReferenceEnablementRegistration{}, err
	}
	if !referenceCodeInCatalogue(kind, code.String()) {
		return ReferenceEnablementRegistration{}, fmt.Errorf("%w: %s %q", ErrReferenceCodeNotInCatalogue, kind, code)
	}
	return registration, nil
}

// RehydrateReferenceEnablementRegistration 从册面读回一行。它不再核目录成员：目录随发版变，ISO 废止了一个码之后，
// 此前合法登过的行仍要读得回、仍如实在册——读回不能因为目录更新而整本册面读不出来。形状与不变量照旧核。
func RehydrateReferenceEnablementRegistration(
	tenant TenantID,
	kind ReferenceKind,
	code ReferenceCode,
	revision int,
	spec ReferenceEnablementSpec,
	registrant OperatorSubjectReference,
	recordedAt time.Time,
) (ReferenceEnablementRegistration, error) {
	return rehydrateReferenceEnablementRegistration(tenant, kind, code, revision, spec, registrant, recordedAt)
}

func rehydrateReferenceEnablementRegistration(
	tenant TenantID,
	kind ReferenceKind,
	code ReferenceCode,
	revision int,
	spec ReferenceEnablementSpec,
	registrant OperatorSubjectReference,
	recordedAt time.Time,
) (ReferenceEnablementRegistration, error) {
	if !tenant.valid() || !kind.valid() || !code.valid() || revision < 1 ||
		!spec.Status.valid() || spec.EffectiveFrom.IsZero() || !registrant.valid() || recordedAt.IsZero() {
		return ReferenceEnablementRegistration{}, ErrInvalidReferenceEnablement
	}
	return ReferenceEnablementRegistration{
		tenant:        tenant,
		kind:          kind,
		code:          code,
		revision:      revision,
		status:        spec.Status,
		effectiveFrom: spec.EffectiveFrom.UTC(),
		source:        spec.Source,
		registrant:    registrant,
		recordedAt:    recordedAt.UTC(),
	}, nil
}

func referenceCodeInCatalogue(kind ReferenceKind, code string) bool {
	switch kind {
	case ReferenceKindCountry:
		_, found := refdata.Country(code)
		return found
	case ReferenceKindCurrency:
		_, found := refdata.Currency(code)
		return found
	}
	return false
}

// FollowsFrom 判这一行能不能作为册上现有最新修订的下一笔落笔；hasPrior=false 表示册上没有这个码，这一行必须是首笔。
//
// 修订从 1 起连续递增：登记方声明的是自己看到的册面，错位说明册面已被别人推进或意图已陈旧，一个字节不写。
// 生效时点不得早于前一修订的：否则“最新一行”会读成一件没人想表达的事——前一修订预告了 7 月停用，后一修订想取消它
// 却把自己写成 6 月生效，它早于预告，预告照样在 7 月起效。要撤销一条预告，就登记一条生效时点不早于它的后继，相等时
// 后到者取胜。前一修订原样留在册上：它是那段时间里确实登记过的话。
func (registration ReferenceEnablementRegistration) FollowsFrom(prior ReferenceEnablementRegistration, hasPrior bool) error {
	if !hasPrior {
		if registration.revision != 1 {
			return fmt.Errorf("%w: first revision must be 1, got %d", ErrReferenceEnablementOutOfSequence, registration.revision)
		}
		return nil
	}
	if registration.tenant != prior.tenant || registration.kind != prior.kind || registration.code != prior.code ||
		registration.revision != prior.revision+1 {
		return fmt.Errorf("%w: latest on the register is %d, got %d",
			ErrReferenceEnablementOutOfSequence, prior.revision, registration.revision)
	}
	if registration.effectiveFrom.Before(prior.effectiveFrom) {
		return fmt.Errorf("%w: %s is before %s", ErrReferenceEnablementEffectiveBeforePrior,
			registration.effectiveFrom.Format(time.RFC3339), prior.effectiveFrom.Format(time.RFC3339))
	}
	return nil
}

// SameContent 判两行是不是同一个意思：状态、生效时点、来源单据都一致。登记责任方与登记时间是审计，不在比对里。
func (registration ReferenceEnablementRegistration) SameContent(other ReferenceEnablementRegistration) bool {
	return registration.status == other.status &&
		registration.effectiveFrom.Equal(other.effectiveFrom) &&
		registration.source == other.source
}

func (registration ReferenceEnablementRegistration) Tenant() TenantID    { return registration.tenant }
func (registration ReferenceEnablementRegistration) Kind() ReferenceKind { return registration.kind }
func (registration ReferenceEnablementRegistration) Code() ReferenceCode { return registration.code }
func (registration ReferenceEnablementRegistration) Revision() int       { return registration.revision }
func (registration ReferenceEnablementRegistration) EffectiveFrom() time.Time {
	return registration.effectiveFrom
}
func (registration ReferenceEnablementRegistration) Status() ReferenceEnablementStatus {
	return registration.status
}
func (registration ReferenceEnablementRegistration) Source() ReferenceEnablementSource {
	return registration.source
}
func (registration ReferenceEnablementRegistration) Registrant() OperatorSubjectReference {
	return registration.registrant
}
func (registration ReferenceEnablementRegistration) RecordedAt() time.Time {
	return registration.recordedAt
}

// EnablementStandingAt 回答某个码在某一刻是启用还是停用：生效时点不晚于该刻的最新一行说了算，“最新”先比生效时点、
// 再比修订。该刻之前册上没有生效的行（含全是未来生效的行、和压根没登记过）答不出来——found=false 就是“未配置”，
// 调用方不得把它折成启用或停用。
//
// history 是同一个租户、种类、码的各修订，次序不限。
func EnablementStandingAt(history []ReferenceEnablementRegistration, at time.Time) (ReferenceEnablementRegistration, bool) {
	var standing ReferenceEnablementRegistration
	found := false
	for _, row := range history {
		if row.effectiveFrom.After(at) {
			continue
		}
		if !found ||
			row.effectiveFrom.After(standing.effectiveFrom) ||
			(row.effectiveFrom.Equal(standing.effectiveFrom) && row.revision > standing.revision) {
			standing = row
			found = true
		}
	}
	return standing, found
}
