package partycommercial_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// 本文件钉提交条件从规则包到依据快照这一段（ADR-0376 决定一，票 declared-goods-lines/09）：构造门逐版过；规则包声明「必要资料」
// 组适用时才读规则引用，读得懂的进快照、不透明的不判；引了没发布的版本或读不回规则包，解析停在无法判定，不当成没有条件。

const fbaHeadhaulCitation = "REFCFG-1:parcel-shipment/submission-conditions/FBA-HEADHAUL@1"

// Covers: ADR-0147 决定五「构造门或样例不过的文件不能发布」——已发布的每一版提交条件参考配置都读得成条件集。
func TestEveryReleasedSubmissionConditionReferencePassesTheGate(t *testing.T) {
	gated := 0
	for _, reference := range referenceconfig.Released() {
		if !strings.HasPrefix(reference.Identifier(), "parcel-shipment/submission-conditions/") {
			continue
		}
		gated++
		conditions, err := adapter.SubmissionConditionsOf(reference)
		if err != nil || conditions.Empty() {
			t.Errorf("%s：err = %v empty = %v", reference, err, conditions.Empty())
		}
	}
	if gated == 0 {
		t.Fatal("没有一版提交条件参考配置被过门")
	}
	reference, err := referenceconfig.ParseReference("parcel-shipment/submission-conditions/FBA-HEADHAUL@1")
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := adapter.SubmissionConditionsOf(reference)
	if err != nil {
		t.Fatal(err)
	}
	want := []psdomain.SubmissionCondition{
		psdomain.PlatformShipmentDeclaredCondition, psdomain.EveryParcelHasPlatformBoxLabelCondition, psdomain.PackingDeclaredCondition,
	}
	got := conditions.Conditions()
	if len(got) != len(want) {
		t.Fatalf("FBA 头程条件 = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("FBA 头程条件 = %v, want %v", got, want)
		}
	}
}

type rulePackageDouble struct {
	pack  pcdomain.AcceptanceRulePackage
	found bool
	err   error
	calls int
}

func (double *rulePackageDouble) LoadAcceptanceRulePackage(
	context.Context, pcdomain.TenantID, pcdomain.CommercialVersion,
) (pcdomain.AcceptanceRulePackage, bool, error) {
	double.calls++
	return double.pack, double.found, double.err
}

var _ pcports.AcceptanceRulePackageContentView = (*rulePackageDouble)(nil)

// rulePackageCiting 造一份把 references 都登在 CROSS_FIELD_CONDITION 类别下的规则包。
func rulePackageCiting(t *testing.T, references ...string) pcdomain.AcceptanceRulePackage {
	t.Helper()
	version := effectiveIn(t, pcdomain.NewCommercialRegistry(), pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	applicability, err := pcdomain.NewRulePackageApplicability(
		value(t, pcdomain.NewCommercialObjectID, "product-1"), value(t, pcdomain.NewCommercialObjectID, "contract-1"),
		value(t, pcdomain.NewLegalEntityReference, "legal-1"), value(t, pcdomain.NewCommercialScopeReference, "scope-a"), interval)
	if err != nil {
		t.Fatal(err)
	}
	var rules []pcdomain.AssembledRule
	for _, reference := range references {
		rule, err := pcdomain.NewAssembledRule(pcdomain.CrossFieldConditionRules, value(t, pcdomain.NewRuleReference, reference))
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	pack, err := pcdomain.NewAcceptanceRulePackage(version, applicability, rules)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

// documentsFixture 是 newBasisFixture 那个世界，规则包另声明「必要资料」组适用，规则包正文由 rules 答。
func documentsFixture(t *testing.T, rules pcports.AcceptanceRulePackageContentView, declareDocuments bool) *basisFixture {
	t.Helper()
	fixture := newBasisFixture(t, true)
	groups := []pcdomain.AcceptanceCheckGroupType{pcdomain.PreAcceptanceFinancialControlCheckGroup, pcdomain.NetworkReachabilityCheckGroup}
	if declareDocuments {
		groups = append(groups, pcdomain.RequiredDocumentCheckGroup)
	}
	version := effectiveIn(t, pcdomain.NewCommercialRegistry(), pcdomain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:r1", "scope-a")
	content, err := pcdomain.DeclareAcceptanceRuleContent(version, groups, pcdomain.ManualReviewNotRequired)
	if err != nil {
		t.Fatal(err)
	}
	fixture.contents.content = content
	fixture.adapter = adapter.NewCommercialBasisAdapter(adapter.CommercialBasisAdapterDeps{
		Resolve:      pcapplication.NewResolveCommercialBasisHandler(fixture.authority, fixture.store, fixedClock{at: judgedAt}),
		Revalidate:   pcapplication.NewValidateCommercialBasisHandler(fixture.store, fixture.authority, fixedClock{at: judgedAt}),
		Judgments:    pcapplication.NewFormJudgmentAsOfHandler(fixture.store, fixture.policies),
		AsOfPolicies: fixture.policies,
		Contents:     fixture.contents,
		Rules:        rules,
		Keys:         fixture.keys,
		Values:       fixture.values,
	})
	return fixture
}

// Covers: 规则包声明了「必要资料」组适用，以 CROSS_FIELD_CONDITION 引用的提交条件读进快照；不带 REFCFG-1 前缀的不透明引用
// 不判，也不让解析停下。
func TestCitedSubmissionConditionsAreCarriedIntoTheSnapshot(t *testing.T) {
	rules := &rulePackageDouble{pack: rulePackageCiting(t, fbaHeadhaulCitation, "SYN-RULE-CROSS-FIELD-01"), found: true}
	fixture := documentsFixture(t, rules, true)
	fixture.fixResolution(t)
	resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("applicability = %q reason = %q", resolution.Applicability, resolution.Reason)
	}
	conditions := resolution.Snapshot.SubmissionConditions()
	if len(conditions.Conditions()) != 3 || !conditions.Has(psdomain.EveryParcelHasPlatformBoxLabelCondition) {
		t.Fatalf("快照上的条件 = %v, want FBA 头程那三条", conditions.Conditions())
	}
}

// Covers: 没声明「必要资料」组就不判，也不多读一次规则包——既有不引用条件的规则包行为不变。
func TestARulePackageNotDeclaringRequiredDocumentsIsNotRead(t *testing.T) {
	rules := &rulePackageDouble{pack: rulePackageCiting(t, fbaHeadhaulCitation), found: true}
	fixture := documentsFixture(t, rules, false)
	fixture.fixResolution(t)
	resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
	if err != nil || resolution.Applicability != psdomain.CommerciallyApplicable {
		t.Fatalf("applicability = %q err = %v", resolution.Applicability, err)
	}
	if rules.calls != 0 || !resolution.Snapshot.SubmissionConditions().Empty() {
		t.Fatalf("读了 %d 次规则包、条件 = %v", rules.calls, resolution.Snapshot.SubmissionConditions().Conditions())
	}
}

// Covers: 读不回规则包、规则包没登、在这一类下引了别的目录的参考配置，解析都停在无法判定：依赖读不回或规则包登错了，不当成
// 没有条件——当成没有条件会让一票缺箱标号的货件被接受。引了没发布的版本那一格在规则包登记时就立不起来（NewRuleReference 的
// 门），这里造不出来，适配器对它的防御分支照留。读口没接上是装配缺件，响亮报错。
func TestUnreadableSubmissionConditionsKeepTheResolutionUndetermined(t *testing.T) {
	for name, rules := range map[string]pcports.AcceptanceRulePackageContentView{
		"规则包读不回":      &rulePackageDouble{err: errors.New("读不动")},
		"规则包没登":       &rulePackageDouble{},
		"引了别的目录的参考配置": &rulePackageDouble{pack: rulePackageCiting(t, "REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1"), found: true},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := documentsFixture(t, rules, true)
			fixture.fixResolution(t)
			resolution, err := fixture.adapter.ResolveCommercialBasis(context.Background(), fixture.resolveQuery(t))
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Applicability != psdomain.CommercialApplicabilityUndetermined {
				t.Fatalf("applicability = %q, want 无法判定", resolution.Applicability)
			}
		})
	}

	unwired := documentsFixture(t, nil, true)
	unwired.fixResolution(t)
	if _, err := unwired.adapter.ResolveCommercialBasis(context.Background(), unwired.resolveQuery(t)); err == nil {
		t.Fatal("规则包读口没接上却没报错")
	}
}
