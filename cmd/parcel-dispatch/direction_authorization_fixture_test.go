package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ppcommercial "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/partycommercial"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// priceCardRegistrarForTest 交回与受控登记口同一种装法的价卡登记用例：真库价卡册、真的方向授权读口、已登记内容读口与时钟。
// 价卡入册前要核它引用的方向授权；不接读口，登记答未决，本包那些靠登卡铸出「计价参数已登记」信封的用例就走不到要证的那一步。
// 这里先为这张卡引用的授权补一版过得了门的规则，授权号取自卡上的引用，不写成产品默认。
func priceCardRegistrarForTest(t *testing.T, db *bentopg.DB, registration ppdomain.PriceCardRegistration) *ppapplication.RegisterPriceCardHandler {
	t.Helper()
	if err := publishDirectionAuthorization(t.Context(), db, registration); err != nil {
		t.Fatalf("为 %s 补方向授权：%v", registration.Plan().Reference().ID(), err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造商业发布册：%v", err)
	}
	authorizations, err := ppcommercial.NewDirectionAuthorizations(publications)
	if err != nil {
		t.Fatalf("构造方向授权读口：%v", err)
	}
	return ppapplication.NewRegisterPriceCardHandler(ppapplication.RegisterPriceCardDeps{
		Catalog: cards, Registered: pricingParameterHandoffForTest(t, db),
		DirectionAuthorizations: authorizations, RegisteredContent: cards, Clock: systemClock{},
	})
}

// publishDirectionAuthorization 为这张卡引用的授权补一版已生效、方向相同的价格政策与指名它的授权规则。同一授权再补一次是重放。
func publishDirectionAuthorization(ctx context.Context, db *bentopg.DB, registration ppdomain.PriceCardRegistration) error {
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return err
	}
	direction, known := pcdomain.PriceDirectionNamed(registration.Plan().Direction().String())
	if !known {
		return fmt.Errorf("价卡方向 %s 不是商业价格方向", registration.Plan().Direction())
	}
	tenant, err := pcdomain.NewTenantID(registration.Tenant().String())
	if err != nil {
		return err
	}
	authID := registration.DirectionAuthorization().ID()
	authVersion := registration.DirectionAuthorization().Version()
	policyID := authID + "-POLICY"
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		return err
	}
	policyVersion, err := publishedAuthorizationFixtureVersion(tenant, pcdomain.PriceRuleObject, policyID, "v1", interval, nil)
	if err != nil {
		return err
	}
	policyObject, err := pcdomain.NewCommercialObjectID(policyID)
	if err != nil {
		return err
	}
	authorizationVersion, err := publishedAuthorizationFixtureVersion(tenant, pcdomain.AuthorizationRuleObject, authID, authVersion, interval,
		map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID{pcdomain.PriceRuleObject: policyObject})
	if err != nil {
		return err
	}
	plan, err := pcdomain.NewPricingPlanReference(policyID)
	if err != nil {
		return err
	}
	policy, err := pcdomain.NewCommercialPricePolicy(policyVersion, direction, plan, direction, pcdomain.PlanBindingConversionNone, policyVersion.Scope(), interval)
	if err != nil {
		return err
	}
	return db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := saveAuthorizationFixtureVersion(txCtx, publications, policyVersion); err != nil {
			return err
		}
		outcome, err := publications.SavePricePolicy(txCtx, policy, direction, pcdomain.PlanBindingConversionNone)
		if err != nil {
			return err
		}
		if outcome != pcports.PricePolicySaved && outcome != pcports.PricePolicyAlreadyRegistered {
			return fmt.Errorf("价格政策 %s", outcome)
		}
		return saveAuthorizationFixtureVersion(txCtx, publications, authorizationVersion)
	})
}

func saveAuthorizationFixtureVersion(ctx context.Context, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion) error {
	outcome, err := publications.SaveVersion(ctx, version)
	if err != nil {
		return err
	}
	if outcome != pcports.PublicationSaved && outcome != pcports.PublicationAlreadyRegistered {
		return fmt.Errorf("发布 %s/%s：%s", version.ObjectID(), version.Version(), outcome)
	}
	return nil
}

func publishedAuthorizationFixtureVersion(
	tenant pcdomain.TenantID,
	kind pcdomain.CommercialObjectKind,
	objectID, label string,
	interval pcdomain.EffectiveInterval,
	references map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID,
) (pcdomain.CommercialVersion, error) {
	object, err := pcdomain.NewCommercialObjectID(objectID)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	version, err := pcdomain.NewCommercialVersionLabel(label)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	scope, err := pcdomain.NewCommercialScopeReference("scope-" + objectID)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	digest, err := pcdomain.NewCommercialContentDigest("digest-" + objectID + "-" + label)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	approvalRef, err := pcdomain.NewApprovalReference("approval-" + objectID + "-" + label)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	source, err := pcdomain.NewCommercialSourceReference("source-" + objectID)
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	approval, err := pcdomain.NewApprovalBasis(approvalRef, source, time.Date(2019, 12, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return pcdomain.CommercialVersion{}, err
	}
	return pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID: tenant, Kind: kind, ObjectID: object, Version: version, Scope: scope,
		ContentDigest: digest, Effective: interval, Status: pcdomain.CommercialVersionEffective,
		Approval: approval, PublishedAt: time.Date(2019, 12, 15, 0, 0, 0, 0, time.UTC),
		EffectiveAt: interval.StartsAt(), References: references,
	})
}
