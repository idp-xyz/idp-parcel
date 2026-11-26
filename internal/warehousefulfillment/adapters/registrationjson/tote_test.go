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
