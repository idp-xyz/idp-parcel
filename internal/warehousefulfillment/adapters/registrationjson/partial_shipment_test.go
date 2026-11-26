package registrationjson_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const ownerOutboundBody = `{"warehouse":"SYN-WH-01","dispatch":"PICKUP","recipient":"合成收件人","address":"合成地址",` +
	`"lines":[{"goodsCode":"SYN-GOODS-01","unit":"件","stockState":"GOOD","quantity":2}]`

// 货主提交出库请求时可以带部分发货规则；不带是没写明，带了就得是三个词之一。
func TestAnOwnerOutboundRequestMayCarryItsPartialShipment(t *testing.T) {
	plain, err := registrationjson.ReadOwnerOutbound(strings.NewReader(ownerOutboundBody + `}`))
	if err != nil || plain.PartialShipment.Given() {
		t.Fatalf("不带规则：%v %s", err, plain.PartialShipment)
	}
	for _, word := range []string{"SHIP_COMPLETE", "SHORT_SHIP", "SPLIT_SHIP"} {
		fields, err := registrationjson.ReadOwnerOutbound(strings.NewReader(ownerOutboundBody + `,"partialShipment":"` + word + `"}`))
		if err != nil || fields.PartialShipment.String() != word {
			t.Errorf("%s：%v %s", word, err, fields.PartialShipment)
		}
	}
	for _, word := range []string{"", "PARTIAL", "ship_complete"} {
		if _, err := registrationjson.ReadOwnerOutbound(strings.NewReader(ownerOutboundBody + `,"partialShipment":"` + word + `"}`)); err == nil {
			t.Errorf("%q 不该被收下", word)
		}
	}
}

// 缺货处置的两格至少带一格，各说各的。
func TestAShortagePolicyDocumentCarriesKindOrPartialShipmentOrBoth(t *testing.T) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	operator, _ := domain.NewRegistrant("operator-1")
	read := func(body string) (string, string, error) {
		command, err := registrationjson.ShortagePolicyCommand(strings.NewReader(body), tenant, operator)
		return command.Kind.String(), command.PartialShipment.String(), err
	}
	for name, testCase := range map[string]struct {
		body, kind, partial string
	}{
		"只说受理时不够怎么办": {`{"ownerAccount":"SYN-OWNER-01","revision":1,"kind":"REJECT"}`, "REJECT", ""},
		"只说拣不齐时怎么发":  {`{"ownerAccount":"SYN-OWNER-01","revision":1,"partialShipment":"SPLIT_SHIP"}`, "", "SPLIT_SHIP"},
		"两件都说":       {`{"ownerAccount":"SYN-OWNER-01","revision":2,"kind":"PARTIAL","partialShipment":"SHORT_SHIP"}`, "PARTIAL", "SHORT_SHIP"},
	} {
		kind, partial, err := read(testCase.body)
		if err != nil || kind != testCase.kind || partial != testCase.partial {
			t.Errorf("%s：%v kind=%q partial=%q", name, err, kind, partial)
		}
	}
	for name, body := range map[string]string{
		"两件都没说":  `{"ownerAccount":"SYN-OWNER-01","revision":1}`,
		"规则词不认识": `{"ownerAccount":"SYN-OWNER-01","revision":1,"partialShipment":"ALLOW"}`,
		"处置词不认识": `{"ownerAccount":"SYN-OWNER-01","revision":1,"kind":"MAYBE","partialShipment":"SHORT_SHIP"}`,
	} {
		if _, _, err := read(body); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}

// 代录部分发货同意：请求、规则、依据都要有；操作者与租户取自身份，货主不许带。
func TestAPartialShipmentConsentDocumentTranslatesIntoTheCommand(t *testing.T) {
	command, err := registrationjson.OperatorPartialShipmentConsent(strings.NewReader(`{"requestId":"OBR-01","partialShipment":"SPLIT_SHIP","basis":" SYN-CONSENT-01 "}`))
	if err != nil || command.Request.String() != "OBR-01" || command.Rule != domain.PartialShipmentSplitShip || command.Basis != "SYN-CONSENT-01" {
		t.Fatalf("译出的命令 %+v：%v", command, err)
	}
	if command.Operator.String() != "" || command.Tenant.String() != "" {
		t.Fatalf("操作者与租户不该从载荷来：%+v", command)
	}
	for name, body := range map[string]string{
		"没有请求号": `{"partialShipment":"SPLIT_SHIP","basis":"B"}`,
		"没有规则":  `{"requestId":"OBR-01","basis":"B"}`,
		"规则不认识": `{"requestId":"OBR-01","partialShipment":"YES","basis":"B"}`,
		"没有依据":  `{"requestId":"OBR-01","partialShipment":"SPLIT_SHIP"}`,
		"依据是空白": `{"requestId":"OBR-01","partialShipment":"SPLIT_SHIP","basis":"  "}`,
		"带了货主":  `{"requestId":"OBR-01","partialShipment":"SPLIT_SHIP","basis":"B","ownerAccount":"SYN-OWNER-01"}`,
		"带了操作者": `{"requestId":"OBR-01","partialShipment":"SPLIT_SHIP","basis":"B","registrant":"someone"}`,
	} {
		if _, err := registrationjson.OperatorPartialShipmentConsent(strings.NewReader(body)); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}
