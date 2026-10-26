package transportfulfillment_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证场外揽收采用、有效交付采用与面单终局判断（经首次有效收寄）三路在反查包裹之前先问节点作业单元册：对象那一刻是集运单元
// 就收工——不反查委托、不采用、不判终局；不是单元的按包裹反查；问不动是续办，答了却译不回是确定性的。单元册用替身（隔离合成 `S`），
// 真库与派发分区的证见 cmd/parcel-dispatch。

const unitObject = "SYN-BAG-1"

// unitLane 是三路各自的构造与观察口，让同一组断言跑在三路上。
type unitLane struct {
	name string
	// handle 用 object 造一封信、过这一路的真适配器，交回处理结果与「反查包裹的串」「下游（采用 / 判断）被调用的次数」
	// 「单元册被问到的次数与问法」。
	handle func(t *testing.T, book psports.ConsolidationUnitView, object string, targetFound bool) unitLaneOutcome
	// occurredAt 是这一路来源事实的业务发生时间：单元那一问带的时点。
	occurredAt time.Time
	// notFound 是这一路反查不中的哨兵。
	notFound error
}

type unitLaneOutcome struct {
	err           error
	lookedUp      string
	downstreamHit int
}

func unitLanes() []unitLane {
	return []unitLane{
		{
			name: "场外揽收", occurredAt: pickedUpAt, notFound: adapter.ErrParcelTargetNotFound,
			handle: func(t *testing.T, book psports.ConsolidationUnitView, object string, targetFound bool) unitLaneOutcome {
				adopter := &pickupAdopterDouble{}
				targets := &pickupTargetViewDouble{target: pickupTarget(t), found: targetFound}
				subject, err := adapter.NewAdoptOnOffsitePickupAdapter(
					&pickupRegistryDouble{record: registeredPickup(t, object), found: true}, targets, adopter, book)
				if err != nil {
					t.Fatalf("构造：%v", err)
				}
				reference := registeredRef()
				reference.Object = object
				err = subject.HandleRegisteredOffsitePickup(t.Context(), reference)
				return unitLaneOutcome{err: err, lookedUp: targets.parcel, downstreamHit: adopter.calls}
			},
		},
		{
			name: "有效交付", occurredAt: deliveredAt, notFound: adapter.ErrParcelTargetNotFound,
			handle: func(t *testing.T, book psports.ConsolidationUnitView, object string, targetFound bool) unitLaneOutcome {
				adopter := &deliveryAdopterDouble{}
				targets := &deliveryTargetViewDouble{target: deliveryTarget(t), found: targetFound}
				subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
					&deliveryStoreDouble{record: registeredDelivery(t, object), found: true}, targets, adopter, book)
				if err != nil {
					t.Fatalf("构造：%v", err)
				}
				reference := registeredDeliveryRef()
				reference.Object = object
				err = subject.HandleRegisteredEffectiveDelivery(t.Context(), reference)
				return unitLaneOutcome{err: err, lookedUp: targets.parcel, downstreamHit: adopter.calls}
			},
		},
		{
			name: "首次有效收寄", occurredAt: firstPickupAt, notFound: labelfinal.ErrParcelTargetNotFound,
			handle: func(t *testing.T, book psports.ConsolidationUnitView, object string, targetFound bool) unitLaneOutcome {
				f := newCarrierPickupFixtureOver(t, book)
				f.targets.found = targetFound
				f.put(recordOf(t, formedCarrierPickup(t, object)))
				reference := registeredCarrierPickupRef("CFEV-1")
				reference.Object = object
				err := f.handle(t, reference)
				return unitLaneOutcome{err: err, lookedUp: f.targets.parcel, downstreamHit: len(f.judge.commands) + len(f.adopter.commands)}
			},
		},
	}
}

// Covers: AT-PS-156、AT-PS-157、AT-PS-158——对象在节点作业单元册上是一袋：这一封收工（nil 即入账），不反查委托、不交采用 / 判断；
// 单元那一问带的是（信封租户，对象串，来源事实的业务发生时间），不是消费这一刻。
func TestACarriedConsolidationUnitIsAccountedForWithoutLookingUpAnyShipmentRequest(t *testing.T) {
	for _, lane := range unitLanes() {
		t.Run(lane.name, func(t *testing.T) {
			book := &unitBook{units: map[string]bool{unitObject: true}}

			outcome := lane.handle(t, book, unitObject, true)

			if outcome.err != nil {
				t.Fatalf("单元的信封没有收工：%v", outcome.err)
			}
			if outcome.lookedUp != "" {
				t.Fatalf("单元串被拿去反查了委托：%q", outcome.lookedUp)
			}
			if outcome.downstreamHit != 0 {
				t.Fatalf("单元走到了采用 / 判断 %d 次", outcome.downstreamHit)
			}
			if len(book.asked) != 1 ||
				book.asked[0] != (unitQuestion{tenant: "tenant-1", object: unitObject, at: lane.occurredAt}) {
				t.Fatalf("单元册的问法 = %+v, want 一次（tenant-1, %s, %s）", book.asked, unitObject, lane.occurredAt)
			}
		})
	}
}

// Covers: AT-PS-156、AT-PS-157、AT-PS-158 的反面——对象串长得像袋号（SYN-BAG-NOT-IN-BOOK）却不在单元册上，或就是个包裹号：
// 单元册问过了，答不是，按包裹反查委托；反查不中仍是各路的未决哨兵（委托可能还没落到已接受），反查命中走到采用 / 判断。
func TestAnObjectThatIsNotInTheUnitBookIsLookedUpAsAParcel(t *testing.T) {
	for _, lane := range unitLanes() {
		for _, object := range []string{"SYN-BAG-NOT-IN-BOOK", "parcel-1"} {
			t.Run(lane.name+"/"+object, func(t *testing.T) {
				book := &unitBook{units: map[string]bool{unitObject: true}}

				missing := lane.handle(t, book, object, false)
				if !errors.Is(missing.err, lane.notFound) {
					t.Fatalf("反查不中 err = %v, want %v——仍是未决", missing.err, lane.notFound)
				}
				if missing.lookedUp != object {
					t.Fatalf("反查包裹 = %q, want %q", missing.lookedUp, object)
				}
				if len(book.asked) != 1 || book.asked[0].object != object {
					t.Fatalf("单元册没有先被问：%+v", book.asked)
				}
			})
		}
	}
}

// Covers: AT-PS-159——单元册读不动是续办格：不收工、不反查（说不出是不是单元就不能拿去反查）、不走到采用 / 判断，哨兵是
// ErrConsolidationUnitNotVisible；它不是「不是单元」，也不是确定性的答复不一致。
func TestAnUnreadableUnitBookIsContinuableAndNeverLooksUpTheShipmentRequest(t *testing.T) {
	for _, lane := range unitLanes() {
		t.Run(lane.name, func(t *testing.T) {
			book := &unitBook{err: errors.New("unit book unavailable")}

			outcome := lane.handle(t, book, unitObject, true)

			if !errors.Is(outcome.err, adapter.ErrConsolidationUnitNotVisible) {
				t.Fatalf("err = %v, want ErrConsolidationUnitNotVisible", outcome.err)
			}
			if errors.Is(outcome.err, adapter.ErrConsolidationUnitAnswerInconsistent) {
				t.Fatal("读不动被登成了答复不一致")
			}
			if outcome.lookedUp != "" || outcome.downstreamHit != 0 {
				t.Fatalf("读不动仍反查了 %q / 走到采用判断 %d 次", outcome.lookedUp, outcome.downstreamHit)
			}
		})
	}
}

// Covers: AT-PS-159——单元册答了却译不回是确定性的：哨兵是 ErrConsolidationUnitAnswerInconsistent，不是读不动，也不放行去反查。
func TestAnUntranslatableUnitAnswerIsDeterministicAndNeverLooksUpTheShipmentRequest(t *testing.T) {
	for _, lane := range unitLanes() {
		t.Run(lane.name, func(t *testing.T) {
			book := &unitBook{err: errors.Join(errors.New("contradictory answer"), psports.ErrConsolidationUnitAnswerUntranslatable)}

			outcome := lane.handle(t, book, unitObject, true)

			if !errors.Is(outcome.err, adapter.ErrConsolidationUnitAnswerInconsistent) {
				t.Fatalf("err = %v, want ErrConsolidationUnitAnswerInconsistent", outcome.err)
			}
			if errors.Is(outcome.err, adapter.ErrConsolidationUnitNotVisible) {
				t.Fatal("答复不一致被登成了读不动：重投不自愈")
			}
			if outcome.lookedUp != "" || outcome.downstreamHit != 0 {
				t.Fatalf("答复不一致仍反查了 %q / 走到采用判断 %d 次", outcome.lookedUp, outcome.downstreamHit)
			}
		})
	}
}

// Covers: AT-PS-158——经首次有效收寄的一路，对象是单元时失效版本同样收工：单元不会有据前版形成的终局，「重派生归 PS owner、
// 尚未裁定」的显式未决（ErrVoidedCarrierPickupRederivationUndecided）与它无关，不拿去让单元的信封重投；对象是包裹的失效版本
// 仍停在那一格。待确认版本不论对象是什么都是交接口的缺陷，仍响亮失败，不被单元收走。
func TestAVoidedCarrierPickupOfAUnitIsAccountedForButAPendingOneStaysLoud(t *testing.T) {
	voidedOf := func(t *testing.T, object string) tfdomain.CarrierFirstEffectivePickup {
		t.Helper()
		voided, err := formedCarrierPickup(t, object).Void(tfdomain.CarrierPickupVoiding{
			Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
			JudgedAt: pickupJudgedAt.Add(time.Minute),
			Bases:    carrierPickupBases(t, "v2"),
		})
		if err != nil {
			t.Fatalf("void: %v", err)
		}
		return voided
	}

	t.Run("单元的失效版本收工", func(t *testing.T) {
		book := &unitBook{units: map[string]bool{unitObject: true}}
		f := newCarrierPickupFixtureOver(t, book)
		f.put(recordOf(t, voidedOf(t, unitObject)))
		reference := registeredCarrierPickupRef("CFEV-2")
		reference.Object = unitObject

		if err := f.handle(t, reference); err != nil {
			t.Fatalf("单元的失效版本没有收工：%v", err)
		}
		if len(book.asked) != 1 || book.asked[0].object != unitObject || !book.asked[0].at.Equal(pickupJudgedAt.Add(time.Minute)) {
			t.Fatalf("单元册的问法 = %+v, want 一次、时点取失效版本的判断时间", book.asked)
		}
		if len(f.judge.commands) != 0 || len(f.adopter.commands) != 0 {
			t.Fatal("单元的失效版本走到了判断 / 采用")
		}
	})

	t.Run("包裹的失效版本仍是显式未决", func(t *testing.T) {
		book := &unitBook{units: map[string]bool{unitObject: true}}
		f := newCarrierPickupFixtureOver(t, book)
		f.put(recordOf(t, voidedOf(t, "parcel-1")))

		if err := f.handle(t, registeredCarrierPickupRef("CFEV-2")); !errors.Is(err, adapter.ErrVoidedCarrierPickupRederivationUndecided) {
			t.Fatalf("err = %v, want ErrVoidedCarrierPickupRederivationUndecided", err)
		}
	})

	t.Run("单元的失效版本问不动仍是续办", func(t *testing.T) {
		f := newCarrierPickupFixtureOver(t, &unitBook{err: errors.New("unit book unavailable")})
		f.put(recordOf(t, voidedOf(t, unitObject)))
		reference := registeredCarrierPickupRef("CFEV-2")
		reference.Object = unitObject

		if err := f.handle(t, reference); !errors.Is(err, adapter.ErrConsolidationUnitNotVisible) {
			t.Fatalf("err = %v, want ErrConsolidationUnitNotVisible", err)
		}
	})

	t.Run("单元的待确认版本仍响亮", func(t *testing.T) {
		book := &unitBook{units: map[string]bool{unitObject: true}}
		f := newCarrierPickupFixtureOver(t, book)
		pending, err := tfdomain.HoldCarrierFirstEffectivePickupPending(tfdomain.PendingCarrierFirstEffectivePickupSpec{
			TenantID: value(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   value(t, tfdomain.NewCarriedObjectReference, unitObject),
			Fact:     value(t, tfdomain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
			Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
			Reason:   tfdomain.PickupCarrierIdentityNotRegistered,
			Material: "ACME EXPRESS",
			JudgedAt: pickupJudgedAt,
			Bases:    carrierPickupBases(t, "v2"),
		})
		if err != nil {
			t.Fatalf("hold pending: %v", err)
		}
		f.put(recordOf(t, pending))
		reference := registeredCarrierPickupRef("CFEV-2")
		reference.Object = unitObject

		if err := f.handle(t, reference); !errors.Is(err, adapter.ErrCarrierPickupRecordInconsistent) {
			t.Fatalf("err = %v, want ErrCarrierPickupRecordInconsistent", err)
		}
		if len(book.asked) != 0 {
			t.Fatalf("待确认版本还去问了单元册：%+v", book.asked)
		}
	})
}

func TestTheOffsiteAndDeliveryAdaptersRefuseToBeBuiltWithoutTheUnitBook(t *testing.T) {
	if _, err := adapter.NewAdoptOnOffsitePickupAdapter(
		&pickupRegistryDouble{}, &pickupTargetViewDouble{}, &pickupAdopterDouble{}, nil); err == nil {
		t.Fatal("场外揽收一路缺单元册读口却构造成功：装配漏接会让每一封单元的揽收信都去反查委托")
	}
	if _, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{}, &deliveryTargetViewDouble{}, &deliveryAdopterDouble{}, nil); err == nil {
		t.Fatal("有效交付一路缺单元册读口却构造成功：装配漏接会让每一封单元的交付信都去反查委托")
	}
}
