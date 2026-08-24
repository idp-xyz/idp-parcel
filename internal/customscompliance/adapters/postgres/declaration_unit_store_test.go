package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 申报单元本体的写口用例（ADR-0073 决定一/二）：往返穿读口、不可覆盖（案件维成立
// 即定）、无环境事务拒、悬空案件撞库上外键防线。案件行先经真写口落库——外键引用的
// 是 customs_case_id_unique，夹具不绕道。

var unitFormedAt = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

type unitStoreFixture struct {
	units *adapter.DeclarationUnits
	cases *adapter.CustomsCases
	view  *viewFixture
}

func newUnitStoreFixture(t *testing.T) *unitStoreFixture {
	t.Helper()
	view := newViewFixture(t)
	units, err := adapter.NewDeclarationUnits(view.db)
	if err != nil {
		t.Fatalf("构造单元库：%v", err)
	}
	cases, err := adapter.NewCustomsCases(view.db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	return &unitStoreFixture{units: units, cases: cases, view: view}
}

// establishCase 走真写口把案件落库，交回其铸造标识。
func (fixture *unitStoreFixture) establishCase(t *testing.T, tenant, caseID string) domain.CustomsCaseID {
	t.Helper()
	key := caseKey(t, tenant)
	err := fixture.view.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, key, establishedCase(t, key, caseID, nil))
		return err
	})
	if err != nil {
		t.Fatalf("落库案件：%v", err)
	}
	return crgValue(t, domain.NewCustomsCaseID, caseID)
}

func formedUnit(t *testing.T, unitID string, customsCase domain.CustomsCaseID, members ...string) domain.DeclarationUnit {
	t.Helper()
	references := make([]domain.DeclaredParcelReference, 0, len(members))
	for _, member := range members {
		references = append(references, crgValue(t, domain.NewDeclaredParcelReference, member))
	}
	unit, err := domain.FormDeclarationUnit(
		crgValue(t, domain.NewDeclarationUnitID, unitID),
		customsCase,
		crgValue(t, domain.NewCustomsProcedureReference, "IMPORT_STANDARD"),
		references,
	)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	return unit
}

func (fixture *unitStoreFixture) saveUnit(
	t *testing.T,
	tenant domain.TenantID,
	unit domain.DeclarationUnit,
) (ports.DeclarationUnitSaveOutcome, error) {
	t.Helper()
	var outcome ports.DeclarationUnitSaveOutcome
	err := fixture.view.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = fixture.units.Save(txCtx, tenant, unit, unitFormedAt)
		return saveErr
	})
	return outcome, err
}

// 往返：成员乱序进库、按字典序读回——形成顺序不构成不同的组成。
func TestASavedDeclarationUnitIsReadBackWithItsCase(t *testing.T) {
	fixture := newUnitStoreFixture(t)
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	customsCase := fixture.establishCase(t, "tenant-a", "case-1")

	outcome, err := fixture.saveUnit(t, tenant, formedUnit(t, "unit-1", customsCase, "parcel-2", "parcel-1"))
	if err != nil || outcome != ports.DeclarationUnitSaved {
		t.Fatalf("落库单元：err=%v outcome=%v", err, outcome)
	}

	loaded, found, err := fixture.units.FindByID(t.Context(), tenant,
		crgValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil || !found {
		t.Fatalf("单元没读回：err=%v found=%v", err, found)
	}
	if loaded.Case() != customsCase {
		t.Fatalf("案件维走样：%q", loaded.Case())
	}
	members := loaded.Members()
	if len(members) != 2 || members[0].String() != "parcel-1" || members[1].String() != "parcel-2" {
		t.Fatalf("组成走样：%v", members)
	}
}

// 案件维成立即定：同键再登另一案件改不动已在册那一行——换案件即替代单元，本表无
// 更新路径。
func TestAUnitCannotBeRebound(t *testing.T) {
	fixture := newUnitStoreFixture(t)
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	first := fixture.establishCase(t, "tenant-a", "case-1")
	second := crgValue(t, domain.NewCustomsCaseID, "case-2")
	secondKey := caseKey(t, "tenant-a")
	secondKey.Obligation = crgValue(t, domain.NewObligationScopeReference, "obligation/partial")
	if err := fixture.view.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := fixture.cases.Save(txCtx, secondKey, establishedCase(t, secondKey, "case-2", nil))
		return err
	}); err != nil {
		t.Fatalf("落库第二个案件：%v", err)
	}

	if outcome, err := fixture.saveUnit(t, tenant, formedUnit(t, "unit-1", first, "parcel-1")); err != nil ||
		outcome != ports.DeclarationUnitSaved {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	outcome, err := fixture.saveUnit(t, tenant, formedUnit(t, "unit-1", second, "parcel-1"))
	if err != nil || outcome != ports.DeclarationUnitAlreadyRecorded {
		t.Fatalf("再登：err=%v outcome=%v，要`已有记录`", err, outcome)
	}

	loaded, found, err := fixture.units.FindByID(t.Context(), tenant,
		crgValue(t, domain.NewDeclarationUnitID, "unit-1"))
	if err != nil || !found {
		t.Fatalf("读回：err=%v found=%v", err, found)
	}
	if loaded.Case() != first {
		t.Fatalf("案件维被顶替成 %q", loaded.Case())
	}
}

func TestDeclarationUnitStoreRequiresAmbientTransaction(t *testing.T) {
	fixture := newUnitStoreFixture(t)
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	customsCase := fixture.establishCase(t, "tenant-a", "case-1")

	if _, err := fixture.units.Save(t.Context(), tenant,
		formedUnit(t, "unit-1", customsCase, "parcel-1"), unitFormedAt); err == nil {
		t.Fatalf("无环境事务的写入要拒")
	}
}

// 悬空案件撞库上外键：用例已按标识反查核存在，这里证同一判断的库内防线真的在。
func TestAUnitReferencingAMissingCaseHitsTheForeignKey(t *testing.T) {
	fixture := newUnitStoreFixture(t)
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")

	dangling := crgValue(t, domain.NewCustomsCaseID, "case-never-established")
	if _, err := fixture.saveUnit(t, tenant, formedUnit(t, "unit-1", dangling, "parcel-1")); err == nil {
		t.Fatalf("悬空案件引用要撞外键")
	}
}

// 案件反查读口（ADR-0073 决定五）：按铸造标识取回同一案件；未登记如实 found=false。
func TestACustomsCaseIsFoundByItsMintedID(t *testing.T) {
	fixture := newUnitStoreFixture(t)
	tenant := crgValue(t, domain.NewTenantID, "tenant-a")
	id := fixture.establishCase(t, "tenant-a", "case-1")

	loaded, found, err := fixture.cases.FindByID(t.Context(), tenant, id)
	if err != nil || !found {
		t.Fatalf("案件没按标识读回：err=%v found=%v", err, found)
	}
	if loaded.ID() != id || loaded.Jurisdiction().String() != "jurisdiction/US" {
		t.Fatalf("反查走样：id=%q jurisdiction=%q", loaded.ID(), loaded.Jurisdiction())
	}

	if _, found, err := fixture.cases.FindByID(t.Context(), tenant,
		crgValue(t, domain.NewCustomsCaseID, "case-unknown")); err != nil || found {
		t.Fatalf("未登记标识要如实 found=false：err=%v found=%v", err, found)
	}
}
