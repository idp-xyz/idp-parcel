package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件的报文都是隔离合成值，只记 `S`。租户与登记责任方由调用方传入，报文里没有这两格。

const fullZoneDocument = `{
	"warehouseCode": "SYN-WH-01",
	"zoneCode": "SYN-ZONE-01",
	"revision": 1,
	"name": "合成存储区",
	"usage": "STORAGE",
	"deactivated": true
}`

func translateZone(t *testing.T, body string) (application.RegisterZoneCommand, error) {
	t.Helper()
	return registrationjson.ZoneCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

func mustTranslateZone(t *testing.T, body string) application.RegisterZoneCommand {
	t.Helper()
	command, err := translateZone(t, body)
	if err != nil {
		t.Fatalf("译 %s：%v", body, err)
	}
	return command
}

// Covers: AT-WF-001（库区的部分）——报文齐全时译得出：每一格进命令，租户与登记责任方是调用方传入的那两个。
func TestACompleteZoneDocumentTranslatesIntoTheCommand(t *testing.T) {
	command := mustTranslateZone(t, fullZoneDocument)

	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("租户与登记责任方 = %q、%q，want 调用方传入的两个", command.Tenant, command.Registrant)
	}
	if command.Warehouse.String() != "SYN-WH-01" || command.Code.String() != "SYN-ZONE-01" || command.Revision != 1 ||
		command.Content.Name().String() != "合成存储区" || command.Content.Usage() != domain.ZoneUsageStorage ||
		!command.Content.Deactivated() {
		t.Fatalf("命令 = %+v，内容 = %+v", command, command.Content)
	}
}

// Covers: AT-WF-004（库区的部分）——封闭的六种用途逐种译得出；是否停用可缺：缺失译成不停用，显式 null 与缺失同义。
func TestEveryClosedZoneUsageTranslatesAndDeactivatedDefaultsToNotDeactivated(t *testing.T) {
	for _, usage := range domain.ZoneUsages() {
		body := `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"` + usage.String() + `"}`
		if got := mustTranslateZone(t, body).Content.Usage(); got != usage {
			t.Errorf("用途 %s 译成 %s", usage, got)
		}
	}
	const minimal = `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"PICKING"`
	for name, body := range map[string]string{
		"key absent":  minimal + `}`,
		"key is null": minimal + `,"deactivated":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			if mustTranslateZone(t, body).Content.Deactivated() {
				t.Fatalf("缺失的 deactivated 该译成不停用")
			}
		})
	}
}

// Covers: AT-WF-001「库区编码逐字保存」——仓编码、库区编码与名称逐字交给领域：带首尾空白、内部连续空白、换行与大小写混写，
// 一个字节都不动。
func TestTheZoneCodesAndNameAreHandedOverVerbatim(t *testing.T) {
	const warehouse, zone, name = " Syn-WH  01\t", "  zone\n 01 ", "  合成  存储区 "
	body := `{"warehouseCode":" Syn-WH  01\t","zoneCode":"  zone\n 01 ","revision":2,"name":"  合成  存储区 ","usage":"DEFECTIVE"}`

	command := mustTranslateZone(t, body)
	if command.Warehouse.String() != warehouse || command.Code.String() != zone || command.Content.Name().String() != name {
		t.Fatalf("仓编码 %q、库区编码 %q、名称 %q，want 逐字 %q、%q、%q",
			command.Warehouse, command.Code, command.Content.Name(), warehouse, zone, name)
	}
}

// 修订号不是正整数的报文译得出，由应用层答成修订错位：译装不提前把它当坏载荷拒掉（同仓那一册）。
func TestAZoneRevisionNumberTheApplicationJudgesIsNotRefusedAsMalformed(t *testing.T) {
	for _, number := range []string{"0", "-3"} {
		body := `{"warehouseCode":"W","zoneCode":"Z","revision":` + number + `,"name":"n","usage":"STORAGE"}`
		if _, err := translateZone(t, body); err != nil {
			t.Fatalf("译装拒了修订号 %s：%v；这一格归应用层判", number, err)
		}
	}
	if command := mustTranslateZone(t, `{"warehouseCode":"W","zoneCode":"Z","revision":-3,"name":"n","usage":"STORAGE"}`); command.Revision != -3 {
		t.Fatalf("revision = %d，want 原样 -3", command.Revision)
	}
}

// Covers: AT-WF-004（库区的部分）——请求自带租户或登记责任方答坏载荷，键在场即拒，不看值：值与认证结果碰巧相同、null 都一样。
func TestACarriedTenantOrRegistrantIsRefusedOnAZoneWhateverItsValue(t *testing.T) {
	const rest = `"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"`
	for name, body := range map[string]string{
		"tenant":                   `{"tenantId":"SYN-TENANT-02",` + rest + `}`,
		"tenant equal to the real": `{"tenantId":"SYN-TENANT-01",` + rest + `}`,
		"tenant null":              `{"tenantId":null,` + rest + `}`,
		"registrant":               `{"registrant":"SYN-OPERATOR-09",` + rest + `}`,
		"registrant null":          `{"registrant":null,` + rest + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateZone(t, body)
			if !errors.Is(err, registrationjson.ErrMalformedDocument) {
				t.Fatalf("err = %v，want ErrMalformedDocument", err)
			}
			var malformed *registrationjson.MalformedDocumentError
			if !errors.As(err, &malformed) || malformed.Reason == "" {
				t.Fatalf("坏载荷要带一句理由：%v", err)
			}
			if strings.Contains(malformed.Reason, "SYN-TENANT") || strings.Contains(malformed.Reason, "SYN-OPERATOR") {
				t.Fatalf("理由回显了报文里的取值：%q", malformed.Reason)
			}
		})
	}
}

// Covers: AT-WF-004（库区的部分）——坏载荷各答 400 一格：JSON 不合法、认不得的键（含仓那一册的格）、必填格缺失或类型不对、用途不在
// 封闭集里、必填文本格是空白。
func TestMalformedZoneDocumentsAreRefused(t *testing.T) {
	cases := map[string]string{
		"empty body":              ``,
		"not JSON":                `not json`,
		"truncated":               `{"warehouseCode":"W","zoneCode":"Z","revision":1`,
		"array at the top":        `[]`,
		"trailing second value":   fullZoneDocument + ` {}`,
		"unknown key":             `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE","floor":3}`,
		"a warehouse cell":        `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE","kind":"AGENT"}`,
		"usage under another key": `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","zoneUsage":"STORAGE"}`,

		"warehouseCode missing": `{"zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"}`,
		"zoneCode missing":      `{"warehouseCode":"W","revision":1,"name":"n","usage":"STORAGE"}`,
		"revision missing":      `{"warehouseCode":"W","zoneCode":"Z","name":"n","usage":"STORAGE"}`,
		"name missing":          `{"warehouseCode":"W","zoneCode":"Z","revision":1,"usage":"STORAGE"}`,
		"usage missing":         `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n"}`,
		"warehouseCode null":    `{"warehouseCode":null,"zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"}`,
		"usage null":            `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":null}`,

		"warehouseCode is number": `{"warehouseCode":1,"zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"}`,
		"zoneCode is number":      `{"warehouseCode":"W","zoneCode":1,"revision":1,"name":"n","usage":"STORAGE"}`,
		"revision is string":      `{"warehouseCode":"W","zoneCode":"Z","revision":"1","name":"n","usage":"STORAGE"}`,
		"revision is fractional":  `{"warehouseCode":"W","zoneCode":"Z","revision":1.5,"name":"n","usage":"STORAGE"}`,
		"name is object":          `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":{},"usage":"STORAGE"}`,
		"usage is number":         `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":1}`,
		"deactivated is string":   `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE","deactivated":"false"}`,

		"usage outside the set":     `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"COLD_STORAGE"}`,
		"usage in lower case":       `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"storage"}`,
		"usage with a space":        `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE "}`,
		"usage empty":               `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":""}`,
		"usage is a warehouse kind": `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"AGENT"}`,

		"warehouseCode blank": `{"warehouseCode":"   ","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"}`,
		"warehouseCode empty": `{"warehouseCode":"","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE"}`,
		"zoneCode blank":      `{"warehouseCode":"W","zoneCode":" \t","revision":1,"name":"n","usage":"STORAGE"}`,
		"zoneCode empty":      `{"warehouseCode":"W","zoneCode":"","revision":1,"name":"n","usage":"STORAGE"}`,
		"name blank":          `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"\n","usage":"STORAGE"}`,
		"name empty":          `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"","usage":"STORAGE"}`,

		"NUL in the zone code": `{"warehouseCode":"W","zoneCode":"Z\u0000","revision":1,"name":"n","usage":"STORAGE"}`,
		"NUL in the name":      `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n\u0000","usage":"STORAGE"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translateZone(t, body)
			if !errors.Is(err, registrationjson.ErrMalformedDocument) {
				t.Fatalf("err = %v，want ErrMalformedDocument", err)
			}
			var malformed *registrationjson.MalformedDocumentError
			if !errors.As(err, &malformed) || malformed.Reason == "" {
				t.Fatalf("坏载荷要带一句理由：%v", err)
			}
		})
	}
}

// 用途不在封闭集里的理由点名封闭集本身，登记方照它改；未知键的理由点名这是一笔库区修订，不借用仓那一册的说法。
func TestTheReasonForAZoneNamesTheClosedSetAndTheKindOfRevision(t *testing.T) {
	_, err := translateZone(t, `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"COLD_STORAGE"}`)
	var malformed *registrationjson.MalformedDocumentError
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v", err)
	}
	for _, usage := range domain.ZoneUsages() {
		if !strings.Contains(malformed.Reason, usage.String()) {
			t.Errorf("理由 %q 没有点名封闭集里的 %s", malformed.Reason, usage)
		}
	}
	if strings.Contains(malformed.Reason, "COLD_STORAGE") {
		t.Errorf("理由回显了报文里的取值：%q", malformed.Reason)
	}

	_, err = translateZone(t, `{"warehouseCode":"W","zoneCode":"Z","revision":1,"name":"n","usage":"STORAGE","floor":3}`)
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "zone revision") {
		t.Errorf("未知键的理由 = %v，want 点名这是一笔库区修订", err)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——错误正文不回显报文里的取值：仓编码、库区编码与名称一个字也不回显。
func TestTheZoneReasonNeverEchoesTheCodesOrTheName(t *testing.T) {
	const secretWarehouse, secretZone, secretName = "SYN-SECRET-WAREHOUSE", "SYN-SECRET-ZONE", "SYN-SECRET-NAME"
	for name, body := range map[string]string{
		"usage outside the set": `{"warehouseCode":"` + secretWarehouse + `","zoneCode":"` + secretZone + `","revision":1,"name":"` + secretName + `","usage":"BAD"}`,
		"revision is string":    `{"warehouseCode":"` + secretWarehouse + `","zoneCode":"` + secretZone + `","revision":"1","name":"` + secretName + `","usage":"STORAGE"}`,
		"tenant carried":        `{"tenantId":"x","warehouseCode":"` + secretWarehouse + `","zoneCode":"` + secretZone + `","revision":1,"name":"` + secretName + `","usage":"STORAGE"}`,
		"name is array":         `{"warehouseCode":"` + secretWarehouse + `","zoneCode":"` + secretZone + `","revision":1,"name":["` + secretName + `"],"usage":"STORAGE"}`,
		"unknown key":           `{"warehouseCode":"` + secretWarehouse + `","zoneCode":"` + secretZone + `","revision":1,"name":"` + secretName + `","usage":"STORAGE","x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateZone(t, body)
			if err == nil {
				t.Fatal("want 坏载荷")
			}
			for _, secret := range []string{secretWarehouse, secretZone, secretName} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("错误正文回显了报文里的取值 %q：%q", secret, err.Error())
				}
			}
		})
	}
}

// Covers: AT-WF-004——请求体读不出（连接断了、请求体检查判到存不下的字节）也是坏载荷，理由不外泄读错的原文。
func TestAZoneBodyThatCannotBeReadIsMalformed(t *testing.T) {
	_, err := registrationjson.ZoneCommand(brokenBody{},
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewRegistrant, "SYN-OPERATOR-01"))
	if !errors.Is(err, registrationjson.ErrMalformedDocument) {
		t.Fatalf("err = %v，want ErrMalformedDocument", err)
	}
	if strings.Contains(err.Error(), "SYN-SECRET") {
		t.Fatalf("错误正文回显了读错的原文：%q", err.Error())
	}
}
