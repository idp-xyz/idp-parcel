package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证 ADR-0105 的领域半边：问题项带结构化的「涉及序列」主体（种类必有、标识可缺），只在
// REFERENCE_SERIES_UNRESOLVED 与 EXCHANGE_RATE_UNRESOLVED 两处产出点填；主体进快照不进规范化文档，
// 语义摘要对同一评价加主体前后不变；旧快照缺该字段可读回。

func seriesSubjectOf(t testing.TB, evaluation domain.PricingEvaluation, code string) (domain.SeriesSubject, bool) {
	t.Helper()
	for _, issue := range evaluation.Issues() {
		if issue.Code() == code {
			return issue.Series()
		}
	}
	t.Fatalf("no issue with code %s in %v", code, issueCodes(evaluation))
	return domain.SeriesSubject{}, false
}

// Covers: ADR-0105 Decision 一「REFERENCE_SERIES_UNRESOLVED 填绑定的种类与标识」——方案绑了燃油序列而输入
// 没有读数，问题项的主体是（FUEL_RATE，fuel-weekly）；code 与 message 一字不改。
func TestUnresolvedSeriesIssueCarriesTheBoundSeriesAsSubject(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluate(t, "eval-issue-subject", plan, syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch)))
	if evaluation.Status() != domain.EvaluationPending {
		t.Fatalf("status = %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	subject, ok := seriesSubjectOf(t, evaluation, "REFERENCE_SERIES_UNRESOLVED")
	if !ok || subject.Kind() != domain.ReferenceSeriesFuelRate {
		t.Fatalf("subject = %#v ok=%v", subject, ok)
	}
	if id, declared := subject.SeriesID(); !declared || id != "fuel-weekly" {
		t.Fatalf("series id = %q declared=%v", id, declared)
	}
	if !strings.Contains(evaluation.Issues()[0].Message(), "fuel-weekly") {
		t.Fatalf("message changed: %q", evaluation.Issues()[0].Message())
	}
}

// Covers: ADR-0105 Decision 一「EXCHANGE_RATE_UNRESOLVED 只填 EXCHANGE_RATE」——结算币种要汇率而方案没绑汇率序列，
// 主体有种类没有标识。
func TestUnresolvedExchangeRateIssueCarriesOnlyTheKind(t *testing.T) {
	plan := planWithStructures(t, domain.PricingPlanStructures{})
	input, err := syntheticInput(t, "5", "Z1").WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement: %v", err)
	}
	evaluation := evaluate(t, "eval-fx-subject", plan, input)
	subject, ok := seriesSubjectOf(t, evaluation, "EXCHANGE_RATE_UNRESOLVED")
	if !ok || subject.Kind() != domain.ReferenceSeriesExchangeRate {
		t.Fatalf("subject = %#v ok=%v", subject, ok)
	}
	if _, declared := subject.SeriesID(); declared {
		t.Fatal("exchange rate issue invented a series id")
	}
}

// Covers: ADR-0105 Decision 六「不给其他问题项发明主体」——排除、缺尺寸、金额精度未声明这些问题项的主体格为空。
func TestOtherIssuesCarryNoSeriesSubject(t *testing.T) {
	undeclared := evaluate(t, "eval-no-subject", planWithStructures(t, domain.PricingPlanStructures{}), syntheticInput(t, "5", "Z1"))
	if undeclared.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s", undeclared.Status())
	}
	for _, issue := range undeclared.Issues() {
		if _, ok := issue.Series(); ok {
			t.Fatalf("issue %s carries a series subject", issue.Code())
		}
	}
	excluded := evaluate(t, "eval-excluded-no-subject", planWithStructures(t, exclusionStructures(t, "48")), syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "60", "10", "10", domain.LengthUnitInch)))
	if excluded.Status() != domain.EvaluationUnratable {
		t.Fatalf("status = %s %v", excluded.Status(), issueCodes(excluded))
	}
	if _, ok := excluded.Issues()[0].Series(); ok {
		t.Fatal("EXCLUDED_BY_RATE_CARD carries a series subject")
	}
}

// Covers: ADR-0105 Decision 二「主体进快照文档，不进规范化文档；规范化版本不升」——往返保主体；同一评价的语义
// 摘要与一份没有主体字段的旧快照读回后相同；旧快照缺字段可读回，主体为空。
func TestSeriesSubjectSurvivesTheSnapshotAndStaysOutOfTheDigest(t *testing.T) {
	plan := seriesPlan(t, "0.8")
	evaluation := evaluate(t, "eval-issue-snapshot", plan, syntheticInputWithDimensions(t, "5", "Z1", dimensions(t, "50", "10", "10", domain.LengthUnitInch)))
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	subject, ok := seriesSubjectOf(t, restored, "REFERENCE_SERIES_UNRESOLVED")
	if !ok || subject.Kind() != domain.ReferenceSeriesFuelRate {
		t.Fatalf("subject lost across the snapshot: %#v %v", subject, ok)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() || restored.PlanCanonicalizationVersion() != domain.CurrentCanonicalizationVersion() {
		t.Fatal("snapshot round trip moved the semantic digest or the canonicalization version")
	}

	// 把快照里问题项的主体两格剥掉，模拟 ADR-0105 之前落册的评价：读回主体为空，摘要自校照过。
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	issues := document["issues"].([]any)
	for _, entry := range issues {
		issue := entry.(map[string]any)
		delete(issue, "seriesKind")
		delete(issue, "seriesId")
	}
	legacy, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	old, err := domain.RehydrateEvaluationSnapshot(legacy)
	if err != nil {
		t.Fatalf("a snapshot without subjects must still rehydrate: %v", err)
	}
	if _, ok := old.Issues()[0].Series(); ok {
		t.Fatal("a legacy snapshot grew a subject out of nowhere")
	}
	if old.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("the subject leaked into the semantic digest: an old snapshot recomputes a different digest")
	}
}
