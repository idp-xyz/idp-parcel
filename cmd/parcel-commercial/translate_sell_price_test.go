package main

import "testing"

// Covers: 票 commercial-four-module-requirements/16 的受控批文输入面（ADR-0380 决定一）——发布批表达得出客户合同版本的
// 销售价格约定：来源照批文里的顺序翻过去、不排序；不计销售价只带依据；缺键就是没这一节。两个键恰一在场在触库前
// 拒，不挑一格读，也不把都没给折成「不计销售价」；未知键拒收。同一计价范围两次是领域构造门的事，翻译层不代判。
func TestASellPriceArrangementTranslatesInTheDeclaredOrder(t *testing.T) {
	item := func(declarations string) string {
		return `{"items": [{"tenantId": "t", "kind": "CUSTOMER_CONTRACT", "objectId": "o", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {` + declarations + `}}]}`
	}

	t.Run("来源照批文的顺序", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item(
			`"sellPriceArrangement": {"sources": [{"pricingScope": "SYN-SCOPE-Z"}, {"pricingScope": "SYN-SCOPE-A"}]}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.SellPriceArrangement
		if declared == nil || len(declared.Sources) != 2 ||
			declared.Sources[0].String() != "SYN-SCOPE-Z" || declared.Sources[1].String() != "SYN-SCOPE-A" ||
			declared.NoSellPriceBasis.String() != "" {
			t.Fatalf("声明 = %+v，翻译变形或重排了来源", declared)
		}
	})

	t.Run("不计销售价只带依据", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item(
			`"sellPriceArrangement": {"noSellPriceBasis": "SYN-NO-SELL-PRICE-01"}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.SellPriceArrangement
		if declared == nil || len(declared.Sources) != 0 || declared.NoSellPriceBasis.String() != "SYN-NO-SELL-PRICE-01" {
			t.Fatalf("声明 = %+v", declared)
		}
	})

	t.Run("缺键就是没这一节", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item(
			`"contractContent": {"customerAccount": "customer-1", "rulePackage": "rules-1"}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		if commands[0].Declarations.SellPriceArrangement != nil {
			t.Fatal("没给这一节却翻出了一份约定")
		}
	})

	refusals := map[string]string{
		"两个键都给":      `{"sources": [{"pricingScope": "SYN-SCOPE-A"}], "noSellPriceBasis": "SYN-NO-SELL-PRICE-01"}`,
		"两个键都没给":     `{}`,
		"来源列表为空":     `{"sources": []}`,
		"来源的计价范围为空串": `{"sources": [{"pricingScope": "SYN-SCOPE-A"}, {"pricingScope": "  "}]}`,
		"来源缺计价范围":    `{"sources": [{}]}`,
		"依据只有空白":     `{"noSellPriceBasis": "  "}`,
		"未知键":        `{"sources": [{"pricingScope": "SYN-SCOPE-A"}], "default": "SYN-SCOPE-STANDARD"}`,
		"来源里的未知键":    `{"sources": [{"pricingScope": "SYN-SCOPE-A", "pricingPlan": "plan-1"}]}`,
	}
	for name, arrangement := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(item(`"sellPriceArrangement": ` + arrangement))); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}
