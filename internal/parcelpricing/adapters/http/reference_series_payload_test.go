package pricinghttp_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证运营操作者面的序列登记载荷解码（票 pricing-reference-series-operations/08；ADR-0101
// 决定一、四）：载荷只有内容没有身份（租户与登记责任方由调用方——将来是操作者信封——给），
// 解出的登记对象与直接走领域构造器造出的逐格相同；三处版本引用只带三元，指纹按来源在场或缺席
// （ADR-0108 Decision 五），本包不铸任何令牌；同一份载荷解成预览命令与登记命令得到同一个登记
// 对象——摘要与三个引用逐字节相同，这是决定四那句硬句在解码这一层的落点；结构不对、领域构造门
// 拒、多出的键，一律 ErrMalformedRequest。

const seriesPayloadJSON = `{
  "seriesId": "SYN-PRC-FUEL-WEEKLY",
  "seriesVersion": "v2",
  "kind": "FUEL_RATE",
  "sourceIdentifier": "SYN-CARRIER/fuel-weekly-bulletin",
  "periods": [
    {"startsAt": "2026-08-03T00:00:00Z", "endsAt": "2026-08-10T00:00:00Z", "value": "0.23", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W32"},
    {"startsAt": "2026-08-10T00:00:00Z", "value": "0.240"}
  ],
  "correction": {"priorVersion": "v1", "priorFingerprint": "sha256:prior-content-v1", "basis": "SYN-CORRECTION/fuel-w32-transcription"},
  "compareWithVersion": "v1"
}`

func decodeSeriesPayload(t *testing.T, raw string) pricinghttp.ReferenceSeriesRegistrationPayload {
	t.Helper()
	payload, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("解码载荷：%v", err)
	}
	return payload
}

func payloadTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return tenant
}

// TestSeriesPayloadTranslatesIntoTheDomainRegistration 证载荷逐格进领域构造器：期次起止与取值
// （规范化后 0.240 → 0.24）、凭证可缺、更正两件、身份来自入参而不是载荷。
func TestSeriesPayloadTranslatesIntoTheDomainRegistration(t *testing.T) {
	payload := decodeSeriesPayload(t, seriesPayloadJSON)
	registration, err := payload.Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成登记：%v", err)
	}

	if registration.Tenant().String() != "tenant-1" || registration.Registrant() != "SYN-PRC-SERIES-REGISTRAR" {
		t.Fatalf("身份没取自入参：%s/%s", registration.Tenant(), registration.Registrant())
	}
	if registration.Kind() != domain.ReferenceSeriesFuelRate || registration.SourceIdentifier() != "SYN-CARRIER/fuel-weekly-bulletin" {
		t.Fatalf("种类或来源变形：%s/%s", registration.Kind(), registration.SourceIdentifier())
	}
	if registration.Reference().ID() != "SYN-PRC-FUEL-WEEKLY" || registration.Reference().Version() != "v2" {
		t.Fatalf("版本引用变形：%v", registration.Reference())
	}
	periods := registration.Periods()
	if len(periods) != 2 {
		t.Fatalf("期次数 = %d", len(periods))
	}
	if !periods[0].StartsAt().Equal(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)) || periods[0].Value().String() != "0.23" {
		t.Fatalf("首期变形：%v %s", periods[0].StartsAt(), periods[0].Value())
	}
	if endsAt, bounded := periods[0].EndsAt(); !bounded || !endsAt.Equal(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("首期止点变形：%v/%v", endsAt, bounded)
	}
	if evidence, ok := periods[0].Evidence(); !ok || evidence != "SYN-EVIDENCE/fuel-2026-W32" {
		t.Fatalf("首期凭证变形：%q/%v", evidence, ok)
	}
	if _, bounded := periods[1].EndsAt(); bounded {
		t.Fatal("末期无上界却有了止点")
	}
	if periods[1].Value().String() != "0.24" || periods[1].Verifiable() {
		t.Fatalf("末期变形：%s verifiable=%v", periods[1].Value(), periods[1].Verifiable())
	}
	prior, basis, corrected := registration.Correction()
	if !corrected || prior.Version() != "v1" || basis != "SYN-CORRECTION/fuel-w32-transcription" {
		t.Fatalf("更正关系变形：%v %q %v", prior, basis, corrected)
	}
	if _, declared := registration.QuoteBasis(); declared {
		t.Fatal("燃油载荷长出了口径")
	}
}

// TestSeriesPayloadCarriesFingerprintsOnlyWhereTheOperatorHasOne 证三处引用的指纹规则（ADR-0108
// Decision 五）：自身引用只带三元、指纹留空；回指带前版内容摘要作指纹，载荷没带就留空；口径未带
// digest 时留空（PC 读口今天不透 content_digest），带来就照实放进指纹。本包不再铸任何令牌——
// 两条路（预览 / 登记）产出同一份引用，靠的是不铸而不是同一条铸法。
func TestSeriesPayloadCarriesFingerprintsOnlyWhereTheOperatorHasOne(t *testing.T) {
	registration, err := decodeSeriesPayload(t, seriesPayloadJSON).Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成登记：%v", err)
	}
	if registration.Reference().HasFingerprint() {
		t.Fatalf("自身引用不该带指纹：%q", registration.Reference().Fingerprint())
	}
	prior, _, _ := registration.Correction()
	if prior.Fingerprint() != "sha256:prior-content-v1" {
		t.Fatalf("回指的指纹应照实取载荷带来的前版内容摘要：%q", prior.Fingerprint())
	}

	fxPayload := decodeSeriesPayload(t, `{
	  "seriesId": "SYN-PRC-USD-CNY", "seriesVersion": "v1", "kind": "EXCHANGE_RATE",
	  "sourceIdentifier": "SYN-FINANCE/usd-cny-daily",
	  "quoteBasis": {"policyId": "SYN-PRC-FX-POLICY", "policyVersion": "v3"},
	  "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "7.2", "evidenceRef": "SYN-EVIDENCE/fx-2026-W32"}],
	  "correction": {"priorVersion": "v0", "basis": "SYN-CORRECTION/fx"}
	}`)
	fx, err := fxPayload.Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成汇率登记：%v", err)
	}
	basis, declared := fx.QuoteBasis()
	if !declared || basis.ID() != "SYN-PRC-FX-POLICY" || basis.Version() != "v3" {
		t.Fatalf("口径变形：%v/%v", basis, declared)
	}
	if basis.HasFingerprint() {
		t.Fatalf("口径未带 digest 时指纹应留空，实得 %q", basis.Fingerprint())
	}
	prior, _, _ = fx.Correction()
	if prior.HasFingerprint() || prior.Version() != "v0" {
		t.Fatalf("回指未带前版摘要时指纹应留空且三元照实：%v", prior)
	}

	withDigest := decodeSeriesPayload(t, `{
	  "seriesId": "SYN-PRC-USD-CNY", "seriesVersion": "v1", "kind": "EXCHANGE_RATE",
	  "sourceIdentifier": "SYN-FINANCE/usd-cny-daily",
	  "quoteBasis": {"policyId": "SYN-PRC-FX-POLICY", "policyVersion": "v3", "digest": "sha256:pc-policy-v3"},
	  "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "7.2"}]
	}`)
	fxWithDigest, err := withDigest.Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成带口径 digest 的登记：%v", err)
	}
	if basis, _ := fxWithDigest.QuoteBasis(); basis.Fingerprint() != "sha256:pc-policy-v3" {
		t.Fatalf("载荷带来的口径 digest 应照实进指纹：%q", basis.Fingerprint())
	}
	if fx.ContentDigest() == "" || fx.ContentDigest() != mustRegistration(t, fxPayload).ContentDigest() {
		t.Fatal("同一份载荷解两次摘要必须相同")
	}
}

func mustRegistration(t *testing.T, payload pricinghttp.ReferenceSeriesRegistrationPayload) domain.ReferenceSeriesRegistration {
	t.Helper()
	registration, err := payload.Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成登记：%v", err)
	}
	return registration
}

// TestSeriesPayloadYieldsOneRegistrationForPreviewAndRegistration 是 ADR-0101 决定四那句硬句在
// 解码层的钉：同一份载荷解成预览命令与登记命令，得到的登记对象摘要逐字节相同、三个引用逐格相同；
// 预览命令另带对照版本号，登记命令没有。
func TestSeriesPayloadYieldsOneRegistrationForPreviewAndRegistration(t *testing.T) {
	payload := decodeSeriesPayload(t, seriesPayloadJSON)
	tenant := payloadTenant(t)

	preview, err := payload.PreviewCommand(tenant, "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("预览命令：%v", err)
	}
	registration, err := payload.RegistrationCommand(tenant, "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("登记命令：%v", err)
	}

	left, right := preview.Registration, registration.Registration
	if left.ContentDigest() != right.ContentDigest() || left.Canonicalization() != right.Canonicalization() {
		t.Fatalf("同一份载荷两条路摘要不同：%q vs %q", left.ContentDigest(), right.ContentDigest())
	}
	if left.Reference() != right.Reference() {
		t.Fatalf("自身引用不同：%v vs %v", left.Reference(), right.Reference())
	}
	leftPrior, _, _ := left.Correction()
	rightPrior, _, _ := right.Correction()
	if leftPrior != rightPrior {
		t.Fatalf("更正回指不同：%v vs %v", leftPrior, rightPrior)
	}
	leftBasis, _ := left.QuoteBasis()
	rightBasis, _ := right.QuoteBasis()
	if leftBasis != rightBasis {
		t.Fatalf("口径引用不同：%v vs %v", leftBasis, rightBasis)
	}
	if preview.CompareWithVersion != "v1" {
		t.Fatalf("预览命令没带上对照版本：%q", preview.CompareWithVersion)
	}
}

// TestSeriesPayloadRejectsWhatItCannotTranslate 证不能翻的一律 ErrMalformedRequest：坏 JSON、
// 多出的键（含任何自报身份）、坏时刻、坏十进制、领域构造门拒（汇率无口径、期次重叠）。
func TestSeriesPayloadRejectsWhatItCannotTranslate(t *testing.T) {
	if _, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(`{"seriesId":`)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("坏 JSON：err = %v", err)
	}
	if _, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(`{"seriesId": "S", "registrant": "me"}`)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("自报身份键应被拒：err = %v", err)
	}
	if _, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(`{"seriesId": "S", "tenant": "t"}`)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("自报租户键应被拒：err = %v", err)
	}

	tenant := payloadTenant(t)
	cases := map[string]string{
		"坏时刻": `{"seriesId": "S", "seriesVersion": "v1", "kind": "FUEL_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03", "value": "0.2"}]}`,
		"坏十进制": `{"seriesId": "S", "seriesVersion": "v1", "kind": "FUEL_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "1e3"}]}`,
		"汇率无口径": `{"seriesId": "S", "seriesVersion": "v1", "kind": "EXCHANGE_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "7.2"}]}`,
		"期次重叠": `{"seriesId": "S", "seriesVersion": "v1", "kind": "FUEL_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03T00:00:00Z", "endsAt": "2026-08-17T00:00:00Z", "value": "0.2"},
		                       {"startsAt": "2026-08-10T00:00:00Z", "value": "0.3"}]}`,
		"缺版本号": `{"seriesId": "S", "kind": "FUEL_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "0.2"}]}`,
		"只带依据不带回指": `{"seriesId": "S", "seriesVersion": "v1", "kind": "FUEL_RATE", "sourceIdentifier": "src",
		           "periods": [{"startsAt": "2026-08-03T00:00:00Z", "value": "0.2"}], "correction": {"basis": "loose"}}`,
	}
	for name, raw := range cases {
		payload, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(raw))
		if err != nil {
			if !errors.Is(err, pricinghttp.ErrMalformedRequest) {
				t.Fatalf("%s：解码错误不是 ErrMalformedRequest：%v", name, err)
			}
			continue
		}
		if _, err := payload.Registration(tenant, "SYN-PRC-SERIES-REGISTRAR"); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}

	// 身份缺位也是 ErrMalformedRequest 的反面——它不是载荷的错，是调用方没给信封：单独一格。
	if _, err := decodeSeriesPayload(t, seriesPayloadJSON).Registration(domain.TenantID{}, "SYN-PRC-SERIES-REGISTRAR"); !errors.Is(err, pricinghttp.ErrOperatorIdentityMissing) {
		t.Fatalf("缺租户：err = %v", err)
	}
	if _, err := decodeSeriesPayload(t, seriesPayloadJSON).Registration(tenant, ""); !errors.Is(err, pricinghttp.ErrOperatorIdentityMissing) {
		t.Fatalf("缺登记责任方：err = %v", err)
	}
}
