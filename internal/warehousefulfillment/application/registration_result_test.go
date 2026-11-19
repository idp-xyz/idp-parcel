package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Covers: 票面「四册共同的规则」——结果的各个字面量是对外的词，各册共用同一套：REFERENCE_NOT_FOUND 是库区起新增的拒绝理由；仓那一册
// 沿用的名字是同一套类型与取值的别名，所以对外的字面量一个字节都没变。下面的赋值是编译期的证明：别名与共用类型是同一个类型，
// 不是另一份枚举。
func TestRegistrationAlgebraIsSharedAcrossRegistersAndKeepsItsLiterals(t *testing.T) {
	var (
		_ application.RegistrationOutcome      = application.WarehouseRegistered
		_ application.RegistrationRefusal      = application.WarehouseRefusalImmutableField
		_ application.RegisterWarehouseOutcome = application.RegistrationAlreadyRegistered
		_ application.RegisterWarehouseRefusal = application.RegistrationRefusalContentInvalid
		_ ports.RegistrySaveOutcome            = ports.WarehouseRevisionSaved
		_ ports.WarehouseSaveOutcome           = ports.RegistryRevisionContentConflict
	)

	for outcome, literal := range map[application.RegistrationOutcome]string{
		application.RegistrationRegistered:        "REGISTERED",
		application.RegistrationAlreadyRegistered: "ALREADY_REGISTERED",
		application.RegistrationContentConflict:   "CONTENT_CONFLICT",
		application.RegistrationNotAccepted:       "NOT_ACCEPTED",
	} {
		if outcome.String() != literal {
			t.Errorf("结果字面量 = %q，want %q", outcome.String(), literal)
		}
	}
	for refusal, literal := range map[application.RegistrationRefusal]string{
		application.RegistrationRefusalRevisionOutOfSequence: "REVISION_OUT_OF_SEQUENCE",
		application.RegistrationRefusalImmutableField:        "IMMUTABLE_FIELD",
		application.RegistrationRefusalContentInvalid:        "CONTENT_INVALID",
		application.RegistrationRefusalReferenceNotFound:     "REFERENCE_NOT_FOUND",
	} {
		if refusal.String() != literal {
			t.Errorf("拒绝理由字面量 = %q，want %q", refusal.String(), literal)
		}
	}
	for outcome, literal := range map[ports.RegistrySaveOutcome]string{
		ports.RegistryRevisionSaved:             "SAVED",
		ports.RegistryRevisionAlreadyRegistered: "ALREADY_REGISTERED",
		ports.RegistryRevisionContentConflict:   "CONTENT_CONFLICT",
	} {
		if outcome.String() != literal {
			t.Errorf("落点字面量 = %q，want %q", outcome.String(), literal)
		}
	}
	if application.RegistrationOutcomeInvalid.String() != "" || application.RegistrationRefusalNone.String() != "" ||
		ports.RegistrySaveOutcomeInvalid.String() != "" {
		t.Errorf("零值的字面量该是空串")
	}
}
