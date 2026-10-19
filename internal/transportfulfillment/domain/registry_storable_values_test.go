package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉票 tf-face-algebra/01 的判零一半：装载分配与移动事实的时刻非零门按登记册精度判（规则见 AtRegistryPrecision）。
// 夹具全为合成值（S 级）。

var (
	// zeroInstant 是 Go 的零时刻；登记册上的样子在测试里独立按微秒截出，不经被测函数。
	zeroInstant = time.Time{}
	// subMicroAfterZero 在登记册上都是零点：timestamptz 截到微秒后存成零时刻，读回即被重建复用的构造门拒。
	subMicroAfterZero = map[string]time.Time{
		"零点后 1ns":            zeroInstant.Add(time.Nanosecond),
		"零点后 999ns":          zeroInstant.Add(999 * time.Nanosecond),
		"+14:00 写法零点后 500ns": time.Date(1, 1, 1, 14, 0, 0, 500, time.FixedZone("SYN+14", 14*3600)),
	}
	// storableNearZero 在登记册上不是零点，照常形成。
	storableNearZero = map[string]time.Time{
		"零点后 1µs": zeroInstant.Add(time.Microsecond),
		"零点前 1ns": zeroInstant.Add(-time.Nanosecond),
	}
)

func TestSubMicrosecondFixturesAreZeroOnTheRegistry(t *testing.T) {
	for name, at := range subMicroAfterZero {
		if at.IsZero() || !at.UTC().Truncate(time.Microsecond).IsZero() {
			t.Fatalf("%s：夹具应按纳秒非零、按微秒为零", name)
		}
	}
	for name, at := range storableNearZero {
		if at.UTC().Truncate(time.Microsecond).IsZero() {
			t.Fatalf("%s：夹具按微秒应非零", name)
		}
	}
}

// Covers: 票 tf-face-algebra/01 修复方向 1——分配时刻在登记册上是零点就不形成；零点后一微秒、零点前一纳秒照常；恰为零点照旧拒。
func TestALoadAssignmentAtTheZeroInstantOnTheRegistryIsNotFormed(t *testing.T) {
	cases := map[string]time.Time{"恰为零点": zeroInstant}
	for name, at := range subMicroAfterZero {
		cases[name] = at
	}
	for name, at := range cases {
		spec := assignmentSpec(t)
		spec.AssignedAt = at
		if _, err := domain.FormLoadAssignment(spec); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("%s：err = %v，期望 ErrInvalidLoadAssignment", name, err)
		}
	}
	for name, at := range storableNearZero {
		spec := assignmentSpec(t)
		spec.AssignedAt = at
		if _, err := domain.FormLoadAssignment(spec); err != nil {
			t.Fatalf("%s：err = %v，期望照常形成", name, err)
		}
	}
}

// Covers: 票 tf-face-algebra/01 修复方向 1——变化与撤回的时刻门照同一判据。分配取零点前一纳秒，让「不早于分配时刻」那道门放行，
// 只剩非零门在判。
func TestLoadAssignmentRevisionAndWithdrawalAtTheZeroInstantOnTheRegistryAreRefused(t *testing.T) {
	spec := assignmentSpec(t)
	spec.AssignedAt = zeroInstant.Add(-time.Nanosecond)
	assignment, err := domain.FormLoadAssignment(spec)
	if err != nil {
		t.Fatalf("形成分配：%v", err)
	}
	members := []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1")}
	next := mustValue(t, domain.NewLoadAssignmentVersion, "SYN-LA-V2")
	for name, at := range subMicroAfterZero {
		if _, err := assignment.ReviseMembers(members, next, at); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("变化 %s：err = %v，期望 ErrInvalidLoadAssignment", name, err)
		}
		if _, err := assignment.Withdraw(next, at); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("撤回 %s：err = %v，期望 ErrInvalidLoadAssignment", name, err)
		}
	}
	at := zeroInstant.Add(time.Microsecond)
	if _, err := assignment.ReviseMembers(members, next, at); err != nil {
		t.Fatalf("变化零点后 1µs：%v", err)
	}
	if _, err := assignment.Withdraw(next, at); err != nil {
		t.Fatalf("撤回零点后 1µs：%v", err)
	}
}

// Covers: 票 tf-face-algebra/01 修复方向 1——移动事实发生时刻在登记册上是零点就不记录；零点后一微秒、零点前一纳秒照常。
func TestAMovementFactAtTheZeroInstantOnTheRegistryIsNotRecorded(t *testing.T) {
	cases := map[string]time.Time{"恰为零点": zeroInstant}
	for name, at := range subMicroAfterZero {
		cases[name] = at
	}
	for name, at := range cases {
		spec := movementSpec(t, domain.ArrivalFact)
		spec.OccurredAt = at
		if _, err := domain.RecordMovementFact(spec); !errors.Is(err, domain.ErrInvalidMovementFact) {
			t.Fatalf("%s：err = %v，期望 ErrInvalidMovementFact", name, err)
		}
	}
	for name, at := range storableNearZero {
		spec := movementSpec(t, domain.ArrivalFact)
		spec.OccurredAt = at
		if _, err := domain.RecordMovementFact(spec); err != nil {
			t.Fatalf("%s：err = %v，期望照常记录", name, err)
		}
	}
}

// Covers: 票 tf-face-algebra/01 修复方向 1——更正的发生时刻与更正时刻照同一判据。
func TestAMovementFactCorrectionAtTheZeroInstantOnTheRegistryIsRefused(t *testing.T) {
	fact, err := domain.RecordMovementFact(movementSpec(t, domain.ArrivalFact))
	if err != nil {
		t.Fatalf("记录事实：%v", err)
	}
	location := mustValue(t, domain.NewMovementLocationReference, "SYN-LOC-2")
	source := mustValue(t, domain.NewMovementSourceReference, "SYN-SOURCE-2")
	next := mustValue(t, domain.NewMovementFactVersion, "SYN-MF-V2")
	for name, at := range subMicroAfterZero {
		if _, err := fact.Correct(location, at, source, next, movementSeenAt); !errors.Is(err, domain.ErrInvalidMovementFact) {
			t.Fatalf("发生时刻 %s：err = %v，期望 ErrInvalidMovementFact", name, err)
		}
		if _, err := fact.Correct(location, movementSeenAt, source, next, at); !errors.Is(err, domain.ErrInvalidMovementFact) {
			t.Fatalf("更正时刻 %s：err = %v，期望 ErrInvalidMovementFact", name, err)
		}
	}
	at := zeroInstant.Add(time.Microsecond)
	if _, err := fact.Correct(location, at, source, next, movementSeenAt); err != nil {
		t.Fatalf("发生时刻零点后 1µs：%v", err)
	}
	if _, err := fact.Correct(location, movementSeenAt, source, next, at); err != nil {
		t.Fatalf("更正时刻零点后 1µs：%v", err)
	}
}
