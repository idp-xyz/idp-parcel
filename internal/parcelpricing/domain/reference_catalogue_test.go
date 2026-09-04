package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func catalogueLink(t testing.TB, kind domain.CatalogueKind, id string) domain.ReferenceCatalogueLink {
	t.Helper()
	link, err := domain.NewReferenceCatalogueLink(kind, id)
	if err != nil {
		t.Fatalf("catalogue link %s/%s: %v", kind, id, err)
	}
	return link
}

func structuresWithCatalogues(t testing.TB, links ...domain.ReferenceCatalogueLink) domain.PricingPlanStructures {
	t.Helper()
	structures, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(links...)
	if err != nil {
		t.Fatalf("structures with catalogues: %v", err)
	}
	return structures
}

// Covers: ADR-0109 Decision 三「价卡以目录绑定声明分区与档位从哪个目录来，绑标识不绑版本」——
// 一张卡对每一种目录至多声明一个来源；同一种声明两遍，评价就说不出该查哪一本。
func TestPlanDeclaresWhichCatalogueSuppliesZoneAndTier(t *testing.T) {
	zone := catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")
	tier := catalogueLink(t, domain.CatalogueKindRemoteTier, "carrier-das-table")

	structures := structuresWithCatalogues(t, zone, tier)
	links := structures.ReferenceCatalogues()
	if len(links) != 2 {
		t.Fatalf("declared %d catalogue links, want 2", len(links))
	}
	// 声明顺序不进内容：两个轴按种类排定，读的人与摘要看到的是同一份。
	if links[0].Kind() != domain.CatalogueKindRemoteTier || links[1].Kind() != domain.CatalogueKindZone {
		t.Fatalf("links are not ordered by kind: %s, %s", links[0].Kind(), links[1].Kind())
	}
	if links[1].CatalogueID() != "carrier-zone-chart" {
		t.Fatalf("zone link points at %q", links[1].CatalogueID())
	}
	if !structures.Declared() {
		t.Fatal("a card that binds a catalogue has declared a structure the evaluator must execute")
	}

	if _, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, " "); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("blank catalogue id accepted: %v", err)
	}
	if _, err := domain.NewReferenceCatalogueLink(domain.CatalogueKind("POSTAL_LIBRARY"), "x"); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("catalogue kind outside the closed set accepted: %v", err)
	}
	other := catalogueLink(t, domain.CatalogueKindZone, "another-zone-chart")
	if _, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(zone, other); !errors.Is(err, domain.ErrInvalidReferenceCatalogue) {
		t.Fatalf("two zone catalogues on one card accepted: %v", err)
	}
}

// Covers: ADR-0109 Decision 四「两条路径在卡的内容摘要里分得开」与 Consequences「规范化形状因绑定
// 进卡内容而换号……记在 PPC-5 下」——绑了目录的卡与没绑的卡摘要不同，而两张都没绑的卡不因这一格
// 多出差异；快照往返之后绑定与摘要都还在。
func TestCatalogueLinkEntersTheContentDigestAndSurvivesTheSnapshot(t *testing.T) {
	unbound := planWithStructures(t, domain.PricingPlanStructures{})
	unboundAgain := planWithStructures(t, domain.PricingPlanStructures{})
	bound := planWithStructures(t, structuresWithCatalogues(t, catalogueLink(t, domain.CatalogueKindZone, "carrier-zone-chart")))

	if unbound.ContentDigest() != unboundAgain.ContentDigest() {
		t.Fatal("two cards without catalogue links disagree on their digest")
	}
	if unbound.ContentDigest() == bound.ContentDigest() {
		t.Fatal("binding a catalogue left the content digest unchanged")
	}
	if bound.CanonicalizationVersion() != domain.CurrentCanonicalizationVersion() {
		t.Fatalf("canonicalization = %q, want the current version (PPC-5 already absorbed this widening)", bound.CanonicalizationVersion())
	}

	raw, err := domain.MarshalPricingPlanSnapshot(bound)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydratePricingPlanSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	links := restored.Structures().ReferenceCatalogues()
	if len(links) != 1 || links[0].Kind() != domain.CatalogueKindZone || links[0].CatalogueID() != "carrier-zone-chart" {
		t.Fatalf("catalogue link did not survive the snapshot: %#v", links)
	}
	if restored.ContentDigest() != bound.ContentDigest() {
		t.Fatal("rehydrated card recomputes a different digest")
	}
}
