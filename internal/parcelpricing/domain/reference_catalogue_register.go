package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	// ErrInvalidReferenceCatalogueRegistration 表示目录登记立不住：来源标识、登记责任方、始发维度、
	// 映射或生效区间缺位（CONTEXT「计价参考目录」四件），条目长度与声明粒度不符、前缀重复，或更正
	// 关系不成对。
	ErrInvalidReferenceCatalogueRegistration = errors.New("parcel pricing: invalid reference catalogue registration")
	// ErrReferenceCatalogueRegistrationSnapshotInvalid 表示目录快照解不出一版立得住的登记——坏写入或
	// 被改过的条目在重建门上暴露，而不是变成一本看起来合法的目录。
	ErrReferenceCatalogueRegistrationSnapshotInvalid = errors.New("parcel pricing: invalid reference catalogue registration snapshot")
)

// catalogueCanonicalization 标识目录登记快照与其内容摘要所依据的文档形状（ADR-0014 同一条纪律）。目录
// 与序列、价卡各持一号，各自演进。
const catalogueCanonicalization = "PRC-1"

// CatalogueOriginScope 是目录始发维度的封闭集（ADR-0109 Decision 二「始发维度」）。偏远档位表通常不
// 区分始发，分区表按始发邮编前缀集分表——两种都是登记方对该表的显式声明，没有一种是默认。
type CatalogueOriginScope string

const (
	// CatalogueOriginIndependent：该表对任何始发都适用。
	CatalogueOriginIndependent CatalogueOriginScope = "INDEPENDENT"
	// CatalogueOriginPostalPrefixes：该表只覆盖声明的始发邮编前缀集。
	CatalogueOriginPostalPrefixes CatalogueOriginScope = "POSTAL_PREFIXES"
)

func (scope CatalogueOriginScope) String() string { return string(scope) }

func (scope CatalogueOriginScope) valid() bool {
	switch scope {
	case CatalogueOriginIndependent, CatalogueOriginPostalPrefixes:
		return true
	default:
		return false
	}
}

// CatalogueOrigin 是一版目录声明的始发维度。零值不是一种声明——始发维度未声明的登记不立。
type CatalogueOrigin struct {
	scope    CatalogueOriginScope
	prefixes []string
}

func NewIndependentCatalogueOrigin() CatalogueOrigin {
	return CatalogueOrigin{scope: CatalogueOriginIndependent}
}

// NewPostalPrefixCatalogueOrigin 声明该表覆盖的始发邮编前缀集：非空、去重、排定。前缀可长短不一——始发侧
// 按「以之开头」判，粒度那一格只对目的侧声明。
func NewPostalPrefixCatalogueOrigin(prefixes []string) (CatalogueOrigin, error) {
	origin := CatalogueOrigin{scope: CatalogueOriginPostalPrefixes, prefixes: uniqueSorted(prefixes)}
	if !origin.valid() {
		return CatalogueOrigin{}, ErrInvalidReferenceCatalogueRegistration
	}
	return origin, nil
}

func (origin CatalogueOrigin) Scope() CatalogueOriginScope { return origin.scope }

// Prefixes 只在按始发邮编前缀集声明时非空。
func (origin CatalogueOrigin) Prefixes() []string {
	return append([]string(nil), origin.prefixes...)
}

func (origin CatalogueOrigin) valid() bool {
	switch origin.scope {
	case CatalogueOriginIndependent:
		return len(origin.prefixes) == 0
	case CatalogueOriginPostalPrefixes:
		if len(origin.prefixes) == 0 {
			return false
		}
		for index, prefix := range origin.prefixes {
			if !trimmed(prefix) || (index > 0 && origin.prefixes[index-1] >= prefix) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// covers 判该表对这个始发邮编是否作答。不区分始发的表对空始发也作答；按前缀集声明的表要求始发邮编
// 以某个声明前缀开头。
func (origin CatalogueOrigin) covers(originPostal string) bool {
	if origin.scope == CatalogueOriginIndependent {
		return true
	}
	for _, prefix := range origin.prefixes {
		if strings.HasPrefix(originPostal, prefix) {
			return true
		}
	}
	return false
}

// CatalogueEntry 是映射里的一行：目的邮编前缀 → 类别值。取值目录属实例半边，这里只要求非空。
type CatalogueEntry struct {
	prefix string
	value  CategoryValue
}

func NewCatalogueEntry(prefix string, value CategoryValue) (CatalogueEntry, error) {
	entry := CatalogueEntry{prefix: prefix, value: value}
	if !entry.valid() {
		return CatalogueEntry{}, ErrInvalidReferenceCatalogueRegistration
	}
	return entry, nil
}

func (entry CatalogueEntry) Prefix() string       { return entry.prefix }
func (entry CatalogueEntry) Value() CategoryValue { return entry.value }

func (entry CatalogueEntry) valid() bool {
	return trimmed(entry.prefix) && trimmed(string(entry.value))
}

// ReferenceCatalogueRegistrationSpec 是登记一版计价参考目录所需的全部输入。内容不归计价生产（ADR-0109
// Decision 一）：这里收的是登记责任方对承运商公布分区表 / 偏远档位表的转录。
type ReferenceCatalogueRegistrationSpec struct {
	Tenant           TenantID
	Kind             CatalogueKind
	Reference        VersionReference
	SourceIdentifier string
	Registrant       string
	Origin           CatalogueOrigin
	// PrefixLength 是目的邮编前缀匹配的粒度（3 位 / 5 位……），由这一版自己声明——CONTEXT「随首份真实
	// 分区表声明，不预拟」，机制只给槽。
	PrefixLength int
	Entries      []CatalogueEntry
	Period       EffectivePeriod
	// PriorVersion 与 CorrectionBasis 成对声明更正关系：映射发现错误时形成新版本，原版本一字不动。
	PriorVersion    VersionReference
	CorrectionBasis string
}

// ReferenceCatalogueRegistration 是一版已立得住的目录登记。登记一版映射是一次来源事实断言：转抄错误由
// 登记责任方承担，与序列登记同一条责任线。
type ReferenceCatalogueRegistration struct {
	tenant           TenantID
	kind             CatalogueKind
	reference        VersionReference
	sourceIdentifier string
	registrant       string
	origin           CatalogueOrigin
	prefixLength     int
	entries          []CatalogueEntry
	period           EffectivePeriod
	priorVersion     *VersionReference
	correctionBasis  string
}

func NewReferenceCatalogueRegistration(spec ReferenceCatalogueRegistrationSpec) (ReferenceCatalogueRegistration, error) {
	entries := append([]CatalogueEntry(nil), spec.Entries...)
	sort.SliceStable(entries, func(left, right int) bool {
		return entries[left].prefix < entries[right].prefix
	})
	registration := ReferenceCatalogueRegistration{
		tenant:           spec.Tenant,
		kind:             spec.Kind,
		reference:        spec.Reference,
		sourceIdentifier: spec.SourceIdentifier,
		registrant:       spec.Registrant,
		origin:           spec.Origin,
		prefixLength:     spec.PrefixLength,
		entries:          entries,
		period:           spec.Period,
		correctionBasis:  spec.CorrectionBasis,
	}
	if spec.PriorVersion != (VersionReference{}) {
		prior := spec.PriorVersion
		registration.priorVersion = &prior
	}
	if !registration.valid() {
		return ReferenceCatalogueRegistration{}, ErrInvalidReferenceCatalogueRegistration
	}
	return registration, nil
}

func (registration ReferenceCatalogueRegistration) Tenant() TenantID    { return registration.tenant }
func (registration ReferenceCatalogueRegistration) Kind() CatalogueKind { return registration.kind }
func (registration ReferenceCatalogueRegistration) Reference() VersionReference {
	return registration.reference
}
func (registration ReferenceCatalogueRegistration) SourceIdentifier() string {
	return registration.sourceIdentifier
}
func (registration ReferenceCatalogueRegistration) Registrant() string {
	return registration.registrant
}
func (registration ReferenceCatalogueRegistration) Origin() CatalogueOrigin {
	return registration.origin
}
func (registration ReferenceCatalogueRegistration) PrefixLength() int {
	return registration.prefixLength
}
func (registration ReferenceCatalogueRegistration) Period() EffectivePeriod {
	return registration.period
}
func (registration ReferenceCatalogueRegistration) Canonicalization() string {
	return catalogueCanonicalization
}
func (registration ReferenceCatalogueRegistration) Entries() []CatalogueEntry {
	return append([]CatalogueEntry(nil), registration.entries...)
}

// Correction 只在更正版本上给出：指回被更正的版本与更正依据。
func (registration ReferenceCatalogueRegistration) Correction() (VersionReference, string, bool) {
	if registration.priorVersion == nil {
		return VersionReference{}, "", false
	}
	return *registration.priorVersion, registration.correctionBasis, true
}

func (registration ReferenceCatalogueRegistration) valid() bool {
	if !registration.tenant.valid() ||
		!registration.kind.valid() ||
		registration.reference.kind != ArtifactReferenceCatalogue ||
		!registration.reference.valid() ||
		!trimmed(registration.sourceIdentifier) ||
		!trimmed(registration.registrant) ||
		!registration.origin.valid() ||
		registration.prefixLength <= 0 ||
		!registration.period.valid() ||
		len(registration.entries) == 0 {
		return false
	}
	for index, entry := range registration.entries {
		// 条目长度必须等于声明的粒度：一张 3 位表里混进一条 5 位前缀，那条永远查不到，而没有任何东西会报。
		if !entry.valid() || len(entry.prefix) != registration.prefixLength {
			return false
		}
		if index > 0 && registration.entries[index-1].prefix >= entry.prefix {
			return false
		}
	}
	// 更正两件成对且不自指、不换目录身份：换身份就是另一本目录，只能另行登记。
	if registration.priorVersion == nil {
		return registration.correctionBasis == ""
	}
	if !trimmed(registration.correctionBasis) {
		return false
	}
	prior := *registration.priorVersion
	if prior.kind != ArtifactReferenceCatalogue || !prior.valid() {
		return false
	}
	return prior.id == registration.reference.id && prior.version != registration.reference.version
}

// ResolveAt 按计价基准时点与邮编路线解一个读数（ADR-0109 Decision 四）。第二个返回值说的是这一版在该
// 时点适用不适用（生效区间 [起, 止)）；适用而查不到——始发不在覆盖内、目的邮编短于粒度、前缀不在表里
// ——交回「查过没查到」的读数，值缺席、版本引用在。不编默认分区。
func (registration ReferenceCatalogueRegistration) ResolveAt(asOf time.Time, route PostalRoute) (ResolvedCatalogueValue, bool) {
	if !registration.period.Contains(asOf) || !route.valid() {
		return ResolvedCatalogueValue{}, false
	}
	consulted := ResolvedCatalogueValue{kind: registration.kind, reference: registration.reference}
	if !registration.origin.covers(route.origin) || len(route.destination) < registration.prefixLength {
		return consulted, true
	}
	prefix := route.destination[:registration.prefixLength]
	for _, entry := range registration.entries {
		if entry.prefix == prefix {
			consulted.value = entry.value
			consulted.resolved = true
			return consulted, true
		}
	}
	return consulted, true
}

type catalogueEntrySnapshot struct {
	Prefix string `json:"prefix"`
	Value  string `json:"value"`
}

type catalogueOriginSnapshot struct {
	Scope    string   `json:"scope"`
	Prefixes []string `json:"prefixes,omitempty"`
}

type referenceCatalogueRegistrationSnapshot struct {
	Canonicalization string                    `json:"canonicalization"`
	Tenant           string                    `json:"tenant"`
	Kind             string                    `json:"kind"`
	Reference        versionReferenceSnapshot  `json:"reference"`
	SourceIdentifier string                    `json:"sourceIdentifier"`
	Registrant       string                    `json:"registrant"`
	Origin           catalogueOriginSnapshot   `json:"origin"`
	PrefixLength     int                       `json:"prefixLength"`
	Entries          []catalogueEntrySnapshot  `json:"entries"`
	Period           periodSnapshot            `json:"period"`
	PriorVersion     *versionReferenceSnapshot `json:"priorVersion,omitempty"`
	CorrectionBasis  string                    `json:"correctionBasis,omitempty"`
	ContentDigest    string                    `json:"contentDigest"`
}

// ContentDigest 是整版登记（含每一条映射）的内容指纹，按 PRC 形状规范化后计算，只在同一形状版本内可比。
// 引用只以三元入摘要（ADR-0108 Decision 二）。
func (registration ReferenceCatalogueRegistration) ContentDigest() string {
	document := registration.snapshotDocument()
	document.ContentDigest = ""
	document.Reference = document.Reference.identityOnly()
	if document.PriorVersion != nil {
		prior := document.PriorVersion.identityOnly()
		document.PriorVersion = &prior
	}
	return hashCanonical(document)
}

func (registration ReferenceCatalogueRegistration) snapshotDocument() referenceCatalogueRegistrationSnapshot {
	document := referenceCatalogueRegistrationSnapshot{
		Canonicalization: catalogueCanonicalization,
		Tenant:           registration.tenant.String(),
		Kind:             registration.kind.String(),
		Reference:        versionReferenceOf(registration.reference),
		SourceIdentifier: registration.sourceIdentifier,
		Registrant:       registration.registrant,
		Origin:           catalogueOriginSnapshot{Scope: registration.origin.scope.String(), Prefixes: registration.origin.Prefixes()},
		PrefixLength:     registration.prefixLength,
		Period:           periodSnapshot{StartsAt: registration.period.startsAt, EndsAt: registration.period.endsAt},
		CorrectionBasis:  registration.correctionBasis,
	}
	if registration.priorVersion != nil {
		prior := versionReferenceOf(*registration.priorVersion)
		document.PriorVersion = &prior
	}
	for _, entry := range registration.entries {
		document.Entries = append(document.Entries, catalogueEntrySnapshot{Prefix: entry.prefix, Value: string(entry.value)})
	}
	return document
}

// MarshalReferenceCatalogueRegistration 把一版目录登记折成持久化快照。快照携带自己的内容摘要，读回经同一道
// 整版重验——列面只是比对，权威内容在快照。
func MarshalReferenceCatalogueRegistration(registration ReferenceCatalogueRegistration) ([]byte, error) {
	if !registration.valid() {
		return nil, ErrReferenceCatalogueRegistrationSnapshotInvalid
	}
	document := registration.snapshotDocument()
	document.ContentDigest = registration.ContentDigest()
	return json.Marshal(document)
}

// PeekReferenceCatalogueRegistrationReference 只从快照里读出这一版的版本引用，不重建整版——在用解析要在一本
// 目录的全部版本上挑一版，挑完才对选中的那一版整版重验；万行级映射逐版重建的代价随版本数乘条目数增长。
func PeekReferenceCatalogueRegistrationReference(raw []byte) (VersionReference, error) {
	var document struct {
		Canonicalization string                   `json:"canonicalization"`
		Reference        versionReferenceSnapshot `json:"reference"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return VersionReference{}, fmt.Errorf("%w: %v", ErrReferenceCatalogueRegistrationSnapshotInvalid, err)
	}
	if document.Canonicalization != catalogueCanonicalization {
		return VersionReference{}, fmt.Errorf("%w: snapshot records %q, this build canonicalizes %q",
			ErrCanonicalizationVersionUnsupported, document.Canonicalization, catalogueCanonicalization)
	}
	reference := versionReferenceFrom(document.Reference)
	if reference.kind != ArtifactReferenceCatalogue || !reference.valid() {
		return VersionReference{}, ErrReferenceCatalogueRegistrationSnapshotInvalid
	}
	return reference, nil
}

// RehydrateReferenceCatalogueRegistration 从快照重建目录登记并整版重验：形状版本不被当前构建支持时拒绝
// 重建，登记后被改过的条目以摘要自校暴露。
func RehydrateReferenceCatalogueRegistration(raw []byte) (ReferenceCatalogueRegistration, error) {
	var document referenceCatalogueRegistrationSnapshot
	if err := json.Unmarshal(raw, &document); err != nil {
		return ReferenceCatalogueRegistration{}, fmt.Errorf("%w: %v", ErrReferenceCatalogueRegistrationSnapshotInvalid, err)
	}
	if document.Canonicalization != catalogueCanonicalization {
		return ReferenceCatalogueRegistration{}, fmt.Errorf("%w: snapshot records %q, this build canonicalizes %q",
			ErrCanonicalizationVersionUnsupported, document.Canonicalization, catalogueCanonicalization)
	}
	registration := ReferenceCatalogueRegistration{
		tenant:           TenantID{identifier{value: document.Tenant}},
		kind:             CatalogueKind(document.Kind),
		reference:        versionReferenceFrom(document.Reference),
		sourceIdentifier: document.SourceIdentifier,
		registrant:       document.Registrant,
		origin:           CatalogueOrigin{scope: CatalogueOriginScope(document.Origin.Scope), prefixes: append([]string(nil), document.Origin.Prefixes...)},
		prefixLength:     document.PrefixLength,
		period:           EffectivePeriod{startsAt: document.Period.StartsAt, endsAt: document.Period.EndsAt},
		correctionBasis:  document.CorrectionBasis,
	}
	if document.PriorVersion != nil {
		prior := versionReferenceFrom(*document.PriorVersion)
		registration.priorVersion = &prior
	}
	for _, row := range document.Entries {
		registration.entries = append(registration.entries, CatalogueEntry{prefix: row.Prefix, value: CategoryValue(row.Value)})
	}
	if !registration.valid() {
		return ReferenceCatalogueRegistration{}, ErrReferenceCatalogueRegistrationSnapshotInvalid
	}
	if registration.ContentDigest() != document.ContentDigest {
		return ReferenceCatalogueRegistration{}, fmt.Errorf(
			"%w: content digest disagrees with the snapshot body", ErrReferenceCatalogueRegistrationSnapshotInvalid)
	}
	return registration, nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
