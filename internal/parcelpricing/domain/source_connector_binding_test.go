package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证 ADR-0099 决定六的实例半边那一格：来源连接器绑定是租户对某来源的声明（连接器种类、
// 序列标识、口径引用、抓取节律、免人工复核），机制只提供格、不设默认——免复核声明未给出即拒，
// 不折成「未声明」；汇率绑定同样不接受未声明口径。

func fxBindingSpec(t *testing.T) domain.SourceConnectorBindingSpec {
	t.Helper()
	return domain.SourceConnectorBindingSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		SeriesID:         "SYN-PRC-USD-CNY",
		Version:          "b1",
		ConnectorKind:    "FILE",
		SourceIdentifier: "SYN-SOURCE/usd-cny-daily",
		SourceLocator:    "rates/usd-cny/latest.json",
		SeriesKind:       domain.ReferenceSeriesExchangeRate,
		QuoteBasis:       versionReference(t, domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1"),
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Cadence:          "DAILY",
		ReviewExemption:  domain.ReviewExemptionGranted,
	}
}

func fxBinding(t *testing.T) domain.SourceConnectorBinding {
	t.Helper()
	binding, err := domain.NewSourceConnectorBinding(fxBindingSpec(t))
	if err != nil {
		t.Fatalf("构造来源连接器绑定：%v", err)
	}
	return binding
}

// TestSourceConnectorBindingCarriesItsDeclaration 证绑定原样带出声明的每一格，连接器身份与
// 免复核依据由绑定派生：身份按连接器种类，依据指回本绑定及其版本（ADR-0099 决定六）。
func TestSourceConnectorBindingCarriesItsDeclaration(t *testing.T) {
	binding := fxBinding(t)

	if binding.Tenant().String() != "tenant-1" || binding.SeriesID() != "SYN-PRC-USD-CNY" || binding.Version() != "b1" ||
		binding.ConnectorKind() != "FILE" || binding.SourceIdentifier() != "SYN-SOURCE/usd-cny-daily" ||
		binding.SourceLocator() != "rates/usd-cny/latest.json" || binding.SeriesKind() != domain.ReferenceSeriesExchangeRate ||
		binding.Registrant() != "SYN-PRC-SERIES-REGISTRAR" || binding.ReviewExemption() != domain.ReviewExemptionGranted {
		t.Fatalf("绑定字段走样：%+v", binding)
	}
	basis, declared := binding.QuoteBasis()
	if !declared || basis.ID() != "SYN-PRC-FX-POLICY" {
		t.Fatalf("口径引用丢了：declared=%v basis=%v", declared, basis)
	}
	cadence, scheduled := binding.Cadence()
	if !scheduled || cadence != "DAILY" {
		t.Fatalf("抓取节律走样：%q %v", cadence, scheduled)
	}
	if binding.ConnectorIdentity() != "connector:FILE" {
		t.Fatalf("连接器身份 = %q", binding.ConnectorIdentity())
	}
	if binding.ExemptionReviewBasis() != "免人工复核声明 ← 来源连接器绑定 SYN-PRC-USD-CNY@b1" {
		t.Fatalf("免复核依据 = %q", binding.ExemptionReviewBasis())
	}
	if binding.RequiresManualReview() {
		t.Fatal("声明免复核的绑定仍要求人工复核")
	}
}

// TestReviewExemptionHasNoDefault 证「未声明 = 需人工复核」是显式取值，不是零值的解读：
// 零值与未知取值都进不了构造门；三个封闭取值里只有「是」免人工复核。
func TestReviewExemptionHasNoDefault(t *testing.T) {
	for _, exemption := range []domain.ReviewExemption{"", "YES", "是"} {
		spec := fxBindingSpec(t)
		spec.ReviewExemption = exemption
		if _, err := domain.NewSourceConnectorBinding(spec); !errors.Is(err, domain.ErrInvalidSourceConnectorBinding) {
			t.Fatalf("免复核声明 %q 被接受了：err=%v", exemption, err)
		}
	}

	requires := map[domain.ReviewExemption]bool{
		domain.ReviewExemptionGranted:    false,
		domain.ReviewExemptionWithheld:   true,
		domain.ReviewExemptionUndeclared: true,
	}
	for exemption, want := range requires {
		spec := fxBindingSpec(t)
		spec.ReviewExemption = exemption
		binding, err := domain.NewSourceConnectorBinding(spec)
		if err != nil {
			t.Fatalf("%s：%v", exemption, err)
		}
		if binding.RequiresManualReview() != want {
			t.Fatalf("%s 要求人工复核 = %v，想要 %v", exemption, binding.RequiresManualReview(), want)
		}
		if exemption.String() != string(exemption) {
			t.Fatalf("%s 的 String 走样：%q", exemption, exemption.String())
		}
	}
}

// TestSourceConnectorBindingRefusesIncompleteDeclarations 证缺任一件立不住：序列标识、绑定版本、
// 连接器种类、来源标识、来源定位符、登记责任方都是硬件；带边空白同样拒（同一条 trimmed 纪律）。
func TestSourceConnectorBindingRefusesIncompleteDeclarations(t *testing.T) {
	mutations := map[string]func(*domain.SourceConnectorBindingSpec){
		"空序列标识":    func(spec *domain.SourceConnectorBindingSpec) { spec.SeriesID = "" },
		"空绑定版本":    func(spec *domain.SourceConnectorBindingSpec) { spec.Version = " " },
		"空连接器种类":   func(spec *domain.SourceConnectorBindingSpec) { spec.ConnectorKind = "" },
		"空来源标识":    func(spec *domain.SourceConnectorBindingSpec) { spec.SourceIdentifier = "" },
		"空来源定位符":   func(spec *domain.SourceConnectorBindingSpec) { spec.SourceLocator = "" },
		"空登记责任方":   func(spec *domain.SourceConnectorBindingSpec) { spec.Registrant = "" },
		"带边空白的定位符": func(spec *domain.SourceConnectorBindingSpec) { spec.SourceLocator = " rates/x.json" },
		"零租户":      func(spec *domain.SourceConnectorBindingSpec) { spec.Tenant = domain.TenantID{} },
		"未知序列种类":   func(spec *domain.SourceConnectorBindingSpec) { spec.SeriesKind = "MOON_PHASE" },
		"口径不是价格政策": func(spec *domain.SourceConnectorBindingSpec) {
			spec.QuoteBasis = versionReference(t, domain.ArtifactRateTable, "SYN-TABLE", "v1")
		},
	}
	for label, mutate := range mutations {
		spec := fxBindingSpec(t)
		mutate(&spec)
		if _, err := domain.NewSourceConnectorBinding(spec); !errors.Is(err, domain.ErrInvalidSourceConnectorBinding) {
			t.Fatalf("%s 被接受了：err=%v", label, err)
		}
	}
}

// TestExchangeRateBindingDemandsAQuoteBasis 证汇率绑定不接受未声明口径（CONTEXT：不接受未声明
// 口径的裸汇率）；燃油绑定的口径可缺——折扣系数写在卡上。节律可缺：机制不调度，只登记声明。
func TestExchangeRateBindingDemandsAQuoteBasis(t *testing.T) {
	bare := fxBindingSpec(t)
	bare.QuoteBasis = domain.VersionReference{}
	if _, err := domain.NewSourceConnectorBinding(bare); !errors.Is(err, domain.ErrInvalidSourceConnectorBinding) {
		t.Fatalf("裸汇率绑定被接受了：err=%v", err)
	}

	fuel := fxBindingSpec(t)
	fuel.SeriesKind = domain.ReferenceSeriesFuelRate
	fuel.QuoteBasis = domain.VersionReference{}
	fuel.Cadence = ""
	binding, err := domain.NewSourceConnectorBinding(fuel)
	if err != nil {
		t.Fatalf("燃油绑定无口径被拒：%v", err)
	}
	if _, declared := binding.QuoteBasis(); declared {
		t.Fatal("没声明口径却报有口径")
	}
	if _, scheduled := binding.Cadence(); scheduled {
		t.Fatal("没声明节律却报有节律")
	}
}
