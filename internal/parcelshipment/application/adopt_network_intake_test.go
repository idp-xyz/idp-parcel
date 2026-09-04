package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var intakeHappenedAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func intakeSourceSpec(t *testing.T) domain.IntakeSourceSpec {
	t.Helper()
	return domain.IntakeSourceSpec{
		Kind:       domain.NodeIntakeSource,
		Object:     mustValue(t, domain.NewSourceObjectReference, "handling-unit-1"),
		Parcel:     mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Place:      mustValue(t, domain.NewIntakePlaceReference, "node-origin"),
		Control:    mustValue(t, domain.NewIntakeControlReference, "NODE-CONTROL/NO-7"),
		Version:    mustValue(t, domain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: intakeHappenedAt,
	}
}

func adoptCommand(t *testing.T) application.AdoptNetworkIntakeCommand {
	t.Helper()
	return application.AdoptNetworkIntakeCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Source:            intakeSourceSpec(t),
	}
}

type intakeEligibilityDouble struct {
	eligibility ports.IntakeEligibility
	configured  bool
	err         error
	asked       int
}

func (double *intakeEligibilityDouble) JudgeIntakeEligibility(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequestID,
	_ domain.IntakeSource,
) (ports.IntakeEligibility, bool, error) {
	double.asked++
	if double.err != nil {
		return ports.IntakeEligibility{}, false, double.err
	}
	return double.eligibility, double.configured, nil
}

type adoptionStoreDouble struct {
	byKey map[ports.IntakeAdoptionKey]ports.IntakeAdoptionRecord
	saved int
}

func newAdoptionStore() *adoptionStoreDouble {
	return &adoptionStoreDouble{byKey: map[ports.IntakeAdoptionKey]ports.IntakeAdoptionRecord{}}
}

func (double *adoptionStoreDouble) FindByKey(
	_ context.Context,
	key ports.IntakeAdoptionKey,
) (ports.IntakeAdoptionRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

// FindResponsibilityStart 答链尾：adopted 且没有别的 adopted 行回指它（与真库读口同一条规则）。
func (double *adoptionStoreDouble) FindResponsibilityStart(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (ports.IntakeAdoptionRecord, bool, error) {
	for _, record := range double.byKey {
		if record.Key.TenantID != tenant || record.Key.Parcel != parcel || !record.Adopted {
			continue
		}
		superseded := false
		for _, other := range double.byKey {
			supersedes, chained := other.Supersedes()
			if other.Adopted && chained && other.Key.TenantID == tenant && other.Key.Parcel == parcel &&
				other.Key.Kind == record.Key.Kind && supersedes == record.Key.Version {
				superseded = true
				break
			}
		}
		if !superseded {
			return record, true, nil
		}
	}
	return ports.IntakeAdoptionRecord{}, false, nil
}

// Save 照真库的三处撞墙译`已有记录`：主键、第二个根、同一前版第二次被取代。
func (double *adoptionStoreDouble) Save(
	_ context.Context,
	record ports.IntakeAdoptionRecord,
) (ports.IntakeAdoptionSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.IntakeAdoptionAlreadyRecorded, nil
	}
	if record.Adopted {
		supersedes, chained := record.Supersedes()
		for _, other := range double.byKey {
			if !other.Adopted || other.Key.TenantID != record.Key.TenantID || other.Key.Parcel != record.Key.Parcel {
				continue
			}
			otherSupersedes, otherChained := other.Supersedes()
			if !chained && !otherChained {
				return ports.IntakeAdoptionAlreadyRecorded, nil
			}
			if chained && otherChained && other.Key.Kind == record.Key.Kind && otherSupersedes == supersedes {
				return ports.IntakeAdoptionAlreadyRecorded, nil
			}
		}
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.IntakeAdoptionSaved, nil
}

type intakeDownstreamDouble struct {
	intents []ports.NetworkIntakeHandoffIntent
	err     error
}

func (double *intakeDownstreamDouble) HandOffNetworkIntake(
	_ context.Context,
	intent ports.NetworkIntakeHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type commitmentIdentityDouble struct{ next int }

func (double *commitmentIdentityDouble) NextCommitmentVersionID(_ context.Context) (domain.CommitmentVersionID, error) {
	double.next++
	return domain.NewCommitmentVersionID("commitment-" + string(rune('0'+double.next)))
}

type intakeFixture struct {
	handler       *application.AdoptNetworkIntakeHandler
	requests      *shipmentRequestRepositoryDouble
	eligibility   *intakeEligibilityDouble
	adoptions     *adoptionStoreDouble
	downstream    *intakeDownstreamDouble
	cancellations *cancellationViewDouble
}

func newIntakeFixture(t *testing.T) *intakeFixture {
	t.Helper()
	fixture := &intakeFixture{
		requests: &shipmentRequestRepositoryDouble{
			records: map[domain.SourceIdentity]domain.ShipmentRequest{},
			record:  func(string) {},
		},
		eligibility: &intakeEligibilityDouble{
			eligibility: ports.IntakeEligibility{Outcome: ports.IntakeEligibilityEstablished},
			configured:  true,
		},
		adoptions:     newAdoptionStore(),
		downstream:    &intakeDownstreamDouble{},
		cancellations: &cancellationViewDouble{},
	}
	fixture.requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = acceptedRequest(t)
	fixture.handler = application.NewAdoptNetworkIntakeHandler(application.AdoptNetworkIntakeDeps{
		Requests:      fixture.requests,
		Eligibility:   fixture.eligibility,
		Adoptions:     fixture.adoptions,
		Identities:    &commitmentIdentityDouble{},
		Downstream:    fixture.downstream,
		Clock:         fixedClock{at: intakeHappenedAt.Add(time.Minute)},
		Cancellations: fixture.cancellations,
	})
	return fixture
}

// Covers: `AT-PS-038`「已接受网络服务包裹取得合格节点收寄结果——以节点收寄业务时间形成
// 有效网络收寄、责任起点和正式承诺」——承诺生效恒等于物理发生时间（处理时钟晚一分钟，
// 生效时间纹丝不动），采用记录连意图一并交付。
func TestAQualifiedIntakeFormsTheCommitmentAtThePhysicalTime(t *testing.T) {
	fixture := newIntakeFixture(t)

	result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q, want COMMITMENT_FORMED", result.Outcome())
	}
	record, present := result.Record()
	if !present || !record.Adopted {
		t.Fatalf("record = %#v present = %v", record, present)
	}
	if !record.Commitment.EffectiveAt().Equal(intakeHappenedAt) {
		t.Fatalf("effective at = %s, want the physical occurrence time %s",
			record.Commitment.EffectiveAt(), intakeHappenedAt)
	}
	if !record.Intake.ResponsibilityStart().Equal(intakeHappenedAt) {
		t.Fatal("责任起点不是物理收寄时间")
	}
	if record.Commitment.Expected().String() == "" {
		t.Fatal("正式承诺没有引用预计承诺")
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.downstream.intents))
	}
}

// Covers: `AT-PS-040`「多包裹委托只有一个包裹先收寄——只为该包裹形成正式承诺，不等待
// 或代替其他包裹」——先收寄的成员立即成立承诺，另一成员各凭各的来源独立成立；两份
// 责任起点互不相扰（逐包裹提交是一致性节的硬句）。
func TestOneParcelsIntakeNeitherWaitsForNorCoversItsSiblings(t *testing.T) {
	fixture := newIntakeFixture(t)

	first, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("first parcel handle: %v", err)
	}
	if first.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q; 先收寄的包裹不等同委托其他成员", first.Outcome())
	}
	record, _ := first.Record()
	if record.Key.Parcel.String() != "parcel-1" {
		t.Fatalf("parcel = %q", record.Key.Parcel)
	}

	sibling := adoptCommand(t)
	sibling.Source.Parcel = mustValue(t, domain.NewDeclaredParcelID, "parcel-2")
	sibling.Source.Object = mustValue(t, domain.NewSourceObjectReference, "handling-unit-2")
	sibling.Source.Version = mustValue(t, domain.NewSourceResultVersion, "intake-result/v9")
	second, err := fixture.handler.Handle(context.Background(), sibling)
	if err != nil {
		t.Fatalf("sibling handle: %v", err)
	}
	if second.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q; 成员各凭各的来源独立成立", second.Outcome())
	}
	if fixture.adoptions.saved != 2 {
		t.Fatalf("saved = %d; 两个成员该有两份互不相扰的采用记录", fixture.adoptions.saved)
	}
	siblingRecord, _ := second.Record()
	if siblingRecord.Commitment.Version() == record.Commitment.Version() {
		t.Fatal("两个成员共用了一份承诺版本")
	}

	// 负向半：后续来源停在资格未决——已成立的两份承诺不回滚（「一个成员未决或不采用
	// 不回滚其他成员已经形成的正式承诺」）。资格检查先于责任起点检查，所以这一探停在
	// 未决而不是不采用。
	fixture.eligibility.configured = false
	stalled := adoptCommand(t)
	stalled.Source.Kind = domain.OffsitePickupSource
	stalled.Source.Version = mustValue(t, domain.NewSourceResultVersion, "pickup-result/v1")
	third, err := fixture.handler.Handle(context.Background(), stalled)
	if err != nil {
		t.Fatalf("stalled handle: %v", err)
	}
	if third.Outcome() != application.IntakeEligibilityUndecided {
		t.Fatalf("outcome = %q", third.Outcome())
	}
	if fixture.adoptions.saved != 2 {
		t.Fatalf("saved = %d; 未决的来源动了别人的记录", fixture.adoptions.saved)
	}
	survivor, found, err := fixture.adoptions.FindByKey(context.Background(), record.Key)
	if err != nil || !found || !survivor.Adopted {
		t.Fatalf("survivor = %#v found = %v err = %v; 已成承诺被回滚了", survivor, found, err)
	}
}

// Covers: `AT-PS-041`「同一来源版本重复交付——返回原采用结果和承诺，不形成第二责任起点」
// 与 `AT-PS-042`「同一采用身份携带不同来源内容——形成冲突并保留原结果」。
func TestAReplayReturnsTheOriginalAndAConflictOverwritesNothing(t *testing.T) {
	fixture := newIntakeFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), adoptCommand(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	replay, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.IntakeExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	if fixture.adoptions.saved != 1 {
		t.Fatalf("saved = %d; 重放形成了第二责任起点", fixture.adoptions.saved)
	}

	conflicting := adoptCommand(t)
	conflicting.Source.Place = mustValue(t, domain.NewIntakePlaceReference, "node-elsewhere")
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.IntakeSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
	}
	if fixture.adoptions.saved != 1 {
		t.Fatal("冲突覆盖了原结果")
	}
}

// Covers: `AT-PS-049`「同一包裹同时出现节点收寄和场外揽收来源——只形成一个责任起点，
// 另一来源进入重复/冲突判断」：后到的场外揽收不采用，原因指名先到者。
func TestASecondSourceKindDoesNotStartASecondResponsibility(t *testing.T) {
	fixture := newIntakeFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), adoptCommand(t)); err != nil {
		t.Fatalf("node intake handle: %v", err)
	}

	offsite := adoptCommand(t)
	offsite.Source.Kind = domain.OffsitePickupSource
	offsite.Source.Version = mustValue(t, domain.NewSourceResultVersion, "pickup-result/v1")
	offsite.Source.Control = mustValue(t, domain.NewIntakeControlReference, "TRANSPORT-CONTROL/TF-3")

	result, err := fixture.handler.Handle(context.Background(), offsite)
	if err != nil {
		t.Fatalf("offsite handle: %v", err)
	}
	if result.Outcome() != application.IntakeSourceNotAdopted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ADOPTED", result.Outcome())
	}
	if result.Basis().String() != "RESPONSIBILITY_ALREADY_STARTED/NODE_INTAKE/intake-result/v1" {
		t.Fatalf("basis = %q; 不采用必须指名先到的责任起点", result.Basis())
	}
}

// pickupCommand 组一份场外揽收来源；version 与 corrects 决定它是首登还是更正版本。
func pickupCommand(t *testing.T, version, corrects string, occurredAt time.Time) application.AdoptNetworkIntakeCommand {
	t.Helper()
	command := adoptCommand(t)
	command.Source.Kind = domain.OffsitePickupSource
	command.Source.Object = mustValue(t, domain.NewSourceObjectReference, "parcel-1")
	command.Source.Control = mustValue(t, domain.NewIntakeControlReference, "OFFSITE-PICKUP/TF-3")
	command.Source.Version = mustValue(t, domain.NewSourceResultVersion, version)
	command.Source.OccurredAt = occurredAt
	if corrects != "" {
		command.Source.Corrects = mustValue(t, domain.NewSourceResultVersion, corrects)
	}
	return command
}

// Covers: `AT-PS-050`「来源后来被更正——形成新采用判断和必要的承诺调整版本，不删除原历史」
// （ADR-0117 决定一至三）：同种类来源自报更正当前采用的那一版，形成新的采用记录回指前版，
// 承诺在前版上重述——新版本号、指回前版、原因点名被更正的来源版本、生效时间随更正后的发生
// 时刻；根记录一字不动；两代各交一份意图。
func TestASameSourceCorrectionSupersedesTheResponsibilityStart(t *testing.T) {
	fixture := newIntakeFixture(t)
	first, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v1", "", intakeHappenedAt))
	if err != nil || first.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("first pickup: outcome = %q err = %v", first.Outcome(), err)
	}
	root, _ := first.Record()

	correctedAt := intakeHappenedAt.Add(-20 * time.Minute)
	result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v2", "pickup-result/v1", correctedAt))
	if err != nil {
		t.Fatalf("corrected pickup: %v", err)
	}
	if result.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q（依据 %q），想要 COMMITMENT_FORMED——同来源更正不是第二责任起点", result.Outcome(), result.Basis())
	}
	record, present := result.Record()
	if !present || !record.Adopted {
		t.Fatalf("record = %#v present = %v", record, present)
	}
	if supersedes, chained := record.Supersedes(); !chained || supersedes.String() != "pickup-result/v1" {
		t.Fatalf("supersedes = %s chained = %v; 更正版没回指它取代的那一版", supersedes, chained)
	}
	if prior, restated := record.Commitment.PriorVersion(); !restated || prior != root.Commitment.Version() {
		t.Fatalf("prior = %s restated = %v; 承诺没指回根采用的承诺版本", prior, restated)
	}
	if reason, _ := record.Commitment.AdjustmentReason(); reason.String() != "SOURCE_CORRECTED/OFFSITE_PICKUP/pickup-result/v1" {
		t.Fatalf("reason = %q", reason)
	}
	if !record.Commitment.EffectiveAt().Equal(correctedAt) || !record.Intake.ResponsibilityStart().Equal(correctedAt) {
		t.Fatalf("effective at = %s; 生效时间必须随更正后的发生时刻走", record.Commitment.EffectiveAt())
	}
	if record.Commitment.Version() == root.Commitment.Version() || record.Commitment.Expected() != root.Commitment.Expected() {
		t.Fatal("承诺版本没换号，或预计承诺引用被换掉")
	}

	untouched, found, err := fixture.adoptions.FindByKey(context.Background(), root.Key)
	if err != nil || !found || !untouched.Adopted || !untouched.Commitment.EffectiveAt().Equal(intakeHappenedAt) {
		t.Fatalf("根记录被改写了：%#v found=%v err=%v", untouched, found, err)
	}
	if _, chained := untouched.Supersedes(); chained {
		t.Fatal("根记录凭空长出了回指")
	}
	tail, _, _ := fixture.adoptions.FindResponsibilityStart(context.Background(), root.Key.TenantID, root.Key.Parcel)
	if tail.Key.Version.String() != "pickup-result/v2" {
		t.Fatalf("链尾 = %s，想要更正版", tail.Key.Version)
	}
	if len(fixture.downstream.intents) != 2 || fixture.adoptions.saved != 2 {
		t.Fatalf("intents = %d saved = %d; 两代各一份记录与意图", len(fixture.downstream.intents), fixture.adoptions.saved)
	}
}

// Covers: `AT-PS-049` 在链之后一字不动（ADR-0117 决定五）：责任起点已由更正版接续，另一来源
// 种类照旧不采用，依据指名链尾那一版；同种类而不声明更正关系的新版本（同一对象的新一次尝试）
// 同样是竞争，不是更正。
func TestACompetingSourceIsStillRefusedAfterACorrection(t *testing.T) {
	fixture := newIntakeFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v1", "", intakeHappenedAt)); err != nil {
		t.Fatalf("first pickup: %v", err)
	}
	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v2", "pickup-result/v1", intakeHappenedAt.Add(-time.Minute))); err != nil {
		t.Fatalf("corrected pickup: %v", err)
	}

	node, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("node intake: %v", err)
	}
	if node.Outcome() != application.IntakeSourceNotAdopted || node.Basis().String() != "RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/pickup-result/v2" {
		t.Fatalf("outcome = %q basis = %q; 另一来源种类必须照旧不采用且指名链尾", node.Outcome(), node.Basis())
	}

	attempt := pickupCommand(t, "pickup-result/v7", "", intakeHappenedAt.Add(time.Hour))
	again, err := fixture.handler.Handle(context.Background(), attempt)
	if err != nil {
		t.Fatalf("second attempt: %v", err)
	}
	if again.Outcome() != application.IntakeSourceNotAdopted || again.Basis().String() != "RESPONSIBILITY_ALREADY_STARTED/OFFSITE_PICKUP/pickup-result/v2" {
		t.Fatalf("outcome = %q basis = %q; 键相同而不声明更正不是更正", again.Outcome(), again.Basis())
	}
	if fixture.adoptions.saved != 4 {
		t.Fatalf("saved = %d; 两份拒绝各自落记录", fixture.adoptions.saved)
	}
}

// Covers: ADR-0117 决定五的两格例外——更正所指的前版尚无任何记录：未决不落库（重投会改变结果）；
// 前版有记录但不是链尾（已被取代、或本就是不采用行）：不采用，依据点名链尾。
func TestACorrectionMustPointAtTheCurrentResponsibilityStart(t *testing.T) {
	fixture := newIntakeFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v1", "", intakeHappenedAt)); err != nil {
		t.Fatalf("first pickup: %v", err)
	}

	early, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v3", "pickup-result/v2", intakeHappenedAt))
	if err != nil {
		t.Fatalf("out-of-order correction: %v", err)
	}
	if early.Outcome() != application.IntakeEligibilityUndecided || early.UndecidedReason() != application.IntakeCorrectionPredecessorUnjudged {
		t.Fatalf("outcome = %q/%q; 前版还没判过的更正只能未决", early.Outcome(), early.UndecidedReason())
	}
	if fixture.adoptions.saved != 1 {
		t.Fatal("未决落了库")
	}

	if _, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v2", "pickup-result/v1", intakeHappenedAt.Add(-time.Minute))); err != nil {
		t.Fatalf("v2 correction: %v", err)
	}
	fork, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v4", "pickup-result/v1", intakeHappenedAt.Add(-2*time.Minute)))
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if fork.Outcome() != application.IntakeSourceNotAdopted || fork.Basis().String() != "CORRECTION_TARGET_NOT_CURRENT/OFFSITE_PICKUP/pickup-result/v2" {
		t.Fatalf("outcome = %q basis = %q; 更正已被取代的一版是分叉", fork.Outcome(), fork.Basis())
	}

	ontoRefusal, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v5", "pickup-result/v4", intakeHappenedAt))
	if err != nil {
		t.Fatalf("correction of a refusal: %v", err)
	}
	if ontoRefusal.Outcome() != application.IntakeSourceNotAdopted || ontoRefusal.Basis().String() != "CORRECTION_TARGET_NOT_CURRENT/OFFSITE_PICKUP/pickup-result/v2" {
		t.Fatalf("outcome = %q basis = %q; 更正一份不采用行接不上链", ontoRefusal.Outcome(), ontoRefusal.Basis())
	}
}

// Covers: ADR-0117 决定一的边界——没有责任起点时，更正声明不改变什么：更正版本按首登走，采用为根
// （前版若曾被拒，它的拒绝行留着，链从更正版起头）。
func TestACorrectionWithoutAResponsibilityStartAdoptsAsTheRoot(t *testing.T) {
	fixture := newIntakeFixture(t)
	fixture.eligibility.configured = false
	if result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v1", "", intakeHappenedAt)); err != nil || result.Outcome() != application.IntakeEligibilityUndecided {
		t.Fatalf("first pickup: outcome = %q err = %v", result.Outcome(), err)
	}
	fixture.eligibility.configured = true

	result, err := fixture.handler.Handle(context.Background(), pickupCommand(t, "pickup-result/v2", "pickup-result/v1", intakeHappenedAt.Add(-time.Minute)))
	if err != nil {
		t.Fatalf("correction: %v", err)
	}
	if result.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q（%q）", result.Outcome(), result.Basis())
	}
	record, _ := result.Record()
	if _, chained := record.Supersedes(); chained {
		t.Fatal("没有前一版采用可接，却长出了回指")
	}
	if _, restated := record.Commitment.PriorVersion(); restated {
		t.Fatal("根采用的承诺凭空带了前版")
	}
}

// Covers: `AT-PS-047`「收寄来源成立但资格规则未配置——保持资格未决，不默认承诺」；资格
// 明确未成立同样未决带依据；`不适用`带产品依据且与资格失败分格。
func TestEligibilityGapsStallWithoutDefaultingToACommitment(t *testing.T) {
	t.Run("rules not configured", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.eligibility.configured = false

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeEligibilityUndecided ||
			result.UndecidedReason() != application.IntakeEligibilityUnconfigured {
			t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
		}
		if fixture.adoptions.saved != 0 {
			t.Fatal("未决落了库")
		}
	})

	t.Run("hard eligibility not established", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.eligibility.eligibility = ports.IntakeEligibility{
			Outcome: ports.IntakeEligibilityNotEstablished,
			Basis:   mustValue(t, domain.NewCheckReason, "INTAKE_QUALIFICATION_MISSING/PC-16"),
		}

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeEligibilityUndecided ||
			result.UndecidedReason() != application.IntakeEligibilityNotEstablishedYet {
			t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
		}
		if result.Basis().String() != "INTAKE_QUALIFICATION_MISSING/PC-16" {
			t.Fatal("未决没带资格缺口依据")
		}
	})

	t.Run("service not applicable", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.eligibility.eligibility = ports.IntakeEligibility{
			Outcome: ports.IntakeServiceNotApplicable,
			Basis:   mustValue(t, domain.NewCheckReason, "PRODUCT-WAYBILL-ONLY"),
		}

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeNotApplicable ||
			result.Basis().String() != "PRODUCT-WAYBILL-ONLY" {
			t.Fatalf("outcome = %q basis = %q", result.Outcome(), result.Basis())
		}
	})
}

// Covers: `AT-PS-052`「跨租户或客户提交来源采用请求——拒绝越权且不泄露」（查无与越权
// 同答）与「委托未接受/基线换代的来源不采用」——不采用记录保留原因，物理事实只读。
//
// 「同答」用两探同形比对钉住：越权探（目标存在但客户不对）与真查无探（目标根本不存在）
// 的完整结果结构必须逐字段相同——任何一个可分辨面（原因、依据、续办引用）都够攻击者
// 枚举别人的委托。存储今天按完整身份键查找让它结构性成立；这条测试守的是「换键型
// （如按编号全局索引）时同形性不得静默失守」。
func TestForeignOrUnacceptedTargetsRefuseWithoutLeaking(t *testing.T) {
	t.Run("a foreign probe and a miss probe are indistinguishable", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		foreign := adoptCommand(t)
		// 目标委托真实存在（tenant-1/customer-1），探针换了客户——若存储按编号索引，
		// 这一探能命中并可能答出「编号不符」之类的可分辨面。
		foreign.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-1")
		foreignResult, err := fixture.handler.Handle(context.Background(), foreign)
		if err != nil {
			t.Fatalf("foreign probe: %v", err)
		}

		miss := adoptCommand(t)
		miss.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-nonexistent")
		missResult, err := fixture.handler.Handle(context.Background(), miss)
		if err != nil {
			t.Fatalf("miss probe: %v", err)
		}

		if foreignResult != missResult {
			t.Fatalf("foreign = %#v miss = %#v; 两探必须同形，否则可枚举", foreignResult, missResult)
		}
		if foreignResult.Outcome() != application.IntakeRequestNotAccepted {
			t.Fatalf("outcome = %q, want REQUEST_NOT_ACCEPTED", foreignResult.Outcome())
		}
	})

	t.Run("a submitted-but-undecided request is not a responsibility start", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = submittedRequest(t)

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeSourceNotAdopted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ADOPTED", result.Outcome())
		}
		if result.Basis().String() != "REQUEST_NOT_ACCEPTED/SUBMITTED" {
			t.Fatalf("basis = %q", result.Basis())
		}
	})

	t.Run("a superseded baseline does not anchor an adoption", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		command := adoptCommand(t)
		command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-9")

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeSourceNotAdopted ||
			result.Basis().String() != "BASELINE_SUPERSEDED/version-1" {
			t.Fatalf("outcome = %q basis = %q", result.Outcome(), result.Basis())
		}
	})
}

type cancellationViewDouble struct {
	cancellation domain.ParcelCancellation
	cancelled    bool
	err          error
}

func (double *cancellationViewDouble) FindCancellation(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DeclaredParcelID,
) (domain.ParcelCancellation, bool, error) {
	if double.err != nil {
		return domain.ParcelCancellation{}, false, double.err
	}
	return double.cancellation, double.cancelled, nil
}

func cancelledAt(t *testing.T, at time.Time) domain.ParcelCancellation {
	t.Helper()
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "cancel-1"),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-1"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/PC-17"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CUSTOMER_CHANGED_MIND"),
		RequestedAt: at,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide parcel cancellation: %v", err)
	}
	return cancellation
}

// Covers: `AT-PS-044`「包裹在收寄事实发生前已经合法取消——保留物理来源但不重开服务」
// 与 `AT-PS-080`「取消先成立，迟到消息证明实物后来到站——取消不被覆盖」：取消决定早于
// 收寄发生即不采用、原因指名取消决定。
//
// 晚于格锚 UC-PS-006 一致性硬句「收寄与取消并发时，以不可覆盖业务决定边界及权威事实
// 的业务发生顺序裁决，不以消息到达或客户端时间最后写入覆盖」——收寄发生早于取消决定
// 说明取消形成时权威事实的顺序已经矛盾（AT-PS-079 要求那时不形成取消），采用侧对这份
// 已成取消停未决是它的镜像防线；不选边，留给事实核对续办。AT-PS-081 的「来源冲突」
// 说的是收寄事实本身的证据冲突，机制未建，此处不锚它。
func TestTheCancellationBoundaryJudgesByBusinessTime(t *testing.T) {
	t.Run("cancellation before the intake refuses adoption", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.cancellations.cancellation = cancelledAt(t, intakeHappenedAt.Add(-time.Hour))
		fixture.cancellations.cancelled = true

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeSourceNotAdopted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ADOPTED", result.Outcome())
		}
		if result.Basis().String() != "CANCELLED_BEFORE_INTAKE/cancel-1" {
			t.Fatalf("basis = %q; 不采用必须指名取消决定", result.Basis())
		}
		record, present := result.Record()
		if !present || record.Adopted {
			t.Fatalf("record = %#v present = %v; 不采用记录保留物理来源引用", record, present)
		}
	})

	t.Run("an intake that happened before the cancellation stalls as a conflict", func(t *testing.T) {
		fixture := newIntakeFixture(t)
		fixture.cancellations.cancellation = cancelledAt(t, intakeHappenedAt.Add(time.Hour))
		fixture.cancellations.cancelled = true

		result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.IntakeEligibilityUndecided ||
			result.UndecidedReason() != application.IntakeCancellationOrderConflict {
			t.Fatalf("outcome = %q/%q; 顺序冲突不得选边", result.Outcome(), result.UndecidedReason())
		}
		if fixture.adoptions.saved != 0 {
			t.Fatal("冲突落了库")
		}
	})
}

// Covers: `AT-PS-051`「正式承诺提交成功但事件投递失败——不回退承诺，只重试同一发布
// 意图」：首投失败承诺不翻、留发布续办引用；重放按已有结果重发同一份意图。
func TestAFailedIntakeIntentIsRetriedWithoutASecondCommitment(t *testing.T) {
	fixture := newIntakeFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	first, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q; 投递失败不得翻承诺", first.Outcome())
	}
	if first.IntakeHandoffReference().String() == "" {
		t.Fatal("首投失败没有留发布续办引用")
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.IntakeExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if len(fixture.downstream.intents) != 1 || !fixture.downstream.intents[0].Record.Adopted {
		t.Fatalf("intents = %d; 重发的必须是原采用那一份", len(fixture.downstream.intents))
	}
	if fixture.adoptions.saved != 1 {
		t.Fatal("重试形成了第二份承诺")
	}
}
