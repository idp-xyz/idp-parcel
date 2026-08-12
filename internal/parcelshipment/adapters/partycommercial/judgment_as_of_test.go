package partycommercial_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 两个时刻刻意不同：值取自消费方形成、时点锚属提供方回显，取成同一个就断言不出
// 「值没有被偷换成别的时钟」。
var (
	anchorAt      = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	judgedAt      = time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
	formedValueAt = time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %T from %q: %v", built, raw, err)
	}
	return built
}

// effectiveIn 用导出 API 把一个已发布生效的商业版本放进登记册并交回它，形状照抄
// party-commercial 自己的应用测试夹具（那份在 _test.go 里，此处不可 import，只能重建）。
// 交回版本是因为内容与时点声明都要按「已选出的那个包/产品」构造（ADR-0042）。
func effectiveIn(
	t *testing.T,
	registry *pcdomain.CommercialRegistry,
	kind pcdomain.CommercialObjectKind,
	objectID, version, digest, scope string,
) pcdomain.CommercialVersion {
	t.Helper()

	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	draft, err := pcdomain.NewCommercialDraft(pcdomain.CommercialVersionSpec{
		TenantID:      value(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          kind,
		ObjectID:      value(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       value(t, pcdomain.NewCommercialVersionLabel, version),
		Scope:         value(t, pcdomain.NewCommercialScopeReference, scope),
		ContentDigest: value(t, pcdomain.NewCommercialContentDigest, digest),
		Effective:     interval,
	})
	if err != nil {
		t.Fatalf("new draft: %v", err)
	}
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	basis, err := pcdomain.NewApprovalBasis(
		value(t, pcdomain.NewApprovalReference, "approval-"+objectID),
		value(t, pcdomain.NewCommercialSourceReference, "source-"+objectID),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("new approval basis: %v", err)
	}
	published, err := draft.Publish(basis, pcdomain.ApprovalRoleConfirmed, approvedAt, nil)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	live, err := published.TakeEffect(approvedAt)
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if _, err := registry.Register(live); err != nil {
		t.Fatalf("register: %v", err)
	}
	return live
}

type authorityDouble struct {
	registry   *pcdomain.CommercialRegistry
	err        error
	loadCalled int
}

func (double *authorityDouble) LoadScope(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialScopeReference,
) (*pcdomain.CommercialRegistry, error) {
	double.loadCalled++
	if double.err != nil {
		return nil, double.err
	}
	return double.registry, nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// resolvedClosure 走真实第一阶段解出一份唯一闭包：适配器测试是两个应用层唯一相遇的地方
// （ADR-0025），提供方一侧必须是真编排而不是提供方替身，否则「翻译对不对」就没有对照面。
func resolvedClosure(t *testing.T) pcdomain.CommercialClosure {
	t.Helper()

	registry := pcdomain.NewCommercialRegistry()
	effectiveIn(t, registry, pcdomain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	effectiveIn(t, registry, pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")

	anchor, err := pcdomain.NewSelectionAnchor(anchorAt, value(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"))
	if err != nil {
		t.Fatalf("new selection anchor: %v", err)
	}
	resolved, err := pcapplication.NewResolveCommercialBasisHandler(&authorityDouble{registry: registry}, fixedClock{at: judgedAt}).
		Handle(context.Background(), pcapplication.ResolveCommercialBasisCommand{
			Key: pcdomain.ClosureResolutionKey{
				TenantID:             value(t, pcdomain.NewTenantID, "tenant-1"),
				CustomerAccountID:    value(t, pcdomain.NewCustomerAccountID, "customer-1"),
				LegalEntityCandidate: value(t, pcdomain.NewLegalEntityReference, "legal-1"),
				Scope:                value(t, pcdomain.NewCommercialScopeReference, "scope-a"),
				Purpose:              pcdomain.AcceptanceControlPurpose,
				Anchor:               anchor,
				RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.CustomerContractObject, pcdomain.AcceptanceRulePackageObject},
			},
		})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Closure().Outcome() != pcdomain.UniquelyResolved {
		t.Fatalf("outcome = %q, want the first phase to succeed", resolved.Closure().Outcome())
	}
	return resolved.Closure()
}

type resolutionStoreDouble struct {
	closure pcdomain.CommercialClosure
	found   bool
	err     error
	asked   []pcdomain.ResolutionID
}

func (double *resolutionStoreDouble) LoadResolution(
	_ context.Context,
	_ pcdomain.TenantID,
	resolution pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	double.asked = append(double.asked, resolution)
	if double.err != nil {
		return pcdomain.CommercialClosure{}, false, double.err
	}
	return double.closure, double.found, nil
}

type asOfPolicyDouble struct {
	policies []pcdomain.AsOfPolicy
	err      error
}

func (double *asOfPolicyDouble) LoadAsOfPolicies(
	_ context.Context,
	_ pcdomain.TenantID,
	_ pcdomain.CommercialVersion,
) ([]pcdomain.AsOfPolicy, error) {
	if double.err != nil {
		return nil, double.err
	}
	return double.policies, nil
}

var (
	_ pcports.CommercialResolutionStore = (*resolutionStoreDouble)(nil)
	_ pcports.AsOfPolicyDeclaration     = (*asOfPolicyDouble)(nil)
)

type valueSourceDouble struct {
	at     time.Time
	formed bool
	err    error
	asked  []psports.JudgmentAsOfQuery
}

func (double *valueSourceDouble) FormAsOfValue(
	_ context.Context,
	query psports.JudgmentAsOfQuery,
) (time.Time, bool, error) {
	double.asked = append(double.asked, query)
	return double.at, double.formed, double.err
}

// providerPolicyFor 是夹具里提供方声明的政策。语义与版本刻意与消费方第一阶段记下的声明
// 不同——回显必须取自提供方答复，两者相同的话「转手声明冒充回显」这类实现照样全绿。
func providerPolicyFor(t *testing.T, judgment pcdomain.JudgmentType) pcdomain.AsOfPolicy {
	t.Helper()
	policy, err := pcdomain.NewAsOfPolicy(
		judgment,
		value(t, pcdomain.NewAsOfSemanticsReference, "ASOF-SEM-PROVIDER-"+judgment.String()),
		value(t, pcdomain.NewAsOfPolicyVersion, "asof-policy-provider-v2"),
	)
	if err != nil {
		t.Fatalf("new as-of policy: %v", err)
	}
	return policy
}

type asOfFixture struct {
	adapter  *adapter.CommercialBasisAdapter
	store    *resolutionStoreDouble
	policies *asOfPolicyDouble
	values   *valueSourceDouble
	closure  pcdomain.CommercialClosure
}

func newAsOfFixture(t *testing.T) *asOfFixture {
	t.Helper()

	closure := resolvedClosure(t)
	fixture := &asOfFixture{
		store: &resolutionStoreDouble{closure: closure, found: true},
		policies: &asOfPolicyDouble{policies: []pcdomain.AsOfPolicy{
			providerPolicyFor(t, pcdomain.NetworkReachabilityJudgment),
			providerPolicyFor(t, pcdomain.PreAcceptanceFinancialControlJudgment),
		}},
		values:  &valueSourceDouble{at: formedValueAt, formed: true},
		closure: closure,
	}
	fixture.adapter = adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Judgments: pcapplication.NewFormJudgmentAsOfHandler(fixture.store, fixture.policies),
		Values:    fixture.values,
	})
	return fixture
}

func (fixture *asOfFixture) query(t *testing.T, kind psdomain.JudgmentKind) psports.JudgmentAsOfQuery {
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
	declared, err := psdomain.NewDeclaredAsOf(
		kind,
		value(t, psdomain.NewAsOfSemanticsReference, "ASOF-SEM-DECLARED"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-policy-declared-v1"),
	)
	if err != nil {
		t.Fatalf("new declared as-of: %v", err)
	}
	return psports.JudgmentAsOfQuery{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Resolution:        value(t, psdomain.NewCommercialResolutionID, fixture.closure.ResolutionID().String()),
		Declared:          declared,
	}
}

// Covers: UC-PC-002 步骤 6「值由消费方逐项形成……校验并回显」与开发主线横切缺口「没有任何
// 东西把 party-commercial 声明的那一份译过去」——第一条真实翻译。回显必须取自提供方第二
// 阶段的答复而不是转手第一阶段的声明（EchoedAsOfPolicy 的类型注释），夹具里两者语义与
// 版本刻意不同，转手声明的实现在此必红。
func TestAFormedAsOfCarriesTheProvidersEchoNotTheDeclaration(t *testing.T) {
	kinds := map[string]struct {
		kind     psdomain.JudgmentKind
		judgment pcdomain.JudgmentType
	}{
		"reachability":      {kind: psdomain.ReachabilityJudgmentKind, judgment: pcdomain.NetworkReachabilityJudgment},
		"financial control": {kind: psdomain.FinancialControlJudgmentKind, judgment: pcdomain.PreAcceptanceFinancialControlJudgment},
	}

	for name, testCase := range kinds {
		t.Run(name, func(t *testing.T) {
			fixture := newAsOfFixture(t)
			query := fixture.query(t, testCase.kind)

			formation, err := fixture.adapter.FormJudgmentAsOf(context.Background(), query)
			if err != nil {
				t.Fatalf("form judgment as-of: %v", err)
			}

			if formation.Outcome != psports.JudgmentAsOfFormed {
				t.Fatalf("outcome = %q, want FORMED", formation.Outcome)
			}
			provider := providerPolicyFor(t, testCase.judgment)
			echoed, err := psdomain.NewEchoedAsOfPolicy(
				testCase.kind,
				value(t, psdomain.NewAsOfSemanticsReference, provider.Semantics().String()),
				value(t, psdomain.NewAsOfPolicyVersion, provider.PolicyVersion().String()),
			)
			if err != nil {
				t.Fatalf("new echoed policy: %v", err)
			}
			expected, err := psdomain.NewJudgmentAsOf(formedValueAt, echoed)
			if err != nil {
				t.Fatalf("new expected judgment as-of: %v", err)
			}
			if formation.AsOf != expected {
				t.Fatalf("as-of = %#v, want the provider echo %#v——回显被转手成了第一阶段的声明或值被偷换", formation.AsOf, expected)
			}
			if len(fixture.values.asked) != 1 || fixture.values.asked[0].Declared != query.Declared {
				t.Fatal("值来源没有拿到声明的语义——实例半边无从按语义分派")
			}
		})
	}
}

// Covers: ports.JudgmentAsOfQuery 的类型注释「没有租户时适配器形不出值，交回`未配置`」——
// 那正是首发要停下的地方。停下必须发生在问提供方之前：没有值的查询送过去只会换回
// `值不合法`，把「等登记」错报成「改请求」。
func TestAnUnconfiguredValueSourceStopsWithoutAskingTheProvider(t *testing.T) {
	cases := map[string]func(*asOfFixture) *adapter.CommercialBasisAdapter{
		"no source at all": func(fixture *asOfFixture) *adapter.CommercialBasisAdapter {
			return adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
				Judgments: pcapplication.NewFormJudgmentAsOfHandler(fixture.store, fixture.policies),
			})
		},
		"source cannot form this semantics": func(fixture *asOfFixture) *adapter.CommercialBasisAdapter {
			fixture.values.formed = false
			return fixture.adapter
		},
	}

	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newAsOfFixture(t)
			subject := arrange(fixture)

			formation, err := subject.FormJudgmentAsOf(context.Background(), fixture.query(t, psdomain.ReachabilityJudgmentKind))
			if err != nil {
				t.Fatalf("form judgment as-of: %v", err)
			}

			if formation.Outcome != psports.JudgmentAsOfNotConfigured {
				t.Fatalf("outcome = %q, want NOT_CONFIGURED", formation.Outcome)
			}
			if len(fixture.store.asked) != 0 {
				t.Fatal("形不出值仍去问了提供方")
			}
		})
	}
}

// Covers: ADR-0025「翻译必须是全函数：提供方封闭集合里的每个取值都要有明确落点」——
// 逐格驱动真实提供方编排走到每种未成形答复，断言各自落在消费方自己的那一格上。
func TestEveryProviderAnswerLandsOnItsOwnConsumerValue(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *asOfFixture) psports.JudgmentAsOfQuery
		want    psports.JudgmentAsOfOutcome
	}{
		"basis not resolved": {
			arrange: func(t *testing.T, fixture *asOfFixture) psports.JudgmentAsOfQuery {
				fixture.store.found = false
				return fixture.query(t, psdomain.ReachabilityJudgmentKind)
			},
			want: psports.JudgmentAsOfBasisNotResolved,
		},
		"pending": {
			arrange: func(t *testing.T, fixture *asOfFixture) psports.JudgmentAsOfQuery {
				fixture.store.err = context.DeadlineExceeded
				return fixture.query(t, psdomain.ReachabilityJudgmentKind)
			},
			want: psports.JudgmentAsOfPending,
		},
		"not configured": {
			arrange: func(t *testing.T, fixture *asOfFixture) psports.JudgmentAsOfQuery {
				fixture.policies.policies = nil
				return fixture.query(t, psdomain.ReachabilityJudgmentKind)
			},
			want: psports.JudgmentAsOfNotConfigured,
		},
		"value rejected": {
			arrange: func(t *testing.T, fixture *asOfFixture) psports.JudgmentAsOfQuery {
				fixture.values.at = time.Time{}
				return fixture.query(t, psdomain.ReachabilityJudgmentKind)
			},
			want: psports.JudgmentAsOfValueRejected,
		},
		"input not accepted": {
			arrange: func(t *testing.T, fixture *asOfFixture) psports.JudgmentAsOfQuery {
				query := fixture.query(t, psdomain.ReachabilityJudgmentKind)
				// 空标识原样传过去：立不立得起来由提供方短路作答，适配器不替它先答。
				query.Resolution = psdomain.CommercialResolutionID{}
				return query
			},
			want: psports.JudgmentAsOfInputNotAccepted,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newAsOfFixture(t)
			query := testCase.arrange(t, fixture)

			formation, err := fixture.adapter.FormJudgmentAsOf(context.Background(), query)
			if err != nil {
				t.Fatalf("form judgment as-of: %v", err)
			}

			if formation.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", formation.Outcome, testCase.want)
			}
			if formation.AsOf != (psdomain.JudgmentAsOf{}) {
				t.Fatal("未成形的答复携带了时点——调用方会拿一个没人授权过的时刻去推进权威判断")
			}
		})
	}
}
