package nodeopshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// 本文件对隔离写路径准入在节点作业命令面的注入式放行（ADR-0091；票 operator-channel/08）证传输面：租户与节点两格
// 只来自注入、事实身份与发生时间照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。

const (
	isolatedCommandTenant = "SYN-TENANT-01"
	isolatedCommandNode   = "SYN-NODE-SHA-HUB"
)

func isolatedCommandIntakeForTest(t *testing.T) *nodeopshttp.IsolatedCommandIntake {
	t.Helper()
	intake, err := nodeopshttp.NewIsolatedCommandIntake(isolatedCommandTenant, isolatedCommandNode)
	if err != nil {
		t.Fatalf("构造隔离命令 Intake：%v", err)
	}
	return intake
}

func receptionRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(body))
}

// Covers: ReceptionIntake 契约「租户与节点身份只能来自认证结果；事实身份与发生时间必须从请求体收」——隔离形态的认证
// 结果是开关值与装配点的合成节点；sourceId 与 occurredAt 逐字来自载荷，请求头与查询串里的自报一律无视。
func TestIsolatedCommandIntakeTranslatesReceptionWithInjectedTenantAndNode(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	request := receptionRequest(`{
		"sourceId":"SYN-DEVICE-07/000123",
		"deliveredBy":"SYN-COURIER-01",
		"handlingUnit":"SYN-HU-0001",
		"mark":"SYN-MARK-0001",
		"claim":"RECEIVED",
		"evidenceRef":"SYN-EVIDENCE/tally-0001",
		"occurredAt":"2026-09-24T09:15:00+08:00"
	}`)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9&node=NODE-9"

	command, err := intake.IntakeReception(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandTenant)
	}
	if got := command.Node.String(); got != isolatedCommandNode {
		t.Fatalf("Node = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandNode)
	}
	if command.SourceID != "SYN-DEVICE-07/000123" {
		t.Fatalf("SourceID = %q, want 载荷里设备签发的那一个", command.SourceID)
	}
	wantOccurred := time.Date(2026, 9, 24, 1, 15, 0, 0, time.UTC)
	if !command.OccurredAt.Equal(wantOccurred) {
		t.Fatalf("OccurredAt = %s, want %s（设备记录的业务时间，不是服务端时钟）", command.OccurredAt, wantOccurred)
	}
	if got := command.DeliveredBy.String(); got != "SYN-COURIER-01" {
		t.Fatalf("DeliveredBy = %q, want SYN-COURIER-01", got)
	}
	if got := command.Unit.String(); got != "SYN-HU-0001" {
		t.Fatalf("Unit = %q, want SYN-HU-0001", got)
	}
	if command.Mark.Mark != "SYN-MARK-0001" {
		t.Fatalf("Mark = %q, want SYN-MARK-0001", command.Mark.Mark)
	}
	if command.Claim != application.ExplicitReception {
		t.Fatalf("Claim = %v, want ExplicitReception", command.Claim)
	}
	if got := command.Evidence.String(); got != "SYN-EVIDENCE/tally-0001" {
		t.Fatalf("Evidence = %q, want SYN-EVIDENCE/tally-0001", got)
	}
	if len(command.ServiceMarkers) != 0 {
		t.Fatalf("ServiceMarkers = %v, want 空——服务结果标记由接入层查好带入，隔离形态没有那道查询，也不从载荷收", command.ServiceMarkers)
	}
}

// Covers: 接收观察的明确拒收与仅扫描——前者带原因、后者带标识，都不带接收证据（与受控批量口同一组形状）。
func TestIsolatedCommandIntakeTranslatesRefusalAndScanOnlyReceptions(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)

	refused, err := intake.IntakeReception(context.Background(), receptionRequest(`{"sourceId":"SYN-DEVICE-07/000124",`+
		`"deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0002","claim":"REFUSED","refusalReason":"SYN 外包装破损拒收",`+
		`"occurredAt":"2026-09-24T09:20:00+08:00"}`))
	if err != nil {
		t.Fatalf("拒收：%v", err)
	}
	if refused.Claim != application.ExplicitRefusal || refused.Refusal != "SYN 外包装破损拒收" {
		t.Fatalf("拒收 = %v / %q, want ExplicitRefusal / 载荷里的原因", refused.Claim, refused.Refusal)
	}

	scanned, err := intake.IntakeReception(context.Background(), receptionRequest(`{"sourceId":"SYN-DEVICE-07/000125",`+
		`"deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0003","mark":"SYN-MARK-0003","claim":"SCAN_ONLY",`+
		`"occurredAt":"2026-09-24T09:25:00+08:00"}`))
	if err != nil {
		t.Fatalf("仅扫描：%v", err)
	}
	if scanned.Claim != application.ScanOnlyObservation || scanned.Mark.Mark != "SYN-MARK-0003" {
		t.Fatalf("仅扫描 = %v / %q, want ScanOnlyObservation / SYN-MARK-0003", scanned.Claim, scanned.Mark.Mark)
	}
}

// Covers: 载荷里带 tenantId 或 node 即拒（MALFORMED_REQUEST）——两格只来自注入，值与注入相同也拒，否则「碰巧相同」的
// 载荷在别的环境里就是穿透 ADR-0003 隔离边界、或替设备报一个它不在的节点的第一步。
func TestIsolatedCommandIntakeRefusesSelfReportedTenantOrNode(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	rest := `"sourceId":"SYN-DEVICE-07/000126","deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0004","mark":"SYN-MARK-0004",` +
		`"claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-0004","occurredAt":"2026-09-24T09:30:00+08:00"`
	for name, testCase := range map[string]struct{ body, key string }{
		"另一租户":   {`{"tenantId":"SYN-TEN-OTHER",` + rest + `}`, "tenantId"},
		"与注入同租户": {`{"tenantId":"` + isolatedCommandTenant + `",` + rest + `}`, "tenantId"},
		"自报节点":   {`{"node":"SYN-NODE-SIN-HUB",` + rest + `}`, "node"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeReception(context.Background(), receptionRequest(testCase.body))
			if !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if !strings.Contains(err.Error(), testCase.key) {
				t.Fatalf("拒绝理由没点名 %s：%v", testCase.key, err)
			}
		})
	}
}

// Covers: 形状级失败包 ErrMalformedRequest（4xx，重发同样内容不会好，判据同 ADR-0029）。接收观察与证据、原因、标识的
// 搭配与受控批量口同一组门：明确接收必带证据与标识（FormNodeIntake 缺证据会让编排答不出，那是 5xx 而它明明是用法错），
// 拒收与仅扫描不许带证据（既有记录形状没有地方放它，静默丢掉会让操作员以为登进去了）。
func TestIsolatedCommandIntakeRefusesMalformedReceptionPayloads(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	base := func(extra string) string {
		return `{"sourceId":"SYN-DEVICE-07/000127","deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0005",` +
			`"occurredAt":"2026-09-24T09:35:00+08:00",` + extra + `}`
	}
	for name, body := range map[string]string{
		"不是 JSON": "!!not-json!!",
		"空载荷":     "",
		"未知字段":    base(`"claim":"SCAN_ONLY","mark":"m","serviceMarkers":["CANCELLED"]`),
		"尾随内容":    base(`"claim":"SCAN_ONLY","mark":"m"`) + ` {"x":1}`,
		"词表外的观察":  base(`"claim":"ARRIVED","mark":"m"`),
		"缺事实身份":   `{"deliveredBy":"d","handlingUnit":"u","claim":"SCAN_ONLY","mark":"m","occurredAt":"2026-09-24T09:35:00+08:00"}`,
		"缺交付方":    `{"sourceId":"s","handlingUnit":"u","claim":"SCAN_ONLY","mark":"m","occurredAt":"2026-09-24T09:35:00+08:00"}`,
		"缺实物":     `{"sourceId":"s","deliveredBy":"d","claim":"SCAN_ONLY","mark":"m","occurredAt":"2026-09-24T09:35:00+08:00"}`,
		"时刻不是时刻":  `{"sourceId":"s","deliveredBy":"d","handlingUnit":"u","claim":"SCAN_ONLY","mark":"m","occurredAt":"昨天"}`,
		"明确接收缺证据": base(`"claim":"RECEIVED","mark":"m"`),
		"明确接收缺标识": base(`"claim":"RECEIVED","evidenceRef":"e"`),
		"明确接收带原因": base(`"claim":"RECEIVED","mark":"m","evidenceRef":"e","refusalReason":"r"`),
		"拒收缺原因":   base(`"claim":"REFUSED"`),
		"拒收带证据":   base(`"claim":"REFUSED","refusalReason":"r","evidenceRef":"e"`),
		"仅扫描缺标识":  base(`"claim":"SCAN_ONLY"`),
		"仅扫描带证据":  base(`"claim":"SCAN_ONLY","mark":"m","evidenceRef":"e"`),
		"仅扫描带原因":  base(`"claim":"SCAN_ONLY","mark":"m","refusalReason":"r"`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := intake.IntakeReception(context.Background(), receptionRequest(body))
			if !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: 立不起来的注入在构造时拒，不等第一个请求（同隔离读 Intake 的纪律）。
func TestNewIsolatedCommandIntakeRejectsBlankInjection(t *testing.T) {
	if _, err := nodeopshttp.NewIsolatedCommandIntake("   ", isolatedCommandNode); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := nodeopshttp.NewIsolatedCommandIntake(isolatedCommandTenant, "   "); err == nil {
		t.Fatal("空节点被接受")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行，不是一次全开」——本类型只实现放行了的口，查阅行归隔离读 Intake，
// 写 Intake 在类型上就装不进去（两个开关分设，ADR-0091 决定四）。
func TestIsolatedCommandIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedCommandIntakeForTest(t)
	if _, ok := intake.(nodeopshttp.ReceptionIntake); !ok {
		t.Fatal("收寄口该已放行")
	}
	if _, ok := intake.(nodeopshttp.CatalogueQueryIntake); ok {
		t.Fatal("查阅行归隔离读 Intake，写 Intake 不该装得进")
	}
}
