package main

import (
	"errors"
	"fmt"
	"sort"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件是派发路由的登记处（ADR-0222）：各 routes_<特性>.go 在 init 里把自己那几路登进 routeRegistry，wireDispatcher
// 展开它。加一条路由、给已有扇出加一路，只动那个特性的文件，不动这里，也不动 wireDispatcher。

// fanOutPosition 是一路在同一类信封扇出里的名次，小者先调。FanOut 每一路都调、各记各的 inbox 账，名次不改记账；它定的是
// 同一封信上谁先落地——一路停在未决重投时，排在它前面已提交的各路按 inbox 跳过，不重复做。同一锚点下要分先后时写成
// 锚点加偏移，偏移的理由写在那一路的登记处。
type fanOutPosition int

const (
	// positionProjection：VE 投影与补派生先行，不把它们堵在资格墙、终局规则墙或路由证据墙上。
	positionProjection fanOutPosition = 100
	// positionPrimary：这一类信封的主业务那一路。
	positionPrimary fanOutPosition = 200
	// positionBilling：计费那一路（SA 发起评价），排在履约与追踪之后——计费不挡履约与追踪。
	positionBilling fanOutPosition = 300
	// positionLast：殿后。TF 实际履约发生项（ADR-0218）停在未决重投时，前面各路按 inbox 跳过，不重复投影与终局。殿后只保住
	// 同一封信上的各路；它停住时同一分区后面的信封仍排在它后面，代价与去处见 ADR-0218 越权风险点。
	positionLast fanOutPosition = 900
)

// routeLeg 是一路：某一类信封交给某个消费者。
type routeLeg struct {
	event    eventing.EventType
	position fanOutPosition
	consumer dispatch.Consumer
	// sentinels 是本路在路由条目处翻译的未决哨兵（dispatch.WithUndecidedSentinels）。翻译落在这一层的理由在那个函数的头注。
	sentinels []error
	// untranslated 为真：本路不在路由条目处翻译未决——消费者自己翻了，或它没有未决面。与 sentinels 二选一，两样都不写是漏写，
	// 装配即拒：漏翻的未决会塌成 dispatch.publish_failed，运维据此去查传输，而实际要查的是消费方等的那个依赖。
	untranslated bool
	// label 只进装配失败的错误正文。
	label string
}

// dispatchSources 是各路构造消费者时共用的件。projectionDerive 是 VE 各投影共用的那一只租户绑定派生，建一次。
type dispatchSources struct {
	db               *bentopg.DB
	outbox           *outbox.Store
	inbox            *inbox.Store
	settings         dispatchSettings
	clock            systemClock
	projectionDerive *tenantBoundProjectionDerive
}

// routeRegistry 只由各特性文件的 init 追加；init 里不构造任何东西，构造发生在 wireDispatcher 开好存储之后。
var routeRegistry []func(dispatchSources) ([]routeLeg, error)

// registerRoutes 登记一个特性的各路。构造函数交回的错误原样上抛，装配点据此带原因退出。
func registerRoutes(build func(dispatchSources) ([]routeLeg, error)) {
	if build == nil {
		panic("parcel-dispatch: a route feature needs a build function")
	}
	routeRegistry = append(routeRegistry, build)
}

// assembleRoutes 展开登记处：逐特性构造各路，再交 routeTable 排成路由表。
func assembleRoutes(sources dispatchSources) (map[eventing.EventType]dispatch.Consumer, error) {
	var legs []routeLeg
	for _, build := range routeRegistry {
		built, err := build(sources)
		if err != nil {
			return nil, err
		}
		legs = append(legs, built...)
	}
	return routeTable(legs)
}

// routeTable 按事件类型归组、按名次排，逐路包哨兵，一路的直接进路由表，多于一路才 FanOut。
// 各路先包自己的哨兵再 FanOut：禁止把几路哨兵合成一份再包 FanOut，FanOut 不翻译。
func routeTable(legs []routeLeg) (map[eventing.EventType]dispatch.Consumer, error) {
	lanes := map[eventing.EventType][]routeLeg{}
	for _, leg := range legs {
		lanes[leg.event] = append(lanes[leg.event], leg)
	}
	routes := make(map[eventing.EventType]dispatch.Consumer, len(lanes))
	for event, lane := range lanes {
		sort.SliceStable(lane, func(i, j int) bool { return lane[i].position < lane[j].position })
		consumers := make([]dispatch.Consumer, 0, len(lane))
		for index, leg := range lane {
			if index > 0 && leg.position == lane[index-1].position {
				return nil, fmt.Errorf("parcel-dispatch: %s: %s and %s share fan-out position %d; the order must be declared, not inherited from file names",
					event, lane[index-1].label, leg.label, leg.position)
			}
			consumer, err := translateUndecided(leg)
			if err != nil {
				return nil, err
			}
			consumers = append(consumers, consumer)
		}
		if len(consumers) == 1 {
			routes[event] = consumers[0]
			continue
		}
		fan, err := dispatch.FanOut(consumers...)
		if err != nil {
			return nil, fmt.Errorf("parcel-dispatch: %s fan-out: %w", event, err)
		}
		routes[event] = fan
	}
	return routes, nil
}

func translateUndecided(leg routeLeg) (dispatch.Consumer, error) {
	switch {
	case leg.untranslated && len(leg.sentinels) > 0:
		return nil, fmt.Errorf("parcel-dispatch: %s declares sentinels and untranslated at once", leg.label)
	case leg.untranslated:
		if leg.consumer == nil {
			return nil, fmt.Errorf("parcel-dispatch: %s: %w", leg.label, errors.New("consumer is required"))
		}
		return leg.consumer, nil
	case len(leg.sentinels) == 0:
		return nil, fmt.Errorf("parcel-dispatch: %s declares no undecided sentinels; list them or mark the leg untranslated", leg.label)
	}
	routed, err := dispatch.WithUndecidedSentinels(leg.consumer, leg.sentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: %s undecided translation: %w", leg.label, err)
	}
	return routed, nil
}
