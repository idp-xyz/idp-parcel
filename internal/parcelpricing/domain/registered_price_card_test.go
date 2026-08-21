package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证价卡登记聚合：登记必须携带源文件身份与 SHA-256（Source integrity gate 的
// 证据索引，真文件外置不入库）、BUY/SELL 方向授权引用（party-commercial 的授权工件，
// 这里只引用不解析）与发布批准责任方；登记快照折装重建同答，元数据被改在重建门上暴露。

const synSourceSHA = "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d"

func syntheticRegistration(t *testing.T) domain.PriceCardRegistration {
	t.Helper()
	source, err := domain.NewSourceFileIdentity("SYN-PRC-CARD-260820.xlsx", synSourceSHA)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	registration, err := domain.NewPriceCardRegistration(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		fullyDeclaredSyntheticPlan(t),
		source,
		versionReference(t, domain.ArtifactCommercialAuthorization, "SYN-PRC-BUY-GRANT", "v1"),
		"SYN-PRC-PRICING-GOVERNANCE",
	)
	if err != nil {
		t.Fatalf("构造价卡登记：%v", err)
	}
	return registration
}

// TestPriceCardRegistrationCarriesGovernanceReferences 证登记聚合把治理引用原样带
// 在身上：源文件身份、方向授权、批准责任方一个都不缺。
func TestPriceCardRegistrationCarriesGovernanceReferences(t *testing.T) {
	registration := syntheticRegistration(t)

	if registration.Tenant().String() != "tenant-1" {
		t.Fatalf("租户漂移：%s", registration.Tenant())
	}
	if registration.SourceFile().Name() != "SYN-PRC-CARD-260820.xlsx" ||
		registration.SourceFile().SHA256() != synSourceSHA {
		t.Fatalf("源文件身份漂移：%+v", registration.SourceFile())
	}
	if registration.DirectionAuthorization().Kind() != domain.ArtifactCommercialAuthorization {
		t.Fatalf("授权引用种类漂移：%s", registration.DirectionAuthorization().Kind())
	}
	if registration.PublicationApprover() != "SYN-PRC-PRICING-GOVERNANCE" {
		t.Fatalf("批准责任方漂移：%s", registration.PublicationApprover())
	}
	if registration.Plan().Direction() != domain.PricingDirectionBuy {
		t.Fatalf("方案方向漂移：%s", registration.Plan().Direction())
	}
}

// TestSourceFileIdentityRefusesWeakEvidence 证源文件身份只收放得进证据索引的形状：
// SHA-256 必须是 64 位小写十六进制，名称不得为空——身份断言弱于哈希匹配的情形必须
// 显式登记，不能靠一个格式错误的哈希混进来。
func TestSourceFileIdentityRefusesWeakEvidence(t *testing.T) {
	cases := map[string]struct{ name, sha string }{
		"空名称":      {"", synSourceSHA},
		"名称带边空白":   {" card.xlsx", synSourceSHA},
		"哈希太短":     {"card.xlsx", "9edaf27e"},
		"哈希混大写":    {"card.xlsx", strings.ToUpper(synSourceSHA)},
		"哈希混非十六进制": {"card.xlsx", strings.Repeat("z", 64)},
		"哈希为空":     {"card.xlsx", ""},
	}
	for label, tc := range cases {
		if _, err := domain.NewSourceFileIdentity(tc.name, tc.sha); !errors.Is(err, domain.ErrInvalidSourceFileIdentity) {
			t.Fatalf("%s: err = %v, 想要 ErrInvalidSourceFileIdentity", label, err)
		}
	}
}

// TestPriceCardRegistrationRefusesMissingGovernance 证登记门：没有方向授权引用形状
// 或批准责任方的登记立不住；授权引用必须是 party-commercial 的授权工件种类，拿别的
// 工件顶数不行。
func TestPriceCardRegistrationRefusesMissingGovernance(t *testing.T) {
	source, err := domain.NewSourceFileIdentity("SYN-PRC-CARD-260820.xlsx", synSourceSHA)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	plan := fullyDeclaredSyntheticPlan(t)

	if _, err := domain.NewPriceCardRegistration(
		tenant, plan, source,
		versionReference(t, domain.ArtifactRateTable, "not-a-grant", "v1"),
		"SYN-PRC-PRICING-GOVERNANCE",
	); !errors.Is(err, domain.ErrInvalidPriceCardRegistration) {
		t.Fatalf("错种类授权：err = %v, 想要 ErrInvalidPriceCardRegistration", err)
	}

	if _, err := domain.NewPriceCardRegistration(
		tenant, plan, source,
		versionReference(t, domain.ArtifactCommercialAuthorization, "SYN-PRC-BUY-GRANT", "v1"),
		"  ",
	); !errors.Is(err, domain.ErrInvalidPriceCardRegistration) {
		t.Fatalf("空批准责任方：err = %v, 想要 ErrInvalidPriceCardRegistration", err)
	}

	if _, err := domain.NewPriceCardRegistration(
		tenant, domain.PricingPlanVersion{}, source,
		versionReference(t, domain.ArtifactCommercialAuthorization, "SYN-PRC-BUY-GRANT", "v1"),
		"SYN-PRC-PRICING-GOVERNANCE",
	); !errors.Is(err, domain.ErrInvalidPriceCardRegistration) {
		t.Fatalf("零值方案：err = %v, 想要 ErrInvalidPriceCardRegistration", err)
	}
}

// TestPriceCardRegistrationSnapshotRoundTrips 证登记快照折装重建逐字节同答，方案
// 内容摘要与治理元数据原值保留。
func TestPriceCardRegistrationSnapshotRoundTrips(t *testing.T) {
	registration := syntheticRegistration(t)

	raw, err := domain.MarshalPriceCardRegistration(registration)
	if err != nil {
		t.Fatalf("折装登记快照：%v", err)
	}
	rebuilt, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatalf("重建登记：%v", err)
	}
	again, err := domain.MarshalPriceCardRegistration(rebuilt)
	if err != nil {
		t.Fatalf("重建后再折装：%v", err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("登记快照往返不同答\n首次=%s\n再次=%s", raw, again)
	}
	if rebuilt.Plan().ContentDigest() != registration.Plan().ContentDigest() {
		t.Fatalf("方案内容摘要漂移")
	}
}

// TestPriceCardRegistrationSnapshotExposesTampering 证登记元数据被改在重建门上
// 暴露：证据索引不是可以悄悄替换的注脚。
func TestPriceCardRegistrationSnapshotExposesTampering(t *testing.T) {
	raw, err := domain.MarshalPriceCardRegistration(syntheticRegistration(t))
	if err != nil {
		t.Fatalf("折装登记快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开登记快照：%v", err)
	}
	document["sourceFileSha256"] = "not-a-digest"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封登记快照：%v", err)
	}

	if _, err := domain.RehydratePriceCardRegistration(tampered); !errors.Is(err, domain.ErrPriceCardRegistrationSnapshotInvalid) {
		t.Fatalf("err = %v, 想要 ErrPriceCardRegistrationSnapshotInvalid", err)
	}
}

// TestPriceCardRegistrationSnapshotRefusesForeignCanonicalization 证登记快照沿用
// 方案快照的规范化门：别的规范化版本记录的方案装在登记里同样拒绝重建。
func TestPriceCardRegistrationSnapshotRefusesForeignCanonicalization(t *testing.T) {
	raw, err := domain.MarshalPriceCardRegistration(syntheticRegistration(t))
	if err != nil {
		t.Fatalf("折装登记快照：%v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("解开登记快照：%v", err)
	}
	plan, ok := document["plan"].(map[string]any)
	if !ok {
		t.Fatalf("登记快照缺方案文档")
	}
	plan["canonicalization"] = "PPC-2"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("重封登记快照：%v", err)
	}

	if _, err := domain.RehydratePriceCardRegistration(tampered); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("err = %v, 想要 ErrCanonicalizationVersionUnsupported", err)
	}
}
