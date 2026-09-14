package domain_test

import (
	"encoding/json"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件钉票 sa-cc/11 裁决 2 的三句话：回指随评价入册并可按它读回；回指不是计算输入——语义摘要与方案内容
// 摘要不因它而变；旧快照没有这一格照样读回、回指为空。

func referencedRequest(t *testing.T, id string, plan domain.PricingPlanVersion, input domain.PricingInputSnapshot) domain.EvaluationRequest {
	t.Helper()
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("evaluation request: %v", err)
	}
	referenced, err := request.WithRequestReference(mustValue(t, domain.NewEvaluationRequestReference, "EVREQ-SYN-1"))
	if err != nil {
		t.Fatalf("with request reference: %v", err)
	}
	return referenced
}

func TestRequestReferenceRidesIntoTheEvaluationWithoutTouchingDigests(t *testing.T) {
	plan := syntheticPlan(t, "buy-ref", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "10", domain.PricingWeightActualOnly, nil)
	input := syntheticInput(t, "5", "Z1")

	plain := evaluate(t, "eval-ref", plan, input)
	referenced := domain.EvaluatePricing(referencedRequest(t, "eval-ref", plan, input))

	if _, has := plain.RequestReference(); has {
		t.Fatal("PP 内部形成的评价不该带回指")
	}
	reference, has := referenced.RequestReference()
	if !has || reference.String() != "EVREQ-SYN-1" {
		t.Fatalf("回指没随评价带出：%q present=%v", reference, has)
	}
	if referenced.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %v", referenced.Status(), referenced.Issues())
	}
	if referenced.SemanticDigest() != plain.SemanticDigest() {
		t.Fatalf("回指改变了语义摘要：\n带=%s\n不带=%s", referenced.SemanticDigest(), plain.SemanticDigest())
	}
	if referenced.PlanContentDigest() != plain.PlanContentDigest() {
		t.Fatal("回指改变了方案内容摘要")
	}
}

func TestRequestReferenceSurvivesTheSnapshotRoundTrip(t *testing.T) {
	plan := syntheticPlan(t, "buy-ref", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "10", domain.PricingWeightActualOnly, nil)
	referenced := domain.EvaluatePricing(referencedRequest(t, "eval-ref", plan, syntheticInput(t, "5", "Z1")))

	raw, err := domain.MarshalEvaluationSnapshot(referenced)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rebuilt, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	reference, has := rebuilt.RequestReference()
	if !has || reference.String() != "EVREQ-SYN-1" {
		t.Fatalf("回指没从快照读回：%q present=%v", reference, has)
	}
	if rebuilt.SemanticDigest() != referenced.SemanticDigest() {
		t.Fatal("读回的语义摘要与写入不同答")
	}

	// 旧形状：抹掉那一格再读，回指为空，其余照旧——快照多一格可缺席字段，不换规范化版本。
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	if _, present := document["requestReference"]; !present {
		t.Fatal("快照没写出 requestReference")
	}
	delete(document, "requestReference")
	legacy, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	fromLegacy, err := domain.RehydrateEvaluationSnapshot(legacy)
	if err != nil {
		t.Fatalf("rehydrate legacy: %v", err)
	}
	if _, has := fromLegacy.RequestReference(); has {
		t.Fatal("旧快照读回竟长出回指")
	}
	if fromLegacy.SemanticDigest() != referenced.SemanticDigest() {
		t.Fatal("旧快照读回的语义摘要与带回指的不同答——回指进了摘要")
	}
}

func TestWithInputKeepsEverythingButTheInput(t *testing.T) {
	plan := syntheticPlan(t, "buy-ref", domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "10", domain.PricingWeightActualOnly, nil)
	request := referencedRequest(t, "eval-ref", plan, syntheticInput(t, "5", "Z1")).WithSeriesResolutionNotes("series note")

	swapped, err := request.WithInput(syntheticInput(t, "7", "Z1"))
	if err != nil {
		t.Fatalf("with input: %v", err)
	}
	if swapped.Input().ActualWeight().Value().String() != "7" {
		t.Fatalf("输入没换：%s", swapped.Input().ActualWeight().Value())
	}
	if swapped.ID() != request.ID() || swapped.Evidence() != request.Evidence() || swapped.Plan().ContentDigest() != plan.ContentDigest() {
		t.Fatal("标识 / 证据层级 / 方案没原样保留")
	}
	if reference, has := swapped.RequestReference(); !has || reference.String() != "EVREQ-SYN-1" {
		t.Fatal("回指在换输入时丢了")
	}
	explanation := domain.EvaluatePricing(swapped).Explanation()
	if len(explanation) == 0 || explanation[0] != "series note" {
		t.Fatalf("解析说明在换输入时丢了：%v", explanation)
	}

	if _, err := request.WithInput(domain.PricingInputSnapshot{}); err == nil {
		t.Fatal("零值输入被接受了")
	}
	if _, err := request.WithRequestReference(domain.EvaluationRequestReference{}); err == nil {
		t.Fatal("零值回指被接受了")
	}
}
