package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证绑定结论在命令口落地时多出的两格（票 ps-external-mark-relations/12）：客户渠道的结论带来源请求键，范围成立带成立时刻。
// 夹具全为合成（S 级）。

// Covers: UC-NO-006 结果语义契约「绑定已否认：否认时刻与来源请求键」——客户渠道的结论必带来源请求键（首方族推导口从接入请求标识
// 取），运营决定的确认不带：它的重放按（交接单，客户账户）认，没有客户面的请求可言。
func TestACustomerChannelConclusionCarriesItsSourceRequestKey(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01")
	customer := domain.DeliveryHandoverBindingSpec{
		TenantID:         note.TenantID(),
		Note:             note.ID(),
		Account:          mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion:       domain.BindingDenied,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        "https://id.syn.example/dex#syn-customer-client-01",
		SourceRequestKey: "SYN-ACCESS-REQUEST-HN-0001",
		ConcludedAt:      handoverOpenedAt.Add(time.Hour),
	}
	denied, err := domain.FormDeliveryHandoverBinding(customer)
	if err != nil {
		t.Fatalf("客户渠道否认：%v", err)
	}
	if denied.SourceRequestKey() != "SYN-ACCESS-REQUEST-HN-0001" || denied.Concluder() != customer.Concluder {
		t.Fatalf("来源 = %q %q", denied.Concluder(), denied.SourceRequestKey())
	}
	missing := customer
	missing.SourceRequestKey = "  "
	if _, err := domain.FormDeliveryHandoverBinding(missing); !errors.Is(err, domain.ErrInvalidDeliveryHandoverNote) {
		t.Fatalf("客户渠道缺来源请求键：err = %v", err)
	}
	operation := domain.DeliveryHandoverBindingSpec{
		TenantID:              note.TenantID(),
		Note:                  note.ID(),
		Account:               customer.Account,
		Conclusion:            domain.BindingConfirmed,
		Route:                 domain.ConcludedViaOperationDecision,
		Concluder:             "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01",
		DeliveringParticipant: "SYN-PARTY-COURIER-01",
		Basis:                 "SYN-BASIS-AUTH-LETTER-01",
		SourceRequestKey:      "SYN-ACCESS-REQUEST-HN-0001",
		ConcludedAt:           handoverOpenedAt.Add(time.Hour),
	}
	if _, err := domain.FormDeliveryHandoverBinding(operation); !errors.Is(err, domain.ErrInvalidDeliveryHandoverNote) {
		t.Fatalf("运营决定带来源请求键：err = %v", err)
	}
	operation.SourceRequestKey = ""
	confirmed, err := domain.FormDeliveryHandoverBinding(operation)
	if err != nil || confirmed.SourceRequestKey() != "" {
		t.Fatalf("运营决定确认：%v %q", err, confirmed.SourceRequestKey())
	}
}

// Covers: UC-NO-006 结果语义契约「可验证交付范围已成立：成立时刻」——范围成立是派生，成立时刻是最后一个声称的客户账户得到确认的
// 那一刻；没成立时不答时刻。
func TestTheScopeIsEstablishedAtTheLastConfirmation(t *testing.T) {
	note := handoverNote(t, "SYN-ACCOUNT-01", "SYN-ACCOUNT-02")
	first := binding(t, note, "SYN-ACCOUNT-02", domain.BindingConfirmed)
	state := domain.DeliveryHandoverNoteState{Note: note, Bindings: []domain.DeliveryHandoverBinding{first}}
	if at, established := state.ScopeEstablishedAt(); established || !at.IsZero() {
		t.Fatalf("只确认一个：%s %v", at, established)
	}
	later, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID:         note.TenantID(),
		Note:             note.ID(),
		Account:          mustBuild(t, domain.NewCustomerAccountReference, "SYN-ACCOUNT-01"),
		Conclusion:       domain.BindingConfirmed,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        "https://id.syn.example/dex#syn-customer-client-01",
		SourceRequestKey: "SYN-ACCESS-REQUEST-HN-0002",
		ConcludedAt:      handoverOpenedAt.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	state.Bindings = append(state.Bindings, later)
	at, established := state.ScopeEstablishedAt()
	if !established || !at.Equal(handoverOpenedAt.Add(3*time.Hour)) {
		t.Fatalf("全部确认：%s %v, want 最后一个确认的时刻", at, established)
	}
}
