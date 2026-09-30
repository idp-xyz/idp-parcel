package domain

import "testing"

func TestAnAmountWithinTheCeilingStaysInsideAuthority(t *testing.T) {
	ceiling := escalationCeiling(t, 12_000)

	judgment, err := ceiling.Judge(12_000)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if judgment != AuditWithinAuthority {
		t.Fatalf("judgment = %s，等于上限仍应在权限内", judgment)
	}
}

func TestAnAmountAboveTheCeilingMustEscalate(t *testing.T) {
	ceiling := escalationCeiling(t, 11_999)

	judgment, err := ceiling.Judge(12_000)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if judgment != AuditMustEscalate {
		t.Fatalf("judgment = %s，想要 MUST_ESCALATE", judgment)
	}
}

func TestARegisteredZeroCeilingIsNotUnconfigured(t *testing.T) {
	ceiling := escalationCeiling(t, 0)

	judgment, err := ceiling.Judge(1)
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if judgment != AuditMustEscalate {
		t.Fatalf("judgment = %s，上限 0 仍是一份登记", judgment)
	}
	var unset AuditEscalationCeiling
	if _, err := unset.Judge(1); err == nil {
		t.Fatal("零值上限被当成一份登记")
	}
}

func TestAuditEscalationRejectsANegativeCeilingOrAmount(t *testing.T) {
	if _, err := NewAuditEscalationCeiling(-1); err == nil {
		t.Fatal("负上限被收下")
	}
	if _, err := escalationCeiling(t, 10).Judge(0); err == nil {
		t.Fatal("金额 0 仍判出了升级结果")
	}
}

func escalationCeiling(t *testing.T, limit int64) AuditEscalationCeiling {
	t.Helper()
	ceiling, err := NewAuditEscalationCeiling(limit)
	if err != nil {
		t.Fatalf("ceiling: %v", err)
	}
	return ceiling
}
