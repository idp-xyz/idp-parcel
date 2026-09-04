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
// 解出的登记对象与直接走领域构造器造出的逐格相同；三处版本引用的 digest 槽装的是**声明令牌**
// （`declared:` 前缀，MCP-3 裁决），册上已有的引用照实回指不重铸；同一份载荷解成预览命令与
// 登记命令得到同一个登记对象——摘要与三个引用逐字节相同，这是决定四那句硬句在解码这一层的
// 落点；结构不对、领域构造门拒、多出的键，一律 ErrMalformedRequest。

const seriesPayloadJSON = `{
  "seriesId": "SYN-PRC-FUEL-WEEKLY",
  "seriesVersion": "v2",
  "kind": "FUEL_RATE",
  "sourceIdentifier": "SYN-CARRIER/fuel-weekly-bulletin",
  "periods": [
    {"startsAt": "2026-08-03T00:00:00Z", "endsAt": "2026-08-10T00:00:00Z", "value": "0.23", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W32"},
    {"startsAt": "2026-08-10T00:00:00Z", "value": "0.240"}
  ],
  "correction": {"priorVersion": "v1", "priorReferenceDigest": "sha256:syn-SYN-PRC-FUEL-WEEKLY-v1", "basis": "SYN-CORRECTION/fuel-w32-transcription"},
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

// TestSeriesPayloadMintsDeclaredTokensOnlyForNewReferences 证 digest 槽的规则（MCP-3 裁决四条之
// 一、三）：新铸的引用装 `declared:` 令牌，形如 declared:<kind>/<id>@<version>；册上已有而载荷
// 带来的引用 digest 照实回指不重铸；口径未带 digest 时同样铸令牌（PC 读口今天不透 digest）。
func TestSeriesPayloadMintsDeclaredTokensOnlyForNewReferences(t *testing.T) {
	registration, err := decodeSeriesPayload(t, seriesPayloadJSON).Registration(payloadTenant(t), "SYN-PRC-SERIES-REGISTRAR")
	if err != nil {
		t.Fatalf("翻成登记：%v", err)
	}
	if got := registration.Reference().Digest(); got != "declared:reference-series/SYN-PRC-FUEL-WEEKLY@v2" {
		t.Fatalf("自身引用 digest = %q", got)
	}
	if got := pricinghttp.DeclaredReferenceToken(domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v2"); got != registration.Reference().Digest() {
		t.Fatalf("令牌函数与解码器不是同一条规则：%q", got)
	}
	prior, _, _ := registration.Correction()
	if prior.Digest() != "sha256:syn-SYN-PRC-FUEL-WEEKLY-v1" {
		t.Fatalf("册上已有的回指被重铸了：%q", prior.Digest())
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
	if basis.Digest() != "declared:commercial-policy/SYN-PRC-FX-POLICY@v3" {
		t.Fatalf("口径未带 digest 时应铸令牌：%q", basis.Digest())
	}
	prior, _, _ = fx.Correction()
	if prior.Digest() != "declared:reference-series/SYN-PRC-USD-CNY@v0" {
		t.Fatalf("回指未带 digest 时应铸令牌：%q", prior.Digest())
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
	if basis, _ := fxWithDigest.QuoteBasis(); basis.Digest() != "sha256:pc-policy-v3" {
		t.Fatalf("载荷带来的口径 digest 被重铸了：%q", basis.Digest())
	}
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
