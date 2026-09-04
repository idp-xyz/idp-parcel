package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证计价参考目录登记册的领域半边（ADR-0109 Decision 二）：一版目录声明来源标识、始发维度、
// 目的邮编前缀映射与生效区间，前缀粒度由该版自己声明；按计价基准时点选版、按目的邮编解出类别值，
// 查不到即不给答案。夹具全部为 SYN 合成目录（S 级），邮编与分区取值都是编出来的形状，不是任何真表。

var (
	catalogueYearStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	catalogueYearEnd   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

func catalogueEntry(t testing.TB, prefix, value string) domain.CatalogueEntry {
	t.Helper()
	entry, err := domain.NewCatalogueEntry(prefix, domain.CategoryValue(value))
	if err != nil {
		t.Fatalf("目录条目 %s→%s：%v", prefix, value, err)
	}
	return entry
}

func zoneChartSpec(t testing.TB) domain.ReferenceCatalogueRegistrationSpec {
	t.Helper()
	period, err := domain.NewEffectivePeriod(catalogueYearStart, catalogueYearEnd)
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	origin, err := domain.NewPostalPrefixCatalogueOrigin([]string{"940", "941"})
	if err != nil {
		t.Fatalf("始发维度：%v", err)
	}
	return domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.CatalogueKindZone,
		Reference:        versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", "v1"),
		SourceIdentifier: "SYN-CARRIER/zone-chart-2026",
		Registrant:       "SYN-CAT-REGISTRAR",
		Origin:           origin,
		PrefixLength:     3,
		Entries: []domain.CatalogueEntry{
			catalogueEntry(t, "100", "Z8"),
			catalogueEntry(t, "941", "Z2"),
			catalogueEntry(t, "902", "Z4"),
		},
		Period: period,
	}
}

func zoneChart(t testing.TB) domain.ReferenceCatalogueRegistration {
	t.Helper()
	registration, err := domain.NewReferenceCatalogueRegistration(zoneChartSpec(t))
	if err != nil {
		t.Fatalf("构造分区目录登记：%v", err)
	}
	return registration
}

// Covers: ADR-0109 Decision 二「按计价基准时点选版」与 Decision 四「查不到即待判断，不给默认」——
// 区间内按声明粒度截目的邮编查表；查不到、始发不在该表覆盖内、邮编短于粒度，都只答「查过没查到」；
// 区间外这一版根本不适用。
func TestCatalogueResolvesTheDestinationPrefixWithinItsPeriod(t *testing.T) {
	registration := zoneChart(t)
	asOf := catalogueYearStart.Add(48 * time.Hour)

	reading, applicable := registration.ResolveAt(asOf, postalRoute(t, "94016", "90210"))
	if !applicable {
		t.Fatal("区间内的时点没选到这一版")
	}
	value, resolved := reading.Value()
	if !resolved || value != "Z4" || reading.Kind() != domain.CatalogueKindZone || reading.Reference().Version() != "v1" {
		t.Fatalf("解出的读数变形：%q resolved=%v %s@%s", value, resolved, reading.Reference().ID(), reading.Reference().Version())
	}

	missing, applicable := registration.ResolveAt(asOf, postalRoute(t, "94016", "33101"))
	if !applicable {
		t.Fatal("查不到邮编不该让这一版变得不适用")
	}
	if _, resolved := missing.Value(); resolved {
		t.Fatal("表里没有的邮编解出了一个分区")
	}
	if missing.Reference().Version() != "v1" {
		t.Fatal("查过没查到的读数丢了它查过的版本")
	}

	foreignOrigin, _ := registration.ResolveAt(asOf, postalRoute(t, "10001", "90210"))
	if _, resolved := foreignOrigin.Value(); resolved {
		t.Fatal("始发不在该表覆盖内却解出了分区")
	}
	tooShort, _ := registration.ResolveAt(asOf, postalRoute(t, "94016", "90"))
	if _, resolved := tooShort.Value(); resolved {
		t.Fatal("短于声明粒度的邮编解出了分区")
	}

	if _, applicable := registration.ResolveAt(catalogueYearEnd, postalRoute(t, "94016", "90210")); applicable {
		t.Fatal("区间右开：终点时点不该选到这一版")
	}
	if _, applicable := registration.ResolveAt(catalogueYearStart.Add(-time.Hour), postalRoute(t, "94016", "90210")); applicable {
		t.Fatal("区间起点之前不该选到这一版")
	}
}

// Covers: CONTEXT「计价参考目录……偏远档位表（目的邮编 → 档位）」——不区分始发的目录对任何始发都作答，
// 没给始发邮编也作答。
func TestOriginIndependentCatalogueAnswersWithoutAnOrigin(t *testing.T) {
	spec := zoneChartSpec(t)
	spec.Kind = domain.CatalogueKindRemoteTier
	spec.Reference = versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-DAS", "v1")
	spec.Origin = domain.NewIndependentCatalogueOrigin()
	spec.Entries = []domain.CatalogueEntry{catalogueEntry(t, "995", "REMOTE"), catalogueEntry(t, "902", "DAS")}
	registration, err := domain.NewReferenceCatalogueRegistration(spec)
	if err != nil {
		t.Fatalf("不区分始发的目录立不住：%v", err)
	}
	reading, _ := registration.ResolveAt(catalogueYearStart.Add(time.Hour), postalRoute(t, "", "99501"))
	if value, resolved := reading.Value(); !resolved || value != "REMOTE" || reading.Kind() != domain.CatalogueKindRemoteTier {
		t.Fatalf("档位读数变形：%q %v", value, resolved)
	}
}

// Covers: ADR-0109 Decision 二「来源标识……始发维度、目的邮编前缀 → 值的映射、生效区间」四件缺一不立；
// 前缀粒度是该版声明的，条目长度与它不符、前缀重复、值为空都拦在构造期。
func TestCatalogueRegistrationRejectsAnIllFormedTable(t *testing.T) {
	cases := map[string]func(spec *domain.ReferenceCatalogueRegistrationSpec){
		"缺来源标识":  func(spec *domain.ReferenceCatalogueRegistrationSpec) { spec.SourceIdentifier = " " },
		"缺登记责任方": func(spec *domain.ReferenceCatalogueRegistrationSpec) { spec.Registrant = "" },
		"没有条目":   func(spec *domain.ReferenceCatalogueRegistrationSpec) { spec.Entries = nil },
		"条目长度与粒度不符": func(spec *domain.ReferenceCatalogueRegistrationSpec) {
			spec.Entries = append(spec.Entries, catalogueEntry(t, "9021", "Z5"))
		},
		"前缀重复": func(spec *domain.ReferenceCatalogueRegistrationSpec) {
			spec.Entries = append(spec.Entries, catalogueEntry(t, "902", "Z5"))
		},
		"粒度为零": func(spec *domain.ReferenceCatalogueRegistrationSpec) { spec.PrefixLength = 0 },
		"引用种类不是目录": func(spec *domain.ReferenceCatalogueRegistrationSpec) {
			spec.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-CAT-ZONE-CHART", "v1")
		},
		"更正只带回指": func(spec *domain.ReferenceCatalogueRegistrationSpec) {
			spec.PriorVersion = versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", "v0")
		},
		"始发维度未声明": func(spec *domain.ReferenceCatalogueRegistrationSpec) { spec.Origin = domain.CatalogueOrigin{} },
	}
	for name, mutate := range cases {
		spec := zoneChartSpec(t)
		mutate(&spec)
		if _, err := domain.NewReferenceCatalogueRegistration(spec); !errors.Is(err, domain.ErrInvalidReferenceCatalogueRegistration) {
			t.Errorf("%s：期望拒绝，得到 %v", name, err)
		}
	}
	if _, err := domain.NewCatalogueEntry("902", ""); !errors.Is(err, domain.ErrInvalidReferenceCatalogueRegistration) {
		t.Fatalf("空取值的条目被接受：%v", err)
	}
	if _, err := domain.NewPostalPrefixCatalogueOrigin(nil); !errors.Is(err, domain.ErrInvalidReferenceCatalogueRegistration) {
		t.Fatalf("空的始发前缀集被接受：%v", err)
	}
}

// Covers: 更正关系成对且不换身份（与序列登记同一条 CONTEXT 生命周期）：新版本回指原版本并带依据，原版本
// 一字不动。
func TestCatalogueCorrectionPointsBackAtThePriorVersion(t *testing.T) {
	spec := zoneChartSpec(t)
	spec.Reference = versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", "v2")
	spec.PriorVersion = versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", "v1")
	spec.CorrectionBasis = "SYN-CORRECTION/zone-chart-902-retyped"
	registration, err := domain.NewReferenceCatalogueRegistration(spec)
	if err != nil {
		t.Fatalf("更正版本立不住：%v", err)
	}
	prior, basis, corrected := registration.Correction()
	if !corrected || prior.Version() != "v1" || basis != "SYN-CORRECTION/zone-chart-902-retyped" {
		t.Fatalf("更正关系变形：%v %q %v", prior, basis, corrected)
	}
	spec.PriorVersion = versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-OTHER", "v1")
	if _, err := domain.NewReferenceCatalogueRegistration(spec); !errors.Is(err, domain.ErrInvalidReferenceCatalogueRegistration) {
		t.Fatalf("回指另一本目录的更正被接受：%v", err)
	}
}

// Covers: 快照往返与摘要自校（与序列登记同一条 ADR-0014 纪律，目录自持 PRC 形状号）：折装再重建得到同一版；
// 快照里的条目被改过在重建门上暴露；带指纹与不带指纹的引用摘要相同（ADR-0108 Decision 二）。
func TestCatalogueSnapshotRoundTripsAndSelfChecks(t *testing.T) {
	registration := zoneChart(t)
	raw, err := domain.MarshalReferenceCatalogueRegistration(registration)
	if err != nil {
		t.Fatalf("折装：%v", err)
	}
	restored, err := domain.RehydrateReferenceCatalogueRegistration(raw)
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if restored.ContentDigest() != registration.ContentDigest() || restored.Canonicalization() != registration.Canonicalization() {
		t.Fatal("重建后的登记与原登记摘要或形状号不一致")
	}
	if len(restored.Entries()) != 3 || restored.PrefixLength() != 3 {
		t.Fatalf("条目或粒度丢失：%d %d", len(restored.Entries()), restored.PrefixLength())
	}
	reference, err := domain.PeekReferenceCatalogueRegistrationReference(raw)
	if err != nil || reference.ID() != "SYN-CAT-ZONE-CHART" || reference.Version() != "v1" {
		t.Fatalf("只读引用失败：%v %v", reference, err)
	}

	tampered := []byte(string(raw))
	tampered = []byte(replaceOnce(string(tampered), `"Z4"`, `"Z5"`))
	if _, err := domain.RehydrateReferenceCatalogueRegistration(tampered); !errors.Is(err, domain.ErrReferenceCatalogueRegistrationSnapshotInvalid) {
		t.Fatalf("被改过的快照没在重建门上暴露：%v", err)
	}

	spec := zoneChartSpec(t)
	spec.Reference = fingerprintedReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", "v1", "sha256:declared")
	fingerprinted, err := domain.NewReferenceCatalogueRegistration(spec)
	if err != nil {
		t.Fatalf("带指纹的登记：%v", err)
	}
	if fingerprinted.ContentDigest() != registration.ContentDigest() {
		t.Fatal("声明时附带的指纹进了内容摘要")
	}
}

// Covers: ADR-0109 Decision 二「复核与在用的门照 ADR-0099 给序列立的那一套」——复核责任方不得是登记责任方；
// 在用版本是该时刻之前复核通过的最新版本，退回不进在用，没有候选不给答案。
func TestCatalogueReviewAndInForceSelectionFollowTheSeriesRules(t *testing.T) {
	registration := zoneChart(t)
	reviewedAt := catalogueYearStart.Add(24 * time.Hour)
	if _, err := domain.NewCatalogueReview(registration, "SYN-CAT-REGISTRAR", reviewedAt, domain.SeriesReviewApproved, "SYN-REVIEW/self"); !errors.Is(err, domain.ErrCatalogueReviewerIsRegistrant) {
		t.Fatalf("登记责任方自己复核被接受：%v", err)
	}
	review, err := domain.NewCatalogueReview(registration, "SYN-CAT-REVIEWER", reviewedAt, domain.SeriesReviewApproved, "SYN-REVIEW/zone-chart-v1")
	if err != nil {
		t.Fatalf("复核立不住：%v", err)
	}
	if review.Reference().ID() != "SYN-CAT-ZONE-CHART" || review.Decision() != domain.SeriesReviewApproved {
		t.Fatalf("复核记录变形：%#v", review)
	}

	v1 := reviewedCatalogue(t, "v1", catalogueYearStart, reviewedAt, domain.SeriesReviewApproved)
	v2 := reviewedCatalogue(t, "v2", catalogueYearStart.Add(time.Hour), reviewedAt.Add(time.Hour), domain.SeriesReviewReturned)
	v3 := reviewedCatalogue(t, "v3", catalogueYearStart.Add(2*time.Hour), reviewedAt.Add(2*time.Hour), domain.SeriesReviewApproved)

	inForce, found := domain.SelectInForceCatalogueVersion([]domain.ReviewedCatalogueVersion{v1, v2, v3}, reviewedAt.Add(90*time.Minute))
	if !found || inForce.Version() != "v1" {
		t.Fatalf("v3 通过之前在用应是 v1：%v %v", inForce, found)
	}
	inForce, found = domain.SelectInForceCatalogueVersion([]domain.ReviewedCatalogueVersion{v1, v2, v3}, reviewedAt.Add(3*time.Hour))
	if !found || inForce.Version() != "v3" {
		t.Fatalf("v3 通过之后在用应是 v3：%v %v", inForce, found)
	}
	if _, found := domain.SelectInForceCatalogueVersion([]domain.ReviewedCatalogueVersion{v2}, reviewedAt.Add(3*time.Hour)); found {
		t.Fatal("只有退回的版本却选出了在用")
	}
}

func reviewedCatalogue(t testing.TB, version string, registeredAt, reviewedAt time.Time, decision domain.SeriesReviewDecision) domain.ReviewedCatalogueVersion {
	t.Helper()
	candidate, err := domain.NewReviewedCatalogueVersion(
		versionReference(t, domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CHART", version), registeredAt, reviewedAt, decision)
	if err != nil {
		t.Fatalf("候选 %s：%v", version, err)
	}
	return candidate
}

func replaceOnce(text, old, replacement string) string {
	for index := 0; index+len(old) <= len(text); index++ {
		if text[index:index+len(old)] == old {
			return text[:index] + replacement + text[index+len(old):]
		}
	}
	return text
}
