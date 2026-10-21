package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// 本文件证价卡收尾两种登记的路由与退出码：文档译成命令（租户、方案、旧版、后继、依据五格照搬），截止与退役各进自己那一口；
// 自报截止点或退役时刻、坏 JSON 在到达用例之前拒成 1；未受理是治理答案译成 2，依赖故障译成 3。收尾判据由领域与真库用例证。

type closerDouble struct {
	result      application.PriceCardClosureResult
	err         error
	cuts        []application.CutPriceCardVersionCommand
	retirements []application.RetirePriceCardVersionCommand
}

func (double *closerDouble) Cut(_ context.Context, command application.CutPriceCardVersionCommand) (application.PriceCardClosureResult, error) {
	double.cuts = append(double.cuts, command)
	return double.result, double.err
}

func (double *closerDouble) RetireAtCoveringSuccessor(_ context.Context, command application.RetirePriceCardVersionCommand) (application.PriceCardClosureResult, error) {
	double.retirements = append(double.retirements, command)
	return double.result, double.err
}

const closureJSON = `{"tenant":"SYN-TENANT-01","planId":"SYN-PLAN-CN-SG-COST-01","version":"v1","successorVersion":"v2","reference":"SYN-CUT/2026-10-21"}`

func TestExecuteClosureRoutesEachKindToItsOwnFace(t *testing.T) {
	closer := &closerDouble{}
	execute := func(kind, raw string) int {
		_, code := executeClosure(t.Context(), kind, []byte(raw), closer, passthroughTransactor{})
		return code
	}
	execute(kindPriceCardCut, closureJSON)
	execute(kindPriceCardRetirement, closureJSON)
	if len(closer.cuts) != 1 || len(closer.retirements) != 1 {
		t.Fatalf("路由错位：cuts=%d retirements=%d", len(closer.cuts), len(closer.retirements))
	}
	cut := closer.cuts[0]
	if cut.Tenant.String() != "SYN-TENANT-01" || cut.PlanID != "SYN-PLAN-CN-SG-COST-01" || cut.Version != "v1" ||
		cut.Successor != "v2" || cut.Reference != "SYN-CUT/2026-10-21" {
		t.Fatalf("命令翻译走样：%+v", cut)
	}

	for name, raw := range map[string]string{
		"自报截止点":  strings.Replace(closureJSON, `"reference"`, `"selectionEndsAt":"2026-06-01T00:00:00Z","reference"`, 1),
		"自报退役时刻": strings.Replace(closureJSON, `"reference"`, `"retiredAt":"2026-06-01T00:00:00Z","reference"`, 1),
		"坏 JSON": `not json`,
		"没有租户":   `{"planId":"p","version":"v1","successorVersion":"v2","reference":"r"}`,
	} {
		if code := execute(kindPriceCardCut, raw); code != exitUsage {
			t.Fatalf("%s：code = %d，want %d", name, code, exitUsage)
		}
	}
	if len(closer.cuts) != 1 {
		t.Fatalf("坏文档到达了用例：%d", len(closer.cuts))
	}
}

func TestExecuteClosureTranslatesTheAnswer(t *testing.T) {
	if _, code := executeClosure(t.Context(), kindPriceCardCut, []byte(closureJSON),
		&closerDouble{err: errors.New("price card register is down")}, passthroughTransactor{}); code != exitUndecided {
		t.Fatalf("依赖故障 code = %d，want %d", code, exitUndecided)
	}
	for outcome, want := range map[application.PriceCardClosureOutcome]int{
		application.PriceCardClosureRecorded:          exitRegistered,
		application.PriceCardClosureAlreadyOnRegister: exitRegistered,
		application.PriceCardClosureNotAccepted:       exitGovernance,
	} {
		if code := closureExitCode(outcome); code != want {
			t.Fatalf("%s：code = %d，want %d", outcome, code, want)
		}
	}
	if !isClosureKind(kindPriceCardCut) || !isClosureKind(kindPriceCardRetirement) || isClosureKind(kindPriceCard) {
		t.Fatal("收尾种类认错了")
	}
}
