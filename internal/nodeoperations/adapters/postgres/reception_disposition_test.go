package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件对真实 PostgreSQL 16 证身份冲突处置的存储面（ADR-0210 决定三、八；票 no-reception-identity/06）：处置文档随处置版落册
// 与读回、按实物读各行，以及按包裹读关联的两面认处置截断。夹具全为合成（S 级）。

const dispositionTenant = "SYN-TENANT-NRI06"

// chainVersion 拼 unit 的一版带收寄的判断：判断版本 version，前版 predecessor（空即首判）；association 非空即形成格，否则待识别。
func chainVersion(t *testing.T, sourceID, unit, version, predecessor, association string, candidates []string, conflict bool) ports.ReceptionRecord {
	t.Helper()
	record := formedRecord(t, dispositionTenant, sourceID)
	spec := domain.NodeIntakeSpec{
		TenantID:    record.Key.TenantID,
		Unit:        ref(t, domain.NewHandlingUnitID, unit),
		Node:        record.Intake.Node(),
		DeliveredBy: record.Intake.DeliveredBy(),
		Evidence:    record.Intake.Evidence(),
		Version:     ref(t, domain.NewIntakeResultVersion, version),
		ReceivedAt:  record.Intake.ReceivedAt(),
	}
	record.Kind = ports.RecordPendingIdentification
	record.ServiceMarkers, record.ServiceResultCheck = nil, ports.ServiceResultCheckAbsent
	if association != "" {
		spec.Association = ref(t, domain.NewParcelAssociationReference, association)
		record.Kind = ports.RecordIntakeFormed
		record.ServiceResultCheck = ports.ServiceResultsChecked
	}
	intake, err := domain.FormNodeIntake(spec)
	if err != nil {
		t.Fatalf("构造收寄：%v", err)
	}
	control, err := domain.EstablishPhysicalControl(domain.PhysicalControlSpec{
		TenantID: record.Key.TenantID, Unit: intake.Unit(), Node: intake.Node(), Kind: domain.EstablishedByNodeIntake,
		Basis: ref(t, domain.NewControlBasisReference, "NODE-INTAKE/"+unit), EstablishedAt: intake.ReceivedAt(),
	})
	if err != nil {
		t.Fatalf("构造控制：%v", err)
	}
	record.Intake, record.Control, record.JudgmentVersion = intake, control, intake.Version()
	record.Source = deliverySource(t, unit, "SYN-MARK-NRI06", "RECEIVED")
	if predecessor != "" {
		record.Predecessor = ref(t, domain.NewIntakeResultVersion, predecessor)
	}
	record.Candidates = nil
	for _, candidate := range candidates {
		record.Candidates = append(record.Candidates, ref(t, domain.NewParcelAssociationReference, candidate))
	}
	record.IdentityConflict = conflict
	return record
}

// disposed 把一版判断标成处置版：带处置文档（主实物 mainUnit、所处置的版本 disposedVersion）。
func disposed(t *testing.T, record ports.ReceptionRecord, mainUnit, disposedVersion string, result domain.IdentityDispositionResult, parcel string, excluded ...string) ports.ReceptionRecord {
	t.Helper()
	disposition := domain.IdentityDisposition{
		Disposed:   ref(t, domain.NewIntakeResultVersion, disposedVersion),
		Unit:       ref(t, domain.NewHandlingUnitID, mainUnit),
		Result:     result,
		Basis:      "SYN-DISPOSITION-BASIS-NRI06",
		Operator:   "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-NRI06",
		DisposedAt: receivedAt.Add(time.Hour),
	}
	if parcel != "" {
		disposition.Parcel = ref(t, domain.NewParcelAssociationReference, parcel)
	}
	for _, raw := range excluded {
		disposition.Excluded = append(disposition.Excluded, ref(t, domain.NewParcelAssociationReference, raw))
	}
	formed, err := domain.FormIdentityDisposition(disposition)
	if err != nil {
		t.Fatalf("立处置文档：%v", err)
	}
	record.Disposition = formed
	record.Key.SourceID = "IDENTITY-DISPOSITION/" + record.Predecessor.String()
	record.ContentDigest = "NOC-1:syn-disposition"
	return record
}

// oneMarkChains 是一码两物落册之后的形状：A 原判断关联 X、A 冲突版接续它；B 记身份冲突的待识别。
func oneMarkChains(t *testing.T) (ports.ReceptionRecord, ports.ReceptionRecord, ports.ReceptionRecord) {
	t.Helper()
	early := chainVersion(t, "SYN-DELIVERY-NRI06-A", "SYN-UNIT-NRI06-A", "SYN-V-A1", "", "SYN-PARCEL-NRI06-X", nil, false)
	conflict := chainVersion(t, "IDENTITY-CONFLICT/SYN-DELIVERY-NRI06-B/SYN-UNIT-NRI06-A", "SYN-UNIT-NRI06-A", "SYN-V-A2", "SYN-V-A1", "", []string{"SYN-PARCEL-NRI06-X"}, true)
	late := chainVersion(t, "SYN-DELIVERY-NRI06-B", "SYN-UNIT-NRI06-B", "SYN-V-B1", "", "", []string{"SYN-PARCEL-NRI06-X"}, true)
	return early, conflict, late
}

// dispositionOfOneMark 是指定 B 是 X、A 连带的那一次处置落下的两版。
func dispositionOfOneMark(t *testing.T) (ports.ReceptionRecord, ports.ReceptionRecord) {
	t.Helper()
	designated := disposed(t,
		chainVersion(t, "", "SYN-UNIT-NRI06-B", "SYN-V-B2", "SYN-V-B1", "SYN-PARCEL-NRI06-X", nil, false),
		"SYN-UNIT-NRI06-B", "SYN-V-B1", domain.DispositionDesignatesParcel, "SYN-PARCEL-NRI06-X")
	notIt := disposed(t,
		chainVersion(t, "", "SYN-UNIT-NRI06-A", "SYN-V-A3", "SYN-V-A2", "", nil, false),
		"SYN-UNIT-NRI06-B", "SYN-V-B1", domain.DispositionDesignatesParcel, "SYN-PARCEL-NRI06-X")
	return designated, notIt
}

func otherUnitSources(t *testing.T, repository interface {
	FindOtherUnitIntakes(context.Context, domain.TenantID, domain.ParcelAssociationReference, domain.HandlingUnitID) ([]ports.ReceptionRecord, error)
}, transactorRun func(func(context.Context) error), unit string) []string {
	t.Helper()
	var sources []string
	transactorRun(func(txCtx context.Context) error {
		records, err := repository.FindOtherUnitIntakes(txCtx, ref(t, domain.NewTenantID, dispositionTenant),
			ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-X"), ref(t, domain.NewHandlingUnitID, unit))
		sources = sourceIDsOf(records)
		return err
	})
	return sources
}

// Covers: ADR-0210 决定三的存储面——处置版带着 NID-1 处置文档落册、按键读回逐格不变；按实物读交回这件实物带收寄的各行（各条链、
// 被接续的与链头都在），按来源标识定序，别的实物与别的租户不在其中。处置文档只能落在指名了前版的形成或待识别那一版上，库面拦着。
func TestADispositionVersionKeepsItsDocumentAndTheUnitReadsEveryRow(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	early, conflict, late := oneMarkChains(t)
	designated, notIt := dispositionOfOneMark(t)
	saveReceptions(t, transactor, ctx, repository, early, conflict, late, designated, notIt)

	readBack, found, err := repository.FindByKey(ctx, notIt.Key)
	if err != nil || !found {
		t.Fatalf("读回处置版：found %v err %v", found, err)
	}
	doc := readBack.Disposition
	if !doc.Recorded() || doc.Disposed != notIt.Disposition.Disposed || doc.Unit != notIt.Disposition.Unit || doc.Result != domain.DispositionDesignatesParcel ||
		doc.Parcel != notIt.Disposition.Parcel || doc.Basis != notIt.Disposition.Basis || doc.Operator != notIt.Disposition.Operator ||
		!doc.DisposedAt.Equal(notIt.Disposition.DisposedAt) || readBack.Predecessor != notIt.Predecessor {
		t.Fatalf("处置文档读回 = %+v, want %+v", doc, notIt.Disposition)
	}
	if plain, _, err := repository.FindByKey(ctx, conflict.Key); err != nil || plain.Disposition.Recorded() {
		t.Fatalf("冲突版读回带了处置文档：%+v err %v", plain.Disposition, err)
	}

	rows, err := repository.FindUnitIntakes(ctx, ref(t, domain.NewTenantID, dispositionTenant), ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-A"))
	if err != nil {
		t.Fatalf("按实物读：%v", err)
	}
	if got := sourceIDsOf(rows); !slices.Equal(got, []string{conflict.Key.SourceID, notIt.Key.SourceID, early.Key.SourceID}) {
		t.Fatalf("按实物读 = %v", got)
	}
	if foreign, err := repository.FindUnitIntakes(ctx, ref(t, domain.NewTenantID, "SYN-TENANT-OTHER"), ref(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-A")); err != nil || len(foreign) != 0 {
		t.Fatalf("别的租户读到 %d 行 err %v", len(foreign), err)
	}

	headless := disposed(t, chainVersion(t, "", "SYN-UNIT-NRI06-C", "SYN-V-C1", "", "", nil, false),
		"SYN-UNIT-NRI06-C", "SYN-V-C0", domain.DispositionNoneOfCandidates, "", "SYN-PARCEL-NRI06-X")
	headless.Key.SourceID = "SYN-HEADLESS-DISPOSITION"
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, saveErr := repository.Save(txCtx, headless)
		return saveErr
	})
	if err == nil {
		t.Fatal("没指名前版的处置版落了册")
	}
}

// Covers: `AT-NO-059` 的存储面与 ADR-0210 决定八：按包裹读别的实物关联——还没处置时，A 的原判断照旧算 A 关联着 X（冲突版接续它
// 不截断）；处置之后，A 的原判断被处置版接续过，不再算；B 的处置版自己带的关联照算。
func TestOtherUnitIntakesStopAtADisposition(t *testing.T) {
	repository, transactor := newReceptions(t)
	ctx := t.Context()
	run := func(fn func(context.Context) error) { inTx(t, transactor, ctx, fn) }
	early, conflict, late := oneMarkChains(t)
	saveReceptions(t, transactor, ctx, repository, early, conflict, late)
	if got := otherUnitSources(t, repository, run, "SYN-UNIT-NRI06-C"); !slices.Equal(got, []string{early.Key.SourceID}) {
		t.Fatalf("还没处置：别的实物关联 = %v, want 只有 A 的原判断", got)
	}

	designated, notIt := dispositionOfOneMark(t)
	saveReceptions(t, transactor, ctx, repository, designated, notIt)
	if got := otherUnitSources(t, repository, run, "SYN-UNIT-NRI06-C"); !slices.Equal(got, []string{designated.Key.SourceID}) {
		t.Fatalf("处置之后：别的实物关联 = %v, want 只有 B 的处置版", got)
	}
	if got := otherUnitSources(t, repository, run, "SYN-UNIT-NRI06-B"); len(got) != 0 {
		t.Fatalf("处置之后问 B 之外的实物：%v, want 没有——A 已被处置为不是它", got)
	}
}

// Covers: `AT-NO-059` 的在袋查询半边（ADR-0210 决定八）：A 在袋。还没处置时 A 的原判断关联 X，答在；处置之后 A 的原判断与冲突版都
// 被处置版接续过，答不在。B 在袋时，处置前 B 记冲突、候选列着 X，答不可归属；B 被处置为「都不是」之后，候选那一路也不再算。
func TestParcelContainmentStopsAtADisposition(t *testing.T) {
	fixture := newContainmentFixture(t)
	ctx := t.Context()
	early, conflict, late := oneMarkChains(t)
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, early, conflict, late)
	fixture.bagWith(t, ctx, dispositionTenant, "SYN-BAG-NRI06-A", "SYN-UNIT-NRI06-A")
	assertContainment(t, fixture.load(t, dispositionTenant, "SYN-PARCEL-NRI06-X"), ports.ParcelContained)

	_, notIt := dispositionOfOneMark(t)
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, notIt)
	assertContainment(t, fixture.load(t, dispositionTenant, "SYN-PARCEL-NRI06-X"), ports.ParcelNotContained)

	fixture.bagWith(t, ctx, dispositionTenant, "SYN-BAG-NRI06-B", "SYN-UNIT-NRI06-B")
	assertContainment(t, fixture.load(t, dispositionTenant, "SYN-PARCEL-NRI06-X"), ports.ParcelContainmentUnattributable)
	none := disposed(t, chainVersion(t, "", "SYN-UNIT-NRI06-B", "SYN-V-B2", "SYN-V-B1", "", nil, false),
		"SYN-UNIT-NRI06-B", "SYN-V-B1", domain.DispositionNoneOfCandidates, "", "SYN-PARCEL-NRI06-X")
	saveReceptions(t, fixture.transactor, ctx, fixture.receptions, none)
	assertContainment(t, fixture.load(t, dispositionTenant, "SYN-PARCEL-NRI06-X"), ports.ParcelNotContained)
}

// Covers: 票 06「做什么」第 7 项的存储面——待识别实物页按实物把各链当前有效版本列在一起，分得出冲突是否已处置、处置了什么。只列
// 有过待识别那一格的实物；每件交回它各条链的链头（形成格也在），处置版带处置文档。
func TestIdentityReviewUnitsListEveryChainHeadWithItsDisposition(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	early, conflict, late := oneMarkChains(t)
	other := chainVersion(t, "SYN-DELIVERY-NRI06-A-AGAIN", "SYN-UNIT-NRI06-A", "SYN-V-A9", "", "", nil, false)
	solo := chainVersion(t, "SYN-DELIVERY-NRI06-SOLO", "SYN-UNIT-NRI06-SOLO", "SYN-V-S1", "", "SYN-PARCEL-NRI06-S", nil, false)
	designated, notIt := dispositionOfOneMark(t)
	saveReceptions(t, transactor, ctx, receptions, early, conflict, late, other, solo, designated, notIt)

	units, err := catalogue.ListIdentityReviewUnits(ctx, ref(t, domain.NewTenantID, dispositionTenant), 10)
	if err != nil {
		t.Fatalf("按实物上列：%v", err)
	}
	byUnit := map[string]ports.IdentityReviewUnitRow{}
	for _, unit := range units {
		byUnit[unit.Unit] = unit
	}
	if _, listed := byUnit["SYN-UNIT-NRI06-SOLO"]; listed || len(units) != 2 {
		t.Fatalf("上列 %d 件（%v）, want A 与 B——从没待识别过的实物不在其中", len(units), byUnit)
	}
	a := byUnit["SYN-UNIT-NRI06-A"]
	var aHeads []string
	for _, head := range a.Heads {
		aHeads = append(aHeads, head.JudgmentVersion)
	}
	slices.Sort(aHeads)
	if !slices.Equal(aHeads, []string{"SYN-V-A3", "SYN-V-A9"}) {
		t.Fatalf("A 的链头 = %v, want 处置版与另一条链", aHeads)
	}
	for _, head := range a.Heads {
		if head.JudgmentVersion == "SYN-V-A3" && (!head.Disposition.Recorded() || head.Disposition.Unit.String() != "SYN-UNIT-NRI06-B" || head.Kind != "PENDING_IDENTIFICATION") {
			t.Fatalf("A 的处置版 = %+v", head)
		}
		if head.JudgmentVersion == "SYN-V-A9" && head.Disposition.Recorded() {
			t.Fatal("A 的另一条链带了处置文档")
		}
	}
	b := byUnit["SYN-UNIT-NRI06-B"]
	if len(b.Heads) != 1 || b.Heads[0].Kind != "INTAKE_FORMED" || b.Heads[0].Association != "SYN-PARCEL-NRI06-X" || !b.Heads[0].Disposition.Recorded() {
		t.Fatalf("B 的链头 = %+v, want 指定一件的处置版", b.Heads)
	}
	if _, err := catalogue.ListIdentityReviewUnits(ctx, ref(t, domain.NewTenantID, dispositionTenant), 0); err == nil {
		t.Fatal("页大小 0 没拒")
	}
}

// Covers: 处置文档坏了读不回——库面只守对象形状，形状版本由领域读回门守；一行坏文档让读回响亮，而不是变成一次看起来合法的处置。
func TestABrokenDispositionDocumentIsLoud(t *testing.T) {
	repository, transactor, pool := newReceptionsWithPool(t)
	ctx := t.Context()
	early, conflict, late := oneMarkChains(t)
	designated, _ := dispositionOfOneMark(t)
	saveReceptions(t, transactor, ctx, repository, early, conflict, late, designated)
	if _, err := pool.Exec(ctx, `UPDATE node_operations.reception SET disposition_document = '{"shape":"NID-0"}'::jsonb WHERE tenant_id = $1 AND source_id = $2`,
		dispositionTenant, designated.Key.SourceID); err != nil {
		t.Fatalf("改坏文档：%v", err)
	}
	if _, _, err := repository.FindByKey(ctx, designated.Key); !errors.Is(err, domain.ErrInvalidIdentityDisposition) {
		t.Fatalf("坏文档读回：err = %v, want ErrInvalidIdentityDisposition", err)
	}
}
