package main

import (
	"context"
	"errors"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
)

type recordingTransactor struct {
	commits int
}

func (transactor *recordingTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	if err := fn(ctx); err != nil {
		return err
	}
	transactor.commits++
	return nil
}

func TestABadRowDoesNotRollBackRowsThatAlreadyLanded(t *testing.T) {
	transactor := &recordingTransactor{}
	rows := []batchRow{{TenantID: "SYN-TENANT-01", Revision: []byte(`{}`)}, {TenantID: "SYN-TENANT-01", Revision: []byte(`{}`)}, {TenantID: "SYN-TENANT-01", Revision: []byte(`{}`)}}
	calls := 0
	reports := applyBatch(context.Background(), transactor, rows, func(context.Context, batchRow) (string, string, error) {
		calls++
		switch calls {
		case 1:
			return "REGISTERED", "", nil
		case 2:
			return "", "", errUsage("revision is required")
		default:
			return "", "", errors.New("registry down")
		}
	})
	if transactor.commits != 1 || !reports[0].Settled || reports[1].Detail == "" || !reports[2].Technical {
		t.Fatalf("commits %d reports %+v", transactor.commits, reports)
	}
	if exitCode(reports) != exitUndecided {
		t.Fatalf("exit %d", exitCode(reports))
	}
}

func TestReplayingLandedRowsAnswersAlreadyRegistered(t *testing.T) {
	transactor := &recordingTransactor{}
	rows := []batchRow{{TenantID: "SYN-TENANT-01"}, {TenantID: "SYN-TENANT-01"}}
	reports := applyBatch(context.Background(), transactor, rows, func(context.Context, batchRow) (string, string, error) {
		return "ALREADY_REGISTERED", "", nil
	})
	if transactor.commits != 2 || exitCode(reports) != exitSettled {
		t.Fatalf("commits %d exit %d", transactor.commits, exitCode(reports))
	}
}

func TestARefusedRowKeepsTheEarlierRegistration(t *testing.T) {
	transactor := &recordingTransactor{}
	rows := []batchRow{{}, {}}
	step := 0
	reports := applyBatch(context.Background(), transactor, rows, func(context.Context, batchRow) (string, string, error) {
		step++
		if step == 1 {
			return "REGISTERED", "", nil
		}
		return "NOT_ACCEPTED", "CONTENT_INVALID", nil
	})
	if transactor.commits != 2 || !reports[0].Settled || reports[1].Outcome != "NOT_ACCEPTED" || exitCode(reports) != exitRefused {
		t.Fatalf("commits %d %+v exit %d", transactor.commits, reports, exitCode(reports))
	}
	conflict := applyBatch(context.Background(), transactor, rows[:1], func(context.Context, batchRow) (string, string, error) {
		return "CONTENT_CONFLICT", "", nil
	})
	if exitCode(conflict) != exitConflict {
		t.Fatalf("conflict exit %d", exitCode(conflict))
	}
}

func TestTheWarehouseRowUsesTheOnlineTranslator(t *testing.T) {
	registrant, err := domainRegistrant()
	if err != nil {
		t.Fatal(err)
	}
	_, err = registrationjson.WarehouseCommand(bytesReader(`{"tenantId":"SYN-TENANT-01","warehouseCode":"SYN-WH-01","revision":1,"name":"合成一号仓","kind":"SELF_OPERATED","address":{"country":"CN","text":"合成路 1 号"}}`), tenantID(t), registrant)
	if err == nil {
		t.Fatal("报文自带租户也被收下了")
	}
	command, err := registrationjson.WarehouseCommand(bytesReader(`{"warehouseCode":"SYN-WH-01","revision":1,"name":"合成一号仓","kind":"SELF_OPERATED","address":{"country":"CN","text":"合成路 1 号"}}`), tenantID(t), registrant)
	if err != nil || command.Registrant.String() != registeredBy || command.Code.String() != "SYN-WH-01" {
		t.Fatalf("command %s %s err %v", command.Registrant, command.Code, err)
	}
}

func TestDecodeBatchRejectsAnEmptyFileAndUnknownFields(t *testing.T) {
	if _, err := decodeBatch([]byte(`[]`)); err == nil {
		t.Fatal("空数组被收下了")
	}
	if _, err := decodeBatch([]byte(`[{"tenantId":"SYN-TENANT-01","revision":{},"extra":1}]`)); err == nil {
		t.Fatal("未知字段被收下了")
	}
	rows, err := decodeBatch([]byte(`[{"tenantId":"SYN-TENANT-01","revision":{"warehouseCode":"SYN-WH-01"}}]`))
	if err != nil || len(rows) != 1 || rows[0].TenantID != "SYN-TENANT-01" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
}
