package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉 DeclaredDemand 对 PS 申报测量各格的全函数翻译（票 demo-chain-gaps/18，ADR-0201 决定八）：基线锚带测量、已采用
// 版本锚带与不带测量、未定 / 未申报 / 无各一例，读口 error 原样上抛、集外取值答译不过去。夹具全部合成。

type measurementLookupStub struct {
	resolution psdomain.DeclaredMeasurementResolution
	err        error
	tenant     string
	parcel     string
}

func (stub *measurementLookupStub) LoadDeclaredMeasurement(
	_ context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.DeclaredMeasurementResolution, error) {
	stub.tenant, stub.parcel = tenant.String(), parcel.String()
	return stub.resolution, stub.err
}

func declaredMeasurement(t *testing.T, withDimensions bool) psdomain.DeclaredMeasurement {
	t.Helper()
	weight, err := psdomain.NewDeclaredWeight(
		mustPS(t, psdomain.NewMeasurementValue, "1.50"), mustPS(t, psdomain.NewMeasurementUnitReference, "KG"))
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	var dimensions psdomain.DeclaredDimensions
	if withDimensions {
		if dimensions, err = psdomain.NewDeclaredDimensions(
			mustPS(t, psdomain.NewMeasurementValue, "20"), mustPS(t, psdomain.NewMeasurementValue, "15"),
			mustPS(t, psdomain.NewMeasurementValue, "10"), mustPS(t, psdomain.NewMeasurementUnitReference, "CM")); err != nil {
			t.Fatalf("外廓：%v", err)
		}
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		t.Fatalf("测量：%v", err)
	}
	return measurement
}

func loadDemand(t *testing.T, stub *measurementLookupStub) (tfdomain.DemandSnapshot, error) {
	t.Helper()
	source, err := adapter.NewDeclaredDemand(stub)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return source.LoadDeclaredDemand(t.Context(),
		mustPS(t, tfdomain.NewTenantID, "SYN-TENANT-01"), mustPS(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-01"))
}

func demandWords(snapshot tfdomain.DemandSnapshot) map[string]string {
	words := map[string]string{}
	for _, quantity := range snapshot.Quantities() {
		words[quantity.Dimension.String()] = quantity.Amount + " " + quantity.Unit.String()
	}
	return words
}

// Covers: AT-TF-132——PS 的每一格都有落点：基线锚记 `baseline` 带重量、体积、件数；已采用版本锚记那一版标识，那一版清空了测量
// 只剩件数、绝不回退基线值；未定、未申报、无都不带锚、只剩件数；按租户与对象原值问。
func TestDeclaredDemandTranslatesEveryMeasurementOutcome(t *testing.T) {
	onBaseline, err := psdomain.DeclaredMeasurementOnBaseline(declaredMeasurement(t, true))
	if err != nil {
		t.Fatalf("基线格：%v", err)
	}
	anchor, err := psdomain.NewSourceDataVersionAnchor(mustPS(t, psdomain.NewSourceDataVersionID, "SYN-SDV-2"))
	if err != nil {
		t.Fatalf("锚：%v", err)
	}
	adopted, err := psdomain.DeclaredMeasurementOnAdoptedVersion(anchor, declaredMeasurement(t, false))
	if err != nil {
		t.Fatalf("已采用版本格：%v", err)
	}
	cleared, err := psdomain.DeclaredMeasurementOnAdoptedVersion(anchor, psdomain.DeclaredMeasurement{})
	if err != nil {
		t.Fatalf("清空的已采用版本格：%v", err)
	}
	for name, test := range map[string]struct {
		resolution psdomain.DeclaredMeasurementResolution
		anchor     string
		words      map[string]string
	}{
		"基线锚":   {onBaseline, "baseline", map[string]string{"WEIGHT": "1.50 KG", "VOLUME": "3000 CM3", "PIECES": "1 PCS"}},
		"已采用版本": {adopted, "SYN-SDV-2", map[string]string{"WEIGHT": "1.50 KG", "PIECES": "1 PCS"}},
		"清空的版本": {cleared, "SYN-SDV-2", map[string]string{"PIECES": "1 PCS"}},
		"未定":    {psdomain.DeclaredMeasurementUndeterminedResolution(), "", map[string]string{"PIECES": "1 PCS"}},
		"未申报":   {psdomain.DeclaredMeasurementNotDeclaredResolution(), "", map[string]string{"PIECES": "1 PCS"}},
		"无":     {psdomain.NoDeclaredMeasurementResolution(), "", map[string]string{"PIECES": "1 PCS"}},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &measurementLookupStub{resolution: test.resolution}
			snapshot, err := loadDemand(t, stub)
			if err != nil {
				t.Fatalf("译：%v", err)
			}
			if stub.tenant != "SYN-TENANT-01" || stub.parcel != "SYN-PARCEL-01" {
				t.Fatalf("问的键 = %s / %s", stub.tenant, stub.parcel)
			}
			if got, _ := snapshot.Anchor(); got != test.anchor || snapshot.Source() != tfdomain.DemandFromDeclaration {
				t.Fatalf("锚 = %q 来源 = %s, want %q declaration", got, snapshot.Source(), test.anchor)
			}
			words := demandWords(snapshot)
			if len(words) != len(test.words) {
				t.Fatalf("各维 = %v, want %v", words, test.words)
			}
			for dimension, word := range test.words {
				if words[dimension] != word {
					t.Fatalf("%s = %q, want %q", dimension, words[dimension], word)
				}
			}
		})
	}
}

// Covers: X6、ADR-0029——PS 读不动原样上抛、不带译不过去的哨兵，由编排答未决，不折成「没报」；集外取值同时是本包的
// ErrUntranslatableAnswer 与端口的 ErrDeclaredDemandUntranslatable，编排认出它不答未决。
func TestDeclaredDemandPropagatesAnUnreadableShipment(t *testing.T) {
	down := errors.New("parcel shipment is down")
	if _, err := loadDemand(t, &measurementLookupStub{err: down}); !errors.Is(err, down) ||
		errors.Is(err, tfports.ErrDeclaredDemandUntranslatable) {
		t.Fatalf("读不动 = %v", err)
	}
	_, err := loadDemand(t, &measurementLookupStub{})
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) || !errors.Is(err, tfports.ErrDeclaredDemandUntranslatable) {
		t.Fatalf("集外取值 = %v", err)
	}
}
