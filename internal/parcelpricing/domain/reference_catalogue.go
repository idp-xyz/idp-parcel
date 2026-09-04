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
