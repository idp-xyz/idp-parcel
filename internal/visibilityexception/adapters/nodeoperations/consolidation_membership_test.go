package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var errMembershipBookDown = errors.New("册不可用")

type membershipsAtDouble struct {
	answers map[string]noports.ConsolidationMembershipAt
	err     error
	asked   []time.Time
}

func (double *membershipsAtDouble) ReadConsolidationMembershipAt(
	_ context.Context, _ nodomain.TenantID, object string, at time.Time,
) (noports.ConsolidationMembershipAt, error) {
	double.asked = append(double.asked, at)
	if double.err != nil {
		return noports.ConsolidationMembershipAt{}, double.err
	}
	return double.answers[object], nil
}

type intakeIdentitiesDouble struct {
	observations map[string][]nodomain.IntakeIdentityObservation
	down         map[string]bool
}

func (double intakeIdentitiesDouble) FindIntakeIdentities(
	_ context.Context, _ nodomain.TenantID, unit nodomain.HandlingUnitID,
) ([]nodomain.IntakeIdentityObservation, error) {
	if double.down[unit.String()] {
		return nil, errMembershipBookDown
	}
	return double.observations[unit.String()], nil
}

func identityObservation(t *testing.T, source, version, predecessor, parcel string) nodomain.IntakeIdentityObservation {
	t.Helper()
	observed := nodomain.IntakeIdentityObservation{
		SourceID:   source,
		Version:    value(t, nodomain.NewIntakeResultVersion, version),
		ReceivedAt: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC),
	}
	if predecessor != "" {
		observed.Predecessor = value(t, nodomain.NewIntakeResultVersion, predecessor)
	}
	if parcel != "" {
		observed.Association = value(t, nodomain.NewParcelAssociationReference, parcel)
	}
	return observed
}

func membershipQuery(t *testing.T, object string, at time.Time) veports.ConsolidationMembershipQuery {
	t.Helper()
	return veports.ConsolidationMembershipQuery{TenantID: value(t, vedomain.NewTenantID, "tenant-1"), Object: object, At: at}
}

func handlingMember(t *testing.T, raw string, nested bool) noports.ConsolidationMemberAt {
	t.Helper()
	return noports.ConsolidationMemberAt{Member: value(t, nodomain.NewHandlingUnitID, raw), NestedUnit: nested}
}

// Covers: AT-VE-196 适配器半边——节点作业的答复只翻译：嵌套成员交回那件单元的引用；其余成员各问一次身份，由节点作业的身份判断
// 认——成立即那件包裹，待识别或没有收寄记录即身份不成立。时点原样交过去。不是单元的对象答不是单元。
func TestMembersAreTranslatedThroughTheNodeIdentityJudgement(t *testing.T) {
	memberships := &membershipsAtDouble{answers: map[string]noports.ConsolidationMembershipAt{
		"bag-1": {Unit: true, Members: []noports.ConsolidationMemberAt{
			handlingMember(t, "bag-inner", true), handlingMember(t, "hu-known", false),
			handlingMember(t, "hu-pending", false), handlingMember(t, "hu-unreceived", false),
		}},
	}}
	identities := intakeIdentitiesDouble{observations: map[string][]nodomain.IntakeIdentityObservation{
		"hu-known":   {identityObservation(t, "scan-1", "v1", "", "PCL-1")},
		"hu-pending": {identityObservation(t, "scan-2", "v2", "", "")},
	}}
	view, err := adapter.NewConsolidationMembershipView(memberships, identities)
	if err != nil {
		t.Fatalf("装适配器：%v", err)
	}
	at := time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)

	answer, err := view.MembersAt(t.Context(), membershipQuery(t, "bag-1", at))
	if err != nil {
		t.Fatalf("问成员：%v", err)
	}
	want := []veports.ConsolidationMember{
		{Kind: veports.ConsolidationMemberUnit, Handling: "bag-inner", Object: "bag-inner"},
		{Kind: veports.ConsolidationMemberParcel, Handling: "hu-known", Object: "PCL-1"},
		{Kind: veports.ConsolidationMemberIdentityUnresolved, Handling: "hu-pending"},
		{Kind: veports.ConsolidationMemberIdentityUnresolved, Handling: "hu-unreceived"},
	}
	if !answer.Unit || len(answer.Members) != len(want) {
		t.Fatalf("答复 = %+v", answer)
	}
	for index, expected := range want {
		if answer.Members[index] != expected {
			t.Fatalf("第 %d 件 = %+v，要 %+v", index, answer.Members[index], expected)
		}
	}
	if !memberships.asked[0].Equal(at) {
		t.Fatalf("时点 = %s，要 %s", memberships.asked[0], at)
	}
	if answer, err := view.MembersAt(t.Context(), membershipQuery(t, "PCL-9", at)); err != nil || answer.Unit || len(answer.Members) != 0 {
		t.Fatalf("不是单元的对象 = %+v / %v", answer, err)
	}
}

// Covers: AT-VE-198 适配器半边——成员关系或收寄身份那一问读不动作为调用失败交回，不读成「不是单元」或「身份不成立」；身份链判不下来
// 是节点作业册上的确定性问题，包住译不回来交回，重问还是同一个结果。缺哪一口都装不起来。
func TestMembershipReadFailuresAreNotReadAsAnAnswer(t *testing.T) {
	bag := map[string]noports.ConsolidationMembershipAt{
		"bag-1": {Unit: true, Members: []noports.ConsolidationMemberAt{handlingMember(t, "hu-1", false)}},
	}
	at := time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)
	for name, test := range map[string]struct {
		memberships    *membershipsAtDouble
		identities     intakeIdentitiesDouble
		untranslatable bool
	}{
		"成员关系读不动": {memberships: &membershipsAtDouble{err: errMembershipBookDown}},
		"收寄身份读不动": {
			memberships: &membershipsAtDouble{answers: bag},
			identities:  intakeIdentitiesDouble{down: map[string]bool{"hu-1": true}},
		},
		"身份链判不下来": {
			memberships: &membershipsAtDouble{answers: bag},
			// 两版互相接续：哪一版是链头说不准。
			identities: intakeIdentitiesDouble{observations: map[string][]nodomain.IntakeIdentityObservation{
				"hu-1": {identityObservation(t, "scan-3", "v3", "v4", "PCL-3"), identityObservation(t, "scan-4", "v4", "v3", "PCL-3")},
			}},
			untranslatable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			view, err := adapter.NewConsolidationMembershipView(test.memberships, test.identities)
			if err != nil {
				t.Fatalf("装适配器：%v", err)
			}
			_, err = view.MembersAt(t.Context(), membershipQuery(t, "bag-1", at))
			if err == nil {
				t.Fatal("读不动却交回了答复")
			}
			if got := errors.Is(err, veports.ErrConsolidationMembershipUntranslatable); got != test.untranslatable {
				t.Fatalf("err = %v，译不回来 = %v，want %v", err, got, test.untranslatable)
			}
			if !test.untranslatable && !errors.Is(err, errMembershipBookDown) {
				t.Fatalf("err = %v，要原样包住调用失败", err)
			}
		})
	}
	if _, err := adapter.NewConsolidationMembershipView(nil, intakeIdentitiesDouble{}); err == nil {
		t.Fatal("缺成员关系那一口也装得起来")
	}
	if _, err := adapter.NewConsolidationMembershipView(&membershipsAtDouble{}, nil); err == nil {
		t.Fatal("缺收寄身份那一口也装得起来")
	}
}
