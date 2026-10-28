package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var collaborationDecidedAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)

func collabValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func acceptanceSpec(t *testing.T, decision domain.AcceptanceDecisionKind) domain.CollaborationAcceptanceSpec {
	t.Helper()
	spec := domain.CollaborationAcceptanceSpec{
		TenantID:  collabValue(t, domain.NewTenantID, "tenant-1"),
		Node:      collabValue(t, domain.NewNodeReference, "node-1"),
		Item:      collabValue(t, domain.NewCollaborationItemReference, "collaboration-item-1"),
		Decision:  decision,
		Authority: collabValue(t, domain.NewAcceptanceAuthorityReference, "node-authority-1"),
		DecidedAt: collaborationDecidedAt,
	}
	if decision != domain.CollaborationDeclined {
		spec.AcceptedUnits = []domain.HandlingUnitID{
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			collabValue(t, domain.NewHandlingUnitID, "unit-2"),
		}
		spec.AcceptedActions = []domain.CollaborationActionKind{domain.UnsealAction, domain.PresentAction}
	}
	if decision != domain.CollaborationAccepted {
		spec.Basis = collabValue(t, domain.NewAcceptanceBasisReference, "basis-"+decision.String())
	}
	return spec
}

func acceptedCollaboration(t *testing.T) domain.CollaborationAcceptance {
	t.Helper()
	acceptance, err := domain.DecideCollaborationAcceptance(acceptanceSpec(t, domain.CollaborationAccepted))
	if err != nil {
		t.Fatalf("decide acceptance: %v", err)
	}
	return acceptance
}

// Covers: UC-NO-001「承接决定是节点自己的决定：接受/拒接带因/部分承接」与硬句「承接
// 不等于执行完成」——三格各守完备性；类型上没有已执行或完成字段（结构防线）。
func TestAnAcceptanceIsTheNodesOwnDecision(t *testing.T) {
	acceptanceType := reflect.TypeOf(domain.CollaborationAcceptance{})
	for index := 0; index < acceptanceType.NumField(); index++ {
		name := strings.ToLower(acceptanceType.Field(index).Name)
		for _, forbidden := range []string{"executed", "completed", "performed", "done"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("CollaborationAcceptance 携带 %q——承接就能被读成执行完成", acceptanceType.Field(index).Name)
			}
		}
	}

	accepted := acceptedCollaboration(t)
	// 计数锚定本夹具：两件两动作（unit-1/unit-2 × UNSEAL/PRESENT，决于 2026-08-13T10:00Z）。
	if len(accepted.AcceptedUnits()) != 2 || len(accepted.AcceptedActions()) != 2 {
		t.Fatalf("units = %d actions = %d, want 2/2", len(accepted.AcceptedUnits()), len(accepted.AcceptedActions()))
	}
	if _, present := accepted.Basis(); present {
		t.Fatal("全量接受凭空带上了拒接原因")
	}

	t.Run("a declined item carries its basis and no scope", func(t *testing.T) {
		declined, err := domain.DecideCollaborationAcceptance(acceptanceSpec(t, domain.CollaborationDeclined))
		if err != nil {
			t.Fatalf("decide declined: %v", err)
		}
		if _, present := declined.Basis(); !present {
			t.Fatal("拒接丢了原因")
		}
		if len(declined.AcceptedUnits()) != 0 {
			t.Fatal("拒接还带了承接范围")
		}
	})

	broken := map[string]func(*domain.CollaborationAcceptanceSpec){
		"declined without a basis": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Decision = domain.CollaborationDeclined
			spec.AcceptedUnits = nil
			spec.AcceptedActions = nil
			spec.Basis = domain.AcceptanceBasisReference{}
		},
		"declined carrying a scope": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Decision = domain.CollaborationDeclined
			spec.Basis = collabValue(t, domain.NewAcceptanceBasisReference, "basis-x")
			spec.AcceptedUnits = []domain.HandlingUnitID{collabValue(t, domain.NewHandlingUnitID, "unit-1")}
			spec.AcceptedActions = nil
		},
		"partial without a basis": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Decision = domain.CollaborationPartiallyAccepted
			spec.Basis = domain.AcceptanceBasisReference{}
		},
		"accepted without units": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.AcceptedUnits = nil
		},
		"accepted without actions": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.AcceptedActions = nil
		},
		"duplicate unit": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.AcceptedUnits = append(spec.AcceptedUnits, spec.AcceptedUnits[0])
		},
		"duplicate action": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.AcceptedActions = append(spec.AcceptedActions, spec.AcceptedActions[0])
		},
		"no authority": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Authority = domain.AcceptanceAuthorityReference{}
		},
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := acceptanceSpec(t, domain.CollaborationAccepted)
			breakSpec(&spec)
			if _, err := domain.DecideCollaborationAcceptance(spec); !errors.Is(err, domain.ErrInvalidCollaborationAcceptance) {
				t.Fatalf("error = %v, want ErrInvalidCollaborationAcceptance", err)
			}
		})
	}

	t.Run("the decision set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, decision := range []domain.AcceptanceDecisionKind{
			domain.CollaborationAccepted, domain.CollaborationDeclined, domain.CollaborationPartiallyAccepted,
			domain.CollaborationPendingSupplement,
		} {
			label := decision.String()
			if label == "" {
				t.Fatalf("decision %d has no label", decision)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.AcceptanceDecisionKind(len(labels)+1).String() != "" {
			t.Fatal("第五个决定取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: UC-NO-001「授权范围内执行（越权拒）」——执行只在承接的对象与动作范围内成立；
// 拒接的事项没有可执行的东西；证据必备；执行不早于承接。
func TestExecutionFactsStayWithinTheAcceptedScope(t *testing.T) {
	acceptance := acceptedCollaboration(t)
	evidence := collabValue(t, domain.NewExecutionEvidenceReference, "execution-evidence-1")

	fact, err := domain.RecordExecutionFact(
		acceptance,
		collabValue(t, domain.NewHandlingUnitID, "unit-1"),
		domain.UnsealAction,
		evidence,
		collaborationDecidedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("record fact: %v", err)
	}
	if fact.Action() != domain.UnsealAction || fact.Unit().String() != "unit-1" {
		t.Fatalf("fact = %q/%q", fact.Action(), fact.Unit())
	}

	t.Run("a unit outside the accepted scope is refused", func(t *testing.T) {
		if _, err := domain.RecordExecutionFact(
			acceptance,
			collabValue(t, domain.NewHandlingUnitID, "unit-9"),
			domain.UnsealAction,
			evidence,
			collaborationDecidedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrOutsideAcceptedScope) {
			t.Fatalf("error = %v, want ErrOutsideAcceptedScope", err)
		}
	})

	t.Run("an action outside the accepted scope is refused", func(t *testing.T) {
		if _, err := domain.RecordExecutionFact(
			acceptance,
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			domain.IsolateAction,
			evidence,
			collaborationDecidedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrOutsideAcceptedScope) {
			t.Fatalf("error = %v; 未承接的动作被执行了", err)
		}
	})

	t.Run("a declined item has nothing to execute", func(t *testing.T) {
		declined, err := domain.DecideCollaborationAcceptance(acceptanceSpec(t, domain.CollaborationDeclined))
		if err != nil {
			t.Fatalf("decide declined: %v", err)
		}
		if _, err := domain.RecordExecutionFact(
			declined,
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			domain.UnsealAction,
			evidence,
			collaborationDecidedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrCollaborationRefused) {
			t.Fatalf("error = %v, want ErrCollaborationRefused", err)
		}
	})

	t.Run("execution without evidence is refused", func(t *testing.T) {
		if _, err := domain.RecordExecutionFact(
			acceptance,
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			domain.UnsealAction,
			domain.ExecutionEvidenceReference{},
			collaborationDecidedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidExecutionFact) {
			t.Fatalf("error = %v; 没有证据的执行与数据丢失无从分辨", err)
		}
	})

	t.Run("execution before the decision is refused", func(t *testing.T) {
		if _, err := domain.RecordExecutionFact(
			acceptance,
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			domain.UnsealAction,
			evidence,
			collaborationDecidedAt.Add(-time.Hour),
		); !errors.Is(err, domain.ErrInvalidExecutionFact) {
			t.Fatalf("error = %v; 承接之前没有可记的协作执行", err)
		}
	})
}

// Covers: CC CONTEXT 177「节点执行事实不冒充监管查验结果」——事实类型上没有任何结论、
// 放行或处置字段（结构防线）；动作封闭五值；逐件形成互不吞并。
func TestExecutionFactsDoNotImpersonateInspectionResults(t *testing.T) {
	factType := reflect.TypeOf(domain.NodeExecutionFact{})
	for index := 0; index < factType.NumField(); index++ {
		name := strings.ToLower(factType.Field(index).Name)
		for _, forbidden := range []string{"verdict", "result", "inspection", "release", "disposition"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("NodeExecutionFact 携带 %q——节点就能替监管说结论", factType.Field(index).Name)
			}
		}
	}

	acceptance := acceptedCollaboration(t)
	evidence := collabValue(t, domain.NewExecutionEvidenceReference, "execution-evidence-1")
	first, err := domain.RecordExecutionFact(acceptance,
		collabValue(t, domain.NewHandlingUnitID, "unit-1"), domain.PresentAction, evidence,
		collaborationDecidedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("record first: %v", err)
	}
	second, err := domain.RecordExecutionFact(acceptance,
		collabValue(t, domain.NewHandlingUnitID, "unit-2"), domain.PresentAction, evidence,
		collaborationDecidedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("record second: %v", err)
	}
	if first.Unit() == second.Unit() {
		t.Fatal("逐件事实塌成了一件")
	}

	t.Run("the action set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, action := range []domain.CollaborationActionKind{
			domain.UnsealAction, domain.IsolateAction, domain.PresentAction,
			domain.TallyAction, domain.ObserveAction,
		} {
			label := action.String()
			if label == "" {
				t.Fatalf("action %d has no label", action)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.CollaborationActionKind(len(labels)+1).String() != "" {
			t.Fatal("第六个动作取值带了标签——放行或查验结论溜进了节点动作集")
		}
	})
}

func pendingSpec(t *testing.T, gaps ...domain.SupplementGapKind) domain.CollaborationAcceptanceSpec {
	t.Helper()
	spec := acceptanceSpec(t, domain.CollaborationDeclined)
	spec.Decision = domain.CollaborationPendingSupplement
	spec.Basis = domain.AcceptanceBasisReference{}
	spec.Gaps = gaps
	return spec
}

// Covers: UC-NO-001 `AT-NO-002`/`AT-NO-003`——对象、动作、节点、授权依据或必要作业条件不足时
// 形成待补充：带缺口、不带承接范围，且没有可执行的东西（与拒接、越权分格）。
func TestAPendingSupplementCarriesGapsAndNoScope(t *testing.T) {
	pending, err := domain.DecideCollaborationAcceptance(pendingSpec(t, domain.MissingObjectGap, domain.MissingAuthorityGap))
	if err != nil {
		t.Fatalf("decide pending: %v", err)
	}
	if pending.Decision() != domain.CollaborationPendingSupplement || len(pending.Gaps()) != 2 {
		t.Fatalf("decision = %v gaps = %v", pending.Decision(), pending.Gaps())
	}
	if len(pending.AcceptedUnits()) != 0 || len(pending.AcceptedActions()) != 0 {
		t.Fatal("待补充不形成承接，却带了承接范围")
	}

	broken := map[string]func(*domain.CollaborationAcceptanceSpec){
		"no gaps": func(spec *domain.CollaborationAcceptanceSpec) { spec.Gaps = nil },
		"unknown gap": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Gaps = []domain.SupplementGapKind{domain.SupplementGapKindInvalid}
		},
		"duplicate gap": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.Gaps = []domain.SupplementGapKind{domain.MissingNodeGap, domain.MissingNodeGap}
		},
		"carrying a scope": func(spec *domain.CollaborationAcceptanceSpec) {
			spec.AcceptedUnits = []domain.HandlingUnitID{collabValue(t, domain.NewHandlingUnitID, "unit-1")}
			spec.AcceptedActions = []domain.CollaborationActionKind{domain.UnsealAction}
		},
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := pendingSpec(t, domain.MissingActionGap)
			breakSpec(&spec)
			if _, err := domain.DecideCollaborationAcceptance(spec); !errors.Is(err, domain.ErrInvalidCollaborationAcceptance) {
				t.Fatalf("error = %v, want ErrInvalidCollaborationAcceptance", err)
			}
		})
	}

	t.Run("gaps belong to pending only", func(t *testing.T) {
		spec := acceptanceSpec(t, domain.CollaborationAccepted)
		spec.Gaps = []domain.SupplementGapKind{domain.MissingObjectGap}
		if _, err := domain.DecideCollaborationAcceptance(spec); !errors.Is(err, domain.ErrInvalidCollaborationAcceptance) {
			t.Fatalf("error = %v, want ErrInvalidCollaborationAcceptance", err)
		}
	})

	t.Run("nothing can be executed against it", func(t *testing.T) {
		_, err := domain.RecordExecutionFact(
			pending,
			collabValue(t, domain.NewHandlingUnitID, "unit-1"),
			domain.UnsealAction,
			collabValue(t, domain.NewExecutionEvidenceReference, "evidence-1"),
			collaborationDecidedAt.Add(time.Hour),
		)
		if !errors.Is(err, domain.ErrCollaborationPendingSupplement) {
			t.Fatalf("error = %v, want ErrCollaborationPendingSupplement", err)
		}
	})

	t.Run("the gap set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, gap := range []domain.SupplementGapKind{
			domain.MissingObjectGap, domain.MissingActionGap, domain.MissingNodeGap,
			domain.MissingAuthorityGap, domain.MissingWorkConditionGap,
		} {
			if gap.String() == "" {
				t.Fatalf("gap %d has no label", gap)
			}
			labels[gap.String()] = struct{}{}
		}
		if len(labels) != 5 || domain.SupplementGapKind(len(labels)+1).String() != "" {
			t.Fatal("缺口取值集合不封闭")
		}
	})
}
