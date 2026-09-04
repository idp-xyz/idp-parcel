package sourcefeed

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// InPlaceArtifactStore 是工件存放端口的「原地引用」实现（票 06 裁决二）：FileConnector 场景下
// 本体本来就躺在受控目录里，存放答 Accepted、定位符即来源地址——不复制字节、不引对象存储。
// 等对象存储存在时换的是这一个实现，端口与迁移不动。
type InPlaceArtifactStore struct{}

func NewInPlaceArtifactStore() InPlaceArtifactStore { return InPlaceArtifactStore{} }

func (InPlaceArtifactStore) Store(_ context.Context, record domain.PublishedRecord) (ports.ArtifactPlacement, error) {
	return ports.ArtifactPlacement{Outcome: outbound.Accept(), Locator: record.SourceLocator()}, nil
}

var _ ports.SourceArtifactStore = InPlaceArtifactStore{}
