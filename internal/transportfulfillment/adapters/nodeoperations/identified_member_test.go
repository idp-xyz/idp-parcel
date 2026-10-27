package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfnode "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证成员补问问节点作业的那两口只翻译（ADR-0331）：收寄形成信封指的那份收寄认出了哪件包裹，以及这个成员曾被装进哪几件单元。收寄身份
// 与成员口径都由节点作业答，本上下文不重判。

type receptionsDouble struct {
	record noports.ReceptionRecord
	found  bool
	err    error
	last   noports.ReceptionKey
}

func (double *receptionsDouble) FindByKey(_ context.Context, key noports.ReceptionKey) (noports.ReceptionRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return noports.ReceptionRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type holdersDouble struct {
	units map[string][]string
	err   error
}

func (double *holdersDouble) ReadUnitsEverHoldingMember(
	_ context.Context,
	_ nodomain.TenantID,
	member string,
) ([]nodomain.ConsolidationUnitID, error) {
	if double.err != nil {
		return nil, double.err
	}
	var units []nodomain.ConsolidationUnitID
	for _, raw := range double.units[member] {
		if raw == "" {
			units = append(units, nodomain.ConsolidationUnitID{})
			continue
		}
		unit, err := nodomain.NewConsolidationUnitID(raw)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

var intakeReceivedAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func intakeOf(t *testing.T, association string) nodomain.NodeIntake {
	t.Helper()
	spec := nodomain.NodeIntakeSpec{
		TenantID:    must(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        must(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        must(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: must(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    must(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     must(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  intakeReceivedAt,
	}
	if association != "" {
		spec.Association = must(t, nodomain.NewParcelAssociationReference, association)
	}
	intake, err := nodomain.FormNodeIntake(spec)
	if err != nil {
		t.Fatalf("形成收寄：%v", err)
	}
	return intake
}

func formedReception(t *testing.T, association string) noports.ReceptionRecord {
	t.Helper()
	return noports.ReceptionRecord{
		Key:        noports.ReceptionKey{TenantID: must(t, nodomain.NewTenantID, "tenant-1"), SourceID: "source-1"},
		Kind:       noports.RecordIntakeFormed,
		Intake:     intakeOf(t, association),
		RecordedAt: intakeReceivedAt.Add(time.Minute),
	}
}

func identifiedView(t *testing.T, receptions *receptionsDouble, holders *holdersDouble) *tfnode.IdentifiedMemberView {
	t.Helper()
	view, err := tfnode.NewIdentifiedMemberView(receptions, holders)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return view
}

// Covers: AT-TF-364 适配器半边——已形成且带版本化包裹关联的收寄认出了包裹：交回作业实物标识原文与载运对象引用，按信封键回读。
func TestAFormedIntakeWithAnAssociationIdentifiesTheParcel(t *testing.T) {
	receptions := &receptionsDouble{record: formedReception(t, "parcel-1"), found: true}
	view := identifiedView(t, receptions, &holdersDouble{})

	identity, err := view.FormedIntakeIdentity(t.Context(), must(t, tfdomain.NewTenantID, "tenant-1"), "source-1")
	if err != nil {
		t.Fatalf("FormedIntakeIdentity: %v", err)
	}
	if !identity.Identified || identity.Handling != "unit-1" || identity.Parcel.String() != "parcel-1" {
		t.Fatalf("身份 = %+v，want 认出 unit-1 → parcel-1", identity)
	}
	if receptions.last.SourceID != "source-1" || receptions.last.TenantID.String() != "tenant-1" {
		t.Fatalf("按键回读 = %+v", receptions.last)
	}
}

// Covers: AT-TF-364 适配器半边——没认出包裹的各格不是错：待识别、未形成、形成了却没有关联，都交回「没认出」，没有可补问的对象。
func TestAnIntakeThatIdentifiesNoParcelIsNotAnError(t *testing.T) {
	pending := formedReception(t, "parcel-1")
	pending.Kind = noports.RecordPendingIdentification
	notFormed := formedReception(t, "parcel-1")
	notFormed.Kind = noports.RecordIntakeNotFormed
	for name, record := range map[string]noports.ReceptionRecord{
		"待识别": pending, "未形成": notFormed, "形成了没有关联": formedReception(t, ""),
	} {
		t.Run(name, func(t *testing.T) {
			view := identifiedView(t, &receptionsDouble{record: record, found: true}, &holdersDouble{})
			identity, err := view.FormedIntakeIdentity(t.Context(), must(t, tfdomain.NewTenantID, "tenant-1"), "source-1")
			if err != nil || identity.Identified {
				t.Fatalf("身份 = %+v err = %v，want 没认出、不是错", identity, err)
			}
		})
	}
}

// Covers: AT-TF-366 适配器半边——读不回（册读不动、按键没有）是普通错误，续办；记录与键不符、租户对不上是册上的确定性问题，包住
// ErrFormedIntakeUntranslatable。
func TestFormedIntakeReadFailuresAreSplitIntoUnreadableAndInconsistent(t *testing.T) {
	tenant := must(t, tfdomain.NewTenantID, "tenant-1")
	for name, test := range map[string]struct {
		receptions   *receptionsDouble
		untranslated bool
	}{
		"册读不动": {receptions: &receptionsDouble{err: errors.New("store unavailable")}},
		"按键没有": {receptions: &receptionsDouble{}},
		"记录指着别的键": {receptions: func() *receptionsDouble {
			record := formedReception(t, "parcel-1")
			record.Key.SourceID = "other-source"
			return &receptionsDouble{record: record, found: true}
		}(), untranslated: true},
		"收寄属于别的租户": {receptions: func() *receptionsDouble {
			record := formedReception(t, "parcel-1")
			record.Key.TenantID = must(t, nodomain.NewTenantID, "tenant-2")
			return &receptionsDouble{record: record, found: true}
		}(), untranslated: true},
		"记录的形成格落在封闭集外": {receptions: func() *receptionsDouble {
			record := formedReception(t, "parcel-1")
			record.Kind = noports.ReceptionRecordKind(99)
			return &receptionsDouble{record: record, found: true}
		}(), untranslated: true},
	} {
		t.Run(name, func(t *testing.T) {
			view := identifiedView(t, test.receptions, &holdersDouble{})
			_, err := view.FormedIntakeIdentity(t.Context(), tenant, "source-1")
			if err == nil {
				t.Fatal("读不回却没有错")
			}
			if got := errors.Is(err, tfports.ErrFormedIntakeUntranslatable); got != test.untranslated {
				t.Fatalf("确定性 = %v，want %v（err = %v）", got, test.untranslated, err)
			}
		})
	}
}

// Covers: AT-TF-364 适配器半边——反向一问只翻译：单元标识原串交回、次序照节点作业；没被装过即空，不是错；读不动原样交回为普通错误；节点作业
// 答了一个空白单元标识，是册上的确定性问题。
func TestUnitsEverHoldingAMemberAreTranslatedAndFailuresAreSplit(t *testing.T) {
	tenant := must(t, tfdomain.NewTenantID, "tenant-1")
	holders := &holdersDouble{units: map[string][]string{"unit-1": {"bag-1", "bag-2"}}}
	view := identifiedView(t, &receptionsDouble{}, holders)

	got, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1")
	if err != nil || len(got) != 2 || got[0] != "bag-1" || got[1] != "bag-2" {
		t.Fatalf("曾装它的单元 = %v err = %v，want bag-1、bag-2", got, err)
	}
	if got, err := view.UnitsEverHolding(t.Context(), tenant, "unit-never"); err != nil || len(got) != 0 {
		t.Fatalf("没被装过 = %v err = %v，want 空、不是错", got, err)
	}

	view = identifiedView(t, &receptionsDouble{}, &holdersDouble{err: errors.New("book down")})
	if _, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1"); err == nil || errors.Is(err, tfports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("读不动 err = %v，want 普通错误", err)
	}

	view = identifiedView(t, &receptionsDouble{}, &holdersDouble{units: map[string][]string{"unit-1": {""}}})
	if _, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1"); !errors.Is(err, tfports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("空白单元标识 err = %v，want ErrConsolidationMembershipUntranslatable", err)
	}
}

// Covers: AT-TF-364 适配器半边——构造期拒缺件。
func TestTheIdentifiedMemberViewRefusesMissingDependencies(t *testing.T) {
	if _, err := tfnode.NewIdentifiedMemberView(nil, &holdersDouble{}); err == nil {
		t.Fatal("缺收寄读口构造成功了")
	}
	if _, err := tfnode.NewIdentifiedMemberView(&receptionsDouble{}, nil); err == nil {
		t.Fatal("缺反向读口构造成功了")
	}
}
