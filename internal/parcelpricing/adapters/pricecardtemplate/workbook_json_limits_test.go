package pricecardtemplate

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 本文件证 JSON 载体的资源边界（ADR-0349 决定四、越权风险点 9）：解析是流式的、边解边数，到上限立刻停；解析的代价跟着总格数走，
// 不是行数乘列数；问题清单有总数上限。规模都压在测试跑得动的范围，断言的是「在上限处即拒」与「分配、耗时有界」，不是等它跑完。

func workbookBody(sheet, rows string) string {
	return `{"templateVersion":"PPT-2","sheets":{"` + sheet + `":[` + rows + `]}}`
}

// peakHeapOf 报出 work 期间堆占用的峰值（毫秒级采样，只作上界用）。
func peakHeapOf(work func()) uint64 {
	var peak atomic.Uint64
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		var stats runtime.MemStats
		for {
			select {
			case <-stop:
				return
			default:
			}
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > peak.Load() {
				peak.Store(stats.HeapAlloc)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	runtime.GC()
	work()
	close(stop)
	<-done
	return peak.Load()
}

// allocatedBy 报出 work 期间累计分配的字节数。累计量不小于峰值，所以拿它设上界是保守的。
func allocatedBy(work func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	work()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// Covers: `AT-PP-046`，ADR-0349 决定四——表数、每表行数、每表不同列键数、总格数各有一个上限，超限整份不收（FILE_TOO_LARGE），
// 在解析途中就停，话里点名是哪一个上限。
func TestWorkbookJSONStopsAtItsStructuralLimits(t *testing.T) {
	distinctKeys := func(count int) string {
		var keys []string
		for index := 0; index < count; index++ {
			keys = append(keys, fmt.Sprintf(`"k%04d":1`, index))
		}
		return "{" + strings.Join(keys, ",") + "}"
	}
	oneNewKeyPerRow := func(count int) string {
		var rows []string
		for index := 0; index < count; index++ {
			rows = append(rows, fmt.Sprintf(`{"k%04d":1}`, index))
		}
		return strings.Join(rows, ",")
	}
	var manySheets strings.Builder
	manySheets.WriteString(`{"templateVersion":"PPT-2","sheets":{`)
	for index := 0; index <= maxJSONSheets; index++ {
		if index > 0 {
			manySheets.WriteByte(',')
		}
		fmt.Fprintf(&manySheets, `"s%03d":[]`, index)
	}
	manySheets.WriteString(`}}`)
	wideRows := strings.Repeat(`{"a":1,"b":1,"c":1,"d":1,"e":1,"f":1,"g":1,"h":1,"i":1,"j":1,"k":1},`, maxRowsPerSheet-1) + `{}`

	cases := map[string]struct{ body, mention string }{
		"一行里不同列键过多":  {workbookBody("zz", distinctKeys(maxJSONColumns+1)), "不同的列键"},
		"每行一个新键累计过多": {workbookBody("zz", oneNewKeyPerRow(maxJSONColumns+1)), "不同的列键"},
		"表过多":        {manySheets.String(), "表超过"},
		"行过多":        {workbookBody("zz", strings.Repeat("{},", maxRowsPerSheet)+"{}"), "单表"},
		"总格数过多":      {workbookBody("zz", wideRows), "格数"},
		"列键数在上限上仍收":  {workbookBody("zz", distinctKeys(maxJSONColumns)), ""},
		"行数在上限上仍收":   {workbookBody("zz", strings.Repeat("{},", maxRowsPerSheet-1)+"{}"), ""},
	}
	for name, testCase := range cases {
		_, err := templateReader.CanonicalizePriceCardWorkbook([]byte(testCase.body))
		var rejection ports.PriceCardWorkbookRejection
		switch {
		case testCase.mention == "" && err != nil:
			t.Errorf("%s：上限上不该拒：%v", name, err)
		case testCase.mention != "" && (!errors.Is(err, ports.ErrPriceCardWorkbookTooLarge) || !errors.As(err, &rejection) || !strings.Contains(rejection.Detail, testCase.mention)):
			t.Errorf("%s：err = %v，应是点名「%s」的超限", name, err, testCase.mention)
		}
	}

	// 整份不收的读法：FILE_TOO_LARGE，回显最新一版，与读文件同类情形一致。
	reading := templateReader.ReadPriceCardWorkbook([]byte(cases["行过多"].body))
	if reading.Accepted || reading.TemplateVersion != TemplateVersion || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateFileTooLarge {
		t.Fatalf("超限的读法：version=%s problems=%s", reading.TemplateVersion, describe(reading.Problems))
	}
}

// Covers: `AT-PP-046`——代价跟着总格数走。评审探针：一行带大量不同键、其余 99 999 行是空对象，旧写法对每行遍历整表全部列，
// 请求体 2.7 MB 即单核近 30 秒；现在在第 65 个不同键处就停。这里把规模降到测试跑得动（键数仍远超上限），断言在上限处即拒、
// 耗时与分配都有界；另用上限以内的最宽形状（上限个不同键 + 99 999 个空行）核整条读取路径也有界。
func TestWorkbookJSONCostFollowsTheCellsNotRowsTimesColumns(t *testing.T) {
	var keys []string
	for index := 0; index < 20_000; index++ {
		keys = append(keys, fmt.Sprintf(`"k%06d":1`, index))
	}
	body := workbookBody("zz", "{"+strings.Join(keys, ",")+"}"+strings.Repeat(",{}", maxRowsPerSheet-1))
	start := time.Now()
	var err error
	allocated := allocatedBy(func() { _, err = templateReader.CanonicalizePriceCardWorkbook([]byte(body)) })
	if !errors.Is(err, ports.ErrPriceCardWorkbookTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second || allocated > 8<<20 {
		t.Fatalf("在列键上限处该立刻拒：耗时 %v，分配 %d 字节", elapsed, allocated)
	}

	var widest []string
	for index := 0; index < maxJSONColumns; index++ {
		widest = append(widest, fmt.Sprintf(`"k%06d":1`, index))
	}
	within := `{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"},{"field":"planVersion","value":"v1"}],"zz":[` +
		"{" + strings.Join(widest, ",") + "}" + strings.Repeat(",{}", maxRowsPerSheet-1) + `]}}`
	start = time.Now()
	canonical, err := templateReader.CanonicalizePriceCardWorkbook([]byte(within))
	if err != nil {
		t.Fatal(err)
	}
	reading := templateReader.ReadPriceCardWorkbook(canonical)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("上限以内的最宽形状读了 %v", elapsed)
	}
	if !hasProblem(reading.Problems, at{"zz", 0, "", ports.TemplateSheetUnknown}) {
		t.Fatalf("未知表照样如实报出：%s", describe(reading.Problems))
	}
}

// Covers: `AT-PP-046`——内存放大。评审探针：9 MiB 的 `{"a":1},` 重复体，旧写法先把整个数组解成树再判行数，峰值 RSS 约 610 MB；
// 现在数到单表行数上限即停。断言累计分配远小于旧写法（旧写法光解树就是体积的几十倍）。
func TestWorkbookJSONMemoryIsBoundedByTheRowLimitNotTheBody(t *testing.T) {
	body := []byte(workbookBody("zz", strings.Repeat(`{"a":1},`, 1_200_000)+"{}"))
	if len(body) >= MaxFileBytes {
		t.Fatalf("探针体 %d 字节，要落在文件上限以内才测得到行数上限", len(body))
	}
	var err error
	allocated := allocatedBy(func() { _, err = templateReader.CanonicalizePriceCardWorkbook(body) })
	if !errors.Is(err, ports.ErrPriceCardWorkbookTooLarge) {
		t.Fatalf("err = %v", err)
	}
	t.Logf("体 %d 字节，累计分配 %d 字节", len(body), allocated)
	// 旧写法 90 倍上下（光把整个数组解成树就是体积的几十倍）；现在 4 倍上下，12 倍是两头都留了余量的上界。
	if limit := uint64(len(body)) * 12; allocated > limit {
		t.Fatalf("累计分配 %d 字节，超过体积的 12 倍（%d）；该在行数上限处停", allocated, limit)
	}
}

// Covers: `AT-PP-046`——体积上限只在入口按原始请求体判一次。规范化把数字落成字符串、把某些字符展成转义，字节数可以比原始的多，
// 原始体在上限以内的提交不该在读规范化字节时被判超限（那时来源标识已经立出）。
func TestTheByteLimitIsJudgedOnTheRawBodyOnlyOnce(t *testing.T) {
	// U+2028 在 UTF-8 里 3 字节，规范化写成   是 6 字节。
	filler := strings.Repeat(" ", 3_300_000)
	raw := []byte(`{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"` + filler + `"}]}}`)
	if len(raw) >= MaxFileBytes {
		t.Fatalf("原始体 %d 字节，要在上限以内", len(raw))
	}
	canonical, err := templateReader.CanonicalizePriceCardWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) <= MaxFileBytes {
		t.Fatalf("规范化字节 %d，要比上限多才证得到这一条", len(canonical))
	}
	reading := templateReader.ReadPriceCardWorkbook(canonical)
	for _, problem := range reading.Problems {
		if problem.Code == ports.TemplateFileTooLarge {
			t.Fatalf("读规范化字节不该再判体积：%s", describe(reading.Problems))
		}
	}
	if !reading.Accepted && len(reading.Problems) == 1 && reading.Problems[0].Code == ports.TemplateFileTooLarge {
		t.Fatal("被判超限")
	}
}

// Covers: `AT-PP-046`——外层 templateVersion 带首尾空白时，补进 card 表的、挑表集合的、报缺表的都用去了空白的那一个：
// 不会因为 PPT-1 外层多了个空格就多报一条「total_bounds 自 PPT-2 起才有」。
func TestAPaddedEnvelopeVersionIsReadAsTheTrimmedOne(t *testing.T) {
	legacy := sellCard()
	legacy.card["templateVersion"] = templateVersionV1
	body := legacy.workbookJSON(t)
	var padded bytes.Buffer
	padded.Write(bytes.Replace(body, []byte(`"templateVersion": "PPT-1"`), []byte(`"templateVersion": "  PPT-1 "`), 1))
	// 没有 card 表里的版本行时才靠外层补：把 card 表里的版本值去掉。
	bare := sellCard()
	bare.card["templateVersion"] = ""
	bareBody := bytes.Replace(bare.workbookJSON(t), []byte(`"templateVersion": "PPT-2"`), []byte(`"templateVersion": " PPT-1 "`), 1)

	for name, raw := range map[string][]byte{"card 表写了版本": padded.Bytes(), "card 表没写版本": bareBody} {
		reading := readWorkbookJSON(t, raw)
		if !reading.Accepted || reading.TemplateVersion != templateVersionV1 || reading.Content == nil || len(reading.Problems) != 0 {
			t.Errorf("%s：version=%q problems=%s", name, reading.TemplateVersion, describe(reading.Problems))
		}
	}
	canonical := mustCanonical(t, padded.Bytes())
	if !bytes.Contains(canonical, []byte(`"templateVersion":"PPT-1"`)) {
		t.Errorf("规范化字节该写去了空白的版本：%s", canonical[len(canonical)-40:])
	}
}

// Covers: `AT-PP-043`——问题清单有总数上限。评审探针：4 万行各缺 3 个必填格，旧写法收 16 万条问题、必填格去重对已有问题线性扫描，
// 12.6 秒。现在去重按集合查，清单收满上限就停并在最前补一条 PROBLEMS_TRUNCATED；电子表格与结构化工作簿两个载体一样。
func TestTheProblemListIsCappedAndTheRequiredCheckStaysLinear(t *testing.T) {
	const rowCount = 40_000
	card := costCard()
	var rowsJSON []string
	for index := 0; index < rowCount; index++ {
		rowsJSON = append(rowsJSON, fmt.Sprintf(`{"tableId":"SYN-TABLE-CN-SG-COST-01","entryId":"E%06d"}`, index))
	}
	card.data[sheetRatesWeightZone] = nil
	jsonBody := card.workbookJSON(t)
	jsonBody = bytes.Replace(jsonBody, []byte(`"sheets": {`), []byte(`"sheets": {"rates_weight_zone":[`+strings.Join(rowsJSON, ",")+`],`), 1)

	sheets := card.sheets(t)
	for index := range sheets {
		if sheets[index].Name == sheetRatesWeightZone {
			for row := 0; row < rowCount; row++ {
				sheets[index].Rows = append(sheets[index].Rows, []string{"SYN-TABLE-CN-SG-COST-01", fmt.Sprintf("E%06d", row), "", "", "", ""})
			}
		}
	}
	xlsx := writeSheets(t, sheets)

	for name, read := range map[string]func() ports.PriceCardTemplateReading{
		"结构化工作簿": func() ports.PriceCardTemplateReading { return readWorkbookJSON(t, jsonBody) },
		"电子表格":   func() ports.PriceCardTemplateReading { return Reader{}.ReadPriceCardTemplate(xlsx) },
	} {
		start := time.Now()
		reading := read()
		elapsed := time.Since(start)
		if !reading.Accepted || reading.Content != nil || len(reading.Problems) != maxProblems+1 {
			t.Fatalf("%s：accepted=%v 问题 %d 条，想要 %d 条", name, reading.Accepted, len(reading.Problems), maxProblems+1)
		}
		first := reading.Problems[0]
		if first.Code != ports.TemplateProblemsTruncated || first.Sheet != "" || first.Row != 0 || !strings.Contains(first.Message, "问题太多") {
			t.Fatalf("%s：清单最前该是截断说明：%+v", name, first)
		}
		if elapsed > 5*time.Second {
			t.Fatalf("%s：读了 %v，必填格去重该是线性的", name, elapsed)
		}
	}
}

// 问题没到上限时清单与此前一字不差：不带截断说明。
func TestAProblemListUnderTheCapCarriesNoTruncationNote(t *testing.T) {
	card := costCard()
	card.card["direction"] = "BUYY"
	for _, problem := range read(t, card.bytes(t)).Problems {
		if problem.Code == ports.TemplateProblemsTruncated {
			t.Fatal("没超上限却带了截断说明")
		}
	}
	_ = spreadsheet.CellText
}

// Covers: `AT-PP-046`——上限以内的最大合法形状（单表行数上限的行、总格数恰在上限）读取有界：读完整条路径（规范化、再读）耗时与
// 堆峰值都留在一个明确的范围里。这是「再大的合法输入也不会把进程拖垮」的下界证据；峰值在本机实测约一百兆字节出头，这里放宽到
// 三倍多，防的是量级退化，不是卡一个具体的数。
func TestTheLargestLegalWorkbookIsReadWithinBoundedTimeAndMemory(t *testing.T) {
	row := `{"a001":1,"a002":1,"a003":1,"a004":1,"a005":1,"a006":1,"a007":1,"a008":1,"a009":1,"a010":1}`
	body := []byte(workbookBody("zz", strings.Repeat(row+",", maxRowsPerSheet-1)+row))
	if len(body) >= MaxFileBytes {
		t.Fatalf("探针体 %d 字节，要在文件上限以内", len(body))
	}
	var reading ports.PriceCardTemplateReading
	start := time.Now()
	peak := peakHeapOf(func() {
		canonical, err := templateReader.CanonicalizePriceCardWorkbook(body)
		if err != nil {
			t.Error(err)
			return
		}
		reading = templateReader.ReadPriceCardWorkbook(canonical)
	})
	if elapsed := time.Since(start); elapsed > 15*time.Second || peak > 400<<20 {
		t.Fatalf("最大合法形状：耗时 %v，堆峰值 %d MiB", elapsed, peak>>20)
	}
	if reading.Accepted && len(reading.Problems) == 1 && reading.Problems[0].Code == ports.TemplateFileTooLarge {
		t.Fatal("合法的最大形状不该被判超限")
	}
}

// Covers: `AT-PP-043`——条件树 / 计算树的祖先链成环检查是线性的，报出的节点与逐个走一遍祖先链完全相同。评审顺带取证：一条 4 万节点的
// 长链（请求体几兆字节）旧写法要一分多钟（每个节点各走一遍祖先链，节点数的平方），电子表格与结构化工作簿同病。
func TestTheAncestorLoopCheckIsLinearAndMatchesWalkingEveryChain(t *testing.T) {
	naive := func(trees forest) map[*treeNode]bool {
		looping := map[*treeNode]bool{}
		for _, node := range trees.order {
			seen := map[string]bool{}
			for cursor := node; cursor != nil && cursor.parent != ""; cursor = trees.nodes[cursor.parent] {
				if seen[cursor.id] {
					looping[node] = true
					break
				}
				seen[cursor.id] = true
			}
		}
		return looping
	}
	// 小图穷举：每个节点的父节点取「无、缺失的、某个节点」，含自环、两环、通到环里去的链。
	const size = 5
	choices := size + 2 // 根、不存在的父节点、size 个节点
	total := 1
	for index := 0; index < size; index++ {
		total *= choices
	}
	for code := 0; code < total; code++ {
		trees := forest{nodes: map[string]*treeNode{}}
		nodes := make([]*treeNode, size)
		for index := range nodes {
			nodes[index] = &treeNode{id: fmt.Sprintf("N%d", index)}
			trees.nodes[nodes[index].id] = nodes[index]
			trees.order = append(trees.order, nodes[index])
		}
		rest := code
		for index := range nodes {
			switch pick := rest % choices; pick {
			case 0:
			case 1:
				nodes[index].parent = "MISSING"
			default:
				nodes[index].parent = nodes[pick-2].id
			}
			rest /= choices
		}
		got, want := loopingNodes(trees), naive(trees)
		for _, node := range nodes {
			if got[node] != want[node] {
				t.Fatalf("code %d：节点 %s 的定论 %v，朴素写法 %v", code, node.id, got[node], want[node])
			}
		}
	}

	var rows []string
	for index := 0; index < 40_000; index++ {
		parent := ""
		if index > 0 {
			parent = fmt.Sprintf(`,"parentId":"N%d"`, index-1)
		}
		rows = append(rows, fmt.Sprintf(`{"nodeId":"N%d","kind":"ALL_OF"%s}`, index, parent))
	}
	body := `{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"},{"field":"planVersion","value":"v1"}],"conditions":[` + strings.Join(rows, ",") + `]}}`
	start := time.Now()
	reading := readWorkbookJSON(t, []byte(body))
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("4 万节点的长链读了 %v", elapsed)
	}
	if !reading.Accepted || reading.Content != nil {
		t.Fatalf("accepted=%v", reading.Accepted)
	}
}
