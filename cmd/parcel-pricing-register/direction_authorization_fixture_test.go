package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	domain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// publishDirectionAuthorizationForCard 为这张卡引用的授权补一版已生效、方向相同的价格政策。
// 只给既有登记用例留一条过得了门的授权，授权号取自卡上的引用，不写成产品默认。
func publishDirectionAuthorizationForCard(t *testing.T, db *bentopg.DB, registration domain.PriceCardRegistration) {
	t.Helper()
	if err := publishDirectionAuthorization(t.Context(), db, registration); err != nil {
		t.Fatal(err)
	}
}

func publishDirectionAuthorizationForPricingSeed(t *testing.T, db *bentopg.DB, file string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", file))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatal(err)
	}
	publishDirectionAuthorizationForCard(t, db, registration)
}

func publishDirectionAuthorization(ctx context.Context, db *bentopg.DB, registration domain.PriceCardRegistration) error {
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
	policyVersion, err := publishedCommercialVersion(tenant, pcdomain.PriceRuleObject, policyID, "v1", interval, nil)
	if err != nil {
		return err
	}
	authVersionValue, err := publishedCommercialVersion(tenant, pcdomain.AuthorizationRuleObject, authID, authVersion, interval,
		map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID{
			pcdomain.PriceRuleObject: mustCommercialID(policyID),
		})
	if err != nil {
		return err
	}
	policy, err := pcdomain.NewCommercialPricePolicy(policyVersion, direction, mustPlanRef(policyID), direction, pcdomain.PlanBindingConversionNone, policyVersion.Scope(), interval)
	if err != nil {
		return err
	}
	return db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := saveCommercialVersion(txCtx, publications, policyVersion); err != nil {
			return err
		}
		outcome, err := publications.SavePricePolicy(txCtx, policy, direction, pcdomain.PlanBindingConversionNone)
		if err != nil {
			return err
		}
		if outcome != pcports.PricePolicySaved && outcome != pcports.PricePolicyAlreadyRegistered {
			return fmt.Errorf("价格政策 %s", outcome)
		}
		return saveCommercialVersion(txCtx, publications, authVersionValue)
	})
}

func saveCommercialVersion(ctx context.Context, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion) error {
	outcome, err := publications.SaveVersion(ctx, version)
	if err != nil {
		return err
	}
	if outcome != pcports.PublicationSaved && outcome != pcports.PublicationAlreadyRegistered {
		return fmt.Errorf("发布 %s/%s：%s", version.ObjectID(), version.Version(), outcome)
	}
	return nil
}

func publishedCommercialVersion(
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

func mustCommercialID(raw string) pcdomain.CommercialObjectID {
	id, err := pcdomain.NewCommercialObjectID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

func mustPlanRef(raw string) pcdomain.PricingPlanReference {
	reference, err := pcdomain.NewPricingPlanReference(raw)
	if err != nil {
		panic(err)
	}
	return reference
}
