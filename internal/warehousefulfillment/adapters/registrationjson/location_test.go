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

const fullLocationDocument = `{
	"warehouseCode": "SYN-WH-01",
	"locationCode": "SYN-LOC-01",
	"revision": 1,
	"zoneCode": "SYN-ZONE-01",
	"locationTypeCode": "SYN-LT-01",
	"priority": -3,
	"deactivated": true
}`

func translateLocation(t *testing.T, body string) (application.RegisterLocationCommand, error) {
	t.Helper()
	return registrationjson.LocationCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

func mustTranslateLocation(t *testing.T, body string) application.RegisterLocationCommand {
	t.Helper()
	command, err := translateLocation(t, body)
	if err != nil {
		t.Fatalf("译 %s：%v", body, err)
	}
	return command
}

// locationBody 拼一份只带必填格的报文，extra 是紧接其后的可选格（含前导逗号）。
func locationBody(extra string) string {
	return `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":"Z"` + extra + `}`
}

// Covers: AT-WF-001（库位的部分）——报文齐全时译得出：每一格进命令，租户与登记责任方是调用方传入的那两个。
func TestACompleteLocationDocumentTranslatesIntoTheCommand(t *testing.T) {
	command := mustTranslateLocation(t, fullLocationDocument)

	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("租户与登记责任方 = %q、%q，want 调用方传入的两个", command.Tenant, command.Registrant)
	}
	locationType, hasType := command.Content.LocationType()
	if command.Warehouse.String() != "SYN-WH-01" || command.Code.String() != "SYN-LOC-01" || command.Revision != 1 ||
		command.Content.ZoneCode().String() != "SYN-ZONE-01" || !hasType || locationType.String() != "SYN-LT-01" ||
		!command.Content.Priority().Given() || command.Content.Priority().Value() != -3 || !command.Content.Deactivated() {
		t.Fatalf("命令 = %+v，内容 = %+v", command, command.Content)
	}
}

// Covers: AT-WF-001（库位的部分）——可缺的三格：库位类型、优先级、是否停用。缺失译成没有、没有、不停用，不取任何默认；显式 null 与
// 缺失同义。优先级 0 与没有优先级是不同的内容。
func TestTheOptionalLocationCellsDefaultToNothingAndNullMeansAbsent(t *testing.T) {
	for name, body := range map[string]string{
		"keys absent": locationBody(``),
		"keys null":   locationBody(`,"locationTypeCode":null,"priority":null,"deactivated":null`),
	} {
		t.Run(name, func(t *testing.T) {
			content := mustTranslateLocation(t, body).Content
			if _, has := content.LocationType(); has || content.Priority().Given() || content.Deactivated() {
				t.Fatalf("缺失的可选格译成了 类型 %v、优先级 %v、停用 %v，want 没有、没有、不停用", has, content.Priority().Given(), content.Deactivated())
			}
		})
	}

	zero := mustTranslateLocation(t, locationBody(`,"priority":0`)).Content
	absent := mustTranslateLocation(t, locationBody(``)).Content
	if !zero.Priority().Given() || zero.Priority().Value() != 0 || zero == absent {
		t.Fatalf("优先级 0 译成 (%v, %d)，且与没有优先级不是同一份内容 = %v", zero.Priority().Given(), zero.Priority().Value(), zero != absent)
	}
}

// Covers: AT-WF-004（库位的部分）——优先级是 JSON 整数，可以为负，范围是 32 位有符号整数，两端都译得出。
func TestTheLocationPriorityTranslatesEveryIntegerOfThirtyTwoBits(t *testing.T) {
	for literal, want := range map[string]int32{
		"0": 0, "1": 1, "-1": -1, "100": 100, "2147483647": 2147483647, "-2147483648": -2147483648, "-0": 0,
	} {
		content := mustTranslateLocation(t, locationBody(`,"priority":`+literal)).Content
		if !content.Priority().Given() || content.Priority().Value() != want {
			t.Errorf("优先级 %s 译成 (%v, %d)，want %d", literal, content.Priority().Given(), content.Priority().Value(), want)
		}
	}
}

// Covers: AT-WF-001「库位编码逐字保存」——仓编码、库位编码、所属库区编码与库位类型编码逐字交给领域：带首尾空白、内部连续空白、
// 换行与大小写混写，一个字节都不动。
func TestTheLocationCodesAreHandedOverVerbatim(t *testing.T) {
	const warehouse, location, zone, locationType = " Syn-WH  01\t", "  loc\n 01 ", " Zone\t1 ", "  Lt  1 "
	body := `{"warehouseCode":" Syn-WH  01\t","locationCode":"  loc\n 01 ","revision":2,"zoneCode":" Zone\t1 ","locationTypeCode":"  Lt  1 "}`

	command := mustTranslateLocation(t, body)
	gotType, _ := command.Content.LocationType()
	if command.Warehouse.String() != warehouse || command.Code.String() != location ||
		command.Content.ZoneCode().String() != zone || gotType.String() != locationType {
		t.Fatalf("仓编码 %q、库位编码 %q、库区编码 %q、库位类型编码 %q，want 逐字 %q、%q、%q、%q",
			command.Warehouse, command.Code, command.Content.ZoneCode(), gotType, warehouse, location, zone, locationType)
	}
}

// 修订号不是正整数的报文译得出，由应用层答成修订错位：译装不提前把它当坏载荷拒掉（同仓那一册）。
func TestALocationRevisionNumberTheApplicationJudgesIsNotRefusedAsMalformed(t *testing.T) {
	for _, number := range []string{"0", "-3"} {
		body := `{"warehouseCode":"W","locationCode":"L","revision":` + number + `,"zoneCode":"Z"}`
		if _, err := translateLocation(t, body); err != nil {
			t.Fatalf("译装拒了修订号 %s：%v；这一格归应用层判", number, err)
		}
	}
	if command := mustTranslateLocation(t, `{"warehouseCode":"W","locationCode":"L","revision":-3,"zoneCode":"Z"}`); command.Revision != -3 {
		t.Fatalf("revision = %d，want 原样 -3", command.Revision)
	}
}

// Covers: AT-WF-004（库位的部分）——请求自带租户或登记责任方答坏载荷，键在场即拒，不看值：值与认证结果碰巧相同、null 都一样。
func TestACarriedTenantOrRegistrantIsRefusedOnALocationWhateverItsValue(t *testing.T) {
	const rest = `"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":"Z"`
	for name, body := range map[string]string{
		"tenant":                   `{"tenantId":"SYN-TENANT-02",` + rest + `}`,
		"tenant equal to the real": `{"tenantId":"SYN-TENANT-01",` + rest + `}`,
		"tenant null":              `{"tenantId":null,` + rest + `}`,
		"registrant":               `{"registrant":"SYN-OPERATOR-09",` + rest + `}`,
		"registrant null":          `{"registrant":null,` + rest + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocation(t, body)
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

// Covers: AT-WF-004（库位的部分）——坏载荷各答 400 一格：JSON 不合法、认不得的键（含库区那一册的格）、必填格缺失或类型不对、必填文本格
// 是空白、给了库位类型编码却是空白、优先级不是 32 位整数（带小数、指数写法、字符串、布尔、对象、超出范围）。
func TestMalformedLocationDocumentsAreRefused(t *testing.T) {
	cases := map[string]string{
		"empty body":             ``,
		"not JSON":               `not json`,
		"truncated":              `{"warehouseCode":"W","locationCode":"L","revision":1`,
		"array at the top":       `[]`,
		"trailing second value":  fullLocationDocument + ` {}`,
		"unknown key":            locationBody(`,"floor":3`),
		"a zone cell":            locationBody(`,"usage":"STORAGE"`),
		"type under another key": `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":"Z","locationType":"T"}`,

		"warehouseCode missing": `{"locationCode":"L","revision":1,"zoneCode":"Z"}`,
		"locationCode missing":  `{"warehouseCode":"W","revision":1,"zoneCode":"Z"}`,
		"revision missing":      `{"warehouseCode":"W","locationCode":"L","zoneCode":"Z"}`,
		"zoneCode missing":      `{"warehouseCode":"W","locationCode":"L","revision":1}`,
		"warehouseCode null":    `{"warehouseCode":null,"locationCode":"L","revision":1,"zoneCode":"Z"}`,
		"zoneCode null":         `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":null}`,

		"warehouseCode is number":    `{"warehouseCode":1,"locationCode":"L","revision":1,"zoneCode":"Z"}`,
		"locationCode is number":     `{"warehouseCode":"W","locationCode":1,"revision":1,"zoneCode":"Z"}`,
		"zoneCode is number":         `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":1}`,
		"revision is string":         `{"warehouseCode":"W","locationCode":"L","revision":"1","zoneCode":"Z"}`,
		"revision is fractional":     `{"warehouseCode":"W","locationCode":"L","revision":1.5,"zoneCode":"Z"}`,
		"locationTypeCode is number": locationBody(`,"locationTypeCode":1`),
		"deactivated is string":      locationBody(`,"deactivated":"false"`),

		"warehouseCode blank": `{"warehouseCode":"   ","locationCode":"L","revision":1,"zoneCode":"Z"}`,
		"warehouseCode empty": `{"warehouseCode":"","locationCode":"L","revision":1,"zoneCode":"Z"}`,
		"locationCode blank":  `{"warehouseCode":"W","locationCode":" \t","revision":1,"zoneCode":"Z"}`,
		"locationCode empty":  `{"warehouseCode":"W","locationCode":"","revision":1,"zoneCode":"Z"}`,
		"zoneCode blank":      `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":"\n"}`,
		"zoneCode empty":      `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":""}`,

		"locationTypeCode blank": locationBody(`,"locationTypeCode":"  "`),
		"locationTypeCode empty": locationBody(`,"locationTypeCode":""`),

		"priority fractional":   locationBody(`,"priority":1.5`),
		"priority one point 0":  locationBody(`,"priority":1.0`),
		"priority exponent":     locationBody(`,"priority":1e2`),
		"priority is string":    locationBody(`,"priority":"5"`),
		"priority is blank":     locationBody(`,"priority":""`),
		"priority is bool":      locationBody(`,"priority":true`),
		"priority is object":    locationBody(`,"priority":{}`),
		"priority is array":     locationBody(`,"priority":[1]`),
		"priority too large":    locationBody(`,"priority":2147483648`),
		"priority too small":    locationBody(`,"priority":-2147483649`),
		"priority far too big":  locationBody(`,"priority":99999999999999999999`),
		"priority leading plus": locationBody(`,"priority":+1`),

		"NUL in the location code": `{"warehouseCode":"W","locationCode":"L\u0000","revision":1,"zoneCode":"Z"}`,
		"NUL in the zone code":     `{"warehouseCode":"W","locationCode":"L","revision":1,"zoneCode":"Z\u0000"}`,
		"NUL in the type code":     locationBody(`,"locationTypeCode":"T\u0000"`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocation(t, body)
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

// 优先级的理由点名这一格要的是什么，登记方照它改；未知键的理由点名这是一笔库位修订，不借用别的册的说法。
func TestTheReasonForALocationNamesThePriorityRuleAndTheKindOfRevision(t *testing.T) {
	_, err := translateLocation(t, locationBody(`,"priority":1.5`))
	var malformed *registrationjson.MalformedDocumentError
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "priority") || !strings.Contains(malformed.Reason, "32-bit") {
		t.Fatalf("优先级的理由 = %v，want 点名 priority 与 32 位整数", err)
	}

	_, err = translateLocation(t, locationBody(`,"floor":3`))
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "location revision") {
		t.Errorf("未知键的理由 = %v，want 点名这是一笔库位修订", err)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——错误正文不回显报文里的取值：仓编码、库位编码、库区编码、库位类型编码与优先级一个字也不回显。
func TestTheLocationReasonNeverEchoesTheCodesOrThePriority(t *testing.T) {
	const secretWarehouse, secretLocation, secretZone, secretType = "SYN-SECRET-WAREHOUSE", "SYN-SECRET-LOCATION", "SYN-SECRET-ZONE", "SYN-SECRET-TYPE"
	head := `"warehouseCode":"` + secretWarehouse + `","locationCode":"` + secretLocation + `","zoneCode":"` + secretZone + `","locationTypeCode":"` + secretType + `"`
	for name, body := range map[string]string{
		"revision is string":  `{` + head + `,"revision":"1"}`,
		"priority fractional": `{` + head + `,"revision":1,"priority":123456.789}`,
		"priority is string":  `{` + head + `,"revision":1,"priority":"SYN-SECRET-PRIORITY"}`,
		"tenant carried":      `{"tenantId":"x",` + head + `,"revision":1}`,
		"unknown key":         `{` + head + `,"revision":1,"x":1}`,
		"type is array":       `{"warehouseCode":"` + secretWarehouse + `","locationCode":"` + secretLocation + `","zoneCode":"` + secretZone + `","revision":1,"locationTypeCode":["` + secretType + `"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translateLocation(t, body)
			if err == nil {
				t.Fatal("want 坏载荷")
			}
			for _, secret := range []string{secretWarehouse, secretLocation, secretZone, secretType, "SYN-SECRET-PRIORITY", "123456"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("错误正文回显了报文里的取值 %q：%q", secret, err.Error())
				}
			}
		})
	}
}

// Covers: AT-WF-004——请求体读不出（连接断了、请求体检查判到存不下的字节）也是坏载荷，理由不外泄读错的原文。
func TestALocationBodyThatCannotBeReadIsMalformed(t *testing.T) {
	_, err := registrationjson.LocationCommand(brokenBody{},
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewRegistrant, "SYN-OPERATOR-01"))
	if !errors.Is(err, registrationjson.ErrMalformedDocument) {
		t.Fatalf("err = %v，want ErrMalformedDocument", err)
	}
	if strings.Contains(err.Error(), "SYN-SECRET") {
		t.Fatalf("错误正文回显了读错的原文：%q", err.Error())
	}
}
