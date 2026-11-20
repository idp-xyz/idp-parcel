package registrationjson_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

const weightRuleKey = `"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-UNIT-01", "action": "LOADING_DEPARTURE", "boundaryRef": "SYN-PROC-CN-EXPORT"`

func TestWeightDiscrepancyGateRuleTranslatesBothShapes(t *testing.T) {
	waived, err := registrationjson.WeightDiscrepancyGateRuleFromJSON([]byte(`{` + weightRuleKey + `, "notAPrecondition": true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !waived.NotAPrecondition || waived.MaxDelta != "" || waived.Unit != "" || waived.Action != domain.LoadingDeparture {
		t.Fatalf("命令 = %+v", waived)
	}
	accepting, err := registrationjson.WeightDiscrepancyGateRuleFromJSON([]byte(`{` + weightRuleKey + `, "maxDelta": "0.50", "unit": "KG"}`))
	if err != nil {
		t.Fatal(err)
	}
	if accepting.NotAPrecondition || accepting.MaxDelta != "0.50" || accepting.Unit != "KG" {
		t.Fatalf("命令 = %+v", accepting)
	}
}

func TestWeightDiscrepancyGateRuleTranslationRejectsMalformedInput(t *testing.T) {
	for name, body := range map[string]string{
		"未知字段": `{` + weightRuleKey + `, "notAPrecondition": true, "extra": 1}`,
		"缺动作":  `{"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-UNIT-01", "boundaryRef": "SYN-PROC-CN-EXPORT", "notAPrecondition": true}`,
	} {
		if _, err := registrationjson.WeightDiscrepancyGateRuleFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s 被收下了", name)
		}
	}
}
