package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpartycommercial "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 psb/16 第 1 项——真 PC 解析库里的闭包，经 NewCommercialEligibility 按命令带来的解析标识取回。
// 网络服务译成要求，面单渠道服务译成不要求；空引用与租户不一致都不被读成未配置。
func TestCommercialEligibilityReadsAStoredClosureByTheCommandResolution(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	saveClosure(t, db.Transactor(), store, storedClosure(t, "RES-NET", pcdomain.NetworkServiceForm, "tenant-1"))
	saveClosure(t, db.Transactor(), store, storedClosure(t, "RES-LABEL", pcdomain.LabelChannelServiceForm, "tenant-1"))
	saveClosure(t, db.Transactor(), store, storedClosure(t, "RES-FOREIGN", pcdomain.NetworkServiceForm, "tenant-1"))
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.commercial_resolution
		    SET snapshot = jsonb_set(snapshot, '{tenantId}', '"tenant-other"')
		  WHERE resolution_id = 'RES-FOREIGN'`); err != nil {
		t.Fatalf("把闭包键改成另一个租户：%v", err)
	}

	view, err := nrpartycommercial.NewCommercialEligibility(store)
	if err != nil {
		t.Fatalf("构造资格视图：%v", err)
	}
	key := storedReachabilityKey(t, "tenant-1")

	required, err := view.AssessNetworkEligibility(t.Context(), key, mustNR(t, nrdomain.NewCommercialResolutionReference, "RES-NET"))
	if err != nil {
		t.Fatalf("要求：%v", err)
	}
	if !required.JudgmentRequired() {
		t.Fatal("真库里的网络服务被译成了不要求")
	}

	notRequired, err := view.AssessNetworkEligibility(t.Context(), key, mustNR(t, nrdomain.NewCommercialResolutionReference, "RES-LABEL"))
	if err != nil {
		t.Fatalf("不要求：%v", err)
	}
	if notRequired.JudgmentRequired() {
		t.Fatal("真库里的面单渠道服务被译成了要求")
	}

	if _, err := view.AssessNetworkEligibility(t.Context(), key, nrdomain.CommercialResolutionReference{}); !errors.Is(err, nrpartycommercial.ErrServiceProductUnavailable) {
		t.Fatalf("空引用 err = %v, want ErrServiceProductUnavailable", err)
	}
	if _, err := view.AssessNetworkEligibility(t.Context(), key, mustNR(t, nrdomain.NewCommercialResolutionReference, "RES-FOREIGN")); !errors.Is(err, nrpartycommercial.ErrClosureTenantMismatch) {
		t.Fatalf("租户不一致 err = %v, want ErrClosureTenantMismatch", err)
	}
}

func saveClosure(
	t *testing.T,
	transactor bentoapp.Transactor,
	store *pcpostgres.CommercialResolutions,
	closure pcdomain.CommercialClosure,
) {
	t.Helper()
	var outcome pcports.ResolutionSaveOutcome
	mustWithinTX(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = store.Save(txCtx, closure)
		return err
	})
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save %s = %s", closure.ResolutionID(), outcome)
	}
}

func storedClosure(t *testing.T, resolutionID string, form pcdomain.ServiceProductForm, tenant string) pcdomain.CommercialClosure {
	t.Helper()
	at := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(at, at.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "approval-"+resolutionID),
		mustPC(t, pcdomain.NewCommercialSourceReference, "source-"+resolutionID),
		at.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, tenant),
		Kind:          pcdomain.ServiceProductObject,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, "product-"+resolutionID),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:"+resolutionID),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   at.Add(-time.Hour),
		EffectiveAt:   at,
	})
	if err != nil {
		t.Fatalf("产品版本：%v", err)
	}
	product, err := pcdomain.NewServiceProduct(version, form)
	if err != nil {
		t.Fatalf("产品：%v", err)
	}
	anchor, err := pcdomain.NewSelectionAnchor(at.Add(24*time.Hour), mustPC(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("锚点：%v", err)
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: mustPC(t, pcdomain.NewResolutionID, resolutionID),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             mustPC(t, pcdomain.NewTenantID, tenant),
			CustomerAccountID:    mustPC(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: mustPC(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                mustPC(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.ServiceProductObject},
		},
		Anchor:       anchor,
		ViewRevision: mustPC(t, pcdomain.NewAuthorityViewRevision, "VIEW-"+resolutionID),
		Adopted: []pcdomain.RehydrateAdoptedBasisSpec{{
			Kind:              pcdomain.ServiceProductObject,
			Version:           product.Version(),
			ServiceProduct:    product,
			HasServiceProduct: true,
		}},
	})
	if err != nil {
		t.Fatalf("闭包：%v", err)
	}
	return closure
}

func storedReachabilityKey(t *testing.T, tenant string) nrdomain.ReachabilityJudgmentKey {
	t.Helper()
	asOf, err := nrdomain.NewJudgmentAsOf(
		mustNR(t, nrdomain.NewAsOfSemantic, "acceptance-as-of"),
		time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
		mustNR(t, nrdomain.NewAsOfStrategyVersion, "asof-v1"),
	)
	if err != nil {
		t.Fatalf("时点：%v", err)
	}
	return nrdomain.ReachabilityJudgmentKey{
		TenantID:          mustNR(t, nrdomain.NewTenantID, tenant),
		CustomerAccountID: mustNR(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: mustNR(t, nrdomain.NewShipmentRequestID, "shipment-1"),
		SubmissionVersion: mustNR(t, nrdomain.NewSubmissionVersionID, "sub-v1"),
		DeclaredParcelID:  mustNR(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:    mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
		AsOf:              asOf,
	}
}
