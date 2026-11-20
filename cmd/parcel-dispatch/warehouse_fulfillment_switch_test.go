package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/cmd/internal/warehouseswitch"
)

// 本文件证仓配部署开关在派发进程里的两处：settingsFromEnv 读一次，仓配专用登记函数按它决定登不登。它们不碰库。

// warehouseSwitchEnvironment 是一份齐全的必填部署形态参数。开关不在其中——它不是必填项，缺省为关。
func warehouseSwitchEnvironment() map[string]string {
	return map[string]string{
		"IDP_PARCEL_POSTGRES_DSN":              "postgres://parcel@127.0.0.1:5432/parcel",
		"IDP_PARCEL_ROUTE_SERVICE_PURPOSE":     "NETWORK_SERVICE",
		"IDP_PARCEL_DISPATCH_DELIVERY_TIMEOUT": "5s",
		"IDP_PARCEL_DISPATCH_LEASE":            "1m",
		"IDP_PARCEL_DISPATCH_RETRY_AFTER":      "30s",
		"IDP_PARCEL_DISPATCH_LIMIT":            "10",
		"IDP_PARCEL_DISPATCH_MAX_ATTEMPTS":     "5",
	}
}

// Covers: ADR-0375 决定四——没设开关时其余部署形态参数的读法一字不变，开关读成关；设为 true 才开，别的取值仍按关读。
func TestSettingsFromEnvReadsTheWarehouseFulfillmentSwitchAndNothingElseChanges(t *testing.T) {
	unset, err := settingsFromEnv(lookupIn(warehouseSwitchEnvironment()))
	if err != nil {
		t.Fatalf("没设开关的齐全配置反被拒：%v", err)
	}
	if unset.warehouseFulfillment {
		t.Fatal("没设开关却读成了开：缺省必须是关")
	}
	if unset.databaseDSN != "postgres://parcel@127.0.0.1:5432/parcel" ||
		unset.purpose.String() != "NETWORK_SERVICE" ||
		unset.deliveryTimeout != 5*time.Second ||
		unset.config.LeaseFor != time.Minute ||
		unset.config.RetryAfter != 30*time.Second ||
		unset.config.Limit != 10 ||
		unset.config.MaxAttempts != 5 {
		t.Fatalf("没设开关时其余参数的读法变了：%+v", unset)
	}

	for value, want := range map[string]bool{"true": true, " true ": true, "TRUE": false, "1": false, "yes": false, "": false} {
		environment := warehouseSwitchEnvironment()
		environment[warehouseswitch.EnabledEnv] = value
		settings, err := settingsFromEnv(lookupIn(environment))
		if err != nil {
			t.Fatalf("开关 %q：齐全配置反被拒：%v", value, err)
		}
		if settings.warehouseFulfillment != want {
			t.Fatalf("开关 %q 读成 %v，want %v", value, settings.warehouseFulfillment, want)
		}
		if settings.databaseDSN != unset.databaseDSN || settings.purpose != unset.purpose ||
			settings.deliveryTimeout != unset.deliveryTimeout || settings.config != unset.config {
			t.Fatalf("开关 %q 牵动了其余参数：%+v 对 %+v", value, settings, unset)
		}
	}
}

// Covers: ADR-0375 决定四——开关打开也不放宽必填项：缺 DSN 照旧点名停下。
func TestTheWarehouseFulfillmentSwitchDoesNotWaiveARequiredSetting(t *testing.T) {
	environment := warehouseSwitchEnvironment()
	delete(environment, "IDP_PARCEL_POSTGRES_DSN")
	environment[warehouseswitch.EnabledEnv] = "true"
	if _, err := settingsFromEnv(lookupIn(environment)); err == nil {
		t.Fatal("开关为 true 时缺 DSN 仍装配出了配置")
	}
}

// warehouseSwitchLines 取日志里带开关环境变量名的那几行的正文。
func warehouseSwitchLines(t *testing.T, raw []byte) []string {
	t.Helper()
	var messages []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("日志行不是 JSON：%q：%v", line, err)
		}
		if decoded["env"] == warehouseswitch.EnabledEnv {
			message, _ := decoded["msg"].(string)
			messages = append(messages, message)
		}
	}
	return messages
}

// Covers: ADR-0375 决定四「各进程在启动时读一次、带启动日志」——派发进程的装配点在开池之前把开关写进启动日志，开与关各一条。
// 库不可达时装配在连库那一步失败，日志已经先出了声；写出的正文与共享件对同一个取值写的一字不差，证明交给日志的是
// settingsFromEnv 读出的那个值。不需要真实 PostgreSQL。
func TestAssemblyAnnouncesTheWarehouseFulfillmentSwitchBeforeTheDatabaseIsReached(t *testing.T) {
	for _, test := range []struct {
		value   string
		enabled bool
	}{{value: "", enabled: false}, {value: "true", enabled: true}} {
		var reference bytes.Buffer
		warehouseswitch.LogStartup(slog.New(slog.NewJSONHandler(&reference, nil)), test.enabled)
		want := warehouseSwitchLines(t, reference.Bytes())
		if len(want) != 1 {
			t.Fatalf("共享件对 enabled=%v 写了 %d 条，本用例的参照失效", test.enabled, len(want))
		}

		base := startupEnv("postgres://parcel:parcel@127.0.0.1:1/postgres?sslmode=disable")
		getenv := func(name string) string {
			if name == warehouseswitch.EnabledEnv {
				return test.value
			}
			return base(name)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		var logged bytes.Buffer
		beat, cleanup, err := assembleDispatcher(ctx, getenv, slog.New(slog.NewJSONHandler(&logged, nil)))
		cancel()
		if err == nil {
			if cleanup != nil {
				cleanup()
			}
			t.Fatalf("库不可达却装配成功了，交回了一拍：%v", beat)
		}

		got := warehouseSwitchLines(t, logged.Bytes())
		if len(got) != 1 || got[0] != want[0] {
			t.Fatalf("开关 %q：装配点写的启动日志 = %q，want 恰好一条 %q", test.value, got, want)
		}
	}
}

var errWarehouseRouteBuild = errors.New("warehouse route build failed")

type warehouseBuildProbe struct {
	calls int
	legs  []routeLeg
	err   error
}

func (probe *warehouseBuildProbe) build(dispatchSources) ([]routeLeg, error) {
	probe.calls++
	return probe.legs, probe.err
}

func warehouseSources(enabled bool) dispatchSources {
	return dispatchSources{settings: dispatchSettings{warehouseFulfillment: enabled}}
}

func warehouseTestLegs() []routeLeg {
	return []routeLeg{{event: "route-test.warehouse", position: positionPrimary, untranslated: true, label: "warehouse test leg"}}
}

// Covers: ADR-0375 决定四——关着时不登记仓配的任何一路：交回零路，并且构造函数一次都不调（它可能要开仓配自己的表与适配器，
// 关着的部署不该为此付任何装配代价，更不该因它而失败）。
func TestAWarehouseFulfillmentRouteIsNotBuiltWhileTheSwitchIsOff(t *testing.T) {
	probe := &warehouseBuildProbe{legs: warehouseTestLegs(), err: errWarehouseRouteBuild}
	legs, err := gatedByWarehouseFulfillmentSwitch(probe.build)(warehouseSources(false))
	if err != nil {
		t.Fatalf("关着时不该有错，实得：%v", err)
	}
	if len(legs) != 0 {
		t.Fatalf("关着时交回了 %d 路，want 零路", len(legs))
	}
	if probe.calls != 0 {
		t.Fatalf("关着时调用了构造函数 %d 次，want 0", probe.calls)
	}
}

// Covers: ADR-0375 决定四——开着时原样交回构造函数的路。
func TestAWarehouseFulfillmentRouteIsBuiltAsGivenWhileTheSwitchIsOn(t *testing.T) {
	probe := &warehouseBuildProbe{legs: warehouseTestLegs()}
	legs, err := gatedByWarehouseFulfillmentSwitch(probe.build)(warehouseSources(true))
	if err != nil {
		t.Fatalf("开着时构造成功，却得到错：%v", err)
	}
	if probe.calls != 1 {
		t.Fatalf("构造函数被调了 %d 次，want 1", probe.calls)
	}
	if len(legs) != 1 || legs[0].label != "warehouse test leg" || legs[0].event != "route-test.warehouse" {
		t.Fatalf("交回的路与构造函数给的不一致：%+v", legs)
	}
}

// Covers: ADR-0375 决定四——开着时构造函数报错，错误原样上抛：装配点据此带原因退出，不吞也不改写。
func TestAWarehouseFulfillmentRouteBuildErrorIsReturnedUnchangedWhileTheSwitchIsOn(t *testing.T) {
	probe := &warehouseBuildProbe{err: errWarehouseRouteBuild}
	legs, err := gatedByWarehouseFulfillmentSwitch(probe.build)(warehouseSources(true))
	if err != errWarehouseRouteBuild {
		t.Fatalf("错误没有原样上抛：%v", err)
	}
	if len(legs) != 0 {
		t.Fatalf("构造失败时交回了 %d 路", len(legs))
	}
	if probe.calls != 1 {
		t.Fatalf("构造函数被调了 %d 次，want 1", probe.calls)
	}
}

// Covers: ADR-0375 决定四——包这一层不吞 registerRoutes 的「特性必须带构造函数」那道护栏：构造函数为空时包出来的仍是空，
// 让 registerRoutes 自己在登记那一刻 panic，而不是拖到关着开关的部署里永远碰不到、开着时才炸。
func TestGatingANilBuildFunctionStaysNil(t *testing.T) {
	if gated := gatedByWarehouseFulfillmentSwitch(nil); gated != nil {
		t.Fatal("空构造函数被包成了非空闭包")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("registerWarehouseFulfillmentRoutes(nil) 没有 panic")
		}
	}()
	registerWarehouseFulfillmentRoutes(nil)
}

// Covers: ADR-0375 决定四——仓配专用登记函数往 routeRegistry 登的是按开关包过的那一份，不是裸构造函数。
//
// routeRegistry 是包级变量，别的用例展开它读各路；这里登完立刻还原，登进去的那一项在还原之后才被调用，
// 假的路因此一刻也不会留在表里。同包的并行用例要等串行用例都跑完才继续，且它们在展开路由表之前就已失败，
// 还原前后不会有别的 goroutine 读到那一格。
func TestTheWarehouseFulfillmentRegistrationRegistersTheSwitchGatedBuild(t *testing.T) {
	probe := &warehouseBuildProbe{legs: warehouseTestLegs()}

	before := append([]func(dispatchSources) ([]routeLeg, error)(nil), routeRegistry...)
	registerWarehouseFulfillmentRoutes(probe.build)
	registered := append([]func(dispatchSources) ([]routeLeg, error)(nil), routeRegistry...)
	routeRegistry = before

	if len(registered) != len(before)+1 {
		t.Fatalf("登记后表里有 %d 项，登记前 %d 项，want 恰多一项", len(registered), len(before))
	}
	entry := registered[len(registered)-1]

	if legs, err := entry(warehouseSources(false)); err != nil || len(legs) != 0 || probe.calls != 0 {
		t.Fatalf("关着时登出的那一项构造了路：legs=%d err=%v calls=%d", len(legs), err, probe.calls)
	}
	if legs, err := entry(warehouseSources(true)); err != nil || len(legs) != 1 || probe.calls != 1 {
		t.Fatalf("开着时登出的那一项没有交回构造函数的路：legs=%d err=%v calls=%d", len(legs), err, probe.calls)
	}
}

// Covers: ADR-0375 决定四——接受决定这一路只在开关打开时出现在扇出里。
func TestTheWarehouseAcceptanceRouteFollowsTheSwitch(t *testing.T) {
	event := eventing.EventType("parcel-shipment.acceptance-decision.formed")
	for _, label := range labelsOf(registeredLegs(t, event, false)) {
		if label == "warehouse acceptance" {
			t.Fatal("开关关着仍登记了仓配接受决定")
		}
	}
	found := false
	for _, label := range labelsOf(registeredLegs(t, event, true)) {
		if label == "warehouse acceptance" {
			found = true
		}
	}
	if !found {
		t.Fatal("开关开着没有仓配接受决定这一路")
	}
	supplement := eventing.EventType("parcel-shipment.customer-supplement.required")
	off := labelsOf(registeredLegs(t, supplement, false))
	on := labelsOf(registeredLegs(t, supplement, true))
	if !containsLabel(off, "customer supplement close-out") || containsLabel(off, "warehouse supplement") {
		t.Fatalf("关着时待补充各路 = %v", off)
	}
	if !containsLabel(on, "warehouse supplement") || containsLabel(on, "customer supplement close-out") {
		t.Fatalf("开着时待补充各路 = %v", on)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}

func labelsOf(legs []routeLeg) []string {
	labels := make([]string, 0, len(legs))
	for _, leg := range legs {
		labels = append(labels, leg.label)
	}
	return labels
}
