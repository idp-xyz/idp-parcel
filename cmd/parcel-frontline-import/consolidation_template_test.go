package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func sampleConsolidationTemplate(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "consolidation-v1.csv"))
	if err != nil {
		t.Fatalf("读样例集运模板：%v", err)
	}
	return raw
}

const consolidationHeader = "templateVersion,batchRef,factRef,operator,performedBy,action,unitRef,assetRef,memberUnit,sealRef,basisRef,evidenceRef,occurredAt\n"

// consolidationLine 拼一行合成值：四个专属列按动作各自给值，空串即留空。
func consolidationLine(factRef, action, asset, member, seal, basis string) string {
	return strings.Join([]string{
		"CONSOLIDATION-1", "SYN-BATCH-1", factRef, "SYN-CLERK-A", "SYN-PACKER-1", action, "SYN-CU-1",
		asset, member, seal, basis, "SYN-WORKSHEET-1", "2026-09-08T14:00:00+08:00",
	}, ",") + "\n"
}

// TestDecodeConsolidationSampleTemplateLandsMarkersAndCommands 证样例模板译装保真：九行六种
// 动作各自落到对应的命令类型；来源身份带批次与行事实号而**没有**录入者，证据引用带录入者
// 与作业单号，执行方是现场的人而不是内勤；业务时间按模板所写的时刻（含时区）解析。
func TestDecodeConsolidationSampleTemplateLandsMarkersAndCommands(t *testing.T) {
	batch, err := decodeConsolidationTemplate(synTenant(t), sampleConsolidationTemplate(t))
	if err != nil {
		t.Fatalf("样例集运模板译装失败：%v", err)
	}
	if batch.BatchRef != "SYN-BATCH-20260908-PM" || len(batch.Rows) != 9 {
		t.Fatalf("批次 = %q，行数 = %d", batch.BatchRef, len(batch.Rows))
	}

	opened := batch.Rows[0]
	if opened.Line != 2 || opened.FactRef != "SYN-WORK-0001" || opened.Operator != "SYN-CLERK-A" || opened.Action != domain.OpenUnitAction {
		t.Fatalf("首行元数据失真：%+v", opened)
	}
	open, ok := opened.Command.(application.OpenUnitCommand)
	if !ok {
		t.Fatalf("OPEN_UNIT 行没译成 OpenUnitCommand：%T", opened.Command)
	}
	if open.TenantID.String() != "SYN-T1" || open.Unit.String() != "SYN-CU-0001" || open.Asset.String() != "SYN-CAGE-01" {
		t.Fatalf("开启命令字段失真：%+v", open)
	}
	if open.Source.SourceID() != "FTI/CONSOLIDATION-1/SYN-BATCH-20260908-PM/SYN-WORK-0001" {
		t.Fatalf("来源身份 = %q，要带批次与行事实号且不带录入者", open.Source.SourceID())
	}
	if open.Source.Evidence().String() != "FTI/CONSOLIDATION-1/SYN-CLERK-A/SYN-WORKSHEET-0001" {
		t.Fatalf("证据引用 = %q，要带录入者与作业单号", open.Source.Evidence())
	}
	if open.Source.PerformedBy().String() != "SYN-PACKER-1" {
		t.Fatalf("执行方 = %q，要现场的人而不是内勤", open.Source.PerformedBy())
	}
	if !open.Source.OccurredAt().Equal(mustInstant(t, "2026-09-08T06:00:00Z")) {
		t.Fatalf("业务时间 = %s，要模板所写的 14:00+08:00", open.Source.OccurredAt())
	}

	added, ok := batch.Rows[1].Command.(application.AddMemberCommand)
	if !ok || added.Member.String() != "SYN-HU-0001" || added.Unit.String() != "SYN-CU-0001" {
		t.Fatalf("ADD_MEMBER 行失真：%T %+v", batch.Rows[1].Command, batch.Rows[1].Command)
	}
	sealed, ok := batch.Rows[3].Command.(application.SealUnitCommand)
	if !ok || sealed.Seal.String() != "SYN-SEAL-0001" || sealed.Basis.String() != "SYN-BASIS-SEAL-01" {
		t.Fatalf("SEAL_UNIT 行失真：%T %+v", batch.Rows[3].Command, batch.Rows[3].Command)
	}
	unsealed, ok := batch.Rows[4].Command.(application.UnsealUnitCommand)
	if !ok || unsealed.Basis.String() != "SYN-BASIS-UNSEAL-01" {
		t.Fatalf("UNSEAL_UNIT 行失真：%T %+v", batch.Rows[4].Command, batch.Rows[4].Command)
	}
	removed, ok := batch.Rows[5].Command.(application.RemoveMemberCommand)
	if !ok || removed.Member.String() != "SYN-HU-0002" {
		t.Fatalf("REMOVE_MEMBER 行失真：%T %+v", batch.Rows[5].Command, batch.Rows[5].Command)
	}
	closed, ok := batch.Rows[8].Command.(application.CloseUnitCommand)
	if !ok || closed.Unit.String() != "SYN-CU-0002" || closed.Disposition.String() != "" {
		t.Fatalf("CLOSE_UNIT 行失真（空单元关闭不带处置依据）：%T %+v", batch.Rows[8].Command, batch.Rows[8].Command)
	}
	if closed.Source.PerformedBy().String() != "SYN-PACKER-2" || closed.Source.Evidence().String() != "FTI/CONSOLIDATION-1/SYN-CLERK-B/SYN-WORKSHEET-0003" {
		t.Fatalf("第二组执行方/证据失真：%+v", closed.Source)
	}
}

// TestDecodeConsolidationTemplateAcceptsCloseWithDisposition 证关闭格的 basisRef 是可选而不是
// 禁填：给了就成为处置转移依据进命令，不给就留零值交领域按成员是否清空把门。
func TestDecodeConsolidationTemplateAcceptsCloseWithDisposition(t *testing.T) {
	raw := consolidationHeader + consolidationLine("SYN-WORK-1", "CLOSE_UNIT", "", "", "", "SYN-BASIS-DISPOSE-1")
	batch, err := decodeConsolidationTemplate(synTenant(t), []byte(raw))
	if err != nil {
		t.Fatalf("带处置依据的关闭行被拒：%v", err)
	}
	closed, ok := batch.Rows[0].Command.(application.CloseUnitCommand)
	if !ok || closed.Disposition.String() != "SYN-BASIS-DISPOSE-1" {
		t.Fatalf("处置依据没进命令：%T %+v", batch.Rows[0].Command, batch.Rows[0].Command)
	}
}

// TestDecodeConsolidationTemplateRejectsShapeErrors 证整批拒绝的各格：骨架那几条（列集、版本、
// 行事实号）挂的是 ErrConsolidationTemplate，六种动作各自的必填与禁填逐格拒且报错点名列与
// 动作。
func TestDecodeConsolidationTemplateRejectsShapeErrors(t *testing.T) {
	openLine := consolidationLine("SYN-WORK-1", "OPEN_UNIT", "SYN-CAGE-1", "", "", "")
	cases := []struct {
		name  string
		raw   string
		wants string
	}{
		{"多一列", strings.TrimSuffix(consolidationHeader, "\n") + ",extra\n" + strings.TrimSuffix(openLine, "\n") + ",x\n", "多列 [extra]"},
		{"缺一列", strings.Replace(consolidationHeader, ",performedBy", "", 1) + strings.Replace(openLine, ",SYN-PACKER-1", "", 1), "缺列 [performedBy]"},
		{"只有表头", consolidationHeader, "只有表头"},
		{"模板版本不符", consolidationHeader + strings.Replace(openLine, "CONSOLIDATION-1", "INTAKE-1", 1), "只认 \"CONSOLIDATION-1\""},
		{"行事实号重复", consolidationHeader + openLine + openLine, "第 3 行 factRef=\"SYN-WORK-1\" 与第 2 行重复"},
		{"录入者为空", consolidationHeader + strings.Replace(openLine, "SYN-CLERK-A", "", 1), "operator 为空"},
		{"执行方为空", consolidationHeader + strings.Replace(openLine, "SYN-PACKER-1", "", 1), "performedBy 为空"},
		{"单元为空", consolidationHeader + strings.Replace(openLine, "SYN-CU-1", "", 1), "unitRef 为空"},
		{"证据为空", consolidationHeader + strings.Replace(openLine, "SYN-WORKSHEET-1", "", 1), "evidenceRef 为空"},
		{"时间不是 RFC3339", consolidationHeader + strings.Replace(openLine, "2026-09-08T14:00:00+08:00", "2026/09/08 14:00", 1), "不是 RFC3339"},
		{"词表外动作", consolidationHeader + consolidationLine("SYN-WORK-1", "PACK", "SYN-CAGE-1", "", "", ""), "不在词表"},
		{"OPEN_UNIT 缺载具", consolidationHeader + consolidationLine("SYN-WORK-1", "OPEN_UNIT", "", "", "", ""), "OPEN_UNIT 行 assetRef 为空"},
		{"OPEN_UNIT 带成员", consolidationHeader + consolidationLine("SYN-WORK-1", "OPEN_UNIT", "SYN-CAGE-1", "SYN-HU-1", "", ""), "OPEN_UNIT 行不得带 memberUnit（这一格只在 ADD_MEMBER / REMOVE_MEMBER 行有意义）"},
		{"ADD_MEMBER 缺成员", consolidationHeader + consolidationLine("SYN-WORK-1", "ADD_MEMBER", "", "", "", ""), "ADD_MEMBER 行 memberUnit 为空"},
		{"ADD_MEMBER 带载具", consolidationHeader + consolidationLine("SYN-WORK-1", "ADD_MEMBER", "SYN-CAGE-1", "SYN-HU-1", "", ""), "ADD_MEMBER 行不得带 assetRef（这一格只在 OPEN_UNIT 行有意义）"},
		{"REMOVE_MEMBER 带封签", consolidationHeader + consolidationLine("SYN-WORK-1", "REMOVE_MEMBER", "", "SYN-HU-1", "SYN-SEAL-1", ""), "REMOVE_MEMBER 行不得带 sealRef（这一格只在 SEAL_UNIT 行有意义）"},
		{"SEAL_UNIT 缺封签", consolidationHeader + consolidationLine("SYN-WORK-1", "SEAL_UNIT", "", "", "", "SYN-BASIS-1"), "SEAL_UNIT 行 sealRef 为空"},
		{"SEAL_UNIT 缺依据", consolidationHeader + consolidationLine("SYN-WORK-1", "SEAL_UNIT", "", "", "SYN-SEAL-1", ""), "SEAL_UNIT 行 basisRef 为空"},
		{"SEAL_UNIT 带成员", consolidationHeader + consolidationLine("SYN-WORK-1", "SEAL_UNIT", "", "SYN-HU-1", "SYN-SEAL-1", "SYN-BASIS-1"), "SEAL_UNIT 行不得带 memberUnit"},
		{"UNSEAL_UNIT 缺依据", consolidationHeader + consolidationLine("SYN-WORK-1", "UNSEAL_UNIT", "", "", "", ""), "UNSEAL_UNIT 行 basisRef 为空"},
		{"UNSEAL_UNIT 带封签", consolidationHeader + consolidationLine("SYN-WORK-1", "UNSEAL_UNIT", "", "", "SYN-SEAL-1", "SYN-BASIS-1"), "UNSEAL_UNIT 行不得带 sealRef"},
		{"CLOSE_UNIT 带载具", consolidationHeader + consolidationLine("SYN-WORK-1", "CLOSE_UNIT", "SYN-CAGE-1", "", "", ""), "CLOSE_UNIT 行不得带 assetRef"},
		{"CLOSE_UNIT 带成员", consolidationHeader + consolidationLine("SYN-WORK-1", "CLOSE_UNIT", "", "SYN-HU-1", "", ""), "CLOSE_UNIT 行不得带 memberUnit"},
		{"非 UTF-8", consolidationHeader + "CONSOLIDATION-1,\xc4\xe3\xba\xc3,SYN-WORK-1\n", "不是合法 UTF-8"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			_, err := decodeConsolidationTemplate(synTenant(t), []byte(spec.raw))
			if err == nil {
				t.Fatalf("应整批拒绝")
			}
			if !errors.Is(err, ErrConsolidationTemplate) {
				t.Fatalf("错误没带 ErrConsolidationTemplate：%v", err)
			}
			if !strings.Contains(err.Error(), spec.wants) {
				t.Fatalf("错误 = %q，要含 %q", err, spec.wants)
			}
		})
	}
}

// TestConsolidationTemplateDoesNotShareIntakeShape 证两份模板互不认：拿收寄样例喂集运口整批
// 拒且错误挂集运的根错误——两口的来源标记各带自己的模板版本，混用会让标记说谎。
func TestConsolidationTemplateDoesNotShareIntakeShape(t *testing.T) {
	_, err := decodeConsolidationTemplate(synTenant(t), sampleTemplate(t))
	if !errors.Is(err, ErrConsolidationTemplate) || !strings.Contains(err.Error(), "缺列") {
		t.Fatalf("收寄样例喂集运口应按列集不符整批拒：%v", err)
	}
	_, err = decodeIntakeTemplate(synTenant(t), sampleConsolidationTemplate(t))
	if !errors.Is(err, ErrIntakeTemplate) || !strings.Contains(err.Error(), "缺列") {
		t.Fatalf("集运样例喂收寄口应按列集不符整批拒：%v", err)
	}
}
