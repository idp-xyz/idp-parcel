package transportfulfillment_test

import (
	"context"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// noUnits 是单元册的替身：什么都不是单元。既有用例证的是包裹照旧反查、采用或判终局，单元那一问对它们不该有任何影响。
type noUnits struct{}

func (noUnits) IsConsolidationUnitAt(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID, time.Time) (bool, error) {
	return false, nil
}

// unitBook 是单元册的可配置替身（隔离合成 `S`）：units 里登记的对象串是单元，err 非空即读不动；记下每次被问的（租户，对象，时点）。
type unitBook struct {
	units map[string]bool
	err   error
	asked []unitQuestion
}

type unitQuestion struct {
	tenant string
	object string
	at     time.Time
}

func (book *unitBook) IsConsolidationUnitAt(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID, at time.Time,
) (bool, error) {
	book.asked = append(book.asked, unitQuestion{tenant: tenant.String(), object: parcel.String(), at: at})
	if book.err != nil {
		return false, book.err
	}
	return book.units[parcel.String()], nil
}
