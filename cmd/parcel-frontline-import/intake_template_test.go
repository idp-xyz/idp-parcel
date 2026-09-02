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

func synTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	return tenant
}

func sampleTemplate(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "intake-v1.csv"))
	if err != nil {
		t.Fatalf("读样例模板：%v", err)
	}
	return raw
}

const templateHeader = "templateVersion,batchRef,factRef,operator,node,deliveredBy,handlingUnit,mark,claim,evidenceRef,refusalReason,occurredAt\n"

func receivedRow(factRef, unit string) string {
	return "INTAKE-1,SYN-BATCH-1," + factRef + ",SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1," + unit + ",SYN-MARK-" + unit + ",RECEIVED,SYN-RECEIPT-" + factRef + ",,2026-09-08T09:15:00+08:00\n"
}

// TestDecodeSampleTemplateLandsMarkersAndClaims 证样例模板译装保真：三行三态各自对到编排
// 的接收观察，来源身份与证据引用带 `FTI/INTAKE-1/` 标记——身份里有批次与行事实号而
// **没有**录入者，证据里有录入者与纸面证据号；业务时间按模板所写的时刻（含时区）解析，
// 不被换成当前时刻。
func TestDecodeSampleTemplateLandsMarkersAndClaims(t *testing.T) {
	batch, err := decodeIntakeTemplate(synTenant(t), sampleTemplate(t))
	if err != nil {
		t.Fatalf("样例模板译装失败：%v", err)
	}
	if batch.BatchRef != "SYN-BATCH-20260908-AM" || len(batch.Rows) != 3 {
		t.Fatalf("批次 = %q，行数 = %d", batch.BatchRef, len(batch.Rows))
	}

	received := batch.Rows[0]
	if received.Line != 2 || received.FactRef != "SYN-SLIP-0001" || received.Operator != "SYN-CLERK-A" {
		t.Fatalf("首行元数据失真：%+v", received)
	}
	if received.Command.SourceID != "FTI/INTAKE-1/SYN-BATCH-20260908-AM/SYN-SLIP-0001" {
		t.Fatalf("来源身份 = %q，要带批次与行事实号且不带录入者", received.Command.SourceID)
	}
	if received.Command.Claim != application.ExplicitReception {
		t.Fatalf("RECEIVED 没译成明确接收：%d", received.Command.Claim)
	}
	if received.Command.Evidence.String() != "FTI/INTAKE-1/SYN-CLERK-A/SYN-RECEIPT-0001" {
		t.Fatalf("证据引用 = %q，要带录入者与纸面证据号", received.Command.Evidence)
	}
	if received.Command.TenantID.String() != "SYN-T1" ||
		received.Command.Node.String() != "SYN-NODE-SZX" ||
		received.Command.DeliveredBy.String() != "SYN-CUSTOMER-1" ||
		received.Command.Unit.String() != "SYN-HU-0001" ||
		received.Command.Mark.Mark != "SYN-MARK-0001" {
		t.Fatalf("首行命令字段失真：%+v", received.Command)
	}
	if !received.Command.OccurredAt.Equal(mustInstant(t, "2026-09-08T01:15:00Z")) {
		t.Fatalf("业务时间 = %s，要模板所写的 09:15+08:00", received.Command.OccurredAt)
	}

	refused := batch.Rows[1]
	if refused.Command.Claim != application.ExplicitRefusal || refused.Command.Refusal != "SYN-REASON-PACKAGING-DAMAGED" {
		t.Fatalf("REFUSED 行失真：claim=%d refusal=%q", refused.Command.Claim, refused.Command.Refusal)
	}
	if refused.Command.Evidence.String() != "" {
		t.Fatalf("REFUSED 行不该带证据引用：%q", refused.Command.Evidence)
	}

	scanOnly := batch.Rows[2]
	if scanOnly.Command.Claim != application.ScanOnlyObservation || scanOnly.Operator != "SYN-CLERK-B" {
		t.Fatalf("SCAN_ONLY 行失真：%+v", scanOnly)
	}
	if len(scanOnly.Command.ServiceMarkers) != 0 {
		t.Fatalf("模板不得往 ServiceMarkers 里放东西：%v", scanOnly.Command.ServiceMarkers)
	}
}

// TestDecodeTemplateToleratesBOMAndColumnOrder 证 Excel「CSV UTF-8」的 BOM 不算进第一列名，
// 列顺序打乱也认——列集是接口，顺序不是。
func TestDecodeTemplateToleratesBOMAndColumnOrder(t *testing.T) {
	shuffled := "\xef\xbb\xbfoccurredAt,claim,mark,handlingUnit,deliveredBy,node,operator,factRef,batchRef,templateVersion,refusalReason,evidenceRef\n" +
		"2026-09-08T09:15:00+08:00,RECEIVED,SYN-MARK-1,SYN-HU-1,SYN-CUSTOMER-1,SYN-NODE-SZX,SYN-CLERK-A,SYN-SLIP-1,SYN-BATCH-1,INTAKE-1,,SYN-RECEIPT-1\n"
	batch, err := decodeIntakeTemplate(synTenant(t), []byte(shuffled))
	if err != nil {
		t.Fatalf("带 BOM 且乱序的模板被拒：%v", err)
	}
	if batch.Rows[0].Command.SourceID != "FTI/INTAKE-1/SYN-BATCH-1/SYN-SLIP-1" {
		t.Fatalf("乱序列没对回去：%q", batch.Rows[0].Command.SourceID)
	}
}

// TestDecodeTemplateRejectsShapeErrors 证整批拒绝的各格：任一格错就整批不开工，且错误带
// ErrIntakeTemplate 与能定位的行号或列名。
func TestDecodeTemplateRejectsShapeErrors(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		wants string
	}{
		{"多一列", strings.TrimSuffix(templateHeader, "\n") + ",extra\n" + strings.TrimSuffix(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "\n") + ",x\n", "多列 [extra]"},
		{"缺一列", strings.Replace(templateHeader, ",occurredAt", "", 1) + "INTAKE-1,SYN-BATCH-1,SYN-SLIP-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-1,SYN-MARK-1,RECEIVED,SYN-RECEIPT-1,\n", "缺列 [occurredAt]"},
		{"重复列", strings.Replace(templateHeader, "refusalReason", "mark", 1) + receivedRow("SYN-SLIP-1", "SYN-HU-1"), "重复"},
		{"只有表头", templateHeader, "只有表头"},
		{"模板版本不符", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "INTAKE-1", "INTAKE-2", 1), "只认 \"INTAKE-1\""},
		{"批次不一致", templateHeader + receivedRow("SYN-SLIP-1", "SYN-HU-1") + strings.Replace(receivedRow("SYN-SLIP-2", "SYN-HU-2"), "SYN-BATCH-1", "SYN-BATCH-2", 1), "一份文件一个批次"},
		{"行事实号重复", templateHeader + receivedRow("SYN-SLIP-1", "SYN-HU-1") + receivedRow("SYN-SLIP-1", "SYN-HU-2"), "第 3 行 factRef=\"SYN-SLIP-1\" 与第 2 行重复"},
		{"行事实号为空", templateHeader + receivedRow("", "SYN-HU-1"), "factRef 为空"},
		{"录入者为空", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "SYN-CLERK-A", "", 1), "operator 为空"},
		{"节点为空", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "SYN-NODE-SZX", "", 1), "node 为空"},
		{"词表外接收观察", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "RECEIVED", "ACCEPTED", 1), "不在词表"},
		{"RECEIVED 缺证据", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "SYN-RECEIPT-SYN-SLIP-1", "", 1), "evidenceRef 为空"},
		{"RECEIVED 缺标识", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "SYN-MARK-SYN-HU-1", "", 1), "mark 为空"},
		{"RECEIVED 带拒收原因", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), ",,2026", ",SYN-REASON,2026", 1), "不得带 refusalReason"},
		{"REFUSED 缺原因", templateHeader + "INTAKE-1,SYN-BATCH-1,SYN-SLIP-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-1,SYN-MARK-1,REFUSED,,,2026-09-08T09:15:00+08:00\n", "refusalReason 为空"},
		{"REFUSED 带证据", templateHeader + "INTAKE-1,SYN-BATCH-1,SYN-SLIP-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-1,SYN-MARK-1,REFUSED,SYN-RECEIPT-1,SYN-REASON,2026-09-08T09:15:00+08:00\n", "无处可登"},
		{"SCAN_ONLY 带证据", templateHeader + "INTAKE-1,SYN-BATCH-1,SYN-SLIP-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-1,SYN-MARK-1,SCAN_ONLY,SYN-RECEIPT-1,,2026-09-08T09:15:00+08:00\n", "有接收证据请改 RECEIVED"},
		{"SCAN_ONLY 缺标识", templateHeader + "INTAKE-1,SYN-BATCH-1,SYN-SLIP-1,SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1,SYN-HU-1,,SCAN_ONLY,,,2026-09-08T09:15:00+08:00\n", "mark 为空"},
		{"时间不是 RFC3339", templateHeader + strings.Replace(receivedRow("SYN-SLIP-1", "SYN-HU-1"), "2026-09-08T09:15:00+08:00", "2026/09/08 09:15", 1), "不是 RFC3339"},
		{"列数不齐", templateHeader + "INTAKE-1,SYN-BATCH-1\n", "第 2 行读取失败"},
		{"非 UTF-8", templateHeader + "INTAKE-1,\xc4\xe3\xba\xc3,SYN-SLIP-1\n", "不是合法 UTF-8"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			_, err := decodeIntakeTemplate(synTenant(t), []byte(spec.raw))
			if err == nil {
				t.Fatalf("应整批拒绝")
			}
			if !errors.Is(err, ErrIntakeTemplate) {
				t.Fatalf("错误没带 ErrIntakeTemplate：%v", err)
			}
			if !strings.Contains(err.Error(), spec.wants) {
				t.Fatalf("错误 = %q，要含 %q", err, spec.wants)
			}
		})
	}
}
