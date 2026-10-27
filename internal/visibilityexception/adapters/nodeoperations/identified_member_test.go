package nodeoperations_test

import (
	"context"
	"errors"
	"testing"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证补派生问节点作业的那两口只翻译（ADR-0299）：收寄形成信封指的那份收寄认出了哪件包裹，以及这个成员曾被装进哪几件单元。收寄身份与
// 成员口径都由节点作业答，本上下文不重判。

type holdersDouble struct {
	units map[string][]string
	err   error
	asked []string
}

func (double *holdersDouble) ReadUnitsEverHoldingMember(
	_ context.Context, _ nodomain.TenantID, member string,
) ([]nodomain.ConsolidationUnitID, error) {
	double.asked = append(double.asked, member)
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

func newIdentifiedMemberView(t *testing.T, receptions *receptionDouble, holders *holdersDouble) *adapter.IdentifiedMemberView {
	t.Helper()
	view, err := adapter.NewIdentifiedMemberView(receptions, holders)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return view
}

// Covers: AT-VE-218 适配器半边——已形成且带版本化包裹关联的收寄认出了包裹：交回作业实物标识原文与包裹引用。
func TestAFormedIntakeWithAnAssociationIdentifiesTheParcel(t *testing.T) {
	receptions := &receptionDouble{record: formedReception(t), found: true}
	view := newIdentifiedMemberView(t, receptions, &holdersDouble{})

	identity, err := view.FormedIntakeIdentity(t.Context(), value(t, vedomain.NewTenantID, "tenant-1"), "source-1")
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

// Covers: AT-VE-218 适配器半边——没认出包裹的各格不是错：待识别、未形成、形成了却没有关联，都交回「没认出」，没有可补的对象。
func TestAnIntakeThatIdentifiesNoParcelIsNotAnError(t *testing.T) {
	pending := formedReception(t)
	pending.Kind = noports.RecordPendingIdentification
	notFormed := formedReception(t)
	notFormed.Kind = noports.RecordIntakeNotFormed
	unassociated := formedReception(t)
	spec, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("形成收寄：%v", err)
	}
	unassociated.Intake = spec

	for name, record := range map[string]noports.ReceptionRecord{"待识别": pending, "未形成": notFormed, "形成了没有关联": unassociated} {
		t.Run(name, func(t *testing.T) {
			view := newIdentifiedMemberView(t, &receptionDouble{record: record, found: true}, &holdersDouble{})
			identity, err := view.FormedIntakeIdentity(t.Context(), value(t, vedomain.NewTenantID, "tenant-1"), "source-1")
			if err != nil || identity.Identified {
				t.Fatalf("身份 = %+v err = %v，want 没认出、不是错", identity, err)
			}
		})
	}
}

// Covers: AT-VE-221 适配器半边——读不回（册读不动、按键没有）是普通错误，续办；记录与键不符、租户对不上是册上的确定性问题，包住
// ErrFormedIntakeUntranslatable。
func TestFormedIntakeReadFailuresAreSplitIntoUnreadableAndInconsistent(t *testing.T) {
	tenant := value(t, vedomain.NewTenantID, "tenant-1")
	for name, test := range map[string]struct {
		receptions    *receptionDouble
		wantUntransl  bool
		wantSomeError bool
	}{
		"册读不动": {receptions: &receptionDouble{err: errors.New("store unavailable")}, wantSomeError: true},
		"按键没有": {receptions: &receptionDouble{}, wantSomeError: true},
		"记录指着别的键": {receptions: func() *receptionDouble {
			record := formedReception(t)
			record.Key.SourceID = "other-source"
			return &receptionDouble{record: record, found: true}
		}(), wantUntransl: true, wantSomeError: true},
		"收寄属于别的租户": {receptions: func() *receptionDouble {
			record := formedReception(t)
			record.Key.TenantID = value(t, nodomain.NewTenantID, "tenant-2")
			return &receptionDouble{record: record, found: true}
		}(), wantUntransl: true, wantSomeError: true},
	} {
		t.Run(name, func(t *testing.T) {
			view := newIdentifiedMemberView(t, test.receptions, &holdersDouble{})
			_, err := view.FormedIntakeIdentity(t.Context(), tenant, "source-1")
			if (err != nil) != test.wantSomeError {
				t.Fatalf("err = %v", err)
			}
			if got := errors.Is(err, veports.ErrFormedIntakeUntranslatable); got != test.wantUntransl {
				t.Fatalf("确定性 = %v，want %v（err = %v）", got, test.wantUntransl, err)
			}
		})
	}
}

// Covers: AT-VE-218 适配器半边——反向一问只翻译：单元标识原串交回、次序照节点作业；没被装过即空，不是错；读不动原样交回为普通错误；
// 节点作业答了一个空白单元标识，是册上的确定性问题。
func TestUnitsEverHoldingAMemberAreTranslatedAndFailuresAreSplit(t *testing.T) {
	tenant := value(t, vedomain.NewTenantID, "tenant-1")
	holders := &holdersDouble{units: map[string][]string{"unit-1": {"bag-1", "bag-2"}}}
	view := newIdentifiedMemberView(t, &receptionDouble{}, holders)

	got, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1")
	if err != nil || len(got) != 2 || got[0] != "bag-1" || got[1] != "bag-2" {
		t.Fatalf("曾装它的单元 = %v err = %v，want bag-1、bag-2", got, err)
	}
	if got, err := view.UnitsEverHolding(t.Context(), tenant, "unit-never"); err != nil || len(got) != 0 {
		t.Fatalf("没被装过 = %v err = %v，want 空、不是错", got, err)
	}

	view = newIdentifiedMemberView(t, &receptionDouble{}, &holdersDouble{err: errors.New("book down")})
	if _, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1"); err == nil || errors.Is(err, veports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("读不动 err = %v，want 普通错误", err)
	}

	view = newIdentifiedMemberView(t, &receptionDouble{}, &holdersDouble{units: map[string][]string{"unit-1": {""}}})
	if _, err := view.UnitsEverHolding(t.Context(), tenant, "unit-1"); !errors.Is(err, veports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("空白单元标识 err = %v，want ErrConsolidationMembershipUntranslatable", err)
	}
}

func TestTheIdentifiedMemberViewRefusesMissingDependencies(t *testing.T) {
	if _, err := adapter.NewIdentifiedMemberView(nil, &holdersDouble{}); err == nil {
		t.Fatal("缺收寄读口构造成功了")
	}
	if _, err := adapter.NewIdentifiedMemberView(&receptionDouble{}, nil); err == nil {
		t.Fatal("缺反向读口构造成功了")
	}
}
