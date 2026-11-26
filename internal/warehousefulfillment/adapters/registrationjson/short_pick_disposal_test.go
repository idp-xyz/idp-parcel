package registrationjson_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
)

// 按已拣数量发货只点名请求；租户与操作者取自身份，货主不许带，也不许顺手带一个规则进来。
func TestAShortShipmentDocumentOnlyNamesTheRequest(t *testing.T) {
	command, err := registrationjson.ShipPickedCommand(strings.NewReader(`{"requestId":"OBR-01"}`))
	if err != nil || command.Request.String() != "OBR-01" || command.Operator.String() != "" || command.Tenant.String() != "" {
		t.Fatalf("译出的命令 %+v：%v", command, err)
	}
	for name, body := range map[string]string{
		"没有请求号":  `{}`,
		"请求号是空白": `{"requestId":"  "}`,
		"带了货主":   `{"requestId":"OBR-01","ownerAccount":"SYN-OWNER-01"}`,
		"带了操作者":  `{"requestId":"OBR-01","registrant":"someone"}`,
		"带了规则":   `{"requestId":"OBR-01","partialShipment":"SHORT_SHIP"}`,
	} {
		if _, err := registrationjson.ShipPickedCommand(strings.NewReader(body)); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}
