package whatsmeow_service

import (
	"sync"

	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

// Versão lida do WhatsApp Web em 02/11/2026, para quando a busca falhar: a padrão
// do whatsmeow envelhece e o WhatsApp recusa com "Client outdated (405)".
var fallbackWAVersion = clientVersion{Major: 2, Minor: 3000, Patch: 1052682108}

// Capturada na carga do pacote, antes de qualquer SetWAVersion (que é global).
var libraryWAVersion = store.GetWAVersion()

var applyWAVersionMu sync.Mutex

func (v clientVersion) complete() bool {
	return v.Major > 0 && v.Minor > 0 && v.Patch > 0
}

func (v clientVersion) container() store.WAVersionContainer {
	return store.WAVersionContainer{uint32(v.Major), uint32(v.Minor), uint32(v.Patch)}
}

func newerWAVersion(a, b store.WAVersionContainer) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// resolveWAVersion escolhe a versão anunciada: a configurada, a do WhatsApp Web ou,
// sem as duas, a mais nova entre a reserva e a do whatsmeow. Devolve também a origem.
func resolveWAVersion(configured clientVersion, web *clientVersion) (store.WAVersionContainer, string) {
	if configured.complete() {
		return configured.container(), "config"
	}
	if web != nil && web.complete() {
		return web.container(), "web"
	}
	if newerWAVersion(fallbackWAVersion.container(), libraryWAVersion) {
		return fallbackWAVersion.container(), "fallback"
	}
	return libraryWAVersion, "whatsmeow"
}

// applyWAVersion grava a versão no handshake (SetWAVersion) e no cadastro do aparelho.
// Só o DeviceProps não basta: o WhatsApp valida a versão do handshake.
func applyWAVersion(v store.WAVersionContainer) {
	applyWAVersionMu.Lock()
	defer applyWAVersionMu.Unlock()

	if store.GetWAVersion() != v {
		store.SetWAVersion(v)
	}
	store.DeviceProps.Version.Primary = proto.Uint32(v[0])
	store.DeviceProps.Version.Secondary = proto.Uint32(v[1])
	store.DeviceProps.Version.Tertiary = proto.Uint32(v[2])
}
