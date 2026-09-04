package domain

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidReferenceCatalogue 表示与计价参考目录有关的声明立不住：目录种类不在封闭集、目录
// 标识为空、同一种目录在一张卡上声明了两个来源、或一次解析读数的形状不合。
var ErrInvalidReferenceCatalogue = errors.New("parcel pricing: invalid reference catalogue")

// CatalogueKind 是计价参考目录的封闭种类（CONTEXT「计价参考目录」：首发闭合为分区表与偏远档位表两种；
// ADR-0109 Decision 二）。两种的取值都是类别不是数值——这正是它与计价参考序列分立成两个词条的理由。
type CatalogueKind string

const (
	// CatalogueKindZone：目的邮编前缀 → 分区。解析结果供基础运费查表与 FeatureZone 判定条件读。
	CatalogueKindZone CatalogueKind = "ZONE"
	// CatalogueKindRemoteTier：目的邮编前缀 → 偏远档位（CONTEXT「地址分类」）。解析结果按 ADR-0109
	// Context 的对应关系落到 FeatureAddressType 那一格类别特征上，只被附加费规则的判定条件读。
	CatalogueKindRemoteTier CatalogueKind = "REMOTE_TIER"
)

func (kind CatalogueKind) String() string { return string(kind) }

func (kind CatalogueKind) valid() bool {
	switch kind {
	case CatalogueKindZone, CatalogueKindRemoteTier:
		return true
	default:
		return false
	}
}

// ReferenceCatalogueLink 是价卡上的「目录绑定」（ADR-0109 Decision 三）：声明「我的分区 / 偏远档位来自
// 目录 X」，绑的是目录标识不是版本——采用哪一版由评价基准时点的在用版本决定，那一版的引用由评价自己
// 的清单冻结，形照 ReferenceSeriesBinding。Go 名不用 Binding 词根：同一个包里「绑定」一词已由序列绑定与
// 来源连接器绑定各占一格，第三个同词根的类型会让三种「绑定」在符号面上分不开。
type ReferenceCatalogueLink struct {
	kind        CatalogueKind
	catalogueID string
}

func NewReferenceCatalogueLink(kind CatalogueKind, catalogueID string) (ReferenceCatalogueLink, error) {
	link := ReferenceCatalogueLink{kind: kind, catalogueID: catalogueID}
	if !link.valid() {
		return ReferenceCatalogueLink{}, ErrInvalidReferenceCatalogue
	}
	return link, nil
}

func (link ReferenceCatalogueLink) Kind() CatalogueKind { return link.kind }

// CatalogueID 是租户内的目录标识，与目录登记册里的目录 ID 同一取值。
func (link ReferenceCatalogueLink) CatalogueID() string { return link.catalogueID }

func (link ReferenceCatalogueLink) valid() bool {
	return link.kind.valid() && trimmed(link.catalogueID)
}

func compareCatalogueLinks(left, right ReferenceCatalogueLink) int {
	for _, pair := range [][2]string{
		{string(left.kind), string(right.kind)},
		{left.catalogueID, right.catalogueID},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

// WithReferenceCatalogues 返回携带目录绑定的结构集合（ADR-0109 Decision 三）。做成 builder 而不是构造
// 参数，理由同金额取整策略：它是可缺的另一个轴，没绑目录的卡保留调用方给分区的路径。每一种目录至多
// 一个来源——同一种声明两个，评价说不出该查哪一本。
func (structures PricingPlanStructures) WithReferenceCatalogues(links ...ReferenceCatalogueLink) (PricingPlanStructures, error) {
	copyOfLinks := append([]ReferenceCatalogueLink(nil), links...)
	seen := make(map[CatalogueKind]struct{}, len(copyOfLinks))
	for _, link := range copyOfLinks {
		if !link.valid() {
			return PricingPlanStructures{}, ErrInvalidReferenceCatalogue
		}
		if _, exists := seen[link.kind]; exists {
			return PricingPlanStructures{}, fmt.Errorf("%w: %s catalogue declared twice", ErrInvalidReferenceCatalogue, link.kind)
		}
		seen[link.kind] = struct{}{}
	}
	sort.SliceStable(copyOfLinks, func(left, right int) bool {
		return compareCatalogueLinks(copyOfLinks[left], copyOfLinks[right]) < 0
	})
	structures.referenceCatalogues = copyOfLinks
	if !structures.valid() {
		return PricingPlanStructures{}, ErrInvalidPlanStructures
	}
	return structures, nil
}

// ReferenceCatalogues 交回卡声明的目录绑定，按种类排定。
func (structures PricingPlanStructures) ReferenceCatalogues() []ReferenceCatalogueLink {
	return append([]ReferenceCatalogueLink(nil), structures.referenceCatalogues...)
}

// catalogueLink 交回卡对某一种目录的绑定；没绑即第二个返回值为假——那是「保留调用方给值路径」的输入。
func (structures PricingPlanStructures) catalogueLink(kind CatalogueKind) (ReferenceCatalogueLink, bool) {
	for _, link := range structures.referenceCatalogues {
		if link.kind == kind {
			return link, true
		}
	}
	return ReferenceCatalogueLink{}, false
}

var (
	// ErrZoneUnresolved 表示本次评价得不到分区（ADR-0109 Decision 四）：绑了分区目录却没有读数、在用版本里
	// 查不到该目的邮编，或卡没绑目录而输入也没给分区。三种都是证据不足，评价待判断，不给默认分区。
	ErrZoneUnresolved = errors.New("parcel pricing: zone unresolved")
	// ErrRemoteTierUnresolved 是偏远档位那一格的同形错误，与分区分开——前者影响基础运费查表，后者只影响
	// 一类附加费，续办不同。
	ErrRemoteTierUnresolved = errors.New("parcel pricing: remote tier unresolved")
	// ErrReferenceCatalogueMismatch 表示读数来自卡从未声明过的目录：那是分歧不是缺口，按别人的分区表
	// 计价会静默用错分区。
	ErrReferenceCatalogueMismatch = errors.New("parcel pricing: catalogue reading comes from a catalogue the plan did not bind")
)

// PostalRoute 是评价输入里的邮编路线：目的邮编必备，始发邮编随目录的始发维度需要而给。它与调用方直接
// 给的分区是两条路径（ADR-0109 Decision 四）：绑了目录的卡从这里解分区，没绑的卡读调用方给的那一格。
type PostalRoute struct {
	origin      string
	destination string
}

func NewPostalRoute(origin, destination string) (PostalRoute, error) {
	route := PostalRoute{origin: origin, destination: destination}
	if !route.valid() {
		return PostalRoute{}, ErrPricingInputInvalid
	}
	return route, nil
}

// Origin 只在给了始发邮编时非空。
func (route PostalRoute) Origin() string      { return route.origin }
func (route PostalRoute) Destination() string { return route.destination }

func (route PostalRoute) valid() bool {
	if !trimmed(route.destination) {
		return false
	}
	return route.origin == "" || trimmed(route.origin)
}

// ResolvedCatalogueValue 是一次按计价基准时点对某本目录的解析读数，冻结进评价输入：解出了值就带值；在用
// 版本里查不到该邮编时值缺席但版本引用仍在——「查过哪一版、没查到」是评价的证据，重放要能按原读数得出
// 同一个待判断。取值随快照进来而不是评价过程中现查，理由与序列取值同（重放不得读目录今天的版本）。
type ResolvedCatalogueValue struct {
	kind      CatalogueKind
	reference VersionReference
	value     CategoryValue
	resolved  bool
}

func NewResolvedCatalogueValue(kind CatalogueKind, reference VersionReference, value CategoryValue) (ResolvedCatalogueValue, error) {
	reading := ResolvedCatalogueValue{kind: kind, reference: reference, value: value, resolved: true}
	if !reading.valid() {
		return ResolvedCatalogueValue{}, ErrInvalidReferenceCatalogue
	}
	return reading, nil
}

// NewUnresolvedCatalogueReading 记下「查过这一版目录、没查到」：值缺席，版本引用在。
func NewUnresolvedCatalogueReading(kind CatalogueKind, reference VersionReference) (ResolvedCatalogueValue, error) {
	reading := ResolvedCatalogueValue{kind: kind, reference: reference}
	if !reading.valid() {
		return ResolvedCatalogueValue{}, ErrInvalidReferenceCatalogue
	}
	return reading, nil
}

func (reading ResolvedCatalogueValue) Kind() CatalogueKind         { return reading.kind }
func (reading ResolvedCatalogueValue) Reference() VersionReference { return reading.reference }

// Value 只在解出了值时给出。
func (reading ResolvedCatalogueValue) Value() (CategoryValue, bool) {
	return reading.value, reading.resolved
}

func (reading ResolvedCatalogueValue) valid() bool {
	if !reading.kind.valid() || reading.reference.kind != ArtifactReferenceCatalogue || !reading.reference.valid() {
		return false
	}
	if reading.resolved {
		return reading.value.valid() && trimmed(string(reading.value))
	}
	return reading.value == ""
}

// WithReferenceCatalogues 返回一份携带目录读数的副本，每种目录至多一条——同一种两条会把选哪一条交给遍历
// 顺序决定。
func (input PricingInputSnapshot) WithReferenceCatalogues(readings ...ResolvedCatalogueValue) (PricingInputSnapshot, error) {
	if !input.valid() {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	seen := make(map[CatalogueKind]struct{}, len(readings))
	copyOfReadings := make([]ResolvedCatalogueValue, 0, len(readings))
	for _, reading := range readings {
		if !reading.valid() {
			return PricingInputSnapshot{}, ErrInvalidReferenceCatalogue
		}
		if _, exists := seen[reading.kind]; exists {
			return PricingInputSnapshot{}, fmt.Errorf("%w: duplicate reading for %s", ErrInvalidReferenceCatalogue, reading.kind)
		}
		seen[reading.kind] = struct{}{}
		copyOfReadings = append(copyOfReadings, reading)
	}
	sort.SliceStable(copyOfReadings, func(left, right int) bool {
		return copyOfReadings[left].kind < copyOfReadings[right].kind
	})
	updated := copyInputSnapshot(input)
	updated.catalogueReadings = copyOfReadings
	return updated, nil
}

// CatalogueReadings 报出冻结进本快照的目录读数。
func (input PricingInputSnapshot) CatalogueReadings() []ResolvedCatalogueValue {
	return append([]ResolvedCatalogueValue(nil), input.catalogueReadings...)
}

func (input PricingInputSnapshot) catalogueReading(kind CatalogueKind) (ResolvedCatalogueValue, bool) {
	for _, reading := range input.catalogueReadings {
		if reading.kind == kind {
			return reading, true
		}
	}
	return ResolvedCatalogueValue{}, false
}

// boundCatalogueReferences 列出落在卡的目录绑定上的目录版本引用——这些是本次评价实际查过的版本，要进
// 评价自己的清单（ADR-0109 Decision 三）。没查到值的读数同样进清单：查过就是采用过。
func (input PricingInputSnapshot) boundCatalogueReferences(links []ReferenceCatalogueLink) []VersionReference {
	references := make([]VersionReference, 0, len(links))
	for _, link := range links {
		reading, found := input.catalogueReading(link.kind)
		if found && reading.reference.ID() == link.catalogueID {
			references = append(references, reading.reference)
		}
	}
	return references
}

// catalogueResolution 是一次评价从目录（或调用方）得到的类别量：分区必有，偏远档位只在卡绑了档位目录时有。
type catalogueResolution struct {
	zone         CategoryValue
	tier         CategoryValue
	explanations []string
}

// resolveCatalogues 按卡的目录绑定从输入的读数解出分区与偏远档位（ADR-0109 Decision 四）。分区：绑了目录
// 就只认读数，没绑就读调用方给的那一格；两边都没有即待判断，不编一个。档位只在绑了档位目录时解，没绑
// 的卡该格缺席，地址类型条件读到缺席仍按既有语义报特征不可用。
func (structures PricingPlanStructures) resolveCatalogues(input PricingInputSnapshot) (catalogueResolution, error) {
	resolution := catalogueResolution{}
	destination := ""
	if route, declared := input.PostalRoute(); declared {
		destination = route.destination
	}
	if link, bound := structures.catalogueLink(CatalogueKindZone); bound {
		value, explanation, err := resolveCatalogueValue(link, input, destination, ErrZoneUnresolved)
		if err != nil {
			return catalogueResolution{}, err
		}
		resolution.zone = value
		resolution.explanations = append(resolution.explanations, explanation)
	} else {
		if input.zone == "" {
			return catalogueResolution{}, fmt.Errorf("%w: the price card binds no zone catalogue and the input names no zone", ErrZoneUnresolved)
		}
		resolution.zone = CategoryValue(input.zone)
	}
	if link, bound := structures.catalogueLink(CatalogueKindRemoteTier); bound {
		value, explanation, err := resolveCatalogueValue(link, input, destination, ErrRemoteTierUnresolved)
		if err != nil {
			return catalogueResolution{}, err
		}
		resolution.tier = value
		resolution.explanations = append(resolution.explanations, explanation)
	}
	return resolution, nil
}

func resolveCatalogueValue(link ReferenceCatalogueLink, input PricingInputSnapshot, destination string, unresolved error) (CategoryValue, string, error) {
	reading, found := input.catalogueReading(link.kind)
	if !found {
		return "", "", fmt.Errorf("%w: no reading from %s catalogue %s", unresolved, link.kind, link.catalogueID)
	}
	if reading.reference.ID() != link.catalogueID {
		return "", "", fmt.Errorf("%w: %s reading comes from catalogue %s but the plan bound catalogue %s",
			ErrReferenceCatalogueMismatch, link.kind, reading.reference.ID(), link.catalogueID)
	}
	if !reading.resolved {
		return "", "", fmt.Errorf("%w: %s catalogue %s@%s has no entry for destination %s",
			unresolved, link.kind, reading.reference.ID(), reading.reference.Version(), destination)
	}
	return reading.value, fmt.Sprintf("reference catalogue %s resolved to %s from %s@%s for destination %s",
		link.kind, reading.value, reading.reference.ID(), reading.reference.Version(), destination), nil
}

func validCatalogueLinks(links []ReferenceCatalogueLink) bool {
	seen := make(map[CatalogueKind]struct{}, len(links))
	for index, link := range links {
		if !link.valid() {
			return false
		}
		if index > 0 && compareCatalogueLinks(links[index-1], link) >= 0 {
			return false
		}
		if _, exists := seen[link.kind]; exists {
			return false
		}
		seen[link.kind] = struct{}{}
	}
	return true
}
