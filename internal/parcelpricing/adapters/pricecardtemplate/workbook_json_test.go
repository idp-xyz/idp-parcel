package pricecardtemplate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// readingKey 把一份读法折成可逐字比较的串：版本回显、是否受理、方案身份、摘要、规范化版本、登记快照里的 plan 块、
// 以及问题清单的每一条（表、行、列、码、话）。
func readingKey(t *testing.T, reading ports.PriceCardTemplateReading) string {
	t.Helper()
	var key strings.Builder
	fmt.Fprintf(&key, "version=%s accepted=%v plan=%s/%s\n", reading.TemplateVersion, reading.Accepted, reading.Plan.ID(), reading.Plan.Version())
	if reading.Content != nil {
		plan := reading.Content.Plan
		fmt.Fprintf(&key, "canonicalization=%s digest=%s\n", plan.CanonicalizationVersion(), plan.ContentDigest())
		fmt.Fprintf(&key, "plan=%v\n", registrationPlanBlock(t, plan, reading.Content.DirectionAuthorization))
	}
	for _, problem := range reading.Problems {
		fmt.Fprintf(&key, "%s!%d:%s %s %s\n", problem.Sheet, problem.Row, problem.Column, problem.Code, problem.Message)
	}
	return key.String()
}

func readWorkbookJSON(t *testing.T, raw []byte) ports.PriceCardTemplateReading {
	t.Helper()
	canonical, err := templateReader.CanonicalizePriceCardWorkbook(raw)
	if err != nil {
		t.Fatalf("规范化：%v", err)
	}
	return templateReader.ReadPriceCardWorkbook(canonical)
}

// Covers: `AT-PP-046`，ADR-0349 决定四——同一份内容写成电子表格与写成结构化工作簿，读出同一份读法：方案、摘要、规范化版本、
// 问题清单的每一条（含表、行、列坐标与说话）逐字相同。覆盖已校验的几张卡，也覆盖各类带问题的写法——判定只有一条路径，
// 问题坐标对 JSON 同样给出（数组下标加 2 即行号）。
func TestWorkbookJSONReadsExactlyWhatTheSpreadsheetReads(t *testing.T) {
	type mutation func(*filled)
	cases := map[string]struct {
		base   func() filled
		mutate mutation
	}{
		"成本卡":      {costCard, nil},
		"销售卡":      {sellCard, nil},
		"全结构卡":     {func() filled { return fullCard().clone() }, nil},
		"完整卡三":     {full03Card, nil},
		"未知代码":     {costCard, func(c *filled) { c.card["direction"] = "BUYY" }},
		"时刻不带时区":   {costCard, func(c *filled) { c.card["periodStartsAt"] = "2026-01-01" }},
		"科学计数法":    {costCard, func(c *filled) { c.data[sheetRatesWeightZone][0]["minimum"] = "1e3" }},
		"必填格为空":    {costCard, func(c *filled) { delete(c.data[sheetRatesWeightZone][1], "zone") }},
		"主价表指空":    {costCard, func(c *filled) { c.card["rateTableId"] = "SYN-NO-SUCH-TABLE" }},
		"区间重叠":     {costCard, func(c *filled) { c.data[sheetRatesWeightZone][1]["minimum"] = "0.5" }},
		"方向与目的不配":  {costCard, func(c *filled) { c.card["purpose"] = "CUSTOMER_CHARGE" }},
		"币种不在目录":   {costCard, func(c *filled) { c.data[sheetTables][0]["currency"] = "XXX" }},
		"三格缺一":     {costCard, func(c *filled) { delete(c.card, "amountRoundingIncrement") }},
		"方案身份空":    {costCard, func(c *filled) { c.card["planId"] = "" }},
		"计重方法不认":   {costCard, func(c *filled) { c.card["weightMethod"] = "MIN" }},
		"指到不存在的节点": {func() filled { return fullCard().clone() }, func(c *filled) { c.data[sheetSurcharges][1]["conditionId"] = "C-NOWHERE" }},
		"成环":       {func() filled { return fullCard().clone() }, func(c *filled) { c.data[sheetConditions][0]["parentId"] = "C-OVERSIZE-LEN" }},
		"取较大值缺子节点": {func() filled { return fullCard().clone() }, func(c *filled) { c.data[sheetCalculations][1]["parentId"] = "" }},
		"计算类型不认":   {func() filled { return fullCard().clone() }, func(c *filled) { c.data[sheetCalculations][3]["method"] = "TABLE" }},
		"按档位查表缺目录": {full03Card, func(c *filled) {
			c.data[sheetReferenceCatalogues] = rows{{"kind": "ZONE", "catalogueId": "SYN-CAT-ZONE-CN-SG-FULL"}}
		}},
		"总额下限不在整倍数":  {full03Card, func(c *filled) { c.data[sheetTotalBounds] = rows{{"minimum": "120.995"}} }},
		"总额上下限多一行":   {full03Card, func(c *filled) { c.data[sheetTotalBounds] = rows{{"minimum": "1"}, {"maximum": "2"}} }},
		"PPT-1 却带新表": {full03Card, func(c *filled) { c.card["templateVersion"] = templateVersionV1 }},
		"模板版本不认":     {costCard, func(c *filled) { c.card["templateVersion"] = "PPT-0" }},
	}
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	problemCases := 0
	for _, name := range names {
		testCase := cases[name]
		card := testCase.base()
		if testCase.mutate != nil {
			testCase.mutate(&card)
		}
		fromFile := readingKey(t, read(t, card.bytes(t)))
		fromForm := readingKey(t, readWorkbookJSON(t, card.workbookJSON(t)))
		if fromFile != fromForm {
			t.Errorf("%s: 两个载体读出不同\n文件：\n%s\n表单：\n%s", name, fromFile, fromForm)
		}
		if strings.Contains(fromFile, "!") {
			problemCases++
		}
	}
	if problemCases < 15 {
		t.Fatalf("带问题的用例只有 %d 个，对拍太薄", problemCases)
	}
}

// 规范化：空白、键序与数字写成数字还是字符串都不改变身份；内容改一个字就变。
func TestCanonicalWorkbookIdentityIgnoresWhitespaceKeyOrderAndNumberQuoting(t *testing.T) {
	spaced := []byte(`{ "templateVersion": "PPT-2",
	  "sheets": { "tables": [ { "tableId": "T", "family": "WEIGHT_ZONE", "currency": "CNY" } ],
	              "total_bounds": [ { "minimum": 12.50, "maximum": null } ] } }`)
	tight := []byte(`{"sheets":{"tables":[{"currency":"CNY","family":"WEIGHT_ZONE","tableId":"T"}],"total_bounds":[{"maximum":null,"minimum":"12.50"}]},"templateVersion":"PPT-2"}`)
	first, err := templateReader.CanonicalizePriceCardWorkbook(spaced)
	if err != nil {
		t.Fatal(err)
	}
	second, err := templateReader.CanonicalizePriceCardWorkbook(tight)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || !bytes.Equal(first, tight) {
		t.Fatalf("规范化字节不同：\n%s\n%s", first, second)
	}
	again, err := templateReader.CanonicalizePriceCardWorkbook(first)
	if err != nil || !bytes.Equal(again, first) {
		t.Fatalf("规范化不幂等：%s %v", again, err)
	}
	changed, err := templateReader.CanonicalizePriceCardWorkbook(bytes.Replace(spaced, []byte("12.50"), []byte("12.51"), 1))
	if err != nil || bytes.Equal(changed, first) {
		t.Fatal("内容改了身份却没变")
	}
	// 数值字面量原样保留，不经浮点：尾部的 0 与 17 位有效数字都不动。
	exact, err := templateReader.CanonicalizePriceCardWorkbook([]byte(`{"templateVersion":"PPT-2","sheets":{"x":[{"a":0.1234567890123456789,"b":1.0}]}}`))
	if err != nil || !strings.Contains(string(exact), `"a":"0.1234567890123456789"`) || !strings.Contains(string(exact), `"b":"1.0"`) {
		t.Fatalf("数值字面量被改写：%s %v", exact, err)
	}
	sum := sha256.Sum256(first)
	if len(hex.EncodeToString(sum[:])) != 64 {
		t.Fatal("摘要长度")
	}
}

// 形状不对的整份不收：答 PriceCardWorkbookRejection，话里说清哪里不对；读法侧同样整份不收，不落草稿。
func TestMalformedWorkbookJSONIsRefusedWithAReason(t *testing.T) {
	cases := map[string]struct{ raw, mention string }{
		"不是 JSON":    {`{"templateVersion":`, "提前结束"},
		"括号不配":       {`{"templateVersion":"PPT-2","sheets":{"tables":[}}}`, "读不出 JSON"},
		"空":          {``, "是空的"},
		"顶层是数组":      {`[]`, "顶层必须是对象"},
		"顶层多键":       {`{"templateVersion":"PPT-2","sheets":{},"tenant":"X"}`, `多出了 "tenant"`},
		"缺版本":        {`{"sheets":{}}`, "缺 templateVersion"},
		"版本不是字符串":    {`{"templateVersion":2,"sheets":{}}`, "缺 templateVersion"},
		"缺表集合":       {`{"templateVersion":"PPT-2"}`, "缺 sheets"},
		"表不是数组":      {`{"templateVersion":"PPT-2","sheets":{"tables":{}}}`, "表 tables 必须是行数组"},
		"行不是对象":      {`{"templateVersion":"PPT-2","sheets":{"tables":["x"]}}`, "第 2 行（数组下标 0）必须是对象"},
		"格是嵌套对象":     {`{"templateVersion":"PPT-2","sheets":{"tables":[{"tableId":{"a":1}}]}}`, "tables 第 2 行的 tableId 只能是字符串、数字或 null"},
		"格是数组":       {`{"templateVersion":"PPT-2","sheets":{"tables":[{"tableId":["a"]}]}}`, "只能是字符串、数字或 null"},
		"同一行同名键出现两次": {`{"templateVersion":"PPT-2","sheets":{"tables":[{"tableId":"A","tableId":"B"}]}}`, `"tableId" 出现了不止一次`},
		"同名表出现两次":    {`{"templateVersion":"PPT-2","sheets":{"tables":[],"tables":[]}}`, `"tables" 出现了不止一次`},
		"尾随内容":       {`{"templateVersion":"PPT-2","sheets":{}} {}`, "多余的内容"},
		"嵌套过深":       {`{"templateVersion":"PPT-2","sheets":{"a":[{"b":[[[[[[[[[1]]]]]]]]]}]}}`, "嵌套过深"},
	}
	for name, testCase := range cases {
		_, err := templateReader.CanonicalizePriceCardWorkbook([]byte(testCase.raw))
		var rejection ports.PriceCardWorkbookRejection
		if !errors.Is(err, ports.ErrPriceCardWorkbookMalformed) || !errors.As(err, &rejection) || !strings.Contains(rejection.Detail, testCase.mention) {
			t.Errorf("%s: err = %v, 应点名「%s」", name, err, testCase.mention)
		}
		reading := templateReader.ReadPriceCardWorkbook([]byte(testCase.raw))
		if reading.Accepted || reading.Content != nil || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateFileNotWorkbook {
			t.Errorf("%s: 读法 accepted=%v problems=%s", name, reading.Accepted, describe(reading.Problems))
		}
	}

	huge := `{"templateVersion":"PPT-2","sheets":{"x":[` + strings.Repeat(`{"a":"b"},`, 1) + `{}]},"pad":"` + strings.Repeat("x", MaxFileBytes) + `"}`
	if _, err := templateReader.CanonicalizePriceCardWorkbook([]byte(huge)); !errors.Is(err, ports.ErrPriceCardWorkbookTooLarge) {
		t.Errorf("超过文件上限：err = %v", err)
	}
	if reading := templateReader.ReadPriceCardWorkbook([]byte(huge)); reading.Accepted || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateFileTooLarge {
		t.Errorf("超限的读法：%s", describe(reading.Problems))
	}
	manyRows := `{"templateVersion":"PPT-2","sheets":{"x":[` + strings.Repeat("{},", maxRowsPerSheet) + `{}]}}`
	if _, err := templateReader.CanonicalizePriceCardWorkbook([]byte(manyRows)); !errors.Is(err, ports.ErrPriceCardWorkbookTooLarge) {
		t.Errorf("超过单表行数上限：err = %v", err)
	}
}

// 载体特有的几条约定（译成内部表示这一步里落定，之后与读文件没有区别）。
func TestWorkbookJSONCarrierConventions(t *testing.T) {
	card := costCard()
	valid := readingKey(t, readWorkbookJSON(t, card.workbookJSON(t)))
	if !strings.Contains(valid, "digest=") {
		t.Fatalf("成本卡的表单工作簿应已校验：%s", valid)
	}

	t.Run("没出现的表按空表算，不报缺表", func(t *testing.T) {
		reading := readWorkbookJSON(t, card.workbookJSON(t))
		for _, problem := range reading.Problems {
			if problem.Code == ports.TemplateSheetMissing {
				t.Fatalf("不该报缺表：%s", describe(reading.Problems))
			}
		}
	})

	t.Run("不在模板里的表与列照样按未知报，带坐标", func(t *testing.T) {
		raw := []byte(`{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"},{"field":"planVersion","value":"v1"}],"notes":[{"a":"b"}],"tables":[{"tableId":"T","nickname":"x"}]}}`)
		reading := readWorkbookJSON(t, raw)
		wants := []at{
			{"notes", 0, "", ports.TemplateSheetUnknown},
			{sheetTables, 1, "nickname", ports.TemplateColumnUnknown},
		}
		for _, want := range wants {
			if !hasProblem(reading.Problems, want) {
				t.Errorf("缺 %+v：%s", want, describe(reading.Problems))
			}
		}
	})

	t.Run("问题的行号是数组下标加二", func(t *testing.T) {
		bad := costCard()
		bad.data[sheetRatesWeightZone][2]["amount"] = "x"
		reading := readWorkbookJSON(t, bad.workbookJSON(t))
		if !hasProblem(reading.Problems, at{sheetRatesWeightZone, 4, "amount", ports.TemplateCellInvalid}) {
			t.Fatalf("problems = %s", describe(reading.Problems))
		}
	})

	t.Run("布尔格按格类型不对报，数字格取书写的字面量", func(t *testing.T) {
		raw := []byte(`{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"},{"field":"planVersion","value":1.50},{"field":"scope","value":true}]}}`)
		reading := readWorkbookJSON(t, raw)
		if !hasProblem(reading.Problems, at{sheetCard, 4, "scope", ports.TemplateCellNotText}) {
			t.Fatalf("布尔格：%s", describe(reading.Problems))
		}
		if reading.Plan.Version() != "1.50" {
			t.Fatalf("数字格应取书写的字面量 1.50，读成 %q", reading.Plan.Version())
		}
	})

	t.Run("外层版本补进 card 表；两处不一致不收", func(t *testing.T) {
		without := full03Card()
		without.card["templateVersion"] = ""
		reading := readWorkbookJSON(t, without.workbookJSON(t))
		if !reading.Accepted || reading.TemplateVersion != TemplateVersion {
			t.Fatalf("card 表没写版本，外层的版本应补进去：%s", describe(reading.Problems))
		}

		onlyEnvelope := []byte(`{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"},{"field":"planVersion","value":"v1"}]}}`)
		if reading := readWorkbookJSON(t, onlyEnvelope); !reading.Accepted || reading.TemplateVersion != TemplateVersion {
			t.Fatalf("card 表里压根没有版本这一行：%s", describe(reading.Problems))
		}

		raw := []byte(`{"templateVersion":"PPT-2","sheets":{"card":[{"field":"templateVersion","value":"PPT-1"},{"field":"planId","value":"P"},{"field":"planVersion","value":"v1"}]}}`)
		reading = readWorkbookJSON(t, raw)
		if reading.Accepted || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateVersionUnsupported ||
			!strings.Contains(reading.Problems[0].Message, "两处须一致") {
			t.Fatalf("外层与 card 表的版本不一致：%s", describe(reading.Problems))
		}
	})

	t.Run("PPT-1 的表单工作簿：不补 total_bounds，读出与 PPT-1 文件相同的方案", func(t *testing.T) {
		legacy := sellCard()
		legacy.card["templateVersion"] = templateVersionV1
		fromForm := readWorkbookJSON(t, legacy.workbookJSON(t))
		fromFile := read(t, legacy.asPPT1(t))
		if readingKey(t, fromForm) != readingKey(t, fromFile) || fromForm.TemplateVersion != templateVersionV1 || !strings.Contains(readingKey(t, fromForm), "canonicalization=PPC-5") {
			t.Fatalf("表单 PPT-1：\n%s\n文件 PPT-1：\n%s", readingKey(t, fromForm), readingKey(t, fromFile))
		}
	})

	t.Run("不认的外层版本整份不收", func(t *testing.T) {
		raw := []byte(`{"templateVersion":"PPT-9","sheets":{"card":[{"field":"planId","value":"P"}]}}`)
		reading := readWorkbookJSON(t, raw)
		if reading.Accepted || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateVersionUnsupported {
			t.Fatalf("problems = %s", describe(reading.Problems))
		}
	})
}
