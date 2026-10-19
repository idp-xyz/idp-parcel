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
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证节点收寄口的操作者渠道 Intake（ADR-0149 决定二；票 operator-channel/10）的传输面：租户与节点两格只来自认证
// 结果、设备与作业范围只从三个头来且种类只认 NODE、先认证后解载荷、事实身份与发生时间照 ADR-0023 从载荷如实收、自报
// 租户即拒、形状错即拒。期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。

const (
	workFactTenant = "SYN-TENANT-01"
	// workFactNode 是替身认证方交回的节点；presentedScopeReference 是请求头 Parcel-Work-Scope-Reference 出示的引用。两值刻意不同：
	// 相同时 Intake 改拿头值铸节点也照样全绿，「节点只来自认证结果」那一格就测不出来。生产上两值相等：`internal/accessidentity`
	// 的 WorkFactMinter.MintWorkFact 只在设备登记的作业范围等于出示的那一份时放行，并把出示的作业范围原样装进信封；
	// `internal/nodeoperations/adapters/accessidentity` 的 WorkFactAuthenticator.AuthenticateNodeReception 再从信封的作业范围
	// 取节点。这里让替身答一个别的节点只为测出差别，不是在模拟真实答复。
	workFactNode            = "SYN-NODE-SZX-01"
	presentedScopeReference = "SYN-NODE-SHA-HUB"
)

// receptionAuthenticatorFake 替身认证方：记下被问的设备与作业范围；带了令牌答合成租户与节点，没带答令牌不过，err 非空时答它。
type receptionAuthenticatorFake struct {
	err   error
	asked *[]string
}

func (fake receptionAuthenticatorFake) AuthenticateNodeReception(_ context.Context, token, device, kind, reference string) (domain.TenantID, domain.NodeReference, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, device+"|"+kind+"|"+reference)
	}
	if token == "" {
		return domain.TenantID{}, domain.NodeReference{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, domain.NodeReference{}, fake.err
	}
	tenant, err := domain.NewTenantID(workFactTenant)
	if err != nil {
		return domain.TenantID{}, domain.NodeReference{}, err
	}
	node, err := domain.NewNodeReference(workFactNode)
	return tenant, node, err
}

func receptionIntakeForTest(t *testing.T) *nodeopshttp.WorkFactReceptionIntake {
	t.Helper()
	return receptionIntakeWith(t, receptionAuthenticatorFake{})
}

func receptionIntakeWith(t *testing.T, authenticator nodeopshttp.WorkFactAuthenticator) *nodeopshttp.WorkFactReceptionIntake {
	t.Helper()
	intake, err := nodeopshttp.NewWorkFactReceptionIntake(authenticator)
	if err != nil {
		t.Fatalf("构造作业事实收寄 Intake：%v", err)
	}
	return intake
}

// receptionRequest 带一份合规的作业事实出示（令牌、在用设备、NODE 作业范围）。
func receptionRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("Parcel-Device-Id", "SYN-DEVICE-NODE")
	request.Header.Set("Parcel-Work-Scope-Kind", "NODE")
	request.Header.Set("Parcel-Work-Scope-Reference", presentedScopeReference)
	return request
}

// Covers: ReceptionIntake 契约「租户与节点身份只能来自认证结果；事实身份与发生时间必须从请求体收」——认证结果是作业事实
// 信封与设备登在的节点；sourceId 与 occurredAt 逐字来自载荷，请求头与查询串里的自报一律无视。
func TestWorkFactReceptionIntakeTranslatesReceptionWithTheAuthenticatedTenantAndNode(t *testing.T) {
	intake := receptionIntakeForTest(t)
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
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q（认证结果，不是请求里的自报）", got, workFactTenant)
	}
	if got := command.Node.String(); got != workFactNode {
		t.Fatalf("Node = %q, want %q（认证结果，不是请求头 Parcel-Work-Scope-Reference 出示的 %q）", got, workFactNode, presentedScopeReference)
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
	// 服务结果标记不从载荷收，也不在命令上：由收寄编排在身份唯一成立时向 PS 取（票 operator-channel/25）。
}

// Covers: 接收观察的明确拒收与仅扫描——前者带原因、后者带标识，都不带接收证据（与受控批量口同一组形状）。
func TestWorkFactReceptionIntakeTranslatesRefusalAndScanOnlyReceptions(t *testing.T) {
	intake := receptionIntakeForTest(t)

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

// Covers: 载荷里带 tenantId 或 node 即拒（MALFORMED_REQUEST）——两格只来自认证结果，值与认证结果相同也拒，否则「碰巧相同」的
// 载荷在别的环境里就是穿透 ADR-0003 隔离边界、或替设备报一个它不在的节点的第一步。
func TestWorkFactReceptionIntakeRefusesSelfReportedTenantOrNode(t *testing.T) {
	intake := receptionIntakeForTest(t)
	rest := `"sourceId":"SYN-DEVICE-07/000126","deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0004","mark":"SYN-MARK-0004",` +
		`"claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-0004","occurredAt":"2026-09-24T09:30:00+08:00"`
	for name, testCase := range map[string]struct{ body, key string }{
		"另一租户":     {`{"tenantId":"SYN-TEN-OTHER",` + rest + `}`, "tenantId"},
		"与认证结果同租户": {`{"tenantId":"` + workFactTenant + `",` + rest + `}`, "tenantId"},
		"自报节点":     {`{"node":"SYN-NODE-SIN-HUB",` + rest + `}`, "node"},
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
func TestWorkFactReceptionIntakeRefusesMalformedReceptionPayloads(t *testing.T) {
	intake := receptionIntakeForTest(t)
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

// Covers: 事实身份与发生时刻缺席，经生产 Intake 走到端点答 400 `MALFORMED_REQUEST`，不交零值给编排——见 ADR-0186
// 「复核记录（2026-10-19，决定原文不动）」：事实身份是判重键，缺了连重放都认不出，与决定四「输入缺陷」同一格。TF 作业
// 事实口那种「缺席交编排、答未受理」在这里不适用；改成交编排时这条先红。编排一侧装的是只会报错的替身：漏过 Intake 就成
// 500，不会碰巧也是 400。
func TestWorkFactReceptionAnswers400ForAbsentFactIdentityOrOccurrenceTime(t *testing.T) {
	endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(receptionIntakeForTest(t), failingHandler{})
	const rest = `"deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0011","claim":"SCAN_ONLY","mark":"SYN-MARK-0011"`
	for name, body := range map[string]string{
		"缺事实身份":   `{` + rest + `,"occurredAt":"2026-09-24T09:45:00+08:00"}`,
		"事实身份为空白": `{"sourceId":" ",` + rest + `,"occurredAt":"2026-09-24T09:45:00+08:00"}`,
		"缺发生时刻":   `{"sourceId":"SYN-DEVICE-07/000131",` + rest + `}`,
		"发生时刻为空串": `{"sourceId":"SYN-DEVICE-07/000132",` + rest + `,"occurredAt":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, receptionRequest(body))
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
				t.Fatalf("status = %d body = %s, want 400 MALFORMED_REQUEST", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// Covers: 缺席的认证方在构造时拒，不等第一个请求。
func TestNewWorkFactReceptionIntakeRejectsAMissingAuthenticator(t *testing.T) {
	if _, err := nodeopshttp.NewWorkFactReceptionIntake(nil); err == nil {
		t.Fatal("没有认证方也装得起来")
	}
}

// Covers: 设备与作业范围只从三个头来，缺一即坏报文；收寄口只认 NODE——认了 OFFSITE，场外揽收的设备就能在节点上打收寄。
// 先认证、后解载荷：认证方的答复原样交回，令牌不过的调用方看不到载荷校验的结果。
func TestWorkFactReceptionIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst(t *testing.T) {
	const body = `{"sourceId":"SYN-DEVICE-07/000130","deliveredBy":"SYN-COURIER-01","handlingUnit":"SYN-HU-0010","mark":"SYN-MARK-0010",` +
		`"claim":"SCAN_ONLY","occurredAt":"2026-09-24T09:40:00+08:00"}`
	var asked []string
	intake := receptionIntakeWith(t, receptionAuthenticatorFake{asked: &asked})
	if _, err := intake.IntakeReception(context.Background(), receptionRequest(body)); err != nil {
		t.Fatalf("合规出示被拒：%v", err)
	}
	if len(asked) != 1 || asked[0] != "SYN-DEVICE-NODE|NODE|"+presentedScopeReference {
		t.Fatalf("认证方被问 %v，want 三个头里的设备与作业范围", asked)
	}
	for name, mutate := range map[string]func(*http.Request){
		"缺设备":     func(request *http.Request) { request.Header.Del("Parcel-Device-Id") },
		"缺作业范围种类": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Kind") },
		"缺作业范围引用": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Reference") },
		"场外作业范围":  func(request *http.Request) { request.Header.Set("Parcel-Work-Scope-Kind", "OFFSITE") },
	} {
		request := receptionRequest(body)
		mutate(request)
		if _, err := intake.IntakeReception(context.Background(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
	refused := receptionIntakeWith(t, receptionAuthenticatorFake{err: nodeopshttp.ErrDeviceNotInUse})
	if _, err := refused.IntakeReception(context.Background(), receptionRequest("!!not-json!!")); !errors.Is(err, nodeopshttp.ErrDeviceNotInUse) {
		t.Fatalf("设备不在用：err = %v, want 认证方的答复，而不是载荷校验的结果", err)
	}
}

// Covers: ADR-0149 决定二——作业事实收寄 Intake 只实现收寄口；查阅行归查阅读，在类型上就装不进本类型。
func TestWorkFactReceptionIntakeServesOnlyTheReceptionLine(t *testing.T) {
	var intake any = receptionIntakeForTest(t)
	if _, ok := intake.(nodeopshttp.ReceptionIntake); !ok {
		t.Fatal("收寄口该由这一面实现")
	}
	if _, ok := intake.(nodeopshttp.CatalogueQueryIntake); ok {
		t.Fatal("查阅行归查阅读，作业事实 Intake 不该装得进")
	}
}
