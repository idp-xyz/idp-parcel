package postgres

import (
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// lifetimeNumberDocument 是终身注册号在快照与 lifetime_registration_numbers 列里的同一个形状（0034）。
type lifetimeNumberDocument struct {
	TypeCode string `json:"typeCode"`
	Number   string `json:"number"`
}

func lifetimeNumberDocumentsOf(layer domain.LegalEntityIdentityLayer) []lifetimeNumberDocument {
	numbers := layer.Numbers()
	documents := make([]lifetimeNumberDocument, len(numbers))
	for index, number := range numbers {
		documents[index] = lifetimeNumberDocument{
			TypeCode: number.TypeCode().String(),
			Number:   number.Number().String(),
		}
	}
	return documents
}

// identityLayerFromDocuments 从快照重建身份层；国家为空即历史修订，交回 false。
func identityLayerFromDocuments(
	country string,
	documents []lifetimeNumberDocument,
) (domain.LegalEntityIdentityLayer, bool, error) {
	if country == "" && len(documents) == 0 {
		return domain.LegalEntityIdentityLayer{}, false, nil
	}
	code, err := domain.NewRegistrationCountryCode(country)
	if err != nil {
		return domain.LegalEntityIdentityLayer{}, false, err
	}
	numbers := make([]domain.LifetimeRegistrationNumber, len(documents))
	for index, document := range documents {
		typeCode, err := domain.NewRegistrationNumberTypeCode(document.TypeCode)
		if err != nil {
			return domain.LegalEntityIdentityLayer{}, false, err
		}
		value, err := domain.NewRegistrationNumber(document.Number)
		if err != nil {
			return domain.LegalEntityIdentityLayer{}, false, err
		}
		if numbers[index], err = domain.NewLifetimeRegistrationNumber(typeCode, value); err != nil {
			return domain.LegalEntityIdentityLayer{}, false, err
		}
	}
	layer, err := domain.NewLegalEntityIdentityLayer(code, numbers)
	if err != nil {
		return domain.LegalEntityIdentityLayer{}, false, err
	}
	return layer, true, nil
}

// identityLayerColumns 是身份层三列从库里读出来的原样：国家与更正依据可空，号数组是 jsonb 原文。
type identityLayerColumns struct {
	country    *string
	numbers    []byte
	correction *string
}

// apply 把三列转写进上列行的身份层三格；国家为空即历史修订，行上 HasIdentityLayer 为假。
func (columns identityLayerColumns) apply(
	hasIdentity *bool,
	country *string,
	numbers *[]ports.LifetimeRegistrationNumberRow,
	hasCorrection *bool,
	correction *string,
) error {
	if columns.country != nil {
		var documents []lifetimeNumberDocument
		if err := json.Unmarshal(columns.numbers, &documents); err != nil {
			return fmt.Errorf("lifetime_registration_numbers 不是本适配器写下的形状：%w", err)
		}
		*hasIdentity = true
		*country = *columns.country
		rows := make([]ports.LifetimeRegistrationNumberRow, len(documents))
		for index, document := range documents {
			rows[index] = ports.LifetimeRegistrationNumberRow{TypeCode: document.TypeCode, Number: document.Number}
		}
		*numbers = rows
	}
	if columns.correction != nil {
		*hasCorrection = true
		*correction = *columns.correction
	}
	return nil
}
