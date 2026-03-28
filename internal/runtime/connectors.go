package runtime

import (
	"strings"

	"github.com/canhta/gistclaw/internal/conversations"
	"github.com/canhta/gistclaw/internal/model"
)

func (r *Runtime) SetConnectors(connectors []model.Connector) {
	catalog := builtinConnectorCatalog()
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		meta := model.NormalizeConnectorMetadata(connector.Metadata())
		if meta.ID == "" {
			continue
		}
		catalog[meta.ID] = meta
	}
	r.connectors = catalog
}

func builtinConnectorCatalog() map[string]model.ConnectorMetadata {
	return map[string]model.ConnectorMetadata{
		conversations.LocalWebConnectorID: model.NormalizeConnectorMetadata(model.ConnectorMetadata{
			ID:       conversations.LocalWebConnectorID,
			Exposure: model.ConnectorExposureLocal,
		}),
	}
}

func (r *Runtime) connectorMetadata(connectorID string) (model.ConnectorMetadata, bool) {
	connectorID = strings.TrimSpace(connectorID)
	if connectorID == "" {
		return model.ConnectorMetadata{}, false
	}
	if len(r.connectors) == 0 {
		return model.ConnectorMetadata{}, false
	}
	meta, ok := r.connectors[connectorID]
	if !ok {
		return model.ConnectorMetadata{}, false
	}
	return meta, true
}

func (r *Runtime) connectorExposure(connectorID string) model.ConnectorExposure {
	meta, ok := r.connectorMetadata(connectorID)
	if !ok {
		return model.ConnectorExposureRemote
	}
	return meta.Exposure
}

func (r *Runtime) shouldQueueOutbound(connectorID string) bool {
	return strings.TrimSpace(connectorID) != "" && r.connectorExposure(connectorID) != model.ConnectorExposureLocal
}

func (r *Runtime) isRemoteConnector(connectorID string) bool {
	if strings.TrimSpace(connectorID) == "" {
		return false
	}
	return r.connectorExposure(connectorID) == model.ConnectorExposureRemote
}
