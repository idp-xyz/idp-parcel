package registrationjson_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 周转箱登记带仓与一批箱号，停用带仓与一个箱号；租户与操作者取自身份，载荷里带了就拒。
func TestToteDocumentsTranslateIntoCommands(t *testing.T) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	operator, _ := domain.NewRegistrant("operator-1")
	registered, err := registrationjson.RegisterTotesCommand(strings.NewReader(`{"warehouse":"SYN-WH-01","toteCodes":["SYN-TOTE-01","SYN-TOTE-02"]}`), tenant, operator)
	if err != nil || registered.Warehouse.String() != "SYN-WH-01" || len(registered.Codes) != 2 || registered.Codes[1].String() != "SYN-TOTE-02" ||
		registered.Tenant != tenant || registered.Operator != operator {
		t.Fatalf("登记译出的命令 %+v：%v", registered, err)
	}
	retired, err := registrationjson.RetireToteCommand(strings.NewReader(`{"warehouse":"SYN-WH-01","toteCode":"SYN-TOTE-01"}`), tenant, operator)
	if err != nil || retired.Warehouse.String() != "SYN-WH-01" || retired.Code.String() != "SYN-TOTE-01" || retired.Operator != operator {
		t.Fatalf("停用译出的命令 %+v：%v", retired, err)
	}
	for name, body := range map[string]string{
		"没有仓":    `{"toteCodes":["SYN-TOTE-01"]}`,
		"没有箱号":   `{"warehouse":"SYN-WH-01"}`,
		"箱号是空的":  `{"warehouse":"SYN-WH-01","toteCodes":[]}`,
		"有一个空白的": `{"warehouse":"SYN-WH-01","toteCodes":["SYN-TOTE-01","  "]}`,
		"带了操作者":  `{"warehouse":"SYN-WH-01","toteCodes":["SYN-TOTE-01"],"registrant":"someone"}`,
	} {
		if _, err := registrationjson.RegisterTotesCommand(strings.NewReader(body), tenant, operator); err == nil {
			t.Errorf("登记，%s：没有被拒", name)
		}
	}
	for name, body := range map[string]string{
		"没有仓":   `{"toteCode":"SYN-TOTE-01"}`,
		"没有箱号":  `{"warehouse":"SYN-WH-01"}`,
		"箱号是空白": `{"warehouse":"SYN-WH-01","toteCode":" "}`,
	} {
		if _, err := registrationjson.RetireToteCommand(strings.NewReader(body), tenant, operator); err == nil {
			t.Errorf("停用，%s：没有被拒", name)
		}
	}
}

// 「拣货必须用周转箱」的登记带仓、修订与 required；required 不带不当成假——假是明说不要求，与没登记是两回事。
func TestAToteRequirementDocumentMustSayWhetherTotesAreRequired(t *testing.T) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	operator, _ := domain.NewRegistrant("operator-1")
	for _, required := range []bool{true, false} {
		body := `{"warehouse":"SYN-WH-01","revision":1,"required":false}`
		if required {
			body = `{"warehouse":"SYN-WH-01","revision":1,"required":true}`
		}
		command, err := registrationjson.RegisterToteRequirementCommand(strings.NewReader(body), tenant, operator)
		if err != nil || command.Warehouse.String() != "SYN-WH-01" || command.Revision != 1 || command.Required != required ||
			command.Tenant != tenant || command.Operator != operator {
			t.Fatalf("required=%v 译出的命令 %+v：%v", required, command, err)
		}
	}
	for name, body := range map[string]string{
		"没有仓":         `{"revision":1,"required":true}`,
		"没有修订":        `{"warehouse":"SYN-WH-01","required":true}`,
		"修订不是正整数":     `{"warehouse":"SYN-WH-01","revision":0,"required":true}`,
		"没说 required": `{"warehouse":"SYN-WH-01","revision":1}`,
		"带了操作者":       `{"warehouse":"SYN-WH-01","revision":1,"required":true,"registrant":"someone"}`,
	} {
		if _, err := registrationjson.RegisterToteRequirementCommand(strings.NewReader(body), tenant, operator); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}

// 成波规则带仓、规则码、修订、名字与一个波次最多几张；货主、发运方式、件型不带是不限，带了就得是认得的值。
// 按规则成波带仓、规则码与执行号，操作者取自身份。
func TestAWaveRuleDocumentNamesItsConditionsAndARunNamesTheRule(t *testing.T) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	operator, _ := domain.NewRegistrant("operator-1")
	plain, err := registrationjson.RegisterWaveRuleCommand(strings.NewReader(
		`{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":" 早班小件 ","maxRequests":20}`), tenant, operator)
	if err != nil || plain.Code.String() != "SYN-RULE-A" || plain.Name != "早班小件" || plain.MaxRequests != 20 || plain.Profile != "ANY" ||
		plain.Owner != "" || plain.Dispatch != "" || plain.BatchPick || plain.Deactivated || plain.Operator != operator {
		t.Fatalf("只带必填的规则 %+v：%v", plain, err)
	}
	full, err := registrationjson.RegisterWaveRuleCommand(strings.NewReader(
		`{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":2,"name":"小件","maxRequests":20,"ownerAccount":"SYN-OWNER-01",`+
			`"dispatch":"PICKUP","profile":"SINGLE_UNIT","batchPick":true,"deactivated":true}`), tenant, operator)
	if err != nil || full.Owner != "SYN-OWNER-01" || full.Dispatch != "PICKUP" || full.Profile != "SINGLE_UNIT" || !full.BatchPick || !full.Deactivated {
		t.Fatalf("带齐条件的规则 %+v：%v", full, err)
	}
	for name, body := range map[string]string{
		"没有规则码":    `{"warehouse":"SYN-WH-01","revision":1,"name":"小件","maxRequests":20}`,
		"没有名字":     `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"maxRequests":20}`,
		"没说一个波次几张": `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":"小件"}`,
		"货主是空串":    `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":"小件","maxRequests":20,"ownerAccount":""}`,
		"不认识的发运方式": `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":"小件","maxRequests":20,"dispatch":"DRONE"}`,
		"不认识的件型":   `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":"小件","maxRequests":20,"profile":"LARGE"}`,
		"带了操作者":    `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":1,"name":"小件","maxRequests":20,"registrant":"someone"}`,
	} {
		if _, err := registrationjson.RegisterWaveRuleCommand(strings.NewReader(body), tenant, operator); err == nil {
			t.Errorf("规则，%s：没有被拒", name)
		}
	}
	timed, err := registrationjson.RegisterWaveRuleCommand(strings.NewReader(
		`{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","revision":3,"name":"小件","maxRequests":20,"releaseTimes":["09:00","14:00"]}`), tenant, operator)
	if err != nil || len(timed.ReleaseTimes) != 2 || timed.ReleaseTimes[1] != "14:00" || len(plain.ReleaseTimes) != 0 {
		t.Fatalf("带时点的规则 %+v：%v", timed, err)
	}
	due, err := registrationjson.RunDueWaveRulesCommand(strings.NewReader(`{}`))
	if err != nil || due.Warehouse != "" || due.Operator.String() != "" {
		t.Fatalf("放到点的规则译出的命令 %+v：%v", due, err)
	}
	if one, err := registrationjson.RunDueWaveRulesCommand(strings.NewReader(`{"warehouse":"SYN-WH-01"}`)); err != nil || one.Warehouse != "SYN-WH-01" {
		t.Fatalf("只看一个仓 %+v：%v", one, err)
	}
	for name, body := range map[string]string{
		"仓是空白":   `{"warehouse":" "}`,
		"自己带了时刻": `{"at":"2026-11-27T09:00:00Z"}`,
		"带了操作者":  `{"registrant":"someone"}`,
	} {
		if _, err := registrationjson.RunDueWaveRulesCommand(strings.NewReader(body)); err == nil {
			t.Errorf("放到点的规则，%s：没有被拒", name)
		}
	}
	run, err := registrationjson.RunWaveRuleCommand(strings.NewReader(`{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","runId":"SYN-RUN-1"}`))
	if err != nil || run.Warehouse.String() != "SYN-WH-01" || run.Rule.String() != "SYN-RULE-A" || run.RunID.String() != "SYN-RUN-1" || run.Operator.String() != "" {
		t.Fatalf("成波译出的命令 %+v：%v", run, err)
	}
	for name, body := range map[string]string{
		"没有执行号": `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A"}`,
		"没有规则码": `{"warehouse":"SYN-WH-01","runId":"SYN-RUN-1"}`,
		"带了操作者": `{"warehouse":"SYN-WH-01","ruleCode":"SYN-RULE-A","runId":"SYN-RUN-1","registrant":"someone"}`,
	} {
		if _, err := registrationjson.RunWaveRuleCommand(strings.NewReader(body)); err == nil {
			t.Errorf("成波，%s：没有被拒", name)
		}
	}
}

// 指派拣货人带波次号与拣货人，不带拣货人是取消指派、空白的不收；领波次带仓与波次号，领的人取自认证。
func TestWaveAssignmentAndClaimDocuments(t *testing.T) {
	assigned, err := registrationjson.AssignWaveCommand(strings.NewReader(`{"waveId":"WAVE-01","assignee":" https://issuer.example.invalid#picker-1 "}`))
	if err != nil || assigned.Wave.String() != "WAVE-01" || assigned.Assignee != "https://issuer.example.invalid#picker-1" || assigned.Operator.String() != "" {
		t.Fatalf("指派译出的命令 %+v：%v", assigned, err)
	}
	cleared, err := registrationjson.AssignWaveCommand(strings.NewReader(`{"waveId":"WAVE-01"}`))
	if err != nil || cleared.Assignee != "" {
		t.Fatalf("取消指派译出的命令 %+v：%v", cleared, err)
	}
	for name, body := range map[string]string{
		"没有波次号":  `{"assignee":"picker-1"}`,
		"拣货人是空白": `{"waveId":"WAVE-01","assignee":"  "}`,
		"带了操作者":  `{"waveId":"WAVE-01","assignee":"picker-1","registrant":"someone"}`,
		"带了别的格":  `{"waveId":"WAVE-01","assignedBy":"someone"}`,
	} {
		if _, err := registrationjson.AssignWaveCommand(strings.NewReader(body)); err == nil {
			t.Errorf("指派，%s：没有被拒", name)
		}
	}
	claim, err := registrationjson.ClaimWaveCommand(strings.NewReader(`{"warehouse":"SYN-WH-01","waveId":"WAVE-01"}`))
	if err != nil || claim.Warehouse.String() != "SYN-WH-01" || claim.Wave.String() != "WAVE-01" || claim.Operator != "" {
		t.Fatalf("领波次译出的命令 %+v：%v", claim, err)
	}
	for name, body := range map[string]string{
		"没有仓":     `{"waveId":"WAVE-01"}`,
		"没有波次号":   `{"warehouse":"SYN-WH-01"}`,
		"自己写了领的人": `{"warehouse":"SYN-WH-01","waveId":"WAVE-01","operator":"picker-1"}`,
	} {
		if _, err := registrationjson.ClaimWaveCommand(strings.NewReader(body)); err == nil {
			t.Errorf("领波次，%s：没有被拒", name)
		}
	}
}

// 分拣的一次投放带波次、请求、商品、件数与箱号，一样都不能少；件数是正整数。
func TestASortPutNamesTheWaveTheRequestTheGoodsAndTheTote(t *testing.T) {
	full := `{"factId":"SYN-FACT-01","warehouse":"SYN-WH-01","waveId":"WAVE-01","requestId":"OBR-1","goodsCode":"SYN-GOODS-01","quantity":2,"toteCode":"SYN-TOTE-01"}`
	command, err := registrationjson.SortCommand(strings.NewReader(full))
	if err != nil || command.FactID.String() != "SYN-FACT-01" || command.Warehouse.String() != "SYN-WH-01" || command.Wave.String() != "WAVE-01" ||
		command.Request.String() != "OBR-1" || command.Goods.String() != "SYN-GOODS-01" || command.Quantity != 2 || command.Tote.String() != "SYN-TOTE-01" {
		t.Fatalf("投放译出的命令 %+v：%v", command, err)
	}
	for name, body := range map[string]string{
		"没有箱号":  strings.Replace(full, `,"toteCode":"SYN-TOTE-01"`, ``, 1),
		"没有波次":  strings.Replace(full, `"waveId":"WAVE-01",`, ``, 1),
		"没有请求":  strings.Replace(full, `"requestId":"OBR-1",`, ``, 1),
		"没有商品":  strings.Replace(full, `"goodsCode":"SYN-GOODS-01",`, ``, 1),
		"件数是零":  strings.Replace(full, `"quantity":2`, `"quantity":0`, 1),
		"带了操作者": strings.Replace(full, `}`, `,"registrant":"someone"}`, 1),
	} {
		if _, err := registrationjson.SortCommand(strings.NewReader(body)); err == nil {
			t.Errorf("投放，%s：没有被拒", name)
		}
	}
}

// 拣到 0 件是「到了库位一件都没有」：收，这时不要放货位；拣到了货就得有放货位；负数不收。
func TestAPickOfNothingNeedsNoStagingLocation(t *testing.T) {
	head := `{"factId":"SYN-FACT-01","taskId":"SYN-TASK-01","warehouse":"SYN-WH-01"`
	empty, err := registrationjson.PickCommand(strings.NewReader(head + `,"quantity":0}`))
	if err != nil || empty.Quantity != 0 || empty.Target.String() != "" {
		t.Fatalf("拣到 0 件：%v %+v", err, empty)
	}
	for name, body := range map[string]string{
		"拣到了货却没有放货位": head + `,"quantity":1}`,
		"负数":         head + `,"quantity":-1,"targetLocation":"SYN-LOC-OUT"}`,
		"没写数量":       head + `,"targetLocation":"SYN-LOC-OUT"}`,
		"放货位是空白":     head + `,"quantity":0,"targetLocation":" "}`,
	} {
		if _, err := registrationjson.PickCommand(strings.NewReader(body)); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}

// 拣货事实可以带箱号，不带就是没用箱；带了空白的不收。由人放开周转箱带仓与一个箱号，操作者取自身份。
func TestAPickMayNameAToteAndAReleaseNamesOne(t *testing.T) {
	pick := `{"factId":"SYN-FACT-01","taskId":"SYN-TASK-01","warehouse":"SYN-WH-01","quantity":2,"targetLocation":"SYN-LOC-OUT"`
	plain, err := registrationjson.PickCommand(strings.NewReader(pick + `}`))
	if err != nil || plain.Tote.String() != "" {
		t.Fatalf("不带箱号：%v %q", err, plain.Tote)
	}
	named, err := registrationjson.PickCommand(strings.NewReader(pick + `,"toteCode":"SYN-TOTE-01"}`))
	if err != nil || named.Tote.String() != "SYN-TOTE-01" {
		t.Fatalf("带箱号：%v %q", err, named.Tote)
	}
	if _, err := registrationjson.PickCommand(strings.NewReader(pick + `,"toteCode":" "}`)); err == nil {
		t.Error("空白的箱号没有被拒")
	}
	released, err := registrationjson.ReleaseToteCommand(strings.NewReader(`{"warehouse":"SYN-WH-01","toteCode":"SYN-TOTE-01"}`))
	if err != nil || released.Warehouse.String() != "SYN-WH-01" || released.Code.String() != "SYN-TOTE-01" || released.Operator.String() != "" {
		t.Fatalf("放开译出的命令 %+v：%v", released, err)
	}
	for name, body := range map[string]string{
		"没有箱号":  `{"warehouse":"SYN-WH-01"}`,
		"带了操作者": `{"warehouse":"SYN-WH-01","toteCode":"SYN-TOTE-01","registrant":"someone"}`,
	} {
		if _, err := registrationjson.ReleaseToteCommand(strings.NewReader(body)); err == nil {
			t.Errorf("放开，%s：没有被拒", name)
		}
	}
}
