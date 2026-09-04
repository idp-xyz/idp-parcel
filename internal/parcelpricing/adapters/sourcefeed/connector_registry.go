package sourcefeed

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ConnectorRegistry 实现 ports.SourceConnectorResolver：装配方把本部署装了的连接器登进来，
// 编排按绑定声明的种类取。装了什么由装配决定，本表不预装任何一家——出厂零真实来源。
type ConnectorRegistry struct {
	byKind map[string]ports.SourceConnector
}

// NewConnectorRegistry 同种类装两个是装配错误，构造期拒：运行期二选一会让「用了哪个」无从解释。
func NewConnectorRegistry(connectors ...ports.SourceConnector) (ConnectorRegistry, error) {
	byKind := make(map[string]ports.SourceConnector, len(connectors))
	for _, connector := range connectors {
		if connector == nil || connector.Kind() == "" {
			return ConnectorRegistry{}, fmt.Errorf("parcel pricing source feed: connector without a kind")
		}
		if _, duplicated := byKind[connector.Kind()]; duplicated {
			return ConnectorRegistry{}, fmt.Errorf("parcel pricing source feed: connector kind %q registered twice", connector.Kind())
		}
		byKind[connector.Kind()] = connector
	}
	return ConnectorRegistry{byKind: byKind}, nil
}

func (registry ConnectorRegistry) ConnectorFor(kind string) (ports.SourceConnector, bool) {
	connector, found := registry.byKind[kind]
	return connector, found
}

var _ ports.SourceConnectorResolver = ConnectorRegistry{}
