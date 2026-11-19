package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// Covers: 三个头名是设备与服务端之间的线格式——字面量写在测试里，常量被改名或改拼写时这条先红，而不是跟着一起变。
func TestWorkFactHeaderNamesAreTheWireFormat(t *testing.T) {
	for name, pair := range map[string][2]string{
		"设备":     {httpapi.HeaderWorkDevice, "Parcel-Device-Id"},
		"作业范围种类": {httpapi.HeaderWorkScopeKind, "Parcel-Work-Scope-Kind"},
		"作业范围引用": {httpapi.HeaderWorkScopeReference, "Parcel-Work-Scope-Reference"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s头名 = %q, want %q", name, pair[0], pair[1])
		}
	}
}

func workFactHeadersRequest(device, kind, reference string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	if device != "" {
		request.Header.Set("Parcel-Device-Id", device)
	}
	if kind != "" {
		request.Header.Set("Parcel-Work-Scope-Kind", kind)
	}
	if reference != "" {
		request.Header.Set("Parcel-Work-Scope-Reference", reference)
	}
	return request
}

// Covers: 三个头齐全时原样交回、complete 为真；种类取什么值平台不判，OFFSITE 与词表外的值同样交回。
func TestReadWorkFactHeadersReturnsAllThreeWhenPresent(t *testing.T) {
	for _, kind := range []string{"NODE", "OFFSITE", "SYN-ANY-KIND"} {
		device, gotKind, reference, complete := httpapi.ReadWorkFactHeaders(workFactHeadersRequest("SYN-DEVICE-01", kind, "SYN-SCOPE-01"))
		if !complete || device != "SYN-DEVICE-01" || gotKind != kind || reference != "SYN-SCOPE-01" {
			t.Fatalf("kind %q：读到 %q|%q|%q complete=%v，want 三个头逐一对上且齐全", kind, device, gotKind, reference, complete)
		}
	}
}

// Covers: 缺任何一个都不齐全，另两个照读——缺头时答什么由调用方定，读取函数只如实说「不齐」。
func TestReadWorkFactHeadersIsIncompleteWhenAnyOneIsMissing(t *testing.T) {
	for name, request := range map[string]*http.Request{
		"缺设备":     workFactHeadersRequest("", "NODE", "SYN-SCOPE-01"),
		"缺作业范围种类": workFactHeadersRequest("SYN-DEVICE-01", "", "SYN-SCOPE-01"),
		"缺作业范围引用": workFactHeadersRequest("SYN-DEVICE-01", "NODE", ""),
		"三个都缺":    workFactHeadersRequest("", "", ""),
	} {
		if _, _, _, complete := httpapi.ReadWorkFactHeaders(request); complete {
			t.Fatalf("%s：complete = true, want false", name)
		}
	}

	device, kind, reference, _ := httpapi.ReadWorkFactHeaders(workFactHeadersRequest("", "NODE", "SYN-SCOPE-01"))
	if device != "" || kind != "NODE" || reference != "SYN-SCOPE-01" {
		t.Fatalf("缺设备时读到 %q|%q|%q, want 缺的那个为空、另两个照读", device, kind, reference)
	}
}

// Covers: 头值两端的空白在读取时去掉，交回的是去过空白的值；只有空白的头值与缺头同算缺。
func TestReadWorkFactHeadersTrimsWhitespaceAndTreatsBlankAsMissing(t *testing.T) {
	device, kind, reference, complete := httpapi.ReadWorkFactHeaders(workFactHeadersRequest("  SYN-DEVICE-01\t", " NODE ", "\tSYN-SCOPE-01  "))
	if !complete || device != "SYN-DEVICE-01" || kind != "NODE" || reference != "SYN-SCOPE-01" {
		t.Fatalf("读到 %q|%q|%q complete=%v, want 去过两端空白的三个值且齐全", device, kind, reference, complete)
	}

	for name, request := range map[string]*http.Request{
		"设备只有空白":     workFactHeadersRequest("   ", "NODE", "SYN-SCOPE-01"),
		"作业范围种类只有空白": workFactHeadersRequest("SYN-DEVICE-01", "\t", "SYN-SCOPE-01"),
		"作业范围引用只有空白": workFactHeadersRequest("SYN-DEVICE-01", "NODE", " \t "),
	} {
		if _, _, _, complete := httpapi.ReadWorkFactHeaders(request); complete {
			t.Fatalf("%s：complete = true, want false", name)
		}
	}
}
