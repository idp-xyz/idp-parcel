package application_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证登记前预览用例（票 pricing-reference-series-operations/08 件②，ADR-0101 决定四）：
// 预览交回的证据等级、规范化版本与内容摘要都取自领域登记对象本身——与登记册写下的是同一个
// 方法算出来的，不存在第二条摘要路径；逐期差异按对照版本从版本读口取回后交领域比对；预览
// **不碰任何写口**（依赖里根本没有登记册）。对照版本三种「没有」分格：没要求比、要求了但
// 不在册、在册但不是同一条序列。

func previewBase(t *testing.T) domain.ReferenceSeriesRegistration {
	t.Helper()
	return fuelRegistrationForReview(t)
}

// previewProposal 造一版同序列的拟登版本：首期改取值，并按 correction 决定是否声明更正关系。
func previewProposal(t *testing.T, version string, correction bool) domain.ReferenceSeriesRegistration {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		mustValue(t, domain.ParseDecimal, "0.23"), "")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, "SYN-FUEL", version, "sha256:syn-fuel-"+version)
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	spec := domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/fuel",
		Registrant:       "SYN-REGISTRAR",
		Periods:          []domain.SeriesPeriodValue{period},
	}
	if correction {
		spec.PriorVersion = previewBase(t).Reference()
		spec.CorrectionBasis = "SYN-CORRECTION/fuel-w32-transcription"
	}
	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

func newPreviewHandler(loader *versionLoaderDouble) *application.PreviewReferenceSeriesHandler {
	return application.NewPreviewReferenceSeriesHandler(application.PreviewReferenceSeriesDeps{Versions: loader})
}

// TestPreviewAnswersGradeAndDigestFromTheDomainObject 证预览的三格取自领域对象自身：摘要与
// 登记册 Register 写下的那个是同一个方法（ContentDigest）的返回值，等级同理；没要求对照时不问
// 版本读口。
func TestPreviewAnswersGradeAndDigestFromTheDomainObject(t *testing.T) {
	loader := &versionLoaderDouble{err: errors.New("不该被调到")}
	proposal := previewProposal(t, "v1", false)

	preview, err := newPreviewHandler(loader).Handle(t.Context(), application.PreviewReferenceSeriesCommand{Registration: proposal})
	if err != nil {
		t.Fatalf("预览报错：%v", err)
	}
	if preview.Outcome != application.ReferenceSeriesPreviewed || preview.Outcome.String() != "PREVIEWED" {
		t.Fatalf("outcome = %s", preview.Outcome)
	}
	if preview.ContentDigest != proposal.ContentDigest() || preview.Canonicalization != proposal.Canonicalization() {
		t.Fatalf("摘要或规范化版本不是领域对象自己算的：%q/%q", preview.ContentDigest, preview.Canonicalization)
	}
	if preview.EvidenceGrade != domain.SeriesEvidenceAsserted {
		t.Fatalf("缺凭证的拟登版本等级 = %s，想要 ASSERTED", preview.EvidenceGrade)
	}
	if preview.Comparison != application.SeriesComparisonNotRequested || preview.Comparison.String() != "NOT_REQUESTED" {
		t.Fatalf("没要求对照却答了 %s", preview.Comparison)
	}
	if preview.BaseVersion != "" || len(preview.Changes) != 0 {
		t.Fatalf("没要求对照却带了对照结果：%q %v", preview.BaseVersion, preview.Changes)
	}
}

// TestPreviewComparesACorrectionWithItsPriorVersionByDefault 证更正版本不指名对照时默认对它
// 声明更正的那一版：取回、交领域逐期比对，差异随预览交回。
func TestPreviewComparesACorrectionWithItsPriorVersionByDefault(t *testing.T) {
	base := previewBase(t)
	loader := &versionLoaderDouble{registrations: map[string]domain.ReferenceSeriesRegistration{"SYN-FUEL@v1": base}}
	proposal := previewProposal(t, "v2", true)

	preview, err := newPreviewHandler(loader).Handle(t.Context(), application.PreviewReferenceSeriesCommand{Registration: proposal})
	if err != nil {
		t.Fatalf("预览报错：%v", err)
	}
	if preview.Comparison != application.SeriesComparisonCompared || preview.BaseVersion != "v1" {
		t.Fatalf("对照 = %s/%q，想要 COMPARED/v1", preview.Comparison, preview.BaseVersion)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].Kind() != domain.SeriesPeriodChanged ||
		!preview.Changes[0].ValueChanged() || !preview.Changes[0].EvidenceChanged() {
		t.Fatalf("逐期差异变形：%+v", preview.Changes)
	}
	if preview.EvidenceGrade != domain.SeriesEvidenceAsserted {
		t.Fatalf("等级 = %s", preview.EvidenceGrade)
	}
}

// TestPreviewHonoursAnExplicitComparisonVersion 证指名的对照版本优先于更正回指，且非更正版本
// 也能指名对照——延展版本要看与上一版差在哪，而它没有更正关系可回指。
func TestPreviewHonoursAnExplicitComparisonVersion(t *testing.T) {
	base := previewBase(t)
	loader := &versionLoaderDouble{registrations: map[string]domain.ReferenceSeriesRegistration{"SYN-FUEL@v1": base}}
	proposal := previewProposal(t, "v3", false)

	preview, err := newPreviewHandler(loader).Handle(t.Context(), application.PreviewReferenceSeriesCommand{
		Registration:       proposal,
		CompareWithVersion: "v1",
	})
	if err != nil {
		t.Fatalf("预览报错：%v", err)
	}
	if preview.Comparison != application.SeriesComparisonCompared || preview.BaseVersion != "v1" || len(preview.Changes) != 1 {
		t.Fatalf("指名对照没生效：%s/%q/%d", preview.Comparison, preview.BaseVersion, len(preview.Changes))
	}
}

// TestPreviewTellsAMissingBaseFromAnIncomparableOne 证对照版本不在册与在册但不是同一条序列分格：
// 前者续办是查版本号，后者续办是查绑错了哪条序列；两者都不是预览失败。
func TestPreviewTellsAMissingBaseFromAnIncomparableOne(t *testing.T) {
	base := previewBase(t)
	loader := &versionLoaderDouble{registrations: map[string]domain.ReferenceSeriesRegistration{"SYN-FUEL@v1": base}}
	handler := newPreviewHandler(loader)

	missing, err := handler.Handle(t.Context(), application.PreviewReferenceSeriesCommand{
		Registration:       previewProposal(t, "v2", false),
		CompareWithVersion: "v0",
	})
	if err != nil {
		t.Fatalf("对照不在册报错：%v", err)
	}
	if missing.Outcome != application.ReferenceSeriesPreviewed || missing.Comparison != application.SeriesComparisonBaseUnknown ||
		missing.BaseVersion != "v0" || len(missing.Changes) != 0 {
		t.Fatalf("对照不在册答 %s/%s/%q/%d", missing.Outcome, missing.Comparison, missing.BaseVersion, len(missing.Changes))
	}
	if missing.Comparison.String() != "BASE_UNKNOWN" {
		t.Fatalf("对照结果原名 = %q", missing.Comparison.String())
	}

	// 对照那一版是另一种种类：版本读口按（序列、版本）取回，种类不合只有领域比对能看出来。
	fxSpec := domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        mustReference(t, domain.ArtifactReferenceSeries, "SYN-FUEL", "v1"),
		SourceIdentifier: "SYN-FINANCE/usd-cny",
		Registrant:       "SYN-REGISTRAR",
		QuoteBasis:       mustReference(t, domain.ArtifactCommercialPolicy, "SYN-FX-POLICY", "v1"),
		Periods:          base.Periods(),
	}
	fx, err := domain.NewReferenceSeriesRegistration(fxSpec)
	if err != nil {
		t.Fatalf("构造异种对照：%v", err)
	}
	loader.registrations["SYN-FUEL@v1"] = fx
	incomparable, err := handler.Handle(t.Context(), application.PreviewReferenceSeriesCommand{
		Registration:       previewProposal(t, "v2", false),
		CompareWithVersion: "v1",
	})
	if err != nil {
		t.Fatalf("异种对照报错：%v", err)
	}
	if incomparable.Comparison != application.SeriesComparisonBaseIncomparable || incomparable.Comparison.String() != "BASE_INCOMPARABLE" {
		t.Fatalf("异种对照答 %s", incomparable.Comparison)
	}
}

// TestPreviewRefusesAZeroRegistrationAndSurfacesLoaderFailures 证零值登记不受理（连租户都指不出
// 来），版本读口故障答未决并带出错误——预览是治理动作的前一步，操作者要拿到原因。
func TestPreviewRefusesAZeroRegistrationAndSurfacesLoaderFailures(t *testing.T) {
	rejected, err := newPreviewHandler(&versionLoaderDouble{}).Handle(t.Context(), application.PreviewReferenceSeriesCommand{})
	if err != nil {
		t.Fatalf("零值登记报错：%v", err)
	}
	if rejected.Outcome != application.ReferenceSeriesPreviewNotAccepted || rejected.Outcome.String() != "NOT_ACCEPTED" {
		t.Fatalf("零值登记答 %s", rejected.Outcome)
	}

	failing := &versionLoaderDouble{err: errors.New("库连不上")}
	undecided, err := newPreviewHandler(failing).Handle(t.Context(), application.PreviewReferenceSeriesCommand{
		Registration:       previewProposal(t, "v2", false),
		CompareWithVersion: "v1",
	})
	if err == nil {
		t.Fatal("读口故障被吞了")
	}
	if undecided.Outcome != application.ReferenceSeriesPreviewUndecided || undecided.Outcome.String() != "UNDECIDED" {
		t.Fatalf("读口故障答 %s", undecided.Outcome)
	}
}

func mustReference(t *testing.T, kind domain.ArtifactKind, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReference(kind, id, version, "sha256:syn-"+id+"-"+version)
	if err != nil {
		t.Fatalf("reference %s@%s: %v", id, version, err)
	}
	return reference
}
