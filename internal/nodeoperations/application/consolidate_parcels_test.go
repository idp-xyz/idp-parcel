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

var consolidationAt = time.Date(2026, 8, 13, 23, 0, 0, 0, time.UTC)

type consolidationStoreDouble struct {
	byID map[string]*domain.ConsolidationUnit
}

func (double *consolidationStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.ConsolidationUnitID,
) (*domain.ConsolidationUnit, bool, error) {
	unit, found := double.byID[id.String()]
	return unit, found, nil
}

func (double *consolidationStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	unit *domain.ConsolidationUnit,
) (ports.ConsolidationSaveOutcome, error) {
	if _, exists := double.byID[unit.ID().String()]; exists {
		return ports.ConsolidationAlreadyRecorded, nil
	}
	double.byID[unit.ID().String()] = unit
	return ports.ConsolidationSaved, nil
}

func (double *consolidationStoreDouble) Update(
	_ context.Context,
	_ domain.TenantID,
	unit *domain.ConsolidationUnit,
) error {
	double.byID[unit.ID().String()] = unit
	return nil
}

// containmentIndexDouble 按替身库里全部未关闭单元的当前成员作答——与真实实现同一
// 语义：封装态成员仍被包含，关闭态不算。
type containmentIndexDouble struct {
	store *consolidationStoreDouble
}

func (double *containmentIndexDouble) CurrentParent(
	_ context.Context,
	_ domain.TenantID,
	member domain.HandlingUnitID,
) (domain.ConsolidationUnitID, bool, error) {
	for _, unit := range double.store.byID {
		if unit.Closed() {
			continue
		}
		for _, contained := range unit.Members() {
			if contained == member {
				return unit.ID(), true, nil
			}
		}
	}
	return domain.ConsolidationUnitID{}, false, nil
}

type snapshotDownstreamDouble struct {
	intents []ports.SealedSnapshotHandoffIntent
	err     error
}

func (double *snapshotDownstreamDouble) HandOffSnapshot(
	_ context.Context,
	intent ports.SealedSnapshotHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

// consolidationFactStoreDouble 是来源事实登记的替身：按（租户+来源身份）幂等，同键
// 第二份答`已有记录`而不覆盖先到者——与真实仓储的 ON CONFLICT DO NOTHING 同一语义
// （AT-NO-043）。
type consolidationFactStoreDouble struct {
	byKey map[ports.ConsolidationFactKey]ports.ConsolidationFactRecord
	saved int
}

func newConsolidationFactStore() *consolidationFactStoreDouble {
	return &consolidationFactStoreDouble{byKey: map[ports.ConsolidationFactKey]ports.ConsolidationFactRecord{}}
}

func (double *consolidationFactStoreDouble) FindByKey(
	_ context.Context,
	key ports.ConsolidationFactKey,
) (ports.ConsolidationFactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *consolidationFactStoreDouble) Save(
	_ context.Context,
	record ports.ConsolidationFactRecord,
) (ports.ConsolidationFactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.ConsolidationFactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.ConsolidationFactSaved, nil
}

type consolidateFixture struct {
	handler    *application.ConsolidateParcelsHandler
	store      *consolidationStoreDouble
	facts      *consolidationFactStoreDouble
	downstream *snapshotDownstreamDouble
}

func newConsolidateFixture(t *testing.T) *consolidateFixture {
	t.Helper()
	store := &consolidationStoreDouble{byID: map[string]*domain.ConsolidationUnit{}}
	fixture := &consolidateFixture{
		store:      store,
		facts:      newConsolidationFactStore(),
		downstream: &snapshotDownstreamDouble{},
	}
	fixture.handler = application.NewConsolidateParcelsHandler(application.ConsolidateParcelsDeps{
		Store:       store,
		Facts:       fixture.facts,
		Containment: &containmentIndexDouble{store: store},
		Downstream:  fixture.downstream,
		Clock:       fixedClock{at: consolidationAt},
	})
	return fixture
}

// workSource 造一份完整的来源表达。业务时间由调用方给，六口各带各的现场时刻——这正是
// Clock.Now() 让位之后现场该有的样子。
//
// 每个调用点都换一个来源身份：同一身份重复报同一内容会被受理闸判成`已有结果`，那是
// 另一条性质（AT-NO-043 的重放向），本文件下面几例要看的是领域三相与跨单元核对。
func workSource(t *testing.T, sourceID string, at time.Time) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		mustValue(t, domain.NewPerformingPartyReference, "packer-1"),
		mustValue(t, domain.NewExecutionEvidenceReference, "WORK-EVIDENCE/"+sourceID),
		at,
	)
	if err != nil {
		t.Fatalf("work fact source %q: %v", sourceID, err)
	}
	return source
}

func consolidationTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return tenant
}

func unitID(t *testing.T, raw string) domain.ConsolidationUnitID {
	t.Helper()
	id, err := domain.NewConsolidationUnitID(raw)
	if err != nil {
		t.Fatalf("unit id %q: %v", raw, err)
	}
	return id
}

func handlingUnit(t *testing.T, raw string) domain.HandlingUnitID {
	t.Helper()
	member, err := domain.NewHandlingUnitID(raw)
	if err != nil {
		t.Fatalf("handling unit %q: %v", raw, err)
	}
	return member
}

func openUnit(t *testing.T, fixture *consolidateFixture, id string) {
	t.Helper()
	asset, err := domain.NewCarrierAssetReference("cage-" + id)
	if err != nil {
		t.Fatalf("asset: %v", err)
	}
	opened, err := fixture.handler.Open(context.Background(), application.OpenUnitCommand{
		TenantID: consolidationTenant(t),
		Unit:     unitID(t, id),
		Asset:    asset,
		Source:   workSource(t, "src-open-"+id, consolidationAt),
	})
	if err != nil {
		t.Fatalf("open %s: %v", id, err)
	}
	if opened.Outcome() != application.UnitOpened {
		t.Fatalf("open outcome = %q", opened.Outcome())
	}
}

// Covers: NO CONTEXT「同一时点最多一个直接物理父级」的编排面（领域注释点名跨单元
// 唯一性需要仓储视野、由收纳编排在加入前核对）——实物已在别的未关闭单元里即业务
// 负向带对方标识（恢复动作是先从那里移出；`AT-NO-035`「同一包裹并发移入两个集运
// 单元→只允许一个当前直接父级，另一操作冲突」）；已在本单元是幂等重放；从原单元移出后
// 方可加入新单元。
func TestOneDirectParentIsEnforcedAcrossUnits(t *testing.T) {
	fixture := newConsolidateFixture(t)
	tenant := consolidationTenant(t)
	openUnit(t, fixture, "unit-1")
	openUnit(t, fixture, "unit-2")

	added, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-add-1", consolidationAt),
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if added.Outcome() != application.MemberAdded {
		t.Fatalf("outcome = %q", added.Outcome())
	}

	// 另一台设备把同一次移入又报了一遍：来源身份不同，因此不是重放而是一次新的作业
	// 报告，跨单元核对照样把它判成`已在本单元`。
	replay, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-add-1-again", consolidationAt.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("replay add: %v", err)
	}
	if replay.Outcome() != application.MemberAlreadyContained {
		t.Fatalf("replay = %q", replay.Outcome())
	}

	elsewhere, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-2"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-add-elsewhere", consolidationAt.Add(2*time.Minute)),
	})
	if err != nil {
		t.Fatalf("elsewhere add: %v", err)
	}
	if elsewhere.Outcome() != application.MemberElsewhereContained ||
		elsewhere.Elsewhere() != unitID(t, "unit-1") {
		t.Fatalf("outcome = %q elsewhere = %v", elsewhere.Outcome(), elsewhere.Elsewhere())
	}

	if _, err := fixture.handler.RemoveMember(context.Background(), application.RemoveMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-remove-1", consolidationAt.Add(3*time.Minute)),
	}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	moved, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-2"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-add-moved", consolidationAt.Add(4*time.Minute)),
	})
	if err != nil {
		t.Fatalf("moved add: %v", err)
	}
	if moved.Outcome() != application.MemberAdded {
		t.Fatalf("moved = %q; 移出后加入新单元被拦了", moved.Outcome())
	}
}

// Covers: NO CONTEXT「封装成员快照」与显式终局——封装冻结快照并随意图交装载交接
// （投递失败封装不翻留续办）；封装态改成员被领域拦（编排透出未受理）；开封回开放态
// 历史快照保留（`AT-NO-036`「封装后需要移出一个成员→先授权开封，再移出并重新封装；
// 两版快照和封签均保留」）；关闭后单元不再参与包含核对（载具复用是新实例的事，
// `AT-NO-040`「已关闭集运单元或同一载具再次使用→原实例不重开；新使用创建新实例」）。
func TestSealingFreezesTheSnapshotAndClosureIsFinal(t *testing.T) {
	fixture := newConsolidateFixture(t)
	tenant := consolidationTenant(t)
	openUnit(t, fixture, "unit-1")
	if _, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-add-seal-case", consolidationAt),
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	seal, err := domain.NewSealReference("seal-1")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	basis, err := domain.NewWorkBasisReference("WORK-ORDER/consolidate-7")
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	fixture.downstream.err = errors.New("downstream unreachable")
	sealed, err := fixture.handler.Seal(context.Background(), application.SealUnitCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Seal:     seal,
		Basis:    basis,
		Source:   workSource(t, "src-seal-1", consolidationAt.Add(time.Minute)),
	})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed.Outcome() != application.UnitSealedRecorded || sealed.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q; 投递失败封装不翻但要留续办", sealed.Outcome(), sealed.HandoffReference())
	}

	frozen, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-2"),
		Source:   workSource(t, "src-add-frozen", consolidationAt.Add(2*time.Minute)),
	})
	if err != nil {
		t.Fatalf("frozen add: %v", err)
	}
	if frozen.Outcome() != application.ConsolidationNotAccepted {
		t.Fatalf("frozen = %q; 封装后改成员必须先开封", frozen.Outcome())
	}

	if _, err := fixture.handler.Unseal(context.Background(), application.UnsealUnitCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Basis:    basis,
		Source:   workSource(t, "src-unseal-1", consolidationAt.Add(3*time.Minute)),
	}); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if _, err := fixture.handler.RemoveMember(context.Background(), application.RemoveMemberCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Member:   handlingUnit(t, "hu-1"),
		Source:   workSource(t, "src-remove-after-unseal", consolidationAt.Add(4*time.Minute)),
	}); err != nil {
		t.Fatalf("remove after unseal: %v", err)
	}

	closed, err := fixture.handler.Close(context.Background(), application.CloseUnitCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Source:   workSource(t, "src-close-1", consolidationAt.Add(5*time.Minute)),
	})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.Outcome() != application.UnitClosedRecorded {
		t.Fatalf("close = %q", closed.Outcome())
	}
	unit, _ := closed.Unit()
	if len(unit.Snapshots()) != 1 {
		t.Fatalf("snapshots = %d; 历史快照被开封或关闭动掉了", len(unit.Snapshots()))
	}

	// 换一个来源身份再关一次：这不是重放（重放答`已有结果`），是另一次真的想关已关
	// 实例的作业，必须落进未受理——终局不重演。
	again, err := fixture.handler.Close(context.Background(), application.CloseUnitCommand{
		TenantID: tenant,
		Unit:     unitID(t, "unit-1"),
		Source:   workSource(t, "src-close-again", consolidationAt.Add(6*time.Minute)),
	})
	if err != nil {
		t.Fatalf("close again: %v", err)
	}
	if again.Outcome() != application.ConsolidationNotAccepted {
		t.Fatalf("again = %q; 终局不重演", again.Outcome())
	}
}
