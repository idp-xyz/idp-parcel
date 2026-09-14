package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉票 sa-cc/08 的领域半边：自然键由（主要范围 + 计算目的 + 来源引用的排序后摘要）算出、摘要带规范化
// 版本前缀（ADR-0014）；发生项的原因与业务时间不进自然键；成分任一不同就是另一份请求；缺件的请求
// 形成不了；计算目的词表封闭。夹具全是合成串（真实供应商协议与费用项目属实例半边 PAR-SET-03）。

func evaluationRequestSources(t *testing.T, occurrence, version, feeItem, agreement string) domain.EligibleSourceReferences {
	t.Helper()
	charge, err := domain.NewTransportChargeOccurrence(
		settlementValue(t, domain.NewChargeOccurrenceID, occurrence),
		settlementValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		settlementValue(t, domain.NewOccurrenceVersion, version),
		time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	return domain.EligibleSourceReferences{
		Occurrence: charge,
		FeeItem:    settlementValue(t, domain.NewFeeItemReference, feeItem),
		Agreement:  settlementValue(t, domain.NewSupplierAgreementReference, agreement),
	}
}

func evaluationRequestSpec(t *testing.T, id string, sources domain.EligibleSourceReferences) domain.EvaluationRequestSpec {
	t.Helper()
	return domain.EvaluationRequestSpec{
		ID:          settlementValue(t, domain.NewEvaluationRequestID, id),
		Scope:       settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-1"),
		Purpose:     domain.BuySupplierCost,
		Sources:     sources,
		RequestedAt: time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC),
		RequestedBy: settlementValue(t, domain.NewRequesterReference, "syn-settlement-job"),
	}
}

func TestANaturalKeyIsStableAcrossSubmissionsAndCarriesItsCanonicalizationVersion(t *testing.T) {
	sources := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	scope := settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-1")

	first, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, sources)
	if err != nil {
		t.Fatalf("自然键：%v", err)
	}
	second, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, sources)
	if err != nil {
		t.Fatalf("第二次自然键：%v", err)
	}
	if first != second {
		t.Fatalf("同成分两次算出不同自然键：%+v vs %+v", first, second)
	}
	if !strings.HasPrefix(first.SourceDigest, "ESRC-1:") {
		t.Fatalf("摘要没带规范化版本前缀：%q", first.SourceDigest)
	}
	if first.Scope.String() != "syn-scope-1" || first.Purpose != domain.BuySupplierCost {
		t.Fatalf("自然键的范围 / 目的没按成分带出：%+v", first)
	}
}

// 原因与业务时间是 TF 在该发生项版本上钉死的属性，不是第二个身份维：同一发生项版本换一种写法
// 交进来仍是同一份请求。
func TestOccurrenceReasonAndTimeDoNotEnterTheNaturalKey(t *testing.T) {
	scope := settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-1")
	base := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	restated, err := domain.NewTransportChargeOccurrence(
		base.Occurrence.ID(),
		settlementValue(t, domain.NewOccurrenceReasonReference, "CANCELLATION"),
		base.Occurrence.Version(),
		base.Occurrence.OccurredAt().Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("重述发生项：%v", err)
	}
	variant := base
	variant.Occurrence = restated

	baseKey, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, base)
	if err != nil {
		t.Fatalf("自然键：%v", err)
	}
	variantKey, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, variant)
	if err != nil {
		t.Fatalf("变体自然键：%v", err)
	}
	if baseKey != variantKey {
		t.Fatalf("原因 / 业务时间改变了自然键：%q vs %q", baseKey.SourceDigest, variantKey.SourceDigest)
	}
}

func TestAnyDifferentComponentYieldsADifferentNaturalKey(t *testing.T) {
	scope := settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-1")
	base := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	baseKey, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, base)
	if err != nil {
		t.Fatalf("自然键：%v", err)
	}

	variants := map[string]domain.EligibleSourceReferences{
		"另一个发生项":   evaluationRequestSources(t, "syn-occ-2", "v1", "syn-fee-1", "syn-agreement-1"),
		"发生项另一版本":  evaluationRequestSources(t, "syn-occ-1", "v2", "syn-fee-1", "syn-agreement-1"),
		"另一个费用项目":  evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-2", "syn-agreement-1"),
		"另一份供应商协议": evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-2"),
	}
	for name, sources := range variants {
		key, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, sources)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if key.SourceDigest == baseKey.SourceDigest {
			t.Fatalf("%s 与基准算出同一摘要 %q", name, key.SourceDigest)
		}
	}

	otherScope := settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-2")
	scoped, err := domain.EvaluationRequestNaturalKeyOf(otherScope, domain.BuySupplierCost, base)
	if err != nil {
		t.Fatalf("另一范围：%v", err)
	}
	if scoped == baseKey {
		t.Fatal("换了主要范围仍是同一自然键")
	}
}

// 种类标签守的是这一格：两件引用的值互换，不能撞成同一摘要——只把三个裸值排序再拼，
// 「费用项目=X、协议=Y」与「费用项目=Y、协议=X」会算出同一串。
func TestSwappingTwoReferenceValuesDoesNotCollide(t *testing.T) {
	scope := settlementValue(t, domain.NewPrimaryScopeReference, "syn-scope-1")
	straight := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-value-a", "syn-value-b")
	swapped := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-value-b", "syn-value-a")

	straightKey, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, straight)
	if err != nil {
		t.Fatalf("自然键：%v", err)
	}
	swappedKey, err := domain.EvaluationRequestNaturalKeyOf(scope, domain.BuySupplierCost, swapped)
	if err != nil {
		t.Fatalf("互换后自然键：%v", err)
	}
	if straightKey.SourceDigest == swappedKey.SourceDigest {
		t.Fatalf("费用项目与协议的值互换后撞成同一摘要 %q", straightKey.SourceDigest)
	}
}

func TestSubmittingAnEvaluationRequestKeepsWhatWasAsked(t *testing.T) {
	sources := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	spec := evaluationRequestSpec(t, "EVREQ-SYN-1", sources)

	request, err := domain.SubmitEvaluationRequest(spec)
	if err != nil {
		t.Fatalf("提交评价请求：%v", err)
	}
	if request.ID() != spec.ID || request.Scope() != spec.Scope || request.Purpose() != domain.BuySupplierCost {
		t.Fatalf("身份 / 范围 / 目的没按规格带出：%+v", request)
	}
	if request.Sources() != sources {
		t.Fatalf("来源引用没按规格带出：%+v", request.Sources())
	}
	if request.RequestedBy() != spec.RequestedBy || !request.RequestedAt().Equal(spec.RequestedAt) {
		t.Fatalf("请求方 / 请求时刻没按规格带出：%+v", request)
	}
	expected, err := domain.EvaluationRequestNaturalKeyOf(spec.Scope, spec.Purpose, sources)
	if err != nil {
		t.Fatalf("自然键：%v", err)
	}
	if request.NaturalKey() != expected {
		t.Fatalf("请求上的自然键 %+v 与按成分算出的 %+v 不同", request.NaturalKey(), expected)
	}
}

func TestAnEvaluationRequestRefusesAnyMissingPiece(t *testing.T) {
	sources := evaluationRequestSources(t, "syn-occ-1", "v1", "syn-fee-1", "syn-agreement-1")
	valid := evaluationRequestSpec(t, "EVREQ-SYN-1", sources)

	mutations := map[string]func(*domain.EvaluationRequestSpec){
		"缺 ID":   func(spec *domain.EvaluationRequestSpec) { spec.ID = domain.EvaluationRequestID{} },
		"缺主要范围":  func(spec *domain.EvaluationRequestSpec) { spec.Scope = domain.PrimaryScopeReference{} },
		"目的词表外":  func(spec *domain.EvaluationRequestSpec) { spec.Purpose = domain.CalculationPurposeInvalid },
		"缺发生项":   func(spec *domain.EvaluationRequestSpec) { spec.Sources.Occurrence = domain.TransportChargeOccurrence{} },
		"缺费用项目":  func(spec *domain.EvaluationRequestSpec) { spec.Sources.FeeItem = domain.FeeItemReference{} },
		"缺供应商协议": func(spec *domain.EvaluationRequestSpec) { spec.Sources.Agreement = domain.SupplierAgreementReference{} },
		"缺请求方":   func(spec *domain.EvaluationRequestSpec) { spec.RequestedBy = domain.RequesterReference{} },
		"缺请求时刻":  func(spec *domain.EvaluationRequestSpec) { spec.RequestedAt = time.Time{} },
	}
	for name, mutate := range mutations {
		spec := valid
		mutate(&spec)
		if _, err := domain.SubmitEvaluationRequest(spec); !errors.Is(err, domain.ErrInvalidEvaluationRequest) {
			t.Fatalf("%s：应拒为 ErrInvalidEvaluationRequest，实得 %v", name, err)
		}
	}
}

func TestCalculationPurposeVocabularyIsClosed(t *testing.T) {
	parsed, err := domain.ParseCalculationPurpose(domain.BuySupplierCost.String())
	if err != nil || parsed != domain.BuySupplierCost {
		t.Fatalf("词表内的值折不回：%v / %v", parsed, err)
	}
	for _, raw := range []string{"", "SELL_CUSTOMER_CHARGE", "SUPPLIER_COST", "buy_supplier_cost"} {
		if _, err := domain.ParseCalculationPurpose(raw); !errors.Is(err, domain.ErrUnknownCalculationPurpose) {
			t.Fatalf("%q 不在词表里却被接受（err = %v）", raw, err)
		}
	}
}
