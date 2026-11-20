package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件的报文都是隔离合成值，只记 `S`。租户与登记责任方由调用方传入，报文里没有这两格。

const goodsOnBehalfDocument = `{
	"ownerAccount": "SYN-OWNER-01",
	"decisionId": "SYN-DECISION-GOODS-01",
	"evidenceReference": "SYN-EVIDENCE-01",
	"goodsCode": "SYN-GOODS-01",
	"name": "合成商品",
	"baseUnit": "件"
}`

func translateGoods(t *testing.T, body string) (application.RegisterGoodsCommand, error) {
	t.Helper()
	return registrationjson.GoodsOnBehalfCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

// Covers: ADR-0384 决定四——代录报文齐全时译得出：货主、决定标识与依据引用是载荷里的业务内容，租户与登记责任方是调用方传入的；
// 提交途径是代录。没给基准修订就是没给，不是基准 0。
func TestACompleteGoodsOnBehalfDocumentTranslatesIntoTheCommand(t *testing.T) {
	command, err := translateGoods(t, goodsOnBehalfDocument)
	if err != nil {
		t.Fatal(err)
	}
	evidence, present := command.Submission.EvidenceReference()
	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" ||
		command.Owner.String() != "SYN-OWNER-01" || command.Code.String() != "SYN-GOODS-01" ||
		command.Content.Name().String() != "合成商品" || command.Content.BaseUnit().String() != "件" ||
		command.Submission.Path() != domain.SubmissionOnBehalf || command.Submission.Key().String() != "SYN-DECISION-GOODS-01" ||
		!present || evidence.String() != "SYN-EVIDENCE-01" || command.Basis.Given() {
		t.Fatalf("命令 = %+v，提交 = %+v", command, command.Submission)
	}
}

// 基准修订 0 是给了基准 0。折合数不为正、区间颠倒留给登记用例：译装提前拒掉，同一份内容原样重投会先于重放被拒。
func TestAGoodsBasisOfZeroAndANonPositiveFactorStillTranslate(t *testing.T) {
	body := strings.TrimRight(strings.TrimSpace(goodsOnBehalfDocument), "}") + `,
		"basisRevision": 0,
		"packagingLevels": [{"name": "箱", "factor": 0}],
		"aliases": [{"code": "SYN-BAR-01", "type": "BARCODE", "validity": {"from": "2026-11-20T00:00:00Z", "to": "2026-11-01T00:00:00Z"}}]
	}`
	command, err := translateGoods(t, body)
	if err != nil {
		t.Fatalf("译装拒了归应用层判的格：%v", err)
	}
	if !command.Basis.Given() || command.Basis.Number() != 0 {
		t.Fatalf("基准 = %+v，want 给了 0", command.Basis)
	}
}

// Covers: 代录缺依据引用、不认识的别名类型或出库方式，是坏载荷。理由只点名格，不回显取值。报文自带租户或登记责任方也拒。
func TestGoodsOnBehalfRefusesAMissingEvidenceAnUnknownWordAndACarriedIdentity(t *testing.T) {
	const unknown = "SECRET-ALIAS-KIND"
	cases := map[string]string{
		"evidence absent": strings.Replace(goodsOnBehalfDocument, `"evidenceReference": "SYN-EVIDENCE-01",`, "", 1),
		"unknown alias": strings.TrimRight(strings.TrimSpace(goodsOnBehalfDocument), "}") + `,
			"aliases": [{"code": "SYN-BAR-01", "type": "` + unknown + `"}]}`,
		"unknown outbound mode": strings.TrimRight(strings.TrimSpace(goodsOnBehalfDocument), "}") + `,
			"composition": {"outboundMode": "` + unknown + `", "components": []}}`,
		"tenant carried":     strings.TrimRight(strings.TrimSpace(goodsOnBehalfDocument), "}") + `, "tenantId": "SYN-TENANT-01"}`,
		"registrant carried": strings.TrimRight(strings.TrimSpace(goodsOnBehalfDocument), "}") + `, "registrant": "someone"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translateGoods(t, body)
			if !errors.Is(err, registrationjson.ErrMalformedDocument) {
				t.Fatalf("err = %v，want ErrMalformedDocument", err)
			}
			if strings.Contains(err.Error(), unknown) || strings.Contains(err.Error(), "SYN-EVIDENCE-01") || strings.Contains(err.Error(), "SYN-TENANT-01") {
				t.Fatalf("坏载荷的理由回显了取值：%v", err)
			}
		})
	}
}
