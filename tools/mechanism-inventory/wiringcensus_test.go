package main

import (
	"testing"
	"testing/fstest"
)

// wiringFixture 造一棵最小的接线树。端点表与路由表都用真实装配点的写法（别名导入、类型化字面量、
// 键值元素），差别只在条目少——清点认的是形状，不是数量。
func wiringFixture() fstest.MapFS {
	file := func(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }
	return fstest.MapFS{
		"cmd/parcel-api/endpoints.go": file(`package main

import (
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

func assembleBusinessEndpoints(fallback httpapi.Handler) []httpapi.BusinessEndpoint {
	return []httpapi.BusinessEndpoint{
		{Pattern: "/shipment-requests", Handler: shipmenthttp.NewSubmitShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, nil)},
		{Pattern: "/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, nil)},
		{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(nodeopshttp.UnconfiguredIntake{}, nil)},
		{Pattern: "/somewhere-else", Handler: fallback},
	}
}
`),
		// 测试文件里的端点表是脚手架，不是装配。
		"cmd/parcel-api/endpoints_test.go": file(`package main

import "go.idp.xyz/idp-parcel/internal/platform/httpapi"

var scaffold = []httpapi.BusinessEndpoint{{Pattern: "/probe"}, {Pattern: "/probe-2"}}
`),
		// 同名类型、别的导入路径：不是接入面端点。
		"cmd/parcel-other/main.go": file(`package main

import "example.com/elsewhere/httpapi"

var notOurs = []httpapi.BusinessEndpoint{{}, {}, {}}
`),
		"cmd/parcel-dispatch/assemble.go": file(`package main

import (
	"go.idp.xyz/idp-bento-go/eventing"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

func wireDispatcher(a, b, c dispatch.Consumer) map[eventing.EventType]dispatch.Consumer {
	return map[eventing.EventType]dispatch.Consumer{
		psinbox.ShipmentRequestSubmittedEventType: a,
		psinbox.ManualReviewCompletedEventType:    b,
		nrinbox.AcceptedDecisionEventType:         c,
	}
}
`),
		"cmd/parcel-dispatch/assemble_test.go": file(`package main

import (
	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

var probeRoutes = map[eventing.EventType]dispatch.Consumer{"probe": nil}
`),
		"internal/parcelshipment/adapters/inbox/submitted_consumer.go":        file("package psinbox"),
		"internal/parcelshipment/adapters/inbox/submitted_consumer_test.go":   file("package psinbox"),
		"internal/parcelshipment/adapters/inbox/node_intake_consumer.go":      file("package psinbox"),
		"internal/parcelshipment/adapters/adoptconsume/adoptconsume.go":       file("package adoptconsume"),
		"internal/parcelshipment/adapters/postgres/store.go":                  file("package postgres"),
		"internal/visibilityexception/adapters/veconsume/veconsume.go":        file("package veconsume"),
		"internal/visibilityexception/adapters/inbox/node_intake_consumer.go": file("package veinbox"),
		// 不在 adapters/ 下、只是碰巧叫 inbox 的目录不是消费门。
		"internal/networkrouting/inbox/helper.go": file("package inbox"),
	}
}

func tallyOf(t *testing.T, tallies []ContextTally, name string) int {
	t.Helper()
	for _, tally := range tallies {
		if tally.Context == name {
			return tally.Count
		}
	}
	t.Fatalf("清点结果里没有 %q", name)
	return 0
}

// 端点按装配点里的条目数，不按处理器文件数。测试文件与别的导入路径下的同名类型都不算。
func TestEndpointsCountAssembledEntriesNotHandlerFiles(t *testing.T) {
	census, err := CensusWiring(wiringFixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if census.EndpointTotal != 4 {
		t.Errorf("接入面端点 = %d，想要 4（测试脚手架与别的 httpapi 不算）", census.EndpointTotal)
	}
	if got := tallyOf(t, census.Endpoints, "parcelshipment"); got != 2 {
		t.Errorf("parcelshipment 端点 = %d，想要 2", got)
	}
	if got := tallyOf(t, census.Endpoints, "nodeoperations"); got != 1 {
		t.Errorf("nodeoperations 端点 = %d，想要 1", got)
	}
}

// Handler 不是一次直接的 adapters/http 构造调用时，条目照数、归属显成未归类，不吞掉也不猜。
func TestEndpointWithoutAConstructorCallIsCountedButUnattributed(t *testing.T) {
	census, err := CensusWiring(wiringFixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if got := tallyOf(t, census.Endpoints, unattributed); got != 1 {
		t.Errorf("未归类端点 = %d，想要 1", got)
	}
	if last := census.Endpoints[len(census.Endpoints)-1].Context; last != unattributed {
		t.Errorf("未归类应排在最后，实际最后一行是 %q", last)
	}
}

// 路由表按字面量条目数，键的包决定消费方；测试文件里的表不算。
func TestRoutesCountTableEntriesByConsumerContext(t *testing.T) {
	census, err := CensusWiring(wiringFixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if census.RouteTotal != 3 {
		t.Errorf("路由表条目 = %d，想要 3（测试文件里的探针表不算）", census.RouteTotal)
	}
	if got := tallyOf(t, census.Routes, "parcelshipment"); got != 2 {
		t.Errorf("parcelshipment 路由 = %d，想要 2", got)
	}
	if got := tallyOf(t, census.Routes, "networkrouting"); got != 1 {
		t.Errorf("networkrouting 路由 = %d，想要 1", got)
	}
}

// 消费适配器只认 adapters/ 下四类目录的生产文件：测试文件不算，postgres 不算，不在 adapters/
// 下的同名目录不算。
func TestConsumersCountOnlyTheFourGateDirectories(t *testing.T) {
	census, err := CensusWiring(wiringFixture())
	if err != nil {
		t.Fatalf("清点失败：%v", err)
	}
	if census.ConsumerTotal != 5 {
		t.Errorf("消费适配器 = %d，想要 5", census.ConsumerTotal)
	}
	byConsumer := map[string]ConsumerTally{}
	for _, tally := range census.Consumers {
		byConsumer[tally.Consumer] = tally
	}
	ps := byConsumer["parcelshipment"]
	if ps.Total != 3 || ps.ByDir["inbox"] != 2 || ps.ByDir["adoptconsume"] != 1 {
		t.Errorf("parcelshipment 消费适配器 = %d（inbox %d、adoptconsume %d），想要 3（2、1）",
			ps.Total, ps.ByDir["inbox"], ps.ByDir["adoptconsume"])
	}
	ve := byConsumer["visibilityexception"]
	if ve.Total != 2 || ve.ByDir["veconsume"] != 1 || ve.ByDir["inbox"] != 1 {
		t.Errorf("visibilityexception 消费适配器 = %d，想要 2（inbox 1、veconsume 1）", ve.Total)
	}
	if _, ok := byConsumer["networkrouting"]; ok {
		t.Error("networkrouting 那个不在 adapters/ 下的 inbox 目录被当成了消费门")
	}
	if totals := census.ConsumerDirTotals(); totals[0] != 3 || totals[1] != 1 || totals[2] != 0 || totals[3] != 1 {
		t.Errorf("四类目录合计 = %v，想要 [3 1 0 1]", totals)
	}
}

// 源文件解析不了要响亮报错，不能把那个文件当零条数过去——数出一个假的 0 比数不出来更糟。
func TestUnparsableProductionFileFailsTheCensus(t *testing.T) {
	fixture := wiringFixture()
	fixture["cmd/parcel-api/broken.go"] = &fstest.MapFile{Data: []byte("package main\n\nfunc (")}
	if _, err := CensusWiring(fixture); err == nil {
		t.Fatal("解析失败的生产文件应让清点报错，实际静默通过")
	}
}
