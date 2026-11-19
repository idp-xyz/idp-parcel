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

const fullDocument = `{
	"warehouseCode": "SYN-WH-01",
	"revision": 1,
	"name": "合成一号仓",
	"kind": "SELF_OPERATED",
	"address": {"country": "CN", "text": "合成路 1 号"},
	"businessTimeZone": "Asia/Shanghai",
	"colocatedNode": "SYN-NODE-01",
	"deactivated": true
}`

func valueOf[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func translate(t *testing.T, body string) (application.RegisterWarehouseCommand, error) {
	t.Helper()
	return registrationjson.WarehouseCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

func mustTranslate(t *testing.T, body string) application.RegisterWarehouseCommand {
	t.Helper()
	command, err := translate(t, body)
	if err != nil {
		t.Fatalf("译 %s：%v", body, err)
	}
	return command
}

// Covers: AT-WF-001（仓的部分）——报文齐全时译得出：每一格进命令，租户与登记责任方是调用方传入的那两个。
func TestACompleteDocumentTranslatesIntoTheCommand(t *testing.T) {
	command := mustTranslate(t, fullDocument)

	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("租户与登记责任方 = %q、%q，want 调用方传入的两个", command.Tenant, command.Registrant)
	}
	content := command.Content
	if command.Code.String() != "SYN-WH-01" || command.Revision != 1 || content.Name().String() != "合成一号仓" ||
		content.Kind() != domain.WarehouseKindSelfOperated ||
		content.Address().Country().String() != "CN" || content.Address().Text().String() != "合成路 1 号" ||
		content.BusinessTimeZone().String() != "Asia/Shanghai" || content.ColocatedNode().String() != "SYN-NODE-01" ||
		!content.Deactivated() {
		t.Fatalf("命令 = %+v，内容 = %+v", command, content)
	}
}

// Covers: AT-WF-005、AT-WF-006——业务时区、同址物流节点、是否停用三格可缺：缺失译成没登记、没指名、不停用；显式 null 与缺失同义。
func TestTheOptionalCellsDefaultToNotRegisteredNotNamedAndNotDeactivated(t *testing.T) {
	const minimal = `{"warehouseCode":"SYN-WH-02","revision":1,"name":"合成二号仓","kind":"AGENT","address":{"country":"SG","text":"合成路 2 号"}`
	for name, body := range map[string]string{
		"keys absent":  minimal + `}`,
		"keys is null": minimal + `,"businessTimeZone":null,"colocatedNode":null,"deactivated":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			content := mustTranslate(t, body).Content
			if content.BusinessTimeZone().Registered() || content.ColocatedNode().Named() || content.Deactivated() {
				t.Fatalf("可缺的格缺失时 = (%t, %t, %t)，want 没登记、没指名、不停用",
					content.BusinessTimeZone().Registered(), content.ColocatedNode().Named(), content.Deactivated())
			}
			if content.Kind() != domain.WarehouseKindAgent {
				t.Fatalf("kind = %s，want AGENT", content.Kind())
			}
		})
	}
}

// Covers: AT-WF-001「仓编码逐字保存」、AT-WF-005「节点标识逐字保存」——仓编码、地址正文、同址物流节点逐字交给领域：带首尾空白、
// 内部连续空白、换行与大小写混写，一个字节都不动。
func TestTheCodeAddressTextAndNodeAreHandedOverVerbatim(t *testing.T) {
	const code, text, node = " Syn-WH  01\t", "  合成路\n 1  号 ", " syn-Node 01 "
	body := `{"warehouseCode":" Syn-WH  01\t","revision":2,"name":"合成仓","kind":"AGENT",` +
		`"address":{"country":"CN","text":"  合成路\n 1  号 "},"colocatedNode":" syn-Node 01 "}`

	command := mustTranslate(t, body)
	if command.Code.String() != code || command.Content.Address().Text().String() != text || command.Content.ColocatedNode().String() != node {
		t.Fatalf("仓编码 %q、地址正文 %q、节点 %q，want 逐字 %q、%q、%q",
			command.Code, command.Content.Address().Text(), command.Content.ColocatedNode(), code, text, node)
	}
}

// Covers: AT-WF-004（国家）、AT-WF-006——国家码不在内置目录、业务时区认不出的报文译得出，由应用层答 NOT_ACCEPTED/CONTENT_INVALID：
// 译装不提前把它们当坏载荷拒掉。修订号不是正整数同理——应用层把它答成修订错位。
func TestValuesTheApplicationJudgesAreNotRefusedAsMalformed(t *testing.T) {
	for name, body := range map[string]string{
		"country outside the catalogue": `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"ZZ","text":"t"}}`,
		"country in lower case":         `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"cn","text":"t"}}`,
		"time zone not recognised":      `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"businessTimeZone":"Mars/Olympus"}`,
		"revision zero":                 `{"warehouseCode":"W","revision":0,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"revision negative":             `{"warehouseCode":"W","revision":-3,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := translate(t, body); err != nil {
				t.Fatalf("译装拒了这份报文：%v；这一格归应用层判", err)
			}
		})
	}
	if command := mustTranslate(t, `{"warehouseCode":"W","revision":-3,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`); command.Revision != -3 {
		t.Fatalf("revision = %d，want 原样 -3", command.Revision)
	}
}

// Covers: AT-WF-004——请求自带租户或登记责任方答坏载荷，键在场即拒，不看值：值与认证结果碰巧相同、null 都一样。
func TestACarriedTenantOrRegistrantIsRefusedWhateverItsValue(t *testing.T) {
	const rest = `"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}`
	for name, body := range map[string]string{
		"tenant":                   `{"tenantId":"SYN-TENANT-02",` + rest + `}`,
		"tenant equal to the real": `{"tenantId":"SYN-TENANT-01",` + rest + `}`,
		"tenant null":              `{"tenantId":null,` + rest + `}`,
		"registrant":               `{"registrant":"SYN-OPERATOR-09",` + rest + `}`,
		"registrant null":          `{"registrant":null,` + rest + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translate(t, body)
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

// Covers: AT-WF-004——坏载荷各答 400 一格：JSON 不合法、认不得的键、必填格缺失或类型不对、种类不在封闭集里、必填文本格是空白。
func TestMalformedDocumentsAreRefused(t *testing.T) {
	cases := map[string]string{
		"empty body":             ``,
		"not JSON":               `not json`,
		"truncated":              `{"warehouseCode":"W","revision":1`,
		"array at the top":       `[]`,
		"trailing second value":  fullDocument + ` {}`,
		"unknown key":            `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"zone":"Z1"}`,
		"unknown key in address": `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t","city":"c"}}`,

		"warehouseCode missing": `{"revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"revision missing":      `{"warehouseCode":"W","name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"name missing":          `{"warehouseCode":"W","revision":1,"kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"kind missing":          `{"warehouseCode":"W","revision":1,"name":"n","address":{"country":"CN","text":"t"}}`,
		"address missing":       `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT"}`,
		"address null":          `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":null}`,
		"country missing":       `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"text":"t"}}`,
		"address text missing":  `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN"}}`,
		"warehouseCode null":    `{"warehouseCode":null,"revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,

		"warehouseCode is number": `{"warehouseCode":1,"revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"revision is string":      `{"warehouseCode":"W","revision":"1","name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"revision is fractional":  `{"warehouseCode":"W","revision":1.5,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"name is object":          `{"warehouseCode":"W","revision":1,"name":{},"kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"kind is number":          `{"warehouseCode":"W","revision":1,"name":"n","kind":1,"address":{"country":"CN","text":"t"}}`,
		"address is string":       `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":"CN"}`,
		"country is number":       `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":86,"text":"t"}}`,
		"address text is array":   `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":[]}}`,
		"zone is number":          `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"businessTimeZone":8}`,
		"node is number":          `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"colocatedNode":8}`,
		"deactivated is string":   `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"deactivated":"false"}`,

		"kind outside the set": `{"warehouseCode":"W","revision":1,"name":"n","kind":"PARTNER","address":{"country":"CN","text":"t"}}`,
		"kind in lower case":   `{"warehouseCode":"W","revision":1,"name":"n","kind":"agent","address":{"country":"CN","text":"t"}}`,
		"kind empty":           `{"warehouseCode":"W","revision":1,"name":"n","kind":"","address":{"country":"CN","text":"t"}}`,

		"warehouseCode blank":  `{"warehouseCode":"   ","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"warehouseCode empty":  `{"warehouseCode":"","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"name blank":           `{"warehouseCode":"W","revision":1,"name":" \t","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"country blank":        `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":" ","text":"t"}}`,
		"address text blank":   `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"\n"}}`,
		"zone given but blank": `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"businessTimeZone":" "}`,
		"node given but blank": `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"},"colocatedNode":""}`,

		"NUL in the code":         `{"warehouseCode":"W\u0000","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t"}}`,
		"NUL in the address text": `{"warehouseCode":"W","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"t\u0000"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translate(t, body)
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

// Covers: UC-WF-001「权限、隔离与审计」——错误正文不带地址正文：坏载荷里的取值（地址正文、仓编码）一个字也不回显。
func TestTheReasonNeverEchoesTheAddressTextOrTheCode(t *testing.T) {
	const secretText, secretCode = "SYN-SECRET-ADDRESS-TEXT", "SYN-SECRET-CODE"
	for name, body := range map[string]string{
		"kind outside the set":  `{"warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"PARTNER","address":{"country":"CN","text":"` + secretText + `"}}`,
		"revision is string":    `{"warehouseCode":"` + secretCode + `","revision":"1","name":"n","kind":"AGENT","address":{"country":"CN","text":"` + secretText + `"}}`,
		"country blank":         `{"warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"AGENT","address":{"country":" ","text":"` + secretText + `"}}`,
		"zone given but blank":  `{"warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"` + secretText + `"},"businessTimeZone":""}`,
		"tenant carried":        `{"tenantId":"x","warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":"` + secretText + `"}}`,
		"address text is array": `{"warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"AGENT","address":{"country":"CN","text":["` + secretText + `"]}}`,
		"address is a string":   `{"warehouseCode":"` + secretCode + `","revision":1,"name":"n","kind":"AGENT","address":"` + secretText + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translate(t, body)
			if err == nil {
				t.Fatal("want 坏载荷")
			}
			if strings.Contains(err.Error(), secretText) || strings.Contains(err.Error(), secretCode) {
				t.Fatalf("错误正文回显了报文里的取值：%q", err.Error())
			}
		})
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by SYN-SECRET-ADDRESS-TEXT")
}

// Covers: AT-WF-004——请求体读不出（连接断了、请求体检查判到存不下的字节）也是坏载荷，理由不外泄读错的原文。
func TestABodyThatCannotBeReadIsMalformed(t *testing.T) {
	_, err := registrationjson.WarehouseCommand(brokenBody{},
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewRegistrant, "SYN-OPERATOR-01"))
	if !errors.Is(err, registrationjson.ErrMalformedDocument) {
		t.Fatalf("err = %v，want ErrMalformedDocument", err)
	}
	if strings.Contains(err.Error(), "SYN-SECRET") {
		t.Fatalf("错误正文回显了读错的原文：%q", err.Error())
	}
}
