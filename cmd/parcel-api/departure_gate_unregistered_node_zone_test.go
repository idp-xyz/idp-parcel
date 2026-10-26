package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// 本文件对真实 PostgreSQL 证租户漏登一个节点的关务区域时出发前问询与出发回给设备的那一格（ADR-0300；票 demo-chain-gaps/147）：
// 关务对不在册的包裹答未配置并指名未登的节点，运输履约照 ADR-0205 决定二译成未决，ADR-0237 只收窄得出放行的那几件，于是段里
// 每件都在列。证据只记 `S`。

// Covers: AT-TF-330 真库（ADR-0300，证据 S）——租户登过区域（上海枢纽在 CN）、漏登了深圳口岸：演示动线上自营上海→深圳一段装着
// 两个客户的包裹，都不在任何关务案卷里。出发前问询与随后同参出发都答未决，回给设备的逐对象那一格列出段里每一件、各答未决，两口
// 逐字节相同——如实的读数：跨区判不出、没立案，执行方确实要拿下这几件；而问的可以是任一场外设备、问询不留痕，那一格的收窄归
// 票 demo-chain-gaps/171（ADR-0237 越权风险点 2）。依据原文（节点区域未登记、指名深圳口岸、关务判断标识）不回给设备，只在出发
// 落册的明细里。同一段换成一条区域都没登的租户，问询答不受门禁、一件不列——纯境内租户行为不变（AT-CC-427 的消费侧半边）。
func TestAZonedTenantMissingANodeZoneShowsEveryUnbookedObjectAsUndecided(t *testing.T) {
	line := previewLineOn(t, newDepartureLineWithZones(t, "27-node-zone-sha-hub.json"))
	const (
		segment = "SYN-SEG-147-UNZONED-GATE"
		fact    = "SYN-MOVE-147-UNZONED-GATE"
	)
	parcels := []string{"SYN-PARCEL-147-CUST-A", "SYN-PARCEL-147-CUST-B"}
	for _, parcel := range parcels {
		line.handOver(segment, parcel)
	}

	asked := line.ask(segment)
	departed := line.departAs(fact, segment)
	if asked.Code != http.StatusOK || departed.Code != http.StatusCreated {
		t.Fatalf("问询 %d %s / 出发 %d %s", asked.Code, asked.Body.String(), departed.Code, departed.Body.String())
	}
	previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed)
	if previewed != recorded {
		t.Fatalf("问询与同参出发回给设备的不是同一份：\n问询 %s\n出发 %s", previewed, recorded)
	}
	reading := executorScopeOf(t, recorded)
	if reading.Answer != "UNDECIDED" || reading.From != "SYN-NODE-SHA-HUB" || reading.To != "SYN-NODE-SZX-GATE" {
		t.Fatalf("整格与起讫 = %s，want 未决、上海→深圳", recorded)
	}
	var shown []string
	for _, object := range reading.Objects {
		if object.Answer != "UNDECIDED" || object.Members != nil {
			t.Fatalf("%s 那一格 = %s，want 未决：%s", object.Object, object.Answer, recorded)
		}
		shown = append(shown, object.Object)
	}
	slices.Sort(shown)
	if !slices.Equal(shown, parcels) {
		t.Fatalf("回给设备的 = %v，want 段里每一件 %v", shown, parcels)
	}
	for _, leaked := range []string{"NODE_ZONE_NOT_REGISTERED", "AADJ-", "CROSS_CUSTOMS_MOVEMENT"} {
		if strings.Contains(asked.Body.String(), leaked) || strings.Contains(departed.Body.String(), leaked) {
			t.Fatalf("依据原文 %s 回给了设备：\n问询 %s\n出发 %s", leaked, asked.Body.String(), departed.Body.String())
		}
	}

	cells, unbound := line.recordedDetail(fact)
	if unbound || len(cells) != len(parcels) {
		t.Fatalf("库面 = %v，未绑定 %v，want 两件各一格、已绑定", cells, unbound)
	}
	for _, parcel := range parcels {
		if cells[parcel] != [2]string{"UNDECIDED", "NODE_ZONE_NOT_REGISTERED"} {
			t.Fatalf("库面 %s 那一格 = %v，want 未决、依据节点区域未登记", parcel, cells[parcel])
		}
	}
	var named int
	if err := line.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.transport_movement_fact f,
		        jsonb_array_elements(f.gate_reading_detail -> 'findings') finding,
		        jsonb_array_elements(finding -> 'bases') basis
		  WHERE f.fact_ref = $1 AND basis ->> 'action' = 'CROSS_CUSTOMS_MOVEMENT'
		    AND basis -> 'sources' = '["SYN-NODE-SZX-GATE"]'::jsonb`, fact).Scan(&named); err != nil {
		t.Fatalf("数库面依据：%v", err)
	}
	if named != len(parcels) {
		t.Fatalf("库面指名深圳口岸的依据 %d 条，want 每件一条", named)
	}

	domestic := previewLineOn(t, newDepartureLineWithZones(t))
	domestic.handOver(segment, parcels[0])
	answered := domestic.ask(segment)
	if answered.Code != http.StatusOK {
		t.Fatalf("纯境内租户问询 HTTP %d %s", answered.Code, answered.Body.String())
	}
	if reading := executorScopeOf(t, gateReadingBytes(t, answered)); reading.Answer != "NOT_GUARDED" || len(reading.Objects) != 0 {
		t.Fatalf("一条区域都没登的租户 = %s，want 不受门禁、一件不列", answered.Body.String())
	}
}
