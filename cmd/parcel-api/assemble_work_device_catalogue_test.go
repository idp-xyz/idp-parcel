package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
)

// Covers: 票 operator-channel/21——查阅端点接在操作者查阅读上。发行方未配置答未配置；没有查阅读授予答未授予；
// 授予齐备而册读失败答答案未形成，不答未配置；空册答空列表。
func TestWorkDeviceCatalogueAnswersFromTheOperatorChannel(t *testing.T) {
	if identityhttp.WorkDeviceCataloguePath != "/work-devices" {
		t.Fatalf("路径常量 = %s，端点表字面量对不上", identityhttp.WorkDeviceCataloguePath)
	}

	granted, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	ungranted, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		minter    *accessidentity.OperatorMinter
		catalogue accessidentity.DeviceCatalogue
		token     string
		status    int
		code      string
		outcome   string
	}{
		{"issuer unconfigured", unconfigured, emptyDeviceCatalogue{}, "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED", ""},
		{"no bearer token", granted, emptyDeviceCatalogue{}, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED", ""},
		{"write grant is not a read grant", ungranted, emptyDeviceCatalogue{}, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED", ""},
		{"reader down", granted, failingDeviceCatalogue{}, "presented.operator.token", http.StatusInternalServerError, "NO_ANSWER_FORMED", ""},
		{"empty catalogue", granted, emptyDeviceCatalogue{}, "presented.operator.token", http.StatusOK, "", "WORK_DEVICES_LISTED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler, err := buildWorkDeviceCatalogue(testCase.minter, testCase.catalogue)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/work-devices?tenant=SYN-TENANT-02", nil)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("status %d body %s", response.Code, response.Body.String())
			}
			if testCase.outcome != "" {
				var listed struct {
					Outcome string            `json:"outcome"`
					Devices []json.RawMessage `json:"devices"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
					t.Fatal(err)
				}
				if listed.Outcome != testCase.outcome || listed.Devices == nil || len(listed.Devices) != 0 {
					t.Fatalf("body %s", response.Body.String())
				}
				return
			}
			if problemCode(t, response) != testCase.code {
				t.Fatalf("code %s body %s", problemCode(t, response), response.Body.String())
			}
		})
	}
}

type emptyDeviceCatalogue struct{}

func (emptyDeviceCatalogue) ListDevices(context.Context, string) ([]accessidentity.DeviceCatalogueRow, error) {
	return []accessidentity.DeviceCatalogueRow{}, nil
}

type failingDeviceCatalogue struct{}

func (failingDeviceCatalogue) ListDevices(context.Context, string) ([]accessidentity.DeviceCatalogueRow, error) {
	return nil, errors.New("device catalogue down")
}
