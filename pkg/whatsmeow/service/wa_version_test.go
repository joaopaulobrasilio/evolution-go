package whatsmeow_service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/store"
)

var versaoDaWeb = clientVersion{Major: 2, Minor: 3000, Patch: 1052747707}

func restaurarVersaoGlobal(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { applyWAVersion(libraryWAVersion) })
}

func TestResolveWAVersionPrefereAConfigurada(t *testing.T) {
	configurada := clientVersion{Major: 2, Minor: 3000, Patch: 1050000000}

	got, origem := resolveWAVersion(configurada, &versaoDaWeb)

	if got != configurada.container() || origem != "config" {
		t.Fatalf("esperava a configurada, veio %v (%s)", got, origem)
	}
}

func TestResolveWAVersionIgnoraConfigIncompleta(t *testing.T) {
	for _, configurada := range []clientVersion{
		{},
		{Major: 2, Minor: 3000},
		{Major: 2, Patch: 1050000000},
		{Minor: 3000, Patch: 1050000000},
	} {
		got, origem := resolveWAVersion(configurada, &versaoDaWeb)
		if got != versaoDaWeb.container() || origem != "web" {
			t.Fatalf("config %+v: esperava a da web, veio %v (%s)", configurada, got, origem)
		}
	}
}

func TestResolveWAVersionUsaAReservaQuandoABuscaFalha(t *testing.T) {
	for _, web := range []*clientVersion{nil, {Major: 2, Minor: 3000}} {
		got, origem := resolveWAVersion(clientVersion{}, web)
		if got != fallbackWAVersion.container() || origem != "fallback" {
			t.Fatalf("web %+v: esperava a reserva, veio %v (%s)", web, got, origem)
		}
	}
}

func TestResolveWAVersionNaoRebaixaAVersaoDoWhatsmeow(t *testing.T) {
	original := libraryWAVersion
	t.Cleanup(func() { libraryWAVersion = original })
	libraryWAVersion = store.WAVersionContainer{2, 3000, 1099999999}

	got, origem := resolveWAVersion(clientVersion{}, nil)

	if got != libraryWAVersion || origem != "whatsmeow" {
		t.Fatalf("esperava a do whatsmeow, mais nova que a reserva; veio %v (%s)", got, origem)
	}
}

func TestReservaEMaisNovaQueAVersaoRecusada(t *testing.T) {
	recusada := store.WAVersionContainer{2, 3000, 1042386815}

	if !newerWAVersion(fallbackWAVersion.container(), recusada) {
		t.Fatalf("a reserva %v precisa ser mais nova que %v", fallbackWAVersion, recusada)
	}
}

func TestNewerWAVersion(t *testing.T) {
	casos := []struct {
		a, b store.WAVersionContainer
		want bool
	}{
		{store.WAVersionContainer{2, 3000, 2}, store.WAVersionContainer{2, 3000, 1}, true},
		{store.WAVersionContainer{2, 3000, 1}, store.WAVersionContainer{2, 3000, 2}, false},
		{store.WAVersionContainer{2, 3000, 1}, store.WAVersionContainer{2, 3000, 1}, false},
		{store.WAVersionContainer{3, 0, 0}, store.WAVersionContainer{2, 9999, 9999}, true},
		{store.WAVersionContainer{2, 2999, 9999}, store.WAVersionContainer{2, 3000, 0}, false},
	}
	for _, c := range casos {
		if got := newerWAVersion(c.a, c.b); got != c.want {
			t.Fatalf("newerWAVersion(%v, %v) = %v, esperava %v", c.a, c.b, got, c.want)
		}
	}
}

// Regressão do 405: a versão ia só para o DeviceProps e o handshake seguia com a do whatsmeow.
func TestApplyWAVersionAtualizaOHandshake(t *testing.T) {
	restaurarVersaoGlobal(t)
	v := versaoDaWeb.container()

	applyWAVersion(v)

	if store.GetWAVersion() != v {
		t.Fatalf("GetWAVersion = %v, esperava %v", store.GetWAVersion(), v)
	}
	app := store.BaseClientPayload.GetUserAgent().GetAppVersion()
	if app.GetPrimary() != v[0] || app.GetSecondary() != v[1] || app.GetTertiary() != v[2] {
		t.Fatalf("handshake com %d.%d.%d, esperava %v", app.GetPrimary(), app.GetSecondary(), app.GetTertiary(), v)
	}
	dev := store.DeviceProps.GetVersion()
	if dev.GetPrimary() != v[0] || dev.GetSecondary() != v[1] || dev.GetTertiary() != v[2] {
		t.Fatalf("DeviceProps com %d.%d.%d, esperava %v", dev.GetPrimary(), dev.GetSecondary(), dev.GetTertiary(), v)
	}
}

func TestApplyWAVersionRepetidoEConcorrente(t *testing.T) {
	restaurarVersaoGlobal(t)
	v := versaoDaWeb.container()

	feito := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			applyWAVersion(v)
			feito <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-feito
	}

	if store.GetWAVersion() != v {
		t.Fatalf("GetWAVersion = %v, esperava %v", store.GetWAVersion(), v)
	}
}

func servirSW(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	urlOriginal, clienteOriginal := whatsAppWebSWURL, webVersionHTTPClient
	whatsAppWebSWURL = srv.URL
	cachedWebVersion = nil
	t.Cleanup(func() {
		srv.Close()
		whatsAppWebSWURL, webVersionHTTPClient = urlOriginal, clienteOriginal
		cachedWebVersion = nil
	})
}

func TestFetchWhatsAppWebVersionLeARevisao(t *testing.T) {
	servirSW(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `self.__swData={"client_revision":1052682108,"server_revision":1}`)
	})

	got, err := fetchWhatsAppWebVersion()

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if *got != (clientVersion{Major: 2, Minor: 3000, Patch: 1052682108}) {
		t.Fatalf("versão lida %+v", *got)
	}
}

func TestFetchWhatsAppWebVersionSemRevisao(t *testing.T) {
	servirSW(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `self.__swData={}`)
	})

	if _, err := fetchWhatsAppWebVersion(); err == nil {
		t.Fatal("esperava erro sem client_revision")
	}
}

func TestFetchWhatsAppWebVersionDesisteNoTimeout(t *testing.T) {
	servirSW(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	webVersionHTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	inicio := time.Now()
	_, err := fetchWhatsAppWebVersion()

	if err == nil {
		t.Fatal("esperava erro de timeout")
	}
	if time.Since(inicio) > time.Second {
		t.Fatalf("demorou %v; o timeout não foi respeitado", time.Since(inicio))
	}
}
