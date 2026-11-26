package registrationjson_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
)

// 放出后撤单的载荷只有波次号与点名的请求；操作者与租户取自身份，载荷里带了就拒。
func TestAWaveWithdrawalDocumentTranslatesIntoTheCommand(t *testing.T) {
	command, err := registrationjson.WithdrawWaveCommand(strings.NewReader(`{"waveId":"WAVE-01","requests":["OBR-01","OBR-02"]}`))
	if err != nil {
		t.Fatalf("译不成命令：%v", err)
	}
	if command.ID.String() != "WAVE-01" || len(command.Requests) != 2 || command.Requests[1].String() != "OBR-02" {
		t.Fatalf("命令 %+v", command)
	}
	if command.Operator.String() != "" || command.Tenant.String() != "" {
		t.Fatalf("操作者与租户不该从载荷来：%+v", command)
	}
}

func TestMalformedWaveWithdrawalDocumentsAreRefused(t *testing.T) {
	for name, body := range map[string]string{
		"没有波次号":  `{"requests":["OBR-01"]}`,
		"没有点名请求": `{"waveId":"WAVE-01"}`,
		"点名是空列表": `{"waveId":"WAVE-01","requests":[]}`,
		"请求号是空白": `{"waveId":"WAVE-01","requests":[" "]}`,
		"带了操作者":  `{"waveId":"WAVE-01","requests":["OBR-01"],"registrant":"someone"}`,
		"带了租户":   `{"waveId":"WAVE-01","requests":["OBR-01"],"tenant":"SYN-TENANT-01"}`,
		"不认识的键":  `{"waveId":"WAVE-01","requests":["OBR-01"],"reason":"x"}`,
		"不是一份文档": `[]`,
	} {
		if _, err := registrationjson.WithdrawWaveCommand(strings.NewReader(body)); err == nil {
			t.Errorf("%s：没有被拒", name)
		}
	}
}
