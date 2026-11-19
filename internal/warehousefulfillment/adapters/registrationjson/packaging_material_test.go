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

const fullPackagingMaterialDocument = `{
	"packagingMaterialCode": "SYN-BOX-01",
	"revision": 1,
	"name": "合成一号纸箱",
	"dimensions": {"length": "30", "width": "20", "height": "15.5", "unit": "CM"},
	"weight": {"value": "0.25", "unit": "KG"},
	"deactivated": true
}`

// packagingDocument 拼一份包材报文，用例只改它关心的那一格；四个数值与两个单位都以 JSON 字符串给出（next 里 raw 是原样的 JSON 片段）。
func packagingDocument(length, width, height, dimensionsUnit, weight, weightUnit string) string {
	return `{"packagingMaterialCode":"SYN-BOX-01","revision":1,"name":"n",` +
		`"dimensions":{"length":` + length + `,"width":` + width + `,"height":` + height + `,"unit":` + dimensionsUnit + `},` +
		`"weight":{"value":` + weight + `,"unit":` + weightUnit + `}}`
}

func translatePackagingMaterial(t *testing.T, body string) (application.RegisterPackagingMaterialCommand, error) {
	t.Helper()
	return registrationjson.PackagingMaterialCommand(
		strings.NewReader(body),
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewRegistrant, "https://id.syn.example/dex#SYN-OPERATOR-01"),
	)
}

func mustTranslatePackagingMaterial(t *testing.T, body string) application.RegisterPackagingMaterialCommand {
	t.Helper()
	command, err := translatePackagingMaterial(t, body)
	if err != nil {
		t.Fatalf("译 %s：%v", body, err)
	}
	return command
}

// Covers: AT-WF-001（包材的部分）——报文齐全时译得出：每一格进命令，租户与登记责任方是调用方传入的那两个；数值进命令时是去掉多余零的规范写法。
func TestACompletePackagingMaterialDocumentTranslatesIntoTheCommand(t *testing.T) {
	command := mustTranslatePackagingMaterial(t, fullPackagingMaterialDocument)

	if command.Tenant.String() != "SYN-TENANT-01" || command.Registrant.String() != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("租户与登记责任方 = %q、%q，want 调用方传入的两个", command.Tenant, command.Registrant)
	}
	content := command.Content
	if command.Code.String() != "SYN-BOX-01" || command.Revision != 1 || content.Name().String() != "合成一号纸箱" || !content.Deactivated() ||
		content.Length().String() != "30" || content.Width().String() != "20" || content.Height().String() != "15.5" ||
		content.DimensionsUnit().String() != "CM" || content.Weight().String() != "0.25" || content.WeightUnit().String() != "KG" {
		t.Fatalf("命令 = %+v，内容 = %+v", command, content)
	}
}

// 是否停用可缺：缺失译成不停用，显式 null 与缺失同义。
func TestPackagingMaterialDeactivatedDefaultsToNotDeactivated(t *testing.T) {
	const minimal = `{"packagingMaterialCode":"B","revision":1,"name":"n",` +
		`"dimensions":{"length":"1","width":"1","height":"1","unit":"CM"},"weight":{"value":"1","unit":"KG"}`
	for name, body := range map[string]string{
		"key absent":  minimal + `}`,
		"key is null": minimal + `,"deactivated":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			if mustTranslatePackagingMaterial(t, body).Content.Deactivated() {
				t.Fatalf("缺失的 deactivated 该译成不停用")
			}
		})
	}
}

// Covers: AT-WF-001「编码逐字保存」——包材编码、名称与单位码逐字交给领域：带首尾空白、内部连续空白、换行与大小写混写，一个字节都不动。单位码 `cm`
// 译装不折成 `CM`：它在不在目录里由应用层按码逐字判。
func TestThePackagingMaterialCodeNameAndUnitsAreHandedOverVerbatim(t *testing.T) {
	const code, name = "  Box\n 01 ", "  合成  纸箱 "
	body := `{"packagingMaterialCode":"  Box\n 01 ","revision":2,"name":"  合成  纸箱 ",` +
		`"dimensions":{"length":"1","width":"1","height":"1","unit":"cm"},"weight":{"value":"1","unit":"Kg"}}`

	command := mustTranslatePackagingMaterial(t, body)
	if command.Code.String() != code || command.Content.Name().String() != name ||
		command.Content.DimensionsUnit().String() != "cm" || command.Content.WeightUnit().String() != "Kg" {
		t.Fatalf("包材编码 %q、名称 %q、单位 %q/%q，want 逐字 %q、%q、cm/Kg",
			command.Code, command.Content.Name(), command.Content.DimensionsUnit(), command.Content.WeightUnit(), code, name)
	}
}

// 修订号不是正整数的报文译得出，由应用层答成修订错位：译装不提前把它当坏载荷拒掉（同别的册）。
func TestAPackagingMaterialRevisionNumberTheApplicationJudgesIsNotRefusedAsMalformed(t *testing.T) {
	for _, number := range []string{"0", "-3"} {
		body := strings.Replace(packagingDocument(`"1"`, `"1"`, `"1"`, `"CM"`, `"1"`, `"KG"`), `"revision":1`, `"revision":`+number, 1)
		if _, err := translatePackagingMaterial(t, body); err != nil {
			t.Fatalf("译装拒了修订号 %s：%v；这一格归应用层判", number, err)
		}
	}
}

// Covers: AT-WF-004（包材的部分）——数值为零、单位不在目录里的报文译得出，不当坏载荷：「为零」「不在目录」「类别不对」答 CONTENT_INVALID 是应用层在
// 判过重放与冲突之后的事，译装提前拒掉，同一份内容在原样重投时就会先于「已登记（重放）」被拒。
func TestZeroValuesAndUnknownUnitsTranslateAndAreLeftToTheApplication(t *testing.T) {
	for name, body := range map[string]string{
		"length zero":       packagingDocument(`"0"`, `"1"`, `"1"`, `"CM"`, `"1"`, `"KG"`),
		"width 0.0":         packagingDocument(`"1"`, `"0.0"`, `"1"`, `"CM"`, `"1"`, `"KG"`),
		"height 000":        packagingDocument(`"1"`, `"1"`, `"000"`, `"CM"`, `"1"`, `"KG"`),
		"weight zero":       packagingDocument(`"1"`, `"1"`, `"1"`, `"CM"`, `"0.000000"`, `"KG"`),
		"unit not in set":   packagingDocument(`"1"`, `"1"`, `"1"`, `"FURLONG"`, `"1"`, `"KG"`),
		"weight unit CM":    packagingDocument(`"1"`, `"1"`, `"1"`, `"CM"`, `"1"`, `"CM"`),
		"dimension unit KG": packagingDocument(`"1"`, `"1"`, `"1"`, `"KG"`, `"1"`, `"KG"`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := translatePackagingMaterial(t, body); err != nil {
				t.Fatalf("译装拒了：%v；这一格归应用层判", err)
			}
		})
	}
}

// Covers: AT-WF-004（包材的部分）「数值按数值比，不按写法比」——同一个数的不同写法译成相等的内容（直接用 == 比），不同的数、不同的单位译成不同的内容。
func TestPackagingMaterialNumbersOfTheSameValueTranslateToEqualContent(t *testing.T) {
	plain := mustTranslatePackagingMaterial(t, packagingDocument(`"12.5"`, `"12"`, `"0.25"`, `"CM"`, `"1"`, `"KG"`)).Content
	rewritten := mustTranslatePackagingMaterial(t, packagingDocument(`"012.500"`, `"12.0"`, `"00.250000"`, `"CM"`, `"1.0"`, `"KG"`)).Content
	if plain != rewritten {
		t.Fatalf("同一组数的不同写法译成了不同的内容：%+v 与 %+v", plain, rewritten)
	}
	if different := mustTranslatePackagingMaterial(t, packagingDocument(`"12.5"`, `"12"`, `"0.25"`, `"M"`, `"1"`, `"KG"`)).Content; plain == different {
		t.Fatalf("不同的尺寸单位译成了同一份内容：登记时不折算单位")
	}
	if different := mustTranslatePackagingMaterial(t, packagingDocument(`"12.5"`, `"12"`, `"0.251"`, `"CM"`, `"1"`, `"KG"`)).Content; plain == different {
		t.Fatalf("不同的数译成了同一份内容")
	}
}

// Covers: AT-WF-004（包材的部分）数值写法的各种坏载荷——负号、正号、指数、千分位、只有小数点、小数点一边没有数字、超位数、空白，以及 JSON 数字（数值是 JSON
// 字符串，不是 JSON 数字）、null、别的 JSON 类型：四个数值格逐一试，各答 400，理由只点名那一格、不回显取值。
func TestMalformedPackagingMaterialNumbersAreRefusedPerCellAndNeverEchoed(t *testing.T) {
	cells := []struct {
		path  string
		build func(raw string) string
	}{
		{"dimensions.length", func(raw string) string { return packagingDocument(raw, `"1"`, `"1"`, `"CM"`, `"1"`, `"KG"`) }},
		{"dimensions.width", func(raw string) string { return packagingDocument(`"1"`, raw, `"1"`, `"CM"`, `"1"`, `"KG"`) }},
		{"dimensions.height", func(raw string) string { return packagingDocument(`"1"`, `"1"`, raw, `"CM"`, `"1"`, `"KG"`) }},
		{"weight.value", func(raw string) string { return packagingDocument(`"1"`, `"1"`, `"1"`, `"CM"`, raw, `"KG"`) }},
	}
	bad := map[string]string{
		"minus sign":                 `"-12"`,
		"plus sign":                  `"+12"`,
		"exponent":                   `"1e3"`,
		"capital exponent":           `"1E3"`,
		"thousands separator":        `"1,000"`,
		"only a decimal point":       `"."`,
		"nothing before the point":   `".5"`,
		"nothing after the point":    `"5."`,
		"two decimal points":         `"1.2.3"`,
		"ten integer digits":         `"1234567890"`,
		"seven fractional digits":    `"1.1234567"`,
		"trailing zero past six":     `"1.5000000"`,
		"leading space":              `" 12"`,
		"trailing space":             `"12 "`,
		"inner space":                `"1 2"`,
		"empty string":               `""`,
		"blank string":               `"   "`,
		"a unit glued on":            `"12cm"`,
		"JSON number":                `12`,
		"JSON fractional number":     `12.5`,
		"JSON number in an exponent": `1e3`,
		"JSON null":                  `null`,
		"JSON true":                  `true`,
		"JSON array":                 `["12"]`,
		"JSON object":                `{"v":"12"}`,
		"a word":                     `"abc"`,
		"hexadecimal":                `"0x10"`,
		"full-width digits":          `"１２"`,
		"newline":                    `"12\n"`,
	}
	for _, cell := range cells {
		for name, raw := range bad {
			t.Run(cell.path+"/"+name, func(t *testing.T) {
				_, err := translatePackagingMaterial(t, cell.build(raw))
				if !errors.Is(err, registrationjson.ErrMalformedDocument) {
					t.Fatalf("err = %v，want ErrMalformedDocument", err)
				}
				var malformed *registrationjson.MalformedDocumentError
				if !errors.As(err, &malformed) || malformed.Reason == "" {
					t.Fatalf("坏载荷要带一句理由：%v", err)
				}
				if !strings.Contains(malformed.Reason, cell.path) {
					t.Fatalf("理由 %q 没有点名 %s 这一格", malformed.Reason, cell.path)
				}
				for _, secret := range []string{"1234567890", "1.1234567", "1.5000000", "abc", "0x10", "１２"} {
					if strings.Contains(malformed.Reason, secret) {
						t.Fatalf("理由回显了报文里的取值 %q：%q", secret, malformed.Reason)
					}
				}
			})
		}
	}
}

// Covers: AT-WF-004——请求自带租户或登记责任方答坏载荷，键在场即拒，不看值：值与认证结果碰巧相同、null 都一样。
func TestACarriedTenantOrRegistrantIsRefusedOnAPackagingMaterialWhateverItsValue(t *testing.T) {
	rest := strings.TrimPrefix(packagingDocument(`"1"`, `"1"`, `"1"`, `"CM"`, `"1"`, `"KG"`), `{`)
	for name, body := range map[string]string{
		"tenant":                   `{"tenantId":"SYN-TENANT-02",` + rest,
		"tenant equal to the real": `{"tenantId":"SYN-TENANT-01",` + rest,
		"tenant null":              `{"tenantId":null,` + rest,
		"registrant":               `{"registrant":"SYN-OPERATOR-09",` + rest,
		"registrant null":          `{"registrant":null,` + rest,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translatePackagingMaterial(t, body)
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

// Covers: AT-WF-004——坏载荷各答 400 一格：JSON 不合法、认不得的键（含别的册的格、外廓与重量下多出来的键）、必填格缺失或类型不对、必填文本格是空白（含两个单位）。
func TestMalformedPackagingMaterialDocumentsAreRefused(t *testing.T) {
	const head = `"packagingMaterialCode":"B","revision":1,"name":"n",`
	const dimensions = `"dimensions":{"length":"1","width":"1","height":"1","unit":"CM"},`
	const weight = `"weight":{"value":"1","unit":"KG"}`
	cases := map[string]string{
		"empty body":            ``,
		"not JSON":              `not json`,
		"truncated":             `{` + head + dimensions,
		"array at the top":      `[]`,
		"trailing second value": fullPackagingMaterialDocument + ` {}`,
		"unknown key":           `{` + head + dimensions + weight + `,"colour":"brown"}`,
		"a zone cell":           `{` + head + dimensions + weight + `,"usage":"STORAGE"}`,
		"unknown dimension key": `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":"CM","depth":"1"},` + weight + `}`,
		"unknown weight key":    `{` + head + dimensions + `"weight":{"value":"1","unit":"KG","tare":"1"}}`,
		"flat dimension keys":   `{` + head + `"length":"1","width":"1","height":"1","dimensionsUnit":"CM",` + weight + `}`,

		"packagingMaterialCode missing": `{"revision":1,"name":"n",` + dimensions + weight + `}`,
		"revision missing":              `{"packagingMaterialCode":"B","name":"n",` + dimensions + weight + `}`,
		"name missing":                  `{"packagingMaterialCode":"B","revision":1,` + dimensions + weight + `}`,
		"dimensions missing":            `{` + head + weight + `}`,
		"weight missing":                `{` + head + strings.TrimSuffix(dimensions, `,`) + `}`,
		"length missing":                `{` + head + `"dimensions":{"width":"1","height":"1","unit":"CM"},` + weight + `}`,
		"width missing":                 `{` + head + `"dimensions":{"length":"1","height":"1","unit":"CM"},` + weight + `}`,
		"height missing":                `{` + head + `"dimensions":{"length":"1","width":"1","unit":"CM"},` + weight + `}`,
		"dimension unit missing":        `{` + head + `"dimensions":{"length":"1","width":"1","height":"1"},` + weight + `}`,
		"weight value missing":          `{` + head + dimensions + `"weight":{"unit":"KG"}}`,
		"weight unit missing":           `{` + head + dimensions + `"weight":{"value":"1"}}`,
		"dimensions null":               `{` + head + `"dimensions":null,` + weight + `}`,
		"weight null":                   `{` + head + dimensions + `"weight":null}`,
		"dimension unit null":           `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":null},` + weight + `}`,

		"packagingMaterialCode is number": `{"packagingMaterialCode":1,"revision":1,"name":"n",` + dimensions + weight + `}`,
		"revision is string":              `{"packagingMaterialCode":"B","revision":"1","name":"n",` + dimensions + weight + `}`,
		"revision is fractional":          `{"packagingMaterialCode":"B","revision":1.5,"name":"n",` + dimensions + weight + `}`,
		"name is object":                  `{"packagingMaterialCode":"B","revision":1,"name":{},` + dimensions + weight + `}`,
		"dimensions is string":            `{` + head + `"dimensions":"1x1x1",` + weight + `}`,
		"weight is number":                `{` + head + dimensions + `"weight":1}`,
		"dimension unit is number":        `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":1},` + weight + `}`,
		"weight unit is object":           `{` + head + dimensions + `"weight":{"value":"1","unit":{}}}`,
		"deactivated is string":           `{` + head + dimensions + weight + `,"deactivated":"false"}`,

		"packagingMaterialCode blank": `{"packagingMaterialCode":"   ","revision":1,"name":"n",` + dimensions + weight + `}`,
		"packagingMaterialCode empty": `{"packagingMaterialCode":"","revision":1,"name":"n",` + dimensions + weight + `}`,
		"name blank":                  `{"packagingMaterialCode":"B","revision":1,"name":"\n",` + dimensions + weight + `}`,
		"name empty":                  `{"packagingMaterialCode":"B","revision":1,"name":"",` + dimensions + weight + `}`,
		"dimension unit blank":        `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":"  "},` + weight + `}`,
		"dimension unit empty":        `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":""},` + weight + `}`,
		"weight unit blank":           `{` + head + dimensions + `"weight":{"value":"1","unit":"\t"}}`,
		"weight unit empty":           `{` + head + dimensions + `"weight":{"value":"1","unit":""}}`,

		"NUL in the code":           `{"packagingMaterialCode":"B\u0000","revision":1,"name":"n",` + dimensions + weight + `}`,
		"NUL in the name":           `{"packagingMaterialCode":"B","revision":1,"name":"n\u0000",` + dimensions + weight + `}`,
		"NUL in the dimension unit": `{` + head + `"dimensions":{"length":"1","width":"1","height":"1","unit":"C\u0000M"},` + weight + `}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := translatePackagingMaterial(t, body)
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

// 未知键的理由点名这是一笔包材修订，不借用别的册的说法；类型错的格点名键路径。
func TestThePackagingMaterialReasonNamesTheKindOfRevisionAndTheCellPath(t *testing.T) {
	_, err := translatePackagingMaterial(t, `{"packagingMaterialCode":"B","revision":1,"name":"n","colour":"brown"}`)
	var malformed *registrationjson.MalformedDocumentError
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "packaging material revision") {
		t.Errorf("未知键的理由 = %v，want 点名这是一笔包材修订", err)
	}

	_, err = translatePackagingMaterial(t, packagingDocument(`30`, `"1"`, `"1"`, `"CM"`, `"1"`, `"KG"`))
	if !errors.As(err, &malformed) || !strings.Contains(malformed.Reason, "dimensions.length") || !strings.Contains(malformed.Reason, "string") {
		t.Errorf("JSON 数字的理由 = %v，want 点名 dimensions.length 该是字符串", err)
	}
}

// Covers: UC-WF-001「权限、隔离与审计」——错误正文不回显报文里的取值：包材编码、名称、单位与数值一个字也不回显。
func TestThePackagingMaterialReasonNeverEchoesTheCodeTheNameOrTheValues(t *testing.T) {
	const secretCode, secretName, secretValue, secretUnit = "SYN-SECRET-BOX", "SYN-SECRET-NAME", "9.87654321", "SYN-SECRET-UNIT"
	prefix := `{"packagingMaterialCode":"` + secretCode + `","revision":1,"name":"` + secretName + `",`
	for name, body := range map[string]string{
		"too many digits": prefix + `"dimensions":{"length":"` + secretValue + `","width":"1","height":"1","unit":"CM"},"weight":{"value":"1","unit":"KG"}}`,
		"revision is string": strings.Replace(prefix, `"revision":1`, `"revision":"1"`, 1) +
			`"dimensions":{"length":"1","width":"1","height":"1","unit":"CM"},"weight":{"value":"1","unit":"KG"}}`,
		"tenant carried": `{"tenantId":"x","packagingMaterialCode":"` + secretCode + `","revision":1,"name":"` + secretName + `",` +
			`"dimensions":{"length":"1","width":"1","height":"1","unit":"CM"},"weight":{"value":"1","unit":"KG"}}`,
		"unknown key":        prefix + `"dimensions":{"length":"1","width":"1","height":"1","unit":"CM"},"weight":{"value":"1","unit":"KG"},"x":1}`,
		"blank unit":         prefix + `"dimensions":{"length":"1","width":"1","height":"1","unit":"  "},"weight":{"value":"1","unit":"` + secretUnit + `"}}`,
		"weight is a string": prefix + `"dimensions":{"length":"1","width":"1","height":"1","unit":"` + secretUnit + `"},"weight":"` + secretValue + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := translatePackagingMaterial(t, body)
			if err == nil {
				t.Fatal("want 坏载荷")
			}
			for _, secret := range []string{secretCode, secretName, secretValue, secretUnit} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("错误正文回显了报文里的取值 %q：%q", secret, err.Error())
				}
			}
		})
	}
}

// Covers: AT-WF-004——请求体读不出（连接断了、请求体检查判到存不下的字节）也是坏载荷，理由不外泄读错的原文。
func TestAPackagingMaterialBodyThatCannotBeReadIsMalformed(t *testing.T) {
	_, err := registrationjson.PackagingMaterialCommand(brokenBody{},
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewRegistrant, "SYN-OPERATOR-01"))
	if !errors.Is(err, registrationjson.ErrMalformedDocument) {
		t.Fatalf("err = %v，want ErrMalformedDocument", err)
	}
	if strings.Contains(err.Error(), "SYN-SECRET") {
		t.Fatalf("错误正文回显了读错的原文：%q", err.Error())
	}
}
