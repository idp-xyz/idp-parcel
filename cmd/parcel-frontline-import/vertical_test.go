package main

import (
	"context"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证 buildIntakeImporter 装配的整条链（隔离合成 S）：模板译装 →
// 既有收寄用例 → 真库，来源标记两格真落库，重放与来源冲突在真库上分得开。
//
// 两个用例分别钉住身份核对缝的两侧，因为那一格是本口今天唯一的运行期阻断：接显式未配置
// 替身时 `RECEIVED` 行必须**什么都不落库**（而不是留个半成品），换能解析的替身时同一份
// 模板必须整行落地。缺了后者，「PS 侧提供方到位后只换这一格」就只是注释里的一句话。

// resolvingParcelIdentityView 是身份核对缝的能解析替身，只在本用例存在。它答唯一候选，
// 因此走的是 AT-NO-015 关联收寄那一格——生产上这一格要等 PS 侧的外部标识关联模型，
// 见票 ps-external-mark-relations/01。
type resolvingParcelIdentityView struct {
	association domain.ParcelAssociationReference
}

var _ ports.ParcelIdentityView = resolvingParcelIdentityView{}

func (view resolvingParcelIdentityView) ResolveParcelIdentity(
	context.Context,
	domain.TenantID,
	ports.ExternalMarkObservation,
) ([]domain.ParcelAssociationReference, error) {
	return []domain.ParcelAssociationReference{view.association}, nil
}

// 样例模板三行的来源身份。写成字面量而不是调 intakeSourceID 拼：这里要证的正是落库的
// 键长什么样，拿被测函数自己算一遍等于什么都没证。
const (
	sourceIDReceived = "FTI/INTAKE-1/SYN-BATCH-20260908-AM/SYN-SLIP-0001"
	sourceIDRefused  = "FTI/INTAKE-1/SYN-BATCH-20260908-AM/SYN-SLIP-0002"
	sourceIDScanOnly = "FTI/INTAKE-1/SYN-BATCH-20260908-AM/SYN-SLIP-0003"
)

func verticalImporter(
	t *testing.T,
	identity ports.ParcelIdentityView,
) (intakeImporter, *nopostgres.Receptions) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	// 记录时刻用固定钟，业务时间来自模板——用例据此证两者不是同一个来源。
	importer, err := buildIntakeImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}, identity)
	if err != nil {
		t.Fatalf("装配导入口：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库读口：%v", err)
	}
	return importer, receptions
}

func importTemplate(t *testing.T, importer intakeImporter, raw []byte) []rowResult {
	t.Helper()
	batch, err := decodeIntakeTemplate(synTenant(t), raw)
	if err != nil {
		t.Fatalf("模板译装失败：%v", err)
	}
	return importIntake(t.Context(), batch, importer)
}

func assertDisposition(t *testing.T, results []rowResult, index int, want rowDisposition, wantOutcome string) {
	t.Helper()
	got := results[index]
	if got.Disposition != want || got.Outcome != wantOutcome {
		t.Fatalf("第 %d 行 factRef=%s → %s [%s]，要 %s [%s]（%s）",
			got.Line, got.FactRef, got.Outcome, got.Disposition, wantOutcome, want, got.Detail)
	}
}

func findReception(t *testing.T, receptions *nopostgres.Receptions, sourceID string) (ports.ReceptionRecord, bool) {
	t.Helper()
	record, found, err := receptions.FindByKey(t.Context(),
		ports.ReceptionKey{TenantID: synTenant(t), SourceID: sourceID})
	if err != nil {
		t.Fatalf("读回 %s：%v", sourceID, err)
	}
	return record, found
}

// TestFrontlineImportVerticalOnRealPostgres 是生产形状：身份核对缝接显式未配置替身。
// 证三件——两支不经身份核对的行真落库并读得回；`RECEIVED` 行答未决且**一行都不落**；
// 同一份文件重跑答重放，同一行事实号换了业务时间答来源冲突且原行不被顶替。
func TestFrontlineImportVerticalOnRealPostgres(t *testing.T) {
	importer, receptions := verticalImporter(t, unconfiguredParcelIdentityView{})
	if importer.identityConfigured {
		t.Fatal("生产形状下 identityConfigured 应为假——开工提示靠它")
	}

	results := importTemplate(t, importer, sampleTemplate(t))
	if len(results) != 3 {
		t.Fatalf("结果行数 = %d，要 3", len(results))
	}
	assertDisposition(t, results, 0, rowUndecided, "RECEPTION_UNDECIDED")
	assertDisposition(t, results, 1, rowLanded, "INTAKE_NOT_FORMED")
	assertDisposition(t, results, 2, rowLanded, "RECEPTION_UNDECIDED")

	// 未决行必须带续办引用：重跑靠它，而不是靠内勤记得自己跑过。
	if !strings.Contains(results[0].Detail, "未落库；续办引用 CONT-") {
		t.Fatalf("RECEIVED 行未决没带续办引用：%q", results[0].Detail)
	}
	// 只要还有未决，退出码就是 3——「重跑同一文件」仍是必要动作。
	if code := exitCodeFor(results); code != exitUndecided {
		t.Fatalf("退出码 = %d，要 %d", code, exitUndecided)
	}

	// 身份核对不通的那一行什么都没留：拿一件没核对过身份的实物记成已核对，是这条断言
	// 要挡住的事。
	if _, found := findReception(t, receptions, sourceIDReceived); found {
		t.Fatal("RECEIVED 行在身份核对缝不通时落库了——本该整笔回滚")
	}

	refused, found := findReception(t, receptions, sourceIDRefused)
	if !found {
		t.Fatalf("REFUSED 行没落库：%s", sourceIDRefused)
	}
	if refused.Kind != ports.RecordIntakeNotFormed {
		t.Fatalf("REFUSED 行落库种类 = %s，要 INTAKE_NOT_FORMED", refused.Kind)
	}
	if refused.RefusalReason != "SYN-REASON-PACKAGING-DAMAGED" {
		t.Fatalf("拒收原因 = %q", refused.RefusalReason)
	}

	scanOnly, found := findReception(t, receptions, sourceIDScanOnly)
	if !found {
		t.Fatalf("SCAN_ONLY 行没落库：%s", sourceIDScanOnly)
	}
	if scanOnly.Kind != ports.RecordReceptionUndecided {
		t.Fatalf("SCAN_ONLY 行落库种类 = %s，要 RECEPTION_UNDECIDED", scanOnly.Kind)
	}
	// 待确认也是一条已提交的记录：扫描来源保全了，只是没建立控制。这一格与「未决不落库」
	// 长得像而语义相反，两者在同一个用例里各钉一次才分得开。
	if scanOnly.Control.Active() {
		t.Fatal("SCAN_ONLY 行不该建立控制")
	}

	// 重放：同一份文件重跑，已落地的两行答已有结果，未决那行仍未决。
	replayed := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, replayed, 0, rowUndecided, "RECEPTION_UNDECIDED")
	assertDisposition(t, replayed, 1, rowReplayed, "EXISTING_RESULT")
	assertDisposition(t, replayed, 2, rowReplayed, "EXISTING_RESULT")

	// 来源冲突：同一行事实号换了业务时间即另一份内容（摘要含业务时间）。绝不覆盖。
	conflicting := strings.Replace(string(sampleTemplate(t)),
		"SYN-REASON-PACKAGING-DAMAGED,2026-09-08T09:20:00+08:00",
		"SYN-REASON-PACKAGING-DAMAGED,2026-09-08T10:20:00+08:00", 1)
	conflicted := importTemplate(t, importer, []byte(conflicting))
	assertDisposition(t, conflicted, 1, rowRejected, "SOURCE_CONFLICT")
	if code := exitCodeFor(conflicted); code != exitUndecided {
		t.Fatalf("未决压过被拒：退出码 = %d，要 %d", code, exitUndecided)
	}

	after, found := findReception(t, receptions, sourceIDRefused)
	if !found || !after.RecordedAt.Equal(refused.RecordedAt) || after.ContentDigest != refused.ContentDigest {
		t.Fatalf("冲突把原行改了：记录时刻 %s→%s，摘要 %s→%s",
			refused.RecordedAt, after.RecordedAt, refused.ContentDigest, after.ContentDigest)
	}
}

// TestFrontlineImportLandsReceivedRowOnceIdentityResolves 换能解析的替身跑同一份模板，
// 证 buildIntakeImporter 的 identity 参数就是 PS 侧提供方到位后唯一要换的那一格：
// `RECEIVED` 行整行落地，两处来源标记真落库，业务时间来自模板而记录时刻来自时钟。
func TestFrontlineImportLandsReceivedRowOnceIdentityResolves(t *testing.T) {
	association, err := domain.NewParcelAssociationReference("SYN-PARCEL-0001")
	if err != nil {
		t.Fatalf("构造包裹关联：%v", err)
	}
	importer, receptions := verticalImporter(t, resolvingParcelIdentityView{association: association})
	if !importer.identityConfigured {
		t.Fatal("换真替身后 identityConfigured 应为真——开工提示不该再打印")
	}

	results := importTemplate(t, importer, sampleTemplate(t))
	assertDisposition(t, results, 0, rowLanded, "INTAKE_FORMED")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("三行全落地退出码 = %d，要 %d", code, exitLanded)
	}

	record, found := findReception(t, receptions, sourceIDReceived)
	if !found {
		t.Fatalf("RECEIVED 行没落库：%s", sourceIDReceived)
	}
	if record.Kind != ports.RecordIntakeFormed || record.IdentityConflict {
		t.Fatalf("落库种类 = %s，冲突 = %v", record.Kind, record.IdentityConflict)
	}
	// 证据引用带录入者：ADR-0089 决定④ 的「录入操作者落在证据引用里」在真库上成立。
	if got := record.Intake.Evidence().String(); got != "FTI/INTAKE-1/SYN-CLERK-A/SYN-RECEIPT-0001" {
		t.Fatalf("落库证据引用 = %q，要带 FTI 标记与录入者", got)
	}
	if got, associated := record.Intake.Association(); !associated || got.String() != "SYN-PARCEL-0001" {
		t.Fatalf("包裹关联 = %q（在场 %v）", got, associated)
	}
	// 业务时间与记录时刻不是同一个来源：前者是内勤抄自纸单的现场时刻，后者是导入这一刻。
	// 两者若都取时钟，导入进来的事实就再也说不出现场什么时候发生的。
	wantOccurred := mustInstant(t, "2026-09-08T09:15:00+08:00")
	if !record.Intake.ReceivedAt().Equal(wantOccurred) {
		t.Fatalf("落库业务时间 = %s，要模板所写的 %s", record.Intake.ReceivedAt(), wantOccurred)
	}
	if !record.RecordedAt.Equal(mustInstant(t, "2026-09-08T12:00:00+08:00")) {
		t.Fatalf("落库记录时刻 = %s，要固定钟给的 12:00+08:00", record.RecordedAt)
	}
	if !record.RecordedAt.After(record.Intake.ReceivedAt()) {
		t.Fatal("记录时刻不晚于业务时间——两者被同一个来源填了")
	}
}
