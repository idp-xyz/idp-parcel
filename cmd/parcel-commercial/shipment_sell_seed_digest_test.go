package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 逐委托销售这一户的合同和结算政策，声明的摘要必须与按正文算出的那一串相同。
func TestTheShipmentSellSeedDigestsMatchTheCanonicalBodies(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "commercial", "publish-shipment-sell-contract.json"))
	if err != nil {
		t.Fatalf("读种子：%v", err)
	}
	commands, err := publishCommandsFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	handler := pcapplication.NewPublishCommercialAuthorityHandler(stopAfterDigest{}, nil, nil)
	for _, command := range commands {
		result, err := handler.Handle(context.Background(), command)
		if err != nil {
			if strings.Contains(err.Error(), "digest matched; stop") {
				continue
			}
			t.Fatalf("%s：%v", command.Spec.ObjectID, err)
		}
		_, computed, ok := result.DigestReconciliation()
		if ok {
			t.Fatalf("%s 声明 %s，算出 %s", command.Spec.ObjectID, command.Spec.ContentDigest, computed)
		}
		t.Fatalf("%s 没有比摘要，也没有停在读册之前：%s", command.Spec.ObjectID, result.Outcome())
	}
}
