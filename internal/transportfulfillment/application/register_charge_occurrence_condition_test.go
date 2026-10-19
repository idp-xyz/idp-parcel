package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证发生条件按采用协议的声明判（票 demo-chain-gaps/29，ADR-0209）：协议没声明这个原因答未配置、声明不计费答
// 协议不计此原因，两格都不落册、不问册，并带回作答的那一版协议；计费的按数量形数出数量与单位；按范围计的一项盖住
// 整个范围。夹具全是合成串，证据只记 S。

// Covers: AT-TF-166——采用的那一版协议没声明这个原因（整节缺席，或只声明了别的原因），答未配置、细格
// CHARGE_CONDITION_NOT_DECLARED、带回那一版协议；不落册、不问覆盖、不交意图。
func TestAnAgreementThatDeclaresNoConditionForTheReasonIsNotConfigured(t *testing.T) {
	cases := map[string]domain.AgreementChargeConditions{
		"no conditions at all": {},
		"another reason only": declaredConditions(t,
			declaredCondition{domain.FailedAttemptOccurrence, true, domain.PerAttemptQuantity}),
	}
	for name, conditions := range cases {
		t.Run(name, func(t *testing.T) {
			rig := newOccurrenceRig(t)
			rig.agreements.check.ChargeConditions = conditions
			result := rig.register(t, actualFulfillmentCommand(t))
			if result.Outcome() != application.ChargeOccurrenceNotConfigured ||
				result.Detail() != application.ChargeConditionNotDeclared {
				t.Fatalf("outcome/detail = %q/%q, want NOT_CONFIGURED/CHARGE_CONDITION_NOT_DECLARED", result.Outcome(), result.Detail())
			}
			if result.Agreement() != trunkSnapshot {
				t.Fatalf("没带回作答的那一版协议：%q", result.Agreement())
			}
			if _, present := result.Record(); present || result.Continuation() != "" {
				t.Fatal("未配置不该回显发生项，也不该给续办引用（不重投）")
			}
			assertNothingWritten(t, rig)
			if len(rig.registry.coverageQueries) != 0 {
				t.Fatal("未配置不该问册")
			}
		})
	}
}

// Covers: AT-TF-167——协议明说这个原因不计费：答 NOT_CHARGED（「协议不计此原因」，不是不适用），带回那一版协议；
// 不落册、不交意图。
func TestAnAgreementThatDoesNotChargeTheReasonAnswersItsOwnCell(t *testing.T) {
	rig := newOccurrenceRig(t)
	rig.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.ActualFulfillmentOccurrence, false, domain.ChargeQuantityFormInvalid})
	result := rig.register(t, actualFulfillmentCommand(t))
	if result.Outcome() != application.ChargeOccurrenceNotCharged || result.Outcome().String() != "NOT_CHARGED" {
		t.Fatalf("outcome = %q, want NOT_CHARGED", result.Outcome())
	}
	if result.Outcome() == application.ChargeOccurrenceNotApplicable {
		t.Fatal("协议不计此原因被说成了不适用")
	}
	if result.Agreement() != trunkSnapshot {
		t.Fatalf("没带回作答的那一版协议：%q", result.Agreement())
	}
	assertNothingWritten(t, rig)
}

// Covers: AT-TF-168——数量与单位由协议的数量形定，登记方不报：按件是成员数、按段是 1 段、按尝试是 1 次；发生条件
// 依据记「按协议声明判」并记登记者，不带断言原话。
func TestTheQuantityFormSetsQuantityAndUnit(t *testing.T) {
	twoParcels := func(t *testing.T, form domain.ChargeQuantityForm) *occurrenceRig {
		rig := newOccurrenceRig(t)
		rig.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
		rig.agreements.check.ChargeConditions = declaredConditions(t,
			declaredCondition{domain.ActualFulfillmentOccurrence, true, form})
		return rig
	}
	for form, want := range map[domain.ChargeQuantityForm]struct {
		quantity int64
		unit     string
	}{
		domain.PerPieceQuantity:   {2, "PIECE"},
		domain.PerSegmentQuantity: {1, "SEGMENT"},
	} {
		t.Run(form.String(), func(t *testing.T) {
			rig := twoParcels(t, form)
			result := rig.register(t, actualFulfillmentCommand(t, trunkParcel, trunkSecondParcel))
			if result.Outcome() != application.ChargeOccurrenceRegistered {
				t.Fatalf("outcome = %q detail = %q", result.Outcome(), result.Detail())
			}
			record, _ := result.Record()
			if quantity, unit := record.Occurrence.Quantity(); quantity != want.quantity || unit.String() != want.unit {
				t.Fatalf("数量/单位 = %d/%q, want %d/%q", quantity, unit, want.quantity, want.unit)
			}
			condition := record.Occurrence.Condition()
			if condition.Basis() != domain.ConditionDeclaredByAgreement || condition.RegisteredBy().String() != registrant {
				t.Fatalf("依据/登记者 = %q/%q", condition.Basis(), condition.RegisteredBy())
			}
			if _, asserted := condition.AssertedBy(); asserted {
				t.Fatal("按协议判的发生项读出了断言人")
			}
			wantWhole := form == domain.PerSegmentQuantity
			if len(rig.registry.coverageQueries) != 1 || rig.registry.coverageQueries[0].WholeScope != wantWhole ||
				rig.registry.coverageQueries[0].Scope.String() != trunkSegment {
				t.Fatalf("覆盖一问 = %+v，按范围计应整范围问", rig.registry.coverageQueries)
			}
		})
	}
}

// Covers: AT-TF-169——按范围计的一项盖住整个范围（ADR-0209 决定四；回答 ADR-0207 越权风险点 4）：按段计时同一段上
// 先登了一件，再登另一件答冲突 SCOPE_ALREADY_CHARGED、回显先登那一项；段已有一项按段计过，换成按件计的协议再登
// 别的成员同样冲突；按尝试计时同一次到场的第二个对象同样冲突。按件计、成员不交的照常登成（AT-TF-125 那一格）。
func TestAScopeCountedOccurrenceCoversTheWholeScope(t *testing.T) {
	twoParcels := func(t *testing.T, form domain.ChargeQuantityForm) *occurrenceRig {
		rig := newOccurrenceRig(t)
		rig.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
		rig.agreements.check.ChargeConditions = declaredConditions(t,
			declaredCondition{domain.ActualFulfillmentOccurrence, true, form})
		return rig
	}

	t.Run("a second registration on a segment charged per segment", func(t *testing.T) {
		rig := twoParcels(t, domain.PerSegmentQuantity)
		first, _ := rig.register(t, actualFulfillmentCommand(t, trunkParcel)).Record()
		result := rig.register(t, actualFulfillmentCommand(t, trunkSecondParcel))
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.ScopeAlreadyCharged {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/SCOPE_ALREADY_CHARGED", result.Outcome(), result.Detail())
		}
		if echoed, _ := result.Record(); echoed.Key != first.Key {
			t.Fatal("冲突没有回显先登的那一项")
		}
		if rig.registry.saves != 1 || len(rig.handoff.intents) != 1 {
			t.Fatalf("不该落第二条：落 %d 条，交 %d 份", rig.registry.saves, len(rig.handoff.intents))
		}
	})

	t.Run("a per piece registration on a segment already charged per segment", func(t *testing.T) {
		rig := twoParcels(t, domain.PerSegmentQuantity)
		rig.register(t, actualFulfillmentCommand(t, trunkParcel))
		rig.agreements.check.ChargeConditions = declaredConditions(t,
			declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerPieceQuantity})
		result := rig.register(t, actualFulfillmentCommand(t, trunkSecondParcel))
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.ScopeAlreadyCharged {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/SCOPE_ALREADY_CHARGED", result.Outcome(), result.Detail())
		}
	})

	t.Run("per piece with disjoint members", func(t *testing.T) {
		rig := twoParcels(t, domain.PerPieceQuantity)
		rig.register(t, actualFulfillmentCommand(t, trunkParcel))
		if outcome := rig.register(t, actualFulfillmentCommand(t, trunkSecondParcel)).Outcome(); outcome != application.ChargeOccurrenceRegistered {
			t.Fatalf("outcome = %q, want REGISTERED", outcome)
		}
	})

	t.Run("an overlapping member on the same fact stays a member conflict", func(t *testing.T) {
		rig := twoParcels(t, domain.PerSegmentQuantity)
		rig.register(t, actualFulfillmentCommand(t, trunkParcel))
		result := rig.register(t, actualFulfillmentCommand(t, trunkParcel, trunkSecondParcel))
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.MembersAlreadyCovered {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/MEMBERS_ALREADY_COVERED", result.Outcome(), result.Detail())
		}
	})

	t.Run("a second object of the same attempt charged per attempt", func(t *testing.T) {
		rig := newOccurrenceRig(t)
		rig.pickups = pickupWithTwoFailures(t)
		first := failedAttemptCommand(t)
		first.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
		if outcome := rig.register(t, first).Outcome(); outcome != application.ChargeOccurrenceRegistered {
			t.Fatalf("第一个对象 outcome = %q", outcome)
		}
		second := first
		second.Objects = []string{trunkSecondParcel}
		result := rig.register(t, second)
		if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.ScopeAlreadyCharged {
			t.Fatalf("outcome/detail = %q/%q, want CONFLICT/SCOPE_ALREADY_CHARGED", result.Outcome(), result.Detail())
		}
	})
}

// Covers: AT-TF-169——自营的两格问册时也认按范围计过的项：段上已有一项按段计的外部发生项，再对段上另一件声明自营，
// 答冲突而不答不适用——同一段不能既有一笔段费又是自营。
func TestDeclaringSelfOperationOverAScopeChargedSegmentIsAConflict(t *testing.T) {
	rig := newOccurrenceRig(t)
	rig.segments = trunkSegmentWith(t, participationRow(t, trunkParcel, true), participationRow(t, trunkSecondParcel, true))
	rig.agreements.check.ChargeConditions = declaredConditions(t,
		declaredCondition{domain.ActualFulfillmentOccurrence, true, domain.PerSegmentQuantity})
	rig.register(t, actualFulfillmentCommand(t, trunkParcel))

	declared := actualFulfillmentCommand(t, trunkSecondParcel)
	declared.SelfOperated, declared.Provider, declared.AgreementObject, declared.AgreementVersion = true, "", "", ""
	result := rig.register(t, declared)
	if result.Outcome() != application.ChargeOccurrenceConflict || result.Detail() != application.ScopeAlreadyCharged {
		t.Fatalf("outcome/detail = %q/%q, want CONFLICT/SCOPE_ALREADY_CHARGED", result.Outcome(), result.Detail())
	}
	if query := rig.registry.coverageQueries[len(rig.registry.coverageQueries)-1]; query.WholeScope || len(query.ScopeUnits) == 0 {
		t.Fatalf("自营问册应按成员与按范围计过的单位问，不整范围问：%+v", query)
	}
}

// Covers: AT-TF-169——「同一次到场」连同侧别认：揽收与派送的尝试身份各由自己的登记册签发，同一个串在两侧各指一次
// 到场。揽收那一次按尝试计过之后，同串的派送失败照常登成，对它声明自营答不适用；揽收那一侧同一次到场的另一个对象
// 照旧冲突。只比范围串，另一家服务方的派送尝试费就登不进去。
func TestAPickupAndADeliveryAttemptSharingAnIdentityAreTwoArrivals(t *testing.T) {
	const shared = "SYN-ATTEMPT-SHARED-01"
	pickupFirst := func(t *testing.T) (*occurrenceRig, application.RegisterChargeOccurrenceCommand) {
		rig := newOccurrenceRig(t)
		rig.pickups = pickupAt(t, shared, domain.GoodsNotReady)
		rig.deliveries = deliveryAt(t, shared, domain.NoOneToReceive)
		pickup := failedAttemptCommand(t)
		pickup.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
		if outcome := rig.register(t, pickup).Outcome(); outcome != application.ChargeOccurrenceRegistered {
			t.Fatalf("揽收那一次 outcome = %q", outcome)
		}
		delivery := failedAttemptCommand(t)
		delivery.DeliveryAttempt = shared
		return rig, delivery
	}

	t.Run("the delivery arrival is charged on its own", func(t *testing.T) {
		rig, delivery := pickupFirst(t)
		result := rig.register(t, delivery)
		if result.Outcome() != application.ChargeOccurrenceRegistered {
			t.Fatalf("outcome/detail = %q/%q, want REGISTERED", result.Outcome(), result.Detail())
		}
		record, _ := result.Record()
		if record.Occurrence.Scope().String() != shared ||
			record.Occurrence.FactBasis().Source() != "DELIVERY-ATTEMPT-RESULT" {
			t.Fatalf("范围/出处 = %q/%q", record.Occurrence.Scope(), record.Occurrence.FactBasis().Source())
		}
		if quantity, unit := record.Occurrence.Quantity(); quantity != 1 || unit.String() != "ATTEMPT" {
			t.Fatalf("数量/单位 = %d/%q", quantity, unit)
		}
		if rig.registry.saves != 2 {
			t.Fatalf("两侧各一项，落了 %d 条", rig.registry.saves)
		}
	})

	t.Run("declaring the delivery arrival self operated is not applicable", func(t *testing.T) {
		rig, delivery := pickupFirst(t)
		delivery.SelfOperated, delivery.Provider, delivery.AgreementObject, delivery.AgreementVersion = true, "", "", ""
		result := rig.register(t, delivery)
		if result.Outcome() != application.ChargeOccurrenceNotApplicable || result.Detail() != application.SelfOperatedDeclared {
			t.Fatalf("outcome/detail = %q/%q, want NOT_APPLICABLE/SELF_OPERATED_DECLARED", result.Outcome(), result.Detail())
		}
	})
}

// pickupWithTwoFailures 是一次揽收到场里两个对象都失败：按尝试计时这一次到场只计一次。
func pickupWithTwoFailures(t *testing.T) ports.FailedAttemptSource {
	t.Helper()
	objects := []domain.CarriedObjectReference{
		occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel),
		occurrenceValue(t, domain.NewCarriedObjectReference, trunkSecondParcel),
	}
	attempt, err := domain.FormFulfillmentAttempt(domain.FulfillmentAttemptSpec{
		TenantID:    occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Attempt:     occurrenceValue(t, domain.NewAttemptReference, "SYN-PICKUP-ATTEMPT-01"),
		Task:        occurrenceValue(t, domain.NewDispatchTaskReference, "SYN-TASK-01"),
		ExecutedBy:  occurrenceValue(t, domain.NewExecutingPartyReference, "SYN-COURIER-01"),
		Place:       occurrenceValue(t, domain.NewAttemptPlaceReference, "SYN-PLACE-01"),
		Evidence:    occurrenceValue(t, domain.NewAttemptEvidenceReference, "SYN-EVIDENCE-01"),
		Objects:     objects,
		PlannedFrom: attemptArrivedAt.Add(-time.Hour),
		PlannedTo:   attemptArrivedAt.Add(time.Hour),
		ArrivedAt:   attemptArrivedAt,
	})
	if err != nil {
		t.Fatalf("尝试夹具：%v", err)
	}
	results := make([]domain.AttemptObjectResult, 0, len(objects))
	for _, object := range objects {
		result, err := domain.FormAttemptObjectResult(attempt, object, domain.GoodsNotReady,
			occurrenceValue(t, domain.NewAttemptResultBasisReference, "SYN-REASON-NOT-READY"), attemptArrivedAt.Add(10*time.Minute))
		if err != nil {
			t.Fatalf("揽收结果夹具：%v", err)
		}
		results = append(results, result)
	}
	return pickupResultSourceDouble{record: ports.PickupAttemptRecord{Attempt: attempt, Results: results}, found: true}
}
