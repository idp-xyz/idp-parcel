package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var journeyStartedAt = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

func alternateJourneySpec(t *testing.T, purpose domain.JourneyPurpose, kind domain.DispositionBasisKind) domain.AlternateJourneySpec {
	t.Helper()
	return domain.AlternateJourneySpec{
		TenantID:        mustValue(t, domain.NewTenantID, "tenant-1"),
		Journey:         mustValue(t, domain.NewJourneyReference, "journey-return-1"),
		Purpose:         purpose,
		OriginalJourney: mustValue(t, domain.NewJourneyReference, "journey-original-1"),
		BasisKind:       kind,
		Basis:           mustValue(t, domain.NewDispositionBasisReference, "DISPOSITION/decision-1"),
		Members: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		},
		StartedAt: journeyStartedAt,
	}
}

// Covers: `AT-TF-079`「普通退运决定、路径和责任齐备 → 建立新路由/旅程并关联原旅程」与
// CONTEXT「退运围绕新的服务目的形成关联但独立的旅程……不得通过倒退原实际履约段、原路由
// 或原交付状态表达」——关联不可缺、身份必须独立；类型上没有任何持有或触碰原段/原交付/
// 原交接本体的地方（结构防线）。
func TestAnAlternateJourneyIsLinkedButIndependent(t *testing.T) {
	journeyType := reflect.TypeOf(domain.AlternateJourney{})
	forbidden := []reflect.Type{
		reflect.TypeOf(domain.ActualFulfillmentSegment{}),
		reflect.TypeOf(domain.EffectiveDelivery{}),
		reflect.TypeOf(domain.TransportHandover{}),
		reflect.TypeOf(domain.OffsitePickup{}),
	}
	for index := 0; index < journeyType.NumField(); index++ {
		field := journeyType.Field(index)
		for _, banned := range forbidden {
			if field.Type == banned {
				t.Fatalf("AlternateJourney 持有 %q（%s）——新旅程就有了倒退原历史的把手", field.Name, banned)
			}
		}
	}
	for index := 0; index < reflect.TypeOf(domain.AlternateJourney{}).NumMethod(); index++ {
		name := strings.ToLower(reflect.TypeOf(domain.AlternateJourney{}).Method(index).Name)
		for _, banned := range []string{"rollback", "revert", "rewind", "overwrite", "cancel"} {
			if strings.Contains(name, banned) {
				t.Fatalf("AlternateJourney 带方法 %q——独立旅程不该有回写入口", name)
			}
		}
	}

	journey, err := domain.FormAlternateJourney(alternateJourneySpec(t, domain.ReturnJourneyPurpose, domain.ServiceDispositionDecision))
	if err != nil {
		t.Fatalf("form alternate journey: %v", err)
	}
	if journey.OriginalJourney().String() != "journey-original-1" {
		t.Fatal("新旅程丢了对原旅程的关联")
	}
	if journey.Journey() == journey.OriginalJourney() {
		t.Fatal("新旅程与原旅程共用了身份")
	}
	if journey.Purpose() != domain.ReturnJourneyPurpose {
		t.Fatalf("purpose = %q, want RETURN", journey.Purpose())
	}
	if !journey.StartedAt().Equal(journeyStartedAt) {
		t.Fatalf("started at = %s", journey.StartedAt())
	}

	broken := map[string]func(*domain.AlternateJourneySpec){
		"same identity as the original": func(spec *domain.AlternateJourneySpec) {
			spec.Journey = spec.OriginalJourney
		},
		"no original journey": func(spec *domain.AlternateJourneySpec) {
			spec.OriginalJourney = domain.JourneyReference{}
		},
		"no disposition basis": func(spec *domain.AlternateJourneySpec) {
			// 只有拒收或失败事实、没有处置决定：新旅程立不起来（AT-TF-078）。
			spec.Basis = domain.DispositionBasisReference{}
		},
		"no basis kind": func(spec *domain.AlternateJourneySpec) {
			spec.BasisKind = domain.DispositionBasisKindInvalid
		},
		"no purpose": func(spec *domain.AlternateJourneySpec) {
			spec.Purpose = domain.JourneyPurposeInvalid
		},
		"no members": func(spec *domain.AlternateJourneySpec) {
			spec.Members = nil
		},
		"duplicate member": func(spec *domain.AlternateJourneySpec) {
			spec.Members = append(spec.Members, spec.Members[0])
		},
		"no start time": func(spec *domain.AlternateJourneySpec) {
			spec.StartedAt = time.Time{}
		},
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := alternateJourneySpec(t, domain.ReturnJourneyPurpose, domain.ServiceDispositionDecision)
			breakSpec(&spec)
			if _, err := domain.FormAlternateJourney(spec); !errors.Is(err, domain.ErrInvalidAlternateJourney) {
				t.Fatalf("error = %v, want ErrInvalidAlternateJourney", err)
			}
		})
	}
}

// Covers: `AT-TF-080`「监管退运决定到达本用例 → 转 UC-TF-001，不得用普通退运绕过监管
// 范围」——决定来源以封闭二值分格并必填，监管来路在对象上可见；两个目的、两种来源的
// 组合各自可立，封闭集合外没有第三格。
func TestRegulatoryReturnIsMarkedByItsBasisKind(t *testing.T) {
	regulatory, err := domain.FormAlternateJourney(alternateJourneySpec(t, domain.ReturnJourneyPurpose, domain.RegulatoryDispositionDecision))
	if err != nil {
		t.Fatalf("form regulatory return: %v", err)
	}
	if !regulatory.RegulatoryOrigin() {
		t.Fatal("监管退运没有被标出来——普通退运的路就能冒走监管范围")
	}

	service, err := domain.FormAlternateJourney(alternateJourneySpec(t, domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision))
	if err != nil {
		t.Fatalf("form service alternate: %v", err)
	}
	if service.RegulatoryOrigin() {
		t.Fatal("普通处置被读成了监管来路")
	}
	if service.Purpose() != domain.AlternateJourneyPurpose {
		t.Fatalf("purpose = %q, want ALTERNATE", service.Purpose())
	}

	t.Run("the purpose and basis kind sets are closed", func(t *testing.T) {
		if domain.JourneyPurpose(3).String() != "" {
			t.Fatal("第三个旅程目的带了标签——封闭集合被悄悄放开")
		}
		if domain.DispositionBasisKind(3).String() != "" {
			t.Fatal("第三个决定来源带了标签——封闭集合被悄悄放开")
		}
		labels := map[string]struct{}{
			domain.AlternateJourneyPurpose.String():       {},
			domain.ReturnJourneyPurpose.String():          {},
			domain.ServiceDispositionDecision.String():    {},
			domain.RegulatoryDispositionDecision.String(): {},
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
	})
}
