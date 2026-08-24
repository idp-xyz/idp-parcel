package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	pgadapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// 本文件证登记口的编排：命令路由与退出码翻译、身份双轨各行其道（②登记内容不得
// 冒充①通道身份）、只有落册/已在册的执行留痕、留痕失败随事务翻成未决。用例结果
// 的派生逻辑在应用层已证，这里接真 handler 配仓储替身，证的是登记口把它们接对。

var executeAt = time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC)

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type cliSuspensionStore struct {
	byID map[domain.SuspensionID]domain.SuspensionDecision
}

func (store *cliSuspensionStore) FindByID(
	_ context.Context,
	id domain.SuspensionID,
) (domain.SuspensionDecision, bool, error) {
	decision, found := store.byID[id]
	return decision, found, nil
}

func (store *cliSuspensionStore) Save(
	_ context.Context,
	decision domain.SuspensionDecision,
) (ports.GovernanceSaveOutcome, error) {
	if _, exists := store.byID[decision.ID()]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	store.byID[decision.ID()] = decision
	return ports.GovernanceSaved, nil
}

type cliResumptionStore struct {
	bySuspension map[domain.SuspensionID]domain.ResumptionDecision
}

func (store *cliResumptionStore) FindBySuspension(
	_ context.Context,
	id domain.SuspensionID,
) (domain.ResumptionDecision, bool, error) {
	decision, found := store.bySuspension[id]
	return decision, found, nil
}

func (store *cliResumptionStore) Save(
	_ context.Context,
	decision domain.ResumptionDecision,
) (ports.GovernanceSaveOutcome, error) {
	if _, exists := store.bySuspension[decision.Suspension()]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	store.bySuspension[decision.Suspension()] = decision
	return ports.GovernanceSaved, nil
}

type cliTakeoverStore struct{}

func (cliTakeoverStore) FindByInterval(
	_ context.Context,
	_ domain.AuthorityInterval,
) (domain.TakeoverRecord, bool, error) {
	return domain.TakeoverRecord{}, false, nil
}

func (cliTakeoverStore) Save(
	_ context.Context,
	_ domain.TakeoverRecord,
) (ports.GovernanceSaveOutcome, error) {
	return ports.GovernanceSaved, nil
}

type cliIntervalStore struct {
	intervals []domain.AuthorityInterval
}

func (store *cliIntervalStore) ListCurrent(_ context.Context) ([]domain.AuthorityInterval, error) {
	return append([]domain.AuthorityInterval(nil), store.intervals...), nil
}

func (store *cliIntervalStore) Append(_ context.Context, interval domain.AuthorityInterval) error {
	store.intervals = append(store.intervals, interval)
	return nil
}

type recordingDownstream struct {
	intents []ports.GovernanceHandoffIntent
}

func (downstream *recordingDownstream) HandOffGovernance(
	_ context.Context,
	intent ports.GovernanceHandoffIntent,
) error {
	downstream.intents = append(downstream.intents, intent)
	return nil
}

type traceRecorder struct {
	executions []pgadapter.ChannelExecution
	err        error
}

func (recorder *traceRecorder) Append(_ context.Context, execution pgadapter.ChannelExecution) error {
	if recorder.err != nil {
		return recorder.err
	}
	recorder.executions = append(recorder.executions, execution)
	return nil
}

type executeFixture struct {
	regs        registrars
	suspensions *cliSuspensionStore
	intervals   *cliIntervalStore
	tracer      *traceRecorder
}

func newExecuteFixture() *executeFixture {
	suspensions := &cliSuspensionStore{byID: map[domain.SuspensionID]domain.SuspensionDecision{}}
	resumptions := &cliResumptionStore{bySuspension: map[domain.SuspensionID]domain.ResumptionDecision{}}
	intervals := &cliIntervalStore{}
	tracer := &traceRecorder{}
	incidents := application.NewGovernIncidentHandler(application.GovernIncidentDeps{
		Suspensions: suspensions,
		Resumptions: resumptions,
		Takeovers:   cliTakeoverStore{},
		Intervals:   intervals,
		Downstream:  &recordingDownstream{},
		Clock:       fixedClock{at: executeAt},
	})
	registration := application.NewRegisterAuthorityIntervalHandler(application.RegisterAuthorityIntervalDeps{
		Intervals: intervals,
	})
	return &executeFixture{
		regs: registrars{
			incidents:  incidents,
			intervals:  registration,
			tracer:     tracer,
			transactor: passthroughTransactor{},
			clock:      fixedClock{at: executeAt},
		},
		suspensions: suspensions,
		intervals:   intervals,
		tracer:      tracer,
	}
}

var executeIdentity = channelIdentity{osUser: "OPSHOST\\operator-a", hostname: "ops-host-01"}

func suspendInput() []byte {
	return []byte(`{
		"suspensionId": "suspension-9",
		"triggerSource": "PILOT-RULE/hard-risk-3",
		"basis": "cross-customer isolation risk",
		"evidence": "evidence-pack/incident-9",
		"scope": "pilot-scope/v2",
		"executedBy": "declared-duty-officer",
		"occurredAt": "2026-08-24T01:00:00Z",
		"effectiveAt": "2026-08-24T01:05:00Z",
		"inTransitNote": "in-transit objects continue under domain owners"
	}`)
}

func intervalInput(authority, from string) []byte {
	return []byte(`{
		"objectScope": "pilot-members/v1",
		"capability": "shipment-intake",
		"factKind": "production-ownership",
		"authority": "` + authority + `",
		"fromAt": "` + from + `"
	}`)
}

// TestExecuteSuspendLandsAndTracesTheChannelIdentity 证绿路径与身份双轨：暂停落册
// 答 0；留痕带的是入口自取的通道身份，不是输入里声明的执行人——②冒充①在这里就
// 该被抓出来。
func TestExecuteSuspendLandsAndTracesTheChannelIdentity(t *testing.T) {
	fixture := newExecuteFixture()

	message, code := execute(context.Background(), commandSuspend, suspendInput(), executeIdentity, fixture.regs)
	if code != exitRegistered {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitRegistered)
	}
	if !strings.Contains(message, "SUSPENSION_RECORDED") {
		t.Fatalf("答复 = %q，要含 SUSPENSION_RECORDED", message)
	}
	if len(fixture.suspensions.byID) != 1 {
		t.Fatalf("暂停在册数 = %d，要 1", len(fixture.suspensions.byID))
	}
	if len(fixture.tracer.executions) != 1 {
		t.Fatalf("留痕数 = %d，要 1", len(fixture.tracer.executions))
	}
	trace := fixture.tracer.executions[0]
	if trace.OSUser != "OPSHOST\\operator-a" || trace.Hostname != "ops-host-01" {
		t.Fatalf("通道身份 = %q@%q，要入口自取的那份", trace.OSUser, trace.Hostname)
	}
	if trace.OSUser == "declared-duty-officer" {
		t.Fatalf("第②轨（executedBy）冒充了第①轨（通道身份）")
	}
	if trace.Command != commandSuspend || trace.RecordReference != "suspension-9" {
		t.Fatalf("留痕指名 = %s/%s，要 suspend/suspension-9", trace.Command, trace.RecordReference)
	}
	if trace.Outcome != "SUSPENSION_RECORDED" {
		t.Fatalf("留痕答案 = %q", trace.Outcome)
	}
	if !trace.ExecutedAt.Equal(executeAt) {
		t.Fatalf("留痕时刻 = %s，要 %s", trace.ExecutedAt, executeAt)
	}
}

// TestExecuteSuspendReplayAnswersExistingAndLeavesASecondTrace 证重放：同标识第二次
// 执行答已在册、退出码仍 0；册上仍一条，而留痕两条——同一登记被执行过几次是要留
// 的事实。
func TestExecuteSuspendReplayAnswersExistingAndLeavesASecondTrace(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	if _, code := execute(ctx, commandSuspend, suspendInput(), executeIdentity, fixture.regs); code != exitRegistered {
		t.Fatalf("首次执行退出码 = %d", code)
	}
	message, code := execute(ctx, commandSuspend, suspendInput(), executeIdentity, fixture.regs)
	if code != exitRegistered {
		t.Fatalf("重放退出码 = %d（%s），要 %d", code, message, exitRegistered)
	}
	if !strings.Contains(message, "SUSPENSION_EXISTING") {
		t.Fatalf("答复 = %q，要含 SUSPENSION_EXISTING", message)
	}
	if len(fixture.suspensions.byID) != 1 {
		t.Fatalf("暂停在册数 = %d，要 1", len(fixture.suspensions.byID))
	}
	if len(fixture.tracer.executions) != 2 {
		t.Fatalf("留痕数 = %d，要 2", len(fixture.tracer.executions))
	}
	if fixture.tracer.executions[1].Outcome != "SUSPENSION_EXISTING" {
		t.Fatalf("重放留痕答案 = %q", fixture.tracer.executions[1].Outcome)
	}
}

// TestExecuteResumeAgainstMissingSuspensionIsUsageAndUntraced 证悬空引用：恢复指名
// 不存在的暂停答未受理（改请求而不是重试），册上无写入亦无痕。
func TestExecuteResumeAgainstMissingSuspensionIsUsageAndUntraced(t *testing.T) {
	fixture := newExecuteFixture()

	raw := []byte(`{
		"suspensionId": "never-suspended",
		"releaseEvidence": "r", "consistencyCheck": "c",
		"inventory": {
			"takenAt": "2026-08-24T02:00:00Z",
			"entries": [{
				"objectIdentity": "parcel-object/p1",
				"currentFacts": "accepted-fact/p1",
				"currentAuthority": "parcel-product",
				"responsibleParty": "ops-owner-1",
				"nextAction": "resume-normal-processing",
				"reviewBy": "2026-08-25T02:00:00Z"
			}]
		},
		"decidedBy": "pilot-business-owner",
		"decidedAt": "2026-08-24T03:00:00Z", "effectiveAt": "2026-08-24T03:05:00Z"
	}`)
	message, code := execute(context.Background(), commandResume, raw, executeIdentity, fixture.regs)
	if code != exitUsage {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitUsage)
	}
	if !strings.Contains(message, "SUSPENSION_NOT_FOUND") {
		t.Fatalf("答复 = %q，要含 SUSPENSION_NOT_FOUND", message)
	}
	if len(fixture.tracer.executions) != 0 {
		t.Fatalf("留痕数 = %d，要 0（没落册就没痕）", len(fixture.tracer.executions))
	}
}

// TestExecuteRejectedContentIsUntraced 证领域拒绝不留痕：九件缺一（空触发来源）过
// 得了翻译、过不了领域门，答未受理且册与痕都干净。
func TestExecuteRejectedContentIsUntraced(t *testing.T) {
	fixture := newExecuteFixture()

	raw := []byte(`{
		"suspensionId": "suspension-9",
		"triggerSource": "  ",
		"basis": "b", "evidence": "e", "scope": "s",
		"executedBy": "x",
		"occurredAt": "2026-08-24T01:00:00Z", "effectiveAt": "2026-08-24T01:05:00Z",
		"inTransitNote": "n"
	}`)
	message, code := execute(context.Background(), commandSuspend, raw, executeIdentity, fixture.regs)
	if code != exitUsage {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitUsage)
	}
	if len(fixture.suspensions.byID) != 0 || len(fixture.tracer.executions) != 0 {
		t.Fatalf("拒绝的内容不得落册或留痕")
	}
}

// TestExecuteIntervalConflictIsAGovernanceAnswer 证冲突阻断：重叠区间答治理退出码 2、
// 冲突对写进答复、不落库不留痕。
func TestExecuteIntervalConflictIsAGovernanceAnswer(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	if _, code := execute(ctx, commandAuthorityInterval,
		intervalInput("parcel-product", "2026-08-24T00:00:00Z"), executeIdentity, fixture.regs); code != exitRegistered {
		t.Fatalf("首条区间退出码 = %d", code)
	}
	message, code := execute(ctx, commandAuthorityInterval,
		intervalInput("legacy-system", "2026-08-24T06:00:00Z"), executeIdentity, fixture.regs)
	if code != exitGovernance {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitGovernance)
	}
	if !strings.Contains(message, "AUTHORITY_CONFLICT") ||
		!strings.Contains(message, "parcel-product") || !strings.Contains(message, "legacy-system") {
		t.Fatalf("答复 = %q，要含冲突双方", message)
	}
	if len(fixture.intervals.intervals) != 1 {
		t.Fatalf("在册区间数 = %d，要 1（冲突不落库）", len(fixture.intervals.intervals))
	}
	if len(fixture.tracer.executions) != 1 {
		t.Fatalf("留痕数 = %d，要 1（阻断的执行没落册）", len(fixture.tracer.executions))
	}
}

// TestExecuteTraceFailureUndoesTheRegistration 证痕与登记同笔事务：留痕写不进去，
// 整笔答未决——登记不许在无痕状态下落地。
func TestExecuteTraceFailureUndoesTheRegistration(t *testing.T) {
	fixture := newExecuteFixture()
	fixture.tracer.err = errors.New("trace store unavailable")

	message, code := execute(context.Background(), commandSuspend, suspendInput(), executeIdentity, fixture.regs)
	if code != exitUndecided {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitUndecided)
	}
	if !strings.Contains(message, "未决") {
		t.Fatalf("答复 = %q，要含 未决", message)
	}
}

// TestExecuteBadInputIsUsage 证形状坏的输入在翻译处就拒：未知字段答用法错误，
// 用例一步没走。
func TestExecuteBadInputIsUsage(t *testing.T) {
	fixture := newExecuteFixture()

	_, code := execute(context.Background(), commandSuspend, []byte(`{"typo": 1}`), executeIdentity, fixture.regs)
	if code != exitUsage {
		t.Fatalf("退出码 = %d，要 %d", code, exitUsage)
	}
	if len(fixture.tracer.executions) != 0 {
		t.Fatalf("坏输入不得留痕")
	}
}

// TestIncidentAnswerCoversEveryOutcome 证退出码翻译对治理用例的封闭结果集逐格成立，
// 含「已入册但下游意图未交出去」的续办格。
func TestIncidentAnswerCoversEveryOutcome(t *testing.T) {
	cases := []struct {
		outcome    application.GovernIncidentOutcome
		handoffRef string
		code       int
	}{
		{application.SuspensionRecorded, "", exitRegistered},
		{application.SuspensionExisting, "", exitRegistered},
		{application.ResumptionRecorded, "", exitRegistered},
		{application.ResumptionExisting, "", exitRegistered},
		{application.SuspensionNotFound, "", exitUsage},
		{application.GovernIncidentNotAccepted, "", exitUsage},
		{application.GovernIncidentUndecided, "", exitUndecided},
		{application.SuspensionRecorded, "CONT-GOV-SUSPENSION", exitGovernance},
	}
	for _, spec := range cases {
		message, code := incidentAnswer(commandSuspend, spec.outcome, spec.handoffRef)
		if code != spec.code {
			t.Fatalf("%s（续办 %q）→ %d，要 %d", spec.outcome, spec.handoffRef, code, spec.code)
		}
		if spec.handoffRef != "" && !strings.Contains(message, spec.handoffRef) {
			t.Fatalf("续办引用没有写进答复：%q", message)
		}
	}
}

// TestIntervalAnswerCoversEveryOutcome 证权威区间登记的退出码翻译逐格成立。
func TestIntervalAnswerCoversEveryOutcome(t *testing.T) {
	cases := []struct {
		outcome application.RegisterAuthorityIntervalOutcome
		code    int
	}{
		{application.IntervalRegistered, exitRegistered},
		{application.IntervalAlreadyRegistered, exitRegistered},
		{application.IntervalConflictBlocked, exitGovernance},
		{application.IntervalNotAccepted, exitUsage},
		{application.IntervalUndecided, exitUndecided},
	}
	for _, spec := range cases {
		if _, code := intervalAnswer(spec.outcome, nil); code != spec.code {
			t.Fatalf("%s → %d，要 %d", spec.outcome, code, spec.code)
		}
	}
}

// TestRunRejectsUsageErrors 证进程口的用法边界：缺命令、未知命令、缺输入文件、
// 缺 DSN 各自以用法错误退出，不碰数据库。
func TestRunRejectsUsageErrors(t *testing.T) {
	ctx := context.Background()
	noEnv := func(string) string { return "" }

	if code := run(ctx, nil, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺命令退出码 = %d", code)
	}
	if code := run(ctx, []string{"take-over"}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("集合外命令退出码 = %d（接管是第二批，不在本入口）", code)
	}
	if code := run(ctx, []string{commandSuspend}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 -input 退出码 = %d", code)
	}

	input := filepath.Join(t.TempDir(), "suspend.json")
	if err := os.WriteFile(input, suspendInput(), 0o600); err != nil {
		t.Fatalf("写输入文件：%v", err)
	}
	if code := run(ctx, []string{commandSuspend, "-input", input}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 DSN 退出码 = %d（登记口不猜连接串）", code)
	}
}
