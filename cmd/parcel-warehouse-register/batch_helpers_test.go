package main

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func tenantID(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func domainRegistrant() (domain.Registrant, error) {
	return domain.NewRegistrant(registeredBy)
}

func bytesReader(body string) *strings.Reader { return strings.NewReader(body) }
