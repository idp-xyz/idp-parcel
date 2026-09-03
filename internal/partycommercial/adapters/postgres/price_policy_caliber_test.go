package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证价格政策口径册（票 party-commercial-context-gaps/02、06，0022）：
// 三口径往返、汇率可缺、缺口径是合法缺席、同内容重放、异内容冲突、租户是身份不是过滤器、
// 口径离不开同方向的正文行、三处耦合由库上 CHECK 守住、目录上列口径随正文左连接。

func newPricePolicyCaliberContents(t *testing.T) (
	*adapter.CommercialPublications, *adapter.PricePolicyCaliberContents, *adapter.OperationsCatalogue, bentoapp.Transactor,
) {
	t.Helper()
	repository, transactor, db := newDeclarationFixture(t)
	contents, err := adapter.NewPricePolicyCaliberContents(db)
	if err != nil {
		t.Fatalf("构造口径读口：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	return repository, contents, catalogue, transactor
}

func taxCaliberOf(t *testing.T, disposition domain.TaxDisposition, classification string) domain.TaxCaliber {
	t.Helper()
	reference := domain.TaxClassificationReference{}
	if classification != "" {
		reference = pcValue(t, domain.NewTaxClassificationReference, classification)
	}
	caliber, err := domain.NewTaxCaliber(disposition, reference)
	if err != nil {
		t.Fatalf("税务口径：%v", err)
	}
	return caliber
}

func volumetricCaliberOf(t *testing.T, direction domain.PriceDirection, factor string) domain.VolumetricCaliber {
	t.Helper()
	reference := domain.VolumetricFactorReference{}
	if factor != "" {
		reference = pcValue(t, domain.NewVolumetricFactorReference, factor)
	}
	caliber, err := domain.NewVolumetricCaliber(direction, reference)
	if err != nil {
		t.Fatalf("体积口径：%v", err)
	}
	return caliber
}

func fxCaliberOf(t *testing.T, quoteType string) domain.FxCaliber {
	t.Helper()
	caliber, err := domain.NewFxCaliber(
		pcValue(t, domain.NewFxQuoteTypeReference, quoteType),
		pcValue(t, domain.NewAsOfSemanticsReference, "AT_ORDER_DATE"),
		pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v3"),
	)
	if err != nil {
		t.Fatalf("汇率口径：%v", err)
	}
	return caliber
}

// sellCaliberWithFx 是一份销售方向、含税、带汇率口径的完整声明。
func sellCaliberWithFx(t *testing.T, version domain.CommercialVersion, quoteType string) domain.PricePolicyCaliber {
	t.Helper()
	caliber, err := domain.NewPricePolicyCaliberWithFx(
		version,
		taxCaliberOf(t, domain.TaxInclusive, "vat-standard"),
		volumetricCaliberOf(t, domain.SellDirection, "sell-divisor-5000-cm"),
		fxCaliberOf(t, quoteType),
	)
	if err != nil {
		t.Fatalf("口径声明：%v", err)
	}
	return caliber
}

// caliberWithoutFx 是一份不含汇率口径的声明，税务按不适用、体积按给定方向。
func caliberWithoutFx(t *testing.T, version domain.CommercialVersion, direction domain.PriceDirection) domain.PricePolicyCaliber {
	t.Helper()
	factor := ""
	if direction == domain.SellDirection {
		factor = "sell-divisor-5000-cm"
	}
	caliber, err := domain.NewPricePolicyCaliber(
		version,
		taxCaliberOf(t, domain.TaxNotApplicable, ""),
		volumetricCaliberOf(t, direction, factor),
	)
	if err != nil {
		t.Fatalf("口径声明：%v", err)
	}
	return caliber
}

func mustSavePricePolicyCaliber(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	caliber domain.PricePolicyCaliber,
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicyCaliber(txCtx, caliber)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicyCaliberSaved {
			t.Fatalf("save outcome = %q, want SAVED", outcome)
		}
		return nil
	})
}

// savedPricePolicy 登记一份已生效的价格规则版本及其正文，交回版本供口径挂靠。
func savedPricePolicy(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.CommercialPublications,
	objectID string,
	direction domain.PriceDirection,
) domain.CommercialVersion {
	t.Helper()
	version := effectiveVersionOfKind(t, domain.PriceRuleObject, objectID, "v1", "digest-"+objectID)
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, version, direction, direction, domain.PlanBindingConversionNone, "plan-"+objectID),
		direction, domain.PlanBindingConversionNone)
	return version
}

// Covers: CONTEXT「商业价格政策版本……声明计价所需的商业口径」与「必须声明含税、未税或税务
// 不适用」——三口径逐格往返；汇率一格可缺，缺席读回来是「没声明」而不是零口径；不适用的税务
// 口径读回来没有分类，采购方向的体积口径读回来没有系数。
func TestPricePolicyCaliberRoundTripsWithAndWithoutFx(t *testing.T) {
	repository, contents, _, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	t.Run("sell with fx", func(t *testing.T) {
		version := savedPricePolicy(t, transactor, ctx, repository, "price-sell", domain.SellDirection)
		mustSavePricePolicyCaliber(t, transactor, ctx, repository, sellCaliberWithFx(t, version, "boc-cash-selling"))

		caliber, found, err := contents.LoadPricePolicyCaliber(ctx, tenant, version)
		if err != nil || !found {
			t.Fatalf("读回：found=%v err=%v", found, err)
		}
		if caliber.Tax().Disposition() != domain.TaxInclusive {
			t.Fatalf("税务口径 = %q", caliber.Tax().Disposition())
		}
		if classification, ok := caliber.Tax().Classification(); !ok || classification.String() != "vat-standard" {
			t.Fatalf("税务分类 = (%q, %v)", classification, ok)
		}
		if factor, ok := caliber.Volumetric().Factor(); !ok || factor.String() != "sell-divisor-5000-cm" ||
			caliber.Volumetric().Direction() != domain.SellDirection {
			t.Fatalf("体积口径 = %#v", caliber.Volumetric())
		}
		fx, declared := caliber.Fx()
		if !declared || fx.QuoteType().String() != "boc-cash-selling" ||
			fx.AsOfSemantics().String() != "AT_ORDER_DATE" || fx.AsOfPolicyVersion().String() != "asof-policy/v3" {
			t.Fatalf("汇率口径 = (%#v, %v)", fx, declared)
		}
		if !caliber.Version().SameVersionAs(version) {
			t.Fatal("口径挂回了另一个版本")
		}
	})

	t.Run("buy without fx", func(t *testing.T) {
		version := savedPricePolicy(t, transactor, ctx, repository, "price-buy", domain.BuyDirection)
		mustSavePricePolicyCaliber(t, transactor, ctx, repository, caliberWithoutFx(t, version, domain.BuyDirection))

		caliber, found, err := contents.LoadPricePolicyCaliber(ctx, tenant, version)
		if err != nil || !found {
			t.Fatalf("读回：found=%v err=%v", found, err)
		}
		if _, declared := caliber.Fx(); declared {
			t.Fatal("没声明汇率口径却读回了一份")
		}
		if caliber.Tax().Disposition() != domain.TaxNotApplicable {
			t.Fatalf("税务口径 = %q", caliber.Tax().Disposition())
		}
		if _, ok := caliber.Tax().Classification(); ok {
			t.Fatal("税务不适用却读回了一份分类")
		}
		if _, ok := caliber.Volumetric().Factor(); ok || caliber.Volumetric().Direction() != domain.BuyDirection {
			t.Fatalf("采购方向读回了体积系数：%#v", caliber.Volumetric())
		}
	})
}

// Covers: 缺口径是合法缺席（found=false），不是 error——0010 早于 0022 存在，只有正文没有口径的
// 政策是合法状态；两种缺席（正文未登记 / 口径未登记）的续办都是去发布，读口不必分。
func TestAPricePolicyWithoutACaliberIsNotFound(t *testing.T) {
	repository, contents, _, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()

	version := savedPricePolicy(t, transactor, ctx, repository, "price-bare", domain.SellDirection)

	caliber, found, err := contents.LoadPricePolicyCaliber(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil {
		t.Fatalf("没口径被当成了错误：%v", err)
	}
	if found {
		t.Fatalf("没登记口径却读回了一份：%#v", caliber)
	}
}

// Covers: ADR-0031——同内容重放答`已登记`，异内容答`内容冲突`（含汇率格从缺席变在场），两者都
// 不是 error，且原行一字不动。
func TestSavingAPricePolicyCaliberTwiceIsAReplayAndAChangedCaliberConflicts(t *testing.T) {
	repository, contents, _, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()

	version := savedPricePolicy(t, transactor, ctx, repository, "price-1", domain.SellDirection)
	original := caliberWithoutFx(t, version, domain.SellDirection)
	mustSavePricePolicyCaliber(t, transactor, ctx, repository, original)

	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicyCaliber(txCtx, original)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicyCaliberAlreadyRegistered {
			t.Fatalf("replay outcome = %q, want ALREADY_REGISTERED", outcome)
		}
		return nil
	})

	changed := sellCaliberWithFx(t, version, "boc-cash-selling")
	mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.SavePricePolicyCaliber(txCtx, changed)
		if err != nil {
			return err
		}
		if outcome != ports.PricePolicyCaliberContentConflict {
			t.Fatalf("conflict outcome = %q, want CONTENT_CONFLICT", outcome)
		}
		return nil
	})

	caliber, found, err := contents.LoadPricePolicyCaliber(ctx, pcTenant(t, "tenant-1"), version)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	if _, declared := caliber.Fx(); declared || caliber.Tax().Disposition() != domain.TaxNotApplicable {
		t.Fatalf("冲突写入改动了原行：%#v", caliber)
	}
}

// Covers: ADR-0003——租户是身份不是过滤器。拿另一个租户去读本租户的版本是 error 且不交内容；
// 他租户登记的同名版本不进本租户的读口。
func TestPricePolicyCaliberIsBoundToItsTenant(t *testing.T) {
	repository, contents, _, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()

	mine := savedPricePolicy(t, transactor, ctx, repository, "price-1", domain.SellDirection)
	theirs := policyVersionInTenant(t, "tenant-2", domain.PriceRuleObject, "price-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, theirs, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-theirs"),
		domain.SellDirection, domain.PlanBindingConversionNone)
	mustSavePricePolicyCaliber(t, transactor, ctx, repository, sellCaliberWithFx(t, theirs, "their-quote"))

	if _, found, err := contents.LoadPricePolicyCaliber(ctx, pcTenant(t, "tenant-2"), mine); err == nil || found {
		t.Fatalf("拿他租户身份读本租户版本：found=%v err=%v，应是 error 且不交内容", found, err)
	}
	if _, found, err := contents.LoadPricePolicyCaliber(ctx, pcTenant(t, "tenant-1"), mine); err != nil || found {
		t.Fatalf("他租户的口径进了本租户的读口：found=%v err=%v", found, err)
	}
}

// Covers: 票 06「口径只能随正文同一次发布登记」与「体积口径的方向必须等于政策自己的方向」——
// 两条都由 0022 的复合外键在库上守住：没有正文行、或方向对不上，口径都进不去，且不留半行。
func TestAPricePolicyCaliberNeedsABodyRowOfTheSameDirection(t *testing.T) {
	repository, contents, _, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	t.Run("no body row", func(t *testing.T) {
		version := effectiveVersionOfKind(t, domain.PriceRuleObject, "price-bodyless", "v1", "digest-bodyless")
		mustSaveVersion(t, transactor, ctx, repository, version)

		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := repository.SavePricePolicyCaliber(txCtx, caliberWithoutFx(t, version, domain.SellDirection))
			return err
		})
		if err == nil {
			t.Fatal("没有正文行却登记了口径")
		}
		if _, found, err := contents.LoadPricePolicyCaliber(ctx, tenant, version); err != nil || found {
			t.Fatalf("被拒的口径留下了行：found=%v err=%v", found, err)
		}
	})

	t.Run("direction mismatch", func(t *testing.T) {
		version := savedPricePolicy(t, transactor, ctx, repository, "price-buy", domain.BuyDirection)

		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := repository.SavePricePolicyCaliber(txCtx, caliberWithoutFx(t, version, domain.SellDirection))
			return err
		})
		if err == nil {
			t.Fatal("销售方向的体积口径挂上了采购政策")
		}
		if _, found, err := contents.LoadPricePolicyCaliber(ctx, tenant, version); err != nil || found {
			t.Fatalf("被拒的口径留下了行：found=%v err=%v", found, err)
		}
	})
}

// Covers: 三处耦合由库上 CHECK 守住——含税无分类、不适用带分类、采购带系数、销售无系数、汇率
// 半缺，五种行都进不来。领域构造门拦得住经它进来的，CHECK 拦的是绕开它的那条路。
func TestPricePolicyCaliberColumnsRejectDecoupledRows(t *testing.T) {
	repository, transactor, pool := newPublications(t)
	ctx := t.Context()
	savedPricePolicy(t, transactor, ctx, repository, "price-sell", domain.SellDirection)
	savedPricePolicy(t, transactor, ctx, repository, "price-buy", domain.BuyDirection)

	insert := func(objectID, direction, taxDisposition, taxClassification, volumetricFactor, fxQuoteType, fxSemantics, fxPolicy string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.price_policy_caliber
				(tenant_id, object_kind, object_id, version_label, direction,
				 tax_disposition, tax_classification_ref, volumetric_factor_ref,
				 fx_quote_type_ref, fx_as_of_semantics_ref, fx_as_of_policy_version)
			 VALUES ('tenant-1', 6, '`+objectID+`', 'v1', '`+direction+`',
			         '`+taxDisposition+`', `+taxClassification+`, `+volumetricFactor+`,
			         `+fxQuoteType+`, `+fxSemantics+`, `+fxPolicy+`)`)
		return err
	}
	if err := insert("price-sell", "SELL", "TAX_INCLUSIVE", "NULL", "'f'", "NULL", "NULL", "NULL"); err == nil {
		t.Fatal("含税却没有分类的口径进了册")
	}
	if err := insert("price-sell", "SELL", "TAX_NOT_APPLICABLE", "'vat'", "'f'", "NULL", "NULL", "NULL"); err == nil {
		t.Fatal("税务不适用却带着分类的口径进了册")
	}
	if err := insert("price-sell", "SELL", "TAX_NOT_APPLICABLE", "NULL", "NULL", "NULL", "NULL", "NULL"); err == nil {
		t.Fatal("销售方向没有体积系数的口径进了册")
	}
	if err := insert("price-buy", "BUY", "TAX_NOT_APPLICABLE", "NULL", "'f'", "NULL", "NULL", "NULL"); err == nil {
		t.Fatal("采购方向带着体积系数的口径进了册")
	}
	if err := insert("price-sell", "SELL", "TAX_NOT_APPLICABLE", "NULL", "'f'", "'quote'", "NULL", "NULL"); err == nil {
		t.Fatal("只有牌价类型没有时点的汇率口径进了册")
	}
	if err := insert("price-sell", "SELL", "TAX_NOT_APPLICABLE", "NULL", "'f'", "NULL", "NULL", "NULL"); err != nil {
		t.Fatalf("一行合法口径被拒：%v", err)
	}
}

// Covers: 目录上列价格政策册时口径随正文左连接（ports.PricePolicyRow 的 HasCaliber / HasFx）——
// 有口径的行带口径列，没口径的行 HasCaliber 为假而正文照常上列；跨租户不可见。
func TestPricePolicyCatalogueListsCaliberBesideTheBody(t *testing.T) {
	repository, _, catalogue, transactor := newPricePolicyCaliberContents(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	withFx := savedPricePolicy(t, transactor, ctx, repository, "price-fx", domain.SellDirection)
	mustSavePricePolicyCaliber(t, transactor, ctx, repository, sellCaliberWithFx(t, withFx, "boc-cash-selling"))
	withoutFx := savedPricePolicy(t, transactor, ctx, repository, "price-plain", domain.BuyDirection)
	mustSavePricePolicyCaliber(t, transactor, ctx, repository, caliberWithoutFx(t, withoutFx, domain.BuyDirection))
	savedPricePolicy(t, transactor, ctx, repository, "price-bare", domain.SellDirection)

	theirs := policyVersionInTenant(t, "tenant-2", domain.PriceRuleObject, "price-theirs", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, theirs, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-theirs"),
		domain.SellDirection, domain.PlanBindingConversionNone)
	mustSavePricePolicyCaliber(t, transactor, ctx, repository, sellCaliberWithFx(t, theirs, "their-quote"))

	rows, err := catalogue.ListPricePolicies(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列价格政策：%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("上列 %d 行，want 3（他租户的不得进本租户目录）", len(rows))
	}
	byObject := map[string]ports.PricePolicyRow{}
	for _, row := range rows {
		byObject[row.ObjectID] = row
	}
	if row := byObject["price-fx"]; !row.HasCaliber || !row.HasFx ||
		row.TaxDisposition != "TAX_INCLUSIVE" || row.TaxClassification != "vat-standard" ||
		row.VolumetricFactor != "sell-divisor-5000-cm" ||
		row.FxQuoteType != "boc-cash-selling" || row.FxAsOfSemantics != "AT_ORDER_DATE" ||
		row.FxAsOfPolicyVersion != "asof-policy/v3" || row.CaliberRegisteredAt.IsZero() {
		t.Fatalf("含汇率的口径行 = %#v", row)
	}
	if row := byObject["price-plain"]; !row.HasCaliber || row.HasFx ||
		row.TaxDisposition != "TAX_NOT_APPLICABLE" || row.TaxClassification != "" ||
		row.VolumetricFactor != "" || row.FxQuoteType != "" {
		t.Fatalf("不含汇率的口径行 = %#v", row)
	}
	if row := byObject["price-bare"]; row.HasCaliber || row.HasFx || row.TaxDisposition != "" || row.Direction != "SELL" {
		t.Fatalf("无口径的正文行 = %#v", row)
	}
}
