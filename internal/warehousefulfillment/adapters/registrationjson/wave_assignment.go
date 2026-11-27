package registrationjson

import (
	"io"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type waveAssignmentDocument struct {
	carriedIdentity
	WaveID *string `json:"waveId"`
	// ZoneCode 带了就只派这个波次里这一个库区的格；不带是派整个波次。空白不是「整个波次」的写法。
	ZoneCode *string `json:"zoneCode"`
	// Assignee 不带是取消指派；带了就得是一个操作者标识，空串不是「取消」的写法。
	Assignee *string `json:"assignee"`
}

// AssignWaveCommand 把指派拣货人译成命令。派的人取自身份。
func AssignWaveCommand(reader io.Reader) (application.AssignWaveCommand, error) {
	var none application.AssignWaveCommand
	document, err := decodeDocument[waveAssignmentDocument](reader, "wave assignment")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	wave, err := requiredText("waveId", document.WaveID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	command := application.AssignWaveCommand{Wave: wave}
	if document.ZoneCode != nil {
		zone, err := domain.NewZoneCode(*document.ZoneCode)
		if err != nil {
			return none, malformed("zoneCode must not be blank")
		}
		command.Zone = zone
	}
	if document.Assignee != nil {
		assignee := strings.TrimSpace(*document.Assignee)
		if assignee == "" {
			return none, malformed("assignee must not be blank")
		}
		command.Assignee = assignee
	}
	return command, nil
}

type waveClaimDocument struct {
	carriedIdentity
	Warehouse *string `json:"warehouse"`
	WaveID    *string `json:"waveId"`
}

// ClaimWaveCommand 把拣货的人领波次译成命令。领的人取自作业事实的认证，不从载荷来。
func ClaimWaveCommand(reader io.Reader) (application.ClaimWaveCommand, error) {
	var none application.ClaimWaveCommand
	document, err := decodeDocument[waveClaimDocument](reader, "wave claim")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	wave, err := requiredText("waveId", document.WaveID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	return application.ClaimWaveCommand{Warehouse: warehouse, Wave: wave}, nil
}
