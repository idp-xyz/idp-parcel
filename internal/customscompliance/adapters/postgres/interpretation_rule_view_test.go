package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newInterpretationRuleView(t *testing.T) (*adapter.InterpretationRuleView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewInterpretationRuleView(fixture.db)
	if err != nil {
		t.Fatalf("构造解释规则读口：%v", err)
	}
	return view, fixture
}

func TestInterpretationRuleIsUnconfiguredWhenTheLayerHasNoRule(t *testing.T) {
	view, _ := newInterpretationRuleView(t)
	_, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer)
	if err != nil || found {
		t.Fatalf("没配置却答出了规则：err=%v found=%v", err, found)
	}
}

func TestInterpretationRuleIsReadBackForItsLayer(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule (tenant_id, result_layer, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'interpret/release/v2')`)

	rule, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer)
	if err != nil || !found || rule.String() != "interpret/release/v2" {
		t.Fatalf("规则没读回：err=%v found=%v rule=%s", err, found, rule)
	}
}

// 分层保存那条硬句在读口上的样子：配了放行层不等于配了处置层。串层会让一份处置决定
// 按放行的口径被解释。
func TestOneLayersRuleDoesNotAnswerForAnother(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule (tenant_id, result_layer, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'interpret/release/v2')`)

	if _, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.DispositionDecisionLayer); err != nil || found {
		t.Fatalf("放行层的规则替处置层作了答：err=%v found=%v", err, found)
	}
}

func TestInterpretationRuleOfAnotherTenantIsInvisible(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule (tenant_id, result_layer, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'interpret/release/v2')`)

	if _, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-b"), domain.ReleaseResultLayer); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}

// 封闭六层之外的层取不出规则，也不静默答「未配置」——那是编程错误，与等实例参数
// 是两回事。
func TestAnUnknownResultLayerIsLoudRatherThanUnconfigured(t *testing.T) {
	view, _ := newInterpretationRuleView(t)
	if _, _, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ResultLayerInvalid); err == nil {
		t.Fatal("非法结果层被当成了未配置")
	}
}

func TestInterpretationRuleCheckRejectsASynthesizedLayer(t *testing.T) {
	_, fixture := newInterpretationRuleView(t)
	fixture.rejects(t, "封闭六层之外的合成层",
		`INSERT INTO customs_compliance.interpretation_rule (tenant_id, result_layer, rule_ref)
		 VALUES ('tenant-a', 'CLEARANCE_SUCCEEDED', 'interpret/whatever')`)
}
