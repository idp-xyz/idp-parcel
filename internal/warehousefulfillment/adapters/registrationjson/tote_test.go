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
