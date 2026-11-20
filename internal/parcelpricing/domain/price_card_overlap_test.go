package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 半开区间的交、产品层会不会撞、以及重叠该走哪一种收尾，都在领域里钉死。适配器只负责把册上折完的选用区间交进来。

var (
	overlapJanuary = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	overlapJune    = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	overlapSept    = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	overlapNext    = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

func overlapPeriod(t *testing.T, starts, ends time.Time) domain.EffectivePeriod {
	t.Helper()
	period, err := domain.NewEffectivePeriod(starts, ends)
	if err != nil {
		t.Fatalf("适用期：%v", err)
	}
	return period
}

func overlapOpen(t *testing.T, starts time.Time) domain.EffectivePeriod {
	t.Helper()
	return overlapPeriod(t, starts, time.Time{})
}

func overlapPlan(t *testing.T, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, id, version)
	if err != nil {
		t.Fatalf("方案引用：%v", err)
	}
	return reference
}

func overlapProduct(t *testing.T, id string) domain.ServiceProductReference {
	t.Helper()
	product, err := domain.NewServiceProductReference(id)
	if err != nil {
		t.Fatalf("服务产品：%v", err)
	}
	return product
}

func TestEffectivePeriodIntersect(t *testing.T) {
	t.Parallel()
	bothOpen := overlapOpen(t, overlapJanuary)
	laterOpen := overlapOpen(t, overlapJune)
	firstHalf := overlapPeriod(t, overlapJanuary, overlapJune)
	secondHalf := overlapPeriod(t, overlapJune, overlapNext)
	middle := overlapPeriod(t, overlapJune, overlapSept)
	after := overlapPeriod(t, overlapSept, overlapNext)

	intersection, ok := bothOpen.Intersect(laterOpen)
	if !ok || !intersection.StartsAt().Equal(overlapJune) || !intersection.EndsAt().IsZero() {
		t.Fatalf("无上界 × 无上界 = %s–%s ok=%v，想要自 6 月起无上界", intersection.StartsAt(), intersection.EndsAt(), ok)
	}
	intersection, ok = middle.Intersect(laterOpen)
	if !ok || !intersection.StartsAt().Equal(overlapJune) || !intersection.EndsAt().Equal(overlapSept) {
		t.Fatalf("有上界 × 无上界 = %s–%s ok=%v，想要 [6 月, 9 月)", intersection.StartsAt(), intersection.EndsAt(), ok)
	}
	if _, ok = firstHalf.Intersect(secondHalf); ok {
		t.Fatal("首尾相接的半开区间被判成重叠了：旧版截止在 T、新版从 T 起，并不同时适用")
	}
	if _, ok = firstHalf.Intersect(after); ok {
		t.Fatal("完全不相交的两段被判成重叠了")
	}
	intersection, ok = bothOpen.Intersect(middle)
	if !ok || !intersection.StartsAt().Equal(middle.StartsAt()) || !intersection.EndsAt().Equal(middle.EndsAt()) {
		t.Fatalf("包含关系的交集 = %s–%s ok=%v，想要被包含的那一段", intersection.StartsAt(), intersection.EndsAt(), ok)
	}
}

func TestServiceProductsCollideOnTheSameTier(t *testing.T) {
	t.Parallel()
	productA := overlapProduct(t, "PROD-A")
	productB := overlapProduct(t, "PROD-B")
	if !domain.ServiceProductsCollide(nil, nil) {
		t.Fatal("两边都不声明产品，选卡落在同一层，应当算会撞")
	}
	if domain.ServiceProductsCollide([]domain.ServiceProductReference{productA}, nil) {
		t.Fatal("一边指名产品、一边不声明，分属两层，不该算会撞")
	}
	if domain.ServiceProductsCollide(nil, []domain.ServiceProductReference{productA}) {
		t.Fatal("一边不声明、一边指名产品，不该算会撞")
	}
	if domain.ServiceProductsCollide([]domain.ServiceProductReference{productA}, []domain.ServiceProductReference{productB}) {
		t.Fatal("只卖 A 与只卖 B 没有共同产品，不该算会撞")
	}
	if !domain.ServiceProductsCollide(
		[]domain.ServiceProductReference{productA, productB},
		[]domain.ServiceProductReference{productB},
	) {
		t.Fatal("两边都卖 A 时应当算会撞")
	}
}

func TestClassifyPriceCardOverlap(t *testing.T) {
	t.Parallel()
	sameV1 := overlapPlan(t, "SYN-PLAN", "v1")
	sameV2 := overlapPlan(t, "SYN-PLAN", "v2")
	other := overlapPlan(t, "SYN-OTHER", "v1")
	openJanuary := overlapOpen(t, overlapJanuary)
	openJune := overlapOpen(t, overlapJune)
	bounded := overlapPeriod(t, overlapJune, overlapSept)

	successor, ok := domain.ClassifyPriceCardOverlap(sameV2, openJune, nil, sameV1, openJanuary, nil)
	if !ok || !successor.SamePlan() || successor.Resolution() != domain.PriceCardOverlapSuccessorCut ||
		successor.Resolution().String() != "SUCCESSOR_CUT" ||
		!successor.Overlap().StartsAt().Equal(overlapJune) || !successor.Overlap().EndsAt().IsZero() ||
		!successor.Plan().SameIdentity(sameV1) || !successor.Selection().StartsAt().Equal(overlapJanuary) {
		t.Fatalf("后继晚于旧版且盖住尾部：ok=%v %+v %s", ok, successor.Plan(), successor.Resolution())
	}

	retire, ok := domain.ClassifyPriceCardOverlap(sameV2, openJanuary, nil, sameV1, openJune, nil)
	if !ok || retire.Resolution() != domain.PriceCardOverlapRetirePredecessor || retire.Resolution().String() != "RETIRE_PREDECESSOR" ||
		!retire.SamePlan() || !retire.Overlap().StartsAt().Equal(overlapJune) {
		t.Fatalf("后继不晚于旧版且盖住尾部：ok=%v %s", ok, retire.Resolution())
	}

	gap, ok := domain.ClassifyPriceCardOverlap(sameV2, bounded, nil, sameV1, openJanuary, nil)
	if !ok || !gap.SamePlan() || gap.Resolution() != domain.PriceCardOverlapGovernanceDecision ||
		gap.Resolution().String() != "GOVERNANCE_DECISION" {
		t.Fatalf("新版有终点、盖不住无上界旧版：ok=%v same=%v %s", ok, gap.SamePlan(), gap.Resolution())
	}

	across, ok := domain.ClassifyPriceCardOverlap(other, openJune, nil, sameV1, openJanuary, nil)
	if !ok || across.SamePlan() || across.Resolution() != domain.PriceCardOverlapGovernanceDecision {
		t.Fatalf("不同方案：ok=%v same=%v %s", ok, across.SamePlan(), across.Resolution())
	}

	if _, ok = domain.ClassifyPriceCardOverlap(sameV1, openJanuary, nil, sameV1, openJanuary, nil); ok {
		t.Fatal("同方案同版本被当成了另一版重叠")
	}
	withFingerprint, err := domain.NewVersionReferenceWithFingerprint(domain.ArtifactPricingPlan, "SYN-PLAN", "v1", "sha256:syn")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok = domain.ClassifyPriceCardOverlap(withFingerprint, openJanuary, nil, sameV1, openJanuary, nil); ok {
		t.Fatal("只差指纹的同一版被当成了重叠")
	}
	if _, ok = domain.ClassifyPriceCardOverlap(sameV2, overlapPeriod(t, overlapSept, overlapNext), nil, sameV1, overlapPeriod(t, overlapJanuary, overlapJune), nil); ok {
		t.Fatal("适用期不相交仍报了重叠")
	}
	productA := overlapProduct(t, "PROD-A")
	if _, ok = domain.ClassifyPriceCardOverlap(other, openJune, []domain.ServiceProductReference{productA}, sameV1, openJanuary, nil); ok {
		t.Fatal("产品层不会撞仍报了重叠")
	}
}

func TestPriceCardOverlapResolutionNames(t *testing.T) {
	t.Parallel()
	if domain.PriceCardOverlapResolutionInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
	named := []struct {
		resolution domain.PriceCardOverlapResolution
		name       string
	}{
		{domain.PriceCardOverlapSuccessorCut, "SUCCESSOR_CUT"},
		{domain.PriceCardOverlapRetirePredecessor, "RETIRE_PREDECESSOR"},
		{domain.PriceCardOverlapGovernanceDecision, "GOVERNANCE_DECISION"},
	}
	for _, item := range named {
		if item.resolution.String() != item.name {
			t.Fatalf("%s 的名字是 %q", item.name, item.resolution.String())
		}
	}
}
