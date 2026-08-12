package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	commissionSubmittedAt = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	bookingRequestedAt    = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
)

func commissionSpec(t *testing.T) domain.TransportCommissionSpec {
	t.Helper()
	return domain.TransportCommissionSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Commission:     mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Provider:       mustValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:      mustValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-1"),
		Conditions:     mustValue(t, domain.NewConditionsSnapshotReference, "conditions-snapshot-1"),
		Role:           mustValue(t, domain.NewRoleSnapshotReference, "role-snapshot-1"),
		Responsibility: mustValue(t, domain.NewResponsibilitySnapshotReference, "responsibility-snapshot-1"),
		Members: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		},
		SubmittedAt: commissionSubmittedAt,
	}
}

func submittedBooking(t *testing.T) domain.BookingRequest {
	t.Helper()
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Booking:     mustValue(t, domain.NewBookingReference, "booking-1"),
		Commission:  mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Quantity:    100,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, "kg"),
		RequestedAt: bookingRequestedAt,
	})
	if err != nil {
		t.Fatalf("submit booking: %v", err)
	}
	return booking
}

// Covers: CONTEXT「运输委托引用供应商商业协议和履约条件快照，但不等于订舱、承运接受、
// 实际履约段或供应商账单」与 243「保存……协议、条件、角色和责任依据快照，不修改商业
// 版本」——四件快照引用缺一不可；类型上只有引用，没有任何商业版本本体可改。
func TestACommissionSnapshotsItsCommercialBasis(t *testing.T) {
	commission, err := domain.SubmitTransportCommission(commissionSpec(t))
	if err != nil {
		t.Fatalf("submit commission: %v", err)
	}
	if commission.Agreement().String() != "agreement-snapshot-1" ||
		commission.Conditions().String() != "conditions-snapshot-1" ||
		commission.Role().String() != "role-snapshot-1" ||
		commission.Responsibility().String() != "responsibility-snapshot-1" {
		t.Fatal("协议/条件/角色/责任快照没有随委托保全")
	}
	if _, _, started := commission.TransportStarted(); started {
		t.Fatal("刚提交的委托凭空开始了运输")
	}

	broken := map[string]func(*domain.TransportCommissionSpec){
		"no agreement":  func(spec *domain.TransportCommissionSpec) { spec.Agreement = domain.AgreementSnapshotReference{} },
		"no conditions": func(spec *domain.TransportCommissionSpec) { spec.Conditions = domain.ConditionsSnapshotReference{} },
		"no role":       func(spec *domain.TransportCommissionSpec) { spec.Role = domain.RoleSnapshotReference{} },
		"no responsibility": func(spec *domain.TransportCommissionSpec) {
			spec.Responsibility = domain.ResponsibilitySnapshotReference{}
		},
		"no provider":      func(spec *domain.TransportCommissionSpec) { spec.Provider = domain.ServiceProviderReference{} },
		"no members":       func(spec *domain.TransportCommissionSpec) { spec.Members = nil },
		"duplicate member": func(spec *domain.TransportCommissionSpec) { spec.Members = append(spec.Members, spec.Members[0]) },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := commissionSpec(t)
			breakSpec(&spec)
			if _, err := domain.SubmitTransportCommission(spec); !errors.Is(err, domain.ErrInvalidTransportCommission) {
				t.Fatalf("error = %v, want ErrInvalidTransportCommission", err)
			}
		})
	}
}

// Covers: CONTEXT「承运接受……不等于容量已经预占、载运对象已经分配或实际承运商已经
// 收寄」与 105「面单生成、渠道受理、预报成功、订舱接受、舱单建立、电子数据接收和车辆
// 到场均不构成实际承运商首次有效收寄」——只有接受成约；拒绝/失效/撤回分格带原因；
// 接受对象上没有任何收寄或控制字段（结构防线）。
func TestBookingIsNotAcceptanceAndOnlyAcceptanceBinds(t *testing.T) {
	for _, subject := range []reflect.Type{
		reflect.TypeOf(domain.BookingRequest{}),
		reflect.TypeOf(domain.CarrierAcceptance{}),
	} {
		for index := 0; index < subject.NumField(); index++ {
			name := strings.ToLower(subject.Field(index).Name)
			for _, forbidden := range []string{"intake", "receipt", "control", "pickup", "custody"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s 携带 %q——订舱/接受就能被读成已收寄或已控制", subject.Name(), subject.Field(index).Name)
				}
			}
		}
	}

	booking := submittedBooking(t)
	accepted, err := domain.FormCarrierAcceptance(
		booking,
		mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-1"),
		domain.BookingAccepted,
		80,
		domain.AcceptanceBasisReference{},
		bookingRequestedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("form acceptance: %v", err)
	}
	if !accepted.Binds() {
		t.Fatal("接受没有成约")
	}
	if accepted.AcceptedQuantity() != 80 {
		t.Fatalf("accepted quantity = %d, want 80（部分接受保留数量）", accepted.AcceptedQuantity())
	}

	t.Run("acceptance beyond the requested quantity is refused", func(t *testing.T) {
		if _, err := domain.FormCarrierAcceptance(
			booking,
			mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-x"),
			domain.BookingAccepted,
			101,
			domain.AcceptanceBasisReference{},
			bookingRequestedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidCarrierAcceptance) {
			t.Fatalf("error = %v; 接受量超过了申请量", err)
		}
	})

	for name, outcome := range map[string]domain.CarrierAcceptanceOutcome{
		"refused":   domain.BookingRefused,
		"expired":   domain.BookingExpired,
		"withdrawn": domain.BookingWithdrawn,
	} {
		t.Run(name+" carries its basis and does not bind", func(t *testing.T) {
			result, err := domain.FormCarrierAcceptance(
				booking,
				mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-"+name),
				outcome,
				0,
				mustValue(t, domain.NewAcceptanceBasisReference, "basis-"+name),
				bookingRequestedAt.Add(time.Hour),
			)
			if err != nil {
				t.Fatalf("form %s: %v", name, err)
			}
			if result.Binds() {
				t.Fatalf("%s 竟然成约了", name)
			}
			if _, present := result.Basis(); !present {
				t.Fatalf("%s 丢了原因来源", name)
			}
		})

		t.Run(name+" without a basis is refused", func(t *testing.T) {
			if _, err := domain.FormCarrierAcceptance(
				booking,
				mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-y"),
				outcome,
				0,
				domain.AcceptanceBasisReference{},
				bookingRequestedAt.Add(time.Hour),
			); !errors.Is(err, domain.ErrInvalidCarrierAcceptance) {
				t.Fatalf("error = %v; 没有原因的%s与数据丢失无从分辨", err, name)
			}
		})
	}

	t.Run("a non-accepted outcome cannot carry a quantity", func(t *testing.T) {
		if _, err := domain.FormCarrierAcceptance(
			booking,
			mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-z"),
			domain.BookingRefused,
			10,
			mustValue(t, domain.NewAcceptanceBasisReference, "basis-z"),
			bookingRequestedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidCarrierAcceptance) {
			t.Fatalf("error = %v; 拒绝带上了接受量", err)
		}
	})

	t.Run("the acceptance outcome set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, outcome := range []domain.CarrierAcceptanceOutcome{
			domain.BookingAccepted, domain.BookingRefused, domain.BookingExpired, domain.BookingWithdrawn,
		} {
			label := outcome.String()
			if label == "" {
				t.Fatalf("outcome %d has no label", outcome)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.CarrierAcceptanceOutcome(len(labels)+1).String() != "" {
			t.Fatal("第五个接受取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: CONTEXT「首个有效出发或移动事实形成前可以取消……之后只能形成中断、改降、
// 折返或其他实际结果」与 195「尚未消耗的运输委托或订舱范围可以取消；已经形成的承运
// 接受……继续按各自规则保留」——取消只对未开始的意图；已开始的委托拒绝取消；已取消
// 的没有可开始的意图；接受对象上没有取消入口（结果不撤销）。
func TestCancellationOnlyReachesUnstartedIntents(t *testing.T) {
	acceptanceType := reflect.TypeOf(domain.CarrierAcceptance{})
	for index := 0; index < acceptanceType.NumMethod(); index++ {
		name := strings.ToLower(acceptanceType.Method(index).Name)
		for _, banned := range []string{"cancel", "rollback", "revert", "withdrawmethod"} {
			if strings.Contains(name, banned) {
				t.Fatalf("CarrierAcceptance 带方法 %q——接受结果就有了被撤销的入口（撤回是新的结果取值，不是对原结果的编辑）", name)
			}
		}
	}

	commission, err := domain.SubmitTransportCommission(commissionSpec(t))
	if err != nil {
		t.Fatalf("submit commission: %v", err)
	}

	cancelled, err := commission.Cancel(commissionSubmittedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("cancel unstarted commission: %v", err)
	}
	if _, ok := cancelled.Cancelled(); !ok {
		t.Fatal("取消没有登记")
	}

	t.Run("a cancelled commission cannot start transport", func(t *testing.T) {
		if _, err := cancelled.MarkTransportStarted(
			mustValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/pickup-result/parcel-1/v1"),
			commissionSubmittedAt.Add(2*time.Hour),
		); !errors.Is(err, domain.ErrInvalidTransportCommission) {
			t.Fatalf("error = %v; 已取消的委托开始了运输", err)
		}
	})

	t.Run("a started commission cannot be cancelled", func(t *testing.T) {
		started, err := commission.MarkTransportStarted(
			mustValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/pickup-result/parcel-1/v1"),
			commissionSubmittedAt.Add(time.Hour),
		)
		if err != nil {
			t.Fatalf("mark started: %v", err)
		}
		if _, err := started.Cancel(commissionSubmittedAt.Add(2 * time.Hour)); !errors.Is(err, domain.ErrTransportAlreadyStarted) {
			t.Fatalf("error = %v, want ErrTransportAlreadyStarted", err)
		}
		if _, err := started.MarkTransportStarted(
			mustValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/pickup-result/parcel-1/v2"),
			commissionSubmittedAt.Add(3*time.Hour),
		); !errors.Is(err, domain.ErrTransportAlreadyStarted) {
			t.Fatalf("error = %v; 开始只发生一次", err)
		}
	})

	t.Run("a double cancel is refused", func(t *testing.T) {
		if _, err := cancelled.Cancel(commissionSubmittedAt.Add(2 * time.Hour)); !errors.Is(err, domain.ErrInvalidTransportCommission) {
			t.Fatalf("error = %v; 取消重复登记", err)
		}
	})

	t.Run("a booking cancels independently and only once", func(t *testing.T) {
		booking := submittedBooking(t)
		cancelledBooking, err := booking.Cancel(bookingRequestedAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("cancel booking: %v", err)
		}
		if _, err := cancelledBooking.Cancel(bookingRequestedAt.Add(2 * time.Hour)); !errors.Is(err, domain.ErrInvalidBookingRequest) {
			t.Fatalf("error = %v; 订舱取消重复登记", err)
		}
		if _, ok := booking.Cancelled(); ok {
			t.Fatal("取消改写了原订舱值——值语义破了")
		}
	})
}
