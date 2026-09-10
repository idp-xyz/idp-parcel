package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证交付条件声明折进 PCC-1 的形（票 admin-write-faces/25）：两册各长一节、omitempty 不换号、键名镜像批文、
// 方式稳定序、折回再算同串；层与拥有它的册对不上、三格缺、同方式两行，答与发布时同一格。夹具 deliveryTerms / tightens
// 复用 delivery_condition_test.go 的（两条规则引用固定为 RULE/recipient-scope 与 RULE/proof-of-delivery）。

func productDeliveryConditions(t *testing.T, methods ...string) *domain.DeliveryConditionBody {
	t.Helper()
	return &domain.DeliveryConditionBody{Terms: deliveryTerms(t, methods...)}
}

func contractDeliveryConditions(t *testing.T, methods ...string) *domain.DeliveryConditionBody {
	t.Helper()
	target := tightens(t, "product-1", "v1")
	return &domain.DeliveryConditionBody{Tightens: &target, Terms: deliveryTerms(t, methods...)}
}

func canonicalServiceProductWith(t *testing.T, body *domain.ServiceProductBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.ServiceProductObject, ServiceProduct: body})
	if err != nil {
		t.Fatalf("canonicalize service product: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一「加键不换号」——服务产品册从「没有正文」变成「正文可缺」后，不带交付条件的版本文档
// 仍恰是 {canonicalization, kind} 两格：nil 正文与零值正文折出同一个串、与本节存在之前的串同；带上一节才是另一个串。
func TestServiceProductWithoutDeliveryConditionsStillCanonicalizesToTwoFields(t *testing.T) {
	bare := canonicalServiceProduct(t)
	empty := canonicalServiceProductWith(t, &domain.ServiceProductBody{})
	if bare.Digest() != empty.Digest() {
		t.Fatalf("nil body and zero body differ: %s vs %s", bare.Digest(), empty.Digest())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(empty.Document(), &document); err != nil {
		t.Fatalf("document is not JSON: %v", err)
	}
	if len(document) != 2 {
		t.Fatalf("document = %s; want exactly {canonicalization, kind}", empty.Document())
	}

	declared := canonicalServiceProductWith(t, &domain.ServiceProductBody{DeliveryConditions: productDeliveryConditions(t, "METHOD/in-person")})
	if declared.Digest() == bare.Digest() {
		t.Fatal("a product layer declaration must change the digest")
	}
	if declared.Canonicalization() != "PCC-1" || !strings.HasPrefix(declared.Digest().String(), "PCC-1:") {
		t.Fatalf("still PCC-1 after adding a section, got %s", declared.Digest())
	}
}

// Covers: 票 25「键名镜像批文」——产品层一节落在 serviceProduct.deliveryConditions 下，三格键名与批文
// deliveryConditionDocument 同名、没有 tightens 键；方式按字面稳定序写出，表单里换行序不换摘要。
func TestServiceProductDeliveryConditionsMirrorTheBatchKeysAndSortMethods(t *testing.T) {
	ordered := canonicalServiceProductWith(t, &domain.ServiceProductBody{DeliveryConditions: productDeliveryConditions(t, "METHOD/in-person", "METHOD/locker", "METHOD/safe-drop")})
	shuffled := canonicalServiceProductWith(t, &domain.ServiceProductBody{DeliveryConditions: productDeliveryConditions(t, "METHOD/safe-drop", "METHOD/in-person", "METHOD/locker")})
	if ordered.Digest() != shuffled.Digest() {
		t.Fatalf("method order changed the digest: %s vs %s", ordered.Digest(), shuffled.Digest())
	}

	var document struct {
		Kind           string `json:"kind"`
		ServiceProduct struct {
			DeliveryConditions map[string]json.RawMessage `json:"deliveryConditions"`
		} `json:"serviceProduct"`
	}
	if err := json.Unmarshal(shuffled.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", shuffled.Document(), err)
	}
	conditions := document.ServiceProduct.DeliveryConditions
	if document.Kind != "SERVICE_PRODUCT" || conditions == nil {
		t.Fatalf("document = %s", shuffled.Document())
	}
	if string(conditions["methods"]) != `["METHOD/in-person","METHOD/locker","METHOD/safe-drop"]` {
		t.Fatalf("methods = %s; want sorted by reference text", conditions["methods"])
	}
	if string(conditions["recipientScopeRule"]) != `"RULE/recipient-scope"` || string(conditions["proofOfDeliveryRule"]) != `"RULE/proof-of-delivery"` {
		t.Fatalf("rule references = %s / %s", conditions["recipientScopeRule"], conditions["proofOfDeliveryRule"])
	}
	if _, present := conditions["tightens"]; present {
		t.Fatalf("a product layer must not carry a tightens key: %s", shuffled.Document())
	}
}

// Covers: 客户合同册第三层——deliveryConditions 与 contractContent / preAcceptanceControl 并列；在场时带 tightens 两键，
// 缺席时整键缺席（不带这一层的合同正文字节与本节存在之前同）；两层的正文各是另一个串。
func TestCustomerContractDeliveryConditionsAreAThirdOptionalLayer(t *testing.T) {
	prepaid := appliedControl(t, "charge-prepaid")
	without := canonicalCustomerContract(t, customerContractBody(t, requiredControl(), prepaid))
	if strings.Contains(string(without.Document()), "deliveryConditions") {
		t.Fatalf("an undeclared contract layer must omit the whole key: %s", without.Document())
	}

	body := customerContractBody(t, requiredControl(), prepaid)
	body.DeliveryConditions = contractDeliveryConditions(t, "METHOD/in-person")
	with := canonicalCustomerContract(t, body)
	if with.Digest() == without.Digest() {
		t.Fatal("declaring a contract layer must change the digest")
	}

	var document struct {
		CustomerContract struct {
			DeliveryConditions struct {
				Tightens map[string]string `json:"tightens"`
				Methods  []string          `json:"methods"`
			} `json:"deliveryConditions"`
		} `json:"customerContract"`
	}
	if err := json.Unmarshal(with.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", with.Document(), err)
	}
	conditions := document.CustomerContract.DeliveryConditions
	if conditions.Tightens["objectId"] != "product-1" || conditions.Tightens["version"] != "v1" || len(conditions.Methods) != 1 {
		t.Fatalf("deliveryConditions = %#v", conditions)
	}
}

// Covers: 折成文档前过的是与发布时同一套门——层与册对不上（产品层带 tightens / 合同层缺 tightens）答
// ErrDeliveryConditionOwner，零方式或规则引用缺席答 ErrDeliveryConditionNotConfigured，同方式两行答
// ErrConflictingDeliveryCondition；服务产品正文冒客户合同的名照旧 kind 不符。
func TestDeliveryConditionCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	product := &domain.ServiceProductBody{DeliveryConditions: productDeliveryConditions(t, "METHOD/in-person")}
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject, ServiceProduct: product})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("product body under contract kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	productRefusals := map[string]struct {
		body *domain.DeliveryConditionBody
		want error
	}{
		"产品层带 tightens": {body: contractDeliveryConditions(t, "METHOD/in-person"), want: domain.ErrDeliveryConditionOwner},
		"零方式":           {body: productDeliveryConditions(t), want: domain.ErrDeliveryConditionNotConfigured},
		"同方式两行":         {body: productDeliveryConditions(t, "METHOD/in-person", "METHOD/in-person"), want: domain.ErrConflictingDeliveryCondition},
		"规则引用缺席": {
			body: &domain.DeliveryConditionBody{Terms: domain.DeliveryConditionTerms{
				Methods: []domain.DeliveryMethodReference{commercialValue(t, domain.NewDeliveryMethodReference, "METHOD/in-person")},
			}},
			want: domain.ErrDeliveryConditionNotConfigured,
		},
	}
	for name, refusal := range productRefusals {
		t.Run("产品层 "+name, func(t *testing.T) {
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
				Kind:           domain.ServiceProductObject,
				ServiceProduct: &domain.ServiceProductBody{DeliveryConditions: refusal.body},
			})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}

	contract := customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))
	contract.DeliveryConditions = productDeliveryConditions(t, "METHOD/in-person")
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CustomerContractObject, CustomerContract: &contract})
	if !errors.Is(err, domain.ErrDeliveryConditionOwner) {
		t.Fatalf("contract layer without tightens: err = %v, want ErrDeliveryConditionOwner", err)
	}
}

// Covers: ADR-0126 Decision 三（正文快照）——两册的文档都折得回带交付条件的正文，折回去再算一遍与列里的摘要相等；
// 快照里坏一格（同方式两行）折不回，快照是数据，正文立不立得住仍由构造门说。
func TestDeliveryConditionDocumentsRehydrateToTheSameBody(t *testing.T) {
	product := canonicalServiceProductWith(t, &domain.ServiceProductBody{DeliveryConditions: productDeliveryConditions(t, "METHOD/locker", "METHOD/in-person")})
	content, err := domain.RehydratePublicationContent(product.Canonicalization(), product.Document())
	if err != nil {
		t.Fatalf("rehydrate product: %v", err)
	}
	if content.Kind != domain.ServiceProductObject || content.ServiceProduct == nil || content.ServiceProduct.DeliveryConditions == nil {
		t.Fatalf("rehydrated product content = %#v", content)
	}
	if conditions := content.ServiceProduct.DeliveryConditions; conditions.Tightens != nil || len(conditions.Terms.Methods) != 2 ||
		conditions.Terms.ProofOfDeliveryRule.String() != "RULE/proof-of-delivery" {
		t.Fatalf("rehydrated product layer = %#v", conditions)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize product: %v", err)
	}
	if recomputed.Digest() != product.Digest() {
		t.Fatalf("rehydrated product digests to %s, column says %s", recomputed.Digest(), product.Digest())
	}

	body := customerContractBody(t, requiredControl(), appliedControl(t, "charge-prepaid"))
	body.DeliveryConditions = contractDeliveryConditions(t, "METHOD/in-person")
	contract := canonicalCustomerContract(t, body)
	content, err = domain.RehydratePublicationContent(contract.Canonicalization(), contract.Document())
	if err != nil {
		t.Fatalf("rehydrate contract: %v", err)
	}
	rehydrated := content.CustomerContract
	if rehydrated == nil || rehydrated.DeliveryConditions == nil || rehydrated.DeliveryConditions.Tightens == nil ||
		rehydrated.DeliveryConditions.Tightens.ObjectID().String() != "product-1" {
		t.Fatalf("rehydrated contract content = %#v", content)
	}
	recomputed, err = domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize contract: %v", err)
	}
	if recomputed.Digest() != contract.Digest() {
		t.Fatalf("rehydrated contract digests to %s, column says %s", recomputed.Digest(), contract.Digest())
	}

	corrupted := strings.Replace(string(product.Document()), `"METHOD/locker"`, `"METHOD/in-person"`, 1)
	if _, err := domain.RehydratePublicationContent(product.Canonicalization(), []byte(corrupted)); !errors.Is(err, domain.ErrConflictingDeliveryCondition) {
		t.Fatalf("corrupted snapshot: err = %v, want ErrConflictingDeliveryCondition", err)
	}
}
