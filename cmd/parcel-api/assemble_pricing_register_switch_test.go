package main

import (
	"os"
	"path/filepath"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: ADR-0352 开关语义——只有恰为 `true` 才开；缺省、空白、大小写不同、`1`、`yes` 都按关读。
func TestTheNewerCanonicalizationSwitchOpensOnlyOnTrue(t *testing.T) {
	cases := map[string]bool{"": false, "true": true, " true ": true, "TRUE": false, "True": false, "1": false, "yes": false, "false": false}
	for value, want := range cases {
		getenv := func(name string) string {
			if name == pricingRegisterNewerCanonicalizationEnv {
				return value
			}
			return ""
		}
		if got := pricingRegisterNewerCanonicalization(getenv); got != want {
			t.Fatalf("%q 读成 %v，want %v", value, got, want)
		}
	}
	if pricingRegisterNewerCanonicalizationEnv != "IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION" {
		t.Fatalf("环境变量名 = %s：改名要同改种子脚本与运维文档", pricingRegisterNewerCanonicalizationEnv)
	}
}

// Covers: ADR-0352 在 `/pricing-price-card-registrations` 真编排上的那一面——种子 FULL-03（PPC-6）在开关关着时答
// NEWER_CANONICALIZATION_NOT_ENABLED、册上没有它；同一份在开关打开的编排上入册。输入是隔离合成，只记 `S`。
func TestTheWiredPriceCardRegistrationHonoursTheNewerCanonicalizationSwitch(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := pricingdomain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Plan().CanonicalizationVersion() == pricingdomain.CurrentCanonicalizationVersion() {
		t.Fatalf("种子 FULL-03 记的是基础版 %s，本用例要较新版", registration.Plan().CanonicalizationVersion())
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	command := pricingapp.RegisterPriceCardCommand{Registration: registration}

	off, err := buildPriceCardRegistrationOrchestration(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := off.Handle(t.Context(), command); err != nil || outcome != pricingapp.PriceCardRegistrationNewerCanonicalizationNotEnabled {
		t.Fatalf("开关关着：outcome = %s err = %v", outcome, err)
	}
	if _, found, err := cards.ReadPriceCardContent(t.Context(), registration.Tenant(), registration.Plan().Reference()); err != nil || found {
		t.Fatalf("开关关着时册上不该有这张卡：found=%v err=%v", found, err)
	}

	on, err := buildPriceCardRegistrationOrchestration(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := on.Handle(t.Context(), command); err != nil || outcome != pricingapp.PriceCardRecorded {
		t.Fatalf("开关开着：outcome = %s err = %v", outcome, err)
	}
	// 已在册之后关掉开关再重放：门不看库，照样答未启用（ADR-0352 的明示取舍），不是已在册。
	if outcome, err := off.Handle(t.Context(), command); err != nil || outcome != pricingapp.PriceCardRegistrationNewerCanonicalizationNotEnabled {
		t.Fatalf("已在册后开关关着重放：outcome = %s err = %v", outcome, err)
	}
}
