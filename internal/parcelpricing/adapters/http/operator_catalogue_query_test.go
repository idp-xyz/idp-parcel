package pricinghttp_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

type catalogueReaderAuthenticator struct {
	tenant domain.TenantID
	err    error
}

func (authenticator catalogueReaderAuthenticator) AuthenticateOperationsRead(context.Context, string) (domain.TenantID, error) {
	return authenticator.tenant, authenticator.err
}

// Covers: 目录查阅的租户只取自信封，页大小是产品常量；请求体不读。
func TestOperatorCatalogueQueryTakesTheTenantFromTheEnvelope(t *testing.T) {
	tenant := pricingValue(t, domain.NewTenantID, "SYN-TENANT-01")
	intake, err := pricinghttp.NewOperatorOperationsReadIntake(catalogueReaderAuthenticator{tenant: tenant})
	if err != nil {
		t.Fatal(err)
	}
	body := &readProbe{Reader: strings.NewReader("ignored")}
	request, err := http.NewRequest(http.MethodGet, "/pricing-price-cards", body)
	if err != nil {
		t.Fatal(err)
	}
	query, err := intake.IntakeCatalogueQuery(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if query.Scope.Tenant() != tenant || query.Limit != 200 || body.read {
		t.Fatalf("query tenant %s limit %d bodyRead %v", query.Scope.Tenant(), query.Limit, body.read)
	}
	refused, err := pricinghttp.NewOperatorOperationsReadIntake(catalogueReaderAuthenticator{err: pricinghttp.ErrOperatorNotGranted})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = refused.IntakeCatalogueQuery(context.Background(), request); !errors.Is(err, pricinghttp.ErrOperatorNotGranted) {
		t.Fatalf("err = %v, want ErrOperatorNotGranted", err)
	}
}

type readProbe struct {
	io.Reader
	read bool
}

func (probe *readProbe) Read(p []byte) (int, error) {
	probe.read = true
	return probe.Reader.Read(p)
}
