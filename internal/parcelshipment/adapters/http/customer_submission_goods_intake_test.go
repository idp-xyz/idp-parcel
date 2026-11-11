package shipmenthttp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// fbaDraft 是一票两箱的 FBA 头程：委托带平台货件段与两行货物，每箱带平台箱标号与装箱分配。
const fbaDraft = `{
  "accessRequestId": "SYN-ACCESS-REQUEST-0001",
  "customerShipmentReference": "SYN-CUSTREF-0001",
  "requestedServiceProduct": "SYN-FBA-AIR",
  "recipientAddress": "合成平台仓地址",
  "platformShipment": {"platform": "AMAZON", "shipmentId": "FBA15SYNTEST", "referenceId": "SYN-REF-1"},
  "goods": {
    "contentCategory": "SALE_OF_GOODS",
    "lines": [
      {"descriptions": [{"lang": "en", "text": "Men's cotton T-shirt"}, {"lang": "zh", "text": "男式棉质T恤"}],
       "quantity": "48", "unit": "PCS", "unitValue": "3.20", "currency": "USD",
       "commodityCodes": [{"side": "EXPORT", "code": "6109100000"}], "originCountry": "CN", "sku": "SKU-TEE-1"},
      {"descriptions": [{"lang": "en", "text": "Wrist watch"}], "quantity": "2", "lineValue": "40.00", "currency": "USD",
       "salesOrder": {"platform": "SHOPIFY", "orderNumber": "1001", "orderLine": "2"}}
    ]
  },
  "parcels": [
    {"customerParcelReference": "SYN-PCLREF-0001", "declaredWeightValue": "12.0", "declaredWeightUnit": "KG",
     "platformBoxLabel": "FBA15SYNTESTU000001", "packing": [{"line": 1, "quantity": "24"}, {"line": 2, "quantity": "2"}]},
    {"customerParcelReference": "SYN-PCLREF-0002", "declaredWeightValue": "11.5", "declaredWeightUnit": "KG",
     "platformBoxLabel": "FBA15SYNTESTU000002", "packing": [{"line": 1, "quantity": "24"}]}
  ]
}`

// Covers: ADR-0373 决定七、ADR-0374 决定三——首方提交口收委托级货物明细、平台货件段与逐箱箱标号、装箱分配；命令里的分配与箱标号
// 挂在现签的内部成员上（同画像），在场的格一律记本单申报；货物明细是内容，进摘要（`AT-PS-188` 的接入半边）。
func TestCustomerSubmissionIntakeCarriesGoodsAndPlatformDeclarationsOnMintedMembers(t *testing.T) {
	intake := newCustomerIntake(t, &customerAuthenticatorDouble{}, &permittingOwnershipDouble{revision: "SYN-REV-1"})
	command, err := intake.IntakeSubmission(context.Background(), draftRequest(fbaDraft))
	if err != nil {
		t.Fatalf("接入 FBA 草案：%v", err)
	}
	members := command.DeclaredParcelIDs
	if len(members) != 2 {
		t.Fatalf("成员 %d 件，want 2", len(members))
	}
	goods := command.DeclaredGoods
	if lines := goods.Lines(); len(lines) != 2 || len(lines[0].Descriptions()) != 2 {
		t.Fatalf("货物明细 = %+v", lines)
	}
	if quantity, _ := goods.Lines()[0].Quantity(); quantity.Value() != "48" || quantity.Mark() != domain.SourceMarkThisDeclaration {
		t.Fatalf("第一行数量 = %q mark %v；客户渠道交来的记本单申报", quantity.Value(), quantity.Mark())
	}
	if category, _, present := goods.Category(); !present || category != domain.ContentCategorySaleOfGoods {
		t.Fatalf("内件性质 = %v present=%v", category, present)
	}
	for _, allocation := range goods.Packing() {
		if allocation.Parcel() != members[0] && allocation.Parcel() != members[1] {
			t.Fatalf("分配指着 %q，不是现签的成员；挂错一侧，版本按内部标识取成员时对不上", allocation.Parcel())
		}
	}
	if shipment := command.PlatformShipment; !shipment.Declared() || shipment.ShipmentNumber().String() != "FBA15SYNTEST" {
		t.Fatalf("平台货件 = %+v", command.PlatformShipment)
	}
	labels := command.PlatformBoxLabels
	if len(labels) != 2 || labels[0].Parcel() != members[0] || labels[1].Number().String() != "FBA15SYNTESTU000002" {
		t.Fatalf("箱标号 = %+v，要挂在现签的成员上、按草案次序", labels)
	}

	otherCategory := strings.Replace(fbaDraft, `"contentCategory": "SALE_OF_GOODS",`, `"contentCategory": "OTHER", "contentCategoryNote": "samples",`, 1)
	other, err := intake.IntakeSubmission(context.Background(), draftRequest(otherCategory))
	if err != nil {
		t.Fatalf("接入改了内件性质的草案：%v", err)
	}
	if other.PayloadDigest == command.PayloadDigest {
		t.Fatal("内件性质换了摘要一样——货物明细是内容，不进摘要就检不出同键异容")
	}
}

// Covers: ADR-0373 决定六、ADR-0374 决定四——货物明细与平台号的形状错答 400；包裹上旧的扁平货物字段已经不在契约里，按未知字段拒（`AT-PS-183`）。
func TestCustomerSubmissionIntakeRefusesMalformedGoodsAndPlatformDeclarations(t *testing.T) {
	intake := newCustomerIntake(t, &customerAuthenticatorDouble{}, &permittingOwnershipDouble{revision: "SYN-REV-1"})
	cases := map[string]string{
		"unknown content category": strings.Replace(fbaDraft, `"SALE_OF_GOODS"`, `"SALES"`, 1),
		"unknown commodity side":   strings.Replace(fbaDraft, `"side": "EXPORT"`, `"side": "OUTBOUND"`, 1),
		"quantity is not a number": strings.Replace(fbaDraft, `"quantity": "48"`, `"quantity": "forty-eight"`, 1),
		"packing on one box only":  strings.Replace(fbaDraft, `"platformBoxLabel": "FBA15SYNTESTU000002", "packing": [{"line": 1, "quantity": "24"}]`, `"platformBoxLabel": "FBA15SYNTESTU000002"`, 1),
		"packing without goods":    strings.Replace(customerDraft, `"declaredWeightUnit": "KG"`, `"declaredWeightUnit": "KG", "packing": [{"line": 1, "quantity": "1"}]`, 1),
		"box labels without platform shipment": strings.Replace(fbaDraft,
			`"platformShipment": {"platform": "AMAZON", "shipmentId": "FBA15SYNTEST", "referenceId": "SYN-REF-1"},`, ``, 1),
		"two boxes with one label": strings.Replace(fbaDraft, `FBA15SYNTESTU000002`, `FBA15SYNTESTU000001`, 1),
		"legacy flat goods field":  strings.Replace(customerDraft, `"declaredWeightUnit": "KG"`, `"declaredWeightUnit": "KG", "goodsDescription": "T-shirt"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := intake.IntakeSubmission(context.Background(), draftRequest(body)); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: ADR-0373 决定四、ADR-0376 决定二——补充整份重报货物明细与平台声明：分配与箱标号都挂在回执里的成员上；带箱标号而没有
// 平台货件答 400，形状规则与提交口同一处；同一份载荷交两次摘要相同（重放），改了箱标号摘要就变（来源冲突）。
func TestCustomerSupplementIntakeCarriesGoodsAndPlatformDeclarations(t *testing.T) {
	withGoods := strings.Replace(customerSupplementDraft, `"parcels": [{"parcelId": "DPCL-SYN-1", "customerParcelReference": "SYN-PCLREF-0001", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG"}]`,
		`"goods": {"lines": [{"descriptions": [{"lang": "en", "text": "Wrist watch"}], "quantity": "1"}]},
  "parcels": [{"parcelId": "DPCL-SYN-1", "customerParcelReference": "SYN-PCLREF-0001", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG", "packing": [{"line": 1, "quantity": "1"}]}]`, 1)
	intake, request := intakeSupplementOf(t, supplementAuthenticatorDouble{}, withGoods)
	command, err := intake.IntakeSupplement(t.Context(), request)
	if err != nil {
		t.Fatalf("接入带货物的补充：%v", err)
	}
	packing := command.DeclaredGoods.Packing()
	if len(command.DeclaredGoods.Lines()) != 1 || len(packing) != 1 || packing[0].Parcel().String() != "DPCL-SYN-1" {
		t.Fatalf("补充的货物明细 = %+v；分配要挂在回执里的成员上", packing)
	}
	if command.PlatformShipment.Declared() || len(command.PlatformBoxLabels) != 0 {
		t.Fatal("没带平台段的补充交出了平台声明")
	}

	labelOnly := strings.Replace(withGoods, `"declaredWeightUnit": "KG", "packing"`, `"declaredWeightUnit": "KG", "platformBoxLabel": "FBA15SYNTESTU000001", "packing"`, 1)
	intake, request = intakeSupplementOf(t, supplementAuthenticatorDouble{}, labelOnly)
	if _, err := intake.IntakeSupplement(t.Context(), request); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("补充带箱标号而没有平台货件 err = %v, want ErrMalformedRequest", err)
	}

	withPlatform := func(label string) string {
		body := strings.Replace(labelOnly, "FBA15SYNTESTU000001", label, 1)
		return strings.Replace(body, `"goods"`, `"platformShipment": {"platform": "AMAZON", "shipmentId": "FBA15SYNTEST", "referenceId": "SYN-REF-2"},
  "goods"`, 1)
	}
	digestOf := func(body string) application.FormNewSubmissionVersionCommand {
		t.Helper()
		intake, request := intakeSupplementOf(t, supplementAuthenticatorDouble{}, body)
		command, err := intake.IntakeSupplement(t.Context(), request)
		if err != nil {
			t.Fatalf("接入带平台段的补充：%v", err)
		}
		return command
	}
	first, again, changed := digestOf(withPlatform("FBA15SYNTESTU000001")), digestOf(withPlatform("FBA15SYNTESTU000001")), digestOf(withPlatform("FBA15SYNTESTU000009"))
	if !first.PlatformShipment.Declared() || len(first.PlatformBoxLabels) != 1 || first.PlatformBoxLabels[0].Parcel().String() != "DPCL-SYN-1" {
		t.Fatalf("补充的平台声明 = %+v；箱标号要挂在回执里的成员上", first.PlatformBoxLabels)
	}
	if first.PayloadDigest != again.PayloadDigest || first.PayloadDigest == changed.PayloadDigest {
		t.Fatal("同一份平台段摘要要相同、改了箱标号摘要要变")
	}
	if first.PayloadDigest == command.PayloadDigest {
		t.Fatal("带平台段与不带平台段的补充摘要相同")
	}
}

// Covers: 代码评审——摘要按客户给的内容算：同一份带装箱分配与箱标号的载荷交两次，现签的成员不同，摘要相同，重发认得出是重放；
// 装箱分配的错指到包裹的 packing，原因点名客户包裹参考。
func TestCustomerSubmissionIntakeDigestsGoodsByCustomerReferencesAndLocatesPackingErrors(t *testing.T) {
	intake := newCustomerIntake(t, &customerAuthenticatorDouble{}, &permittingOwnershipDouble{revision: "SYN-REV-1"})
	first, err := intake.IntakeSubmission(context.Background(), draftRequest(fbaDraft))
	if err != nil {
		t.Fatalf("第一次：%v", err)
	}
	second, err := intake.IntakeSubmission(context.Background(), draftRequest(fbaDraft))
	if err != nil {
		t.Fatalf("第二次：%v", err)
	}
	if first.DeclaredParcelIDs[0] == second.DeclaredParcelIDs[0] {
		t.Fatal("两次现签了同一个成员标识；本例要证的是成员不同而摘要相同")
	}
	if first.PayloadDigest != second.PayloadDigest {
		t.Fatalf("同一份载荷两次摘要不同：%s vs %s；分配或箱标号按现签标识进了摘要，重发会被判冲突", first.PayloadDigest, second.PayloadDigest)
	}
	if second.DeclaredGoods.Packing()[0].Parcel() != second.DeclaredParcelIDs[0] {
		t.Fatal("命令里的分配没挂到这一次现签的成员上")
	}

	short := strings.Replace(fbaDraft, `"packing": [{"line": 1, "quantity": "24"}]}`, `"packing": [{"line": 1, "quantity": "23"}]}`, 1)
	_, err = intake.IntakeSubmission(context.Background(), draftRequest(short))
	if !errors.Is(err, shipmenthttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "parcels[].packing") {
		t.Fatalf("err = %v, want ErrMalformedRequest located at parcels[].packing", err)
	}
	missing := strings.Replace(fbaDraft, `"platformBoxLabel": "FBA15SYNTESTU000002", "packing": [{"line": 1, "quantity": "24"}]`, `"platformBoxLabel": "FBA15SYNTESTU000002"`, 1)
	_, err = intake.IntakeSubmission(context.Background(), draftRequest(missing))
	if err == nil || !strings.Contains(err.Error(), "SYN-PCLREF-0002") {
		t.Fatalf("err = %v, want the left-out box named by its customer reference", err)
	}
}
