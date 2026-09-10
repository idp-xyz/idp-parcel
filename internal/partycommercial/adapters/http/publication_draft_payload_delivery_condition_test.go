package commercialhttp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对交付条件一节的运营操作者面载荷（票 admin-write-faces/25）证传输面：产品层挂在 serviceProduct 格下、合同层挂在
// customerContract 格下，节内键名镜像受控批文；逐格问题收齐；层与 tightens 的配对、零方式、同方式两行由领域在预览上答成
// `未受理`带成因；不带这一节的服务产品载荷仍是壳、摘要与票 09 的两格串同。

const serviceProductWithDeliveryConditionsPayload = `{
  "kind": "SERVICE_PRODUCT",
  "objectId": "product-1",
  "version": "v2",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "serviceProduct": {
    "deliveryConditions": {
      "methods": ["METHOD/safe-drop", "METHOD/in-person"],
      "recipientScopeRule": "RULE/recipient-scope-1",
      "proofOfDeliveryRule": "RULE/proof-1"
    }
  }
}`

const customerContractWithDeliveryConditionsPayload = `{
  "kind": "CUSTOMER_CONTRACT",
  "objectId": "contract-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "references": {"ACCEPTANCE_RULE_PACKAGE": "rules-1"},
  "customerContract": {
    "contractContent": {"rulePackage": "rules-1"},
    "deliveryConditions": {
      "tightens": {"objectId": "product-1", "version": "v2"},
      "methods": ["METHOD/in-person"],
      "recipientScopeRule": "RULE/recipient-scope-1",
      "proofOfDeliveryRule": "RULE/proof-contract-1"
    }
  }
}`

func previewOf(t *testing.T, raw string) application.CommercialPublicationPreview {
	t.Helper()
	command, err := decodePublication(t, raw).PreviewCommand(pcNew(t, domain.NewTenantID, pcTenant))
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	answer, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	return answer
}

// Covers: 两册各自的正文格折成领域正文——产品层三格到达、不带 tightens；合同层第三层带所收紧的产品版本到达、与另两层同居
// 一格；预览算出的摘要与直接用领域值对象算出的逐字节同（载荷层没有第二处算摘要），且带本节与不带本节不共一个串。
func TestDeliveryConditionPayloadTranslatesBothRegisters(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	_, product, err := decodePublication(t, serviceProductWithDeliveryConditionsPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("product publication: %v", err)
	}
	if product.Kind != domain.ServiceProductObject || product.ServiceProduct == nil || product.ServiceProduct.DeliveryConditions == nil {
		t.Fatalf("product content = %#v", product)
	}
	conditions := product.ServiceProduct.DeliveryConditions
	if conditions.Tightens != nil || len(conditions.Terms.Methods) != 2 || conditions.Terms.Methods[0].String() != "METHOD/safe-drop" ||
		conditions.Terms.RecipientScopeRule.String() != "RULE/recipient-scope-1" || conditions.Terms.ProofOfDeliveryRule.String() != "RULE/proof-1" {
		t.Fatalf("product layer = %#v", conditions)
	}
	previewed, _ := previewOf(t, serviceProductWithDeliveryConditionsPayload).Canonical()
	direct, err := domain.CanonicalizePublicationContent(product)
	if err != nil {
		t.Fatalf("canonicalize product: %v", err)
	}
	if previewed.Digest() != direct.Digest() || !strings.HasPrefix(previewed.Digest().String(), "PCC-1:") {
		t.Fatalf("preview digest %s ≠ domain digest %s", previewed.Digest(), direct.Digest())
	}
	bare, _ := previewOf(t, serviceProductPayload).Canonical()
	if bare.Digest() == previewed.Digest() {
		t.Fatal("a product layer section must change the service product digest")
	}

	_, contract, err := decodePublication(t, customerContractWithDeliveryConditionsPayload).Publication(tenant)
	if err != nil {
		t.Fatalf("contract publication: %v", err)
	}
	if contract.CustomerContract == nil || contract.CustomerContract.DeliveryConditions == nil || contract.CustomerContract.DeliveryConditions.Tightens == nil {
		t.Fatalf("contract content = %#v", contract)
	}
	layer := contract.CustomerContract.DeliveryConditions
	if layer.Tightens.ObjectID().String() != "product-1" || layer.Tightens.Version().String() != "v2" ||
		len(layer.Terms.Methods) != 1 || layer.Terms.ProofOfDeliveryRule.String() != "RULE/proof-contract-1" {
		t.Fatalf("contract layer = %#v", layer)
	}
	contractPreview, _ := previewOf(t, customerContractWithDeliveryConditionsPayload).Canonical()
	directContract, err := domain.CanonicalizePublicationContent(contract)
	if err != nil {
		t.Fatalf("canonicalize contract: %v", err)
	}
	if contractPreview.Digest() != directContract.Digest() {
		t.Fatalf("contract preview digest %s ≠ domain digest %s", contractPreview.Digest(), directContract.Digest())
	}
	withoutLayer := strings.Replace(customerContractWithDeliveryConditionsPayload, `,
    "deliveryConditions": {
      "tightens": {"objectId": "product-1", "version": "v2"},
      "methods": ["METHOD/in-person"],
      "recipientScopeRule": "RULE/recipient-scope-1",
      "proofOfDeliveryRule": "RULE/proof-contract-1"
    }`, "", 1)
	withoutPreview, _ := previewOf(t, withoutLayer).Canonical()
	if withoutPreview.Digest() == contractPreview.Digest() {
		t.Fatal("dropping the contract layer must change the contract digest")
	}
}

// Covers: 逐格问题收齐——方式某项空串、两条规则引用空、tightens 两格空各记在自己那一格（产品层与合同层的路径各自带自己的
// 格根）；成立的格不被点名；不带本节的服务产品载荷仍只有壳，一格问题都没有。
func TestDeliveryConditionPayloadCollectsEveryFieldProblem(t *testing.T) {
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	product := strings.Replace(strings.Replace(serviceProductWithDeliveryConditionsPayload,
		`"methods": ["METHOD/safe-drop", "METHOD/in-person"]`, `"methods": ["METHOD/safe-drop", " "]`, 1),
		`"recipientScopeRule": "RULE/recipient-scope-1"`, `"recipientScopeRule": ""`, 1)
	_, _, err := decodePublication(t, product).Publication(tenant)
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("product err = %v, want *PublicationPayloadProblems", err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	for _, want := range []string{"serviceProduct.deliveryConditions.methods[1]", "serviceProduct.deliveryConditions.recipientScopeRule"} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for _, unwanted := range []string{"serviceProduct.deliveryConditions.methods[0]", "serviceProduct.deliveryConditions.proofOfDeliveryRule"} {
		if fields[unwanted] {
			t.Errorf("a valid field was reported: %v", fields)
		}
	}

	contract := strings.Replace(customerContractWithDeliveryConditionsPayload,
		`"tightens": {"objectId": "product-1", "version": "v2"}`, `"tightens": {"objectId": "", "version": " "}`, 1)
	_, _, err = decodePublication(t, contract).Publication(tenant)
	if !errors.As(err, &problems) {
		t.Fatalf("contract err = %v, want *PublicationPayloadProblems", err)
	}
	fields = map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	if !fields["customerContract.deliveryConditions.tightens.objectId"] || !fields["customerContract.deliveryConditions.tightens.version"] {
		t.Fatalf("tightens problems missing: %v", fields)
	}
	if fields["customerContract.deliveryConditions.tightens"] || fields["customerContract.deliveryConditions.methods[0]"] {
		t.Fatalf("a valid field was reported: %v", fields)
	}

	if _, _, err := decodePublication(t, serviceProductPayload).Publication(tenant); err != nil {
		t.Fatalf("a shell-only service product must still decode without problems: %v", err)
	}
}

// Covers: 层与 tightens 的配对、零方式、同方式两行不是解码问题，是领域折成文档时的门（与发布用例按版本类别选层同一判据）：
// 产品层带 tightens 答 ErrDeliveryConditionOwner，合同层缺 tightens 答 ErrDeliveryConditionNotConfigured，零方式答
// ErrDeliveryConditionNotConfigured，同方式两行答 ErrConflictingDeliveryCondition——预览一律`未受理`带成因、不带摘要。
func TestDeliveryConditionPreviewAnswersTheDomainGateForLayering(t *testing.T) {
	refused := func(t *testing.T, raw string, want error) {
		t.Helper()
		answer := previewOf(t, raw)
		if answer.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(answer.RefusalCause(), want) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / %v", answer.Outcome(), answer.RefusalCause(), want)
		}
		if _, present := answer.Canonical(); present {
			t.Fatal("a refused preview must not carry a digest")
		}
	}

	productWithTightens := strings.Replace(serviceProductWithDeliveryConditionsPayload, `"deliveryConditions": {`,
		`"deliveryConditions": {"tightens": {"objectId": "product-0", "version": "v1"},`, 1)
	refused(t, productWithTightens, domain.ErrDeliveryConditionOwner)

	contractWithoutTightens := strings.Replace(customerContractWithDeliveryConditionsPayload,
		`"tightens": {"objectId": "product-1", "version": "v2"},`, "", 1)
	refused(t, contractWithoutTightens, domain.ErrDeliveryConditionNotConfigured)

	zeroMethods := strings.Replace(serviceProductWithDeliveryConditionsPayload, `"methods": ["METHOD/safe-drop", "METHOD/in-person"]`, `"methods": []`, 1)
	refused(t, zeroMethods, domain.ErrDeliveryConditionNotConfigured)

	duplicated := strings.Replace(serviceProductWithDeliveryConditionsPayload, `"methods": ["METHOD/safe-drop", "METHOD/in-person"]`, `"methods": ["METHOD/in-person", "METHOD/in-person"]`, 1)
	refused(t, duplicated, domain.ErrConflictingDeliveryCondition)

	// 别册的正文格冒本册的名：壳说 CUSTOMER_CONTRACT、正文格是 serviceProduct，按 kind 不符拒。
	mislabelled := strings.Replace(serviceProductWithDeliveryConditionsPayload, `"kind": "SERVICE_PRODUCT"`, `"kind": "CUSTOMER_CONTRACT"`, 1)
	refused(t, mislabelled, domain.ErrPublicationContentKindMismatch)
}

// Covers: ADR-0126 Decision 四对带本节的服务产品成立——同一份载荷过预览与过录入逐字节同摘要；录入的载体折回后仍带这一节
// （文档快照带上了它，发布时才折得回 DELIVERY_CONDITION 通道）。
func TestDeliveryConditionPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, serviceProductWithDeliveryConditionsPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	previewed, _ := previewOf(t, serviceProductWithDeliveryConditionsPayload).Canonical()

	store := newDraftStore()
	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitted, err := application.NewSubmitPublicationDraftHandler(store, pcClock{at: pcNow}).Handle(context.Background(), submitCommand)
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("submit = %q, %v", submitted.Outcome(), err)
	}
	draft, _ := submitted.Draft()
	if draft.Canonical().Digest() != previewed.Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest %s", previewed.Digest(), draft.Canonical().Digest())
	}
	content, err := domain.RehydratePublicationContent(draft.Canonical().Canonicalization(), draft.Canonical().Document())
	if err != nil || content.ServiceProduct == nil || content.ServiceProduct.DeliveryConditions == nil || len(content.ServiceProduct.DeliveryConditions.Terms.Methods) != 2 {
		t.Fatalf("the snapshot lost the section: %#v, %v", content, err)
	}
}
