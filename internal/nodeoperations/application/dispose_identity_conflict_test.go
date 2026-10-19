package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证身份冲突处置编排（UC-NO-004；ADR-0210 决定二至四、六、七、九）。收寄库替身沿用收寄编排那一只，外面一层补上按实物读
// 各行，并照真库认处置截断（决定八）——收寄与重核在这里走的是同一只，处置之后的第三件到站与重核才看得到截断。夹具全为合成（S 级）。

const dispositionOperator = "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06"

// dispositionStore 在收寄库替身外面补两问：按实物读带收寄的各行；按包裹读别的实物关联时跳过被处置版接续过的行。holds 记下取锁的
// 那两问（实物、包裹）被问的次序。
type dispositionStore struct {
	*receptionStoreDouble
	holds *[]string
}

func (store dispositionStore) FindCurrentJudgmentsAtNode(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
	node domain.NodeReference,
) ([]ports.ReceptionRecord, error) {
	*store.holds = append(*store.holds, "unit:"+unit.String())
	return store.receptionStoreDouble.FindCurrentJudgmentsAtNode(ctx, tenant, unit, node)
}

func (store dispositionStore) FindOtherUnitIntakes(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelAssociationReference,
	unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	*store.holds = append(*store.holds, "parcel:"+parcel.String())
	records, err := store.receptionStoreDouble.FindOtherUnitIntakes(ctx, tenant, parcel, unit)
	if err != nil {
		return nil, err
	}
	kept := []ports.ReceptionRecord{}
	for _, record := range records {
		if !store.succeededByDisposition(tenant, record.JudgmentVersion) {
			kept = append(kept, record)
		}
	}
	return kept, nil
}

// succeededByDisposition 沿后继走：这一版之后的哪一版带处置文档，它就被处置版接续过。
func (store dispositionStore) succeededByDisposition(tenant domain.TenantID, version domain.IntakeResultVersion) bool {
	for range len(store.byKey) + 1 {
		next, found := store.successorOf(tenant, version)
		if !found {
			return false
		}
		for key, record := range store.byKey {
			if key.TenantID == tenant && record.JudgmentVersion == next && record.Disposition.Recorded() {
				return true
			}
		}
		version = next
	}
	return false
}

func (store dispositionStore) FindUnitIntakes(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) ([]ports.ReceptionRecord, error) {
	found := []ports.ReceptionRecord{}
	for key, record := range store.byKey {
		if key.TenantID == tenant && !record.Intake.ReceivedAt().IsZero() && record.Intake.Unit() == unit {
			found = append(found, record)
		}
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found, nil
}

type dispositionFixture struct {
	store       dispositionStore
	identity    *identityViewDouble
	services    *serviceResultViewDouble
	versions    *versionFactoryDouble
	downstream  *downstreamDouble
	receive     *application.ReceiveDeliveredUnitHandler
	recheck     *application.RecheckPendingIdentityHandler
	dispose     *application.DisposeIdentityConflictHandler
	parcel      domain.ParcelAssociationReference
	disposedNow time.Time
}

func newDispositionFixture(t *testing.T) *dispositionFixture {
	t.Helper()
	fixture := &dispositionFixture{
		store:       dispositionStore{receptionStoreDouble: newReceptionStore(), holds: &[]string{}},
		identity:    &identityViewDouble{},
		services:    &serviceResultViewDouble{},
		versions:    &versionFactoryDouble{},
		downstream:  &downstreamDouble{},
		parcel:      mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-X"),
		disposedNow: deliveredAt.Add(3 * time.Hour),
	}
	fixture.identity.candidates = []domain.ParcelAssociationReference{fixture.parcel}
	var err error
	fixture.receive, err = application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity: fixture.identity, ServiceResults: fixture.services, Receptions: fixture.store,
		Versions: fixture.versions, Downstream: fixture.downstream, Doorbell: &doorbellDouble{},
		Scope: newScopeEvidence(), Handover: newHandoverRegister(), Clock: fixedClock{at: deliveredAt.Add(time.Minute)},
	})
	if err != nil {
		t.Fatalf("收寄编排：%v", err)
	}
	fixture.recheck, err = application.NewRecheckPendingIdentityHandler(application.RecheckPendingIdentityDeps{
		Identity: fixture.identity, ServiceResults: fixture.services, Receptions: fixture.store, Pending: fixture.store,
		Versions: fixture.versions, Downstream: fixture.downstream, Scope: newScopeEvidence(),
		Clock: fixedClock{at: deliveredAt.Add(2 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("重核编排：%v", err)
	}
	fixture.dispose, err = application.NewDisposeIdentityConflictHandler(application.DisposeIdentityConflictDeps{
		ServiceResults: fixture.services, Receptions: fixture.store, Register: fixture.store,
		Versions: fixture.versions, Downstream: fixture.downstream, Clock: fixedClock{at: fixture.disposedNow},
	})
	if err != nil {
		t.Fatalf("处置编排：%v", err)
	}
	return fixture
}

// arrive 让一件实物贴着 mark 经收寄编排送站，PS 照 candidates 作答，交回落册的那一行。
func (fixture *dispositionFixture) arrive(t *testing.T, sourceID, unit string, ambiguous bool, candidates ...domain.ParcelAssociationReference) ports.ReceptionRecord {
	t.Helper()
	return fixture.arriveAt(t, oneMarkCommand(t, sourceID, unit), ambiguous, candidates...)
}

// arriveAt 同 arrive，命令由调用方给：同一件实物在另一个节点再送站才另起一条判断链，在同一节点再来是并入已有收寄（ADR-0199
// 决定四），不成链。
func (fixture *dispositionFixture) arriveAt(t *testing.T, command application.ReceiveDeliveredUnitCommand, ambiguous bool, candidates ...domain.ParcelAssociationReference) ports.ReceptionRecord {
	t.Helper()
	sourceID := command.SourceID
	fixture.identity.candidates, fixture.identity.ambiguous = candidates, ambiguous
	result, err := fixture.receive.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("收寄 %s：%v", sourceID, err)
	}
	record, _ := result.Record()
	return record
}

// oneMarkOnTwoUnits 落一码两物：A 先到关联成立，B 后到记身份冲突，A 那条链追加一版冲突版。交回 A 的原判断、A 的冲突版与 B。
func (fixture *dispositionFixture) oneMarkOnTwoUnits(t *testing.T) (ports.ReceptionRecord, ports.ReceptionRecord, ports.ReceptionRecord) {
	t.Helper()
	early := fixture.arrive(t, "SYN-DELIVERY-NRI06-A", "SYN-UNIT-NRI06-A", false, fixture.parcel)
	late := fixture.arrive(t, "SYN-DELIVERY-NRI06-B", "SYN-UNIT-NRI06-B", false, fixture.parcel)
	if early.Kind != ports.RecordIntakeFormed || late.Kind != ports.RecordPendingIdentification || !late.IdentityConflict {
		t.Fatalf("一码两物没落成：A %v / B %v 冲突 %v", early.Kind, late.Kind, late.IdentityConflict)
	}
	conflict := fixture.head(t, "SYN-UNIT-NRI06-A")
	if conflict.Predecessor != early.JudgmentVersion || !conflict.IdentityConflict {
		t.Fatalf("A 那条链的链头 = %+v, want 接续原判断的冲突版", conflict)
	}
	return early, conflict, late
}

// head 交回这件实物唯一那条判断链的当前有效版本。
func (fixture *dispositionFixture) head(t *testing.T, unit string) ports.ReceptionRecord {
	t.Helper()
	heads := fixture.heads(t, unit)
	if len(heads) != 1 {
		t.Fatalf("%s 有 %d 条链, want 1", unit, len(heads))
	}
	return heads[0]
}

func (fixture *dispositionFixture) heads(t *testing.T, unit string) []ports.ReceptionRecord {
	t.Helper()
	var heads []ports.ReceptionRecord
	for _, record := range fixture.store.recordsOfUnit(unit) {
		if _, succeeded := fixture.store.successorOf(record.Key.TenantID, record.JudgmentVersion); !succeeded {
			heads = append(heads, record)
		}
	}
	return heads
}

func (fixture *dispositionFixture) command(t *testing.T, unit string, disposed ports.ReceptionRecord, result domain.IdentityDispositionResult) application.DisposeIdentityConflictCommand {
	t.Helper()
	command := application.DisposeIdentityConflictCommand{
		TenantID: mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Operator: dispositionOperator,
		Unit:     mustValue(t, domain.NewHandlingUnitID, unit),
		Disposed: disposed.JudgmentVersion,
		Result:   result,
		Basis:    "SYN-DISPOSITION-BASIS-NRI06",
	}
	if result == domain.DispositionDesignatesParcel {
		command.Parcel = fixture.parcel
	}
	return command
}

func coDisposed(t *testing.T, unit string, version domain.IntakeResultVersion) domain.CoDisposedUnit {
	t.Helper()
	return domain.CoDisposedUnit{Unit: mustValue(t, domain.NewHandlingUnitID, unit), Version: version}
}

func (fixture *dispositionFixture) disposeOK(t *testing.T, command application.DisposeIdentityConflictCommand) application.DisposeIdentityConflictResult {
	t.Helper()
	result, err := fixture.dispose.Dispose(context.Background(), command)
	if err != nil {
		t.Fatalf("处置：%v", err)
	}
	return result
}

// versionOn 交回处置追加在 predecessor 之后的那一版。
func (fixture *dispositionFixture) versionOn(t *testing.T, predecessor ports.ReceptionRecord) ports.ReceptionRecord {
	t.Helper()
	record, found := fixture.store.byKey[ports.ReceptionKey{TenantID: predecessor.Key.TenantID, SourceID: "IDENTITY-DISPOSITION/" + predecessor.JudgmentVersion.String()}]
	if !found {
		t.Fatalf("%s 之后没有处置版", predecessor.JudgmentVersion)
	}
	return record
}

func intentSources(intents []ports.NodeIntakeHandoffIntent) []string {
	var sources []string
	for _, intent := range intents {
		sources = append(sources, intent.Record.Key.SourceID)
	}
	return sources
}

// Covers: `AT-NO-053` 正向：一码两物之后，持授予的操作者指定后到那件（B）是那件包裹，同一次处置把先到那件（A）列为连带实物。
// B 那条链追加一版`节点收寄已形成`关联 X、照 ADR-0186 取服务结果；A 那条链追加一版「不是 X」的待识别、不记冲突。两版都指名
// 前版、来源标识由前版派生、收寄事实与控制照前版、来源文档取各自链起点那一版的、带同一份处置文档与同一个比对摘要。原判断与
// 冲突版逐字不变。身份一格 B 成立、A 缺少·待识别；意图一封、属 B 那一版；A 早先交出的那一封不撤回。
func TestDesignatingTheLateUnitDisposesBothUnitsOfOneMark(t *testing.T) {
	fixture := newDispositionFixture(t)
	early, conflict, late := fixture.oneMarkOnTwoUnits(t)
	intentsBefore := slices.Clone(fixture.downstream.intents)
	rowsBefore := map[ports.ReceptionKey]ports.ReceptionRecord{}
	for key, record := range fixture.store.byKey {
		rowsBefore[key] = record
	}

	command := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionDesignatesParcel)
	command.CoDisposed = []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", conflict.JudgmentVersion)}
	result := fixture.disposeOK(t, command)
	if result.Outcome() != application.IdentityConflictDisposed || result.Outcome().String() != "IDENTITY_CONFLICT_DISPOSED" {
		t.Fatalf("outcome = %v, want IDENTITY_CONFLICT_DISPOSED", result.Outcome())
	}

	designated := fixture.versionOn(t, late)
	notIt := fixture.versionOn(t, conflict)
	association, associated := designated.Intake.Association()
	switch {
	case designated.Kind != ports.RecordIntakeFormed || !associated || association != fixture.parcel:
		t.Fatalf("B 的处置版 = %+v, want 节点收寄已形成、关联 X", designated)
	case designated.Predecessor != late.JudgmentVersion || designated.Intake.Version() != designated.JudgmentVersion:
		t.Fatalf("B 的处置版前版 %s、收寄版本 %s", designated.Predecessor, designated.Intake.Version())
	case !designated.Intake.ReceivedAt().Equal(late.Intake.ReceivedAt()) || designated.Control != late.Control || designated.Source != late.Source:
		t.Fatal("B 的处置版没照前版的收寄事实、控制与链起点的来源文档")
	case designated.ServiceResultCheck != ports.ServiceResultsChecked || designated.IdentityConflict || len(designated.Candidates) != 0:
		t.Fatalf("B 的处置版核对态 %q、冲突 %v、候选 %v", designated.ServiceResultCheck, designated.IdentityConflict, designated.Candidates)
	}
	if _, notAssociated := notIt.Intake.Association(); notAssociated || notIt.Kind != ports.RecordPendingIdentification ||
		notIt.IdentityConflict || len(notIt.Candidates) != 0 || notIt.Predecessor != conflict.JudgmentVersion ||
		notIt.Control != early.Control || !notIt.Intake.ReceivedAt().Equal(early.Intake.ReceivedAt()) {
		t.Fatalf("A 的处置版 = %+v, want 不带关联、不记冲突的待识别，接续 A 的冲突版", notIt)
	}
	if notIt.Source != early.Source || notIt.Source == conflict.Source {
		t.Fatal("A 的处置版来源文档不是 A 那条链起点那一版的——冲突版带的是 B 那次送站的来源")
	}
	if notIt.ContentDigest != designated.ContentDigest || !strings.HasPrefix(designated.ContentDigest, "NOC-1:") {
		t.Fatalf("同一次处置的两版摘要 %q / %q", designated.ContentDigest, notIt.ContentDigest)
	}
	for name, version := range map[string]ports.ReceptionRecord{"B": designated, "A": notIt} {
		doc := version.Disposition
		if !doc.Recorded() || doc.Disposed != late.JudgmentVersion || doc.Unit.String() != "SYN-UNIT-NRI06-B" ||
			doc.Result != domain.DispositionDesignatesParcel || doc.Parcel != fixture.parcel || doc.Operator != dispositionOperator ||
			doc.Basis != "SYN-DISPOSITION-BASIS-NRI06" || !doc.DisposedAt.Equal(fixture.disposedNow) ||
			len(doc.CoDisposed) != 1 || doc.CoDisposed[0].Version != conflict.JudgmentVersion {
			t.Fatalf("%s 的处置文档 = %+v", name, doc)
		}
	}
	if excluded := notIt.Disposition.ExcludedFor(notIt.Intake.Unit()); len(excluded) != 1 || excluded[0] != fixture.parcel {
		t.Fatalf("A 那一版排除 %v, want 只有 X", excluded)
	}
	for key, before := range rowsBefore {
		if after := fixture.store.byKey[key]; after.JudgmentVersion != before.JudgmentVersion || after.Kind != before.Kind ||
			after.IdentityConflict != before.IdentityConflict || after.Disposition.Recorded() {
			t.Fatalf("%s 被改了", key.SourceID)
		}
	}

	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-B")); len(reasons) != 0 {
		t.Fatalf("B 的身份一格缺少 %v, want 成立", reasons)
	}
	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-A")); !slices.Equal(reasons, []domain.DirectionalBasisReason{domain.ReasonPendingIdentification}) {
		t.Fatalf("A 的身份一格缺少 %v, want 只缺·待识别", reasons)
	}
	added := fixture.downstream.intents[len(intentsBefore):]
	if !slices.Equal(intentSources(fixture.downstream.intents[:len(intentsBefore)]), intentSources(intentsBefore)) ||
		len(added) != 1 || added[0].Record.Key != designated.Key {
		t.Fatalf("意图 %v, want 原有的照留、新交一封属 B 的处置版", intentSources(fixture.downstream.intents))
	}
	if key, handed := result.IntakeHandedOff(); !handed || key != designated.Key {
		t.Fatalf("结果说交意图的那一版 = %v %v", key, handed)
	}
	if versions := result.Versions(); len(versions) != 2 {
		t.Fatalf("结果带回 %d 版, want 2", len(versions))
	}
}

// Covers: `AT-NO-053` 反向：先处置先到那件（A），指定那件包裹——B 没有关联，不必连带。A 此前已就那件包裹交出过收寄结果，不再交
// 意图。B 另行处置为都不是：待识别版排除 X、不交意图。之后 A 成立、B 缺少·待识别。
func TestDesignatingTheEarlyUnitNeedsNoCoDispositionAndHandsNoSecondIntent(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, conflict, late := fixture.oneMarkOnTwoUnits(t)
	intents := len(fixture.downstream.intents)

	result := fixture.disposeOK(t, fixture.command(t, "SYN-UNIT-NRI06-A", conflict, domain.DispositionDesignatesParcel))
	if result.Outcome() != application.IdentityConflictDisposed {
		t.Fatalf("处置 A：outcome = %v", result.Outcome())
	}
	if _, handed := result.IntakeHandedOff(); handed || len(fixture.downstream.intents) != intents {
		t.Fatal("A 此前已就 X 交出过收寄结果，处置又交了一封")
	}
	if association, associated := fixture.versionOn(t, conflict).Intake.Association(); !associated || association != fixture.parcel {
		t.Fatal("A 的处置版没关联 X")
	}

	none := fixture.disposeOK(t, fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionNoneOfCandidates))
	if none.Outcome() != application.IdentityConflictDisposed || len(fixture.downstream.intents) != intents {
		t.Fatalf("处置 B 为都不是：outcome = %v、意图 %d", none.Outcome(), len(fixture.downstream.intents))
	}
	notIt := fixture.versionOn(t, late)
	if excluded := notIt.Disposition.ExcludedFor(notIt.Intake.Unit()); len(excluded) != 1 || excluded[0] != fixture.parcel || notIt.IdentityConflict {
		t.Fatalf("B 的处置版排除 %v、冲突 %v", excluded, notIt.IdentityConflict)
	}
	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-A")); len(reasons) != 0 {
		t.Fatalf("A 缺少 %v, want 成立", reasons)
	}
	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-B")); !slices.Equal(reasons, []domain.DirectionalBasisReason{domain.ReasonPendingIdentification}) {
		t.Fatalf("B 缺少 %v, want 只缺·待识别", reasons)
	}
}

// Covers: `AT-NO-054`：多候选与恰一个候选带歧义信号各一件，处置确认都不是——待识别版、不记冲突、不带候选、记下排除的那一组（就是
// 处置候选），不交意图，身份一格由身份冲突变为待识别。之后重核：PS 仍只唯一命中被排除的那件，不追加、照零候选答；号码另有新的
// 包裹关联、重核唯一命中它，照常追加关联版。
func TestConfirmingNoneOfTheCandidatesKeepsTheUnitPendingAndRecheckSkipsTheExcluded(t *testing.T) {
	fixture := newDispositionFixture(t)
	y := mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y")
	z := mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Z")
	w := mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-W")
	many := fixture.arrive(t, "SYN-DELIVERY-NRI06-MANY", "SYN-UNIT-NRI06-MANY", false, z, y)
	ambiguous := fixture.arrive(t, "SYN-DELIVERY-NRI06-AMB", "SYN-UNIT-NRI06-AMB", true, y)
	intents := len(fixture.downstream.intents)

	for _, testCase := range []struct {
		unit     string
		record   ports.ReceptionRecord
		excluded []domain.ParcelAssociationReference
	}{
		{"SYN-UNIT-NRI06-MANY", many, []domain.ParcelAssociationReference{y, z}},
		{"SYN-UNIT-NRI06-AMB", ambiguous, []domain.ParcelAssociationReference{y}},
	} {
		if !testCase.record.IdentityConflict {
			t.Fatalf("%s 没落成身份冲突", testCase.unit)
		}
		if got := fixture.disposeOK(t, fixture.command(t, testCase.unit, testCase.record, domain.DispositionNoneOfCandidates)); got.Outcome() != application.IdentityConflictDisposed {
			t.Fatalf("%s：outcome = %v", testCase.unit, got.Outcome())
		}
		notIt := fixture.versionOn(t, testCase.record)
		if notIt.Kind != ports.RecordPendingIdentification || notIt.IdentityConflict || len(notIt.Candidates) != 0 ||
			!slices.Equal(notIt.Disposition.Excluded, testCase.excluded) || !slices.Equal(notIt.Disposition.ExcludedFor(notIt.Intake.Unit()), testCase.excluded) {
			t.Fatalf("%s 的处置版 = %+v", testCase.unit, notIt)
		}
		if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit(testCase.unit)); !slices.Equal(reasons, []domain.DirectionalBasisReason{domain.ReasonPendingIdentification}) {
			t.Fatalf("%s 缺少 %v, want 只缺·待识别", testCase.unit, reasons)
		}
	}
	if len(fixture.downstream.intents) != intents {
		t.Fatal("确认都不是交了意图")
	}

	notMany := fixture.versionOn(t, many)
	fixture.identity.candidates, fixture.identity.ambiguous = []domain.ParcelAssociationReference{y}, false
	rows := len(fixture.store.byKey)
	still, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{TenantID: notMany.Key.TenantID, Version: notMany.JudgmentVersion})
	if err != nil || still.Outcome() != application.RecheckStillUnidentified || len(fixture.store.byKey) != rows {
		t.Fatalf("唯一命中被排除的那件：outcome = %v err = %v、册上多了 %d 行", still.Outcome(), err, len(fixture.store.byKey)-rows)
	}
	byMark, err := fixture.recheck.RecheckByMark(context.Background(), notMany.Key.TenantID, "SYN-BARCODE-NRI01")
	if err != nil || len(fixture.store.byKey) != rows {
		t.Fatalf("门铃按号重核：err = %v、册上多了 %d 行", err, len(fixture.store.byKey)-rows)
	}
	for _, result := range byMark {
		if result.Outcome() != application.RecheckStillUnidentified {
			t.Fatalf("门铃重核 %s：outcome = %v, want STILL_UNIDENTIFIED", result.Rechecked(), result.Outcome())
		}
	}

	fixture.identity.candidates = []domain.ParcelAssociationReference{w}
	formed, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{TenantID: notMany.Key.TenantID, Version: notMany.JudgmentVersion})
	if err != nil || formed.Outcome() != application.RecheckIntakeFormed {
		t.Fatalf("号码另有新的包裹关联：outcome = %v err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	if record, _ := formed.Record(); record.Predecessor != notMany.JudgmentVersion {
		t.Fatalf("重核版前版 %s, want 处置版 %s", record.Predecessor, notMany.JudgmentVersion)
	}
}

// Covers: `AT-NO-055`：指定的包裹此刻关联在先到那件上——连带实物漏列、多列、所带版本过时、那件实物另牵连别的包裹，各一条，都答
// 关联对象另有归属，回显此刻关联到 X 的实物与它们的当前有效版本；不落版、不耗版本号、不交意图。
func TestADesignationMustCoDisposeExactlyTheUnitsAssociatedNow(t *testing.T) {
	type arranged struct {
		fixture  *dispositionFixture
		early    ports.ReceptionRecord
		conflict ports.ReceptionRecord
		late     ports.ReceptionRecord
	}
	arrange := func(t *testing.T) arranged {
		fixture := newDispositionFixture(t)
		early, conflict, late := fixture.oneMarkOnTwoUnits(t)
		return arranged{fixture, early, conflict, late}
	}
	cases := map[string]func(t *testing.T, a arranged) []domain.CoDisposedUnit{
		"漏列": func(*testing.T, arranged) []domain.CoDisposedUnit { return nil },
		"多列": func(t *testing.T, a arranged) []domain.CoDisposedUnit {
			return []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", a.conflict.JudgmentVersion), coDisposed(t, "SYN-UNIT-NRI06-C", a.conflict.JudgmentVersion)}
		},
		"版本过时": func(t *testing.T, a arranged) []domain.CoDisposedUnit {
			return []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", a.early.JudgmentVersion)}
		},
		"另牵连别的包裹": func(t *testing.T, a arranged) []domain.CoDisposedUnit {
			again := oneMarkCommand(t, "SYN-DELIVERY-NRI06-A-AGAIN", "SYN-UNIT-NRI06-A")
			again.Node = mustValue(t, domain.NewNodeReference, "SYN-NODE-NRI06-OTHER")
			a.fixture.arriveAt(t, again, false,
				mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y"), mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Z"))
			return []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", a.conflict.JudgmentVersion)}
		},
	}
	for name, coDisposedOf := range cases {
		t.Run(name, func(t *testing.T) {
			a := arrange(t)
			command := a.fixture.command(t, "SYN-UNIT-NRI06-B", a.late, domain.DispositionDesignatesParcel)
			command.CoDisposed = coDisposedOf(t, a)
			rows, issued, intents := len(a.fixture.store.byKey), a.fixture.versions.next, len(a.fixture.downstream.intents)
			result := a.fixture.disposeOK(t, command)
			if result.Outcome() != application.DispositionAssociatedElsewhere || result.Outcome().String() != "ASSOCIATED_ELSEWHERE" {
				t.Fatalf("outcome = %v, want ASSOCIATED_ELSEWHERE", result.Outcome())
			}
			associated := result.AssociatedUnits()
			if len(associated) != 1 || associated[0].Unit.String() != "SYN-UNIT-NRI06-A" {
				t.Fatalf("回显 %+v, want 只有 A", associated)
			}
			var versions []domain.IntakeResultVersion
			for _, head := range associated[0].CurrentVersions {
				versions = append(versions, head.JudgmentVersion)
			}
			if !slices.Contains(versions, a.conflict.JudgmentVersion) {
				t.Fatalf("回显 A 的当前有效版本 %v, want 含 A 的冲突版 %s", versions, a.conflict.JudgmentVersion)
			}
			if len(a.fixture.store.byKey) != rows || a.fixture.versions.next != issued || len(a.fixture.downstream.intents) != intents {
				t.Fatal("答关联对象另有归属却落了版、耗了版本号或交了意图")
			}
		})
	}
}

// Covers: `AT-NO-056` 代数：原样重投答已有结果并回显在册那次；换处置依据引用或换操作者原样重投仍答已有结果、回显原值；换结果或
// 换指定的包裹答已处置、回显在册那次；三者都不追加、不耗版本号、不重复意图。
func TestReplayingADispositionAnswersTheRecordedOne(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, conflict, late := fixture.oneMarkOnTwoUnits(t)
	command := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionDesignatesParcel)
	command.CoDisposed = []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", conflict.JudgmentVersion)}
	fixture.disposeOK(t, command)
	designated := fixture.versionOn(t, late)
	rows, issued, intents := len(fixture.store.byKey), fixture.versions.next, len(fixture.downstream.intents)

	renamed := command
	renamed.Basis, renamed.Operator = "SYN-DISPOSITION-BASIS-NRI06-AGAIN", "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-OTHER"
	otherResult := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionNoneOfCandidates)
	otherParcel := command
	otherParcel.Parcel = mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y")
	for name, testCase := range map[string]struct {
		command application.DisposeIdentityConflictCommand
		want    application.DispositionOutcome
		wire    string
	}{
		"原样重投":      {command, application.DispositionExistingResult, "EXISTING_RESULT"},
		"换依据引用与操作者": {renamed, application.DispositionExistingResult, "EXISTING_RESULT"},
		"换结果":       {otherResult, application.DispositionAlreadyDisposed, "ALREADY_DISPOSED"},
		"换指定的包裹":    {otherParcel, application.DispositionAlreadyDisposed, "ALREADY_DISPOSED"},
	} {
		result := fixture.disposeOK(t, testCase.command)
		if result.Outcome() != testCase.want || result.Outcome().String() != testCase.wire {
			t.Fatalf("%s：outcome = %v, want %s", name, result.Outcome(), testCase.wire)
		}
		recorded, present := result.Disposition()
		if !present || recorded.Basis != "SYN-DISPOSITION-BASIS-NRI06" || recorded.Operator != dispositionOperator || recorded.Parcel != fixture.parcel {
			t.Fatalf("%s：回显 %+v, want 在册那次的原值", name, recorded)
		}
		if versions := result.Versions(); len(versions) != 1 || versions[0].Key != designated.Key {
			t.Fatalf("%s：回显的版本 %+v, want 接续所处置版本的那一版", name, versions)
		}
		if len(fixture.store.byKey) != rows || fixture.versions.next != issued || len(fixture.downstream.intents) != intents {
			t.Fatalf("%s：落了版、耗了版本号或交了意图", name)
		}
	}
}

// Covers: `AT-NO-056` 换代：所处置的版本已被第三件到站追加的冲突版接续——答版本已换代，回显这件实物此刻各链的当前有效版本；不追加、
// 不耗版本号、不交意图。
func TestDisposingASupersededVersionAnswersVersionSuperseded(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, conflict, _ := fixture.oneMarkOnTwoUnits(t)
	fixture.arrive(t, "SYN-DELIVERY-NRI06-C", "SYN-UNIT-NRI06-C", false, fixture.parcel)
	current := fixture.head(t, "SYN-UNIT-NRI06-A")
	if current.Predecessor != conflict.JudgmentVersion {
		t.Fatalf("第三件到站没在 A 那条链上接续冲突版：%+v", current)
	}
	rows, issued, intents := len(fixture.store.byKey), fixture.versions.next, len(fixture.downstream.intents)
	result := fixture.disposeOK(t, fixture.command(t, "SYN-UNIT-NRI06-A", conflict, domain.DispositionDesignatesParcel))
	if result.Outcome() != application.DispositionVersionSuperseded || result.Outcome().String() != "VERSION_SUPERSEDED" {
		t.Fatalf("outcome = %v, want VERSION_SUPERSEDED", result.Outcome())
	}
	if heads := result.CurrentVersions(); len(heads) != 1 || heads[0].JudgmentVersion != current.JudgmentVersion {
		t.Fatalf("回显 %+v, want A 此刻的链头 %s", heads, current.JudgmentVersion)
	}
	if len(fixture.store.byKey) != rows || fixture.versions.next != issued || len(fixture.downstream.intents) != intents {
		t.Fatal("答版本已换代却落了版、耗了版本号或交了意图")
	}
}

// Covers: `AT-NO-057`：指定处置候选之外的包裹答不在候选之内、回显处置候选；没有身份冲突的实物（关联成立的、零候选待识别的）答无可
// 处置的冲突。都不落版。`AT-NO-058` 的编排半边：指名的实物或版本在本租户查不到（别的租户、实物与版本对不上）交回
// ErrDispositionSubjectNotFound，跨租户与查无此物同答。
func TestRejectedDispositionsLandNothing(t *testing.T) {
	fixture := newDispositionFixture(t)
	early, conflict, late := fixture.oneMarkOnTwoUnits(t)
	outsider := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionDesignatesParcel)
	outsider.Parcel = mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-OUTSIDE")
	outsider.CoDisposed = []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", conflict.JudgmentVersion)}
	rows, issued := len(fixture.store.byKey), fixture.versions.next

	result := fixture.disposeOK(t, outsider)
	if result.Outcome() != application.DispositionNotACandidate || result.Outcome().String() != "NOT_A_CANDIDATE" ||
		!slices.Equal(result.Candidates(), []domain.ParcelAssociationReference{fixture.parcel}) {
		t.Fatalf("候选之外：outcome = %v、候选 %v", result.Outcome(), result.Candidates())
	}

	pending := fixture.arrive(t, "SYN-DELIVERY-NRI06-ZERO", "SYN-UNIT-NRI06-ZERO", false)
	solo := fixture.arrive(t, "SYN-DELIVERY-NRI06-SOLO", "SYN-UNIT-NRI06-SOLO", false, mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-SOLO"))
	rows, issued = len(fixture.store.byKey), fixture.versions.next
	for unit, record := range map[string]ports.ReceptionRecord{"SYN-UNIT-NRI06-ZERO": pending, "SYN-UNIT-NRI06-SOLO": solo} {
		got := fixture.disposeOK(t, fixture.command(t, unit, record, domain.DispositionNoneOfCandidates))
		if got.Outcome() != application.DispositionNoIdentityConflict || got.Outcome().String() != "NO_IDENTITY_CONFLICT" {
			t.Fatalf("%s：outcome = %v, want NO_IDENTITY_CONFLICT", unit, got.Outcome())
		}
	}
	if len(fixture.store.byKey) != rows || fixture.versions.next != issued {
		t.Fatal("被拒的处置落了版或耗了版本号")
	}

	foreign := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionNoneOfCandidates)
	foreign.TenantID = mustValue(t, domain.NewTenantID, "SYN-TENANT-02")
	mismatched := fixture.command(t, "SYN-UNIT-NRI06-A", late, domain.DispositionNoneOfCandidates)
	absent := fixture.command(t, "SYN-UNIT-NRI06-B", early, domain.DispositionNoneOfCandidates)
	absent.Disposed = mustValue(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI06-ABSENT")
	for name, command := range map[string]application.DisposeIdentityConflictCommand{"别的租户": foreign, "实物与版本对不上": mismatched, "查无此版": absent} {
		if _, err := fixture.dispose.Dispose(context.Background(), command); !errors.Is(err, application.ErrDispositionSubjectNotFound) {
			t.Fatalf("%s：err = %v, want ErrDispositionSubjectNotFound", name, err)
		}
	}
	blank := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionNoneOfCandidates)
	blank.Basis = "  "
	if _, err := fixture.dispose.Dispose(context.Background(), blank); !errors.Is(err, application.ErrDispositionCommandRejected) {
		t.Fatalf("依据空白：err = %v, want ErrDispositionCommandRejected", err)
	}
}

// Covers: `AT-NO-060`：一件实物在两条判断链上都有当前有效版本（在两个节点各送站一次：一次多候选冲突、一次零候选），处置指名其中一版——两条链
// 各追加一版处置版；指定一件时意图只交一封，落在接收时刻最早的那一版上。身份一格不再被另一条链上的旧判断左右。
func TestADispositionAppendsOnEveryChainOfTheUnit(t *testing.T) {
	y := func(t *testing.T) domain.ParcelAssociationReference {
		return mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y")
	}
	arrange := func(t *testing.T) (*dispositionFixture, ports.ReceptionRecord, ports.ReceptionRecord) {
		fixture := newDispositionFixture(t)
		fixture.identity.candidates = []domain.ParcelAssociationReference{y(t), mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Z")}
		later := oneMarkCommand(t, "SYN-DELIVERY-NRI06-M1", "SYN-UNIT-NRI06-M")
		later.OccurredAt = deliveredAt.Add(time.Hour)
		conflictResult, err := fixture.receive.Handle(context.Background(), later)
		if err != nil {
			t.Fatal(err)
		}
		conflicted, _ := conflictResult.Record()
		other := oneMarkCommand(t, "SYN-DELIVERY-NRI06-M2", "SYN-UNIT-NRI06-M")
		other.Node = mustValue(t, domain.NewNodeReference, "SYN-NODE-NRI06-OTHER")
		earlier := fixture.arriveAt(t, other, false)
		if len(fixture.heads(t, "SYN-UNIT-NRI06-M")) != 2 || !conflicted.IdentityConflict || earlier.IdentityConflict {
			t.Fatal("没落成两条链")
		}
		return fixture, conflicted, earlier
	}

	fixture, conflicted, earlier := arrange(t)
	if got := fixture.disposeOK(t, fixture.command(t, "SYN-UNIT-NRI06-M", conflicted, domain.DispositionNoneOfCandidates)); got.Outcome() != application.IdentityConflictDisposed || len(got.Versions()) != 2 {
		t.Fatalf("都不是：outcome = %v、%d 版", got.Outcome(), len(got.Versions()))
	}
	onEarlier := fixture.versionOn(t, earlier)
	if onEarlier.Source != earlier.Source || !slices.Equal(onEarlier.Disposition.Excluded, []domain.ParcelAssociationReference{y(t), mustValue(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Z")}) {
		t.Fatalf("另一条链上的处置版 = %+v", onEarlier)
	}
	fixture.versionOn(t, conflicted)
	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-M")); !slices.Equal(reasons, []domain.DirectionalBasisReason{domain.ReasonPendingIdentification}) {
		t.Fatalf("缺少 %v, want 只缺·待识别", reasons)
	}

	fixture, conflicted, earlier = arrange(t)
	designate := fixture.command(t, "SYN-UNIT-NRI06-M", conflicted, domain.DispositionDesignatesParcel)
	designate.Parcel = y(t)
	intents := len(fixture.downstream.intents)
	result := fixture.disposeOK(t, designate)
	if result.Outcome() != application.IdentityConflictDisposed {
		t.Fatalf("指定一件：outcome = %v", result.Outcome())
	}
	onEarlier, onLater := fixture.versionOn(t, earlier), fixture.versionOn(t, conflicted)
	for _, version := range []ports.ReceptionRecord{onEarlier, onLater} {
		if association, associated := version.Intake.Association(); !associated || association != y(t) {
			t.Fatalf("%s 没关联 Y", version.Key.SourceID)
		}
	}
	added := fixture.downstream.intents[intents:]
	if len(added) != 1 || added[0].Record.Key != onEarlier.Key {
		t.Fatalf("意图 %v, want 恰一封、落在接收最早的那一版 %s", intentSources(added), onEarlier.Key.SourceID)
	}
	if reasons := identityReasonsOf(t, fixture.store.recordsOfUnit("SYN-UNIT-NRI06-M")); len(reasons) != 0 {
		t.Fatalf("缺少 %v, want 成立", reasons)
	}
}

// Covers: `AT-NO-059` 的编排面：处置之后贴同号的第三件到站，一码两物只按此刻关联到那件包裹的实物（B）认——冲突版追加在 B 那条链上，
// 被处置为「不是它」的 A 不再被追加冲突版。
func TestAThirdArrivalAfterADispositionConflictsOnlyWithTheUnitAssociatedNow(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, conflict, late := fixture.oneMarkOnTwoUnits(t)
	command := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionDesignatesParcel)
	command.CoDisposed = []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", conflict.JudgmentVersion)}
	fixture.disposeOK(t, command)
	designated, notIt := fixture.versionOn(t, late), fixture.versionOn(t, conflict)

	third := fixture.arrive(t, "SYN-DELIVERY-NRI06-C", "SYN-UNIT-NRI06-C", false, fixture.parcel)
	if !third.IdentityConflict {
		t.Fatal("第三件没落成身份冲突")
	}
	if head := fixture.head(t, "SYN-UNIT-NRI06-B"); head.Predecessor != designated.JudgmentVersion || !head.IdentityConflict {
		t.Fatalf("B 那条链的链头 = %+v, want 接续处置版的冲突版", head)
	}
	if head := fixture.head(t, "SYN-UNIT-NRI06-A"); head.JudgmentVersion != notIt.JudgmentVersion {
		t.Fatalf("A 那条链被追加了 %s——被处置为「不是它」的实物又被拉回冲突", head.Key.SourceID)
	}
}

// Covers: 取锁次序「先实物后包裹」（ADR-0210 决定四，与收寄同一个次序）：主实物与连带实物按标识排序取实物锁，之后才取包裹锁。
func TestADispositionHoldsTheUnitsBeforeTheParcel(t *testing.T) {
	fixture := newDispositionFixture(t)
	_, conflict, late := fixture.oneMarkOnTwoUnits(t)
	*fixture.store.holds = nil
	command := fixture.command(t, "SYN-UNIT-NRI06-B", late, domain.DispositionDesignatesParcel)
	command.CoDisposed = []domain.CoDisposedUnit{coDisposed(t, "SYN-UNIT-NRI06-A", conflict.JudgmentVersion)}
	fixture.disposeOK(t, command)
	holds := *fixture.store.holds
	if len(holds) < 3 || !slices.Equal(holds[:3], []string{"unit:SYN-UNIT-NRI06-A", "unit:SYN-UNIT-NRI06-B", "parcel:SYN-PARCEL-NRI06-X"}) {
		t.Fatalf("取锁次序 %v, want 先 A、B 两件实物，再那件包裹", holds)
	}
	for _, hold := range holds[3:] {
		if strings.HasPrefix(hold, "unit:") {
			t.Fatalf("取过包裹锁之后又取实物锁：%v", holds)
		}
	}
}
