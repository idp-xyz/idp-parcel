package main

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 证受控批量口上同一作业实物在同一节点的去向（ADR-0199 决定四，票 no-reception-source/04；隔离合成 S）：
// 生产装配的导入链，身份核对接 PS 的外部标识关联册，册里没有模板上的号码。

// sameUnitTemplate 是同一件实物在同一节点先后三行：仅扫描、明确接收、明确拒收。
const sameUnitTemplate = "templateVersion,batchRef,factRef,operator,node,deliveredBy,handlingUnit,mark,claim,evidenceRef,refusalReason,occurredAt\n" +
	"INTAKE-1,SYN-BATCH-NRS04,SYN-SLIP-NRS04-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-NRS04,SYN-MARK-NRS04,SCAN_ONLY,,,2026-09-08T09:15:00+08:00\n" +
	"INTAKE-1,SYN-BATCH-NRS04,SYN-SLIP-NRS04-2,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-NRS04,SYN-MARK-NRS04,RECEIVED,SYN-RECEIPT-NRS04,,2026-09-08T09:20:00+08:00\n" +
	"INTAKE-1,SYN-BATCH-NRS04,SYN-SLIP-NRS04-3,SYN-CLERK-B,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-NRS04,SYN-MARK-NRS04,REFUSED,,SYN-REASON-NRS04,2026-09-08T09:25:00+08:00\n"

// Covers: `AT-NO-050` 与 `AT-NO-049` 的受控批量口面（ADR-0199 决定四）：同一件实物在同一节点先仅扫描、再明确接收、再明确拒收。
// 仅扫描落下待确认；明确接收续办它——号码查无此号，答待识别、建控制，以那条待确认为前版；明确拒收并入那条待识别，答已有
// 结果、记重放，明细写明并入的那一版、本行来源已保全。同一份文件重跑，三行都答已有结果，第三行的明细照样写明并入。
func TestFrontlineImportContinuesAndMergesRowsOfTheSameUnit(t *testing.T) {
	importer, receptions := verticalImporter(t, nil)
	results := importTemplate(t, importer, []byte(sameUnitTemplate))
	if len(results) != 3 {
		t.Fatalf("结果行数 = %d，要 3", len(results))
	}
	assertDisposition(t, results, 0, rowLanded, "RECEPTION_UNDECIDED")
	assertDisposition(t, results, 1, rowLanded, "PENDING_IDENTIFICATION")
	assertDisposition(t, results, 2, rowReplayed, "EXISTING_RESULT")
	if code := exitCodeFor(results); code != exitLanded {
		t.Fatalf("退出码 = %d，要 %d", code, exitLanded)
	}

	scan, _ := findReception(t, receptions, "FTI/INTAKE-1/SYN-BATCH-NRS04/SYN-SLIP-NRS04-1")
	pending, found := findReception(t, receptions, "FTI/INTAKE-1/SYN-BATCH-NRS04/SYN-SLIP-NRS04-2")
	if !found || pending.Predecessor != scan.JudgmentVersion || scan.JudgmentVersion.String() == "" {
		t.Fatalf("明确接收那一行 前版 = %q, want 仅扫描那一行的判断版本 %q", pending.Predecessor, scan.JudgmentVersion)
	}
	merged, found := findReception(t, receptions, "FTI/INTAKE-1/SYN-BATCH-NRS04/SYN-SLIP-NRS04-3")
	if !found || merged.Kind != ports.RecordMergedIntoExistingIntake || merged.MergedInto != pending.JudgmentVersion ||
		merged.Source.Refusal != "SYN-REASON-NRS04" {
		t.Fatalf("拒收那一行在册 = %+v, want 并入行指名 %q、来源带拒收原因", merged, pending.JudgmentVersion)
	}
	want := "并入已有收寄 " + pending.JudgmentVersion.String()
	if !strings.Contains(results[2].Detail, want) || !strings.Contains(results[2].Detail, "原判断 PENDING_IDENTIFICATION") {
		t.Fatalf("并入那一行的明细 = %q, want 含 %q 与原判断种类", results[2].Detail, want)
	}

	replayed := importTemplate(t, importer, []byte(sameUnitTemplate))
	for index := range replayed {
		assertDisposition(t, replayed, index, rowReplayed, "EXISTING_RESULT")
	}
	if !strings.Contains(replayed[2].Detail, want) {
		t.Fatalf("重跑时并入那一行的明细 = %q, want 含 %q", replayed[2].Detail, want)
	}
}
