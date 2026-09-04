package domain

import (
	"encoding/json"
	"testing"
)

// 本文件从包内钉 ADR-0108 Decision 二在规范化文档上的形状：指纹不进内容摘要与语义摘要，
// 旧快照的 digest 读回落进可选指纹。包内测试才摸得到 fingerprint 字段与规范化函数。

func stampFingerprints(references []VersionReference) []VersionReference {
	stamped := make([]VersionReference, 0, len(references))
	for _, reference := range references {
		reference.fingerprint = "sha256:" + reference.id + "-" + reference.version
		stamped = append(stamped, reference)
	}
	return stamped
}

// Covers: 同一份方案，引用带不带指纹，内容摘要逐字相同。
func TestPlanContentDigestIgnoresReferenceFingerprints(t *testing.T) {
	plan, _ := replayIntegrityFixture(t)
	bare := calculatePricingPlanContentDigest(plan)

	printed := plan
	printed.reference.fingerprint = "sha256:plan"
	printed.rateTable.reference.fingerprint = "sha256:table"
	printed.weight.reference.fingerprint = "sha256:weight"
	printed.manifest.references = stampFingerprints(plan.manifest.references)

	if got := calculatePricingPlanContentDigest(printed); got != bare {
		t.Fatalf("内容摘要随指纹变了：%s vs %s", got, bare)
	}
}

// Covers: 同一次评价，事实引用与清单带不带指纹，语义摘要逐字相同。
func TestEvaluationSemanticDigestIgnoresReferenceFingerprints(t *testing.T) {
	plan, input := replayIntegrityFixture(t)
	id, _ := NewEvaluationID("eval-fingerprint-agnostic")
	request, err := NewEvaluationRequest(id, plan, input, EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	evaluation := EvaluatePricing(request)
	bare := evaluation.calculateSemanticDigest()

	printed := evaluation
	printed.planReference.fingerprint = "sha256:plan"
	printed.manifest.references = stampFingerprints(evaluation.manifest.references)
	facts := make([]VersionedFactReference, 0, len(evaluation.input.factReferences))
	for _, fact := range evaluation.input.factReferences {
		fact.reference.fingerprint = "sha256:fact"
		facts = append(facts, fact)
	}
	printed.input.factReferences = facts

	if got := printed.calculateSemanticDigest(); got != bare {
		t.Fatalf("语义摘要随指纹变了：%s vs %s", got, bare)
	}
}

// Covers: ADR-0108 Consequences 点名的兼容读法——旧形状快照只有 digest 那一格，读回放进可选
// 指纹；新形状写 fingerprint 不再写 digest；两格都在时以 fingerprint 为准。
func TestSnapshotReferenceReadsLegacyDigestIntoTheFingerprint(t *testing.T) {
	var legacy versionReferenceSnapshot
	if err := json.Unmarshal([]byte(`{"kind":"rate-table","id":"table-a","version":"v1","digest":"sha256:old"}`), &legacy); err != nil {
		t.Fatalf("解旧形状：%v", err)
	}
	if got := versionReferenceFrom(legacy); got.fingerprint != "sha256:old" || got.id != "table-a" {
		t.Fatalf("旧 digest 没落进指纹：%+v", got)
	}

	both := versionReferenceSnapshot{Kind: "rate-table", ID: "table-a", Version: "v1", Fingerprint: "sha256:new", Digest: "sha256:old"}
	if got := versionReferenceFrom(both); got.fingerprint != "sha256:new" {
		t.Fatalf("两格都在时应以 fingerprint 为准，实得 %q", got.fingerprint)
	}

	written, err := json.Marshal(versionReferenceOf(VersionReference{kind: ArtifactRateTable, id: "table-a", version: "v1", fingerprint: "sha256:new"}))
	if err != nil {
		t.Fatalf("写新形状：%v", err)
	}
	if string(written) != `{"kind":"rate-table","id":"table-a","version":"v1","fingerprint":"sha256:new"}` {
		t.Fatalf("新形状不该再写 digest：%s", written)
	}
	bareWritten, err := json.Marshal(versionReferenceOf(VersionReference{kind: ArtifactRateTable, id: "table-a", version: "v1"}))
	if err != nil {
		t.Fatalf("写无指纹形状：%v", err)
	}
	if string(bareWritten) != `{"kind":"rate-table","id":"table-a","version":"v1"}` {
		t.Fatalf("无指纹时两格都该省略：%s", bareWritten)
	}
}

// Covers: ADR-0108 Decision 五——内置数值口径引用只带三元，不再把 `builtin:` 常量装进 digest 槽。
func TestNumericProfileReferenceCarriesNoFingerprint(t *testing.T) {
	if reference := NumericProfileV1Reference(); reference.HasFingerprint() || !reference.valid() {
		t.Fatalf("内置口径引用 = %+v", reference)
	}
}
