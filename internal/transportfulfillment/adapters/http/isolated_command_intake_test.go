package tfhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件对隔离写路径准入在运输履约主链命令面的注入式放行（ADR-0091；票 operator-channel/08）证传输面：租户格只来自
// 注入、事实内容与发生时间照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、未放行的口在类型上装不进去。
// 期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。成形与否（对象引用立不立得住、控制证据缺不缺）不在这里证
// ——那是编排答`未受理`的格，本 Intake 不预判。

const (
	isolatedCommandTenant         = "SYN-TENANT-01"
	isolatedCommandMovementSource = "SYN-SOURCE/self-operated-executor"
)

func isolatedCommandIntakeForTest(t *testing.T) *tfhttp.IsolatedCommandIntake {
	t.Helper()
	intake, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{
		Tenant:         isolatedCommandTenant,
		MovementSource: isolatedCommandMovementSource,
	})
	if err != nil {
		t.Fatalf("构造隔离命令 Intake：%v", err)
	}
	return intake
}

func commandRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"
	return request
}

// isolatedLine 是一口已放行的线：怎么调它的 Intake、一份合规载荷。自报租户、尾随内容与类型锁三条对每一口都成立，
// 表驱动逐口跑——每放一口在这里加一行。
type isolatedLine struct {
	intake func(*tfhttp.IsolatedCommandIntake, *http.Request) error
	valid  string
}

var isolatedLines = map[string]isolatedLine{
	"/transport-fulfillment/offsite-pickups": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupRegistration(context.Background(), request)
			return err
		},
		valid: offsitePickupRegistrationBody,
	},
	"/transport-fulfillment/offsite-pickup-attempts": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupAttempt(context.Background(), request)
			return err
		},
		valid: offsitePickupAttemptBody,
	},
	"/transport-fulfillment-carrier-first-effective-pickup-judgments": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakeCarrierPickupJudgment(context.Background(), request)
			return err
		},
		valid: carrierPickupJudgmentBody,
	},
	"/transport-fulfillment/handovers": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakeHandoverRegistration(context.Background(), request)
			return err
		},
		valid: handoverRegistrationBody,
	},
	"/transport-fulfillment/movement-facts": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakeMovementFact(context.Background(), request)
			return err
		},
		valid: isolatedMovementFactBody,
	},
	"/transport-fulfillment-dispatch-task-registrations": {
		intake: func(intake *tfhttp.IsolatedCommandIntake, request *http.Request) error {
			_, err := intake.IntakeDispatchTask(context.Background(), request)
			return err
		},
		valid: isolatedDispatchTaskBody,
	},
}

const isolatedDispatchTaskBody = `{"task":"SYN-DISPATCH-08-07","kind":"DELIVERY","objects":["SYN-PARCEL-08-07","SYN-PARCEL-08-08"],` +
	`"place":"SYN-PLACE/consignee-07","windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00",` +
	`"conditions":"SYN-CONDITION/signature-required","openedAt":"2026-09-24T20:00:00+08:00"}`

// Covers: DispatchTaskIntake 契约「工作范围七件全从请求收」——任务、种类、对象范围、地点、时间窗、条件与建立时刻逐字来自载荷。
func TestIsolatedCommandIntakeTranslatesDispatchTaskWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeDispatchTask(context.Background(), commandRequest(isolatedDispatchTaskBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q", got, isolatedCommandTenant)
	}
	if command.Task != "SYN-DISPATCH-08-07" || command.Kind.String() != "DELIVERY" || len(command.Objects) != 2 ||
		command.Objects[0] != "SYN-PARCEL-08-07" || command.Objects[1] != "SYN-PARCEL-08-08" ||
		command.Place != "SYN-PLACE/consignee-07" || command.Conditions != "SYN-CONDITION/signature-required" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	for name, pair := range map[string][2]time.Time{
		"windowFrom": {command.WindowFrom, time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)},
		"windowTo":   {command.WindowTo, time.Date(2026, 9, 25, 4, 0, 0, 0, time.UTC)},
		"openedAt":   {command.OpenedAt, time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
}

// Covers: 任务种类取 domain.DispatchTaskKind 的封闭词——词表外是用法错误（400）。
func TestIsolatedCommandIntakeRefusesAnUnknownDispatchTaskKind(t *testing.T) {
	_, err := isolatedCommandIntakeForTest(t).IntakeDispatchTask(context.Background(), commandRequest(`{"task":"t","kind":"RETURN"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

const isolatedMovementFactBody = `{"fact":"SYN-MOVE-08-06","schedule":"SYN-SCHEDULE/linehaul-06","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",` +
	`"version":"v1","occurredAt":"2026-09-24T14:00:00+08:00","gateRequired":true,"gateClearance":"SYN-GATE/release-06"}`

// Covers: MovementFactIntake 契约「这个口只收自营执行方的事实；『自营还是外部』由 Intake 的认证结果说」——隔离形态的认证
// 结果是装配点给定的合成来源，事实本体与门禁两格（gateRequired / gateClearance 必须收）逐字来自载荷。
func TestIsolatedCommandIntakeTranslatesMovementFactWithInjectedTenantAndSource(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeMovementFact(context.Background(), commandRequest(isolatedMovementFactBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q", got, isolatedCommandTenant)
	}
	if command.Source != isolatedCommandMovementSource {
		t.Fatalf("Source = %q, want %q（注入值——外部轨迹不从这个口进，来源不由请求自报）", command.Source, isolatedCommandMovementSource)
	}
	if command.Fact != "SYN-MOVE-08-06" || command.Schedule != "SYN-SCHEDULE/linehaul-06" || command.Kind.String() != "DEPARTURE" ||
		command.Location != "SYN-NODE-SHA-HUB" || command.Version != "v1" || !command.GateRequired ||
		command.GateClearance != "SYN-GATE/release-06" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
}

// Covers: 载荷里带 source 即拒——来源只来自注入，放它进来就是让外部轨迹冒充自营事实从这个口进。事实种类取封闭词，
// 词表外是用法错误（400）。
func TestIsolatedCommandIntakeRefusesSelfReportedMovementSourceAndUnknownKinds(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	_, err := intake.IntakeMovementFact(context.Background(), commandRequest(`{"source":"SYN-SOURCE/carrier-feed",`+strings.TrimPrefix(isolatedMovementFactBody, "{")))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "source") {
		t.Fatalf("自报来源：err = %v, want 点名 source 的 ErrMalformedRequest", err)
	}
	_, err = intake.IntakeMovementFact(context.Background(), commandRequest(`{"fact":"f","kind":"TELEPORT"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("词表外种类：err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 立不起来的合成来源在构造时拒（同租户那一格）。
func TestNewIsolatedCommandIntakeRejectsBlankMovementSource(t *testing.T) {
	if _, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{Tenant: isolatedCommandTenant, MovementSource: " "}); err == nil {
		t.Fatal("空来源被接受")
	}
}

const handoverRegistrationBody = `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","releasedBy":"SYN-PARTY/hub-08",` +
	`"receivedBy":"SYN-PARTY/linehaul-08","verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/release-05",` +
	`"receivingEvidence":"SYN-EVIDENCE/receive-05","rule":"SYN-RULE/handover@v1","basis":"SYN-BASIS/handover-05","version":"v1",` +
	`"judgedAt":"2026-09-24T13:00:00+08:00","segment":"SYN-SEGMENT-08-05","plannedSegment":"SYN-PLANNED-08-05",` +
	`"segmentServiceAction":"LINEHAUL"}`

// Covers: HandoverIntake 契约「`Segment` 与 `PlannedSegment` 是命令的一部分，Intake 必须收」——交接判断的十三格逐字来自
// 载荷，判断时刻照 ADR-0023 不由服务端补。
func TestIsolatedCommandIntakeTranslatesHandoverRegistrationWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeHandoverRegistration(context.Background(), commandRequest(handoverRegistrationBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q", got, isolatedCommandTenant)
	}
	for name, pair := range map[string][2]string{
		"object":               {command.Object, "SYN-PARCEL-08-05"},
		"scope":                {command.Scope, "SYN-SCOPE/hub-dock-05"},
		"releasedBy":           {command.ReleasedBy, "SYN-PARTY/hub-08"},
		"receivedBy":           {command.ReceivedBy, "SYN-PARTY/linehaul-08"},
		"verdict":              {command.Verdict.String(), "HANDED_OVER"},
		"releasingEvidence":    {command.ReleasingEvidence, "SYN-EVIDENCE/release-05"},
		"receivingEvidence":    {command.ReceivingEvidence, "SYN-EVIDENCE/receive-05"},
		"rule":                 {command.Rule, "SYN-RULE/handover@v1"},
		"basis":                {command.Basis, "SYN-BASIS/handover-05"},
		"version":              {command.Version, "v1"},
		"segment":              {command.Segment, "SYN-SEGMENT-08-05"},
		"plannedSegment":       {command.PlannedSegment, "SYN-PLANNED-08-05"},
		"segmentServiceAction": {command.SegmentServiceAction, "LINEHAUL"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if want := time.Date(2026, 9, 24, 5, 0, 0, 0, time.UTC); !command.JudgedAt.Equal(want) {
		t.Fatalf("JudgedAt = %s, want %s", command.JudgedAt, want)
	}
}

// Covers: 交接结论取 domain.HandoverVerdict 的封闭词——词表外是用法错误（400）。
func TestIsolatedCommandIntakeRefusesAnUnknownHandoverVerdict(t *testing.T) {
	_, err := isolatedCommandIntakeForTest(t).IntakeHandoverRegistration(context.Background(),
		commandRequest(`{"object":"o","scope":"s","verdict":"MAYBE"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

const carrierPickupJudgmentBody = `{"object":"SYN-PARCEL-08-04","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SYN-SCAN-08-04",` +
	`"evidenceVersion":"v1","expressesControl":true,"occurredAt":"2026-09-24T11:00:00+08:00","carrierKind":"EXTERNAL_PARTY",` +
	`"carrierReference":"SYN-PARTY/carrier-08","segment":"SYN-SEGMENT-08-04"}`

// Covers: 判断口的线格式是本包既有的 CarrierPickupJudgmentPayload，隔离 Intake 只把注入的租户交进它的 Command——读法本体
// （expressesControl）、证据来源与业务发生时间逐字来自载荷。
func TestIsolatedCommandIntakeTranslatesCarrierPickupJudgmentWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeCarrierPickupJudgment(context.Background(), commandRequest(carrierPickupJudgmentBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q", got, isolatedCommandTenant)
	}
	if command.Object != "SYN-PARCEL-08-04" || command.Source.String() != "CARRIER_PICKUP_SCAN" ||
		command.EvidenceReference != "SYN-SCAN-08-04" || command.EvidenceVersion != "v1" || !command.ExpressesControl ||
		command.SubjectKind.String() != "EXTERNAL_PARTY" || command.SubjectReference != "SYN-PARTY/carrier-08" ||
		command.Segment != "SYN-SEGMENT-08-04" {
		t.Fatalf("command = %+v，与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 3, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
}

const offsitePickupAttemptBody = `{"sourceId":"SYN-DEVICE-08/attempt-01","task":"SYN-TASK-08-02","attempt":"SYN-ATTEMPT-08-02",` +
	`"executedBy":"SYN-COURIER-08","place":"SYN-PLACE/shipper-dock-02","plannedFrom":"2026-09-24T08:00:00+08:00",` +
	`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-02",` +
	`"rescheduledFrom":"SYN-ATTEMPT-08-00","segment":"SYN-SEGMENT-08-02","objects":[` +
	`{"object":"SYN-PARCEL-08-02","outcome":"PICKED_UP","control":"SYN-CONTROL/signed-02",` +
	`"occurredAt":"2026-09-24T08:45:00+08:00","plannedSegment":"SYN-PLANNED-08-02"},` +
	`{"object":"SYN-PARCEL-08-03","outcome":"GOODS_NOT_READY","basis":"SYN-BASIS/note-03","occurredAt":"2026-09-24T08:46:00+08:00"}]}`

// Covers: PickupAttemptIntake 契约「段引用两层都要收」——整次到访一个 segment、逐对象各自的 plannedSegment；事实身份
// sourceId 与各时刻照 ADR-0023 从载荷如实收，逐对象成败与依据逐字进命令，传输层不替它汇总。
func TestIsolatedCommandIntakeTranslatesOffsitePickupAttemptWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(offsitePickupAttemptBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q", got, isolatedCommandTenant)
	}
	for name, pair := range map[string][2]string{
		"sourceId":        {command.SourceID, "SYN-DEVICE-08/attempt-01"},
		"task":            {command.Task, "SYN-TASK-08-02"},
		"attempt":         {command.Attempt, "SYN-ATTEMPT-08-02"},
		"executedBy":      {command.ExecutedBy, "SYN-COURIER-08"},
		"place":           {command.Place, "SYN-PLACE/shipper-dock-02"},
		"evidence":        {command.Evidence, "SYN-EVIDENCE/visit-02"},
		"rescheduledFrom": {command.RescheduledFrom, "SYN-ATTEMPT-08-00"},
		"segment":         {command.Segment, "SYN-SEGMENT-08-02"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	for name, pair := range map[string][2]time.Time{
		"plannedFrom": {command.PlannedFrom, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)},
		"plannedTo":   {command.PlannedTo, time.Date(2026, 9, 24, 2, 0, 0, 0, time.UTC)},
		"arrivedAt":   {command.ArrivedAt, time.Date(2026, 9, 24, 0, 40, 0, 0, time.UTC)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
	if len(command.Objects) != 2 {
		t.Fatalf("objects = %d 项, want 2——逐对象分别成败，不许合并", len(command.Objects))
	}
	picked, notReady := command.Objects[0], command.Objects[1]
	if picked.Object.String() != "SYN-PARCEL-08-02" || picked.Outcome.String() != "PICKED_UP" ||
		picked.Basis.String() != "" || picked.Control.String() != "SYN-CONTROL/signed-02" ||
		picked.PlannedSegment != "SYN-PLANNED-08-02" ||
		!picked.OccurredAt.Equal(time.Date(2026, 9, 24, 0, 45, 0, 0, time.UTC)) {
		t.Fatalf("第一件 = %+v，与载荷不符", picked)
	}
	if notReady.Object.String() != "SYN-PARCEL-08-03" || notReady.Outcome.String() != "GOODS_NOT_READY" ||
		notReady.Basis.String() != "SYN-BASIS/note-03" || notReady.Control.String() != "" || notReady.PlannedSegment != "" {
		t.Fatalf("第二件 = %+v，与载荷不符——没给的控制依据与计划段不该被补上", notReady)
	}
}

// Covers: 对象成败取封闭词表——词表外的词是用法错误（400），不留到编排去答成`未受理`；空着则交给编排判。
func TestIsolatedCommandIntakeRefusesAnUnknownAttemptObjectOutcome(t *testing.T) {
	_, err := isolatedCommandIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(
		`{"sourceId":"s","attempt":"a","objects":[{"object":"o","outcome":"LOST","occurredAt":"2026-09-24T08:45:00+08:00"}]}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

const offsitePickupRegistrationBody = `{"object":"SYN-PARCEL-08-01","task":"SYN-TASK-08-01","attempt":"SYN-ATTEMPT-08-01",` +
	`"place":"SYN-PLACE/shipper-dock-01","control":"SYN-CONTROL/signed-pickup-01","executedBy":"SYN-COURIER-08",` +
	`"occurredAt":"2026-09-24T08:30:00+08:00","segment":"SYN-SEGMENT-08-01","plannedSegment":"SYN-PLANNED-08-01",` +
	`"segmentServiceAction":"OFFSITE_PICKUP"}`

// Covers: PickupRegistrationIntake 契约「租户只能来自认证结果、事实内容从请求体收」+ `Segment`/`PlannedSegment` 必须收——
// 隔离形态的认证结果是开关值；十格逐字来自载荷，请求头与查询串里的自报一律无视。
func TestIsolatedCommandIntakeTranslatesOffsitePickupRegistrationWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(), commandRequest(offsitePickupRegistrationBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != isolatedCommandTenant {
		t.Fatalf("TenantID = %q, want %q（注入值，不是请求里的自报）", got, isolatedCommandTenant)
	}
	for name, pair := range map[string][2]string{
		"object":               {command.Object, "SYN-PARCEL-08-01"},
		"task":                 {command.Task, "SYN-TASK-08-01"},
		"attempt":              {command.Attempt, "SYN-ATTEMPT-08-01"},
		"place":                {command.Place, "SYN-PLACE/shipper-dock-01"},
		"control":              {command.Control, "SYN-CONTROL/signed-pickup-01"},
		"executedBy":           {command.ExecutedBy, "SYN-COURIER-08"},
		"segment":              {command.Segment, "SYN-SEGMENT-08-01"},
		"plannedSegment":       {command.PlannedSegment, "SYN-PLANNED-08-01"},
		"segmentServiceAction": {command.SegmentServiceAction, "OFFSITE_PICKUP"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if want := time.Date(2026, 9, 24, 0, 30, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s（设备记录的业务时间，不是服务端时钟）", command.OccurredAt, want)
	}
}

// Covers: 时刻缺席交给编排答`未受理`（200，形成了的业务答案），解不出才是坏报文（400）——同本包判断载荷的分格。
func TestIsolatedCommandIntakeLeavesAnAbsentPickupInstantToTheOrchestration(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01"}`))
	if err != nil {
		t.Fatalf("缺时刻被当成坏报文：%v", err)
	}
	if !command.OccurredAt.IsZero() {
		t.Fatalf("OccurredAt = %s, want 零值——缺席不是服务端补一个时钟值的理由", command.OccurredAt)
	}
}

// Covers: 载荷里带 tenantId 即拒（MALFORMED_REQUEST），值与开关相同也拒——每一口都一样。
func TestIsolatedCommandIntakeRefusesSelfReportedTenantOnEveryAdmittedLine(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	for pattern, line := range isolatedLines {
		t.Run(pattern, func(t *testing.T) {
			body := `{"tenantId":"` + isolatedCommandTenant + `",` + strings.TrimPrefix(line.valid, "{")
			err := line.intake(intake, commandRequest(body))
			if !errors.Is(err, tfhttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
			if !strings.Contains(err.Error(), "tenantId") {
				t.Fatalf("拒绝理由没点名 tenantId：%v", err)
			}
		})
	}
}

// Covers: 一份载荷只许一个文档、只许认识的键——尾随的第二个 JSON 值与未知键都拒，放过它们就是无声丢掉一段输入。
func TestIsolatedCommandIntakeRefusesTrailingContentAndUnknownKeysOnEveryAdmittedLine(t *testing.T) {
	intake := isolatedCommandIntakeForTest(t)
	for pattern, line := range isolatedLines {
		t.Run(pattern, func(t *testing.T) {
			for name, body := range map[string]string{
				"尾随内容":    line.valid + ` {"x":1}`,
				"未知键":     `{"unknownKey":"x",` + strings.TrimPrefix(line.valid, "{"),
				"不是 JSON": "!!not-json!!",
				"空载荷":     "",
			} {
				if err := line.intake(intake, commandRequest(body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
					t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
				}
			}
		})
	}
}

// Covers: 时刻给了却解不出是坏报文（400）。
func TestIsolatedCommandIntakeRefusesAnUnparsablePickupInstant(t *testing.T) {
	_, err := isolatedCommandIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01","occurredAt":"昨天"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 立不起来的注入在构造时拒，不等第一个请求（同隔离读 Intake 的纪律）。
func TestNewIsolatedCommandIntakeRejectsBlankTenant(t *testing.T) {
	if _, err := tfhttp.NewIsolatedCommandIntake(tfhttp.IsolatedCommandIntakeDeps{Tenant: "   ", MovementSource: isolatedCommandMovementSource}); err == nil {
		t.Fatal("空租户被接受")
	}
}

// Covers: ADR-0091 Consequences「命令面按端点逐口放行，不是一次全开」——本类型只实现放行了的口；更正口、登记册配置口与
// 查阅行在类型上就装不进去。每放一口，该口的断言从「装不进」挪到「该已放行」。
func TestIsolatedCommandIntakeServesOnlyAdmittedLines(t *testing.T) {
	var intake any = isolatedCommandIntakeForTest(t)
	if _, ok := intake.(tfhttp.PickupRegistrationIntake); !ok {
		t.Fatal("场外揽收登记口该已放行")
	}
	if _, ok := intake.(tfhttp.PickupAttemptIntake); !ok {
		t.Fatal("场外揽收尝试口该已放行")
	}
	if _, ok := intake.(tfhttp.CarrierPickupJudgmentIntake); !ok {
		t.Fatal("承运商首次有效收寄判断口该已放行")
	}
	if _, ok := intake.(tfhttp.HandoverRegistrationIntake); !ok {
		t.Fatal("交接首登口该已放行")
	}
	if _, ok := intake.(tfhttp.MovementFactIntake); !ok {
		t.Fatal("移动事实口该已放行")
	}
	if _, ok := intake.(tfhttp.DispatchTaskIntake); !ok {
		t.Fatal("派送任务登记口该已放行")
	}
	for name, refused := range map[string]bool{
		"揽收更正口（同族未列）":      isA[tfhttp.PickupCorrectionIntake](intake),
		"交接更正口（同族未列）":      isA[tfhttp.HandoverCorrectionIntake](intake),
		"交付更正口（同族未列）":      isA[tfhttp.DeliveryProofCorrectionIntake](intake),
		"装载分配口（票面未列）":      isA[tfhttp.LoadAssignmentIntake](intake),
		"终止参与口（票面未列）":      isA[tfhttp.ParticipationTerminationIntake](intake),
		"承运凭证登记口（04 那族）":   isA[tfhttp.CredentialIntake](intake),
		"有效时间规则登记口（04 那族）": isA[tfhttp.EffectiveTimeRuleIntake](intake),
		"总单登记口（04 那族）":     isA[tfhttp.MasterDocumentIntake](intake),
		"查阅行（归隔离读 Intake）": isA[tfhttp.CatalogueQueryIntake](intake),
	} {
		if refused {
			t.Fatalf("%s 不在放行名单，隔离命令 Intake 不该装得进", name)
		}
	}
}

func isA[Interface any](value any) bool {
	_, ok := value.(Interface)
	return ok
}
