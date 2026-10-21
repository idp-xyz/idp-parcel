package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// buildDeliveryGatePreview 装配 `/transport-fulfillment/delivery-gate-previews` 背后的问询编排（ADR-0238；票 demo-chain-gaps/94）。
// 读数编排取 buildDeliveryGateReader，与有效交付登记同一处构造：同参交付与问询读数同一套，装配上不留第二种接法。派送尝试读面照
// 那一处的签名交进去，问询这一侧不读它。
//
// 不包事务，理由同 buildDepartureGatePreview：本上下文这一侧什么都不写，关务的判断在它自己那层壳的事务里记下。
func buildDeliveryGatePreview(db *bentopg.DB) (tfhttp.DeliveryGatePreviewer, error) {
	attempts, err := tfpostgres.NewDeliveryAttempts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate preview: delivery attempts: %w", err)
	}
	reader, err := buildDeliveryGateReader(db, attempts)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate preview: %w", err)
	}
	preview, err := tfapp.NewPreviewDeliveryGateHandler(reader)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate preview: %w", err)
	}
	return preview, nil
}
