package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉 ADR-0108：版本引用的身份是（种类，标识，版本）三元，指纹是声明时附带的可选痕迹
// ——相等、去重、排序与摘要一律不看它。

// Covers: ADR-0108 Decision 一——相等、清单去重、排序三处同一套判据，只看三元。
func TestVersionReferenceIdentityIgnoresTheFingerprint(t *testing.T) {
	bare := versionReference(t, domain.ArtifactRateTable, "table-a", "v1")
	printed := fingerprintedReference(t, domain.ArtifactRateTable, "table-a", "v1", "sha256:abc")

	if !bare.SameIdentity(printed) || !printed.SameIdentity(bare) {
		t.Fatal("同三元只差指纹的两条引用必须同身份")
	}
	if bare.HasFingerprint() || !printed.HasFingerprint() || printed.Fingerprint() != "sha256:abc" {
		t.Fatalf("指纹在场性读错：bare=%v printed=%q", bare.HasFingerprint(), printed.Fingerprint())
	}

	if _, err := domain.NewVersionManifest([]domain.VersionReference{bare, printed}); !errors.Is(err, domain.ErrDuplicateVersionReference) {
		t.Fatalf("清单去重要按三元判重复，实得 %v", err)
	}

	other := versionReference(t, domain.ArtifactWeightPolicy, "weight-a", "v1")
	withPrinted, err := domain.NewVersionManifest([]domain.VersionReference{other, printed})
	if err != nil {
		t.Fatalf("清单：%v", err)
	}
	withBare, err := domain.NewVersionManifest([]domain.VersionReference{bare, other})
	if err != nil {
		t.Fatalf("清单：%v", err)
	}
	if !withPrinted.Equal(withBare) {
		t.Fatal("两份清单只差一枚指纹，必须相等——重放时组合出的清单不该因指纹在不在场而对不上")
	}
	if got := withPrinted.References(); got[0].Kind() != domain.ArtifactRateTable || got[1].Kind() != domain.ArtifactWeightPolicy {
		t.Fatalf("排序 = [%s %s]，想要按三元排", got[0].Kind(), got[1].Kind())
	}
}

// Covers: ADR-0108 Decision 三——三元一格不许空、不许带首尾空白；指纹可空，带值时不许首尾空白。
// 三步法留下的旧签名 NewVersionReference 第四参如今允许为空。
func TestVersionReferenceConstructionGuardsTheTripleAndTheFingerprintWhitespace(t *testing.T) {
	if _, err := domain.NewVersionReferenceIdentity(domain.ArtifactRateTable, "", "v1"); !errors.Is(err, domain.ErrInvalidVersionReference) {
		t.Fatalf("空标识应拒，实得 %v", err)
	}
	if _, err := domain.NewVersionReferenceIdentity(domain.ArtifactRateTable, "table-a", " v1"); !errors.Is(err, domain.ErrInvalidVersionReference) {
		t.Fatalf("带空白的版本应拒，实得 %v", err)
	}
	if _, err := domain.NewVersionReferenceIdentity("", "table-a", "v1"); !errors.Is(err, domain.ErrInvalidVersionReference) {
		t.Fatalf("空种类应拒，实得 %v", err)
	}
	if _, err := domain.NewVersionReferenceWithFingerprint(domain.ArtifactRateTable, "table-a", "v1", " sha256:abc"); !errors.Is(err, domain.ErrInvalidVersionReference) {
		t.Fatalf("带空白的指纹应拒，实得 %v", err)
	}
	legacy, err := domain.NewVersionReference(domain.ArtifactRateTable, "table-a", "v1", "")
	if err != nil || legacy.HasFingerprint() {
		t.Fatalf("旧签名第四参为空应构造出不带指纹的引用，实得 %v / %v", legacy, err)
	}
}

// Covers: ADR-0108 Decision 二在序列登记上的形状——自身引用、口径与回指上的指纹不进内容摘要：
// 同一份登记带不带回指指纹，ContentDigest 逐字相同；指纹仍随快照往返。
func TestSeriesRegistrationContentDigestIgnoresReferenceFingerprints(t *testing.T) {
	bare := fuelSeriesSpec(t)
	bare.Reference = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2")
	bare.PriorVersion = versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1")
	bare.CorrectionBasis = "SYN-CORRECTION/fuel-w32-transcription"

	printed := bare
	printed.PriorVersion = fingerprintedReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1", "sha256:prior-content")

	bareRegistration, err := domain.NewReferenceSeriesRegistration(bare)
	if err != nil {
		t.Fatalf("不带指纹的登记：%v", err)
	}
	printedRegistration, err := domain.NewReferenceSeriesRegistration(printed)
	if err != nil {
		t.Fatalf("带指纹的登记：%v", err)
	}
	if bareRegistration.ContentDigest() != printedRegistration.ContentDigest() {
		t.Fatal("回指带不带指纹，序列登记的内容摘要必须相同")
	}

	raw, err := domain.MarshalReferenceSeriesRegistration(printedRegistration)
	if err != nil {
		t.Fatalf("折装：%v", err)
	}
	rebuilt, err := domain.RehydrateReferenceSeriesRegistration(raw)
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	prior, _, corrected := rebuilt.Correction()
	if !corrected || prior.Fingerprint() != "sha256:prior-content" {
		t.Fatalf("回指的指纹没随快照往返：%v corrected=%v", prior, corrected)
	}
	if rebuilt.Reference().HasFingerprint() {
		t.Fatalf("自身引用没声明指纹，读回却带了 %q", rebuilt.Reference().Fingerprint())
	}
}
