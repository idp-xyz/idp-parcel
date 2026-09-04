package application_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	carrierEvidenceAt = pickupOccurredAt.Add(90 * time.Minute)
	carrierJudgedAt   = pickupRegisteredAt.Add(2 * time.Hour)
)

type carrierJudgmentFixture struct {
	segments  *segmentRegistryDouble
	judgments *judgmentRegistryDouble
	directory *identityDirectoryDouble
	clock     *movingClock
	handler   *application.FormActualCarrierJudgmentHandler
}

type movingClock struct{ at time.Time }

func (clock *movingClock) Now() time.Time { return clock.at }

// newCarrierJudgmentFixture 先经收寄登记把 segment-1 立起来（首版由挂点铸出），再装「形成实际承运商判断」
// 的编排——判断的入口永远是一个已经成立的段。
func newCarrierJudgmentFixture(t *testing.T, registered ...domain.CarrierSubject) *carrierJudgmentFixture {
	t.Helper()
	establishment := newJudgmentOnEstablishmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	if _, err := establishment.handler.Register(t.Context(), command); err != nil {
		t.Fatalf("立段：%v", err)
	}
	fixture := &carrierJudgmentFixture{
		segments:  establishment.segments,
		judgments: establishment.judgments,
		directory: newIdentityDirectory(registered...),
		clock:     &movingClock{at: carrierJudgedAt},
	}
	fixture.handler = application.NewFormActualCarrierJudgmentHandler(application.FormActualCarrierJudgmentDeps{
		Segments:   fixture.segments,
		Judgments:  fixture.judgments,
		Identities: fixture.directory,
		Clock:      fixture.clock,
	})
	return fixture
}

func carrierX(t *testing.T) domain.CarrierSubject {
	t.Helper()
	return mustSubject(t, domain.ExternalCarrierParty, "party/carrier-x")
}

func mustSubject(t *testing.T, kind domain.CarrierSubjectKind, reference string) domain.CarrierSubject {
	t.Helper()
	subject, err := domain.NewCarrierSubject(kind, reference)
	if err != nil {
		t.Fatalf("carrier subject: %v", err)
	}
	return subject
}

func scanCommand(t *testing.T) application.FormActualCarrierJudgmentCommand {
	t.Helper()
	return application.FormActualCarrierJudgmentCommand{
		TenantID:          pickupRegistrationCommand(t).TenantID,
		Segment:           "segment-1",
		Source:            domain.CarrierDirectPickupScan,
		EvidenceReference: "SCAN-1",
		OccurredAt:        carrierEvidenceAt,
		SubjectKind:       domain.ExternalCarrierParty,
		SubjectReference:  "party/carrier-x",
	}
}

func (fixture *carrierJudgmentFixture) currentJudgment(t *testing.T) domain.ActualCarrierJudgment {
	t.Helper()
	record, found, err := fixture.judgments.FindByKey(t.Context(), judgmentKeyFixture(t, "tenant-1", "segment-1"))
	if err != nil || !found {
		t.Fatalf("判断读不回：found=%v err=%v", found, err)
	}
	return record.Judgment
}

// Covers: 票面「形成实际承运商判断」用例（收一条合格证据引用 + 承运主体引用 → 查身份 → 形成新版本）；
// CONTEXT 生命周期「待确认 → 已识别」。
func TestQualifiedEvidenceNamingARegisteredPartyFormsAnIdentifiedVersion(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))

	result, err := fixture.handler.Form(t.Context(), scanCommand(t))
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if result.Outcome() != application.CarrierJudgmentVersionFormed {
		t.Fatalf("outcome = %q, want VERSION_FORMED", result.Outcome())
	}
	judgment := fixture.currentJudgment(t)
	if len(judgment.Versions()) != 2 {
		t.Fatalf("应在首版之上追加一版，实得 %d 版", len(judgment.Versions()))
	}
	subject, identified := judgment.Current().Verdict().Identified()
	if !identified || subject != carrierX(t) {
		t.Fatalf("应识别为 carrier-x，实得 identified=%v subject=%+v", identified, subject)
	}
	if !judgment.Current().BusinessTime().Equal(carrierEvidenceAt) || !judgment.Current().FormedAt().Equal(carrierJudgedAt) {
		t.Fatalf("业务时间 %s / 形成时间 %s", judgment.Current().BusinessTime(), judgment.Current().FormedAt())
	}
	if len(fixture.directory.asked) != 1 || fixture.directory.asked[0] != carrierX(t) {
		t.Fatalf("身份读口应恰被问一次、问的是 carrier-x：%+v", fixture.directory.asked)
	}
	record, has := result.Record()
	if !has || len(record.Judgment.Versions()) != 2 {
		t.Fatal("结果应带回追加后的判断记录")
	}
}

// Covers: CONTEXT「名称在参与方册上找不到对应身份时，不据名称铸身份，形成待确认（承运主体身份未登记），
// 名称作为素材随依据保留」与生命周期「待确认（承运主体身份未登记）→ 已识别：PC 登记了对应身份后，凭
// 同一份证据形成新版本」——同一条命令在登记前后各来一次，前一次留素材、后一次补认成新版本。
func TestAnUnregisteredSubjectStaysPendingUntilTheDirectoryKnowsItThenTheSameEvidenceRecognisesIt(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t)
	command := scanCommand(t)

	first, err := fixture.handler.Form(t.Context(), command)
	if err != nil || first.Outcome() != application.CarrierJudgmentVersionFormed {
		t.Fatalf("未登记：outcome=%q err=%v", first.Outcome(), err)
	}
	judgment := fixture.currentJudgment(t)
	if reason, pending := judgment.Current().Verdict().Pending(); !pending || reason != domain.CarrierIdentityNotRegistered {
		t.Fatalf("应为待确认（承运主体身份未登记），实得 pending=%v reason=%s", pending, reason)
	}
	if got := judgment.Current().Bases()[0].Material(); got != "party/carrier-x" {
		t.Fatalf("未登记的引用应作素材随依据保留：%q", got)
	}

	// 同一份证据再来一次而身份仍未登记：重放，不多出一版。
	replay, err := fixture.handler.Form(t.Context(), command)
	if err != nil || replay.Outcome() != application.CarrierEvidenceAlreadyConsidered {
		t.Fatalf("身份未变的重放：outcome=%q err=%v，want ALREADY_CONSIDERED", replay.Outcome(), err)
	}

	// PC 登记之后，凭同一份证据补认：新版本已识别，形成时间是现在，业务时间仍是证据时间。
	fixture.directory.registered[domain.ExternalCarrierParty.String()+"|party/carrier-x"] = true
	fixture.clock.at = carrierJudgedAt.Add(24 * time.Hour)
	recognised, err := fixture.handler.Form(t.Context(), command)
	if err != nil || recognised.Outcome() != application.CarrierJudgmentVersionFormed {
		t.Fatalf("登记后补认：outcome=%q err=%v", recognised.Outcome(), err)
	}
	judgment = fixture.currentJudgment(t)
	if len(judgment.Versions()) != 3 {
		t.Fatalf("补认应追加成第三版，实得 %d", len(judgment.Versions()))
	}
	subject, identified := judgment.Current().Verdict().Identified()
	if !identified || subject != carrierX(t) {
		t.Fatalf("补认后应识别为 carrier-x：identified=%v subject=%+v", identified, subject)
	}
	if !judgment.Current().FormedAt().Equal(carrierJudgedAt.Add(24*time.Hour)) || !judgment.Current().BusinessTime().Equal(carrierEvidenceAt) {
		t.Fatalf("形成时间 %s / 业务时间 %s", judgment.Current().FormedAt(), judgment.Current().BusinessTime())
	}
	if judgment.Versions()[1].Bases()[0].Material() != "party/carrier-x" {
		t.Fatal("未登记期间那一版被追溯改写了")
	}
}

// 只给名称素材、不给引用：不问身份读口——名称不是身份，本上下文也不按名称去册上找。
func TestNameMaterialAloneFormsAPendingVersionWithoutAskingTheDirectory(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	command := scanCommand(t)
	command.SubjectKind, command.SubjectReference, command.NameMaterial = domain.CarrierSubjectKindInvalid, "", "Carrier X Express"

	result, err := fixture.handler.Form(t.Context(), command)
	if err != nil || result.Outcome() != application.CarrierJudgmentVersionFormed {
		t.Fatalf("outcome=%q err=%v", result.Outcome(), err)
	}
	judgment := fixture.currentJudgment(t)
	if reason, pending := judgment.Current().Verdict().Pending(); !pending || reason != domain.CarrierIdentityNotRegistered {
		t.Fatalf("pending=%v reason=%s", pending, reason)
	}
	if judgment.Current().Bases()[0].Material() != "Carrier X Express" || len(fixture.directory.asked) != 0 {
		t.Fatalf("素材 %q，读口被问 %d 次", judgment.Current().Bases()[0].Material(), len(fixture.directory.asked))
	}
}

// Covers: CONTEXT「业务时间不得早于该段成立时间」——单独一格：恢复动作是去看这条证据该归哪一段，不是改输入。
func TestEvidenceBeforeTheSegmentWasEstablishedIsAnsweredAsPrecedingTheSegment(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	command := scanCommand(t)
	command.OccurredAt = pickupOccurredAt.Add(-time.Minute)

	result, err := fixture.handler.Form(t.Context(), command)
	if err != nil || result.Outcome() != application.CarrierEvidencePrecedesSegment {
		t.Fatalf("outcome=%q err=%v, want EVIDENCE_PRECEDES_SEGMENT", result.Outcome(), err)
	}
	if fixture.judgments.appends != 0 {
		t.Fatal("被拒的证据不该落任何版本")
	}
}

// 同一份依据、身份状态未变：重放答已考虑，不追加。
func TestReplayingTheSameEvidenceDoesNotAppendAVersion(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	if _, err := fixture.handler.Form(t.Context(), scanCommand(t)); err != nil {
		t.Fatalf("first: %v", err)
	}
	result, err := fixture.handler.Form(t.Context(), scanCommand(t))
	if err != nil || result.Outcome() != application.CarrierEvidenceAlreadyConsidered {
		t.Fatalf("outcome=%q err=%v, want ALREADY_CONSIDERED", result.Outcome(), err)
	}
	if fixture.judgments.appends != 1 {
		t.Fatalf("appends = %d, want 1", fixture.judgments.appends)
	}
}

// Covers: CONTEXT 生命周期「实际履约段结束 → 判断历史封存：不再接受新版本，除来源事实更正引起的重新派生」。
// 封存不是一格存下来的标记，是段的关闭状态派生出来的——段登记册说段已关闭，判断就不收新证据。
func TestAClosedSegmentSealsItsJudgmentAgainstNewEvidence(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	rows := fixture.segments.rows[segmentRegistryKey(segmentKeyOf(t, "tenant-1", "segment-1"))]
	rows.participations[0].EndKind = domain.EndedByControlTermination
	rows.participations[0].EndBasis = value(t, domain.NewParticipationBasisReference, "TERMINATION/1")
	rows.participations[0].EndedAt = pickupOccurredAt.Add(time.Hour)
	rows.closed, rows.closedAt = true, pickupOccurredAt.Add(time.Hour)

	result, err := fixture.handler.Form(t.Context(), scanCommand(t))
	if err != nil || result.Outcome() != application.CarrierJudgmentSealed {
		t.Fatalf("outcome=%q err=%v, want JUDGMENT_SEALED", result.Outcome(), err)
	}
	if fixture.judgments.appends != 0 || len(fixture.directory.asked) != 0 {
		t.Fatal("封存的判断不该去问身份也不该追加")
	}
}

func TestAnUnknownSegmentIsAnsweredAsSegmentNotFound(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	command := scanCommand(t)
	command.Segment = "segment-never"

	result, err := fixture.handler.Form(t.Context(), command)
	if err != nil || result.Outcome() != application.CarrierJudgmentSegmentNotFound {
		t.Fatalf("outcome=%q err=%v, want SEGMENT_NOT_FOUND", result.Outcome(), err)
	}
}

// 段在册而判断不在册——挂点那一半曾欠过账：本用例补开首版再追加，形成时间如实是现在，业务时间是段成立时刻。
func TestAMissingJudgmentIsOpenedBeforeTheEvidenceIsConsidered(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t, carrierX(t))
	delete(fixture.judgments.rows, judgmentRegistryKey(judgmentKeyFixture(t, "tenant-1", "segment-1")))

	result, err := fixture.handler.Form(t.Context(), scanCommand(t))
	if err != nil || result.Outcome() != application.CarrierJudgmentVersionFormed {
		t.Fatalf("outcome=%q err=%v", result.Outcome(), err)
	}
	judgment := fixture.currentJudgment(t)
	if len(judgment.Versions()) != 2 {
		t.Fatalf("补开首版再追加应两版，实得 %d", len(judgment.Versions()))
	}
	first := judgment.Versions()[0]
	if reason, pending := first.Verdict().Pending(); !pending || reason != domain.NoQualifiedCarrierEvidence {
		t.Fatal("补开的首版应为待确认（无合格证据）")
	}
	if !first.BusinessTime().Equal(pickupOccurredAt) || !first.FormedAt().Equal(carrierJudgedAt) {
		t.Fatalf("补开首版业务时间 %s / 形成时间 %s", first.BusinessTime(), first.FormedAt())
	}
}

// 依赖调不通一律`未决`并留续办引用，绝不折成某一格业务答案（ADR-0029）：把身份读口读不到折成「未登记」，
// 一次故障就会变成一版待确认。
func TestDependencyFailuresLeaveTheJudgmentUndecided(t *testing.T) {
	unavailable := errors.New("读不到")
	cases := map[string]struct {
		breakOne func(*carrierJudgmentFixture)
		reason   application.CarrierJudgmentUndecidedReason
	}{
		"身份读口读不通": {
			breakOne: func(fixture *carrierJudgmentFixture) { fixture.directory.err = unavailable },
			reason:   application.CarrierIdentityDirectoryUnavailable,
		},
		"判断登记册读不通": {
			breakOne: func(fixture *carrierJudgmentFixture) { fixture.judgments.findErr = unavailable },
			reason:   application.CarrierJudgmentRegistryUnavailable,
		},
		"判断登记册写不进": {
			breakOne: func(fixture *carrierJudgmentFixture) { fixture.judgments.appendErr = unavailable },
			reason:   application.CarrierJudgmentRegistryUnavailable,
		},
		"段登记册读不通": {
			breakOne: func(fixture *carrierJudgmentFixture) { fixture.segments.findErr = unavailable },
			reason:   application.CarrierJudgmentRegistryUnavailable,
		},
		"并发撞序号——另一次追加先落了同一版": {
			breakOne: func(fixture *carrierJudgmentFixture) {
				fixture.judgments.forceAppend = ports.JudgmentVersionAlreadyRecorded
			},
			reason: application.CarrierJudgmentVersionRaced,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newCarrierJudgmentFixture(t, carrierX(t))
			test.breakOne(fixture)

			result, err := fixture.handler.Form(t.Context(), scanCommand(t))
			if err != nil {
				t.Fatalf("form: %v", err)
			}
			if result.Outcome() != application.CarrierJudgmentUndecided || result.UndecidedReason() != test.reason {
				t.Fatalf("outcome=%q reason=%q, want UNDECIDED/%s", result.Outcome(), result.UndecidedReason(), test.reason)
			}
			if result.ContinuationReference() == "" {
				t.Fatal("`未决`没留续办引用")
			}
		})
	}
}

// 身份读口未装而命令给了引用：答`未决`而不是「未登记」——装配缺件不是业务答案。
func TestAMissingDirectoryIsUndecidedNotUnregistered(t *testing.T) {
	fixture := newCarrierJudgmentFixture(t)
	fixture.handler = application.NewFormActualCarrierJudgmentHandler(application.FormActualCarrierJudgmentDeps{
		Segments:  fixture.segments,
		Judgments: fixture.judgments,
		Clock:     fixture.clock,
	})
	result, err := fixture.handler.Form(t.Context(), scanCommand(t))
	if err != nil || result.Outcome() != application.CarrierJudgmentUndecided || result.UndecidedReason() != application.CarrierIdentityDirectoryUnavailable {
		t.Fatalf("outcome=%q reason=%q err=%v", result.Outcome(), result.UndecidedReason(), err)
	}
}

func TestMalformedCommandsAreNotAccepted(t *testing.T) {
	cases := map[string]func(*application.FormActualCarrierJudgmentCommand){
		"缺租户":  func(command *application.FormActualCarrierJudgmentCommand) { command.TenantID = domain.TenantID{} },
		"缺段引用": func(command *application.FormActualCarrierJudgmentCommand) { command.Segment = " " },
		"来源不在四格内": func(command *application.FormActualCarrierJudgmentCommand) {
			command.Source = domain.CarrierEvidenceSourceInvalid
		},
		"缺证据引用": func(command *application.FormActualCarrierJudgmentCommand) { command.EvidenceReference = "" },
		"缺业务时间": func(command *application.FormActualCarrierJudgmentCommand) { command.OccurredAt = time.Time{} },
		"引用有而分支缺": func(command *application.FormActualCarrierJudgmentCommand) {
			command.SubjectKind = domain.CarrierSubjectKindInvalid
		},
		"引用与素材都缺": func(command *application.FormActualCarrierJudgmentCommand) { command.SubjectReference = "" },
		"分支有而引用与素材都缺": func(command *application.FormActualCarrierJudgmentCommand) {
			command.SubjectReference, command.NameMaterial = "", ""
		},
	}
	for name, breakOne := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newCarrierJudgmentFixture(t, carrierX(t))
			command := scanCommand(t)
			breakOne(&command)
			result, err := fixture.handler.Form(t.Context(), command)
			if err != nil || result.Outcome() != application.CarrierJudgmentNotAccepted {
				t.Fatalf("outcome=%q err=%v, want NOT_ACCEPTED", result.Outcome(), err)
			}
			if fixture.judgments.appends != 0 || fixture.judgments.opens != 1 {
				t.Fatal("未受理的命令碰了判断登记册")
			}
		})
	}
}
