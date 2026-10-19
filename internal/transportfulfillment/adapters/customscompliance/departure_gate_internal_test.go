package customscompliance

import (
	"errors"
	"testing"
	"time"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 关务答复各格到出发读数各格的对照（ADR-0205 决定二）——未配置落未决，越界的一格是译不回来，不冒充任何一格。
func TestEveryCustomsAnswerHasExactlyOneDepartureAnswer(t *testing.T) {
	for customs, want := range map[ccdomain.AdmissionAnswer]tfdomain.DepartureGateAnswer{
		ccdomain.AdmissionNotGuarded:    tfdomain.DepartureGateNotGuarded,
		ccdomain.AdmissionAdmitted:      tfdomain.DepartureGateAdmitted,
		ccdomain.AdmissionUndecided:     tfdomain.DepartureGateUndecided,
		ccdomain.AdmissionNotConfigured: tfdomain.DepartureGateUndecided,
		ccdomain.AdmissionBlocked:       tfdomain.DepartureGateBlocked,
	} {
		got, err := answerFor(customs)
		if err != nil || got != want {
			t.Errorf("%s → %s (%v), want %s", customs, got, err, want)
		}
	}
	if _, err := answerFor(ccdomain.AdmissionAnswerInvalid); !errors.Is(err, ErrUntranslatableAnswer) ||
		!errors.Is(err, tfports.ErrDepartureGateAnswerUntranslatable) {
		t.Fatalf("越界的一格：err = %v, want ErrUntranslatableAnswer 且认得出端口的译不回来", err)
	}
}

// Covers: 关务形成了答复、逐对象那一格却立不起来（这里是没有同格依据）时归译不回来，编排据此与调用失败分开记依据种类。
func TestAFindingThatCannotBeFormedIsUntranslatable(t *testing.T) {
	object, err := tfdomain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	_, err = findingOf(tfdomain.NewDepartureGateFinding(object, tfdomain.DepartureGateNotGuarded, "AADJ-1", nil))
	if !errors.Is(err, tfports.ErrDepartureGateAnswerUntranslatable) {
		t.Fatalf("err = %v, want 端口的译不回来", err)
	}
}

// Covers: 只带回与整次答复同格的依据——跨境出发对象不在案时，装载出发那一道答不受门禁、跨区那一道答未决，整次答未决；带回的
// 只有跨区那一条，免得读的人把被压下去的「不受门禁」当成这次出发的理由。
func TestOnlyTheBasesOfTheWinningAnswerAreCarriedBack(t *testing.T) {
	node := func(value string) ccdomain.NetworkNodeReference {
		reference, err := ccdomain.NewNetworkNodeReference(value)
		if err != nil {
			t.Fatalf("节点：%v", err)
		}
		return reference
	}
	zone := func(value string) ccdomain.CustomsZoneReference {
		reference, err := ccdomain.NewCustomsZoneReference(value)
		if err != nil {
			t.Fatalf("区域：%v", err)
		}
		return reference
	}
	intent, err := ccdomain.NewDepartureIntent(node("SZX"), node("SIN"))
	if err != nil {
		t.Fatalf("意图：%v", err)
	}
	object, err := ccdomain.NewDeclaredParcelReference("parcel-1")
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	at := time.Date(2026, 9, 24, 14, 0, 0, 0, time.UTC)
	judgement, err := ccdomain.JudgeActionAdmission(ccdomain.ActionAdmissionInput{
		Object: object,
		Intent: intent,
		At:     at,
		Zones: []ccdomain.NodeZoneReading{
			{Node: node("SZX"), Zone: zone("CN"), Registered: true},
			{Node: node("SIN"), Zone: zone("SG"), Registered: true},
		},
		JudgedAt: at.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if judgement.Answer() != ccdomain.AdmissionUndecided || len(judgement.Findings()) != 2 {
		t.Fatalf("前提不成立：%s %+v", judgement.Answer(), judgement.Findings())
	}

	bases := basesFor(judgement)
	if len(bases) != 1 || bases[0].Action != "CROSS_CUSTOMS_MOVEMENT" || bases[0].Cause != "CUSTOMS_SCOPE_UNRESOLVED" {
		t.Fatalf("带回的依据 = %+v，只该有跨区那一条", bases)
	}
}
