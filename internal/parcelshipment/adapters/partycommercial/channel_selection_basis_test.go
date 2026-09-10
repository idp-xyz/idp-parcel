package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证择优结果 → 面单交易七类依据引用的翻译（票 `label-channel/29`）。翻译自己不判候选该不该赢、不复制
// 授权与协议的任何规则——它只回答「选中的这个候选，建立面单交易要的七格各是什么」，答不出的格具名停下。
//
// 「这次该用哪一条授权、哪一版协议、哪一次接受时解析」三问是消费方的实例半边（裁决 (1)）：三个源在测试里
// 都是替身，替身答「未配置」时翻译停在各自具名的那一格，不代拟。夹具全部合成。

// 翻译器就是编排要的那个口。
var _ psports.ChannelSelectionBasisTranslator = (*adapter.ChannelSelectionBasisTranslator)(nil)

const (
	basisCandidate     = "channel-a"
	basisAuthorization = "authorization-1"
	basisAccount       = "account-1"
	basisHolder        = "party-holder"
	basisOperator      = "party-operator"
	basisSupplier      = "party-supplier"
	basisAgreement     = "agreement-1"
	basisResolution    = "resolution-1"
)

type stubAccountUseSource struct {
	selection adapter.ChannelAccountUseSelection
	found     bool
	err       error
}

func (stub stubAccountUseSource) AuthorizationFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
	_ psdomain.ChannelCandidateID,
) (adapter.ChannelAccountUseSelection, bool, error) {
	return stub.selection, stub.found, stub.err
}

type stubAgreementSource struct {
	version pcdomain.CommercialVersion
	found   bool
	err     error
}

func (stub stubAgreementSource) AgreementFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
	_ psdomain.ChannelCandidateID,
) (pcdomain.CommercialVersion, bool, error) {
	return stub.version, stub.found, stub.err
}

type stubResolutionSource struct {
	resolution psdomain.CommercialResolutionID
	found      bool
	err        error
}

func (stub stubResolutionSource) ResolutionFor(
	_ context.Context,
	_ psports.ChannelSelectionQuery,
) (psdomain.CommercialResolutionID, bool, error) {
	return stub.resolution, stub.found, stub.err
}

type stubAuthorizations struct {
	registrations map[string]pcdomain.ChannelAccountUseAuthorizationRegistration
}

func (stub stubAuthorizations) LoadLatest(
	_ context.Context,
	_ pcdomain.TenantID,
	id pcdomain.ChannelAccountUseAuthorizationID,
) (pcdomain.ChannelAccountUseAuthorizationRegistration, bool, error) {
	registration, found := stub.registrations[id.String()]
	return registration, found, nil
}

type stubAgreements struct {
	agreements map[string]pcdomain.SupplierAgreement
}

func (stub stubAgreements) LoadSupplierAgreement(
	_ context.Context,
	_ pcdomain.TenantID,
	version pcdomain.CommercialVersion,
) (pcdomain.SupplierAgreement, bool, error) {
	agreement, found := stub.agreements[version.ObjectID().String()+"/"+version.Version().String()]
	return agreement, found, nil
}

func selectionBasisQuery(t testing.TB) psports.ChannelSelectionQuery {
	t.Helper()
	return psports.ChannelSelectionQuery{
		Tenant:  assemblyValue(t, psdomain.NewTenantID, assemblyTenant),
		Scope:   assemblyValue(t, psdomain.NewCommercialScopeReference, assemblyScope),
		Mapping: assemblyValue(t, psdomain.NewProductChannelMappingReference, assemblyMapping),
		At:      assemblyAt(t),
	}
}

// pricedSelection 造择优步交出的赢家：候选 + 评价痕迹 + 费率。
func pricedSelection(t testing.TB) psdomain.SelectedChannelCandidate {
	t.Helper()
	selected, err := psdomain.NewSelectedChannelCandidate(assemblyValue(t, psdomain.NewChannelCandidateID, basisCandidate))
	if err != nil {
		t.Fatalf("造选中候选：%v", err)
	}
	if selected, err = selected.WithEvaluation(assemblyValue(t, psdomain.NewChannelCostEvaluationReference, "eval-1")); err != nil {
		t.Fatalf("带评价：%v", err)
	}
	if selected, err = selected.WithRate(assemblyValue(t, psdomain.NewChannelRateReference, "buy-plan-1/v1")); err != nil {
		t.Fatalf("带费率：%v", err)
	}
	return selected
}

// publishedAuthorization 造一条已发布的账号使用授权登记：账号 → 持有人授给运营企业、盖 channel 与 scope、
// 有效区间盖住 assemblyAt。走 PublishChannelAccountUseAuthorization 那扇门而不是拼值：业务授权未确认就
// 发布不出来，那条不变量正是翻译要依赖的。
func publishedAuthorization(t testing.TB, channel, grantee, scope string, effective pcdomain.EffectiveInterval) pcdomain.ChannelAccountUseAuthorizationRegistration {
	t.Helper()
	authorization, err := pcdomain.PublishChannelAccountUseAuthorization(
		assemblyValue(t, pcdomain.NewChannelAccountID, basisAccount),
		assemblyValue(t, pcdomain.NewPartyID, basisHolder),
		assemblyValue(t, pcdomain.NewPartyID, grantee),
		assemblyValue(t, pcdomain.NewChannelProductReference, channel),
		assemblyValue(t, pcdomain.NewCommercialScopeReference, scope),
		effective,
		pcdomain.ChannelAccountTechnicallyAvailable,
		pcdomain.ChannelAccountBusinessAuthorized,
		time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("发布账号使用授权：%v", err)
	}
	return registrationOf(t, authorization)
}

func registrationOf(t testing.TB, authorization pcdomain.ChannelAccountUseAuthorization) pcdomain.ChannelAccountUseAuthorizationRegistration {
	t.Helper()
	registration, err := pcdomain.NewChannelAccountUseAuthorizationRegistration(
		assemblyValue(t, pcdomain.NewTenantID, assemblyTenant),
		assemblyValue(t, pcdomain.NewChannelAccountUseAuthorizationID, basisAuthorization),
		1,
		authorization,
	)
	if err != nil {
		t.Fatalf("登记账号使用授权：%v", err)
	}
	return registration
}

func year2026Interval(t testing.TB) pcdomain.EffectiveInterval {
	t.Helper()
	return intervalOf(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
}

func intervalOf(t testing.TB, from, until time.Time) pcdomain.EffectiveInterval {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(from, until)
	if err != nil {
		t.Fatalf("构造生效区间：%v", err)
	}
	return interval
}

// effectiveSupplierAgreement 造一份已生效的供应商协议版本（草稿 → 发布 → 生效三步，同 effectiveLabelChannelProduct
// 的理由）与它的正文：供应商 party-supplier、运营企业法人 legal-1、BUY 方案 buy-plan-1。
func effectiveSupplierAgreement(t testing.TB, effective pcdomain.EffectiveInterval) pcdomain.SupplierAgreement {
	t.Helper()
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      assemblyValue(t, pcdomain.NewTenantID, assemblyTenant),
		Kind:          pcdomain.SupplierAgreementObject,
		ObjectID:      assemblyValue(t, pcdomain.NewCommercialObjectID, basisAgreement),
		Version:       assemblyValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         assemblyValue(t, pcdomain.NewCommercialScopeReference, assemblyScope),
		ContentDigest: assemblyValue(t, pcdomain.NewCommercialContentDigest, "sha256:syn-agreement-v1"),
		Effective:     effective,
	})
	if err != nil {
		t.Fatalf("构造协议草稿：%v", err)
	}
	basis, err := pcdomain.NewApprovalBasis(
		assemblyValue(t, pcdomain.NewApprovalReference, "approval-agreement-v1"),
		assemblyValue(t, pcdomain.NewCommercialSourceReference, "syn-source"),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造批准依据：%v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatalf("发布协议：%v", err)
	}
	live, err := published.TakeEffect(time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("协议生效：%v", err)
	}
	agreement, err := pcdomain.NewSupplierAgreement(
		live,
		assemblyValue(t, pcdomain.NewPartyID, basisSupplier),
		assemblyValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		assemblyValue(t, pcdomain.NewCommercialScopeReference, assemblyScope),
		assemblyValue(t, pcdomain.NewPricingPlanReference, "buy-plan-1"),
		effective,
	)
	if err != nil {
		t.Fatalf("构造供应商协议：%v", err)
	}
	return agreement
}

// selectionFixture 是一套三源全配上、授权与协议都在册且都盖住 assemblyAt 的正路夹具；各条停点测试从它出发
// 只改一格。
type selectionFixture struct {
	accounts    stubAccountUseSource
	agreements  stubAgreementSource
	resolutions stubResolutionSource
	registry    stubAuthorizations
	contents    stubAgreements
}

func happySelectionFixture(t testing.TB) selectionFixture {
	t.Helper()
	registration := publishedAuthorization(t, basisCandidate, basisOperator, assemblyScope, year2026Interval(t))
	agreement := effectiveSupplierAgreement(t, year2026Interval(t))
	return selectionFixture{
		accounts: stubAccountUseSource{
			selection: adapter.ChannelAccountUseSelection{
				Authorization: registration.ID(),
				Grantee:       assemblyValue(t, pcdomain.NewPartyID, basisOperator),
			},
			found: true,
		},
		agreements:  stubAgreementSource{version: agreement.Version(), found: true},
		resolutions: stubResolutionSource{resolution: assemblyValue(t, psdomain.NewCommercialResolutionID, basisResolution), found: true},
		registry:    stubAuthorizations{registrations: map[string]pcdomain.ChannelAccountUseAuthorizationRegistration{registration.ID().String(): registration}},
		contents:    stubAgreements{agreements: map[string]pcdomain.SupplierAgreement{basisAgreement + "/v1": agreement}},
	}
}

func (fixture selectionFixture) translator() *adapter.ChannelSelectionBasisTranslator {
	return adapter.NewChannelSelectionBasisTranslator(adapter.ChannelSelectionBasisTranslatorDeps{
		Accounts:       fixture.accounts,
		Agreements:     fixture.agreements,
		Resolutions:    fixture.resolutions,
		Authorizations: fixture.registry,
		Contents:       fixture.contents,
	})
}

// Covers: 票 29 判据 3 正路——三源配上、授权与协议在册且盖住时点，七格齐：账号 / 持有人取自授权，服务方与
// 结算相对方同取协议的 Supplier()（裁决 (2)：同源是首发的实例事实，不是类型上的同义），合同取协议版本
// 「对象/版本」，费率照择优步带出的，责任依据回指接受时解析；评价痕迹随行。七格再照抄进 Establish 过门。
func TestASelectedCandidateTranslatesIntoAllSevenReferences(t *testing.T) {
	t.Parallel()

	basis, err := happySelectionFixture(t).translator().TranslateSelectedCandidate(context.Background(), selectionBasisQuery(t), pricedSelection(t))
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}

	want := map[string][2]string{
		"候选":    {basis.Candidate().String(), basisCandidate},
		"渠道账号":  {basis.ChannelAccount().String(), basisAccount},
		"账号持有人": {basis.AccountHolder().String(), basisHolder},
		"渠道服务方": {basis.ServiceProvider().String(), basisSupplier},
		"结算相对方": {basis.SettlementCounterparty().String(), basisSupplier},
		"合同":    {basis.Contract().String(), basisAgreement + "/v1"},
		"费率":    {basis.Rate().String(), "buy-plan-1/v1"},
		"责任依据":  {basis.ResponsibilityBasis().String(), basisResolution},
	}
	for name, pair := range want {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q，want %q", name, pair[0], pair[1])
		}
	}
	if evaluation, present := basis.Evaluation(); !present || evaluation.String() != "eval-1" {
		t.Errorf("评价痕迹 = %q/%v，want eval-1 在场", evaluation.String(), present)
	}

	if _, err := psdomain.EstablishLabelTransaction(psdomain.EstablishLabelTransactionSpec{
		Tenant:                 assemblyValue(t, psdomain.NewTenantID, assemblyTenant),
		ID:                     assemblyValue(t, psdomain.NewLabelTransactionID, "LT-1"),
		CoveredParcels:         []psdomain.DeclaredParcelID{assemblyValue(t, psdomain.NewDeclaredParcelID, "P-1")},
		ChannelAccount:         basis.ChannelAccount(),
		AccountHolder:          basis.AccountHolder(),
		ServiceProvider:        basis.ServiceProvider(),
		SettlementCounterparty: basis.SettlementCounterparty(),
		Contract:               basis.Contract(),
		Rate:                   basis.Rate(),
		ResponsibilityBasis:    basis.ResponsibilityBasis(),
		EstablishedAt:          assemblyAt(t),
	}); err != nil {
		t.Fatalf("七格照抄进 Establish 被拒：%v", err)
	}
}

// Covers: 票 29 判据 3 的停点逐格具名——每格各改一处，错误各自可辨；三源答「未配置」与源本身没装是同一格。
func TestEachTranslationStopIsNamed(t *testing.T) {
	t.Parallel()

	beforeStart := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	stops := map[string]struct {
		mutate func(testing.TB, *selectionFixture)
		want   error
	}{
		"授权源未装": {
			mutate: func(_ testing.TB, fixture *selectionFixture) { fixture.accounts = stubAccountUseSource{} },
			want:   adapter.ErrChannelAccountUseNotConfigured,
		},
		"协议源答未配置": {
			mutate: func(_ testing.TB, fixture *selectionFixture) { fixture.agreements.found = false },
			want:   adapter.ErrSupplierAgreementNotConfigured,
		},
		"接受时解析源答未配置": {
			mutate: func(_ testing.TB, fixture *selectionFixture) { fixture.resolutions.found = false },
			want:   adapter.ErrAcceptanceResolutionNotConfigured,
		},
		"授权不在册": {
			mutate: func(_ testing.TB, fixture *selectionFixture) { fixture.registry.registrations = nil },
			want:   adapter.ErrChannelAccountUseNotRegistered,
		},
		"授权盖的是另一个渠道产品": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				registration := publishedAuthorization(t, "channel-b", basisOperator, assemblyScope, year2026Interval(t))
				fixture.registry.registrations[registration.ID().String()] = registration
			},
			want: adapter.ErrChannelAccountUseChannelMismatch,
		},
		"授权授给的不是运营企业": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				registration := publishedAuthorization(t, basisCandidate, "party-someone-else", assemblyScope, year2026Interval(t))
				fixture.registry.registrations[registration.ID().String()] = registration
			},
			want: adapter.ErrChannelAccountUseGranteeMismatch,
		},
		"授权盖的是另一个商业范围": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				registration := publishedAuthorization(t, basisCandidate, basisOperator, "scope-b", year2026Interval(t))
				fixture.registry.registrations[registration.ID().String()] = registration
			},
			want: adapter.ErrChannelAccountUseScopeMismatch,
		},
		"授权在时点前已撤销": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				registration := publishedAuthorization(t, basisCandidate, basisOperator, assemblyScope, year2026Interval(t))
				revoked, err := registration.Authorization().Revoke(
					assemblyValue(t, pcdomain.NewChannelAccountRevocationBasisReference, "holder-notice-1"),
					time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				)
				if err != nil {
					t.Fatalf("撤销：%v", err)
				}
				fixture.registry.registrations[registration.ID().String()] = registrationOf(t, revoked)
			},
			want: adapter.ErrChannelAccountUseRevoked,
		},
		"授权不在有效期": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				registration := publishedAuthorization(t, basisCandidate, basisOperator, assemblyScope,
					intervalOf(t, beforeStart, time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)))
				fixture.registry.registrations[registration.ID().String()] = registration
			},
			want: adapter.ErrChannelAccountUseNotEffective,
		},
		"协议不在册": {
			mutate: func(_ testing.TB, fixture *selectionFixture) { fixture.contents.agreements = nil },
			want:   adapter.ErrSupplierAgreementNotRegistered,
		},
		"协议在时点前已终止": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				terminated, err := effectiveSupplierAgreement(t, year2026Interval(t)).Terminate(
					assemblyValue(t, pcdomain.NewRelationshipBasisReference, "termination-notice-1"),
					time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				)
				if err != nil {
					t.Fatalf("终止协议：%v", err)
				}
				fixture.contents.agreements[basisAgreement+"/v1"] = terminated
			},
			want: adapter.ErrSupplierAgreementTerminated,
		},
		"协议不在有效期": {
			mutate: func(t testing.TB, fixture *selectionFixture) {
				fixture.contents.agreements[basisAgreement+"/v1"] = effectiveSupplierAgreement(t,
					intervalOf(t, beforeStart, time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)))
			},
			want: adapter.ErrSupplierAgreementNotEffective,
		},
	}
	for name, stop := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := happySelectionFixture(t)
			stop.mutate(t, &fixture)
			_, err := fixture.translator().TranslateSelectedCandidate(context.Background(), selectionBasisQuery(t), pricedSelection(t))
			if !errors.Is(err, stop.want) {
				t.Fatalf("err = %v，want %v", err, stop.want)
			}
		})
	}
}

// Covers: 择优步交出的赢家没带费率时翻译停下、不代填——Rate 是 Establish 必填的一格，缺席只能是成本取值没带
// 费率过来（装配缺件），不是「这笔交易没有费率」。
func TestATranslationStopsWhenTheSelectedCandidateCarriesNoRate(t *testing.T) {
	t.Parallel()

	unrated, err := psdomain.NewSelectedChannelCandidate(assemblyValue(t, psdomain.NewChannelCandidateID, basisCandidate))
	if err != nil {
		t.Fatalf("造选中候选：%v", err)
	}
	_, err = happySelectionFixture(t).translator().TranslateSelectedCandidate(context.Background(), selectionBasisQuery(t), unrated)
	if !errors.Is(err, adapter.ErrSelectedCandidateRateAbsent) {
		t.Fatalf("err = %v，want ErrSelectedCandidateRateAbsent", err)
	}
}

// Covers: 没经构造门的零值候选译不成提供方的渠道产品引用——查询出不去那一格（ErrUntranslatableQuery），
// 在问任何源之前就停。
func TestATranslationRefusesAnUntranslatableSelection(t *testing.T) {
	t.Parallel()

	_, err := happySelectionFixture(t).translator().TranslateSelectedCandidate(context.Background(), selectionBasisQuery(t), psdomain.SelectedChannelCandidate{})
	if !errors.Is(err, adapter.ErrUntranslatableQuery) {
		t.Fatalf("err = %v，want ErrUntranslatableQuery", err)
	}
}
