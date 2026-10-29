package pricecardtemplate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// registrationPlanBlock 把读出的方案折成登记快照，取其中的 plan 块（与种子文件里的 plan 块同一个折装函数出的）。
func registrationPlanBlock(t *testing.T, plan domain.PricingPlanVersion, authorization domain.VersionReference) map[string]any {
	t.Helper()
	tenant := value(t, domain.NewTenantID, "SYN-TENANT-01")
	source := try(domain.NewSourceFileIdentity("SYN-CARD-CN-SG-FULL-03.xlsx", strings.Repeat("a", 64)))
	registration := try(domain.NewPriceCardRegistration(tenant, plan, source, authorization, "SYN-PRICING-GOVERNOR-01"))
	raw := try(domain.MarshalPriceCardRegistration(registration))
	return planBlockOf(t, raw)
}

func planBlockOf(t *testing.T, snapshot []byte) map[string]any {
	t.Helper()
	var document struct {
		Plan map[string]any `json:"plan"`
	}
	if err := json.Unmarshal(snapshot, &document); err != nil {
		t.Fatal(err)
	}
	return document.Plan
}

// Covers: `AT-PP-041`，ADR-0349 决定一、二——用模板把演示租户的完整价卡三 FULL-03 重新表达一遍（偏远一条规则按档位查价表、
// 总额下限 120.99），导入得到的方案与种子的 plan 块逐值相等、内容摘要相同、规范化版本 PPC-6；同一份内容读文件与读表单工作簿
// 得同一个结果。这是模板新结构的最强验收：两条构造路径互相独立，相等即翻译没有漏格、没有改值。
func TestFull03ReExpressedInTheTemplateImportsToTheSeedPlan(t *testing.T) {
	seedRaw, seed := seedFull03(t)
	card := full03Card()

	reading := read(t, card.bytes(t))
	plan := validated(t, reading)
	if plan.ContentDigest() != seed.Plan().ContentDigest() {
		t.Fatalf("导入摘要 %s，种子 %s", plan.ContentDigest(), seed.Plan().ContentDigest())
	}
	if plan.CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("带总额下限与按档位查表的方案写 %s，要 PPC-6", plan.CanonicalizationVersion())
	}
	if !reading.Plan.SameIdentity(seed.Plan().Reference()) || !reading.Content.DirectionAuthorization.SameIdentity(seed.DirectionAuthorization()) {
		t.Fatal("方案身份或方向授权对不上")
	}
	if reading.TemplateVersion != TemplateVersion {
		t.Fatalf("template version %q", reading.TemplateVersion)
	}
	got, want := registrationPlanBlock(t, plan, reading.Content.DirectionAuthorization), planBlockOf(t, seedRaw)
	if !reflect.DeepEqual(got, want) {
		gotRaw, _ := json.MarshalIndent(got, "", " ")
		wantRaw, _ := json.MarshalIndent(want, "", " ")
		t.Fatalf("plan 块与种子不逐值相等\n导入：%s\n种子：%s", gotRaw, wantRaw)
	}

	fromForm := Reader{}.ReadPriceCardWorkbook(mustCanonical(t, card.workbookJSON(t)))
	formPlan := validated(t, fromForm)
	if formPlan.ContentDigest() != plan.ContentDigest() || !reflect.DeepEqual(registrationPlanBlock(t, formPlan, fromForm.Content.DirectionAuthorization), want) {
		t.Fatal("表单工作簿读出的方案与读文件读出的不同")
	}
}

func mustCanonical(t *testing.T, raw []byte) []byte {
	t.Helper()
	canonical, err := Reader{}.CanonicalizePriceCardWorkbook(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

// golden 夹具：入库的 xlsx 与 JSON 工作簿与测试里的 FULL-03 写法逐字节相同，且各自读出种子的摘要。夹具给下一位做编辑器的人
// 当样例（JSON）、给人手工导入试（xlsx）；它们不能与代码漂开。
func TestFull03GoldenFixturesAreCurrentAndImportToTheSeedDigest(t *testing.T) {
	_, seed := seedFull03(t)
	card := full03Card()
	golden := map[string][]byte{
		full03XLSXGolden: card.bytes(t),
		full03JSONGolden: card.workbookJSON(t),
	}
	for name, generated := range golden {
		path := filepath.Join("testdata", name)
		if *updateGolden {
			if err := os.WriteFile(path, generated, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		committed, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读不到 golden %s：%v；加 -update-golden 生成", name, err)
		}
		if !bytes.Equal(committed, generated) {
			t.Fatalf("golden %s 与测试里的写法不同；改了列定义或夹具就加 -update-golden 重写", name)
		}
	}
	xlsx, _ := os.ReadFile(filepath.Join("testdata", full03XLSXGolden))
	if digest := validated(t, read(t, xlsx)).ContentDigest(); digest != seed.Plan().ContentDigest() {
		t.Errorf("golden xlsx 摘要 %s，种子 %s", digest, seed.Plan().ContentDigest())
	}
	workbook, _ := os.ReadFile(filepath.Join("testdata", full03JSONGolden))
	canonical := mustCanonical(t, workbook)
	if digest := validated(t, Reader{}.ReadPriceCardWorkbook(canonical)).ContentDigest(); digest != seed.Plan().ContentDigest() {
		t.Errorf("golden JSON 摘要 %s，种子 %s", digest, seed.Plan().ContentDigest())
	}
}

// Covers: `AT-PP-042`，ADR-0349 决定三——PPT-1 的文件在新构建里读出与此前一字不差的东西：同一个内容摘要与规范化版本 PPC-5，
// 读法的版本回显仍是 PPT-1；PPT-2 的文件不带新结构也仍写 PPC-5、摘要不变。
func TestPPT1FilesStillImportToTheSameDigestAndPPT2WithoutNewStructuresStaysPPC5(t *testing.T) {
	for file, card := range map[string]filled{"price-card-cn-sg-cost.json": costCard(), "price-card-cn-sg.json": sellCard()} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds", "data", "pricing", file))
		if err != nil {
			t.Fatal(err)
		}
		seed, err := domain.RehydratePriceCardRegistration(raw)
		if err != nil {
			t.Fatal(err)
		}
		legacy := read(t, card.asPPT1(t))
		plan := validated(t, legacy)
		if legacy.TemplateVersion != templateVersionV1 || plan.ContentDigest() != seed.Plan().ContentDigest() || plan.CanonicalizationVersion() != "PPC-5" {
			t.Errorf("%s: PPT-1 文件读出 version=%s digest=%s canonicalization=%s，种子 %s", file, legacy.TemplateVersion, plan.ContentDigest(), plan.CanonicalizationVersion(), seed.Plan().ContentDigest())
		}
		current := validated(t, read(t, card.bytes(t)))
		if current.ContentDigest() != plan.ContentDigest() || current.CanonicalizationVersion() != "PPC-5" {
			t.Errorf("%s: 同一份内容的 PPT-2 文件读出 %s / %s，与 PPT-1 的 %s 不同", file, current.ContentDigest(), current.CanonicalizationVersion(), plan.ContentDigest())
		}
	}
	// 全结构卡（用到旧版每一张表）也一样。
	legacy := validated(t, read(t, fullCard().asPPT1(t)))
	current := validated(t, read(t, fullCard().bytes(t)))
	if legacy.ContentDigest() != current.ContentDigest() || legacy.ContentDigest() != fullPlan(t).ContentDigest() {
		t.Fatal("全结构卡的 PPT-1 与 PPT-2 文件读出不同的方案")
	}
}

// Covers: `AT-PP-043`，ADR-0349 决定三——PPT-1 文件里出现 PPT-2 才有的东西，答明确的问题（说人话：自哪一版起才有、怎么办），
// 而不是笼统的「未知表」「不是可取值」：整张 total_bounds 表，或 calculations 的 TABLE_LOOKUP_BY_REMOTE_TIER。
func TestPPT1FilesUsingPPT2ThingsAreToldWhy(t *testing.T) {
	// 带 total_bounds 表的 PPT-1 文件：把 PPT-2 文件的 card 表版本改回 PPT-1，表集合不动。
	card := full03Card()
	card.card["templateVersion"] = templateVersionV1
	reading := read(t, card.bytes(t))
	if !reading.Accepted || reading.Content != nil || reading.TemplateVersion != templateVersionV1 {
		t.Fatalf("accepted=%v content=%v version=%s", reading.Accepted, reading.Content != nil, reading.TemplateVersion)
	}
	found := map[string]bool{}
	for _, problem := range reading.Problems {
		switch {
		case problem.Sheet == sheetTotalBounds && problem.Row == 0 && problem.Column == "" && problem.Code == ports.TemplateSheetUnknown:
			found["sheet"] = strings.Contains(problem.Message, "自模板 PPT-2 起才有") && strings.Contains(problem.Message, "换用 PPT-2 模板")
		case problem.Sheet == sheetCalculations && problem.Column == "method" && problem.Code == ports.TemplateCellInvalid:
			found["method"] = strings.Contains(problem.Message, methodTableLookupByRemoteTier) && strings.Contains(problem.Message, "自模板 PPT-2 起才可用")
		}
	}
	if !found["sheet"] || !found["method"] {
		t.Fatalf("要点名「自 PPT-2 起才有」：%s", describe(reading.Problems))
	}
	for _, problem := range reading.Problems {
		if strings.Contains(problem.Message, "可取：") && strings.Contains(problem.Message, methodTableLookupByRemoteTier) {
			t.Errorf("旧版文件的可取值清单里不该有新取值：%s", problem.Message)
		}
	}
}

// Covers: `AT-PP-044`，ADR-0349 决定一——total_bounds 的各种写法：只填一端、两端都填、两端都缺、多行、值立不住、下限高于上限、卡声明了
// 金额取整而值不在进位单位的整数倍上；问题落在 total_bounds 的格上，而不是整份方案上。
func TestTotalBoundsRowsAreReadAndRefusedAtTheirCells(t *testing.T) {
	plan := func(boundsRows rows) domain.PricingPlanVersion {
		card := full03Card()
		card.data[sheetTotalBounds] = boundsRows
		return validated(t, read(t, card.bytes(t)))
	}
	only := plan(rows{{"minimum": "120.99"}})
	if bounds, ok := only.Structures().TotalChargeBounds(); !ok {
		t.Fatal("没读出总额上下限")
	} else if _, hasMax := bounds.Maximum(); hasMax {
		t.Fatal("只填了最低收费，不该有最高收费")
	}
	capped := plan(rows{{"maximum": "900"}})
	if bounds, _ := capped.Structures().TotalChargeBounds(); func() bool { _, ok := bounds.Minimum(); return ok }() {
		t.Fatal("只填了最高收费，不该有最低收费")
	}
	both := plan(rows{{"minimum": "120.99", "maximum": "900"}})
	if bounds, _ := both.Structures().TotalChargeBounds(); func() bool { maximum, ok := bounds.Maximum(); return ok && maximum.Amount().String() == "900" }() == false {
		t.Fatal("两端都填，最高收费应为 900")
	}

	cases := map[string]struct {
		rows rows
		want at
	}{
		"多出一行":      {rows{{"minimum": "120.99"}, {"maximum": "900"}}, at{sheetTotalBounds, 3, "minimum", ports.TemplateCellNotApplicable}},
		"不是十进制数":    {rows{{"minimum": "1e2"}}, at{sheetTotalBounds, 2, "minimum", ports.TemplateCellInvalid}},
		"下限高于上限":    {rows{{"minimum": "900", "maximum": "120.99"}}, at{sheetTotalBounds, 2, "minimum", ports.TemplateConstructionRejected}},
		"零下限":       {rows{{"minimum": "0"}}, at{sheetTotalBounds, 2, "minimum", ports.TemplateConstructionRejected}},
		"不在进位单位整倍数": {rows{{"minimum": "120.995"}}, at{sheetTotalBounds, 2, "minimum", ports.TemplateConstructionRejected}},
		"最高不在整倍数":   {rows{{"maximum": "900.005"}}, at{sheetTotalBounds, 2, "maximum", ports.TemplateConstructionRejected}},
	}
	for name, testCase := range cases {
		card := full03Card()
		card.data[sheetTotalBounds] = testCase.rows
		reading := read(t, card.bytes(t))
		if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, testCase.want) {
			t.Errorf("%s: want %+v, got problems=%s", name, testCase.want, describe(reading.Problems))
		}
		for _, problem := range reading.Problems {
			if problem.Sheet == sheetCard && problem.Code == ports.TemplateConstructionRejected {
				t.Errorf("%s: 问题不该落到整份方案上：%s", name, describe(reading.Problems))
			}
		}
	}

	// 没有一行就是没声明：方案不带总额上下限，也不写 PPC-6 之外的东西（这张卡还有按档位查表，所以仍是 PPC-6）。
	none := plan(nil)
	if _, declared := none.Structures().TotalChargeBounds(); declared {
		t.Fatal("没填 total_bounds 却读出了总额上下限")
	}
}

// Covers: `AT-PP-045`，ADR-0349 决定二——TABLE_LOOKUP_BY_REMOTE_TIER：必须在 reference_catalogues 绑 REMOTE_TIER 目录，否则问题落在计算
// 那一格并说清缘由；被它指到的价表不会被报「没人用」；不带这类计算也不带总额上下限的卡仍写 PPC-5。
func TestRemoteTierLookupNeedsTheCatalogueAndOnlyNewStructuresWritePPC6(t *testing.T) {
	card := full03Card()
	card.data[sheetReferenceCatalogues] = rows{{"kind": "ZONE", "catalogueId": "SYN-CAT-ZONE-CN-SG-FULL"}}
	reading := read(t, card.bytes(t))
	var methodRow int
	for index, line := range card.data[sheetCalculations] {
		if line["nodeId"] == "K-REMOTE" {
			methodRow = index + 2
		}
	}
	want := at{sheetCalculations, methodRow, "method", ports.TemplateReferenceUnresolved}
	if !reading.Accepted || reading.Content != nil || !hasProblem(reading.Problems, want) {
		t.Fatalf("want %+v, got problems=%s", want, describe(reading.Problems))
	}
	for _, problem := range reading.Problems {
		if problem.Code == ports.TemplateConstructionRejected || problem.Code == ports.TemplateReferenceUnused {
			t.Errorf("同一件事不报两遍，也不连带报别的：%s", describe([]ports.PriceCardTemplateProblem{problem}))
		}
	}

	// 去掉总额下限、保留按档位查表：仍是 PPC-6；两样都去掉（偏远退回按分区查）才是 PPC-5。
	card = full03Card()
	card.data[sheetTotalBounds] = nil
	if got := validated(t, read(t, card.bytes(t))).CanonicalizationVersion(); got != "PPC-6" {
		t.Errorf("按档位查表而无总额上下限：%s", got)
	}
	card = full03Card()
	card.data[sheetTotalBounds] = nil
	for _, line := range card.data[sheetCalculations] {
		if line["nodeId"] == "K-REMOTE" {
			line["method"] = "TABLE_LOOKUP"
		}
	}
	if got := validated(t, read(t, card.bytes(t))).CanonicalizationVersion(); got != "PPC-5" {
		t.Errorf("无新结构：%s", got)
	}
}

// 空白模板新带 total_bounds 表与说明行，且模板版本是 PPT-2；说明表点出 PPT-1 仍接受。
func TestBlankTemplateCarriesThePPT2Sheet(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteTemplate(&buffer); err != nil {
		t.Fatal(err)
	}
	if TemplateFileName != "price-card-import-template-PPT-2.xlsx" {
		t.Fatalf("file name %s", TemplateFileName)
	}
	book := mustReadBook(t, buffer.Bytes())
	names := map[string]bool{}
	var readme []string
	for _, sheet := range book.Sheets {
		names[sheet.Name] = true
		if sheet.Name == sheetReadme {
			for _, row := range sheet.Rows {
				for _, cell := range row.Cells {
					readme = append(readme, cell.Text)
				}
			}
		}
	}
	if !names[sheetTotalBounds] || !names[sheetReadme] {
		t.Fatalf("sheets = %v", names)
	}
	text := strings.Join(readme, "\n")
	for _, want := range []string{"最低收费", "最高收费", methodTableLookupByRemoteTier, "旧版 PPT-1 的文件仍然接受", "补足到最低收费"} {
		if !strings.Contains(text, want) {
			t.Errorf("说明表缺「%s」", want)
		}
	}
}

// 不认的版本答整份不收，话里列出本构建认的全部版本。
func TestUnsupportedVersionNamesTheAcceptedOnes(t *testing.T) {
	card := costCard()
	card.card["templateVersion"] = "PPT-0"
	reading := read(t, card.bytes(t))
	if reading.Accepted || len(reading.Problems) != 1 || reading.Problems[0].Code != ports.TemplateVersionUnsupported ||
		!strings.Contains(reading.Problems[0].Message, "PPT-1、PPT-2") {
		t.Fatalf("problems = %s", describe(reading.Problems))
	}
}

// 旧版文件里写错的计算类型，话里列的可取值仍是旧版那五个：新取值不泄进旧版的清单（PPT-1 文件的问题清单与此前逐字相同）。
func TestPPT1ClosedValueListingsDoNotMentionPPT2Values(t *testing.T) {
	card := fullCard().clone()
	card.data[sheetCalculations][3]["method"] = "TABLE"
	reading := read(t, card.asPPT1(t))
	var message string
	for _, problem := range reading.Problems {
		if problem.Sheet == sheetCalculations && problem.Column == "method" {
			message = problem.Message
		}
	}
	const want = "「TABLE」不是可取值；可取：FIXED_AMOUNT、TABLE_LOOKUP、PERCENT_OF_BASIS、GREATER_OF、SERIES_AMOUNT"
	if message != want {
		t.Fatalf("PPT-1 的取值清单：%q", message)
	}
	current := read(t, func() []byte {
		c := fullCard().clone()
		c.data[sheetCalculations][3]["method"] = "TABLE"
		return c.bytes(t)
	}())
	for _, problem := range current.Problems {
		if problem.Sheet == sheetCalculations && problem.Column == "method" && !strings.Contains(problem.Message, methodTableLookupByRemoteTier) {
			t.Errorf("PPT-2 的取值清单该有新计算类型：%s", problem.Message)
		}
	}
}
