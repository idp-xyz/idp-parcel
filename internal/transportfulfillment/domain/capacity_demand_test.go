package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证 ADR-0203 决定三至五在领域上的半边：预占与释放都声明数量、单位与来源；单位不换算；硬上限在预占整段里每一刻
// 都守；释放有身份、按时点归还。夹具全为合成值（S 级），计数锚定容量 100 kg 的夹具池。

// Covers: ADR-0203 决定三——来源种类是封闭词，大小写不折；数量不为正、单位缺、来源缺都不成需求量。
func TestCapacityDemandIsDeclaredWithUnitAndSource(t *testing.T) {
	for _, word := range []string{"OPERATOR_DECLARATION", "PREPARATION_DEMAND_SNAPSHOT"} {
		kind, err := domain.ParseCapacityDemandSourceKind(word)
		if err != nil || kind.String() != word {
			t.Fatalf("%s 认回 = %q %v", word, kind, err)
		}
	}
	for _, word := range []string{"", "operator_declaration", "MEMBER_COUNT", "OPERATOR_DECLARATION "} {
		if _, err := domain.ParseCapacityDemandSourceKind(word); !errors.Is(err, domain.ErrInvalidCapacityDemand) {
			t.Fatalf("%q 应拒：%v", word, err)
		}
	}

	reference := mustValue(t, domain.NewCapacityDemandSourceReference, "SYN-DEMAND-DECL-1/v1")
	if _, err := domain.NewCapacityDemandSource(domain.CapacityDemandSourceInvalid, reference); !errors.Is(err, domain.ErrInvalidCapacityDemand) {
		t.Fatalf("无种类的来源应拒：%v", err)
	}
	if _, err := domain.NewCapacityDemandSource(domain.CapacityDemandOperatorDeclaration, domain.CapacityDemandSourceReference{}); !errors.Is(err, domain.ErrInvalidCapacityDemand) {
		t.Fatalf("无引用的来源应拒：%v", err)
	}
	source := declaredDemand(t, 1).Source()
	kg := mustValue(t, domain.NewQuantityUnitReference, "kg")
	for name, build := range map[string]func() error{
		"zero quantity":     func() error { _, err := domain.NewCapacityDemand(0, kg, source); return err },
		"negative quantity": func() error { _, err := domain.NewCapacityDemand(-5, kg, source); return err },
		"no unit": func() error {
			_, err := domain.NewCapacityDemand(5, domain.QuantityUnitReference{}, source)
			return err
		},
		"no source": func() error { _, err := domain.NewCapacityDemand(5, kg, domain.CapacityDemandSource{}); return err },
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidCapacityDemand) {
			t.Fatalf("%s: err = %v, want ErrInvalidCapacityDemand", name, err)
		}
	}
}

// Covers: ADR-0203 决定三与 CONTEXT「只有明确业务规则存在时才能形成可追溯换算」——声明的单位不是池的单位，预占与释放
// 都拒，不换算、不按大小写归一；单位只有在池上才判得出，所以这一格在池上而不在需求量的构造门上。
func TestDeclaredUnitMustBeThePoolUnit(t *testing.T) {
	pool := establishedPool(t, 100)
	for _, unit := range []string{"t", "KG", "lb"} {
		if _, err := pool.Reserve(reservationRef(t, "reservation-1"), demandIn(t, 10, unit), reservedUntil, reservedAt); !errors.Is(err, domain.ErrCapacityUnitMismatch) {
			t.Fatalf("单位 %s 预占：err = %v, want ErrCapacityUnitMismatch", unit, err)
		}
	}
	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), declaredDemand(t, 10), reservedUntil, reservedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Release(releaseRef(t, "release-1"), reservationRef(t, "reservation-1"), demandIn(t, 5, "KG"), reservedAt.Add(time.Hour)); !errors.Is(err, domain.ErrCapacityUnitMismatch) {
		t.Fatalf("单位 KG 释放：err = %v, want ErrCapacityUnitMismatch", err)
	}
}

// Covers: ADR-0203 决定四与 `AT-TF-031`——硬上限在预占的整段里每一刻都守。一份声明在别的预占过期之后、有效期却跨进
// 另一份预占时段的预占，按判断时点那一刻的余量会被放行，按整段最紧那一刻就拒；拒时报的余量是那一刻的。
func TestHardCapacityHoldsThroughTheWholeReservationWindow(t *testing.T) {
	pool := establishedPool(t, 100)
	morning := reservedAt
	pool, err := pool.Reserve(reservationRef(t, "evening"), declaredDemand(t, 70), reservedUntil.Add(4*time.Hour), reservedUntil)
	if err != nil {
		t.Fatalf("先占傍晚那一段：%v", err)
	}
	if available := pool.AvailableAt(morning); available != 100 {
		t.Fatalf("早上的余量 = %d, want 100（傍晚的预占在它的判断时点之前不占）", available)
	}
	if available := pool.AvailableThrough(morning, reservedUntil.Add(time.Hour)); available != 30 {
		t.Fatalf("早上到傍晚之后这一段最紧的余量 = %d, want 30", available)
	}
	if _, err := pool.Reserve(reservationRef(t, "all-day"), declaredDemand(t, 50), reservedUntil.Add(time.Hour), morning); !errors.Is(err, domain.ErrCapacityExceeded) {
		t.Fatalf("跨进傍晚的 50：err = %v, want ErrCapacityExceeded（70+50 > 100）", err)
	}
	if _, err := pool.Reserve(reservationRef(t, "morning-only"), declaredDemand(t, 100), reservedUntil.Add(-time.Hour), morning); err != nil {
		t.Fatalf("傍晚之前就到期的 100 应放行：%v", err)
	}

	t.Run("a reservation declared later still counts from its instant on", func(t *testing.T) {
		later := establishedPool(t, 100)
		later, err := later.Reserve(reservationRef(t, "later"), declaredDemand(t, 80), reservedUntil.Add(48*time.Hour), reservedUntil.Add(24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		// 判断时点在今天、有效期跨到后天：后天那 80 已经占着，今天只看今天的余量会放行 30。
		if _, err := later.Reserve(reservationRef(t, "today"), declaredDemand(t, 30), reservedUntil.Add(30*time.Hour), reservedAt); !errors.Is(err, domain.ErrCapacityExceeded) {
			t.Fatalf("err = %v, want ErrCapacityExceeded", err)
		}
	})
}

// Covers: ADR-0203 决定五——释放有身份、在池内唯一：同一释放引用不论落在哪一份预占上都只认一次；释放从它的时点起归还，
// 之前那段仍占着；释放早于预占的判断时点答预占尚未生效，不是坏载荷。
func TestReleasesHaveIdentityAndReturnCapacityFromTheirInstant(t *testing.T) {
	pool := establishedPool(t, 100)
	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), declaredDemand(t, 60), reservedUntil, reservedAt)
	if err != nil {
		t.Fatal(err)
	}
	pool, err = pool.Reserve(reservationRef(t, "reservation-2"), declaredDemand(t, 20), reservedUntil, reservedAt)
	if err != nil {
		t.Fatal(err)
	}
	releasedAt := reservedAt.Add(2 * time.Hour)
	pool, err = pool.Release(releaseRef(t, "release-1"), reservationRef(t, "reservation-1"), declaredDemand(t, 25), releasedAt)
	if err != nil {
		t.Fatal(err)
	}
	if before, after := pool.AvailableAt(releasedAt.Add(-time.Minute)), pool.AvailableAt(releasedAt); before != 20 || after != 45 {
		t.Fatalf("释放前后余量 = %d / %d, want 20 / 45（释放从它的时点起归还）", before, after)
	}
	on, release, found := pool.ReleaseFor(releaseRef(t, "release-1"))
	if !found || on != reservationRef(t, "reservation-1") || release.Quantity() != 25 || !release.ReleasedAt().Equal(releasedAt) ||
		release.Source() != declaredDemand(t, 1).Source() || !release.Declares(declaredDemand(t, 25), releasedAt) {
		t.Fatalf("ReleaseFor = %v %+v %v", on, release, found)
	}
	if release.Declares(declaredDemand(t, 26), releasedAt) || release.Declares(declaredDemand(t, 25), releasedAt.Add(time.Second)) {
		t.Fatal("数量或时点不同的释放被认成了同一次")
	}

	for _, target := range []string{"reservation-1", "reservation-2"} {
		if _, err := pool.Release(releaseRef(t, "release-1"), reservationRef(t, target), declaredDemand(t, 1), releasedAt.Add(time.Hour)); !errors.Is(err, domain.ErrDuplicateRelease) {
			t.Fatalf("同一释放引用再落到 %s：err = %v, want ErrDuplicateRelease", target, err)
		}
	}
	if _, err := pool.Release(releaseRef(t, "release-early"), reservationRef(t, "reservation-2"), declaredDemand(t, 1), reservedAt.Add(-time.Minute)); !errors.Is(err, domain.ErrReservationNotYetEffective) {
		t.Fatalf("早于预占判断时点的释放：err = %v, want ErrReservationNotYetEffective", err)
	}
	reservation, _ := pool.ReservationFor(reservationRef(t, "reservation-1"))
	if releases := reservation.Releases(); len(releases) != 1 || releases[0].Reference() != releaseRef(t, "release-1") {
		t.Fatalf("预占上的释放 = %+v", releases)
	}
}

// Covers: 重放的整份比对——在册预占与这次声明数量、单位、来源、判断时点、有效期任一格不同都不是同一份。
func TestReservationDeclaresComparesTheWholeDecision(t *testing.T) {
	pool := establishedPool(t, 100)
	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), declaredDemand(t, 60), reservedUntil, reservedAt)
	if err != nil {
		t.Fatal(err)
	}
	reservation, _ := pool.ReservationFor(reservationRef(t, "reservation-1"))
	if !reservation.Declares(declaredDemand(t, 60), reservedUntil, reservedAt) {
		t.Fatal("同一份声明没认出来")
	}
	if !reservation.ReservedAt().Equal(reservedAt) || reservation.Source() != declaredDemand(t, 1).Source() {
		t.Fatalf("判断时点 / 来源 = %v / %+v", reservation.ReservedAt(), reservation.Source())
	}
	otherSource, err := domain.NewCapacityDemandSource(domain.CapacityDemandPreparationSnapshot,
		mustValue(t, domain.NewCapacityDemandSourceReference, "SYN-PREP-1"))
	if err != nil {
		t.Fatal(err)
	}
	fromPreparation, err := domain.NewCapacityDemand(60, mustValue(t, domain.NewQuantityUnitReference, "kg"), otherSource)
	if err != nil {
		t.Fatal(err)
	}
	for name, differs := range map[string]bool{
		"quantity":    reservation.Declares(declaredDemand(t, 61), reservedUntil, reservedAt),
		"unit":        reservation.Declares(demandIn(t, 60, "KG"), reservedUntil, reservedAt),
		"source":      reservation.Declares(fromPreparation, reservedUntil, reservedAt),
		"valid until": reservation.Declares(declaredDemand(t, 60), reservedUntil.Add(time.Second), reservedAt),
		"instant":     reservation.Declares(declaredDemand(t, 60), reservedUntil, reservedAt.Add(time.Second)),
	} {
		if differs {
			t.Fatalf("%s 不同却被认成同一份", name)
		}
	}
	// 登记册截到微秒：差在微秒以下的两个时刻在册上是同一个，读回的那一份仍要认得出原样重交。
	if !reservation.Declares(declaredDemand(t, 60), reservedUntil.Add(400*time.Nanosecond), reservedAt.Add(400*time.Nanosecond)) {
		t.Fatal("微秒以下的差被当成了不同的声明")
	}
}

// Covers: 重建门核释放明细——已释放列必须等于逐次释放之和；释放引用在池内不重；释放早于预占判断时点、预占缺来源或判断
// 时点、有效期不晚于判断时点都拒。
func TestRehydrateCapacityPoolChecksReleaseDetails(t *testing.T) {
	spec := domain.CapacityPoolSpec{
		TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
		Schedule:  mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Dimension: domain.CapacityWeight,
		Unit:      mustValue(t, domain.NewQuantityUnitReference, "kg"),
		Capacity:  100,
	}
	source := declaredDemand(t, 1).Source()
	snapshot := func(reference string, released int64, releases ...domain.CapacityReleaseSnapshot) domain.CapacityReservationSnapshot {
		return domain.CapacityReservationSnapshot{
			Reference: reservationRef(t, reference), Quantity: 50, Released: released, ValidUntil: reservedUntil,
			Source: source, ReservedAt: reservedAt, Releases: releases,
		}
	}
	release := func(reference string, quantity int64, at time.Time) domain.CapacityReleaseSnapshot {
		return domain.CapacityReleaseSnapshot{Reference: releaseRef(t, reference), Quantity: quantity, Source: source, ReleasedAt: at}
	}
	later := reservedAt.Add(time.Hour)

	pool, err := domain.RehydrateCapacityPool(spec, []domain.CapacityReservationSnapshot{snapshot("r-1", 10, release("rel-1", 10, later))})
	if err != nil {
		t.Fatal(err)
	}
	if again := pool.ReservationSnapshots(); len(again) != 1 || len(again[0].Releases) != 1 || again[0].Releases[0].Quantity != 10 {
		t.Fatalf("往返后的快照 = %+v", again)
	}

	cases := map[string]struct {
		snapshots []domain.CapacityReservationSnapshot
		want      error
	}{
		"released column disagrees with the releases": {[]domain.CapacityReservationSnapshot{snapshot("r-1", 15, release("rel-1", 10, later))}, domain.ErrReservationOverdrawn},
		"release reference repeats across reservations": {[]domain.CapacityReservationSnapshot{
			snapshot("r-1", 10, release("rel-1", 10, later)), snapshot("r-2", 5, release("rel-1", 5, later)),
		}, domain.ErrDuplicateRelease},
		"release before the reservation instant": {[]domain.CapacityReservationSnapshot{snapshot("r-1", 10, release("rel-1", 10, reservedAt.Add(-time.Second)))}, domain.ErrInvalidCapacityPool},
		"reservation without source": {[]domain.CapacityReservationSnapshot{func() domain.CapacityReservationSnapshot {
			bad := snapshot("r-1", 0)
			bad.Source = domain.CapacityDemandSource{}
			return bad
		}()}, domain.ErrInvalidCapacityPool},
		"reservation without instant": {[]domain.CapacityReservationSnapshot{func() domain.CapacityReservationSnapshot {
			bad := snapshot("r-1", 0)
			bad.ReservedAt = time.Time{}
			return bad
		}()}, domain.ErrInvalidCapacityPool},
		"valid until not after the instant": {[]domain.CapacityReservationSnapshot{func() domain.CapacityReservationSnapshot {
			bad := snapshot("r-1", 0)
			bad.ValidUntil = reservedAt
			return bad
		}()}, domain.ErrInvalidCapacityPool},
	}
	for name, testCase := range cases {
		if _, err := domain.RehydrateCapacityPool(spec, testCase.snapshots); !errors.Is(err, testCase.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, testCase.want)
		}
	}
}
