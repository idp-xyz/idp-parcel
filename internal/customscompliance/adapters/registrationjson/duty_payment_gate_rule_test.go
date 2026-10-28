package registrationjson_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

const dutyRuleKey = `"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-SCOPE-01", "action": "CROSS_CUSTOMS_MOVEMENT", "boundaryRef": "SYN-PROC-CN-EXPORT"`

// 两形各自译成命令：「不构成前置条件」不带集合；接受集合形三维齐全，字面与付款核对同一份封闭词表。
func TestDutyPaymentGateRuleTranslatesBothShapes(t *testing.T) {
	waived, err := registrationjson.DutyPaymentGateRuleFromJSON([]byte(`{` + dutyRuleKey + `, "notAPrecondition": true}`))
	if err != nil {
		t.Fatalf("不构成前置条件：%v", err)
	}
	if !waived.NotAPrecondition || len(waived.AcceptCoverage)+len(waived.AcceptDelta)+len(waived.AcceptValidity) != 0 ||
		waived.Action != domain.CrossCustomsMovement || waived.Boundary.String() != "SYN-PROC-CN-EXPORT" {
		t.Fatalf("命令 = %+v", waived)
	}

	accepting, err := registrationjson.DutyPaymentGateRuleFromJSON([]byte(`{` + dutyRuleKey + `,
		"acceptCoverage": ["COVERED"], "acceptDelta": ["NO_DELTA", "EXCESS"], "acceptValidity": ["VALID"]}`))
	if err != nil {
		t.Fatalf("接受集合：%v", err)
	}
	if accepting.NotAPrecondition || len(accepting.AcceptCoverage) != 1 || accepting.AcceptCoverage[0] != domain.CoverageFull ||
		len(accepting.AcceptDelta) != 2 || len(accepting.AcceptValidity) != 1 || accepting.AcceptValidity[0] != domain.FundsFactValid {
		t.Fatalf("命令 = %+v", accepting)
	}
}

// 打错的键、封闭词表外的字面、缺的键维都当场拒——不静默丢字段，让操作员以为登进去的比实际多。
func TestDutyPaymentGateRuleTranslationRejectsMalformedInput(t *testing.T) {
	for name, body := range map[string]string{
		"未知字段":    `{` + dutyRuleKey + `, "notAPrecondition": true, "extra": 1}`,
		"覆盖字面越界":  `{` + dutyRuleKey + `, "acceptCoverage": ["FULL"], "acceptDelta": ["NO_DELTA"], "acceptValidity": ["VALID"]}`,
		"缺动作":     `{"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-SCOPE-01", "boundaryRef": "SYN-PROC-CN-EXPORT", "notAPrecondition": true}`,
		"不是 JSON": `not json`,
	} {
		if _, err := registrationjson.DutyPaymentGateRuleFromJSON([]byte(body)); err == nil {
			t.Errorf("%s：应拒", name)
		}
	}
	// 待确认 / 冲突能译出来，由领域构造去拒；这里不替它吞掉。
	command, err := registrationjson.DutyPaymentGateRuleFromJSON([]byte(strings.Replace(
		`{`+dutyRuleKey+`, "acceptCoverage": ["COVERED"], "acceptDelta": ["PENDING"], "acceptValidity": ["VALID"]}`, "\n", "", -1)))
	if err != nil || len(command.AcceptDelta) != 1 {
		t.Fatalf("PENDING 应原样译出交领域拒：%v %+v", err, command)
	}
}
