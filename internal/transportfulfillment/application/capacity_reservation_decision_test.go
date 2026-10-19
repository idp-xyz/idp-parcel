package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证预占与释放两种运营决定在编排上的分格与次序（ADR-0203 决定三至六）。夹具池容量 100 kg、身份 schedule-1/WEIGHT，
// 计数都锚定它；夹具全为合成值（S 级）。

// Covers: `AT-TF-136`——预占与释放都要声明数量、单位与来源；缺一格、来源种类不在封闭词里、数量不为正、有效期不晚于判断
// 时点都答未受理，且不读池（池册读不动也答未受理，证明没走到读）。
func TestCapacityDecisionsRequireADeclaredDemand(t *testing.T) {
	fixture := newOpportunityFixture(t)
	fixture.pools.findErr = errors.New("store down")
	reserveBroken := map[string]func(*application.ReserveCapacityCommand){
		"no unit":                  func(command *application.ReserveCapacityCommand) { command.Unit = "" },
		"no source kind":           func(command *application.ReserveCapacityCommand) { command.SourceKind = "" },
		"source kind outside list": func(command *application.ReserveCapacityCommand) { command.SourceKind = "MEMBER_COUNT" },
		"no source reference":      func(command *application.ReserveCapacityCommand) { command.SourceReference = " " },
		"zero quantity":            func(command *application.ReserveCapacityCommand) { command.Quantity = 0 },
		"no reservation reference": func(command *application.ReserveCapacityCommand) { command.Reservation = "" },
		"no instant":               func(command *application.ReserveCapacityCommand) { command.At = time.Time{} },
		"window not after instant": func(command *application.ReserveCapacityCommand) { command.ValidUntil = command.At },
	}
	for name, breakCommand := range reserveBroken {
		command := reserveCommand(t, 10)
		breakCommand(&command)
		result, err := fixture.handler.Reserve(context.Background(), command)
		if err != nil || result.Outcome() != application.OpportunityNotAccepted {
			t.Fatalf("reserve %s: %q %v, want INPUT_NOT_ACCEPTED", name, result.Outcome(), err)
		}
	}
	releaseBroken := map[string]func(*application.ReleaseCapacityCommand){
		"no unit":              func(command *application.ReleaseCapacityCommand) { command.Unit = "" },
		"no source reference":  func(command *application.ReleaseCapacityCommand) { command.SourceReference = "" },
		"no release reference": func(command *application.ReleaseCapacityCommand) { command.Release = "" },
		"negative quantity":    func(command *application.ReleaseCapacityCommand) { command.Quantity = -1 },
		"no instant":           func(command *application.ReleaseCapacityCommand) { command.At = time.Time{} },
	}
	for name, breakCommand := range releaseBroken {
		command := releaseCommand(t, "release-1", 10, reserveAt.Add(time.Hour))
		breakCommand(&command)
		result, err := fixture.handler.Release(context.Background(), command)
		if err != nil || result.Outcome() != application.OpportunityNotAccepted {
			t.Fatalf("release %s: %q %v, want INPUT_NOT_ACCEPTED", name, result.Outcome(), err)
		}
	}
	if result, _ := fixture.handler.Reserve(context.Background(), reserveCommand(t, 10)); result.Outcome() != application.OpportunityUndecided {
		t.Fatalf("载荷齐全时池册读不动应答未决：%q", result.Outcome())
	}
}

// Covers: `AT-TF-137`——池不在本租户册上（没登过、或只登在他租户名下）答池不在册；释放指名的预占不在那个池上答预占不在册。
// 两格都不写。
func TestCapacityDecisionsAnswerMissingPoolsAndReservations(t *testing.T) {
	fixture := newOpportunityFixture(t)
	result, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 10))
	if err != nil || result.Outcome() != application.PoolNotFound {
		t.Fatalf("没登过的池：%q %v", result.Outcome(), err)
	}
	establishedPool(t, fixture)
	elsewhere := reserveCommand(t, 10)
	elsewhere.TenantID = mustTenant(t, "tenant-2")
	if result, _ := fixture.handler.Reserve(context.Background(), elsewhere); result.Outcome() != application.PoolNotFound {
		t.Fatalf("他租户读这个池：%q, want POOL_NOT_FOUND", result.Outcome())
	}
	if result, _ := fixture.handler.Release(context.Background(), releaseCommand(t, "release-1", 5, reserveAt.Add(time.Hour))); result.Outcome() != application.ReservationNotFound {
		t.Fatalf("池在、预占不在：%q, want RESERVATION_NOT_FOUND", result.Outcome())
	} else if _, present := result.Pool(); !present {
		t.Fatal("预占不在册时应交回那个池")
	}
	if fixture.pools.replaces != 0 {
		t.Fatalf("replaces = %d, want 0", fixture.pools.replaces)
	}
}

// Covers: `AT-TF-138`——声明的单位不是池的单位，预占与释放都答单位不符并交回池（池上写着它的单位），不换算、不写。
func TestCapacityDecisionsRefuseAnotherUnit(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	inTonnes := reserveCommand(t, 1)
	inTonnes.Unit = "t"
	result, err := fixture.handler.Reserve(context.Background(), inTonnes)
	if err != nil || result.Outcome() != application.DemandUnitMismatch {
		t.Fatalf("吨预占千克池：%q %v", result.Outcome(), err)
	}
	if record, present := result.Pool(); !present || record.Pool.Unit().String() != "kg" {
		t.Fatalf("单位不符应交回池：%+v %v", record, present)
	}
	if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); err != nil {
		t.Fatal(err)
	}
	release := releaseCommand(t, "release-1", 5, reserveAt.Add(time.Hour))
	release.Unit = "KG"
	if result, _ := fixture.handler.Release(context.Background(), release); result.Outcome() != application.DemandUnitMismatch {
		t.Fatalf("释放单位按字面比：%q, want UNIT_MISMATCH", result.Outcome())
	}
	if fixture.pools.replaces != 1 {
		t.Fatalf("replaces = %d, want 1（只有那一次合格的预占落了）", fixture.pools.replaces)
	}
}

// Covers: `AT-TF-139` 与 `AT-TF-031`——超出硬上限答容量超出，交回池并带预占整段最紧那一刻的余量；命令上没有任何
// 越过上限的格，审批也越不过去。
func TestOverReservationCarriesTheTightestRemainderAndHasNoOverride(t *testing.T) {
	commandType := reflect.TypeOf(application.ReserveCapacityCommand{})
	for index := 0; index < commandType.NumField(); index++ {
		name := strings.ToLower(commandType.Field(index).Name)
		for _, forbidden := range []string{"approv", "override", "force", "bypass", "authoriz"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("ReserveCapacityCommand 带 %q——硬上限就有了越过的入口", commandType.Field(index).Name)
			}
		}
	}

	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	tomorrow := reserveCommand(t, 70)
	tomorrow.Reservation = "tomorrow"
	tomorrow.At = reserveValidUntil.Add(-2 * time.Hour)
	tomorrow.ValidUntil = reserveValidUntil.Add(24 * time.Hour)
	if result, _ := fixture.handler.Reserve(context.Background(), tomorrow); result.Outcome() != application.CapacityReserved {
		t.Fatalf("先占明天那一段：%q", result.Outcome())
	}
	today := reserveCommand(t, 50)
	result, err := fixture.handler.Reserve(context.Background(), today)
	if err != nil || result.Outcome() != application.CapacityExceededOutcome {
		t.Fatalf("今天起、跨进明天的 50：%q %v, want CAPACITY_EXCEEDED", result.Outcome(), err)
	}
	// 判断时点那一刻余 100，跨进明天那一刻只余 30：报的是整段里最紧的那一刻。
	if result.AvailableQuantity() != 30 {
		t.Fatalf("available = %d, want 30", result.AvailableQuantity())
	}
	if _, present := result.Pool(); !present {
		t.Fatal("容量超出应交回池")
	}
}

// Covers: `AT-TF-140`——同一预占引用原样重交答已有预占并交回那一份，池此刻占满也一样，不重扣；数量、单位、来源、判断
// 时点、有效期任一格不同答预占冲突，交回在册那一份，不改写。
func TestReservationReplayAndConflictEchoTheRegisteredReservation(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	if result, _ := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); result.Outcome() != application.CapacityReserved {
		t.Fatalf("首次预占：%q", result.Outcome())
	} else if reservation, present := result.Reservation(); !present || reservation.Reference().String() != "reservation-1" ||
		reservation.Source().Kind().String() != "OPERATOR_DECLARATION" || !reservation.ReservedAt().Equal(reserveAt) {
		t.Fatalf("预占应交回那一份：%+v %v", reservation, present)
	}
	fill := reserveCommand(t, 40)
	fill.Reservation = "reservation-2"
	if result, _ := fixture.handler.Reserve(context.Background(), fill); result.Outcome() != application.CapacityReserved {
		t.Fatalf("占满：%q", result.Outcome())
	}

	replay, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60))
	if err != nil || replay.Outcome() != application.ReservationExistingResult {
		t.Fatalf("池满时原样重交：%q %v, want EXISTING_RESERVATION", replay.Outcome(), err)
	}
	if reservation, present := replay.Reservation(); !present || reservation.Reference().String() != "reservation-1" {
		t.Fatalf("重放应交回原预占：%v", present)
	}
	replaces := fixture.pools.replaces

	conflicts := map[string]func(*application.ReserveCapacityCommand){
		"quantity":    func(command *application.ReserveCapacityCommand) { command.Quantity = 59 },
		"source kind": func(command *application.ReserveCapacityCommand) { command.SourceKind = "PREPARATION_DEMAND_SNAPSHOT" },
		"source":      func(command *application.ReserveCapacityCommand) { command.SourceReference = "SYN-DEMAND-DECL-1/v9" },
		"unit":        func(command *application.ReserveCapacityCommand) { command.Unit = "KG" },
		"instant":     func(command *application.ReserveCapacityCommand) { command.At = command.At.Add(time.Minute) },
		"valid until": func(command *application.ReserveCapacityCommand) {
			command.ValidUntil = command.ValidUntil.Add(time.Minute)
		},
	}
	for name, change := range conflicts {
		command := reserveCommand(t, 60)
		change(&command)
		result, err := fixture.handler.Reserve(context.Background(), command)
		if err != nil || result.Outcome() != application.ReservationConflict {
			t.Fatalf("%s 不同：%q %v, want RESERVATION_CONFLICT", name, result.Outcome(), err)
		}
		if reservation, present := result.Reservation(); !present {
			t.Fatalf("%s：冲突应交回在册那一份", name)
		} else if reserved, _, _ := reservation.Quantities(); reserved != 60 {
			t.Fatalf("%s：交回的不是在册那一份（%d）", name, reserved)
		}
	}
	if fixture.pools.replaces != replaces {
		t.Fatalf("重放与冲突落了 %d 次写", fixture.pools.replaces-replaces)
	}
}

// Covers: `AT-TF-141`——同一释放引用原样重交答已有释放，不再还一份；改了数量、来源、时点或挪到另一份预占上答释放冲突，
// 交回在册那一次与它所落的预占。
func TestReleaseReplayDoesNotReturnCapacityTwice(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); err != nil {
		t.Fatal(err)
	}
	other := reserveCommand(t, 10)
	other.Reservation = "reservation-2"
	if _, err := fixture.handler.Reserve(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	releasedAt := reserveAt.Add(time.Hour)
	released, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-1", 20, releasedAt))
	if err != nil || released.Outcome() != application.CapacityReleased {
		t.Fatalf("释放：%q %v", released.Outcome(), err)
	}
	if on, release, present := released.Release(); !present || on.String() != "reservation-1" || release.Quantity() != 20 {
		t.Fatalf("释放应交回那一次：%v %+v %v", on, release, present)
	}

	for attempt := 0; attempt < 2; attempt++ {
		replay, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-1", 20, releasedAt))
		if err != nil || replay.Outcome() != application.ReleaseExistingResult {
			t.Fatalf("原样重交：%q %v, want EXISTING_RELEASE", replay.Outcome(), err)
		}
		record, _ := replay.Pool()
		reservation, _ := record.Pool.ReservationFor(mustReservation(t, "reservation-1"))
		if _, releasedQuantity, _ := reservation.Quantities(); releasedQuantity != 20 {
			t.Fatalf("重放又还了一份：已释放 %d", releasedQuantity)
		}
	}

	conflicts := map[string]func(*application.ReleaseCapacityCommand){
		"quantity":            func(command *application.ReleaseCapacityCommand) { command.Quantity = 21 },
		"source":              func(command *application.ReleaseCapacityCommand) { command.SourceReference = "SYN-DEMAND-DECL-1/v3" },
		"instant":             func(command *application.ReleaseCapacityCommand) { command.At = command.At.Add(time.Second) },
		"another reservation": func(command *application.ReleaseCapacityCommand) { command.Reservation = "reservation-2" },
	}
	for name, change := range conflicts {
		command := releaseCommand(t, "release-1", 20, releasedAt)
		change(&command)
		result, err := fixture.handler.Release(context.Background(), command)
		if err != nil || result.Outcome() != application.ReleaseConflict {
			t.Fatalf("%s 不同：%q %v, want RELEASE_CONFLICT", name, result.Outcome(), err)
		}
		if on, release, present := result.Release(); !present || on.String() != "reservation-1" || release.Quantity() != 20 {
			t.Fatalf("%s：冲突应交回在册那一次：%v %+v %v", name, on, release, present)
		}
	}
}

// Covers: `AT-TF-142`——释放量连同已释放、已消耗超过预占量答守恒撞线，释放时点早于预占的判断时点答预占尚未生效，预占过期
// 答预占过期；三格都交回那一份预占（判断时点、有效期与三量在上面，声明方据此看该在哪一段、还能释放多少），不写。尚未生效
// 不是未受理：载荷单看成立，要读过池、对着那一份预占才判得出。
func TestReleaseRefusalsEchoTheReservation(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); err != nil {
		t.Fatal(err)
	}
	if result, _ := fixture.handler.Release(context.Background(), releaseCommand(t, "release-1", 50, reserveAt.Add(time.Hour))); result.Outcome() != application.CapacityReleased {
		t.Fatalf("释放 50：%q", result.Outcome())
	}
	replaces := fixture.pools.replaces
	over, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-2", 11, reserveAt.Add(2*time.Hour)))
	if err != nil || over.Outcome() != application.ReservationOverdrawnOutcome {
		t.Fatalf("再释放 11：%q %v, want RESERVATION_OVERDRAWN", over.Outcome(), err)
	}
	if reservation, present := over.Reservation(); !present {
		t.Fatal("守恒撞线应交回那一份预占")
	} else if reserved, released, consumed := reservation.Quantities(); reserved-released-consumed != 10 {
		t.Fatalf("还能释放 %d，want 10", reserved-released-consumed)
	}
	expired, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-3", 1, reserveValidUntil.Add(time.Hour)))
	if err != nil || expired.Outcome() != application.ReservationExpiredOutcome {
		t.Fatalf("过期后释放：%q %v, want RESERVATION_EXPIRED", expired.Outcome(), err)
	}
	if _, present := expired.Reservation(); !present {
		t.Fatal("预占过期应交回那一份预占")
	}
	early, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-4", 1, reserveAt.Add(-time.Minute)))
	if err != nil || early.Outcome() != application.ReservationNotYetEffectiveOutcome {
		t.Fatalf("早于判断时点释放：%q %v, want RESERVATION_NOT_YET_EFFECTIVE", early.Outcome(), err)
	}
	if reservation, present := early.Reservation(); !present || !reservation.ReservedAt().Equal(reserveAt) {
		t.Fatal("预占尚未生效应交回那一份预占，判断时点在上面")
	}
	if fixture.pools.replaces != replaces {
		t.Fatalf("拒绝落了 %d 次写", fixture.pools.replaces-replaces)
	}
}

func mustReservation(t *testing.T, raw string) domain.CapacityReservationReference {
	t.Helper()
	reference, err := domain.NewCapacityReservationReference(raw)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

// Covers: 判断与落册用同一份截到登记册精度的时刻——带亚微秒的判断时点、有效期与释放时点，首答交回的已是截过的值，
// 与库里读回的那一份相同，重放回显也就与首答一致。
func TestCapacityDecisionsEchoInstantsAtRegistryPrecision(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	reserve := reserveCommand(t, 10)
	reserve.At = reserve.At.Add(400 * time.Nanosecond)
	reserve.ValidUntil = reserve.ValidUntil.Add(700 * time.Nanosecond)
	reserved, err := fixture.handler.Reserve(context.Background(), reserve)
	if err != nil || reserved.Outcome() != application.CapacityReserved {
		t.Fatalf("预占：%q %v", reserved.Outcome(), err)
	}
	reservation, _ := reserved.Reservation()
	if !reservation.ReservedAt().Equal(domain.AtRegistryPrecision(reserve.At)) ||
		!reservation.ValidUntil().Equal(domain.AtRegistryPrecision(reserve.ValidUntil)) {
		t.Fatalf("首答时刻 = %s / %s，want 截到登记册精度", reservation.ReservedAt(), reservation.ValidUntil())
	}
	release := releaseCommand(t, "release-1", 1, reserveAt.Add(time.Hour+300*time.Nanosecond))
	released, err := fixture.handler.Release(context.Background(), release)
	if err != nil || released.Outcome() != application.CapacityReleased {
		t.Fatalf("释放：%q %v", released.Outcome(), err)
	}
	if _, landed, _ := released.Release(); !landed.ReleasedAt().Equal(domain.AtRegistryPrecision(release.At)) {
		t.Fatalf("首答释放时点 = %s，want 截到登记册精度", landed.ReleasedAt())
	}
}
