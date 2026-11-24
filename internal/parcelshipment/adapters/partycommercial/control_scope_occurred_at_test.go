package partycommercial_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 同一客户先按账户行上的旧锚点把旧合同与旧结算政策固定下来，再发新版。这一版来源发生时间落在新版区间。
// formAdoptedBasis 交给解析口的那一格（CommercialBasisAdapter 且 AcceptanceFix）与接受前财务控制的作用域
// 必须交回同一次解析，合同与结算政策都是新版。账户行上的锚点时刻不变。夹具 SYN-，证据 S。
func TestControlScopeFixesTheSameCommercialResolutionAsThisAcceptance(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	transactor := db.Transactor()
	ctx := t.Context()

	keyStore, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatalf("解析键库：%v", err)
	}
	keys, err := adapter.NewCommercialResolutionKeys(keyStore)
	if err != nil {
		t.Fatalf("解析键：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("发布册：%v", err)
	}
	authority, err := pcpostgres.NewCommercialAuthority(publications)
	if err != nil {
		t.Fatalf("权威：%v", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatalf("解析库：%v", err)
	}

	tenant := value(t, pcdomain.NewTenantID, "SYN-TENANT-01")
	customer := value(t, pcdomain.NewCustomerAccountID, "SYN-CUSTOMER-01")
	commercialScope := value(t, pcdomain.NewCommercialScopeReference, "SYN-COMM-SCOPE")
	rules := acceptanceVersion(t, pcdomain.AcceptanceRulePackageObject, "SYN-RULES", "v1", "digest-rules", acceptanceVersionStart, time.Time{}, nil)
	product := acceptanceVersion(t, pcdomain.ServiceProductObject, "SYN-PRODUCT", "v1", "digest-product", acceptanceVersionStart, time.Time{}, nil)
	contractV1 := acceptanceVersion(t, pcdomain.CustomerContractObject, "SYN-CONTRACT", "v1", "digest-v1", acceptanceVersionStart, time.Time{}, nil)
	settleV1 := acceptanceVersion(t, pcdomain.SettlementPolicyObject, "SYN-SETTLE", "v1", "digest-settle-v1", acceptanceVersionStart, time.Time{}, nil)
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		for _, version := range []pcdomain.CommercialVersion{rules, product, contractV1, settleV1} {
			if err := saveAcceptanceVersion(txCtx, publications, version); err != nil {
				return err
			}
		}
		if err := saveAcceptanceContract(txCtx, publications, contractV1, customer); err != nil {
			return err
		}
		return saveAcceptanceSettlementPolicy(txCtx, publications, acceptanceSettlementPolicy(t, settleV1, "v1", acceptanceVersionStart, time.Time{}))
	})
	mustRegisterKey(t, transactor, keys, acceptanceSettlementKey(t, tenant, customer, commercialScope), adapter.ResolutionKeySaved)

	content, err := pcdomain.DeclareAcceptanceRuleContent(rules, []pcdomain.AcceptanceCheckGroupType{
		pcdomain.NetworkReachabilityCheckGroup,
		pcdomain.PreAcceptanceFinancialControlCheckGroup,
	}, pcdomain.ManualReviewNotRequired)
	if err != nil {
		t.Fatalf("规则正文：%v", err)
	}
	permission, err := pcdomain.DeclarePendingRoutingPermission(product, value(t, pcdomain.NewPendingRoutingBasisReference, "SYN-PENDING-1"))
	if err != nil {
		t.Fatalf("待路由许可：%v", err)
	}
	const (
		requestKey = "SYN-REQ-SCOPE"
		shipment   = "SYN-SHIP-SCOPE"
		versionID  = "SYN-VER-SCOPE"
	)
	requests := &occurredRequestFinder{byKey: map[string]psdomain.ShipmentRequest{
		requestKey: acceptanceRequest(t, requestKey, shipment, versionID, acceptanceSecondAt),
	}}
	asOf := &asOfPolicyDouble{policies: []pcdomain.AsOfPolicy{
		providerPolicyFor(t, pcdomain.NetworkReachabilityJudgment),
		providerPolicyFor(t, pcdomain.PreAcceptanceFinancialControlJudgment),
	}}
	basis := adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Resolve:      pcapplication.NewResolveCommercialBasisHandler(authority, resolutions, fixedClock{at: judgedAt}),
		Revalidate:   pcapplication.NewValidateCommercialBasisHandler(resolutions, authority, fixedClock{at: judgedAt}),
		Judgments:    pcapplication.NewFormJudgmentAsOfHandler(resolutions, asOf),
		AsOfPolicies: asOf,
		Contents:     &contentDouble{content: content, contentFound: true, permission: permission, permissionFound: true},
		Keys:         keys,
		Values:       &valueSourceDouble{at: formedValueAt, formed: true},
		Requests:     requests,
	})

	heldQuery := acceptanceQuery(t, requestKey, shipment, versionID)
	heldQuery.AcceptanceFix = false
	held := acceptCommercialBasis(t, transactor, basis, heldQuery)
	if contract, settlement := acceptanceAdoptedLabels(t, ctx, resolutions, tenant, held.Snapshot.ResolutionID().String()); contract != "SYN-CONTRACT/v1" || settlement != "SYN-SETTLE/v1" {
		t.Fatalf("账户行锚点落下的合同/结算政策 = %s/%s，want SYN-CONTRACT/v1 与 SYN-SETTLE/v1", contract, settlement)
	}
	if got := anchorOf(t, ctx, resolutions, tenant, held.Snapshot.ResolutionID().String()); !got.Equal(acceptanceRowAnchor) {
		t.Fatalf("账户行锚点落下的时刻 = %s，want %s", got, acceptanceRowAnchor)
	}

	contractV2 := acceptanceVersion(t, pcdomain.CustomerContractObject, "SYN-CONTRACT", "v2", "digest-v2", acceptanceCutAt, time.Time{}, nil)
	settleV2 := acceptanceVersion(t, pcdomain.SettlementPolicyObject, "SYN-SETTLE", "v2", "digest-settle-v2", acceptanceCutAt, time.Time{}, nil)
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := saveAcceptanceVersion(txCtx, publications, contractV2); err != nil {
			return err
		}
		if err := saveAcceptanceVersion(txCtx, publications, settleV2); err != nil {
			return err
		}
		if err := saveAcceptanceContract(txCtx, publications, contractV2, customer); err != nil {
			return err
		}
		return saveAcceptanceSettlementPolicy(txCtx, publications, acceptanceSettlementPolicy(t, settleV2, "v2", acceptanceCutAt, time.Time{}))
	})
	cutAcceptanceSuccessor(t, transactor, publications, pcdomain.CustomerContractObject, "SYN-CONTRACT", "v1", "v2", "SYN-CUT-CONTRACT")
	cutAcceptanceSuccessor(t, transactor, publications, pcdomain.SettlementPolicyObject, "SYN-SETTLE", "v1", "v2", "SYN-CUT-SETTLE")

	fixed := acceptCommercialBasis(t, transactor, basis, acceptanceQuery(t, requestKey, shipment, versionID))
	if fixed.Snapshot.ResolutionID() == held.Snapshot.ResolutionID() {
		t.Fatal("新版区间上的接受与账户行旧锚点落成了同一次解析")
	}
	account := mustPS(t, sadomain.NewSettlementAccountID, "SYN-ACCOUNT-01")
	source := pssettlement.NewPolicyBackedControlScopeSource(basis, acceptanceAccountDirectory{account: account})
	var controlScope pssettlement.ControlScope
	var gap pssettlement.ControlScopeGap
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var formErr error
		controlScope, gap, formErr = source.FormControlScope(txCtx, acceptanceIdentity(t, requestKey),
			mustPS(t, psdomain.NewShipmentRequestID, shipment),
			mustPS(t, psdomain.NewSubmissionVersionID, versionID))
		return formErr
	})
	if gap != pssettlement.ControlScopeFormed {
		t.Fatalf("作用域 gap = %d，want 形成——接受前财务控制没有用上这次接受的解析", gap)
	}
	if controlScope.Resolution.String() != fixed.Snapshot.ResolutionID().String() {
		t.Fatalf("作用域解析 = %s，接受解析 = %s，两路分裂成了两次固定",
			controlScope.Resolution, fixed.Snapshot.ResolutionID())
	}
	contract, settlement := acceptanceAdoptedLabels(t, ctx, resolutions, tenant, controlScope.Resolution.String())
	if contract != "SYN-CONTRACT/v2" || settlement != "SYN-SETTLE/v2" {
		t.Fatalf("共用解析的合同/结算政策 = %s/%s，want SYN-CONTRACT/v2 与 SYN-SETTLE/v2", contract, settlement)
	}
	terms, present := fixed.Snapshot.SettlementTerms()
	if !present || terms.Policy().String() != "SYN-SETTLE/v2" {
		t.Fatalf("接受快照上的结算政策 = %s（在场=%v），want SYN-SETTLE/v2", terms.Policy(), present)
	}
	if got := anchorOf(t, ctx, resolutions, tenant, controlScope.Resolution.String()); !got.Equal(acceptanceSecondAt) {
		t.Fatalf("共用解析的锚点时刻 = %s，want 该版来源发生时间 %s", got, acceptanceSecondAt)
	}
	if contract, settlement := acceptanceAdoptedLabels(t, ctx, resolutions, tenant, held.Snapshot.ResolutionID().String()); contract != "SYN-CONTRACT/v1" || settlement != "SYN-SETTLE/v1" {
		t.Fatalf("新版之后旧解析的合同/结算政策 = %s/%s，want 仍是 v1", contract, settlement)
	}

	row, found, err := keyStore.FindResolutionKey(ctx, tenant.String(), customer.String())
	if err != nil || !found {
		t.Fatalf("读解析键：found=%v err=%v", found, err)
	}
	if !row.AnchorAt.Equal(acceptanceRowAnchor) || row.AnchorPolicy != "SYN-ANCHOR-POLICY-1" {
		t.Fatalf("解析键锚点 = %s policy = %s，登记行被改了", row.AnchorAt, row.AnchorPolicy)
	}
}

func acceptanceSettlementKey(
	t *testing.T,
	tenant pcdomain.TenantID,
	customer pcdomain.CustomerAccountID,
	scope pcdomain.CommercialScopeReference,
) adapter.ResolutionKeyRegistration {
	t.Helper()
	registration := acceptanceKey(t, tenant, customer, scope)
	registration.RequiredBases = append(registration.RequiredBases, pcdomain.SettlementPolicyObject)
	registration.SettlementCounterparty = value(t, pcdomain.NewCounterpartyReference, customer.String())
	registration.SettlementChargeScope = value(t, pcdomain.NewChargeScopeReference, "SYN-CHARGE-01")
	registration.SettlementCurrency = value(t, pcdomain.NewCurrencyCode, "CNY")
	return registration
}

func acceptanceSettlementPolicy(
	t *testing.T,
	version pcdomain.CommercialVersion,
	contractLabel string,
	starts, ends time.Time,
) pcdomain.SettlementPolicy {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(starts, ends)
	if err != nil {
		t.Fatalf("结算区间：%v", err)
	}
	contract, err := pcdomain.NewQualifiedVersionLabel(
		value(t, pcdomain.NewCommercialObjectID, "SYN-CONTRACT"),
		value(t, pcdomain.NewCommercialVersionLabel, contractLabel),
	)
	if err != nil {
		t.Fatalf("合同指称：%v", err)
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		value(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-01"),
		value(t, pcdomain.NewCounterpartyReference, "SYN-CUSTOMER-01"),
		contract,
		value(t, pcdomain.NewChargeScopeReference, "SYN-CHARGE-01"),
		value(t, pcdomain.NewCurrencyCode, "CNY"),
		interval,
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := pcdomain.NewSettlementPolicy(version, pcdomain.TermsMethod, applicability)
	if err != nil {
		t.Fatalf("结算政策：%v", err)
	}
	return policy
}

func saveAcceptanceSettlementPolicy(
	ctx context.Context,
	publications *pcpostgres.CommercialPublications,
	policy pcdomain.SettlementPolicy,
) error {
	outcome, err := publications.SaveSettlementPolicy(ctx, policy)
	if err != nil {
		return err
	}
	if outcome != pcports.SettlementPolicySaved {
		return errString("结算政策没有入册")
	}
	return nil
}

func cutAcceptanceSuccessor(
	t *testing.T,
	transactor bentoapp.Transactor,
	publications *pcpostgres.CommercialPublications,
	kind pcdomain.CommercialObjectKind,
	objectID, predecessor, successor, reference string,
) {
	t.Helper()
	mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		tenant := value(t, pcdomain.NewTenantID, "SYN-TENANT-01")
		scope := value(t, pcdomain.NewCommercialScopeReference, "SYN-COMM-SCOPE")
		registry, err := publications.LoadForScope(txCtx, tenant, scope)
		if err != nil {
			return err
		}
		correction, replay, err := registry.CutAtSuccessorStart(
			tenant, scope, kind,
			value(t, pcdomain.NewCommercialObjectID, objectID),
			value(t, pcdomain.NewCommercialVersionLabel, predecessor),
			value(t, pcdomain.NewCommercialVersionLabel, successor),
			value(t, pcdomain.NewValidityCorrectionReference, reference),
			acceptanceCutAt.Add(time.Hour),
		)
		if err != nil {
			return err
		}
		if replay {
			return errReplayCut
		}
		outcome, err := publications.SaveValidityCorrection(txCtx, correction)
		if err != nil {
			return err
		}
		if outcome != pcports.ValidityCorrectionSaved {
			return errCutNotSaved
		}
		return nil
	})
}

func acceptanceAdoptedLabels(
	t *testing.T,
	ctx context.Context,
	resolutions *pcpostgres.CommercialResolutions,
	tenant pcdomain.TenantID,
	resolutionID string,
) (string, string) {
	t.Helper()
	closure := loadFixedClosure(t, ctx, resolutions, tenant, resolutionID)
	contractBasis, ok := closure.AdoptedFor(pcdomain.CustomerContractObject)
	if !ok {
		t.Fatal("固定解析没有采用客户合同")
	}
	settlementBasis, ok := closure.AdoptedFor(pcdomain.SettlementPolicyObject)
	if !ok {
		t.Fatal("固定解析没有采用结算政策")
	}
	policy, present := settlementBasis.SettlementPolicy()
	if !present {
		t.Fatal("结算依据没有带上政策正文")
	}
	return contractBasis.Version().QualifiedLabel().String(), policy.Version().QualifiedLabel().String()
}

type acceptanceAccountDirectory struct {
	account sadomain.SettlementAccountID
}

func (directory acceptanceAccountDirectory) FindSettlementAccount(
	context.Context,
	psdomain.SourceIdentity,
	psdomain.AdoptedSettlementTerms,
) (sadomain.SettlementAccountID, bool, error) {
	return directory.account, true, nil
}
