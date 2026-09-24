package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidRegistrationCountry              = errors.New("party commercial: invalid registration country or region")
	ErrInvalidRegistrationNumberFormat         = errors.New("party commercial: invalid registration number format")
	ErrInvalidRegistrationNumberType           = errors.New("party commercial: invalid registration number type registration")
	ErrInvalidRegistrationNumberTypeTransition = errors.New("party commercial: invalid registration number type transition")
	ErrInvalidRegistrationNumberTypeCatalogue  = errors.New("party commercial: invalid registration number type catalogue")
	ErrInvalidRegistrationNumberCheck          = errors.New("party commercial: invalid registration number check")
)

// RegistrationCountryCode 是注册国家 / 地区（CONTEXT「责任法人」）。只收两位大写拉丁字母那一种
// 形状（ISO 3166-1 alpha-2），不内置码表：哪个国家 / 地区有注册号类型由目录登记回答，查无就是
// `未登记`。锁形状是为了一个国家 / 地区在目录里只有一个键——cn、CN、CHN 并存时，目录登在 CN 下
// 而校验按 cn 查，会把登过的国家 / 地区答成未登记。
type RegistrationCountryCode struct{ value string }

var registrationCountryCodeShape = regexp.MustCompile(`^[A-Z]{2}$`)

func NewRegistrationCountryCode(value string) (RegistrationCountryCode, error) {
	if !registrationCountryCodeShape.MatchString(value) {
		return RegistrationCountryCode{}, fmt.Errorf("%w: %q", ErrInvalidRegistrationCountry, value)
	}
	return RegistrationCountryCode{value: value}, nil
}

func (code RegistrationCountryCode) String() string {
	return code.value
}

func (code RegistrationCountryCode) valid() bool {
	return code.value != ""
}

// RegistrationNumberTypeCode 是一个注册号类型在其注册国家 / 地区内的代码。代码只在国家 / 地区
// 之内唯一，目录的键因此是（租户，国家 / 地区，类型代码）。
type RegistrationNumberTypeCode struct{ requiredValue }

func NewRegistrationNumberTypeCode(value string) (RegistrationNumberTypeCode, error) {
	required, err := newRequiredValue("registration number type code", value)
	return RegistrationNumberTypeCode{required}, err
}

type RegistrationNumberTypeName struct{ requiredValue }

func NewRegistrationNumberTypeName(value string) (RegistrationNumberTypeName, error) {
	required, err := newRequiredValue("registration number type name", value)
	return RegistrationNumberTypeName{required}, err
}

// RegistrationNumberTypeBasisReference 指向一次类型登记或停用所依据的东西。判据同
// IdentityBasisReference，但目录条目不是身份，两个引用不共用一个类型。
type RegistrationNumberTypeBasisReference struct{ requiredValue }

func NewRegistrationNumberTypeBasisReference(value string) (RegistrationNumberTypeBasisReference, error) {
	required, err := newRequiredValue("registration number type basis reference", value)
	return RegistrationNumberTypeBasisReference{required}, err
}

// RegistrationNumber 是一个待校验的注册号取值。它属哪一类、格式合不合格由目录回答
// （RegistrationNumberTypeCatalogue.Check），号本身不带类型。
type RegistrationNumber struct{ requiredValue }

func NewRegistrationNumber(value string) (RegistrationNumber, error) {
	required, err := newRequiredValue("registration number", value)
	return RegistrationNumber{required}, err
}

// RegistrationNumberLayer 是注册号类型所属的层（ADR-0145 决定一）：身份层收终身注册号——一个
// 法人只有一个、不会换的那种；资料层收税务登记号——可后办、可变的那种。两层各只收自己那一类。
type RegistrationNumberLayer uint8

const (
	RegistrationNumberLayerInvalid RegistrationNumberLayer = iota
	RegistrationNumberIdentityLayer
	RegistrationNumberProfileLayer
)

func (layer RegistrationNumberLayer) String() string {
	switch layer {
	case RegistrationNumberIdentityLayer:
		return "IDENTITY"
	case RegistrationNumberProfileLayer:
		return "PROFILE"
	default:
		return ""
	}
}

func (layer RegistrationNumberLayer) valid() bool {
	return layer.String() != ""
}

// RegistrationNumberLayerNamed 把原词反查回封闭集；空串与集外都答不认识。
func RegistrationNumberLayerNamed(name string) (RegistrationNumberLayer, bool) {
	return closedCodeNamed(RegistrationNumberLayer.valid, RegistrationNumberLayer.String, name)
}

// RegistrationNumberFormat 是一个注册号类型的格式校验：一条 RE2 正则，按整串匹配。
//
// 登记的正文原样保存（目录与快照回显的是它），匹配时另包一层 `^(?:…)$`：登记者漏写锚点时，
// 「含一段像号的子串」不能算格式合格。包之前先单独编译一遍正文——`a)|(b` 这种不配平的正文
// 单独编不过，而包进去却编得过、且会把整串锚点拆开。选 RE2 是因为它按线性时间匹配：格式正文
// 是租户登记进来的内容，不能让一条正文把校验拖进回溯爆炸。
type RegistrationNumberFormat struct {
	pattern string
	whole   *regexp.Regexp
}

func NewRegistrationNumberFormat(pattern string) (RegistrationNumberFormat, error) {
	if strings.TrimSpace(pattern) == "" {
		return RegistrationNumberFormat{}, fmt.Errorf("%w: blank pattern", ErrInvalidRegistrationNumberFormat)
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return RegistrationNumberFormat{}, fmt.Errorf("%w: %v", ErrInvalidRegistrationNumberFormat, err)
	}
	whole, err := regexp.Compile(`^(?:` + pattern + `)$`)
	if err != nil {
		return RegistrationNumberFormat{}, fmt.Errorf("%w: %v", ErrInvalidRegistrationNumberFormat, err)
	}
	return RegistrationNumberFormat{pattern: pattern, whole: whole}, nil
}

func (format RegistrationNumberFormat) Pattern() string {
	return format.pattern
}

func (format RegistrationNumberFormat) matches(number RegistrationNumber) bool {
	return format.whole != nil && format.whole.MatchString(number.String())
}

func (format RegistrationNumberFormat) valid() bool {
	return format.whole != nil
}

// RegistrationNumberTypeStatus 是目录条目的生命周期：已登记（生效时点未到）→ 已生效 → 已停用。
// 与参与方身份同样三格，但条目不是身份，状态不共用一个类型。
type RegistrationNumberTypeStatus uint8

const (
	RegistrationNumberTypeStatusInvalid RegistrationNumberTypeStatus = iota
	RegistrationNumberTypeStatusRegistered
	RegistrationNumberTypeStatusEffective
	RegistrationNumberTypeStatusDeactivated
)

func (status RegistrationNumberTypeStatus) String() string {
	switch status {
	case RegistrationNumberTypeStatusRegistered:
		return "REGISTERED"
	case RegistrationNumberTypeStatusEffective:
		return "EFFECTIVE"
	case RegistrationNumberTypeStatusDeactivated:
		return "DEACTIVATED"
	default:
		return ""
	}
}

// RegistrationNumberTypeLifecycle 是目录条目的生命周期事实：生效自何时、是否已被停用。状态由
// 时点导出而不另存一格，判据同 IdentityLifecycle。
type RegistrationNumberTypeLifecycle struct {
	effectiveFrom     time.Time
	deactivatedAt     time.Time
	deactivationBasis RegistrationNumberTypeBasisReference
}

func NewRegistrationNumberTypeLifecycle(effectiveFrom time.Time) (RegistrationNumberTypeLifecycle, error) {
	if effectiveFrom.IsZero() {
		return RegistrationNumberTypeLifecycle{}, ErrInvalidRegistrationNumberType
	}
	return RegistrationNumberTypeLifecycle{effectiveFrom: effectiveFrom.UTC()}, nil
}

// Deactivate 以显式依据与时点停用一个类型。停用只自其时点起不再收新的号，不回头改判已经按它
// 登记过的号。二次停用与无依据停用拒绝。
func (lifecycle RegistrationNumberTypeLifecycle) Deactivate(
	basis RegistrationNumberTypeBasisReference,
	at time.Time,
) (RegistrationNumberTypeLifecycle, error) {
	if !lifecycle.deactivatedAt.IsZero() || !basis.valid() || at.IsZero() {
		return RegistrationNumberTypeLifecycle{}, ErrInvalidRegistrationNumberTypeTransition
	}
	lifecycle.deactivatedAt = at.UTC()
	lifecycle.deactivationBasis = basis
	return lifecycle, nil
}

// StatusAt 回答该类型在明确时点处于哪一格。停用判断在先：生效前被撤下的类型自撤下时点起就是
// `已停用`，不会先「生效」一下。
func (lifecycle RegistrationNumberTypeLifecycle) StatusAt(at time.Time) RegistrationNumberTypeStatus {
	if lifecycle.effectiveFrom.IsZero() || at.IsZero() {
		return RegistrationNumberTypeStatusInvalid
	}
	if !lifecycle.deactivatedAt.IsZero() && !at.Before(lifecycle.deactivatedAt) {
		return RegistrationNumberTypeStatusDeactivated
	}
	if !at.Before(lifecycle.effectiveFrom) {
		return RegistrationNumberTypeStatusEffective
	}
	return RegistrationNumberTypeStatusRegistered
}

func (lifecycle RegistrationNumberTypeLifecycle) EffectiveFrom() time.Time {
	return lifecycle.effectiveFrom
}

func (lifecycle RegistrationNumberTypeLifecycle) Deactivation() (RegistrationNumberTypeBasisReference, time.Time, bool) {
	if lifecycle.deactivatedAt.IsZero() {
		return RegistrationNumberTypeBasisReference{}, time.Time{}, false
	}
	return lifecycle.deactivationBasis, lifecycle.deactivatedAt, true
}

func (lifecycle RegistrationNumberTypeLifecycle) valid() bool {
	return !lifecycle.effectiveFrom.IsZero()
}

// RegistrationNumberTypeSpec 是一笔类型登记的正文：名称、所属层、格式校验与登记依据。
type RegistrationNumberTypeSpec struct {
	Name   RegistrationNumberTypeName
	Layer  RegistrationNumberLayer
	Format RegistrationNumberFormat
	Basis  RegistrationNumberTypeBasisReference
}

func (spec RegistrationNumberTypeSpec) valid() bool {
	return spec.Name.valid() && spec.Layer.valid() && spec.Format.valid() && spec.Basis.valid()
}

// RegistrationNumberTypeRegistration 是注册号类型目录上的一笔修订（ADR-0145 决定一）：租户 +
// 注册国家 / 地区 + 类型代码 + 修订。修订从 1 起连续递增，内容更正与停用都形成新修订，不原地
// 改写——与本上下文其余登记册同一纪律。目录内容是实施时登记的配置，产品不带任何国家 / 地区的
// 生产默认条目。
type RegistrationNumberTypeRegistration struct {
	tenant    TenantID
	country   RegistrationCountryCode
	code      RegistrationNumberTypeCode
	revision  int
	spec      RegistrationNumberTypeSpec
	lifecycle RegistrationNumberTypeLifecycle
}

func NewRegistrationNumberTypeRegistration(
	tenant TenantID,
	country RegistrationCountryCode,
	code RegistrationNumberTypeCode,
	revision int,
	spec RegistrationNumberTypeSpec,
	lifecycle RegistrationNumberTypeLifecycle,
) (RegistrationNumberTypeRegistration, error) {
	if !tenant.valid() || !country.valid() || !code.valid() || revision < 1 ||
		!spec.valid() || !lifecycle.valid() {
		return RegistrationNumberTypeRegistration{}, ErrInvalidRegistrationNumberType
	}
	return RegistrationNumberTypeRegistration{
		tenant:    tenant,
		country:   country,
		code:      code,
		revision:  revision,
		spec:      spec,
		lifecycle: lifecycle,
	}, nil
}

// Deactivate 交回下一笔修订：同一类型、修订号加一、生命周期进入已停用。
func (registration RegistrationNumberTypeRegistration) Deactivate(
	basis RegistrationNumberTypeBasisReference,
	at time.Time,
) (RegistrationNumberTypeRegistration, error) {
	deactivated, err := registration.lifecycle.Deactivate(basis, at)
	if err != nil {
		return RegistrationNumberTypeRegistration{}, err
	}
	registration.revision++
	registration.lifecycle = deactivated
	return registration, nil
}

func (registration RegistrationNumberTypeRegistration) Tenant() TenantID {
	return registration.tenant
}

func (registration RegistrationNumberTypeRegistration) Country() RegistrationCountryCode {
	return registration.country
}

func (registration RegistrationNumberTypeRegistration) Code() RegistrationNumberTypeCode {
	return registration.code
}

func (registration RegistrationNumberTypeRegistration) Revision() int {
	return registration.revision
}

func (registration RegistrationNumberTypeRegistration) Name() RegistrationNumberTypeName {
	return registration.spec.Name
}

func (registration RegistrationNumberTypeRegistration) Layer() RegistrationNumberLayer {
	return registration.spec.Layer
}

func (registration RegistrationNumberTypeRegistration) Format() RegistrationNumberFormat {
	return registration.spec.Format
}

func (registration RegistrationNumberTypeRegistration) Basis() RegistrationNumberTypeBasisReference {
	return registration.spec.Basis
}

func (registration RegistrationNumberTypeRegistration) Lifecycle() RegistrationNumberTypeLifecycle {
	return registration.lifecycle
}

// RegistrationNumberCheckOutcome 是「这个号是否属目录里的某一类型且格式合格」的答案代数。各格
// 的续办不同：国家 / 地区未登记与类型未登记要实施方去目录登记；类型不在生效期要换一个在用的
// 类型或等它生效；层不符与格式不符要登记方改号——所以分格交回，不压成一个「不合格」。
type RegistrationNumberCheckOutcome uint8

const (
	RegistrationNumberCheckOutcomeInvalid RegistrationNumberCheckOutcome = iota
	RegistrationNumberAccepted
	RegistrationCountryNotRegistered
	RegistrationNumberTypeNotRegistered
	RegistrationNumberTypeNotEffective
	RegistrationNumberLayerMismatch
	RegistrationNumberFormatMismatch
)

func (outcome RegistrationNumberCheckOutcome) String() string {
	switch outcome {
	case RegistrationNumberAccepted:
		return "ACCEPTED"
	case RegistrationCountryNotRegistered:
		return "COUNTRY_NOT_REGISTERED"
	case RegistrationNumberTypeNotRegistered:
		return "TYPE_NOT_REGISTERED"
	case RegistrationNumberTypeNotEffective:
		return "TYPE_NOT_EFFECTIVE"
	case RegistrationNumberLayerMismatch:
		return "LAYER_MISMATCH"
	case RegistrationNumberFormatMismatch:
		return "FORMAT_MISMATCH"
	default:
		return ""
	}
}

// RegistrationNumberCheck 是一次校验的答案，连同判断所对照的那笔类型修订——登记方据此能把
// 「按哪一版类型判的」一并固定下来。
type RegistrationNumberCheck struct {
	outcome    RegistrationNumberCheckOutcome
	numberType RegistrationNumberTypeRegistration
	hasType    bool
}

func (check RegistrationNumberCheck) Outcome() RegistrationNumberCheckOutcome {
	return check.outcome
}

// Type 交回判断所对照的类型修订；国家 / 地区未登记与类型未登记两格没有可对照的类型。
func (check RegistrationNumberCheck) Type() (RegistrationNumberTypeRegistration, bool) {
	return check.numberType, check.hasType
}

// RegistrationNumberTypeCatalogue 是一个租户在一个注册国家 / 地区下的注册号类型目录，每个类型
// 取其最新修订。身份登记与法人资料都按它判号（CONTEXT Rules「注册号的类型与格式按注册国家 /
// 地区取自本上下文拥有的注册号类型目录」），不各写一份格式。空目录是合法的——它说的正是
// 「这个国家 / 地区在目录里未登记」。
type RegistrationNumberTypeCatalogue struct {
	tenant  TenantID
	country RegistrationCountryCode
	types   map[RegistrationNumberTypeCode]RegistrationNumberTypeRegistration
}

// NewRegistrationNumberTypeCatalogue 要求每一笔都属同一租户与国家 / 地区、类型代码不重复——同一
// 代码出现两笔说明读侧没有只取最新修订，照原样收下就等于让校验随取数次序变答案。
func NewRegistrationNumberTypeCatalogue(
	tenant TenantID,
	country RegistrationCountryCode,
	latest []RegistrationNumberTypeRegistration,
) (RegistrationNumberTypeCatalogue, error) {
	if !tenant.valid() || !country.valid() {
		return RegistrationNumberTypeCatalogue{}, ErrInvalidRegistrationNumberTypeCatalogue
	}
	types := make(map[RegistrationNumberTypeCode]RegistrationNumberTypeRegistration, len(latest))
	for _, registration := range latest {
		if registration.tenant != tenant || registration.country != country || registration.revision < 1 {
			return RegistrationNumberTypeCatalogue{}, ErrInvalidRegistrationNumberTypeCatalogue
		}
		if _, duplicated := types[registration.code]; duplicated {
			return RegistrationNumberTypeCatalogue{}, fmt.Errorf(
				"%w: type %s appears twice", ErrInvalidRegistrationNumberTypeCatalogue, registration.code)
		}
		types[registration.code] = registration
	}
	return RegistrationNumberTypeCatalogue{tenant: tenant, country: country, types: types}, nil
}

func (catalogue RegistrationNumberTypeCatalogue) Tenant() TenantID {
	return catalogue.tenant
}

func (catalogue RegistrationNumberTypeCatalogue) Country() RegistrationCountryCode {
	return catalogue.country
}

// Check 回答一个按类型给出的号在明确时点是否属目录里的该类型、且落在要求的那一层、格式合格。
//
// 判断次序即答案的优先级：国家 / 地区 → 类型 → 层 → 生效期 → 格式。层排在生效期之前，因为交错
// 了层是登记方把一类号填进了另一格，不论那个类型此刻在不在用都要改号；目录里没有该国家 / 地区
// 时答`未登记`，不以任何默认格式代替（ADR-0145 决定一）。
func (catalogue RegistrationNumberTypeCatalogue) Check(
	code RegistrationNumberTypeCode,
	layer RegistrationNumberLayer,
	number RegistrationNumber,
	at time.Time,
) (RegistrationNumberCheck, error) {
	if !catalogue.tenant.valid() || !code.valid() || !layer.valid() || !number.valid() || at.IsZero() {
		return RegistrationNumberCheck{}, ErrInvalidRegistrationNumberCheck
	}
	if len(catalogue.types) == 0 {
		return RegistrationNumberCheck{outcome: RegistrationCountryNotRegistered}, nil
	}
	numberType, found := catalogue.types[code]
	if !found {
		return RegistrationNumberCheck{outcome: RegistrationNumberTypeNotRegistered}, nil
	}
	answer := func(outcome RegistrationNumberCheckOutcome) (RegistrationNumberCheck, error) {
		return RegistrationNumberCheck{outcome: outcome, numberType: numberType, hasType: true}, nil
	}
	if numberType.spec.Layer != layer {
		return answer(RegistrationNumberLayerMismatch)
	}
	if numberType.lifecycle.StatusAt(at) != RegistrationNumberTypeStatusEffective {
		return answer(RegistrationNumberTypeNotEffective)
	}
	if !numberType.spec.Format.matches(number) {
		return answer(RegistrationNumberFormatMismatch)
	}
	return answer(RegistrationNumberAccepted)
}
