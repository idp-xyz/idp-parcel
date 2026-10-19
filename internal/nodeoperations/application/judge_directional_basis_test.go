package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var judgedAt = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

// judgedBasis 是不关心方向依据的用例交给 AddMember 的那份记录：一份合法的「身份缺少·无收寄记录」。
func judgedBasis(t *testing.T) domain.DirectionalBasisRecord {
	t.Helper()
	identity, err := domain.JudgeIdentityBasis(nil)
	if err != nil {
		t.Fatalf("identity basis: %v", err)
	}
	record, err := domain.RecordDirectionalBasisWithoutIdentity(identity, judgedAt)
	if err != nil {
		t.Fatalf("record without identity: %v", err)
	}
	return record
}

type intakeIdentityViewDouble struct {
	observations []domain.IntakeIdentityObservation
	err          error
}

func (double intakeIdentityViewDouble) FindIntakeIdentities(
	context.Context, domain.TenantID, domain.HandlingUnitID,
) ([]domain.IntakeIdentityObservation, error) {
	return double.observations, double.err
}

type basisServiceResultDouble struct {
	results ports.ParcelServiceResults
	err     error
	asked   []domain.ParcelAssociationReference
}

func (double *basisServiceResultDouble) FindParcelServiceResults(
	_ context.Context, _ domain.TenantID, parcel domain.ParcelAssociationReference,
) (ports.ParcelServiceResults, error) {
	double.asked = append(double.asked, parcel)
	return double.results, double.err
}

type routingBasisViewDouble struct {
	basis domain.RoutingBasis
	err   error
	asked int
}

func (double *routingBasisViewDouble) FindRoutingBasis(context.Context, ports.RoutingBasisQuery) (domain.RoutingBasis, error) {
	double.asked++
	return double.basis, double.err
}

func associatedIntake(t *testing.T, source, parcel string) domain.IntakeIdentityObservation {
	t.Helper()
	return domain.IntakeIdentityObservation{
		SourceID:    source,
		Version:     mustValue(t, domain.NewIntakeResultVersion, "v-"+source),
		Association: mustValue(t, domain.NewParcelAssociationReference, parcel),
		ReceivedAt:  consolidationAt,
	}
}

func addMemberCommand(t *testing.T, source string) application.AddMemberCommand {
	t.Helper()
	return application.AddMemberCommand{
		TenantID: consolidationTenant(t),
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, source, consolidationAt),
	}
}

type judgeFixture struct {
	intakes intakeIdentityViewDouble
	service *basisServiceResultDouble
	routing *routingBasisViewDouble
}

func (fixture judgeFixture) judge(t *testing.T) domain.DirectionalBasisRecord {
	t.Helper()
	judge, err := application.NewDirectionalBasisJudge(application.DirectionalBasisDeps{
		Intakes:        fixture.intakes,
		ServiceResults: fixture.service,
		Routing:        fixture.routing,
		Clock:          fixedClock{at: judgedAt},
	})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	record, err := judge.Judge(context.Background(), addMemberCommand(t, "src-judge"))
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if !record.Recorded() || !record.JudgedAt().Equal(judgedAt) {
		t.Fatalf("record judged at %v, recorded=%v", record.JudgedAt(), record.Recorded())
	}
	return record
}

func newJudgeFixture(intakes intakeIdentityViewDouble) judgeFixture {
	return judgeFixture{
		intakes: intakes,
		service: &basisServiceResultDouble{},
		routing: &routingBasisViewDouble{basis: domain.NotConfiguredRoutingBasis()},
	}
}

func assertCell(t *testing.T, cell domain.DirectionalBasisCell, answer domain.DirectionalBasisAnswer, reasons ...domain.DirectionalBasisReason) {
	t.Helper()
	if cell.Answer() != answer {
		t.Fatalf("%s answer = %s, want %s", cell.Aspect(), cell.Answer(), answer)
	}
	got := cell.Reasons()
	if len(got) != len(reasons) {
		t.Fatalf("%s reasons = %v, want %v", cell.Aspect(), got, reasons)
	}
	for index := range reasons {
		if got[index] != reasons[index] {
			t.Fatalf("%s reasons = %v, want %v", cell.Aspect(), got, reasons)
		}
	}
}

// Covers: 票 no-directional-basis/01「偏离标记半边：设计代决」的身份三种缺少——身份不成立时另两格`未判`，PS 与
// 路由依据都不问（无从问）。
func TestDirectionalBasisIdentityMissingLeavesTheOtherCellsUnjudged(t *testing.T) {
	pending := domain.IntakeIdentityObservation{
		SourceID: "src-pending", Version: mustValue(t, domain.NewIntakeResultVersion, "v-pending"), ReceivedAt: consolidationAt,
	}
	conflicted := pending
	conflicted.IdentityConflict = true
	cases := []struct {
		name         string
		observations []domain.IntakeIdentityObservation
		reasons      []domain.DirectionalBasisReason
	}{
		{"无收寄记录", nil, []domain.DirectionalBasisReason{domain.ReasonNoIntakeRecord}},
		{"待识别", []domain.IntakeIdentityObservation{pending}, []domain.DirectionalBasisReason{domain.ReasonPendingIdentification}},
		{"待识别且身份冲突", []domain.IntakeIdentityObservation{conflicted},
			[]domain.DirectionalBasisReason{domain.ReasonPendingIdentification, domain.ReasonIdentityConflict}},
		{"两行关联到不同正式包裹", []domain.IntakeIdentityObservation{
			associatedIntake(t, "src-a", "DPCL-A"), associatedIntake(t, "src-b", "DPCL-B"),
		}, []domain.DirectionalBasisReason{domain.ReasonIdentityConflict}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newJudgeFixture(intakeIdentityViewDouble{observations: testCase.observations})
			record := fixture.judge(t)
			assertCell(t, record.Identity().Cell(), domain.BasisMissing, testCase.reasons...)
			assertCell(t, record.ServiceResult().Cell(), domain.BasisNotJudged)
			assertCell(t, record.Routing().Cell(), domain.BasisNotJudged)
			if len(fixture.service.asked) != 0 || fixture.routing.asked != 0 {
				t.Fatalf("asked PS %d times and routing %d times without an established identity",
					len(fixture.service.asked), fixture.routing.asked)
			}
		})
	}
}

// Covers: 身份成立时服务结果现问 PS（问的是所依收寄上的正式包裹关联），已取消、已终局、两者并列各记`缺少`并带引用；
// 路由依据生产装配显式未配置，记`未配置`。
func TestDirectionalBasisServiceResultIsAskedOfParcelShipmentNow(t *testing.T) {
	cases := []struct {
		name    string
		results ports.ParcelServiceResults
		answer  domain.DirectionalBasisAnswer
		reasons []domain.DirectionalBasisReason
	}{
		{"无取消无终局", ports.ParcelServiceResults{}, domain.BasisEstablished, nil},
		{"已取消", ports.ParcelServiceResults{Cancellation: "CANCEL-1"}, domain.BasisMissing,
			[]domain.DirectionalBasisReason{domain.ReasonCancelled}},
		{"已终局", ports.ParcelServiceResults{FinalKind: "RETURNED", FinalVersion: "FINAL-2"}, domain.BasisMissing,
			[]domain.DirectionalBasisReason{domain.ReasonFinal}},
		{"两者并列", ports.ParcelServiceResults{Cancellation: "CANCEL-1", FinalKind: "RETURNED", FinalVersion: "FINAL-2"},
			domain.BasisMissing, []domain.DirectionalBasisReason{domain.ReasonCancelled, domain.ReasonFinal}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newJudgeFixture(intakeIdentityViewDouble{observations: []domain.IntakeIdentityObservation{
				associatedIntake(t, "src-intake", "DPCL-1"),
			}})
			fixture.service.results = testCase.results
			record := fixture.judge(t)

			assertCell(t, record.Identity().Cell(), domain.BasisEstablished)
			if parcel, _ := record.Identity().Association(); parcel.String() != "DPCL-1" ||
				record.Identity().IntakeSourceID() != "src-intake" || record.Identity().IntakeVersion().String() != "v-src-intake" {
				t.Fatalf("identity basedOn = %q %q %q", parcel, record.Identity().IntakeSourceID(), record.Identity().IntakeVersion())
			}
			if len(fixture.service.asked) != 1 || fixture.service.asked[0].String() != "DPCL-1" {
				t.Fatalf("PS asked about %v", fixture.service.asked)
			}
			assertCell(t, record.ServiceResult().Cell(), testCase.answer, testCase.reasons...)
			if record.ServiceResult().Cancellation() != testCase.results.Cancellation {
				t.Fatalf("cancellation = %q", record.ServiceResult().Cancellation())
			}
			if kind, version := record.ServiceResult().Final(); kind != testCase.results.FinalKind || version != testCase.results.FinalVersion {
				t.Fatalf("final = %q %q", kind, version)
			}
			assertCell(t, record.Routing().Cell(), domain.BasisNotConfigured)
		})
	}
}

// Covers: 来源答不出就记那一格`读不动`，记录照样成立——读不动不挡落册（设计代决「不挡事实」）。收寄册读不动时身份
// 不成立，另两格`未判`；PS 或路由依据读不动只落在它自己那一格。
func TestDirectionalBasisUnreadableSourcesAreRecordedNotRaised(t *testing.T) {
	broken := errors.New("connection reset")

	t.Run("收寄册读不动", func(t *testing.T) {
		fixture := newJudgeFixture(intakeIdentityViewDouble{err: broken})
		record := fixture.judge(t)
		assertCell(t, record.Identity().Cell(), domain.BasisUnreadable)
		assertCell(t, record.ServiceResult().Cell(), domain.BasisNotJudged)
		assertCell(t, record.Routing().Cell(), domain.BasisNotJudged)
	})
	t.Run("PS 读不动", func(t *testing.T) {
		fixture := newJudgeFixture(intakeIdentityViewDouble{observations: []domain.IntakeIdentityObservation{
			associatedIntake(t, "src-intake", "DPCL-1"),
		}})
		fixture.service.err = broken
		record := fixture.judge(t)
		assertCell(t, record.Identity().Cell(), domain.BasisEstablished)
		assertCell(t, record.ServiceResult().Cell(), domain.BasisUnreadable)
		assertCell(t, record.Routing().Cell(), domain.BasisNotConfigured)
	})
	t.Run("PS 答出半个终局", func(t *testing.T) {
		fixture := newJudgeFixture(intakeIdentityViewDouble{observations: []domain.IntakeIdentityObservation{
			associatedIntake(t, "src-intake", "DPCL-1"),
		}})
		fixture.service.results = ports.ParcelServiceResults{FinalKind: "RETURNED"}
		assertCell(t, fixture.judge(t).ServiceResult().Cell(), domain.BasisUnreadable)
	})
	t.Run("路由依据读不动", func(t *testing.T) {
		fixture := newJudgeFixture(intakeIdentityViewDouble{observations: []domain.IntakeIdentityObservation{
			associatedIntake(t, "src-intake", "DPCL-1"),
		}})
		fixture.routing = &routingBasisViewDouble{err: broken}
		assertCell(t, fixture.judge(t).Routing().Cell(), domain.BasisUnreadable)
	})
	t.Run("路由依据答出零值", func(t *testing.T) {
		fixture := newJudgeFixture(intakeIdentityViewDouble{observations: []domain.IntakeIdentityObservation{
			associatedIntake(t, "src-intake", "DPCL-1"),
		}})
		fixture.routing = &routingBasisViewDouble{}
		assertCell(t, fixture.judge(t).Routing().Cell(), domain.BasisUnreadable)
	})
}

// Covers: 方向依据记录只随新形成的移入事实落册；缺依据照常 MEMBER_ADDED（UC-NO-003「已经发生的错误分拣保留为
// 事实并形成偏离，不回写为未发生」）；同一来源重投交回册上那一份，不用本次刚判的。
func TestAddMemberRecordsTheJudgedBasisWithTheFactAndReplaysTheStoredOne(t *testing.T) {
	fixture := newConsolidateFixture(t)
	openUnit(t, fixture, "unit-1")

	cancelled, err := domain.JudgeServiceResultBasis("CANCEL-1", "", "")
	if err != nil {
		t.Fatalf("service result: %v", err)
	}
	identity, err := domain.JudgeIdentityBasis([]domain.IntakeIdentityObservation{associatedIntake(t, "src-intake", "DPCL-1")})
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	first, err := domain.RecordDirectionalBasis(identity, cancelled, domain.NotConfiguredRoutingBasis(), judgedAt)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	command := addMemberCommand(t, "src-add-cancelled")
	added, err := fixture.handler.AddMember(context.Background(), command, first)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added.Outcome() != application.MemberAdded {
		t.Fatalf("outcome = %q, want MEMBER_ADDED even though the parcel is cancelled", added.Outcome())
	}
	assertCell(t, added.DirectionalBasis().ServiceResult().Cell(), domain.BasisMissing, domain.ReasonCancelled)
	stored := fixture.facts.byKey[ports.ConsolidationFactKey{TenantID: command.TenantID, SourceID: "src-add-cancelled"}]
	if !stored.DirectionalBasis.Recorded() || stored.DirectionalBasis.ServiceResult().Cell().Answer() != domain.BasisMissing {
		t.Fatalf("stored basis = %+v", stored.DirectionalBasis)
	}

	// 同一来源重投：本次带进来的是一份新判的「无收寄记录」，交回的仍是册上那份「已取消」，判断时刻也是当时的。
	replayed, err := fixture.handler.AddMember(context.Background(), command, judgedBasis(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Outcome() != application.ConsolidationExistingResult {
		t.Fatalf("replay outcome = %q", replayed.Outcome())
	}
	assertCell(t, replayed.DirectionalBasis().ServiceResult().Cell(), domain.BasisMissing, domain.ReasonCancelled)
	if fixture.facts.saved != 2 { // 开单元一条、移入一条
		t.Fatalf("facts saved = %d", fixture.facts.saved)
	}
}

// Covers: 完成判据「各格均照常 MEMBER_ADDED」——AddMember 只问记录在不在，不看任何一格的取值：身份三种缺少、读不动、
// 已取消、已终局、两者并列、路由依据未配置，各自落成事实并原样带着记录。
func TestAddMemberRecordsEveryKindOfBasisAsTheFactHappened(t *testing.T) {
	pending := domain.IntakeIdentityObservation{
		SourceID: "src-pending", Version: mustValue(t, domain.NewIntakeResultVersion, "v-pending"), ReceivedAt: consolidationAt,
	}
	conflicted := pending
	conflicted.IdentityConflict = true
	identityCases := map[string][]domain.IntakeIdentityObservation{
		"无收寄记录": nil,
		"待识别":   {pending},
		"身份冲突":  {conflicted},
	}
	established := []domain.IntakeIdentityObservation{associatedIntake(t, "src-intake", "DPCL-1")}
	serviceCases := map[string]ports.ParcelServiceResults{
		"已取消":  {Cancellation: "CANCEL-1"},
		"已终局":  {FinalKind: "RETURNED", FinalVersion: "FINAL-2"},
		"两者并列": {Cancellation: "CANCEL-1", FinalKind: "RETURNED", FinalVersion: "FINAL-2"},
		"依据齐备": {},
	}
	var records []domain.DirectionalBasisRecord
	for _, observations := range identityCases {
		records = append(records, newJudgeFixture(intakeIdentityViewDouble{observations: observations}).judge(t))
	}
	records = append(records, newJudgeFixture(intakeIdentityViewDouble{err: errors.New("down")}).judge(t))
	for _, results := range serviceCases {
		fixture := newJudgeFixture(intakeIdentityViewDouble{observations: established})
		fixture.service.results = results
		records = append(records, fixture.judge(t))
	}
	fixture := newConsolidateFixture(t)
	openUnit(t, fixture, "unit-1")
	for index, record := range records {
		command := addMemberCommand(t, "src-every-kind")
		command.Source = workSource(t, "src-every-kind-"+string(rune('a'+index)), consolidationAt)
		command.Member = handlingUnit(t, "hu-kind-"+string(rune('a'+index)))
		added, err := fixture.handler.AddMember(context.Background(), command, record)
		if err != nil || added.Outcome() != application.MemberAdded {
			t.Fatalf("record %v: outcome = %v err = %v", record.Cells(), added.Outcome(), err)
		}
		stored := fixture.facts.byKey[ports.ConsolidationFactKey{TenantID: command.TenantID, SourceID: command.Source.SourceID()}]
		if !stored.DirectionalBasis.JudgedAt().Equal(record.JudgedAt()) || len(stored.DirectionalBasis.Cells()) != len(record.Cells()) {
			t.Fatalf("stored basis differs from the judged one: %v vs %v", stored.DirectionalBasis.Cells(), record.Cells())
		}
	}
}

// Covers: 不交方向依据记录是装配缺陷，不是业务答案：拒在一切读写之前，不落一条读作`未记录`的新事实。
func TestAddMemberRefusesAnUnjudgedBasis(t *testing.T) {
	fixture := newConsolidateFixture(t)
	openUnit(t, fixture, "unit-1")
	_, err := fixture.handler.AddMember(context.Background(), addMemberCommand(t, "src-unjudged"), domain.DirectionalBasisRecord{})
	if !errors.Is(err, application.ErrNoDirectionalBasisRecord) {
		t.Fatalf("err = %v", err)
	}
	if fixture.facts.saved != 1 {
		t.Fatalf("facts saved = %d", fixture.facts.saved)
	}
}

// Covers: 同一来源两份写入撞键时，交回的是先到那一份落在册上的记录，不是输家刚判的。
func TestAddMemberRaceLoserAnswersWithTheWinnersBasis(t *testing.T) {
	fixture := newConsolidateFixture(t)
	openUnit(t, fixture, "unit-1")
	command := addMemberCommand(t, "src-race")

	winner := judgedBasis(t)
	payload, digest, err := domain.CanonicalizeAddMemberPayload(command.Source, command.Unit, command.Member)
	if err != nil || len(payload) == 0 {
		t.Fatalf("canonicalize: %v", err)
	}
	key := ports.ConsolidationFactKey{TenantID: command.TenantID, SourceID: "src-race"}
	racing := &racingFactStore{consolidationFactStoreDouble: fixture.facts, key: key, winner: ports.ConsolidationFactRecord{
		Key: key, ContentDigest: digest, Action: domain.AddMemberAction, Unit: command.Unit, Member: command.Member,
		DirectionalBasis: winner,
	}}
	fixture.handler = application.NewConsolidateParcelsHandler(application.ConsolidateParcelsDeps{
		Store:       fixture.store,
		Facts:       racing,
		Containment: &containmentIndexDouble{store: fixture.store},
		Downstream:  fixture.downstream,
		Clock:       fixedClock{at: consolidationAt},
	})

	identity, err := domain.JudgeIdentityBasis([]domain.IntakeIdentityObservation{associatedIntake(t, "src-intake", "DPCL-1")})
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	established, err := domain.JudgeServiceResultBasis("", "", "")
	if err != nil {
		t.Fatalf("service result: %v", err)
	}
	loser, err := domain.RecordDirectionalBasis(identity, established, domain.NotConfiguredRoutingBasis(), judgedAt.Add(time.Second))
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	result, err := fixture.handler.AddMember(context.Background(), command, loser)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if result.Outcome() != application.ConsolidationExistingResult {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	assertCell(t, result.DirectionalBasis().Identity().Cell(), domain.BasisMissing, domain.ReasonNoIntakeRecord)
	if !result.DirectionalBasis().JudgedAt().Equal(judgedAt) {
		t.Fatalf("judged at = %v, want the winner's", result.DirectionalBasis().JudgedAt())
	}
}

// racingFactStore 在第一次按键读时答「没有」（受理闸放行），写时答`已有记录`并让赢家那一份此后可读——两份写入
// 在受理闸与写之间交错的样子。
type racingFactStore struct {
	*consolidationFactStoreDouble
	key    ports.ConsolidationFactKey
	winner ports.ConsolidationFactRecord
	landed bool
}

func (store *racingFactStore) FindByKey(ctx context.Context, key ports.ConsolidationFactKey) (ports.ConsolidationFactRecord, bool, error) {
	if key == store.key {
		return store.winner, store.landed, nil
	}
	return store.consolidationFactStoreDouble.FindByKey(ctx, key)
}

func (store *racingFactStore) Save(ctx context.Context, record ports.ConsolidationFactRecord) (ports.ConsolidationFactSaveOutcome, error) {
	if record.Key == store.key {
		store.landed = true
		return ports.ConsolidationFactAlreadyRecorded, nil
	}
	return store.consolidationFactStoreDouble.Save(ctx, record)
}

// Covers: 撞键读回的赢家是同一来源的另一份内容时，输家答来源冲突，不借赢家的记录答已有结果——与受理闸同一条代数。
func TestAddMemberRaceLoserWithDifferentContentAnswersSourceConflict(t *testing.T) {
	fixture := newConsolidateFixture(t)
	openUnit(t, fixture, "unit-1")
	command := addMemberCommand(t, "src-race-conflict")
	_, otherDigest, err := domain.CanonicalizeAddMemberPayload(command.Source, command.Unit, handlingUnit(t, "hu-other"))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	key := ports.ConsolidationFactKey{TenantID: command.TenantID, SourceID: "src-race-conflict"}
	racing := &racingFactStore{consolidationFactStoreDouble: fixture.facts, key: key, winner: ports.ConsolidationFactRecord{
		Key: key, ContentDigest: otherDigest, Action: domain.AddMemberAction, Unit: command.Unit,
		Member: handlingUnit(t, "hu-other"), DirectionalBasis: judgedBasis(t),
	}}
	fixture.handler = application.NewConsolidateParcelsHandler(application.ConsolidateParcelsDeps{
		Store:       fixture.store,
		Facts:       racing,
		Containment: &containmentIndexDouble{store: fixture.store},
		Downstream:  fixture.downstream,
		Clock:       fixedClock{at: consolidationAt},
	})
	result, err := fixture.handler.AddMember(context.Background(), command, judgedBasis(t))
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if result.Outcome() != application.ConsolidationSourceConflict || result.DirectionalBasis().Recorded() {
		t.Fatalf("outcome = %q, basis recorded = %v", result.Outcome(), result.DirectionalBasis().Recorded())
	}
}
