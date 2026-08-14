package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

type keySourceDouble struct {
	key    pcdomain.ClosureResolutionKey
	formed bool
	err    error
}

func (double *keySourceDouble) FormResolutionKey(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	return double.key, double.formed, double.err
}

type contentDouble struct {
	content         pcdomain.AcceptanceRuleContent
	contentFound    bool
	contentErr      error
	permission      pcdomain.PendingRoutingPermission
	permissionFound bool
	permissionErr   error
}

func (double *contentDouble) LoadAcceptanceRuleContent(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) (pcdomain.AcceptanceRuleContent, bool, error) {
	if double.contentErr != nil {
		return pcdomain.AcceptanceRuleContent{}, false, double.contentErr
	}
	return double.content, double.contentFound, nil
}

func (double *contentDouble) LoadPendingRoutingPermission(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) (pcdomain.PendingRoutingPermission, bool, error) {
	if double.permissionErr != nil {
		return pcdomain.PendingRoutingPermission{}, false, double.permissionErr
	}
	return double.permission, double.permissionFound, nil
}

var _ pcports.AcceptanceContentDeclaration = (*contentDouble)(nil)

type basisFixture struct {
	registry  *pcdomain.CommercialRegistry
	authority *authorityDouble
	store     *resolutionStoreDouble
	policies  *asOfPolicyDouble
	contents  *contentDouble
	keys      *keySourceDouble
	values    *valueSourceDouble
	adapter   *adapter.CommercialBasisAdapter
}

// newBasisFixture 造一份三个必需依据（合同、规则包、服务产品）都唯一生效的世界，内容与
// 时点声明齐备。withContract=false 时留出`无适用依据`那一格。
func newBasisFixture(t *testing.T, withContract bool) *basisFixture {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	if withContract {
		effectiveIn(t, registry, pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	}
	rules := effectiveIn(t, registry, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")
	product := effectiveIn(t, registry, pcdomain.ServiceProductObject, "product-1", "v1", "sha256:p1", "scope-a")

	content, err := pcdomain.DeclareAcceptanceRuleContent(rules,
		[]pcdomain.AcceptanceCheckGroupType{
			pcdomain.PreAcceptanceFinancialControlCheckGroup,
			pcdomain.NetworkReachabilityCheckGroup,
		},
		pcdomain.ManualReviewNotRequired,
	)
	if err != nil {
		t.Fatalf("declare acceptance rule content: %v", err)
	}
	permission, err := pcdomain.DeclarePendingRoutingPermission(product,
		value(t, pcdomain.NewPendingRoutingBasisReference, "PC-PENDING-ROUTING-1"))
	if err != nil {
		t.Fatalf("declare pending routing permission: %v", err)
	}

	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}

	fixture := &basisFixture{
		registry:  registry,
		authority: &authorityDouble{registry: registry},
		store:     &resolutionStoreDouble{},
		policies: &asOfPolicyDouble{policies: []pcdomain.AsOfPolicy{
			providerPolicyFor(t, pcdomain.NetworkReachabilityJudgment),
			providerPolicyFor(t, pcdomain.PreAcceptanceFinancialControlJudgment),
		}},
		contents: &contentDouble{content: content, contentFound: true, permission: permission, permissionFound: true},
		keys: &keySourceDouble{formed: true, key: pcdomain.ClosureResolutionKey{
			TenantID:             value(t, pcdomain.NewTenantID, "tenant-1"),
			CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases: []pcdomain.CommercialObjectKind{
				pcdomain.CustomerContractObject,
				pcdomain.AcceptanceRulePackageObject,
				pcdomain.ServiceProductObject,
			},
		}},
		values: &valueSourceDouble{at: formedValueAt, formed: true},
	}
	fixture.adapter = adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Resolve:      pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: judgedAt}),
		Revalidate:   pcapplication.NewValidateCommercialBasisHandler(fixture.store, fixture.authority, fixedClock{at: judgedAt}),
		Judgments:    pcapplication.NewFormJudgmentAsOfHandler(fixture.store, fixture.policies),
		AsOfPolicies: fixture.policies,
		Contents:     fixture.contents,
		Keys:         fixture.keys,
		Values:       fixture.values,
	})
	return fixture
}

func (fixture *basisFixture) identity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func (fixture *basisFixture) resolveQuery(t *testing.T) psports.CommercialBasisQuery {
	t.Helper()
	return psports.CommercialBasisQuery{
		Identity:          fixture.identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
	}
}

// fixResolution 走真实第一阶段并把闭包放进取回端口，交回它供重校验回指。
func (fixture *basisFixture) fixResolution(t *testing.T) pcdomain.CommercialClosure {
	t.Helper()
	resolved, err := pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: judgedAt}).
		Handle(context.Background(), pcapplication.ResolveCommercialBasisCommand{Key: fixture.keys.key})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	closure := resolved.Closure()
	if closure.Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("fixture closure outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}
	fixture.store.closure = closure
	fixture.store.found = true
	return closure
}

// withSettlementBasis 给夹具补上结算依据：登记一份生效政策并把结算成员加进闭包键
// （选择器与政策适用范围逐维对齐，锚点落在有效区间内）。
func (fixture *basisFixture) withSettlementBasis(t *testing.T, method pcdomain.SettlementMethod) {
	t.Helper()
	version := effectiveIn(t, fixture.registry, pcdomain.SettlementPolicyObject, "settle-1", "v1", "sha256:s1", "scope-a")
	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		value(t, pcdomain.NewLegalEntityReference, "legal-1"),
		value(t, pcdomain.NewCounterpartyReference, "customer-1"),
		value(t, pcdomain.NewCommercialVersionLabel, "contract-1/v1"),
		value(t, pcdomain.NewChargeScopeReference, "charge-express"),
		value(t, pcdomain.NewCurrencyCode, "SYN"),
		interval,
	)
	if err != nil {
		t.Fatalf("new settlement applicability: %v", err)
	}
	policy, err := pcdomain.NewSettlementPolicy(version, method, applicability)
	if err != nil {
		t.Fatalf("new settlement policy: %v", err)
	}
	fixture.registry.RegisterSettlementPolicy(policy)

	fixture.keys.key.RequiredBases = append(fixture.keys.key.RequiredBases, pcdomain.SettlementPolicyObject)
	fixture.keys.key.Settlement = pcdomain.SettlementSelector{
		Counterparty: value(t, pcdomain.NewCounterpartyReference, "customer-1"),
		Contract:     value(t, pcdomain.NewCommercialVersionLabel, "contract-1/v1"),
		ChargeScope:  value(t, pcdomain.NewChargeScopeReference, "charge-express"),
		Currency:     value(t, pcdomain.NewCurrencyCode, "SYN"),
	}
}

// Covers: ADR-0044 的消费侧回显与 ADR-0047 作用域缝的输入——闭包采用结算政策时，快照
// 携带政策引用、方式与作用域三维；闭包不含结算依据时回显缺席，那是真话不是翻译失败。
func TestAnAdoptedSettlementPolicyIsEchoedIntoTheSnapshot(t *testing.T) {
	fixture := newBasisFixture(t, true)
	fixture.withSettlementBasis(t, pcdomain.TermsMethod)

	resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
	if err != nil {
		t.Fatalf("resolve commercial basis: %v", err)
	}
	if resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("applicability = %q reason = %q, want APPLICABLE", resolution.Applicability, resolution.Reason)
	}

	terms, present := resolution.Snapshot.SettlementTerms()
	if !present {
		t.Fatal("闭包采用了结算政策，快照却没带回显——作用域缝断在翻译这一步")
	}
	if terms.Policy().String() != "settle-1/v1" || terms.Method().String() != "TERMS" {
		t.Fatalf("policy/method = %q/%q, want settle-1/v1 与 TERMS", terms.Policy(), terms.Method())
	}
	if terms.LegalEntity().String() != "legal-1" ||
		terms.Counterparty().String() != "customer-1" ||
		terms.Currency().String() != "SYN" {
		t.Fatalf("scope dims = %s/%s/%s; 三维必须来自政策适用范围", terms.LegalEntity(), terms.Counterparty(), terms.Currency())
	}

	plain := newBasisFixture(t, true)
	bare, err := plain.adapter.ResolveCommercialBasis(context.Background(), plain.resolveQuery(t))
	if err != nil {
		t.Fatalf("resolve without settlement basis: %v", err)
	}
	if _, present := bare.Snapshot.SettlementTerms(); present {
		t.Fatal("不含结算依据的解析凭空长出了回显")
	}
}

func (fixture *basisFixture) revalidateQuery(t *testing.T, closure pcdomain.CommercialClosure) psports.CommercialRevalidationQuery {
	t.Helper()
	return psports.CommercialRevalidationQuery{
		Identity:          fixture.identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Resolution:        value(t, psdomain.NewCommercialResolutionID, closure.ResolutionID().String()),
	}
}

// Covers: UC-PC-002 步骤 4/5（唯一解析携带采用版本与当前修订）与交接「消费者端口必须返回
// 结构化的唯一成功……不能只返回对象」；ADR-0042「内容声明在唯一选出后按已选对象读取」。
// 快照的每一项都要能追回提供方的声明——少一项，接受语言就有一块是适配器编的。
func TestAUniquelyResolvedClosureBecomesAnAdoptedSnapshot(t *testing.T) {
	fixture := newBasisFixture(t, true)
	expected := fixture.fixResolution(t)

	resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
	if err != nil {
		t.Fatalf("resolve commercial basis: %v", err)
	}

	if resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("applicability = %q reason = %q, want APPLICABLE", resolution.Applicability, resolution.Reason)
	}
	snapshot := resolution.Snapshot
	if snapshot.ResolutionID().String() != expected.ResolutionID().String() {
		t.Fatalf("resolution ID = %q, want %q", snapshot.ResolutionID(), expected.ResolutionID())
	}
	if snapshot.RulePackage().String() != "rules-1/v1" {
		t.Fatalf("rule package = %q, want rules-1/v1", snapshot.RulePackage())
	}
	expectedRevision, ok := expected.ViewRevision()
	if !ok {
		t.Fatal("fixture closure has no view revision")
	}
	if snapshot.ViewRevision().String() != expectedRevision.String() {
		t.Fatalf("view revision = %q, want %q", snapshot.ViewRevision(), expectedRevision)
	}
	for kind, judgment := range map[psdomain.JudgmentKind]pcdomain.JudgmentType{
		psdomain.ReachabilityJudgmentKind:     pcdomain.NetworkReachabilityJudgment,
		psdomain.FinancialControlJudgmentKind: pcdomain.PreAcceptanceFinancialControlJudgment,
	} {
		provider := providerPolicyFor(t, judgment)
		want, err := psdomain.NewDeclaredAsOf(
			kind,
			value(t, psdomain.NewAsOfSemanticsReference, provider.Semantics().String()),
			value(t, psdomain.NewAsOfPolicyVersion, provider.PolicyVersion().String()),
		)
		if err != nil {
			t.Fatalf("new expected declared as-of: %v", err)
		}
		declared, present := snapshot.DeclaredAsOfFor(kind)
		if !present || declared != want {
			t.Fatalf("declared as-of for %q = %#v present = %v, want %#v", kind, declared, present, want)
		}
	}
	if snapshot.ManualReviewPolicy() != psdomain.ManualReviewNotRequiredByRules {
		t.Fatalf("manual review = %q, want NOT_REQUIRED", snapshot.ManualReviewPolicy())
	}
	wantAllowance, err := psdomain.NewPendingRoutingAllowance(value(t, psdomain.NewPendingRoutingBasis, "PC-PENDING-ROUTING-1"))
	if err != nil {
		t.Fatalf("new expected allowance: %v", err)
	}
	if snapshot.PendingRoutingAllowance() != wantAllowance {
		t.Fatal("待路由许可没有带上服务产品声明的依据（AT-PS-007 要保存的正是它）")
	}
}

// Covers: ResolutionKeySource 的类型注释「没有租户时谁也说不出这份委托该在哪个商业范围下
// 解析」——键属实例半边，未配置停在`解析未决`，且停在问提供方之前。
func TestAnUnconfiguredKeySourceStopsUndeterminedWithoutResolving(t *testing.T) {
	cases := map[string]func(*testing.T, *basisFixture) *adapter.CommercialBasisAdapter{
		"no key source at all": func(t *testing.T, fixture *basisFixture) *adapter.CommercialBasisAdapter {
			return adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
				Resolve:      pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: judgedAt}),
				AsOfPolicies: fixture.policies,
				Contents:     fixture.contents,
			})
		},
		"source cannot form the key": func(_ *testing.T, fixture *basisFixture) *adapter.CommercialBasisAdapter {
			fixture.keys.formed = false
			return fixture.adapter
		},
	}

	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newBasisFixture(t, true)
			subject := arrange(t, fixture)

			resolution, err := subject.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
			if err != nil {
				t.Fatalf("resolve commercial basis: %v", err)
			}

			if resolution.Applicability != psdomain.CommercialApplicabilityUndetermined {
				t.Fatalf("applicability = %q, want UNDETERMINED", resolution.Applicability)
			}
			if resolution.Reason.String() != "COMMERCIAL_RESOLUTION_KEY_NOT_CONFIGURED" {
				t.Fatalf("reason = %q", resolution.Reason)
			}
			if fixture.authority.loadCalled != 0 {
				t.Fatal("键都立不起来仍去读了权威视图")
			}
		})
	}
}

// Covers: ADR-0025「翻译必须是全函数」在第一阶段一侧——`无适用依据`是唯一可拿去拒单的
// 答案（AT-PC-020），`适用冲突`（AT-PC-021/032 阻断不任选）、`解析未决`与`输入未受理`都
// 落`无法判定`，原因引用带出提供方的具名原因。
func TestEveryFirstPhaseAnswerLandsOnItsOwnApplicability(t *testing.T) {
	cases := map[string]struct {
		arrange           func(*testing.T) *basisFixture
		wantApplicability psdomain.CommercialApplicability
		wantReason        string
	}{
		"no applicable basis rejects": {
			arrange: func(t *testing.T) *basisFixture {
				return newBasisFixture(t, false)
			},
			wantApplicability: psdomain.CommerciallyNotApplicable,
			wantReason:        "PC-NO_APPLICABLE_BASIS",
		},
		"conflict blocks without picking": {
			arrange: func(t *testing.T) *basisFixture {
				fixture := newBasisFixture(t, true)
				effectiveIn(t, fixture.registry, pcdomain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")
				return fixture
			},
			wantApplicability: psdomain.CommercialApplicabilityUndetermined,
			wantReason:        "PC-APPLICABILITY_CONFLICT",
		},
		"unreadable authority stays pending": {
			arrange: func(t *testing.T) *basisFixture {
				fixture := newBasisFixture(t, true)
				fixture.authority.err = errors.New("authority down")
				return fixture
			},
			wantApplicability: psdomain.CommercialApplicabilityUndetermined,
			wantReason:        "PC-AUTHORITY_UNREADABLE",
		},
		"incomplete key is not accepted": {
			arrange: func(t *testing.T) *basisFixture {
				fixture := newBasisFixture(t, true)
				fixture.keys.key.CustomerAccountID = pcdomain.CustomerAccountID{}
				return fixture
			},
			wantApplicability: psdomain.CommercialApplicabilityUndetermined,
			wantReason:        "PC-INPUT_NOT_ACCEPTED",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := testCase.arrange(t)

			resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
			if err != nil {
				t.Fatalf("resolve commercial basis: %v", err)
			}

			if resolution.Applicability != testCase.wantApplicability {
				t.Fatalf("applicability = %q, want %q", resolution.Applicability, testCase.wantApplicability)
			}
			if resolution.Reason.String() != testCase.wantReason {
				t.Fatalf("reason = %q, want %q", resolution.Reason, testCase.wantReason)
			}
			if resolution.Snapshot.ResolutionID().String() != "" {
				t.Fatal("非唯一解析携带了快照")
			}
		})
	}
}

// Covers: ADR-0042「found=false 即实例未配置……消费方据以停在未决，不是放行」，以及
// adoptedResolution 的注释——空适用组等于无条件接受，适配器不得在翻译里造出一份缺声明
// 的快照。读不回与没配置都停；唯一不停的是「产品答过了：不许待路由」。
func TestContentDeclarationGapsKeepTheResolutionUndetermined(t *testing.T) {
	gaps := map[string]struct {
		arrange    func(*basisFixture)
		wantReason string
	}{
		"rule content not configured": {
			arrange:    func(fixture *basisFixture) { fixture.contents.contentFound = false },
			wantReason: "PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED",
		},
		"rule content unreadable": {
			arrange:    func(fixture *basisFixture) { fixture.contents.contentErr = errors.New("content down") },
			wantReason: "PC-ACCEPTANCE_CONTENT_UNREADABLE",
		},
		"as-of policies unreadable": {
			arrange:    func(fixture *basisFixture) { fixture.policies.err = errors.New("policies down") },
			wantReason: "PC-ASOF_POLICIES_UNREADABLE",
		},
		"pending routing unreadable": {
			arrange:    func(fixture *basisFixture) { fixture.contents.permissionErr = errors.New("permission down") },
			wantReason: "PC-PENDING_ROUTING_UNREADABLE",
		},
	}

	for name, gap := range gaps {
		t.Run(name, func(t *testing.T) {
			fixture := newBasisFixture(t, true)
			gap.arrange(fixture)

			resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
			if err != nil {
				t.Fatalf("resolve commercial basis: %v", err)
			}

			if resolution.Applicability != psdomain.CommercialApplicabilityUndetermined {
				t.Fatalf("applicability = %q, want UNDETERMINED", resolution.Applicability)
			}
			if resolution.Reason.String() != gap.wantReason {
				t.Fatalf("reason = %q, want %q", resolution.Reason, gap.wantReason)
			}
		})
	}

	t.Run("an undeclared permission is an answer, not a gap", func(t *testing.T) {
		fixture := newBasisFixture(t, true)
		fixture.contents.permissionFound = false

		resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
		if err != nil {
			t.Fatalf("resolve commercial basis: %v", err)
		}

		if resolution.Applicability != psdomain.CommerciallyApplicable {
			t.Fatalf("applicability = %q, want APPLICABLE——产品没声明许可是`未许可`，不是未决", resolution.Applicability)
		}
		if resolution.Snapshot.PendingRoutingAllowance() != (psdomain.PendingRoutingAllowance{}) {
			t.Fatal("没有声明却带出了许可")
		}
	})
}

// Covers: UC-PC-002 步骤 8 与 AT-PC-024 —— 视图没变时重校验交回原解析，标识不换、快照
// 可用；重校验特有的结果代数由第三阶段方法翻译。
func TestARevalidationConfirmsTheStandingResolution(t *testing.T) {
	fixture := newBasisFixture(t, true)
	closure := fixture.fixResolution(t)

	revalidation, err := fixture.adapter.RevalidateCommercialBasis(context.Background(), fixture.revalidateQuery(t, closure))
	if err != nil {
		t.Fatalf("revalidate commercial basis: %v", err)
	}

	if revalidation.Outcome != psports.CommercialBasisStillValid {
		t.Fatalf("outcome = %q reason = %q, want STILL_VALID", revalidation.Outcome, revalidation.Reason)
	}
	if revalidation.Resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("resolution applicability = %q", revalidation.Resolution.Applicability)
	}
	if revalidation.Resolution.Snapshot.ResolutionID().String() != closure.ResolutionID().String() {
		t.Fatal("重校验换掉了解析标识——原判断会对不上它形成时的依据")
	}
}

// Covers: UC-PC-002 结果语义`已失效`与 ports.CommercialRevalidation 注释「`已失效`不带
// 解析：交回一份，调用方会以为可以继续用它」。
func TestASupersededResolutionIsReportedWithoutCarryingIt(t *testing.T) {
	fixture := newBasisFixture(t, true)
	closure := fixture.fixResolution(t)
	effectiveIn(t, fixture.registry, pcdomain.CustomerContractObject, "contract-2", "v1", "sha256:c2", "scope-a")

	revalidation, err := fixture.adapter.RevalidateCommercialBasis(context.Background(), fixture.revalidateQuery(t, closure))
	if err != nil {
		t.Fatalf("revalidate commercial basis: %v", err)
	}

	if revalidation.Outcome != psports.CommercialBasisSuperseded {
		t.Fatalf("outcome = %q, want SUPERSEDED", revalidation.Outcome)
	}
	if revalidation.Reason.String() != "PC-CURRENT_RESOLUTION_CHANGED" {
		t.Fatalf("reason = %q", revalidation.Reason)
	}
	if revalidation.Resolution.Snapshot.ResolutionID().String() != "" {
		t.Fatal("已失效的答复携带了解析")
	}
}

// Covers: ADR-0025「翻译必须是全函数」在第三阶段一侧，与 ADR-0029/0033 的落格：读不回是
// `无法判定`（重试同一次），`依据未解析`回第一阶段，`输入未受理`是短路支；内容声明缺口让
// 一次「仍然唯一」的确认停在`无法判定`——快照建不起来就不能说它仍然成立。
func TestEveryRevalidationAnswerLandsOnItsOwnConsumerValue(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*testing.T, *basisFixture) psports.CommercialRevalidationQuery
		want       psports.CommercialRevalidationOutcome
		wantReason string
	}{
		"unreadable store stays undetermined": {
			arrange: func(t *testing.T, fixture *basisFixture) psports.CommercialRevalidationQuery {
				closure := fixture.fixResolution(t)
				fixture.store.err = errors.New("store down")
				return fixture.revalidateQuery(t, closure)
			},
			want:       psports.CommercialRevalidationUndetermined,
			wantReason: "PC-AUTHORITY_UNREADABLE",
		},
		"unknown resolution is basis not resolved": {
			arrange: func(t *testing.T, fixture *basisFixture) psports.CommercialRevalidationQuery {
				closure := fixture.fixResolution(t)
				fixture.store.found = false
				return fixture.revalidateQuery(t, closure)
			},
			want: psports.CommercialRevalidationBasisNotResolved,
		},
		"empty identity is not accepted": {
			arrange: func(t *testing.T, fixture *basisFixture) psports.CommercialRevalidationQuery {
				closure := fixture.fixResolution(t)
				query := fixture.revalidateQuery(t, closure)
				query.Resolution = psdomain.CommercialResolutionID{}
				return query
			},
			want: psports.CommercialRevalidationInputNotAccepted,
		},
		"content gap keeps a confirmed closure undetermined": {
			arrange: func(t *testing.T, fixture *basisFixture) psports.CommercialRevalidationQuery {
				closure := fixture.fixResolution(t)
				fixture.contents.contentFound = false
				return fixture.revalidateQuery(t, closure)
			},
			want:       psports.CommercialRevalidationUndetermined,
			wantReason: "PC-ACCEPTANCE_CONTENT_NOT_CONFIGURED",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newBasisFixture(t, true)
			query := testCase.arrange(t, fixture)

			revalidation, err := fixture.adapter.RevalidateCommercialBasis(context.Background(), query)
			if err != nil {
				t.Fatalf("revalidate commercial basis: %v", err)
			}

			if revalidation.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", revalidation.Outcome, testCase.want)
			}
			if testCase.wantReason != "" && revalidation.Reason.String() != testCase.wantReason {
				t.Fatalf("reason = %q, want %q", revalidation.Reason, testCase.wantReason)
			}
			if revalidation.Resolution.Snapshot.ResolutionID().String() != "" {
				t.Fatal("未确认成立的答复携带了解析")
			}
		})
	}
}
