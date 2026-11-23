package partycommercial_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 同一客户先按旧合同接受，再发新一版合同换来源列表，再按落在新版区间里的发生时间接受第二份。
// 第一份仍是旧列表，第二份是新列表。解析键那一行的锚点没变。夹具 SYN-，证据 S。

var (
	acceptanceVersionStart = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	acceptanceRowAnchor    = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	acceptanceFirstAt      = time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	acceptanceCutAt        = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	acceptanceSecondAt     = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
)

func TestAcceptanceFixDoesNotFallBackToTheRegisteredAnchor(t *testing.T) {
	fixture := newBasisFixture(t, true)
	subject := adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Resolve:      pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: judgedAt}),
		Revalidate:   pcapplication.NewValidateCommercialBasisHandler(fixture.store, fixture.authority, fixedClock{at: judgedAt}),
		Judgments:    pcapplication.NewFormJudgmentAsOfHandler(fixture.store, fixture.policies),
		AsOfPolicies: fixture.policies,
		Contents:     fixture.contents,
		Keys:         fixture.keys,
		Values:       fixture.values,
	})
	query := fixture.resolveQuery(t)
	query.AcceptanceFix = true

	resolution, err := subject.ResolveCommercialBasis(context.Background(), query)
	if err != nil {
		t.Fatalf("解析：%v", err)
	}
	if resolution.Applicability != psdomain.CommercialApplicabilityUndetermined ||
		resolution.Reason.String() != "ACCEPTANCE_OCCURRED_AT_NOT_CONFIGURED" {
		t.Fatalf("applicability = %s reason = %s，want 未配置", resolution.Applicability, resolution.Reason)
	}
	if fixture.store.found {
		t.Fatal("发生时间没有拿到，却按账户行上的锚点落了一份解析")
	}
}

func TestALaterContractChangesTheSourceListOnlyForTheAcceptanceAtThatTime(t *testing.T) {
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
	arrangements, err := pcpostgres.NewContractSellPriceArrangements(db)
	if err != nil {
		t.Fatalf("销售价格约定：%v", err)
	}

	tenant := value(t, pcdomain.NewTenantID, "SYN-TENANT-01")
	customer := value(t, pcdomain.NewCustomerAccountID, "SYN-CUSTOMER-01")
	scope := value(t, pcdomain.NewCommercialScopeReference, "SYN-COMM-SCOPE")
	rules := acceptanceVersion(t, pcdomain.AcceptanceRulePackageObject, "SYN-RULES", "v1", "digest-rules", acceptanceVersionStart, time.Time{}, nil)
	product := acceptanceVersion(t, pcdomain.ServiceProductObject, "SYN-PRODUCT", "v1", "digest-product", acceptanceVersionStart, time.Time{}, nil)
	contractV1 := acceptanceVersion(t, pcdomain.CustomerContractObject, "SYN-CONTRACT", "v1", "digest-v1", acceptanceVersionStart, time.Time{}, nil)
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := saveAcceptanceVersion(txCtx, publications, rules); err != nil {
			return err
		}
		if err := saveAcceptanceVersion(txCtx, publications, product); err != nil {
			return err
		}
		if err := saveAcceptanceVersion(txCtx, publications, contractV1); err != nil {
			return err
		}
		if err := saveAcceptanceContract(txCtx, publications, contractV1, customer); err != nil {
			return err
		}
		return saveAcceptanceSources(txCtx, publications, contractV1, "SYN-SELL-OLD-1", "SYN-SELL-OLD-2")
	})
	mustRegisterKey(t, transactor, keys, acceptanceKey(t, tenant, customer, scope), adapter.ResolutionKeySaved)

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
	requests := &occurredRequestFinder{byKey: map[string]psdomain.ShipmentRequest{
		"SYN-REQ-1": acceptanceRequest(t, "SYN-REQ-1", "SYN-SHIP-1", "SYN-VER-1", acceptanceFirstAt),
		"SYN-REQ-2": acceptanceRequest(t, "SYN-REQ-2", "SYN-SHIP-2", "SYN-VER-2", acceptanceSecondAt),
	}}
	basis := adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Resolve:    pcapplication.NewResolveCommercialBasisHandler(authority, resolutions, fixedClock{at: judgedAt}),
		Revalidate: pcapplication.NewValidateCommercialBasisHandler(resolutions, authority, fixedClock{at: judgedAt}),
		Judgments: pcapplication.NewFormJudgmentAsOfHandler(resolutions, &asOfPolicyDouble{policies: []pcdomain.AsOfPolicy{
			providerPolicyFor(t, pcdomain.NetworkReachabilityJudgment),
			providerPolicyFor(t, pcdomain.PreAcceptanceFinancialControlJudgment),
		}}),
		AsOfPolicies: &asOfPolicyDouble{policies: []pcdomain.AsOfPolicy{
			providerPolicyFor(t, pcdomain.NetworkReachabilityJudgment),
			providerPolicyFor(t, pcdomain.PreAcceptanceFinancialControlJudgment),
		}},
		Contents: &contentDouble{content: content, contentFound: true, permission: permission, permissionFound: true},
		Keys:     keys,
		Values:   &valueSourceDouble{at: formedValueAt, formed: true},
		Requests: requests,
	})

	first := acceptCommercialBasis(t, transactor, basis, acceptanceQuery(t, "SYN-REQ-1", "SYN-SHIP-1", "SYN-VER-1"))
	firstSources := sourceListOf(t, ctx, resolutions, arrangements, tenant, first.Snapshot.ResolutionID().String())
	if firstSources != "SYN-SELL-OLD-1,SYN-SELL-OLD-2" {
		t.Fatalf("第一份来源 = %s，want 旧列表", firstSources)
	}
	if got := anchorOf(t, ctx, resolutions, tenant, first.Snapshot.ResolutionID().String()); !got.Equal(acceptanceFirstAt) {
		t.Fatalf("第一份锚点时刻 = %s，want 该版来源发生时间 %s", got, acceptanceFirstAt)
	}

	contractV2 := acceptanceVersion(t, pcdomain.CustomerContractObject, "SYN-CONTRACT", "v2", "digest-v2", acceptanceCutAt, time.Time{}, nil)
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if err := saveAcceptanceVersion(txCtx, publications, contractV2); err != nil {
			return err
		}
		if err := saveAcceptanceContract(txCtx, publications, contractV2, customer); err != nil {
			return err
		}
		return saveAcceptanceSources(txCtx, publications, contractV2, "SYN-SELL-NEW")
	})
	mustWithinKeyTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		registry, err := publications.LoadForScope(txCtx, tenant, scope)
		if err != nil {
			return err
		}
		correction, replay, err := registry.CutAtSuccessorStart(
			tenant, scope, pcdomain.CustomerContractObject,
			value(t, pcdomain.NewCommercialObjectID, "SYN-CONTRACT"),
			value(t, pcdomain.NewCommercialVersionLabel, "v1"),
			value(t, pcdomain.NewCommercialVersionLabel, "v2"),
			value(t, pcdomain.NewValidityCorrectionReference, "SYN-CUT-1"),
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

	second := acceptCommercialBasis(t, transactor, basis, acceptanceQuery(t, "SYN-REQ-2", "SYN-SHIP-2", "SYN-VER-2"))
	if second.Snapshot.ResolutionID() == first.Snapshot.ResolutionID() {
		t.Fatal("两份接受落成了同一次解析")
	}
	if got := sourceListOf(t, ctx, resolutions, arrangements, tenant, second.Snapshot.ResolutionID().String()); got != "SYN-SELL-NEW" {
		t.Fatalf("第二份来源 = %s，want 新列表", got)
	}
	if got := sourceListOf(t, ctx, resolutions, arrangements, tenant, first.Snapshot.ResolutionID().String()); got != "SYN-SELL-OLD-1,SYN-SELL-OLD-2" {
		t.Fatalf("新合同之后第一份来源 = %s，want 仍是旧列表", got)
	}
	if got := anchorOf(t, ctx, resolutions, tenant, second.Snapshot.ResolutionID().String()); !got.Equal(acceptanceSecondAt) {
		t.Fatalf("第二份锚点时刻 = %s，want 该版来源发生时间 %s", got, acceptanceSecondAt)
	}

	row, found, err := keyStore.FindResolutionKey(ctx, tenant.String(), customer.String())
	if err != nil || !found {
		t.Fatalf("读解析键：found=%v err=%v", found, err)
	}
	if !row.AnchorAt.Equal(acceptanceRowAnchor) || row.AnchorPolicy != "SYN-ANCHOR-POLICY-1" {
		t.Fatalf("解析键锚点 = %s policy = %s，登记行被改了", row.AnchorAt, row.AnchorPolicy)
	}

	control := acceptanceQuery(t, "SYN-REQ-1", "SYN-SHIP-1", "SYN-VER-1")
	control.AcceptanceFix = false
	held := acceptCommercialBasis(t, transactor, basis, control)
	if got := anchorOf(t, ctx, resolutions, tenant, held.Snapshot.ResolutionID().String()); !got.Equal(acceptanceRowAnchor) {
		t.Fatalf("不标成接受固定时锚点 = %s，want 账户行上的 %s", got, acceptanceRowAnchor)
	}
	if got := sourceListOf(t, ctx, resolutions, arrangements, tenant, held.Snapshot.ResolutionID().String()); got != "SYN-SELL-OLD-1,SYN-SELL-OLD-2" {
		t.Fatalf("接受前这条路的来源 = %s，want 仍按账户行时刻选旧列表", got)
	}
}

var (
	errReplayCut   = errString("截止被认成重放")
	errCutNotSaved = errString("截止没有入册")
)

type errString string

func (err errString) Error() string { return string(err) }

func acceptCommercialBasis(
	t *testing.T,
	transactor bentoapp.Transactor,
	basis *adapter.CommercialBasisAdapter,
	query psports.CommercialBasisQuery,
) psports.CommercialBasisResolution {
	t.Helper()
	var resolution psports.CommercialBasisResolution
	mustWithinKeyTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		resolution, err = basis.ResolveCommercialBasis(txCtx, query)
		return err
	})
	if resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("接受解析 applicability = %s reason = %s", resolution.Applicability, resolution.Reason)
	}
	return resolution
}

func sourceListOf(
	t *testing.T,
	ctx context.Context,
	resolutions *pcpostgres.CommercialResolutions,
	arrangements *pcpostgres.ContractSellPriceArrangements,
	tenant pcdomain.TenantID,
	resolutionID string,
) string {
	t.Helper()
	closure := loadFixedClosure(t, ctx, resolutions, tenant, resolutionID)
	adopted, ok := closure.AdoptedFor(pcdomain.CustomerContractObject)
	if !ok {
		t.Fatal("固定解析没有采用客户合同")
	}
	arrangement, found, err := arrangements.LoadContractSellPriceArrangement(ctx, tenant, adopted.Version())
	if err != nil || !found {
		t.Fatalf("读来源列表：found=%v err=%v", found, err)
	}
	sources, ordered := arrangement.Sources()
	if !ordered {
		t.Fatal("有序列表被读成别的答复")
	}
	joined := ""
	for index, source := range sources {
		if index > 0 {
			joined += ","
		}
		joined += source.String()
	}
	return joined
}

func anchorOf(
	t *testing.T,
	ctx context.Context,
	resolutions *pcpostgres.CommercialResolutions,
	tenant pcdomain.TenantID,
	resolutionID string,
) time.Time {
	t.Helper()
	return loadFixedClosure(t, ctx, resolutions, tenant, resolutionID).Anchor().At()
}

func loadFixedClosure(
	t *testing.T,
	ctx context.Context,
	resolutions *pcpostgres.CommercialResolutions,
	tenant pcdomain.TenantID,
	resolutionID string,
) pcdomain.CommercialClosure {
	t.Helper()
	id, err := pcdomain.NewResolutionID(resolutionID)
	if err != nil {
		t.Fatalf("解析标识：%v", err)
	}
	closure, found, err := resolutions.LoadResolution(ctx, tenant, id)
	if err != nil || !found {
		t.Fatalf("读回固定解析 %s：found=%v err=%v", resolutionID, found, err)
	}
	return closure
}

func acceptanceKey(
	t *testing.T,
	tenant pcdomain.TenantID,
	customer pcdomain.CustomerAccountID,
	scope pcdomain.CommercialScopeReference,
) adapter.ResolutionKeyRegistration {
	t.Helper()
	return adapter.ResolutionKeyRegistration{
		TenantID:          psdomain.TenantID(mustPS(t, psdomain.NewTenantID, tenant.String())),
		CustomerAccountID: psdomain.CustomerAccountID(mustPS(t, psdomain.NewCustomerAccountID, customer.String())),
		Scope:             scope,
		LegalEntity:       value(t, pcdomain.NewLegalEntityReference, "SYN-LEGAL-01"),
		AnchorPolicy:      value(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"),
		AnchorAt:          acceptanceRowAnchor,
		RequiredBases: []pcdomain.CommercialObjectKind{
			pcdomain.CustomerContractObject,
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.ServiceProductObject,
		},
	}
}

func mustPS[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", built, raw, err)
	}
	return built
}

func acceptanceQuery(t *testing.T, requestKey, shipment, version string) psports.CommercialBasisQuery {
	t.Helper()
	return psports.CommercialBasisQuery{
		Identity:          acceptanceIdentity(t, requestKey),
		ShipmentRequestID: mustPS(t, psdomain.NewShipmentRequestID, shipment),
		SubmissionVersion: mustPS(t, psdomain.NewSubmissionVersionID, version),
		AcceptanceFix:     true,
	}
}

func acceptanceIdentity(t *testing.T, requestKey string) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		mustPS(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		mustPS(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-01"),
		mustPS(t, psdomain.NewSource, "SYN-PORTAL"),
		mustPS(t, psdomain.NewSourceRequestKey, requestKey),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	return identity
}

func acceptanceRequest(t *testing.T, requestKey, shipment, version string, occurredAt time.Time) psdomain.ShipmentRequest {
	t.Helper()
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		acceptanceIdentity(t, requestKey),
		mustPS(t, psdomain.NewPayloadDigest, "digest-"+requestKey),
		occurredAt,
		occurredAt.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("来源指纹：%v", err)
	}
	candidate, err := psdomain.NewSubmissionCandidate(
		fingerprint,
		mustPS(t, psdomain.NewSubmissionBatchID, "SYN-BATCH-"+requestKey),
		mustPS(t, psdomain.NewShipmentRequestID, shipment),
		[]psdomain.DeclaredParcelID{mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-"+requestKey)},
	)
	if err != nil {
		t.Fatalf("提交候选：%v", err)
	}
	admission, err := psdomain.NewAdmissionScope(
		mustPS(t, psdomain.NewAdmissionScopeReference, "SYN-ADMISSION"),
		mustPS(t, psdomain.NewAdmissionScopeDigest, "SYN-ADMISSION-DIGEST"),
	)
	if err != nil {
		t.Fatalf("受理范围：%v", err)
	}
	validity, err := psdomain.NewOwnershipValidityInterval(occurredAt.Add(-48*time.Hour), occurredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("归属区间：%v", err)
	}
	ownership, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustPS(t, psdomain.NewProductionOwnershipDecisionID, "SYN-OWN-"+requestKey),
		Scope:            admission,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      mustPS(t, psdomain.NewProductionOwnershipRuleVersion, "SYN-RULE"),
		AsOf:             occurredAt,
		Validity:         validity,
		Revision:         mustPS(t, psdomain.NewProductionOwnershipRevision, "SYN-REV-1"),
		DecisionAt:       occurredAt,
	})
	if err != nil {
		t.Fatalf("生产归属：%v", err)
	}
	gate, err := psdomain.EvaluateFutureSubmissionGate(ownership, admission.Digest(),
		mustPS(t, psdomain.NewProductionOwnershipRevision, "SYN-REV-1"), occurredAt)
	if err != nil {
		t.Fatalf("门禁：%v", err)
	}
	request, err := psdomain.SubmitShipmentRequest(psdomain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustPS(t, psdomain.NewSubmissionVersionID, version),
		TaskID:      mustPS(t, psdomain.NewAcceptanceDecisionTaskID, "SYN-TASK-"+requestKey),
		SubmittedAt: occurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("提交委托：%v", err)
	}
	return request
}

type occurredRequestFinder struct {
	byKey map[string]psdomain.ShipmentRequest
}

func (finder *occurredRequestFinder) FindBySourceIdentity(
	_ context.Context,
	identity psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	request, found := finder.byKey[identity.RequestKey().String()]
	return request, found, nil
}

func acceptanceVersion(
	t *testing.T,
	kind pcdomain.CommercialObjectKind,
	objectID, label, digest string,
	starts, ends time.Time,
	references map[pcdomain.CommercialObjectKind]pcdomain.CommercialObjectID,
) pcdomain.CommercialVersion {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(starts, ends)
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID+"-"+label),
		value(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID),
		starts.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("批准：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "SYN-TENANT-01"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, label),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, "SYN-COMM-SCOPE"),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   starts.Add(-time.Minute),
		EffectiveAt:   starts,
		References:    references,
	})
	if err != nil {
		t.Fatalf("重建 %s %s：%v", objectID, label, err)
	}
	return version
}

func saveAcceptanceVersion(ctx context.Context, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion) error {
	outcome, err := publications.SaveVersion(ctx, version)
	if err != nil {
		return err
	}
	if outcome != pcports.PublicationSaved {
		return errString("版本没有入册")
	}
	return nil
}

func saveAcceptanceContract(
	ctx context.Context,
	publications *pcpostgres.CommercialPublications,
	version pcdomain.CommercialVersion,
	customer pcdomain.CustomerAccountID,
) error {
	content, err := pcdomain.NewCustomerContract(version, customer, pcdomain.CommercialObjectID{}, nil)
	if err != nil {
		return err
	}
	outcome, err := publications.SaveCustomerContractContent(ctx, content)
	if err != nil {
		return err
	}
	if outcome != pcports.DeclarationSaved {
		return errString("合同正文没有入册")
	}
	return nil
}

func saveAcceptanceSources(ctx context.Context, publications *pcpostgres.CommercialPublications, version pcdomain.CommercialVersion, scopes ...string) error {
	sources := make([]pcdomain.PricingScopeReference, 0, len(scopes))
	for _, scope := range scopes {
		reference, err := pcdomain.NewPricingScopeReference(scope)
		if err != nil {
			return err
		}
		sources = append(sources, reference)
	}
	arrangement, err := pcdomain.DeclareContractSellPriceArrangement(version, sources, pcdomain.NoSellPriceBasis{})
	if err != nil {
		return err
	}
	outcome, err := publications.SaveContractSellPriceArrangement(ctx, arrangement)
	if err != nil {
		return err
	}
	if outcome != pcports.DeclarationSaved {
		return errString("来源列表没有入册")
	}
	return nil
}
