package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

var receivedAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func intakeSpec(t *testing.T) domain.NodeIntakeSpec {
	t.Helper()
	return domain.NodeIntakeSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Unit:        mustValue(t, domain.NewHandlingUnitID, "unit-1"),
		Node:        mustValue(t, domain.NewNodeReference, "node-origin"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "RECEPTION/SIGN-7"),
		Version:     mustValue(t, domain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  receivedAt,
	}
}

// Covers: node-operations CONTEXT「节点收寄：客户或其授权交付方在物流节点直接交付作业
// 实物，节点完成明确接收并取得控制的业务结果」——接收证据是与「到站扫描、卸载或发现
// 实物」的分界，构造期必备；包裹关联可缺席（待识别实物同样真实被收寄）。
func TestANodeIntakeDemandsReceptionEvidenceButNotAnAssociation(t *testing.T) {
	pending, err := domain.FormNodeIntake(intakeSpec(t))
	if err != nil {
		t.Fatalf("form node intake: %v", err)
	}
	if _, identified := pending.Association(); identified {
		t.Fatal("待识别实物凭空长出了包裹关联")
	}
	if !pending.ReceivedAt().Equal(receivedAt) {
		t.Fatalf("received at = %s", pending.ReceivedAt())
	}

	associated := intakeSpec(t)
	associated.Association = mustValue(t, domain.NewParcelAssociationReference, "parcel-1/link-v1")
	identified, err := domain.FormNodeIntake(associated)
	if err != nil {
		t.Fatalf("form associated intake: %v", err)
	}
	if association, present := identified.Association(); !present || association.String() != "parcel-1/link-v1" {
		t.Fatalf("association = %v present = %v", association, present)
	}

	cases := map[string]func(domain.NodeIntakeSpec) domain.NodeIntakeSpec{
		"no reception evidence": func(spec domain.NodeIntakeSpec) domain.NodeIntakeSpec {
			spec.Evidence = domain.ReceptionEvidenceReference{}
			return spec
		},
		"no delivering party": func(spec domain.NodeIntakeSpec) domain.NodeIntakeSpec {
			spec.DeliveredBy = domain.DeliveringPartyReference{}
			return spec
		},
		"no node": func(spec domain.NodeIntakeSpec) domain.NodeIntakeSpec {
			spec.Node = domain.NodeReference{}
			return spec
		},
		"no result version": func(spec domain.NodeIntakeSpec) domain.NodeIntakeSpec {
			spec.Version = domain.IntakeResultVersion{}
			return spec
		},
		"no reception time": func(spec domain.NodeIntakeSpec) domain.NodeIntakeSpec {
			spec.ReceivedAt = time.Time{}
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.FormNodeIntake(mutate(intakeSpec(t))); !errors.Is(err, domain.ErrInvalidNodeIntake) {
				t.Fatalf("err = %v, want ErrInvalidNodeIntake", err)
			}
		})
	}
}

// Covers: CONTEXT「待识别实物……识别成功后通过版本化关联连接正式对象，不删除原记录」——
// 识别换新版本补关联，原版本原样保留；已有关联不得换绑（那是身份冲突的处置不是识别），
// 重号版本分不出两代。
func TestIdentificationAddsAVersionedAssociationWithoutRewriting(t *testing.T) {
	pending, err := domain.FormNodeIntake(intakeSpec(t))
	if err != nil {
		t.Fatalf("form node intake: %v", err)
	}

	identified, err := pending.Identify(
		mustValue(t, domain.NewParcelAssociationReference, "parcel-1/link-v1"),
		mustValue(t, domain.NewIntakeResultVersion, "intake-result/v2"),
	)
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	association, present := identified.Association()
	if !present || association.String() != "parcel-1/link-v1" {
		t.Fatalf("association = %v present = %v", association, present)
	}
	if identified.Version().String() != "intake-result/v2" {
		t.Fatalf("version = %s; 识别必须换新版本", identified.Version())
	}
	if _, stillPending := pending.Association(); stillPending {
		t.Fatal("原记录被改写了")
	}
	if !identified.ReceivedAt().Equal(pending.ReceivedAt()) {
		t.Fatal("识别复制了已发生的接收事实")
	}

	if _, err := identified.Identify(
		mustValue(t, domain.NewParcelAssociationReference, "parcel-2/link-v1"),
		mustValue(t, domain.NewIntakeResultVersion, "intake-result/v3"),
	); !errors.Is(err, domain.ErrInvalidNodeIntake) {
		t.Fatalf("err = %v; 已有关联被换绑了", err)
	}
	if _, err := pending.Identify(
		mustValue(t, domain.NewParcelAssociationReference, "parcel-1/link-v1"),
		pending.Version(),
	); !errors.Is(err, domain.ErrInvalidNodeIntake) {
		t.Fatalf("err = %v; 重号版本分不出两代", err)
	}
}
