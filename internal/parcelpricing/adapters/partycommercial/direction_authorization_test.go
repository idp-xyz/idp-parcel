package partycommercial_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/partycommercial"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 真库：三格拒绝不入册；方向相符的登记入册；同一份已入册内容在授权后来被收尾之后重放仍答已登记。
// 价格政策与授权规则的目录能按引用筛到对方。授权号与政策号都由本用例现造，不是产品默认。
func TestPriceCardRegistrationChecksTheCitedDirectionAuthorizationOnARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := partycommercial.NewDirectionAuthorizations(publications)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	handler := application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{
		Catalog: cards, Registered: directionHandoff{}, DirectionAuthorizations: reader,
		RegisteredContent: cards, Clock: directionClock{at: at},
	})
	tenant := pcMust(t, pcdomain.NewTenantID, "tenant-dir")
	open := pcInterval(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	ended := pcInterval(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))

	sellPolicy := pcVersion(t, pcdomain.PriceRuleObject, "policy-sell", "v1", open, nil)
	sellAuth := pcVersionReferencing(t, pcdomain.AuthorizationRuleObject, "auth-sell", "v1", open, pcdomain.PriceRuleObject, "policy-sell")
	mustPublishPolicy(t, db, publications, sellPolicy, pcdomain.SellDirection)
	mustPublishVersion(t, db, publications, sellAuth)

	buyPolicy := pcVersion(t, pcdomain.PriceRuleObject, "policy-buy", "v1", open, nil)
	buyAuth := pcVersionReferencing(t, pcdomain.AuthorizationRuleObject, "auth-buy", "v1", open, pcdomain.PriceRuleObject, "policy-buy")
	mustPublishPolicy(t, db, publications, buyPolicy, pcdomain.BuyDirection)
	mustPublishVersion(t, db, publications, buyAuth)

	closedPolicy := pcVersion(t, pcdomain.PriceRuleObject, "policy-closed", "v1", ended, nil)
	closedAuth := pcVersionReferencing(t, pcdomain.AuthorizationRuleObject, "auth-closed", "v1", ended, pcdomain.PriceRuleObject, "policy-closed")
	mustPublishPolicy(t, db, publications, closedPolicy, pcdomain.SellDirection)
	mustPublishVersion(t, db, publications, closedAuth)

	recorded := cardRegistration(t, "plan-ok", "auth-sell", ppdomain.PricingDirectionSell)
	if outcome := mustRegisterCard(t, db, handler, recorded); outcome != application.PriceCardRecorded {
		t.Fatalf("方向相符：%s", outcome)
	}
	if outcome := mustRegisterCard(t, db, handler, cardRegistration(t, "plan-missing", "auth-missing", ppdomain.PricingDirectionSell)); outcome != application.PriceCardRegistrationDirectionAuthorizationNotPublished {
		t.Fatalf("不在册：%s", outcome)
	}
	if outcome := mustRegisterCard(t, db, handler, cardRegistration(t, "plan-closed", "auth-closed", ppdomain.PricingDirectionSell)); outcome != application.PriceCardRegistrationDirectionAuthorizationNotInForce {
		t.Fatalf("已到期：%s", outcome)
	}
	if outcome := mustRegisterCard(t, db, handler, cardRegistration(t, "plan-mismatch", "auth-buy", ppdomain.PricingDirectionSell)); outcome != application.PriceCardRegistrationDirectionAuthorizationDirectionMismatch {
		t.Fatalf("方向不符：%s", outcome)
	}
	for _, planID := range []string{"plan-missing", "plan-closed", "plan-mismatch"} {
		reference, refErr := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactPricingPlan, planID, "v1")
		if refErr != nil {
			t.Fatal(refErr)
		}
		if _, found, findErr := cards.FindByReference(t.Context(), recorded.Tenant(), reference); findErr != nil || found {
			t.Fatalf("%s 不该入册：found=%v err=%v", planID, found, findErr)
		}
	}

	successor := pcVersionReferencing(t, pcdomain.AuthorizationRuleObject, "auth-sell", "v2", open, pcdomain.PriceRuleObject, "policy-sell")
	mustPublishVersion(t, db, publications, successor)
	mustRetire(t, db, publications, sellAuth, "v2", at.Add(-time.Hour))
	if outcome := mustRegisterCard(t, db, handler, recorded); outcome != application.PriceCardAlreadyOnRegister {
		t.Fatalf("同一份内容在授权收尾后重放：%s", outcome)
	}
	if outcome := mustRegisterCard(t, db, handler, cardRegistration(t, "plan-after-close", "auth-sell", ppdomain.PricingDirectionSell)); outcome != application.PriceCardRegistrationDirectionAuthorizationNotInForce {
		t.Fatalf("收尾后的新登记：%s", outcome)
	}

	pricingCatalogue, err := pppostgres.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	cardQuery, err := ppports.PriceCardPages.Decode(url.Values{
		"authorizationId":      {recorded.DirectionAuthorization().ID()},
		"authorizationVersion": {recorded.DirectionAuthorization().Version()},
	})
	if err != nil {
		t.Fatal(err)
	}
	cardPage, err := pricingCatalogue.ListPriceCards(t.Context(), recorded.Tenant(), 10, cardQuery)
	if err != nil {
		t.Fatalf("按授权列价卡：%v", err)
	}
	if len(cardPage.Rows) != 1 || cardPage.Rows[0].PlanID != "plan-ok" {
		t.Fatalf("引用这版授权的价卡 = %+v", cardPage.Rows)
	}

	commercialCatalogue, err := pcpostgres.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	policyQuery, err := pcports.AuthorizationRulePages.Decode(url.Values{"pricePolicy": {"policy-sell"}})
	if err != nil {
		t.Fatal(err)
	}
	policyPage, err := commercialCatalogue.ListAuthorizationRules(t.Context(), tenant, 10, policyQuery)
	if err != nil {
		t.Fatalf("按价格政策列授权：%v", err)
	}
	if len(policyPage.Rows) != 2 {
		t.Fatalf("指名 policy-sell 的授权规则 = %d 行，want 2", len(policyPage.Rows))
	}
}

type directionClock struct{ at time.Time }

func (clock directionClock) Now() time.Time { return clock.at }

type directionHandoff struct{}

func (directionHandoff) HandOffPricingParameterRegistered(context.Context, ppports.PricingParameterRegisteredIntent) error {
	return nil
}

func mustRegisterCard(t *testing.T, db *bentopg.DB, handler *application.RegisterPriceCardHandler, registration ppdomain.PriceCardRegistration) application.RegisterPriceCardOutcome {
	t.Helper()
	var outcome application.RegisterPriceCardOutcome
	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var handleErr error
		outcome, handleErr = handler.Handle(ctx, application.RegisterPriceCardCommand{Registration: registration})
		return handleErr
	})
	if err != nil {
		t.Fatalf("登记 %s：%v", registration.Plan().Reference().ID(), err)
	}
	return outcome
}

func mustPublishVersion(t *testing.T, db *bentopg.DB, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion) {
	t.Helper()
	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := publications.SaveVersion(ctx, version)
		if saveErr != nil {
			return saveErr
		}
		if outcome != pcports.PublicationSaved {
			return errOutcome(outcome.String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("发布 %s/%s：%v", version.ObjectID(), version.Version(), err)
	}
}

func mustPublishPolicy(t *testing.T, db *bentopg.DB, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion, direction pcdomain.PriceDirection) {
	t.Helper()
	mustPublishVersion(t, db, publications, version)
	policy, err := pcdomain.NewCommercialPricePolicy(
		version, direction,
		pcMust(t, pcdomain.NewPricingPlanReference, "plan-bound"),
		direction, pcdomain.PlanBindingConversionNone, version.Scope(), version.Effective(),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := publications.SavePricePolicy(ctx, policy, direction, pcdomain.PlanBindingConversionNone)
		if saveErr != nil {
			return saveErr
		}
		if outcome != pcports.PricePolicySaved {
			return errOutcome(outcome.String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("价格政策正文 %s：%v", version.ObjectID(), err)
	}
}

func mustRetire(t *testing.T, db *bentopg.DB, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion, successor string, at time.Time) {
	t.Helper()
	label := pcMust(t, pcdomain.NewCommercialVersionLabel, successor)
	reference := pcMust(t, pcdomain.NewRetirementReference, "retire-"+version.ObjectID().String())
	retirement, err := version.RetireInFavourOf(label, reference, at)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		outcome, saveErr := publications.SaveVersionRetirement(ctx, retirement)
		if saveErr != nil {
			return saveErr
		}
		if outcome != pcports.VersionRetirementSaved {
			return errOutcome(outcome.String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("收尾 %s：%v", version.ObjectID(), err)
	}
}

type outcomeError string

func (err outcomeError) Error() string { return string(err) }

func errOutcome(outcome string) error { return outcomeError(outcome) }

func pcInterval(t *testing.T, startsAt, endsAt time.Time) pcdomain.EffectiveInterval {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatal(err)
	}
	return interval
}

func pcVersion(t *testing.T, kind pcdomain.CommercialObjectKind, objectID, label string, interval pcdomain.EffectiveInterval, references map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID) pcdomain.CommercialVersion {
	t.Helper()
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      pcMust(t, pcdomain.NewTenantID, "tenant-dir"),
		Kind:          kind,
		ObjectID:      pcMust(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       pcMust(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         pcMust(t, pcdomain.NewCommercialScopeReference, "scope-dir"),
		ContentDigest: pcMust(t, pcdomain.NewCommercialContentDigest, "digest-"+objectID+"-"+label),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      pcApproval(t, "approval-"+objectID+"-"+label),
		PublishedAt:   time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   interval.StartsAt(),
		References:    references,
	})
	if err != nil {
		t.Fatalf("重建 %s/%s：%v", objectID, label, err)
	}
	return version
}

func pcVersionReferencing(t *testing.T, kind pcdomain.CommercialObjectKind, objectID, label string, interval pcdomain.EffectiveInterval, referenced pcdomain.CommercialObjectKind, referencedID string) pcdomain.CommercialVersion {
	t.Helper()
	return pcVersion(t, kind, objectID, label, interval, map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID{
		referenced: pcMust(t, pcdomain.NewCommercialObjectID, referencedID),
	})
}

func pcApproval(t *testing.T, reference string) pcdomain.ApprovalBasis {
	t.Helper()
	basis, err := pcdomain.NewApprovalBasis(
		pcMust(t, pcdomain.NewApprovalReference, reference),
		pcMust(t, pcdomain.NewCommercialSourceReference, "source-"+reference),
		time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return basis
}

func pcMust[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%s：%v", raw, err)
	}
	return value
}

func cardRegistration(t *testing.T, planID, authID string, direction ppdomain.PricingDirection) ppdomain.PriceCardRegistration {
	t.Helper()
	currency := pcMust(t, ppdomain.NewCurrency, "USD")
	amount, err := ppdomain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := ppdomain.NewRateEntry(pcMust(t, ppdomain.NewRateEntryID, planID+"-entry"), "Z1", ppWeight(t, "0"), ppWeight(t, "10"), amount)
	if err != nil {
		t.Fatal(err)
	}
	period, err := ppdomain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	table, err := ppdomain.NewRateTableVersion(
		ppRef(t, ppdomain.ArtifactRateTable, planID+"-table", "v1"),
		ppdomain.RateTableFamilyWeightZone, currency, ppdomain.WeightUnitKilogram, period, []ppdomain.RateEntry{entry},
	)
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, ppWeight(t, "0.5"))
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewPricingWeightPolicy(
		ppRef(t, ppdomain.ArtifactWeightPolicy, planID+"-weight", "v1"),
		ppdomain.PricingWeightActualOnly, rounding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	purpose := ppdomain.PricingPurposeCustomerCharge
	if direction == ppdomain.PricingDirectionBuy {
		purpose = ppdomain.PricingPurposeSupplierCost
	}
	plan, err := ppdomain.NewPricingPlanVersion(
		ppRef(t, ppdomain.ArtifactPricingPlan, planID, "v1"),
		pcMust(t, ppdomain.NewPricingScopeID, "scope-card"),
		direction, purpose, pcMust(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		period, table, weight, nil, ppdomain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatal(err)
	}
	source, err := ppdomain.NewSourceFileIdentity(planID+".xlsx", "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(
		pcMust(t, ppdomain.NewTenantID, "tenant-dir"), plan, source,
		ppRef(t, ppdomain.ArtifactCommercialAuthorization, authID, "v1"), "governor-dir",
	)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func ppWeight(t *testing.T, value string) ppdomain.Weight {
	t.Helper()
	weight, err := ppdomain.NewWeight(pcMust(t, ppdomain.ParseDecimal, value), ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func ppRef(t *testing.T, kind ppdomain.ArtifactKind, id, version string) ppdomain.VersionReference {
	t.Helper()
	reference, err := ppdomain.NewVersionReference(kind, id, version, "sha256:"+id)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}
