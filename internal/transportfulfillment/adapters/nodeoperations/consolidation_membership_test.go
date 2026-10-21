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

var errBookDown = errors.New("册不可用")

type membershipsDouble struct {
	answers map[string]noports.ConsolidationMembershipAt
	err     error
	asked   []time.Time
}

func (double *membershipsDouble) ReadConsolidationMembershipAt(
	_ context.Context,
	_ nodomain.TenantID,
	object string,
	at time.Time,
) (noports.ConsolidationMembershipAt, error) {
	double.asked = append(double.asked, at)
	if double.err != nil {
		return noports.ConsolidationMembershipAt{}, double.err
	}
	return double.answers[object], nil
}

type identitiesDouble struct {
	observations map[string][]nodomain.IntakeIdentityObservation
	down         map[string]bool
}

func (double identitiesDouble) FindIntakeIdentities(
	_ context.Context,
	_ nodomain.TenantID,
	unit nodomain.HandlingUnitID,
) ([]nodomain.IntakeIdentityObservation, error) {
	if double.down[unit.String()] {
		return nil, errBookDown
	}
	return double.observations[unit.String()], nil
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("%q：%v", raw, err)
	}
	return value
}

func observation(t *testing.T, source, version, predecessor, parcel string) nodomain.IntakeIdentityObservation {
	t.Helper()
	observed := nodomain.IntakeIdentityObservation{
		SourceID:   source,
		Version:    must(t, nodomain.NewIntakeResultVersion, version),
		ReceivedAt: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC),
	}
	if predecessor != "" {
		observed.Predecessor = must(t, nodomain.NewIntakeResultVersion, predecessor)
	}
	if parcel != "" {
		observed.Association = must(t, nodomain.NewParcelAssociationReference, parcel)
	}
	return observed
}

func query(t *testing.T, object string, at time.Time) tfports.ConsolidationMembershipQuery {
	t.Helper()
	return tfports.ConsolidationMembershipQuery{
		TenantID: must(t, tfdomain.NewTenantID, "tenant-1"),
		Object:   must(t, tfdomain.NewCarriedObjectReference, object),
		At:       at,
	}
}

// Covers: AT-TF-277 适配器半边（ADR-0225 决定二、四）——节点作业的答复只翻译：嵌套成员译成那件单元的引用；其余成员各问一次身份，
// 由节点作业的身份判断认——成立即那件包裹，待识别或没有收寄记录即身份不成立，读不动或身份链说不准即身份读不动。时点按本上下文的
// 登记精度交过去。成员关系那一问读不动作为错误交回，不读成「不是单元」。
func TestMembersAreTranslatedThroughTheNodeIdentityJudgement(t *testing.T) {
	member := func(raw string, nested bool) noports.ConsolidationMemberAt {
		return noports.ConsolidationMemberAt{Member: must(t, nodomain.NewHandlingUnitID, raw), NestedUnit: nested}
	}
	memberships := &membershipsDouble{answers: map[string]noports.ConsolidationMembershipAt{
		"bag-1": {Unit: true, Members: []noports.ConsolidationMemberAt{
			member("bag-inner", true), member("hu-known", false), member("hu-pending", false), member("hu-unreceived", false),
			member("hu-down", false), member("hu-looped", false),
		}},
	}}
	identities := identitiesDouble{
		observations: map[string][]nodomain.IntakeIdentityObservation{
			"hu-known":   {observation(t, "scan-1", "v1", "", "PCL-1")},
			"hu-pending": {observation(t, "scan-2", "v2", "", "")},
			// 两版互相接续：哪一版是链头说不准。
			"hu-looped": {observation(t, "scan-3", "v3", "v4", "PCL-3"), observation(t, "scan-4", "v4", "v3", "PCL-3")},
		},
		down: map[string]bool{"hu-down": true},
	}
	view, err := tfnode.NewConsolidationMembershipView(memberships, identities)
	if err != nil {
		t.Fatalf("装适配器：%v", err)
	}
	at := time.Date(2026, 10, 21, 14, 5, 0, 123456789, time.UTC)

	answer, err := view.MembersAt(t.Context(), query(t, "bag-1", at))
	if err != nil {
		t.Fatalf("问成员：%v", err)
	}
	if !answer.Unit || len(answer.Members) != 6 {
		t.Fatalf("答复 = %+v", answer)
	}
	want := []struct {
		kind   tfports.ConsolidationMemberKind
		object string
	}{
		{tfports.ConsolidationMemberUnit, "bag-inner"},
		{tfports.ConsolidationMemberParcel, "PCL-1"},
		{tfports.ConsolidationMemberIdentityUnresolved, ""},
		{tfports.ConsolidationMemberIdentityUnresolved, ""},
		{tfports.ConsolidationMemberIdentityUnreadable, ""},
		{tfports.ConsolidationMemberIdentityUnreadable, ""},
	}
	for index, expected := range want {
		got := answer.Members[index]
		if got.Kind != expected.kind || (expected.object != "" && got.Object.String() != expected.object) || got.Handling == "" {
			t.Fatalf("第 %d 件 = %+v，要 %+v", index, got, expected)
		}
	}
	if !memberships.asked[0].Equal(tfdomain.AtRegistryPrecision(at)) {
		t.Fatalf("时点 = %s，要按登记精度", memberships.asked[0])
	}

	if answer, err := view.MembersAt(t.Context(), query(t, "PCL-9", at)); err != nil || answer.Unit || len(answer.Members) != 0 {
		t.Fatalf("不是单元的对象 = %+v / %v", answer, err)
	}
	memberships.err = errBookDown
	if _, err := view.MembersAt(t.Context(), query(t, "bag-1", at)); !errors.Is(err, errBookDown) ||
		errors.Is(err, tfports.ErrConsolidationMembershipUntranslatable) {
		t.Fatalf("成员关系读不动 = %v，要调用失败而不是译不回来", err)
	}
	if _, err := tfnode.NewConsolidationMembershipView(nil, identities); err == nil {
		t.Fatal("缺成员关系那一口也装得起来")
	}
}
