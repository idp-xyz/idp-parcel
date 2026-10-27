package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证 AT-VE-174 的整段：PS 终局采用账上真的有一条同种更正链
// （首派生 → 重派生翻旧插新），VE 终局译装器按信封读回、按终局版本找回前一代，派生编排
// 用真实的事实库与投影库落账——更正之后投影里这条终局流只有一代当前有效，两代到达先后不
// 改变结论。映射目录给已采用终局登了一行里程碑，正是「时间线上同一终局出现两次」会显形的
// 那种配置。

// finalMilestoneMapping 只把已采用终局归进一个里程碑，其余如实未归类。这是测试夹具，
// 不是任何租户的映射目录。
type finalMilestoneMapping struct{}

func (finalMilestoneMapping) ClassifyFact(
	_ context.Context, fact vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	mapping, err := vedomain.NewMappingVersionReference("SYN-MAPPING-V1")
	if err != nil {
		return veports.MilestoneAnswer{}, false, err
	}
	if fact.Kind().String() != vedomain.FinalOutcomeAdoptedFactKind {
		return veports.MilestoneAnswer{Mapping: mapping}, true, nil
	}
	milestone, err := vedomain.NewMilestoneReference("SYN-SERVICE-FINAL")
	if err != nil {
		return veports.MilestoneAnswer{}, false, err
	}
	return veports.MilestoneAnswer{Milestone: milestone, Classified: true, Mapping: mapping}, true, nil
}

type unconfiguredConflictRules struct{}

func (unconfiguredConflictRules) ConflictSignalRule(context.Context) (veports.ConflictSignalRule, bool, error) {
	return veports.ConflictSignalRule{}, false, nil
}

var errSignalRaised = errors.New("一条前后分明的更正链不该走到冲突信号")

type refusingSignals struct{}

func (refusingSignals) Handle(
	context.Context, veapplication.RaiseSignalCommand,
) (veapplication.RaiseSignalResult, error) {
	return veapplication.RaiseSignalResult{}, errSignalRaised
}

type acceptingProjectionHandoff struct{}

func (acceptingProjectionHandoff) HandOffProjection(context.Context, veports.ProjectionHandoffIntent) error {
	return nil
}

type fixedProjectionClock struct{ at time.Time }

func (clock fixedProjectionClock) Now() time.Time { return clock.at }

// outcomeRecordingProjection 留下派生编排每次的结论：消费门把 EXISTING_RESULT 与
// SOURCE_CONFLICT 都按已消费确认，只看消费口的返回分不出重投落在哪一格。
type outcomeRecordingProjection struct {
	inner    adapter.FinalProjectionHandler
	outcomes []veapplication.DeriveProjectionOutcome
}

func (recorder *outcomeRecordingProjection) Handle(
	ctx context.Context, command veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	result, err := recorder.inner.Handle(ctx, command)
	if err == nil {
		recorder.outcomes = append(recorder.outcomes, result.Outcome())
	}
	return result, err
}

type finalChainFixture struct {
	finals      *pspostgres.FinalOutcomes
	facts       *vepostgres.AcceptedFacts
	projections *vepostgres.Projections
	transactor  bentoapp.Transactor
	derive      *outcomeRecordingProjection
	subject     *adapter.DeriveOnFinalOutcomeAdapter
}

func newFinalChainFixture(t *testing.T) *finalChainFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造 PS 终局库：%v", err)
	}
	facts, err := vepostgres.NewAcceptedFacts(db)
	if err != nil {
		t.Fatalf("构造 VE 事实库：%v", err)
	}
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		t.Fatalf("构造 VE 投影库：%v", err)
	}
	identities, err := veidentity.NewProjectionVersions()
	if err != nil {
		t.Fatalf("构造投影版本签发：%v", err)
	}
	derive := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:         facts,
		Mapping:       finalMilestoneMapping{},
		Projections:   projections,
		Identities:    identities,
		ConflictRules: unconfiguredConflictRules{},
		Signals:       refusingSignals{},
		Downstream:    acceptingProjectionHandoff{},
		Clock:         fixedProjectionClock{at: finalAdoptedAt.Add(3 * time.Hour)},
	})
	recorder := &outcomeRecordingProjection{inner: derive}
	subject, err := adapter.NewDeriveOnFinalOutcomeAdapter(finals, recorder)
	if err != nil {
		t.Fatalf("构造译装器：%v", err)
	}
	return &finalChainFixture{
		finals:      finals,
		facts:       facts,
		projections: projections,
		transactor:  db.Transactor(),
		derive:      recorder,
		subject:     subject,
	}
}

// adoptCorrectionChain 照 FormParcelFinalHandler 的写法把首派生与重派生依次落进 PS 采用账：
// 后一代翻下前一代的当前位、以终局版本指回它。
func (fixture *finalChainFixture) adoptCorrectionChain(t *testing.T) {
	t.Helper()
	for _, record := range []psports.FinalOutcomeRecord{adoptedFinalRecord(t), rederivedFinalRecord(t)} {
		var saved psports.FinalOutcomeSaveOutcome
		err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			var err error
			saved, err = fixture.finals.Save(txCtx, record)
			return err
		})
		if err != nil || saved != psports.FinalOutcomeSaved {
			t.Fatalf("PS 采用账落 %q：saved=%d err=%v", record.Key.Version.String(), saved, err)
		}
	}
}

// consume 照 inbox 消费门的形状在一个事务里跑译装器：派生编排的写口要求事务。
func (fixture *finalChainFixture) consume(t *testing.T, formed veinbox.FormedFinalOutcome) {
	t.Helper()
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.subject.HandleFormedFinalOutcome(txCtx, formed)
	})
	if err != nil {
		t.Fatalf("消费 %q：%v", formed.Version, err)
	}
}

func (fixture *finalChainFixture) currentProjection(t *testing.T) vedomain.TrackingProjection {
	t.Helper()
	projection, found, err := fixture.projections.FindCurrent(t.Context(),
		finalValue(t, vedomain.NewTenantID, "tenant-1"),
		finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"))
	if err != nil || !found {
		t.Fatalf("当前投影：found=%v err=%v", found, err)
	}
	return projection
}

// adoptedFinalEntries 取投影里来自 PS 的已采用终局条目。
func adoptedFinalEntries(projection vedomain.TrackingProjection) []vedomain.MilestoneClassification {
	var entries []vedomain.MilestoneClassification
	for _, entry := range projection.Entries() {
		if entry.Fact().Source() == vedomain.SourceParcelShipment &&
			entry.Fact().Kind().String() == vedomain.FinalOutcomeAdoptedFactKind {
			entries = append(entries, entry)
		}
	}
	return entries
}

// assertOnlyTheCorrectedFinalIsCurrent 是 AT-VE-174 的结论半边：当前投影里这条终局流只剩
// 更正后那一代；事实库里两代都在，更正后那一代指名前一代的采用版本为前身——前一代留档，
// 只是不参与派生。
func (fixture *finalChainFixture) assertOnlyTheCorrectedFinalIsCurrent(t *testing.T) {
	t.Helper()
	projection := fixture.currentProjection(t)
	finals := adoptedFinalEntries(projection)
	if len(finals) != 1 {
		t.Fatalf("当前投影里有 %d 代已采用终局，want 1——被替代的那一代不得参与派生", len(finals))
	}
	current := finals[0].Fact()
	if current.Version().String() != "responsibility-outcome/v2" {
		t.Fatalf("当前有效的终局版本 = %q, want 更正后的 responsibility-outcome/v2", current.Version().String())
	}
	if milestone, classified := finals[0].Milestone(); !classified || milestone.String() != "SYN-SERVICE-FINAL" {
		t.Fatalf("终局条目没有归进夹具登的里程碑：%q %v", milestone.String(), classified)
	}
	if !projection.CarriesCustomerServiceFinal() {
		t.Fatal("更正后的投影答不出已采用终局")
	}

	records, err := fixture.facts.FindByParcel(t.Context(),
		finalValue(t, vedomain.NewTenantID, "tenant-1"),
		finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"))
	if err != nil {
		t.Fatalf("读事实库：%v", err)
	}
	// 前身只问事实库：投影条目落库时不带前身（另立票 demo-chain-gaps/117），当前有效集是
	// 派生编排从事实库现算的，替代关系的权威留档也在那里。
	supersedes := map[string]string{}
	for _, record := range records {
		predecessor, _ := record.Fact.Supersedes()
		supersedes[record.Fact.Version().String()] = predecessor.String()
	}
	if len(records) != 2 {
		t.Fatalf("事实库里的终局 = %v, want 两代都留档", supersedes)
	}
	if predecessor, kept := supersedes["responsibility-outcome/v1"]; !kept || predecessor != "" {
		t.Fatalf("前一代没有原样留档：%v", supersedes)
	}
	if supersedes["responsibility-outcome/v2"] != "responsibility-outcome/v1" {
		t.Fatalf("更正后那一代没有指名前一代的采用版本：%v", supersedes)
	}
}

// Covers: AT-VE-174——POD 更正一类的同种来源更正让 PS 重派生出新一代终局，两代依次进投影：
// 更正后当前投影只有一代终局当前有效；更正前那一版投影仍可按版本读回、里面是前一代。
func TestACorrectedFinalOutcomeLeavesOnlyOneGenerationCurrentInTheProjection(t *testing.T) {
	fixture := newFinalChainFixture(t)
	fixture.adoptCorrectionChain(t)

	fixture.consume(t, formedFinalRef())
	before := fixture.currentProjection(t)
	if finals := adoptedFinalEntries(before); len(finals) != 1 ||
		finals[0].Fact().Version().String() != "responsibility-outcome/v1" {
		t.Fatalf("首派生之后的投影应当只有前一代终局：%d 条", len(finals))
	}

	fixture.consume(t, formedRederivedFinalRef())
	fixture.assertOnlyTheCorrectedFinalIsCurrent(t)

	after := fixture.currentProjection(t)
	prior, has := after.PriorVersion()
	if !has || prior != before.Version() {
		t.Fatalf("更正后的投影没有指回更正前那一版")
	}
	retained, found, err := fixture.projections.FindByVersion(t.Context(),
		finalValue(t, vedomain.NewTenantID, "tenant-1"), prior)
	if err != nil || !found {
		t.Fatalf("更正前那一版投影读不回：found=%v err=%v", found, err)
	}
	if finals := adoptedFinalEntries(retained); len(finals) != 1 ||
		finals[0].Fact().Version().String() != "responsibility-outcome/v1" {
		t.Fatal("更正前那一版投影里的前一代终局没有原样留存")
	}
}

// Covers: AT-VE-174（到达先后）——更正后那一代先到、前一代迟到，结论不变：「已被替代」是按
// 在场事实集重新回答的派生问答，迟到的前一代一到就已经是被替代的。
func TestALatePredecessorFinalNeverBecomesCurrentAgain(t *testing.T) {
	fixture := newFinalChainFixture(t)
	fixture.adoptCorrectionChain(t)

	fixture.consume(t, formedRederivedFinalRef())
	fixture.consume(t, formedFinalRef())

	fixture.assertOnlyTheCorrectedFinalIsCurrent(t)
}

// Covers: AT-VE-174（重投）——同一封重派生信封再投一次答已有结果，不形成新的投影版本。前身进
// 事实内容指纹，所以它必须取自 PS 那一行不可变的采用键；若改成随读取时刻现算（比如读当前位），
// 重投时前身一格就变了，派生编排会把它判成同版本的来源冲突。
func TestARedeliveredRederivedFinalIsTheExistingResult(t *testing.T) {
	fixture := newFinalChainFixture(t)
	fixture.adoptCorrectionChain(t)
	fixture.consume(t, formedFinalRef())
	fixture.consume(t, formedRederivedFinalRef())
	settled := fixture.currentProjection(t)

	fixture.consume(t, formedRederivedFinalRef())

	outcomes := fixture.derive.outcomes
	if last := outcomes[len(outcomes)-1]; last != veapplication.FactExistingResult {
		t.Fatalf("重投的结论 = %s, want EXISTING_RESULT", last)
	}
	if again := fixture.currentProjection(t); again.Version() != settled.Version() {
		t.Fatal("重投形成了新的投影版本")
	}
	fixture.assertOnlyTheCorrectedFinalIsCurrent(t)
}

// voidThenSave 把 PS 采用账上同一采用键的下一份记录落进去：此前依据某份取消的不采用行已被决定作废（物理删除），续办用同一键重新形成记录（ADR-0295）。
func (fixture *finalChainFixture) voidThenSave(t *testing.T, voidedCancellation string, next psports.FinalOutcomeRecord) {
	t.Helper()
	var cancellation psdomain.ParcelCancellationID
	if voidedCancellation != "" {
		cancellation = finalValue(t, psdomain.NewParcelCancellationID, voidedCancellation)
	}
	var voided int
	var saved psports.FinalOutcomeSaveOutcome
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if voidedCancellation != "" {
			var err error
			if voided, err = fixture.finals.VoidCancellationRefusedFinals(txCtx, next.Key.TenantID, next.Key.Parcel, cancellation); err != nil {
				return err
			}
		}
		var err error
		saved, err = fixture.finals.Save(txCtx, next)
		return err
	})
	if err != nil {
		t.Fatalf("重新形成：%v", err)
	}
	if voidedCancellation != "" && voided != 1 {
		t.Fatalf("作废 %s：voided=%d, want 1", voidedCancellation, voided)
	}
	if saved != psports.FinalOutcomeSaved {
		t.Fatalf("PS 采用账落下一份记录：saved=%d", saved)
	}
}

func refusedByCancellation(t *testing.T, cancellation string, adoptedAt time.Time) psports.FinalOutcomeRecord {
	t.Helper()
	record := notAdoptedFinalRecord(t)
	record.RefusalBasis = finalValue(t, psdomain.NewCheckReason, "PARCEL_CANCELLATION/"+cancellation)
	record.AdoptedAt = adoptedAt
	return record
}

// factsByVersion 取事实库里这条终局流的各版本及其前身。
func (fixture *finalChainFixture) factsByVersion(t *testing.T) map[string]string {
	t.Helper()
	records, err := fixture.facts.FindByParcel(t.Context(),
		finalValue(t, vedomain.NewTenantID, "tenant-1"),
		finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"))
	if err != nil {
		t.Fatalf("读事实库：%v", err)
	}
	versions := map[string]string{}
	for _, record := range records {
		predecessor, _ := record.Fact.Supersedes()
		versions[record.Fact.Version().String()+"/"+record.Fact.Kind().String()] = predecessor.String()
	}
	return versions
}

// Covers: AT-VE-174；ADR-0295 决定六——同一采用键的记录因取消被认定不成立而被删后重新形成（不采用 → 删 → 再被另一份取消挡成不采用 → 再删 → 终局），每一代信封带自己的
// 作废代次，VE 终局投影随之换代：事实库里三代都留档，各带下一代的替代关系，当前投影里这条终局流只剩最后那一代已采用终局；同代次重放答已有结果。
// 代次不进事实版本时，重新形成的终局与被删的不采用行同事实键、内容不同，被判成来源冲突、保留原事实，投影停在「不采用」——下面先钉住这一点。
func TestAReformedFinalOutcomeSupersedesTheRefusalItReplacedInTheProjection(t *testing.T) {
	fixture := newFinalChainFixture(t)
	first := refusedByCancellation(t, "PCXL-1", finalAdoptedAt)
	fixture.voidThenSave(t, "", first)
	fixture.consume(t, formedFinalRef())

	second := refusedByCancellation(t, "PCXL-2", finalAdoptedAt.Add(time.Hour))
	fixture.voidThenSave(t, "PCXL-1", second)
	generationOne := formedFinalRef()
	generationOne.VoidGeneration = 1
	fixture.consume(t, generationOne)

	final := adoptedFinalRecord(t)
	final.AdoptedAt = finalAdoptedAt.Add(2 * time.Hour)
	fixture.voidThenSave(t, "PCXL-2", final)

	// 代次不进事实版本时：同一事实键、不同内容，来源冲突，原事实保留。
	fixture.consume(t, generationOne)
	if last := fixture.derive.outcomes[len(fixture.derive.outcomes)-1]; last != veapplication.FactSourceConflict {
		t.Fatalf("以旧代次消费重新形成的终局 = %s, want SOURCE_CONFLICT（同键不同内容）", last)
	}

	generationTwo := formedFinalRef()
	generationTwo.VoidGeneration = 2
	fixture.consume(t, generationTwo)
	fixture.consume(t, generationTwo)
	want := []veapplication.DeriveProjectionOutcome{
		veapplication.ProjectionDerived, veapplication.ProjectionDerived,
		veapplication.FactSourceConflict, veapplication.ProjectionDerived, veapplication.FactExistingResult,
	}
	if len(fixture.derive.outcomes) != len(want) {
		t.Fatalf("派生结论 = %v, want %v", fixture.derive.outcomes, want)
	}
	for index := range want {
		if fixture.derive.outcomes[index] != want[index] {
			t.Fatalf("派生结论 = %v, want %v", fixture.derive.outcomes, want)
		}
	}

	versions := fixture.factsByVersion(t)
	wantVersions := map[string]string{
		"responsibility-outcome/v1/final-outcome-not-adopted":        "",
		"responsibility-outcome/v1#void-1/final-outcome-not-adopted": "responsibility-outcome/v1",
		"responsibility-outcome/v1#void-2/final-outcome-adopted":     "responsibility-outcome/v1#void-1",
	}
	if len(versions) != len(wantVersions) {
		t.Fatalf("事实库 = %v, want %v", versions, wantVersions)
	}
	for version, predecessor := range wantVersions {
		if got, kept := versions[version]; !kept || got != predecessor {
			t.Fatalf("事实库 = %v, want %v", versions, wantVersions)
		}
	}

	projection := fixture.currentProjection(t)
	if !projection.CarriesCustomerServiceFinal() {
		t.Fatal("重新形成的终局之后，投影答不出已采用终局——仍停在不采用")
	}
	if adopted := adoptedFinalEntries(projection); len(adopted) != 1 ||
		adopted[0].Fact().Version().String() != "responsibility-outcome/v1#void-2" {
		t.Fatalf("已采用终局条目 = %d 条, want 恰一条且是代次二的版本", len(adopted))
	}
	for _, entry := range projection.Entries() {
		if entry.Fact().Kind().String() == vedomain.FinalOutcomeNotAdoptedFactKind {
			t.Fatalf("被取代的不采用事实仍参与派生：%s", entry.Fact().Version().String())
		}
	}
}
