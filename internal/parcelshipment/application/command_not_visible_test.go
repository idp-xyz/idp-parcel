package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: ADR-0022「`!found` 一律映射为对应上下文的`统一不可见结果`」在受控补充、主动拒绝与授权处置三例上——指名一份查不到的
// 委托上抛同一个具名错误，接 HTTP 时答 404；它仍是 ErrInvalidShipmentRequest 的一种，按旧名判的调用方不受影响。授权在定位之后，
// 查不到的委托连授权都不问。
func TestCommandsNamingAnUnknownRequestAreUniformlyInvisible(t *testing.T) {
	t.Run("受控补充", func(t *testing.T) {
		fixture := newSupplementFixture(t)
		fixture.requests.missing = true
		_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		assertNotVisible(t, err)
	})
	t.Run("主动拒绝", func(t *testing.T) {
		fixture := newRejectionFixture(t)
		fixture.requests.missing = true
		_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		assertNotVisible(t, err)
	})
	t.Run("授权处置", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.requests.missing = true
		_, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		assertNotVisible(t, err)
		if len(fixture.authorizer.asked) != 0 {
			t.Fatal("对一份查不到的委托问了授权")
		}
	})
}

func assertNotVisible(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
		t.Fatalf("err = %v, want ErrShipmentRequestNotVisible", err)
	}
	if !errors.Is(err, domain.ErrInvalidShipmentRequest) {
		t.Fatalf("err = %v 不再是 ErrInvalidShipmentRequest 的一种", err)
	}
}
