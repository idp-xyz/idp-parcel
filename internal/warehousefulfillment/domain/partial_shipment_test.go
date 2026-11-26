package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestPartialShipmentAcceptsOnlyItsThreeWords(t *testing.T) {
	for _, word := range []string{"SHIP_COMPLETE", "SHORT_SHIP", "SPLIT_SHIP"} {
		rule, err := domain.NewPartialShipment(word)
		if err != nil || rule.String() != word || !rule.Given() {
			t.Errorf("%s：%v %s", word, err, rule)
		}
	}
	for _, word := range []string{"", "PARTIAL", "short_ship", "ALLOW"} {
		if rule, err := domain.NewPartialShipment(word); err == nil || rule.Given() {
			t.Errorf("%q 不该被收下", word)
		}
	}
	if domain.PartialShipmentUnset.String() != "" || domain.PartialShipmentUnset.Given() {
		t.Error("没写明不该有词")
	}
}

// 越贴近这一张请求、越晚说的越优先；哪一层都没写明就是未配置。
func TestTheEffectivePartialShipmentPrefersTheLayerClosestToTheOrder(t *testing.T) {
	unset := domain.PartialShipmentUnset
	complete, short, split := domain.PartialShipmentShipComplete, domain.PartialShipmentShortShip, domain.PartialShipmentSplitShip
	for name, testCase := range map[string]struct {
		consent, order, owner domain.PartialShipment
		want                  domain.PartialShipment
		source                string
	}{
		"都没写明":       {unset, unset, unset, unset, ""},
		"只有货主默认":     {unset, unset, short, short, "OWNER"},
		"订单写明盖过货主默认": {unset, complete, short, complete, "ORDER"},
		"订单写了整单发，货主默认少发也不少发": {unset, complete, split, complete, "ORDER"},
		"代录的同意盖过订单提交时带的":     {split, complete, unset, split, "ORDER_CONSENT"},
		"代录的同意盖过货主默认":        {short, unset, complete, short, "ORDER_CONSENT"},
		"只有订单提交时带的":          {unset, split, unset, split, "ORDER"},
	} {
		rule, source := domain.EffectivePartialShipment(testCase.consent, testCase.order, testCase.owner)
		if rule != testCase.want || source.String() != testCase.source {
			t.Errorf("%s：得 %s 出自 %q，应为 %s 出自 %q", name, rule, source, testCase.want, testCase.source)
		}
	}
}
