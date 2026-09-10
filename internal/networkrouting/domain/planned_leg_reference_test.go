package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// Covers: ADR-0131 决定一——计划履约段对外以「路由计划版本 + 段序位」引用，拼写由本上下文
// 一处定义、String 与解析往返相等。TF 只搬运这个串不解读，所以往返相等是它能依赖的全部。
func TestAPlannedLegReferenceRoundTripsThroughItsSpelling(t *testing.T) {
	version := mustValue(t, domain.NewRoutePlanVersionID, "RPV-000000000007")
	reference, err := domain.NewPlannedLegReference(version, 2)
	if err != nil {
		t.Fatalf("new planned leg reference: %v", err)
	}
	if reference.Version() != version || reference.Ordinal() != 2 {
		t.Fatalf("reference = %+v", reference)
	}
	if got := reference.String(); got != "RPV-000000000007#2" {
		t.Fatalf("spelling = %q, want RPV-000000000007#2", got)
	}

	parsed, err := domain.ParsePlannedLegReference(reference.String())
	if err != nil {
		t.Fatalf("parse %q: %v", reference.String(), err)
	}
	if parsed != reference {
		t.Fatalf("parsed = %+v, want %+v; 往返必须逐值相等", parsed, reference)
	}

	// 版本标识本身含分隔符时也要往返：序位是最后一段且只由数字组成，解析从最后一个分隔符切。
	withSeparator, err := domain.NewPlannedLegReference(mustValue(t, domain.NewRoutePlanVersionID, "plan#1"), 3)
	if err != nil {
		t.Fatalf("new planned leg reference: %v", err)
	}
	parsed, err = domain.ParsePlannedLegReference(withSeparator.String())
	if err != nil || parsed != withSeparator {
		t.Fatalf("parse %q = %+v, %v; want %+v", withSeparator.String(), parsed, err, withSeparator)
	}
}

// Covers: 构造门拒空版本与非正序位——序位自首段起计、首段为 1，0 与负数不指任何一段；
// 没有版本的序位不指任何一版计划。
func TestAPlannedLegReferenceRefusesABlankVersionOrANonPositiveOrdinal(t *testing.T) {
	version := mustValue(t, domain.NewRoutePlanVersionID, "RPV-000000000007")

	if _, err := domain.NewPlannedLegReference(domain.RoutePlanVersionID{}, 1); !errors.Is(err, domain.ErrInvalidPlannedLegReference) {
		t.Fatalf("blank version: err = %v, want ErrInvalidPlannedLegReference", err)
	}
	for _, ordinal := range []int{0, -1} {
		if _, err := domain.NewPlannedLegReference(version, ordinal); !errors.Is(err, domain.ErrInvalidPlannedLegReference) {
			t.Fatalf("ordinal %d: err = %v, want ErrInvalidPlannedLegReference", ordinal, err)
		}
	}
}

// Covers: 解析只认能原样往返的拼写——缺段、空版本、非正序位、前导零、非数字、夹空白都拒。
// 「接受但写回不同」会让 TF 存的串与 NR 认的串是两份，往返相等就守不住了。
func TestParsingAPlannedLegReferenceRefusesSpellingsThatWouldNotRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"",
		"RPV-000000000007",
		"RPV-000000000007#",
		"#2",
		"  #2",
		"RPV-000000000007#0",
		"RPV-000000000007#-1",
		"RPV-000000000007#+1",
		"RPV-000000000007#02",
		"RPV-000000000007#x",
		"RPV-000000000007# 2",
		"RPV-000000000007#2 ",
	} {
		if _, err := domain.ParsePlannedLegReference(raw); !errors.Is(err, domain.ErrInvalidPlannedLegReference) {
			t.Errorf("parse %q: err = %v, want ErrInvalidPlannedLegReference", raw, err)
		}
	}
}

// Covers: 按序位取段自首段起计（CONTEXT「自首段起计」同口径），越界答「没有」不 panic——
// 引用指到段链之外是业务答案，窄读口据此答未找到。
func TestAPlanAnswersALegByOrdinalCountedFromTheFirstLeg(t *testing.T) {
	plan, err := domain.FormInitialRoutePlan(planSpec(t))
	if err != nil {
		t.Fatalf("form initial route plan: %v", err)
	}

	first, found := plan.LegAt(1)
	if !found || first.From().String() != "node-origin" || first.To().String() != "node-hub" {
		t.Fatalf("LegAt(1) = %+v, %v; 首段序位为 1", first, found)
	}
	second, found := plan.LegAt(2)
	if !found || second.To().String() != "node-destination" {
		t.Fatalf("LegAt(2) = %+v, %v", second, found)
	}
	for _, ordinal := range []int{0, -1, 3} {
		if _, found := plan.LegAt(ordinal); found {
			t.Errorf("LegAt(%d) 答了找到；段链只有两段", ordinal)
		}
	}
	if _, found := (domain.InitialRoutePlan{}).LegAt(1); found {
		t.Error("零值计划的 LegAt 答了找到")
	}
}
