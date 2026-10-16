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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证作业事实各口的操作者渠道 Intake（ADR-0149 决定二；票 operator-channel/10）的传输面：租户只来自认证结果、
// 设备与作业范围只从三个头来且种类只认 OFFSITE、先认证后解载荷、事实内容与发生时间照 ADR-0023 从载荷如实收、自报租户
// 即拒、形状错即拒、运营决定与登记写面的口在类型上装不进去。这组译装用例原挂在隔离命令 Intake 上，随它退场迁来（票 22；
// ADR-0150 决定三），期望值不变。期望值取自载荷字面量而不是重算：这里证的是「译装没换字」。成形与否（对象引用立不立得住、
// 控制证据缺不缺）不在这里证——那是编排答`未受理`的格，本 Intake 不预判。

const (
	workFactTenant         = "SYN-TENANT-01"
	workFactMovementSource = "SELF_OPERATED"
)

// workFactAuthenticatorFake 替身认证方：记下被问的设备与作业范围；带了令牌答合成租户，没带答令牌不过，err 非空时答它。
type workFactAuthenticatorFake struct {
	err   error
	asked *[]string
}

func (fake workFactAuthenticatorFake) AuthenticateWorkFact(_ context.Context, token, device, kind, reference string) (domain.TenantID, error) {
	if fake.asked != nil {
		*fake.asked = append(*fake.asked, device+"|"+kind+"|"+reference)
	}
	if token == "" {
		return domain.TenantID{}, tfhttp.ErrOperatorCredentialRejected
	}
	if fake.err != nil {
		return domain.TenantID{}, fake.err
	}
	return domain.NewTenantID(workFactTenant)
}

func workFactIntakeForTest(t *testing.T) *tfhttp.WorkFactCommandIntake {
	t.Helper()
	return workFactIntakeWith(t, workFactAuthenticatorFake{})
}

func workFactIntakeWith(t *testing.T, authenticator tfhttp.WorkFactAuthenticator) *tfhttp.WorkFactCommandIntake {
	t.Helper()
	intake, err := tfhttp.NewWorkFactCommandIntake(authenticator, workFactMovementSource)
	if err != nil {
		t.Fatalf("构造作业事实 Intake：%v", err)
	}
	return intake
}

// commandRequest 带一份合规的作业事实出示（令牌、在用设备、OFFSITE 作业范围），另在头与查询串里夹带自报租户——它们一律
// 无视，租户只来自认证结果。
func commandRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("Parcel-Device-Id", "SYN-DEVICE-OFFSITE")
	request.Header.Set("Parcel-Work-Scope-Kind", "OFFSITE")
	request.Header.Set("Parcel-Work-Scope-Reference", "SYN-OFFSITE-01")
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	request.URL.RawQuery = "tenant=TENANT-9"
	return request
}

// workFactLine 是作业事实的一口：怎么调它的 Intake、一份合规载荷。自报租户、尾随内容与类型锁三条对每一口都成立，
// 表驱动逐口跑。
type workFactLine struct {
	intake func(*tfhttp.WorkFactCommandIntake, *http.Request) error
	valid  string
}

var workFactLines = map[string]workFactLine{
	"/transport-fulfillment/offsite-pickups": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupRegistration(context.Background(), request)
			return err
		},
		valid: offsitePickupRegistrationBody,
	},
	"/transport-fulfillment/offsite-pickup-attempts": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupAttempt(context.Background(), request)
			return err
		},
		valid: offsitePickupAttemptBody,
	},
	"/transport-fulfillment/handovers": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeHandoverRegistration(context.Background(), request)
			return err
		},
		valid: handoverRegistrationBody,
	},
	"/transport-fulfillment/movement-facts": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeMovementFact(context.Background(), request)
			return err
		},
		valid: workFactMovementFactBody,
	},
	"/transport-fulfillment-delivery-dispatch-triggers": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeDeliveryDispatchTrigger(context.Background(), request)
			return err
		},
		valid: workFactDeliveryDispatchTriggerBody,
	},
	"/transport-fulfillment/deliveries": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeRegistration(context.Background(), request)
			return err
		},
		valid: workFactEffectiveDeliveryBody,
	},
	"/transport-fulfillment/delivery-attempts": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeDeliveryAttempt(context.Background(), request)
			return err
		},
		valid: workFactDeliveryAttemptBody,
	},
}

const workFactEffectiveDeliveryBody = `{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","method":"HANDED_TO_RECIPIENT",` +
	`"recipient":"SYN-RECIPIENT/consignee-10","proof":"SYN-POD/signature-10"}`

// Covers: DeliveryIntake 契约「事实内容必须从请求体收（ADR-0023）：对象、尝试、POD 证据引用都是派送端记录的事实」——
// 各格逐字来自载荷，租户来自认证结果。
func TestWorkFactCommandIntakeTranslatesEffectiveDeliveryWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeRegistration(context.Background(), commandRequest(workFactEffectiveDeliveryBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != workFactTenant || command.Attempt != "SYN-ATTEMPT-08-10" || command.Object != "SYN-PARCEL-08-10" ||
		command.Method != "HANDED_TO_RECIPIENT" || command.Recipient != "SYN-RECIPIENT/consignee-10" || command.Proof != "SYN-POD/signature-10" {
		t.Fatalf("command = %+v，与认证结果与载荷不符", command)
	}
}

const workFactDeliveryDispatchTriggerBody = `{"segment":"SYN-SEGMENT-08-09","object":"SYN-PARCEL-08-09","occurredAt":"2026-09-25T07:00:00+08:00"}`

// Covers: DeliveryDispatchTriggerIntake 契约「租户从信封给，段、对象与这一拍的业务时间从请求收」——请求里没有任何地点、
// 时间窗或条件（ADR-0114 决定三），载荷形状只有段、对象与业务时间。
func TestWorkFactCommandIntakeTranslatesDeliveryDispatchTriggerWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeDeliveryDispatchTrigger(context.Background(), commandRequest(workFactDeliveryDispatchTriggerBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != workFactTenant || command.Segment != "SYN-SEGMENT-08-09" || command.Object != "SYN-PARCEL-08-09" {
		t.Fatalf("command = %+v，与认证结果与载荷不符", command)
	}
	if want := time.Date(2026, 9, 24, 23, 0, 0, 0, time.UTC); !command.OccurredAt.Equal(want) {
		t.Fatalf("OccurredAt = %s, want %s", command.OccurredAt, want)
	}
	_, err = workFactIntakeForTest(t).IntakeDeliveryDispatchTrigger(context.Background(),
		commandRequest(`{"segment":"s","object":"o","place":"SYN-PLACE/x"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("带地点的触发：err = %v, want ErrMalformedRequest——地点由执行器向所有者取，调用方给不了也不该给", err)
	}
}

const workFactMovementFactBody = `{"fact":"SYN-MOVE-08-06","schedule":"SYN-SCHEDULE/linehaul-06","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",` +
	`"version":"v1","occurredAt":"2026-09-24T14:00:00+08:00","gateRequired":true,"gateClearance":"SYN-GATE/release-06"}`

// Covers: MovementFactIntake 契约「这个口只收自营执行方的事实；『自营还是外部』由 Intake 的认证结果说」——作业事实的认证
// 结果是这一面构造时给定的来源（自营），事实本体与门禁两格（gateRequired / gateClearance 必须收）逐字来自载荷。
func TestWorkFactCommandIntakeTranslatesMovementFactWithTheAuthenticatedTenantAndSource(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeMovementFact(context.Background(), commandRequest(workFactMovementFactBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q", got, workFactTenant)
	}
	if command.Source != workFactMovementSource {
		t.Fatalf("Source = %q, want %q（作业事实这一面说自营——外部轨迹不从这个口进，来源不由请求自报）", command.Source, workFactMovementSource)
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

// Covers: 载荷里带 source 即拒——来源只来自作业事实这一面，放它进来就是让外部轨迹冒充自营事实从这个口进。事实种类取封闭词，
// 词表外是用法错误（400）。
func TestWorkFactCommandIntakeRefusesSelfReportedMovementSourceAndUnknownKinds(t *testing.T) {
	intake := workFactIntakeForTest(t)
	_, err := intake.IntakeMovementFact(context.Background(), commandRequest(`{"source":"SYN-SOURCE/carrier-feed",`+strings.TrimPrefix(workFactMovementFactBody, "{")))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), "source") {
		t.Fatalf("自报来源：err = %v, want 点名 source 的 ErrMalformedRequest", err)
	}
	_, err = intake.IntakeMovementFact(context.Background(), commandRequest(`{"fact":"f","kind":"TELEPORT"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("词表外种类：err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 立不起来的来源格与缺席的认证方在构造时拒，不等第一个请求。
func TestNewWorkFactCommandIntakeRejectsABlankMovementSourceOrNoAuthenticator(t *testing.T) {
	if _, err := tfhttp.NewWorkFactCommandIntake(workFactAuthenticatorFake{}, " "); err == nil {
		t.Fatal("空来源被接受")
	}
	if _, err := tfhttp.NewWorkFactCommandIntake(nil, workFactMovementSource); err == nil {
		t.Fatal("没有认证方也装得起来")
	}
}

// Covers: 作业事实的设备与作业范围只从三个头来，缺一即坏报文；这一面只认 OFFSITE——认了 NODE，登在节点上的设备就能拿
// 节点授予打揽收、交接与交付。先认证、后解载荷：认证方的答复原样交回，令牌不过的调用方看不到载荷校验的结果。
func TestWorkFactCommandIntakeTakesDeviceAndScopeFromHeadersAndAuthenticatesFirst(t *testing.T) {
	var asked []string
	intake := workFactIntakeWith(t, workFactAuthenticatorFake{asked: &asked})
	if _, err := intake.IntakeRegistration(context.Background(), commandRequest(workFactEffectiveDeliveryBody)); err != nil {
		t.Fatalf("合规出示被拒：%v", err)
	}
	if len(asked) != 1 || asked[0] != "SYN-DEVICE-OFFSITE|OFFSITE|SYN-OFFSITE-01" {
		t.Fatalf("认证方被问 %v，want 三个头里的设备与作业范围", asked)
	}
	for name, mutate := range map[string]func(*http.Request){
		"缺设备":     func(request *http.Request) { request.Header.Del("Parcel-Device-Id") },
		"缺作业范围种类": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Kind") },
		"缺作业范围引用": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Reference") },
		"节点作业范围":  func(request *http.Request) { request.Header.Set("Parcel-Work-Scope-Kind", "NODE") },
	} {
		request := commandRequest(workFactEffectiveDeliveryBody)
		mutate(request)
		if _, err := intake.IntakeRegistration(context.Background(), request); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
	refused := workFactIntakeWith(t, workFactAuthenticatorFake{err: tfhttp.ErrDeviceNotInUse})
	if _, err := refused.IntakeRegistration(context.Background(), commandRequest("!!not-json!!")); !errors.Is(err, tfhttp.ErrDeviceNotInUse) {
		t.Fatalf("设备不在用：err = %v, want 认证方的答复，而不是载荷校验的结果", err)
	}
}

const handoverRegistrationBody = `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","releasedBy":"SYN-PARTY/hub-08",` +
	`"receivedBy":"SYN-PARTY/linehaul-08","verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/release-05",` +
	`"receivingEvidence":"SYN-EVIDENCE/receive-05","rule":"SYN-RULE/handover@v1","basis":"SYN-BASIS/handover-05","version":"v1",` +
	`"judgedAt":"2026-09-24T13:00:00+08:00","segment":"SYN-SEGMENT-08-05","plannedSegment":"SYN-PLANNED-08-05",` +
	`"segmentServiceAction":"LINEHAUL"}`

// Covers: HandoverIntake 契约「`Segment` 与 `PlannedSegment` 是命令的一部分，Intake 必须收」——交接判断各格逐字来自
// 载荷，判断时刻照 ADR-0023 不由服务端补。
func TestWorkFactCommandIntakeTranslatesHandoverRegistrationWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeHandoverRegistration(context.Background(), commandRequest(handoverRegistrationBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q", got, workFactTenant)
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
func TestWorkFactCommandIntakeRefusesAnUnknownHandoverVerdict(t *testing.T) {
	_, err := workFactIntakeForTest(t).IntakeHandoverRegistration(context.Background(),
		commandRequest(`{"object":"o","scope":"s","verdict":"MAYBE"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
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
func TestWorkFactCommandIntakeTranslatesOffsitePickupAttemptWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(offsitePickupAttemptBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q", got, workFactTenant)
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
func TestWorkFactCommandIntakeRefusesAnUnknownAttemptObjectOutcome(t *testing.T) {
	_, err := workFactIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(
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
// 认证结果是作业事实信封；各格逐字来自载荷，请求头与查询串里的自报一律无视。
func TestWorkFactCommandIntakeTranslatesOffsitePickupRegistrationWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakePickupRegistration(context.Background(), commandRequest(offsitePickupRegistrationBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q（认证结果，不是请求里的自报）", got, workFactTenant)
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
func TestWorkFactCommandIntakeLeavesAnAbsentPickupInstantToTheOrchestration(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01"}`))
	if err != nil {
		t.Fatalf("缺时刻被当成坏报文：%v", err)
	}
	if !command.OccurredAt.IsZero() {
		t.Fatalf("OccurredAt = %s, want 零值——缺席不是服务端补一个时钟值的理由", command.OccurredAt)
	}
}

// Covers: 载荷里带 tenantId 即拒（MALFORMED_REQUEST），值与开关相同也拒——每一口都一样。
func TestWorkFactCommandIntakeRefusesSelfReportedTenantOnEveryLine(t *testing.T) {
	intake := workFactIntakeForTest(t)
	for pattern, line := range workFactLines {
		t.Run(pattern, func(t *testing.T) {
			body := `{"tenantId":"` + workFactTenant + `",` + strings.TrimPrefix(line.valid, "{")
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
func TestWorkFactCommandIntakeRefusesTrailingContentAndUnknownKeysOnEveryLine(t *testing.T) {
	intake := workFactIntakeForTest(t)
	for pattern, line := range workFactLines {
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
func TestWorkFactCommandIntakeRefusesAnUnparsablePickupInstant(t *testing.T) {
	_, err := workFactIntakeForTest(t).IntakePickupRegistration(context.Background(),
		commandRequest(`{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01","occurredAt":"昨天"}`))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: ADR-0149 决定二——作业事实这一面实现首登与更正各口；运营决定（ADR-0151）、登记写面（ADR-0100 决定四）与查阅行
// 各有自己的能力面，在类型上就装不进本类型。
func TestWorkFactCommandIntakeServesOnlyWorkFactLines(t *testing.T) {
	var intake any = workFactIntakeForTest(t)
	for name, served := range map[string]bool{
		"场外揽收登记口": isA[tfhttp.PickupRegistrationIntake](intake),
		"场外揽收尝试口": isA[tfhttp.PickupAttemptIntake](intake),
		"揽收更正口":   isA[tfhttp.PickupCorrectionIntake](intake),
		"交接首登口":   isA[tfhttp.HandoverRegistrationIntake](intake),
		"交接更正口":   isA[tfhttp.HandoverCorrectionIntake](intake),
		"移动事实口":   isA[tfhttp.MovementFactIntake](intake),
		"派送发起口":   isA[tfhttp.DeliveryDispatchTriggerIntake](intake),
		"交付生效首登口": isA[tfhttp.DeliveryRegistrationIntake](intake),
		"交付更正口":   isA[tfhttp.DeliveryProofCorrectionIntake](intake),
		"派送尝试登记口": isA[tfhttp.DeliveryAttemptIntake](intake),
	} {
		if !served {
			t.Fatalf("%s 是作业事实，这一面该实现它", name)
		}
	}
	for name, refused := range map[string]bool{
		"关段口（运营决定）":       isA[tfhttp.SegmentClosureIntake](intake),
		"建派送任务口（运营决定）":    isA[tfhttp.DispatchTaskIntake](intake),
		"有效时间判断口（运营决定）":   isA[tfhttp.EffectiveTimeJudgmentIntake](intake),
		"承运商收寄判断口（运营决定）":  isA[tfhttp.CarrierPickupJudgmentIntake](intake),
		"装载分配口（运营决定）":     isA[tfhttp.LoadAssignmentIntake](intake),
		"终止参与口（运营决定）":     isA[tfhttp.ParticipationTerminationIntake](intake),
		"承运凭证登记口（登记写面）":   isA[tfhttp.CredentialIntake](intake),
		"有效时间规则登记口（登记写面）": isA[tfhttp.EffectiveTimeRuleIntake](intake),
		"总单登记口（登记写面）":     isA[tfhttp.MasterDocumentIntake](intake),
		"查阅行（查阅读）":        isA[tfhttp.CatalogueQueryIntake](intake),
	} {
		if refused {
			t.Fatalf("%s 不归作业事实，这一面不该装得进", name)
		}
	}
}

func isA[Interface any](value any) bool {
	_, ok := value.(Interface)
	return ok
}
