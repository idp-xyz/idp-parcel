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

const fullLocationTypeDocument = `{
	"locationTypeCode": "SYN-LT-01",
	"revision": 1,
	"name": "合成货架位",
	"deactivated": true
}`

func translateLocationType(t *testing.T, body string) (application.RegisterLocationTypeCommand, error) {
	t.Helper()
	return registrationjson.LocationTypeCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

func mustTranslateLocationType(t *testing.T, body string) application.RegisterLocationTypeCommand {
	t.Helper()
	command, err := translateLocationType(t, body)
	if err != nil {
		t.Fatalf("译 %s：%v", body, err)
	}
	return command
}

// Covers: AT-WF-001（库位类型的部分）——报文齐全时译得出：每一格进命令，租户与登记责任方是调用方传入的那两个。
func TestACompleteLocationTypeDocumentTranslatesIntoTheCommand(t *testing.T) {
	command := mustTranslateLocationType(t, fullLocationTypeDocument)

	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("租户与登记责任方 = %q、%q，want 调用方传入的两个", command.Tenant, command.Registrant)
	}
	if command.Code.String() != "SYN-LT-01" || command.Revision != 1 ||
		command.Content.Name().String() != "合成货架位" || !command.Content.Deactivated() {
		t.Fatalf("命令 = %+v，内容 = %+v", command, command.Content)
	}
}

// 是否停用可缺：缺失译成不停用，显式 null 与缺失同义。
func TestLocationTypeDeactivatedDefaultsToNotDeactivated(t *testing.T) {
	const minimal = `{"locationTypeCode":"L","revision":1,"name":"n"`
	for name, body := range map[string]string{
		"key absent":  minimal + `}`,
		"key is null": minimal + `,"deactivated":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			if mustTranslateLocationType(t, body).Content.Deactivated() {
				t.Fatalf("缺失的 deactivated 该译成不停用")
			}
		})
	}
}

// Covers: AT-WF-001「编码逐字保存」——库位类型编码与名称逐字交给领域：带首尾空白、内部连续空白、换行与大小写混写，一个字节都不动。
func TestTheLocationTypeCodeAndNameAreHandedOverVerbatim(t *testing.T) {
	const code, name = "  Rack\n 01 ", "  合成  货架位 "
	body := `{"locationTypeCode":"  Rack\n 01 ","revision":2,"name":"  合成  货架位 "}`

	command := mustTranslateLocationType(t, body)
	if command.Code.String() != code || command.Content.Name().String() != name {
		t.Fatalf("库位类型编码 %q、名称 %q，want 逐字 %q、%q", command.Code, command.Content.Name(), code, name)
	}
}

// 修订号不是正整数的报文译得出，由应用层答成修订错位：译装不提前把它当坏载荷拒掉（同仓、库区那两册）。
func TestALocationTypeRevisionNumberTheApplicationJudgesIsNotRefusedAsMalformed(t *testing.T) {
	for _, number := range []string{"0", "-3"} {
		body := `{"locationTypeCode":"L","revision":` + number + `,"name":"n"}`
		if _, err := translateLocationType(t, body); err != nil {
			t.Fatalf("译装拒了修订号 %s：%v；这一格归应用层判", number, err)
		}
	}
	if command := mustTranslateLocationType(t, `{"locationTypeCode":"L","revision":-3,"name":"n"}`); command.Revision != -3 {
		t.Fatalf("revision = %d，want 原样 -3", command.Revision)
	}
}

// Covers: AT-WF-004——请求自带租户或登记责任方答坏载荷，键在场即拒，不看值：值与认证结果碰巧相同、null 都一样。
func TestACarriedTenantOrRegistrantIsRefusedOnALocationTypeWhateverItsValue(t *testing.T) {
	const rest = `"locationTypeCode":"L","revision":1,"name":"n"`
	for name, body := range map[string]string{
		"tenant":                   `{"tenantId":"SYN-TENANT-02",` + rest + `}`,
		"tenant equal to the real": `{"tenantId":"SYN-TENANT-01",` + rest + `}`,
		"tenant null":              `{"tenantId":null,` + rest + `}`,
		"registrant":               `{"registrant":"SYN-OPERATOR-09",` + rest + `}`,
		"registrant null":          `{"registrant":null,` + rest + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocationType(t, body)
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

// Covers: AT-WF-004——坏载荷各答 400 一格：JSON 不合法、认不得的键（含别的册的格）、必填格缺失或类型不对、必填文本格是空白。
func TestMalformedLocationTypeDocumentsAreRefused(t *testing.T) {
	cases := map[string]string{
		"empty body":            ``,
		"not JSON":              `not json`,
		"truncated":             `{"locationTypeCode":"L","revision":1`,
		"array at the top":      `[]`,
		"trailing second value": fullLocationTypeDocument + ` {}`,
		"unknown key":           `{"locationTypeCode":"L","revision":1,"name":"n","floor":3}`,
		"a zone cell":           `{"locationTypeCode":"L","revision":1,"name":"n","usage":"STORAGE"}`,
		"a packaging cell":      `{"locationTypeCode":"L","revision":1,"name":"n","weight":{"value":"1","unit":"KG"}}`,
		"code under a zone key": `{"zoneCode":"L","revision":1,"name":"n"}`,

		"locationTypeCode missing": `{"revision":1,"name":"n"}`,
		"revision missing":         `{"locationTypeCode":"L","name":"n"}`,
		"name missing":             `{"locationTypeCode":"L","revision":1}`,
		"locationTypeCode null":    `{"locationTypeCode":null,"revision":1,"name":"n"}`,
		"name null":                `{"locationTypeCode":"L","revision":1,"name":null}`,

		"locationTypeCode is number": `{"locationTypeCode":1,"revision":1,"name":"n"}`,
		"revision is string":         `{"locationTypeCode":"L","revision":"1","name":"n"}`,
		"revision is fractional":     `{"locationTypeCode":"L","revision":1.5,"name":"n"}`,
		"name is object":             `{"locationTypeCode":"L","revision":1,"name":{}}`,
		"deactivated is string":      `{"locationTypeCode":"L","revision":1,"name":"n","deactivated":"false"}`,

		"locationTypeCode blank": `{"locationTypeCode":"   ","revision":1,"name":"n"}`,
		"locationTypeCode empty": `{"locationTypeCode":"","revision":1,"name":"n"}`,
		"name blank":             `{"locationTypeCode":"L","revision":1,"name":"\n"}`,
		"name empty":             `{"locationTypeCode":"L","revision":1,"name":""}`,

		"NUL in the code": `{"locationTypeCode":"L\u0000","revision":1,"name":"n"}`,
		"NUL in the name": `{"locationTypeCode":"L","revision":1,"name":"n\u0000"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocationType(t, body)
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

// 未知键的理由点名这是一笔库位类型修订，不借用别的册的说法。
func TestTheUnknownKeyReasonOfALocationTypeNamesTheKindOfRevision(t *testing.T) {
	_, err := translateLocationType(t, `{"locationTypeCode":"L","revision":1,"name":"n","floor":3}`)
	var malformed *registrationjson.MalformedDocumentError
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "location type revision") {
		t.Errorf("未知键的理由 = %v，want 点名这是一笔库位类型修订", err)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——错误正文不回显报文里的取值：库位类型编码与名称一个字也不回显。
func TestTheLocationTypeReasonNeverEchoesTheCodeOrTheName(t *testing.T) {
	const secretCode, secretName = "SYN-SECRET-LOCATION-TYPE", "SYN-SECRET-NAME"
	for name, body := range map[string]string{
		"revision is string": `{"locationTypeCode":"` + secretCode + `","revision":"1","name":"` + secretName + `"}`,
		"tenant carried":     `{"tenantId":"x","locationTypeCode":"` + secretCode + `","revision":1,"name":"` + secretName + `"}`,
		"name is array":      `{"locationTypeCode":"` + secretCode + `","revision":1,"name":["` + secretName + `"]}`,
		"unknown key":        `{"locationTypeCode":"` + secretCode + `","revision":1,"name":"` + secretName + `","x":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocationType(t, body)
			if err == nil {
				t.Fatal("want 坏载荷")
			}
			for _, secret := range []string{secretCode, secretName} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("错误正文回显了报文里的取值 %q：%q", secret, err.Error())
				}
			}
		})
	}
}

// Covers: AT-WF-004——请求体读不出（连接断了、请求体检查判到存不下的字节）也是坏载荷，理由不外泄读错的原文。
func TestALocationTypeBodyThatCannotBeReadIsMalformed(t *testing.T) {
	_, err := registrationjson.LocationTypeCommand(brokenBody{},
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewRegistrant, "SYN-OPERATOR-01"))
	if !errors.Is(err, registrationjson.ErrMalformedDocument) {
		t.Fatalf("err = %v，want ErrMalformedDocument", err)
	}
	if strings.Contains(err.Error(), "SYN-SECRET") {
		t.Fatalf("错误正文回显了读错的原文：%q", err.Error())
	}
}
