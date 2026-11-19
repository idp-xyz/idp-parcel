package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// Covers: 票面「四册共同的规则」——NOT_CONFIGURED 是共用结果代数里的一格，字面量是对外的词；它不是拒绝，所以与 NOT_ACCEPTED 是
// 两个不同的取值，零值仍是「没有结果」。别的结果的字面量由 TestRegistrationAlgebraIsSharedAcrossRegistersAndKeepsItsLiterals 钉着，
// 不在这里重述。
func TestTheNotConfiguredOutcomeHasItsOwnLiteral(t *testing.T) {
	if got := application.RegistrationNotConfigured.String(); got != "NOT_CONFIGURED" {
		t.Fatalf("字面量 = %q，want NOT_CONFIGURED", got)
	}
	if application.RegistrationNotConfigured == application.RegistrationNotAccepted ||
		application.RegistrationNotConfigured == application.RegistrationOutcomeInvalid {
		t.Fatalf("NOT_CONFIGURED 与别的结果撞了取值")
	}
}
