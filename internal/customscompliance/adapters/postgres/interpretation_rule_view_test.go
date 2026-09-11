package postgres_test

import (
	"testing"
	"time"

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

// loadReleaseRuleAt 是放行层 JURIS/DE 支上的读口短手；本文件全部播种都落在这一支。
func loadReleaseRuleAt(
	t *testing.T,
	view *adapter.InterpretationRuleView,
	tenant string,
	evaluatedAt time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	t.Helper()
	return view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), domain.ReleaseResultLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"), evaluatedAt)
}

func TestInterpretationRuleIsUnconfiguredWhenTheLayerHasNoRule(t *testing.T) {
	view, _ := newInterpretationRuleView(t)
	_, found, err := loadReleaseRuleAt(t, view, "tenant-a", viewBaseAt)
	if err != nil || found {
		t.Fatalf("没配置却答出了规则：err=%v found=%v", err, found)
	}
}

// 半开区间解析（ADR-0070 问一甲）：起点之前无版本、闭区间内取本版、终点起换后继、
// 开放尾段一直答当前版。终点等于评估时点的版本已不再适用——与义务盘点同一口径。
func TestInterpretationRuleResolvesByHalfOpenValidity(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	successionAt := viewBaseAt.Add(48 * time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, applies_until, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, $2, 'interpret/release/v1')`,
		viewBaseAt, successionAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v2')`,
		successionAt)

	if _, found, err := loadReleaseRuleAt(t, view, "tenant-a", viewBaseAt.Add(-time.Second)); err != nil || found {
		t.Fatalf("首版起点之前竟然有版本可答：err=%v found=%v", err, found)
	}
	early, foundEarly, err := loadReleaseRuleAt(t, view, "tenant-a", viewBaseAt.Add(time.Hour))
	if err != nil || !foundEarly || early.String() != "interpret/release/v1" {
		t.Fatalf("旧区间时点没解析回 v1：err=%v found=%v rule=%s", err, foundEarly, early)
	}
	atBoundary, foundBoundary, err := loadReleaseRuleAt(t, view, "tenant-a", successionAt)
	if err != nil || !foundBoundary || atBoundary.String() != "interpret/release/v2" {
		t.Fatalf("换版边界该归后继（半开区间）：err=%v found=%v rule=%s", err, foundBoundary, atBoundary)
	}
	tail, foundTail, err := loadReleaseRuleAt(t, view, "tenant-a", successionAt.Add(365*24*time.Hour))
	if err != nil || !foundTail || tail.String() != "interpret/release/v2" {
		t.Fatalf("开放尾段没答当前版：err=%v found=%v rule=%s", err, foundTail, tail)
	}
}

// 分层保存那条硬句在读口上的样子：配了放行层不等于配了处置层。串层会让一份处置决定
// 按放行的口径被解释。
func TestOneLayersRuleDoesNotAnswerForAnother(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v2')`,
		viewBaseAt)

	if _, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.DispositionDecisionLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
		viewBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("放行层的规则替处置层作了答：err=%v found=%v", err, found)
	}
}

// 辖区维隔离：JURIS/DE 的版本不替 JURIS/US 作答——多辖区租户下按错辖区选版正是
// CONTEXT「不能统一替代规则的法定适用时点」要挡的实错，单辖区期间这一维也不许折叠掉（ADR-0070 问三丙的错法）。
func TestOneJurisdictionsRuleDoesNotAnswerForAnother(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v2')`,
		viewBaseAt)

	if _, found, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/US"),
		viewBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("跨辖区可见：err=%v found=%v", err, found)
	}
}

func TestInterpretationRuleOfAnotherTenantIsInvisible(t *testing.T) {
	view, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v2')`,
		viewBaseAt)

	if _, found, err := loadReleaseRuleAt(t, view, "tenant-b", viewBaseAt.Add(time.Hour)); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}

// 封闭六层之外的层、空辖区与零评估时点都不静默答「未配置」——那是编程错误，与等
// 实例参数是两回事。
func TestBadSelectionInputsAreLoudRatherThanUnconfigured(t *testing.T) {
	view, _ := newInterpretationRuleView(t)
	if _, _, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ResultLayerInvalid,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"), viewBaseAt); err == nil {
		t.Fatal("非法结果层被当成了未配置")
	}
	if _, _, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
		domain.RegulatoryJurisdictionReference{}, viewBaseAt); err == nil {
		t.Fatal("空辖区被当成了未配置")
	}
	if _, _, err := view.LoadInterpretationRule(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"), domain.ReleaseResultLayer,
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"), time.Time{}); err == nil {
		t.Fatal("零评估时点被当成了未配置")
	}
}

func TestInterpretationRuleCheckRejectsASynthesizedLayer(t *testing.T) {
	_, fixture := newInterpretationRuleView(t)
	fixture.rejects(t, "封闭六层之外的合成层",
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'CLEARANCE_SUCCEEDED', 'JURIS/DE', $1, 'interpret/whatever')`,
		viewBaseAt)
}

// 迁移的排他约束在旁路写入下也守着不重叠：与开放版同支且更晚起点的直插被拒——正道
// 是写口的换版（先给前版落终点）。
func TestOverlapCheckRejectsABypassingInsert(t *testing.T) {
	_, fixture := newInterpretationRuleView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v1')`,
		viewBaseAt)
	fixture.rejects(t, "与开放版重叠的旁路直插",
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ('tenant-a', 'RELEASE_RESULT', 'JURIS/DE', $1, 'interpret/release/v2')`,
		viewBaseAt.Add(48*time.Hour))
}
