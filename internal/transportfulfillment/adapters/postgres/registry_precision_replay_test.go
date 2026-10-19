package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证票 operator-channel/30：编排接真登记册、每次调用一笔事务（同生产装配的事务包装），
// 命令里的时刻一律带纳秒尾巴。原样重放答重放那一格，跨一微秒才答冲突、签新版或来源冲突；同一微秒内的窗口与
// 区间答`未受理`，事务照常提交。夹具全为合成值（S 级）。

var (
	nsFrom    = time.Date(2026, 9, 1, 9, 0, 0, 123456789, time.UTC)
	nsUntil   = time.Date(2026, 12, 1, 9, 0, 0, 987654321, time.UTC)
	nsChanged = time.Date(2026, 9, 3, 9, 0, 0, 555555555, time.UTC)
)

func newPrecisionDB(t *testing.T) (*bentopg.DB, bentoapp.Transactor) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db, db.Transactor()
}

// inTransaction 让一次编排调用独占一笔事务，并要求事务提交成功：编排把「存不进去」答成别的格却让事务中止时，
// 提交会失败，这里就红——生产装配的事务包装此时答的是 500。
func inTransaction[R any](t *testing.T, transactor bentoapp.Transactor, call func(context.Context) (R, error)) R {
	t.Helper()
	var result R
	if err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = call(txCtx)
		return err
	}); err != nil {
		t.Fatalf("事务没有提交：%v", err)
	}
	return result
}

// pinRoundTrip 是「登记册时刻落在 timestamptz 上」这条前提的钉：读回的值必须恰是写入值在登记册精度上的样子。
// 哪一册改存 jsonb 快照或改了列型，读回就不再等于它，这里先红——AtRegistryPrecision 头注那条前提要重看。
func pinRoundTrip(t *testing.T, name string, readBack, written time.Time) {
	t.Helper()
	if want := domain.AtRegistryPrecision(written); !readBack.Equal(want) {
		t.Fatalf("%s读回 %s，期望 %s", name, readBack.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func newPrecisionCredentialHandler(t *testing.T, db *bentopg.DB) *application.RegisterExternalCarrierCredentialHandler {
	t.Helper()
	credentials, err := adapter.NewExternalCarrierCredentials(db, credentialClock{at: credentialNow})
	if err != nil {
		t.Fatalf("凭证登记册：%v", err)
	}
	return application.NewRegisterExternalCarrierCredentialHandler(application.RegisterExternalCarrierCredentialDeps{
		Credentials: credentials,
		Clock:       credentialClock{at: credentialNow},
	})
}

func nsCredentialCommand(t *testing.T, version string) application.RegisterExternalCarrierCredentialCommand {
	t.Helper()
	return application.RegisterExternalCarrierCredentialCommand{
		TenantID:       segmentRef(t, domain.NewTenantID, "SYN-TENANT-01"),
		Credential:     "SYN-CARRIER-X/1Z-NS",
		Version:        version,
		Assigner:       "SYN-CARRIER-X",
		IdentifiedKind: "CARRIED_OBJECT",
		IdentifiedRef:  "SYN-PARCEL-NS",
		EffectiveFrom:  nsFrom,
		EffectiveUntil: nsUntil,
	}
}

func registerCredential(t *testing.T, transactor bentoapp.Transactor, handler *application.RegisterExternalCarrierCredentialHandler, command application.RegisterExternalCarrierCredentialCommand) application.CredentialRegistrationOutcome {
	t.Helper()
	return inTransaction(t, transactor, func(ctx context.Context) (application.RegisterExternalCarrierCredentialResult, error) {
		return handler.Register(ctx, command)
	}).Outcome()
}

func changeCredential(t *testing.T, transactor bentoapp.Transactor, handler *application.RegisterExternalCarrierCredentialHandler, command application.ChangeCredentialApplicabilityCommand) application.CredentialRegistrationOutcome {
	t.Helper()
	return inTransaction(t, transactor, func(ctx context.Context) (application.RegisterExternalCarrierCredentialResult, error) {
		return handler.ChangeApplicability(ctx, command)
	}).Outcome()
}

// Covers: 凭证首登与改变适用关系——适用起止与改变时间带纳秒的命令原样重放答`已有版本`；任一格跨一微秒答`内容冲突`。
func TestCredentialRegistrationAndChangeReplayWithNanosecondsAgainstARealRegistry(t *testing.T) {
	db, transactor := newPrecisionDB(t)
	handler := newPrecisionCredentialHandler(t, db)
	command := nsCredentialCommand(t, "SYN-ECV-NS-1")

	if got := registerCredential(t, transactor, handler, command); got != application.CredentialRegistered {
		t.Fatalf("首登 = %s", got)
	}
	replay := inTransaction(t, transactor, func(ctx context.Context) (application.RegisterExternalCarrierCredentialResult, error) {
		return handler.Register(ctx, command)
	})
	if replay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("带纳秒原样重放 = %s，期望 EXISTING_VERSION", replay.Outcome())
	}
	stored, _ := replay.Record()
	storedUntil, _ := stored.Credential.Applicability().Until()
	pinRoundTrip(t, "凭证适用起点", stored.Credential.Applicability().From(), nsFrom)
	pinRoundTrip(t, "凭证适用终点", storedUntil, nsUntil)
	from, until := command, command
	from.EffectiveFrom = from.EffectiveFrom.Add(time.Microsecond)
	until.EffectiveUntil = until.EffectiveUntil.Add(time.Microsecond)
	for name, apart := range map[string]application.RegisterExternalCarrierCredentialCommand{"from": from, "until": until} {
		if got := registerCredential(t, transactor, handler, apart); got != application.CredentialContentConflict {
			t.Fatalf("%s 跨一微秒 = %s，期望 CONTENT_CONFLICT", name, got)
		}
	}

	revoke := application.ChangeCredentialApplicabilityCommand{
		TenantID: command.TenantID, Credential: command.Credential, Change: domain.CredentialRevoked,
		At: nsChanged, NewVersion: "SYN-ECV-NS-2",
	}
	if got := changeCredential(t, transactor, handler, revoke); got != application.CredentialApplicabilityChanged {
		t.Fatalf("作废 = %s", got)
	}
	changeReplay := inTransaction(t, transactor, func(ctx context.Context) (application.RegisterExternalCarrierCredentialResult, error) {
		return handler.ChangeApplicability(ctx, revoke)
	})
	if changeReplay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("改变时间带纳秒原样重放 = %s，期望 EXISTING_VERSION", changeReplay.Outcome())
	}
	changed, _ := changeReplay.Record()
	changedAt, _ := changed.Credential.ChangedAt()
	pinRoundTrip(t, "凭证改变时间", changedAt, nsChanged)
	later := revoke
	later.At = later.At.Add(time.Microsecond)
	if got := changeCredential(t, transactor, handler, later); got != application.CredentialContentConflict {
		t.Fatalf("改变时间跨一微秒 = %s，期望 CONTENT_CONFLICT", got)
	}
}

// Covers: 凭证首登的并发输家（READ COMMITTED）——A 写入后不提交；B 查时看不见 A，在唯一键上排队；A 提交后 B 读回赢家，
// 带纳秒的同一条命令答`已有版本`。排队的取证照 party-commercial 的 TestSameVersionCorrectionsSerializeOnTheVersionRow：
// B 在 A 提交前就返回，说明根本没有排队，那么读回赢家这条路也就没走到。
func TestACredentialRaceLoserWithNanosecondsReadsBackTheWinnerAsTheExistingVersion(t *testing.T) {
	db, transactor := newPrecisionDB(t)
	handler := newPrecisionCredentialHandler(t, db)
	command := nsCredentialCommand(t, "SYN-ECV-RACE-1")
	ctx := t.Context()

	aHolds, releaseA := make(chan struct{}), make(chan struct{})
	aDone := make(chan error, 1)
	go func() {
		aDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := handler.Register(txCtx, command)
			if err == nil && result.Outcome() != application.CredentialRegistered {
				t.Errorf("A = %s，期望 CREDENTIAL_REGISTERED", result.Outcome())
			}
			close(aHolds)
			<-releaseA
			return err
		})
	}()
	<-aHolds

	type answer struct {
		outcome application.CredentialRegistrationOutcome
		err     error
	}
	bDone := make(chan answer, 1)
	go func() {
		var outcome application.CredentialRegistrationOutcome
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := handler.Register(txCtx, command)
			outcome = result.Outcome()
			return err
		})
		bDone <- answer{outcome, err}
	}()

	select {
	case early := <-bDone:
		close(releaseA)
		t.Fatalf("B 在 A 提交前就返回了（%s，err=%v）——没有在唯一键上排队", early.outcome, early.err)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseA)
	if err := <-aDone; err != nil {
		t.Fatalf("A 提交失败：%v", err)
	}
	loser := <-bDone
	if loser.err != nil || loser.outcome != application.CredentialExistingVersion {
		t.Fatalf("B = %s（err=%v），期望 EXISTING_VERSION", loser.outcome, loser.err)
	}
}

// Covers: 总单形成新版本——改变时间带纳秒的命令原样重放答`已有版本`，读回的改变时间是它在登记册上的样子；
// 跨一微秒答`内容冲突`。
func TestAMasterDocumentRevisionReplayWithNanosecondsAgainstARealRegistry(t *testing.T) {
	db, transactor := newPrecisionDB(t)
	documents, err := adapter.NewMasterDocuments(db)
	if err != nil {
		t.Fatalf("总单登记册：%v", err)
	}
	handler := application.NewRegisterMasterDocumentHandler(application.RegisterMasterDocumentDeps{
		Documents: documents, Clock: credentialClock{at: credentialNow},
	})
	tenant := segmentRef(t, domain.NewTenantID, "SYN-TENANT-01")
	register := application.RegisterMasterDocumentCommand{
		TenantID: tenant, Document: "SYN-MAWB-NS", Version: "SYN-MDV-1", Issuer: "SYN-ISSUER-1", Scope: "SYN-SCOPE-1",
	}
	if got := inTransaction(t, transactor, func(ctx context.Context) (application.RegisterMasterDocumentResult, error) {
		return handler.Register(ctx, register)
	}).Outcome(); got != application.MasterDocumentRegistered {
		t.Fatalf("首登 = %s", got)
	}
	revise := func(command application.ReviseMasterDocumentCommand) application.RegisterMasterDocumentResult {
		t.Helper()
		return inTransaction(t, transactor, func(ctx context.Context) (application.RegisterMasterDocumentResult, error) {
			return handler.Revise(ctx, command)
		})
	}
	revoke := application.ReviseMasterDocumentCommand{
		TenantID: tenant, Document: "SYN-MAWB-NS", Revision: domain.MasterDocumentRevocation, At: nsChanged, NewVersion: "SYN-MDV-2",
	}
	if got := revise(revoke).Outcome(); got != application.MasterDocumentRevised {
		t.Fatalf("撤销 = %s", got)
	}
	replay := revise(revoke)
	if replay.Outcome() != application.MasterDocumentExistingVersion {
		t.Fatalf("带纳秒原样重放 = %s，期望 EXISTING_VERSION", replay.Outcome())
	}
	stored, _ := replay.Record()
	changedAt, _ := stored.Document.ChangedAt()
	pinRoundTrip(t, "总单改变时间", changedAt, nsChanged)
	apart := revoke
	apart.At = domain.AtRegistryPrecision(nsChanged).Add(time.Microsecond)
	if got := revise(apart).Outcome(); got != application.MasterDocumentContentConflict {
		t.Fatalf("跨一微秒 = %s，期望 CONTENT_CONFLICT", got)
	}
}

type recordingTrackingHandoff struct {
	intents []ports.ExternalTrackingFactHandoffIntent
}

func (handoff *recordingTrackingHandoff) HandOffExternalTrackingFact(_ context.Context, intent ports.ExternalTrackingFactHandoffIntent) error {
	handoff.intents = append(handoff.intents, intent)
	return nil
}

// Covers: 有效时间显式判断——有效时间带纳秒的命令连发，第二次答`已按所给判断`、不签新版、重发的意图仍指原版本；
// 读回的有效时间是它在登记册上的样子；跨一微秒是改判，签新版。
func TestAnExplicitJudgmentReplayWithNanosecondsAgainstARealRegistry(t *testing.T) {
	db, transactor := newPrecisionDB(t)
	facts, err := adapter.NewExternalTrackingFacts(db)
	if err != nil {
		t.Fatalf("事实登记册：%v", err)
	}
	identities, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("身份签发：%v", err)
	}
	credentials, err := adapter.NewExternalCarrierCredentials(db, credentialClock{at: credentialNow})
	if err != nil {
		t.Fatalf("凭证登记册：%v", err)
	}
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	mustSaveCredential(t, transactor, ctx, credentials, credentialRecord(t, credentialFixtureOptions{
		credential: "SYN-CARRIER-X/1Z-JUDGE-NS", version: "SYN-ECV-1", reference: "SYN-PARCEL-JUDGE-NS",
	}))
	adopter := application.NewAdoptTrackingMaterialHandler(application.AdoptTrackingMaterialDeps{
		Facts: facts, Identities: identities, Credentials: credentials, Rules: absentRules{}, Ledger: facts,
		Downstream: noHandoff{}, Clock: credentialClock{at: credentialNow},
	})
	adopted := inTransaction(t, transactor, func(txCtx context.Context) (application.AdoptTrackingMaterialResult, error) {
		return adopter.Adopt(txCtx, ports.TrackingMaterial{
			Source:          "SYN-AGGREGATOR-A",
			Subject:         ports.TrackingSubject{Tenant: tenant, CredentialReference: "SYN-CARRIER-X/1Z-JUDGE-NS"},
			SourceEventID:   "SYN-EVT-JUDGE-NS",
			OccurredAt:      ports.SourceTime{Given: true, At: credentialNow.Add(-time.Hour)},
			ReceivedAt:      credentialNow.Add(-30 * time.Minute),
			StatusReference: "IN_TRANSIT",
			PayloadDigest:   "sha256:SYN-JUDGE-NS",
		})
	})
	pending, has := adopted.Record()
	if adopted.Outcome() != application.TrackingFactAdoptedPendingJudgment || !has {
		t.Fatalf("前置：认领为待判断 = %s", adopted.Outcome())
	}

	handoff := &recordingTrackingHandoff{}
	judge := application.NewJudgeEffectiveTimeHandler(application.JudgeEffectiveTimeDeps{
		Facts: facts, Identities: identities, Downstream: handoff, Clock: credentialClock{at: credentialNow},
	})
	judgeAt := func(at time.Time) application.JudgeEffectiveTimeResult {
		t.Helper()
		return inTransaction(t, transactor, func(txCtx context.Context) (application.JudgeEffectiveTimeResult, error) {
			return judge.Judge(txCtx, application.JudgeEffectiveTimeCommand{TenantID: tenant, Fact: pending.Key.Fact.String(), EffectiveAt: at})
		})
	}
	first := judgeAt(nsFrom)
	judged, _ := first.Record()
	if first.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("首次判断 = %s", first.Outcome())
	}
	replay := judgeAt(nsFrom)
	if replay.Outcome() != application.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("带纳秒原样重放 = %s，期望 ALREADY_JUDGED_AS_GIVEN", replay.Outcome())
	}
	again, _ := replay.Record()
	if again.Key != judged.Key {
		t.Fatalf("重放应交回原版本：%v，期望 %v", again.Key, judged.Key)
	}
	if last := handoff.intents[len(handoff.intents)-1]; len(handoff.intents) != 2 || last.Record.Key != judged.Key {
		t.Fatalf("重放应重发一份意图且仍指原版本：%d 份", len(handoff.intents))
	}
	effectiveAt, _ := again.Fact.EffectiveAt()
	pinRoundTrip(t, "有效时间", effectiveAt, nsFrom)
	if got := judgeAt(domain.AtRegistryPrecision(nsFrom).Add(time.Microsecond)).Outcome(); got != application.EffectiveTimeJudged {
		t.Fatalf("跨一微秒 = %s，期望改判签新版", got)
	}
}

// deliveryPrecisionFixture 把派送任务开立、派送尝试回传都接在真登记册上。
type deliveryPrecisionFixture struct {
	t          *testing.T
	transactor bentoapp.Transactor
	tenant     domain.TenantID
	tasks      *application.OpenDispatchTaskHandler
	attempts   *application.RecordDeliveryAttemptHandler
}

func newDeliveryPrecisionFixture(t *testing.T) *deliveryPrecisionFixture {
	t.Helper()
	db, transactor := newPrecisionDB(t)
	tasks, err := adapter.NewDispatchTasks(db)
	if err != nil {
		t.Fatalf("揽派任务登记册：%v", err)
	}
	attempts, err := adapter.NewDeliveryAttempts(db)
	if err != nil {
		t.Fatalf("派送尝试册：%v", err)
	}
	clock := credentialClock{at: credentialNow}
	return &deliveryPrecisionFixture{
		t: t, transactor: transactor, tenant: segmentRef(t, domain.NewTenantID, "SYN-TENANT-01"),
		tasks:    application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{Tasks: tasks, Clock: clock}),
		attempts: application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{Attempts: attempts, Tasks: tasks, Clock: clock}),
	}
}

func (fixture *deliveryPrecisionFixture) open(task string, from, to time.Time) application.OpenDispatchTaskResult {
	fixture.t.Helper()
	return inTransaction(fixture.t, fixture.transactor, func(ctx context.Context) (application.OpenDispatchTaskResult, error) {
		return fixture.tasks.Open(ctx, application.OpenDispatchTaskCommand{
			TenantID: fixture.tenant, Task: task, Kind: domain.DeliveryDispatch, Objects: []string{"SYN-PARCEL-D1"},
			Place: "SYN-PLACE-1", WindowFrom: from, WindowTo: to, Conditions: "SYN-CONDITIONS-1", OpenedAt: nsFrom.Add(-time.Hour),
		})
	})
}

func (fixture *deliveryPrecisionFixture) record(command application.RecordDeliveryAttemptCommand) application.RecordDeliveryAttemptResult {
	fixture.t.Helper()
	return inTransaction(fixture.t, fixture.transactor, func(ctx context.Context) (application.RecordDeliveryAttemptResult, error) {
		return fixture.attempts.Handle(ctx, command)
	})
}

// nsDeliveryCommand 交回一条各时刻格都带纳秒尾巴的派送尝试命令。
func (fixture *deliveryPrecisionFixture) nsDeliveryCommand(attempt string) application.RecordDeliveryAttemptCommand {
	return application.RecordDeliveryAttemptCommand{
		TenantID: fixture.tenant, Task: "SYN-TASK-D1", Attempt: attempt, ExecutedBy: "SYN-COURIER-1", Place: "SYN-PLACE-1",
		PlannedFrom: nsFrom, PlannedTo: nsFrom.Add(4*time.Hour + 864197532), ArrivedAt: nsFrom.Add(10*time.Minute + 432098766),
		Evidence: "SYN-EVIDENCE-1",
		Objects: []application.ObjectDeliverySubmission{{
			Object:     segmentRef(fixture.t, domain.NewCarriedObjectReference, "SYN-PARCEL-D1"),
			Outcome:    domain.ObjectDelivered,
			OccurredAt: nsFrom.Add(15*time.Minute + 98765432),
		}},
	}
}

// Covers: 派送尝试——计划窗两端、到场时刻与逐对象发生时刻都带纳秒的命令原样重放答`原结果`，读回的各时刻是它们在
// 登记册上的样子；任一格跨一微秒答`来源冲突`。揽派任务时间窗带纳秒照常开立，读回同样是登记册上的样子。
func TestADeliveryAttemptReplayWithNanosecondsAgainstARealRegistry(t *testing.T) {
	fixture := newDeliveryPrecisionFixture(t)
	windowTo := nsFrom.Add(8*time.Hour + 876543210)
	opened := fixture.open("SYN-TASK-D1", nsFrom, windowTo)
	if opened.Outcome() != application.DispatchTaskOpened {
		t.Fatalf("开立任务 = %s", opened.Outcome())
	}
	reopened := fixture.open("SYN-TASK-D1", nsFrom, windowTo)
	task, _ := reopened.Record()
	from, to := task.Task.Window()
	pinRoundTrip(t, "任务时间窗起点", from, nsFrom)
	pinRoundTrip(t, "任务时间窗终点", to, windowTo)

	command := fixture.nsDeliveryCommand("SYN-ATTEMPT-NS")
	if got := fixture.record(command).Outcome(); got != application.DeliveryAttemptRecorded {
		t.Fatalf("首次回传 = %s", got)
	}
	replay := fixture.record(command)
	if replay.Outcome() != application.DeliveryAttemptExistingResult {
		t.Fatalf("带纳秒原样重放 = %s，期望 EXISTING_RESULT", replay.Outcome())
	}
	stored, _ := replay.Record()
	pinRoundTrip(t, "计划窗起点", stored.Attempt.PlannedFrom(), command.PlannedFrom)
	pinRoundTrip(t, "计划窗终点", stored.Attempt.PlannedTo(), command.PlannedTo)
	pinRoundTrip(t, "到场时刻", stored.Attempt.ArrivedAt(), command.ArrivedAt)
	pinRoundTrip(t, "对象发生时刻", stored.Results[0].OccurredAt(), command.Objects[0].OccurredAt)

	shifts := map[string]func(*application.RecordDeliveryAttemptCommand){
		"planned from": func(c *application.RecordDeliveryAttemptCommand) {
			c.PlannedFrom = domain.AtRegistryPrecision(c.PlannedFrom).Add(time.Microsecond)
		},
		"planned to": func(c *application.RecordDeliveryAttemptCommand) {
			c.PlannedTo = domain.AtRegistryPrecision(c.PlannedTo).Add(time.Microsecond)
		},
		"arrived at": func(c *application.RecordDeliveryAttemptCommand) {
			c.ArrivedAt = domain.AtRegistryPrecision(c.ArrivedAt).Add(time.Microsecond)
		},
		"occurred at": func(c *application.RecordDeliveryAttemptCommand) {
			c.Objects = append([]application.ObjectDeliverySubmission(nil), c.Objects...)
			c.Objects[0].OccurredAt = domain.AtRegistryPrecision(c.Objects[0].OccurredAt).Add(time.Microsecond)
		},
	}
	for name, shift := range shifts {
		apart := command
		shift(&apart)
		if got := fixture.record(apart).Outcome(); got != application.DeliveryAttemptSourceConflict {
			t.Fatalf("%s 跨一微秒 = %s，期望 SOURCE_CONFLICT", name, got)
		}
	}
}

var (
	// sameMicroStart、sameMicroEnd 落在同一微秒里：按纳秒是严格先后，在登记册上相等。
	sameMicroStart = time.Date(2026, 9, 1, 9, 0, 0, 123456100, time.UTC)
	sameMicroEnd   = time.Date(2026, 9, 1, 9, 0, 0, 123456900, time.UTC)
	nextMicroEnd   = sameMicroStart.Truncate(time.Microsecond).Add(time.Microsecond)
)

// Covers: 严格先后门——派送任务窗、派送尝试计划窗落在同一微秒内答`未受理`，事务照常提交；跨一微秒照常受理。
func TestDispatchWindowsWithinOneMicrosecondAreNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newDeliveryPrecisionFixture(t)
	if got := fixture.open("SYN-TASK-SAME-µS", sameMicroStart, sameMicroEnd).Outcome(); got != application.DispatchTaskNotAccepted {
		t.Fatalf("同一微秒内的任务窗 = %s，期望 NOT_ACCEPTED", got)
	}
	if got := fixture.open("SYN-TASK-D1", sameMicroStart, nextMicroEnd).Outcome(); got != application.DispatchTaskOpened {
		t.Fatalf("跨一微秒的任务窗 = %s，期望 OPENED", got)
	}
	command := fixture.nsDeliveryCommand("SYN-ATTEMPT-SAME-µS")
	command.PlannedFrom, command.PlannedTo = sameMicroStart, sameMicroEnd
	if got := fixture.record(command).Outcome(); got != application.DeliveryAttemptNotAccepted {
		t.Fatalf("同一微秒内的计划窗 = %s，期望 SOURCE_NOT_ACCEPTED", got)
	}
	command.Attempt, command.PlannedTo = "SYN-ATTEMPT-NEXT-µS", nextMicroEnd
	if got := fixture.record(command).Outcome(); got != application.DeliveryAttemptRecorded {
		t.Fatalf("跨一微秒的计划窗 = %s，期望 ATTEMPT_RECORDED", got)
	}
}

// Covers: 严格先后门——凭证首登区间落在同一微秒内、作废点与起点同一微秒、作废点恰在起点，都答`未受理`，事务照常
// 提交；跨一微秒照常受理。
func TestCredentialRangesWithinOneMicrosecondAreNotAcceptedAgainstARealRegistry(t *testing.T) {
	db, transactor := newPrecisionDB(t)
	handler := newPrecisionCredentialHandler(t, db)
	sameMicro := nsCredentialCommand(t, "SYN-ECV-SAME-µS")
	sameMicro.Credential = "SYN-CARRIER-X/1Z-SAME-µS"
	sameMicro.EffectiveFrom, sameMicro.EffectiveUntil = sameMicroStart, sameMicroEnd
	if got := registerCredential(t, transactor, handler, sameMicro); got != application.CredentialRegistrationNotAccepted {
		t.Fatalf("同一微秒内的适用区间 = %s，期望 NOT_ACCEPTED", got)
	}
	nextMicro := sameMicro
	nextMicro.EffectiveUntil = nextMicroEnd
	if got := registerCredential(t, transactor, handler, nextMicro); got != application.CredentialRegistered {
		t.Fatalf("跨一微秒的适用区间 = %s，期望 REGISTERED", got)
	}

	whole := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		credential string
		from, at   time.Time
	}{
		{"SYN-CARRIER-X/1Z-REVOKE-µS", sameMicroStart, sameMicroEnd},
		{"SYN-CARRIER-X/1Z-REVOKE-START", whole, whole},
	} {
		open := nsCredentialCommand(t, "SYN-ECV-1")
		open.Credential, open.EffectiveFrom, open.EffectiveUntil = testCase.credential, testCase.from, time.Time{}
		if got := registerCredential(t, transactor, handler, open); got != application.CredentialRegistered {
			t.Fatalf("%s 首登 = %s", testCase.credential, got)
		}
		revoke := application.ChangeCredentialApplicabilityCommand{
			TenantID: open.TenantID, Credential: testCase.credential, Change: domain.CredentialRevoked,
			At: testCase.at, NewVersion: "SYN-ECV-2",
		}
		if got := changeCredential(t, transactor, handler, revoke); got != application.CredentialRegistrationNotAccepted {
			t.Fatalf("%s 作废点在登记册上等于起点 = %s，期望 NOT_ACCEPTED", testCase.credential, got)
		}
		revoke.At = domain.AtRegistryPrecision(testCase.from).Add(time.Microsecond)
		if got := changeCredential(t, transactor, handler, revoke); got != application.CredentialApplicabilityChanged {
			t.Fatalf("%s 跨一微秒的作废点 = %s，期望 APPLICABILITY_CHANGED", testCase.credential, got)
		}
	}
}
