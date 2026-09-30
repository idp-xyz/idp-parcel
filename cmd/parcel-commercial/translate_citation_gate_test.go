package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// Covers: ADR-0157 决定三——演示种子的时点语义引用经发布翻译立得起来，声明的摘要与原文一致。
func TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests(t *testing.T) {
	commands := publishCommands(t, demoSeed(t))
	for i, command := range commands {
		if err := citationGateAndDigest(command); err != nil {
			t.Fatalf("第 %d 项 %v：%v", i, command.Spec.Kind, err)
		}
	}
}

func TestAnUnreleasedAsOfCitationIsRefusedAtTranslation(t *testing.T) {
	raw := bytes.Replace(demoSeed(t),
		[]byte("REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1"),
		[]byte("REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@2"),
		1)
	if _, err := publishCommandsFromJSON(raw); err == nil {
		t.Fatal("未发布的引用过了发布翻译")
	}
}

func TestAMalformedAsOfCitationIsRefusedAtTranslation(t *testing.T) {
	raw := bytes.Replace(demoSeed(t),
		[]byte("REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1"),
		[]byte("REFCFG-1:not-a-reference"),
		1)
	if _, err := publishCommandsFromJSON(raw); err == nil {
		t.Fatal("坏形状的引用过了发布翻译")
	}
}

func demoSeed(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "scripts/demo-seeds/data/commercial/publish-batch.json"))
	if err != nil {
		t.Fatalf("读演示种子：%v", err)
	}
	return raw
}

func publishCommands(t *testing.T, raw []byte) []pcapplication.PublishCommercialAuthorityCommand {
	t.Helper()
	commands, err := publishCommandsFromJSON(raw)
	if err != nil {
		t.Fatalf("发布翻译：%v", err)
	}
	return commands
}

// 摘要对得上之后才会碰到空登记册。空指针恐慌说明翻译与摘要都过了；对不上或引用立不起来则在此之前返回错误。
func citationGateAndDigest(command pcapplication.PublishCommercialAuthorityCommand) (failed error) {
	handler := pcapplication.NewPublishCommercialAuthorityHandler(nil, nil, nil)
	defer func() {
		if recover() != nil {
			failed = nil
		}
	}()
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		return err
	}
	declared, computed, compared := result.DigestReconciliation()
	if compared && declared != computed {
		return errDigest(declared, computed)
	}
	return errStopped()
}

type digestMismatchError struct{ declared, computed string }

func (err digestMismatchError) Error() string {
	return "声明 " + err.declared + " 算出 " + err.computed
}

func errDigest(declared, computed string) error {
	return digestMismatchError{declared, computed}
}

type stoppedError struct{}

func (stoppedError) Error() string { return "在登记册之前停下，没有摘要拒因" }

func errStopped() error { return stoppedError{} }

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("工作目录：%v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到 go.mod")
		}
		dir = parent
	}
}
