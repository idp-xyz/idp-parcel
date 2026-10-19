package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证作业事实各口的操作者渠道 Intake（ADR-0149 决定二；票 operator-channel/10）的传输面：租户只来自认证结果、
// 设备与作业范围只从请求头来（读法见 WorkFactCommandIntake.tenantFrom）且种类只认 OFFSITE、先认证后解载荷、事实内容
// 与发生时间照 ADR-0023 从载荷如实收、自报租户即拒、形状错即拒、运营决定与登记写面的口在类型上装不进去。期望值取自
// 载荷字面量而不是重算：这里证的是「译装没换字」。成形与否（对象引用立不立得住、控制证据缺不缺）不在这里证——那是
// 编排答`未受理`的格，本 Intake 不预判。

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

// workFactLine 是作业事实的一口：怎么调它的 Intake、一份合规载荷。自报租户、封闭解码与先认证后解载荷对每一口都成立，
// 带 OnEveryLine 的用例逐口跑这张表；类型锁 TestWorkFactCommandIntakeServesOnlyWorkFactLines 按接口另列，加口时两处都要加。
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
	"/transport-fulfillment/offsite-pickup-corrections": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakePickupCorrection(context.Background(), request)
			return err
		},
		valid: workFactPickupCorrectionBody,
	},
	"/transport-fulfillment/handovers": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeHandoverRegistration(context.Background(), request)
			return err
		},
		valid: handoverRegistrationBody,
	},
	"/transport-fulfillment/handover-corrections": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeHandoverCorrection(context.Background(), request)
			return err
		},
		valid: workFactHandoverCorrectionBody,
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
	"/transport-fulfillment/delivery-proof-corrections": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeCorrection(context.Background(), request)
			return err
		},
		valid: workFactDeliveryProofCorrectionBody,
	},
	"/transport-fulfillment/delivery-attempts": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeDeliveryAttempt(context.Background(), request)
			return err
		},
		valid: workFactDeliveryAttemptBody,
	},
	"/transport-fulfillment/departure-gate-previews": {
		intake: func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) error {
			_, err := intake.IntakeDepartureGatePreview(context.Background(), request)
			return err
		},
		valid: workFactDepartureGatePreviewBody,
	},
}

const workFactDepartureGatePreviewBody = `{"schedule":"SYN-SCHEDULE/linehaul-06","segment":"SYN-SEGMENT-08-06",` +
	`"departsAt":"2026-09-24T14:00:00+08:00"}`

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

const workFactDeliveryProofCorrectionBody = `{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10",` +
	`"predecessorVersion":"SYN-DRV-08-10-1","newProof":"SYN-POD/signature-10b","correctedAt":"2026-09-06T16:00:00+08:00"}`

// Covers: 票 operator-channel/41 裁定 (a)——POD 更正的线格式指名前版，与揽收更正同形；前版逐字来自载荷，是编排认
// 「同一次更正」的锚，租户来自认证结果。
func TestWorkFactCommandIntakeTranslatesDeliveryProofCorrectionWithItsPredecessor(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeCorrection(context.Background(), commandRequest(workFactDeliveryProofCorrectionBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != workFactTenant || command.Attempt != "SYN-ATTEMPT-08-10" || command.Object != "SYN-PARCEL-08-10" ||
		command.PredecessorVersion != "SYN-DRV-08-10-1" || command.NewProof != "SYN-POD/signature-10b" ||
		!command.CorrectedAt.Equal(time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)) {
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
// 结果是这一面构造时给定的来源（自营），事实本体与放行凭据逐字来自载荷；退役键 gateRequired 照收照弃，不让封闭解码答 400。
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
		command.Location != "SYN-NODE-SHA-HUB" || command.Version != "v1" || command.GateClearance != "SYN-GATE/release-06" {
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

// Covers: 先认证、后解载荷，逐口成立——认证方被问的是请求头里那份设备与作业范围，它的答复原样交回，令牌不过的调用方
// 看不到载荷校验的结果；令牌原样转交，Intake 不替调用方补。请求头那几格在问认证方之前就判，即票 10 的判断项
// 「三头缺一答 400 `MALFORMED_REQUEST`，不占渠道那几格」
// 「种类不对答 400，发生在铸造之前」。
// 这一面只认 OFFSITE——认了 NODE，登在节点上的设备就能拿节点授予打揽收、交接与交付。
func TestWorkFactCommandIntakeAuthenticatesBeforeDecodingOnEveryLine(t *testing.T) {
	for pattern, line := range workFactLines {
		t.Run(pattern, func(t *testing.T) {
			var asked []string
			intake := workFactIntakeWith(t, workFactAuthenticatorFake{asked: &asked})
			if err := line.intake(intake, commandRequest(line.valid)); err != nil {
				t.Fatalf("合规出示被拒：%v", err)
			}
			if len(asked) != 1 || asked[0] != "SYN-DEVICE-OFFSITE|OFFSITE|SYN-OFFSITE-01" {
				t.Fatalf("认证方被问 %v，want 请求头里的设备与作业范围", asked)
			}

			refused := workFactIntakeWith(t, workFactAuthenticatorFake{err: tfhttp.ErrDeviceNotInUse})
			if err := line.intake(refused, commandRequest("!!not-json!!")); !errors.Is(err, tfhttp.ErrDeviceNotInUse) {
				t.Fatalf("设备不在用：err = %v, want 认证方的答复，而不是载荷校验的结果", err)
			}
			anonymous := commandRequest("!!not-json!!")
			anonymous.Header.Del("Authorization")
			if err := line.intake(intake, anonymous); !errors.Is(err, tfhttp.ErrOperatorCredentialRejected) {
				t.Fatalf("没带令牌：err = %v, want ErrOperatorCredentialRejected，而不是载荷校验的结果", err)
			}

			for name, mutate := range map[string]func(*http.Request){
				"缺设备":     func(request *http.Request) { request.Header.Del("Parcel-Device-Id") },
				"缺作业范围种类": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Kind") },
				"缺作业范围引用": func(request *http.Request) { request.Header.Del("Parcel-Work-Scope-Reference") },
				"节点作业范围":  func(request *http.Request) { request.Header.Set("Parcel-Work-Scope-Kind", "NODE") },
			} {
				asked = nil
				request := commandRequest(line.valid)
				mutate(request)
				if err := line.intake(intake, request); !errors.Is(err, tfhttp.ErrMalformedRequest) {
					t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
				}
				if len(asked) != 0 {
					t.Fatalf("%s：认证方被问 %v，want 一次也不问", name, asked)
				}
			}
		})
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

// Covers: 载荷里带 tenantId 即拒（MALFORMED_REQUEST），值与认证出的租户相同也拒——每一口都一样。
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

// Covers: 作业事实这一面实现首登各口（ADR-0149 决定二）与更正各口（随原事实归族，ADR-0151 决定四）；运营决定（ADR-0151
// 决定一）、登记写面（ADR-0100 决定四）与查阅行各有自己的能力面，在类型上就装不进本类型。
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
		"出发前问询口":  isA[tfhttp.DepartureGatePreviewIntake](intake),
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

const workFactPickupCorrectionBody = `{"object":"SYN-PARCEL-08-01","attempt":"SYN-ATTEMPT-08-01","predecessorVersion":"SYN-PICKUP-VERSION-08-01",` +
	`"place":"SYN-PLACE/shipper-dock-01b","control":"SYN-CONTROL/signed-pickup-01b","executedBy":"SYN-COURIER-08b",` +
	`"occurredAt":"2026-09-24T08:35:00+08:00","correctedAt":"2026-09-24T18:00:00+08:00"}`

const workFactHandoverCorrectionBody = `{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","predecessorVersion":"v1",` +
	`"verdict":"REFUSED","releasingEvidence":"SYN-EVIDENCE/release-05b","receivingEvidence":"SYN-EVIDENCE/receive-05b",` +
	`"rule":"SYN-RULE/handover@v2","basis":"SYN-BASIS/refusal-05","newVersion":"v2","correctedAt":"2026-09-24T15:00:00+08:00"}`

// Covers: PickupCorrectionIntake——指名的前版与更正后的地点、控制证据、执行方、发生时刻逐字来自载荷，更正时刻照 ADR-0023
// 从载荷收，租户来自认证结果。
func TestWorkFactCommandIntakeTranslatesPickupCorrectionWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakePickupCorrection(context.Background(), commandRequest(workFactPickupCorrectionBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q", got, workFactTenant)
	}
	for name, pair := range map[string][2]string{
		"object":             {command.Object, "SYN-PARCEL-08-01"},
		"attempt":            {command.Attempt, "SYN-ATTEMPT-08-01"},
		"predecessorVersion": {command.PredecessorVersion, "SYN-PICKUP-VERSION-08-01"},
		"place":              {command.Place, "SYN-PLACE/shipper-dock-01b"},
		"control":            {command.Control, "SYN-CONTROL/signed-pickup-01b"},
		"executedBy":         {command.ExecutedBy, "SYN-COURIER-08b"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	for name, pair := range map[string][2]time.Time{
		"occurredAt":  {command.OccurredAt, time.Date(2026, 9, 24, 0, 35, 0, 0, time.UTC)},
		"correctedAt": {command.CorrectedAt, time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)},
	} {
		if !pair[0].Equal(pair[1]) {
			t.Fatalf("%s = %s, want %s", name, pair[0], pair[1])
		}
	}
}

// Covers: HandoverCorrectionIntake——指名的前版、新版本号与更正后的结论、证据、规则、依据逐字来自载荷，更正时刻照 ADR-0023
// 从载荷收，租户来自认证结果。
func TestWorkFactCommandIntakeTranslatesHandoverCorrectionWithTheAuthenticatedTenant(t *testing.T) {
	command, err := workFactIntakeForTest(t).IntakeHandoverCorrection(context.Background(), commandRequest(workFactHandoverCorrectionBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := command.TenantID.String(); got != workFactTenant {
		t.Fatalf("TenantID = %q, want %q", got, workFactTenant)
	}
	for name, pair := range map[string][2]string{
		"object":             {command.Object, "SYN-PARCEL-08-05"},
		"scope":              {command.Scope, "SYN-SCOPE/hub-dock-05"},
		"predecessorVersion": {command.PredecessorVersion, "v1"},
		"verdict":            {command.Verdict.String(), "REFUSED"},
		"releasingEvidence":  {command.ReleasingEvidence, "SYN-EVIDENCE/release-05b"},
		"receivingEvidence":  {command.ReceivingEvidence, "SYN-EVIDENCE/receive-05b"},
		"rule":               {command.Rule, "SYN-RULE/handover@v2"},
		"basis":              {command.Basis, "SYN-BASIS/refusal-05"},
		"newVersion":         {command.NewVersion, "v2"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if want := time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC); !command.CorrectedAt.Equal(want) {
		t.Fatalf("CorrectedAt = %s, want %s", command.CorrectedAt, want)
	}
}

// bodyWith 交回合规载荷改了一格的副本：value 为 nil 时删掉那个键，否则那一格换成 value。
func bodyWith(t *testing.T, body, key string, value any) string {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("合规载荷解不开：%v", err)
	}
	if value == nil {
		delete(fields, key)
	} else {
		fields[key] = value
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("改写载荷：%v", err)
	}
	return string(encoded)
}

// correctionInstant 是更正载荷里的一个时刻格：哪一口、哪个键、Intake 交出的命令里对应哪一格，以及合规载荷里那个值
// 解出来应是哪一刻。
type correctionInstant struct {
	body string
	key  string
	read func(*tfhttp.WorkFactCommandIntake, *http.Request) (time.Time, error)
	want time.Time
}

var correctionInstants = map[string]correctionInstant{
	"揽收更正 correctedAt": {workFactPickupCorrectionBody, "correctedAt",
		func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) (time.Time, error) {
			command, err := intake.IntakePickupCorrection(context.Background(), request)
			return command.CorrectedAt, err
		}, time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)},
	"揽收更正 occurredAt": {workFactPickupCorrectionBody, "occurredAt",
		func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) (time.Time, error) {
			command, err := intake.IntakePickupCorrection(context.Background(), request)
			return command.OccurredAt, err
		}, time.Date(2026, 9, 24, 0, 35, 0, 0, time.UTC)},
	"交接更正 correctedAt": {workFactHandoverCorrectionBody, "correctedAt",
		func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) (time.Time, error) {
			command, err := intake.IntakeHandoverCorrection(context.Background(), request)
			return command.CorrectedAt, err
		}, time.Date(2026, 9, 24, 7, 0, 0, 0, time.UTC)},
	"POD 更正 correctedAt": {workFactDeliveryProofCorrectionBody, "correctedAt",
		func(intake *tfhttp.WorkFactCommandIntake, request *http.Request) (time.Time, error) {
			command, err := intake.IntakeCorrection(context.Background(), request)
			return command.CorrectedAt, err
		}, time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)},
}

// Covers: 更正口的时刻照本包的分格走（parseOptionalInstant）：键缺席、空串或纯空白都是「空着」，交零值给编排答`未受理`；
// 前后带空白的合法时刻照常解出。缺席也不拿服务端时钟补——DeliveryIntake 契约「服务器代铸任何一样，离线补传的重放就会被
// 误判成新事实」。
func TestWorkFactCommandIntakeLeavesAbsentCorrectionInstantsToTheOrchestration(t *testing.T) {
	intake := workFactIntakeForTest(t)
	for row, instant := range correctionInstants {
		var fields map[string]any
		if err := json.Unmarshal([]byte(instant.body), &fields); err != nil {
			t.Fatalf("合规载荷解不开：%v", err)
		}
		given, _ := fields[instant.key].(string)
		for cell, body := range map[string]string{
			"键缺席": bodyWith(t, instant.body, instant.key, nil),
			"空串":  bodyWith(t, instant.body, instant.key, ""),
			"纯空白": bodyWith(t, instant.body, instant.key, " \t "),
		} {
			t.Run(row+"/"+cell, func(t *testing.T) {
				at, err := instant.read(intake, commandRequest(body))
				if err != nil {
					t.Fatalf("被当成坏报文：%v", err)
				}
				if !at.IsZero() {
					t.Fatalf("%s = %s, want 零值——空着交给编排答未受理，不由服务端补", instant.key, at)
				}
			})
		}
		t.Run(row+"/前后带空白的合法时刻", func(t *testing.T) {
			at, err := instant.read(intake, commandRequest(bodyWith(t, instant.body, instant.key, " "+given+" ")))
			if err != nil {
				t.Fatalf("被拒：%v", err)
			}
			if !at.Equal(instant.want) {
				t.Fatalf("%s = %s, want %s", instant.key, at, instant.want)
			}
		})
	}
}

// Covers: 更正口的时刻给了却解不出是坏报文（400），拒绝理由点名那个键。
func TestWorkFactCommandIntakeRefusesUnparsableCorrectionInstants(t *testing.T) {
	intake := workFactIntakeForTest(t)
	for row, instant := range correctionInstants {
		t.Run(row, func(t *testing.T) {
			_, err := instant.read(intake, commandRequest(bodyWith(t, instant.body, instant.key, "昨天")))
			if !errors.Is(err, tfhttp.ErrMalformedRequest) || !strings.Contains(err.Error(), instant.key) {
				t.Fatalf("err = %v, want 点名 %s 的 ErrMalformedRequest", err, instant.key)
			}
		})
	}
}

// Covers: 交接更正的结论与首登同一分格（HandoverRegistrationPayload.Command）：空着照零值交进去，由编排答`未受理`；
// 词表外是用法错误（400）。
func TestWorkFactCommandIntakeLeavesAnAbsentHandoverCorrectionVerdictToTheOrchestration(t *testing.T) {
	intake := workFactIntakeForTest(t)
	command, err := intake.IntakeHandoverCorrection(context.Background(),
		commandRequest(bodyWith(t, workFactHandoverCorrectionBody, "verdict", nil)))
	if err != nil {
		t.Fatalf("缺结论被当成坏报文：%v", err)
	}
	if command.Verdict != domain.HandoverVerdictInvalid {
		t.Fatalf("Verdict = %s, want 零值", command.Verdict)
	}
	_, err = intake.IntakeHandoverCorrection(context.Background(),
		commandRequest(bodyWith(t, workFactHandoverCorrectionBody, "verdict", "MAYBE")))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("词表外结论：err = %v, want ErrMalformedRequest", err)
	}
}

// correctionEndpoint 是一个更正口接上本包真编排之后的样子：前版已经由同一个 Intake 登上，body 是指名那一版的合规更正
// 载荷，ledger 交回册上此刻各行的指纹。
type correctionEndpoint struct {
	correct http.Handler
	body    string
	ledger  func() []string
}

func serveWorkFact(handler http.Handler, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, commandRequest(body))
	return recorder
}

func pickupCorrectionEndpoint(t *testing.T, intake *tfhttp.WorkFactCommandIntake) correctionEndpoint {
	t.Helper()
	fixture := newPickupRegistrationFixture(t)
	seed := serveWorkFact(tfhttp.NewRegisterOffsitePickupEndpoint(intake, fixture.handler), offsitePickupRegistrationBody)
	if seed.Code != http.StatusCreated {
		t.Fatalf("登前版：status = %d body = %s", seed.Code, seed.Body.String())
	}
	seeded := decodePickupRegistration(t, seed)
	return correctionEndpoint{
		correct: tfhttp.NewCorrectOffsitePickupEndpoint(intake, fixture.handler),
		body:    bodyWith(t, workFactPickupCorrectionBody, "predecessorVersion", seeded.PickupVersion),
		ledger: func() []string {
			var rows []string
			for chain, records := range fixture.registry.records {
				for _, record := range records {
					rows = append(rows, chain+"@"+record.Pickup.Version().String())
				}
			}
			sort.Strings(rows)
			return rows
		},
	}
}

func handoverCorrectionEndpoint(t *testing.T, intake *tfhttp.WorkFactCommandIntake) correctionEndpoint {
	t.Helper()
	registry, segments := newHandoverRegistry(), newSegmentRegistry()
	clock := tfClock{at: time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)}
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  registry,
		Segments:   segments,
		Downstream: handoverHandoffDouble{},
		Clock:      clock,
		ParticipationEnds: application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
			Segments:   segments,
			Handovers:  registry,
			Deliveries: newDeliveryStore(),
			Clock:      clock,
		}),
	})
	// 译装用例的首登载荷带着 basis 以证那一格也照收；`已交接`带依据在领域上不成形，登前版时去掉它。
	seed := serveWorkFact(tfhttp.NewRegisterTransportHandoverEndpoint(intake, handler), bodyWith(t, handoverRegistrationBody, "basis", nil))
	if seed.Code != http.StatusCreated {
		t.Fatalf("登前版：status = %d body = %s", seed.Code, seed.Body.String())
	}
	return correctionEndpoint{
		correct: tfhttp.NewCorrectTransportHandoverEndpoint(intake, handler),
		body:    workFactHandoverCorrectionBody,
		ledger: func() []string {
			var rows []string
			for key := range registry.records {
				rows = append(rows, key)
			}
			sort.Strings(rows)
			return rows
		},
	}
}

func deliveryProofCorrectionEndpoint(t *testing.T, intake *tfhttp.WorkFactCommandIntake) correctionEndpoint {
	t.Helper()
	store := newDeliveryStore()
	handler := application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:          &deliveryViewDouble{outcome: domain.ObjectDelivered},
		Deliveries:        store,
		Versions:          &deliveryVersionFactory{},
		Downstream:        deliveryHandoffDouble{},
		Clock:             tfClock{at: time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)},
		ParticipationEnds: participationEnderDouble{outcome: application.ParticipationNoActiveParticipation},
	})
	seed := serveWorkFact(tfhttp.NewRegisterEffectiveDeliveryEndpoint(intake, handler), workFactEffectiveDeliveryBody)
	if seed.Code != http.StatusCreated {
		t.Fatalf("登前版：status = %d body = %s", seed.Code, seed.Body.String())
	}
	return correctionEndpoint{
		correct: tfhttp.NewCorrectDeliveryProofEndpoint(intake, handler),
		body:    bodyWith(t, workFactDeliveryProofCorrectionBody, "predecessorVersion", decodeDelivery(t, seed).DeliveryVersion),
		// 这一册按键只存当前版，更正是顶替而不是加行：指纹带上版本号，顶替了也看得见。
		ledger: func() []string {
			var rows []string
			for key, record := range store.records {
				rows = append(rows, key+"@"+record.Delivery.Version().String())
			}
			sort.Strings(rows)
			return rows
		},
	}
}

// Covers: 更正时刻缺席与解不出在线上是两格（ADR-0022 的表：2xx 出队按 `outcome` 呈现，4xx 出队交给人）。缺席走到编排，
// 答 200 `SOURCE_NOT_ACCEPTED`、册上不变——领域的零值门在线上走得到；解不出是 400 `MALFORMED_REQUEST`，没有 `outcome`。
// 末一格是对照：同一份载荷带上更正时刻就落新版，前两格答的确是时刻那一格。
func TestWorkFactCorrectionEndpointsLeaveAnAbsentCorrectionInstantToTheOrchestration(t *testing.T) {
	for line, assemble := range map[string]func(*testing.T, *tfhttp.WorkFactCommandIntake) correctionEndpoint{
		"/transport-fulfillment/offsite-pickup-corrections": pickupCorrectionEndpoint,
		"/transport-fulfillment/handover-corrections":       handoverCorrectionEndpoint,
		"/transport-fulfillment/delivery-proof-corrections": deliveryProofCorrectionEndpoint,
	} {
		t.Run(line, func(t *testing.T) {
			endpoint := assemble(t, workFactIntakeForTest(t))
			before := strings.Join(endpoint.ledger(), "\n")

			absent := serveWorkFact(endpoint.correct, bodyWith(t, endpoint.body, "correctedAt", nil))
			var answer map[string]any
			if err := json.Unmarshal(absent.Body.Bytes(), &answer); err != nil {
				t.Fatalf("缺更正时刻：答复解不开：%v", err)
			}
			if absent.Code != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
				t.Fatalf("缺更正时刻：answer = %d %s, want 200 SOURCE_NOT_ACCEPTED", absent.Code, absent.Body.String())
			}
			if after := strings.Join(endpoint.ledger(), "\n"); after != before {
				t.Fatalf("缺更正时刻却改了册：\n%s\nwant\n%s", after, before)
			}

			garbled := serveWorkFact(endpoint.correct, bodyWith(t, endpoint.body, "correctedAt", "昨天"))
			var problem map[string]any
			if err := json.Unmarshal(garbled.Body.Bytes(), &problem); err != nil {
				t.Fatalf("更正时刻解不出：答复解不开：%v", err)
			}
			if _, carriesOutcome := problem["outcome"]; garbled.Code != http.StatusBadRequest ||
				!strings.Contains(garbled.Body.String(), `"MALFORMED_REQUEST"`) || carriesOutcome {
				t.Fatalf("更正时刻解不出：answer = %d %s, want 400 MALFORMED_REQUEST 且不带 outcome", garbled.Code, garbled.Body.String())
			}
			if after := strings.Join(endpoint.ledger(), "\n"); after != before {
				t.Fatalf("更正时刻解不出却改了册：\n%s\nwant\n%s", after, before)
			}

			if complete := serveWorkFact(endpoint.correct, endpoint.body); complete.Code != http.StatusCreated {
				t.Fatalf("对照：带上更正时刻：status = %d body = %s, want 201", complete.Code, complete.Body.String())
			}
		})
	}
}
