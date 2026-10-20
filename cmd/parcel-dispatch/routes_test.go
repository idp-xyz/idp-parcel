package main

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件证路由登记处展开时的几道装配期护栏（ADR-0222 决定一、六、七）：它们不碰库，错在装配期就报，不等哪一拍撞上。

type recordingConsumer struct {
	name  string
	calls *[]string
}

func (consumer recordingConsumer) Consume(context.Context, eventing.Envelope) error {
	*consumer.calls = append(*consumer.calls, consumer.name)
	return nil
}

var errRouteTestUndecided = errors.New("route test undecided")

// Covers: ADR-0222 决定七 — 同一类信封上各路按名次调，与登记先后无关。
func TestFanOutLegsAreCalledByPositionNotByRegistrationOrder(t *testing.T) {
	var calls []string
	const event eventing.EventType = "route-test.event"
	routes, err := routeTable([]routeLeg{
		{event: event, position: positionLast, consumer: recordingConsumer{"last", &calls}, untranslated: true, label: "last"},
		{event: event, position: positionProjection, consumer: recordingConsumer{"projection", &calls}, untranslated: true, label: "projection"},
		{event: event, position: positionPrimary, consumer: recordingConsumer{"primary", &calls}, sentinels: []error{errRouteTestUndecided}, label: "primary"},
	})
	if err != nil {
		t.Fatalf("路由表：%v", err)
	}
	if err := routes[event].Consume(t.Context(), eventing.Envelope{}); err != nil {
		t.Fatalf("投递：%v", err)
	}
	if got := len(calls); got != 3 || calls[0] != "projection" || calls[1] != "primary" || calls[2] != "last" {
		t.Fatalf("调用次序 = %v，want [projection primary last]", calls)
	}
}

// Covers: ADR-0222 决定六 — 同一类信封两路名次相同，装配即拒：次序不许由文件名定。
func TestLegsSharingAFanOutPositionAreRefused(t *testing.T) {
	var calls []string
	const event eventing.EventType = "route-test.event"
	_, err := routeTable([]routeLeg{
		{event: event, position: positionPrimary, consumer: recordingConsumer{"a", &calls}, untranslated: true, label: "a"},
		{event: event, position: positionPrimary, consumer: recordingConsumer{"b", &calls}, untranslated: true, label: "b"},
	})
	if err == nil {
		t.Fatal("two legs at one position accepted")
	}
}

// Covers: ADR-0222 决定一 — 一路要么列哨兵、要么声明不在路由条目处翻译；两样都不写是漏写，装配即拒。
func TestALegMustListSentinelsOrDeclareItselfUntranslated(t *testing.T) {
	var calls []string
	cases := map[string]routeLeg{
		"neither": {event: "route-test.event", position: positionPrimary, consumer: recordingConsumer{"a", &calls}, label: "neither"},
		"both": {
			event: "route-test.event", position: positionPrimary, consumer: recordingConsumer{"a", &calls},
			sentinels: []error{errRouteTestUndecided}, untranslated: true, label: "both",
		},
		"no consumer": {event: "route-test.event", position: positionPrimary, untranslated: true, label: "no consumer"},
	}
	for name, leg := range cases {
		if _, err := routeTable([]routeLeg{leg}); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
