package gincollector

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	collector "github.com/cresenity/devcloud-collector-client-golang"
	"github.com/gin-gonic/gin"
)

func newTestCollector(t *testing.T) (*collector.Collector, string) {
	t.Helper()
	docRoot, err := os.MkdirTemp("", "gincollector-test-")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(docRoot) })
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return collector.New(collector.Config{
		Enabled: collector.Bool(true),
		AppCode: "gate",
		DocRoot: docRoot,
		AppRoot: wd,
	}), docRoot
}

func eventFileContent(t *testing.T, docRoot string) string {
	t.Helper()
	dir := filepath.Join(docRoot, "temp", "collector", "exception")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return ""
	}
	content, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("baca berkas event: %v", err)
	}

	return string(content)
}

func decodeEvent(t *testing.T, docRoot string) map[string]interface{} {
	t.Helper()
	line := strings.TrimSpace(eventFileContent(t, docRoot))
	if line == "" {
		t.Fatal("tidak ada event tertulis")
	}
	var ev map[string]interface{}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("event bukan JSON sah: %v", err)
	}

	return ev
}

func TestRecoveryMelaporkanPanicDanMenjawab500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, docRoot := newTestCollector(t)

	r := gin.New()
	r.Use(Recovery(c))
	r.PUT("/api/domain/:domain/:ipAddress", func(*gin.Context) {
		panic(errors.New("nginx reload gagal"))
	})

	req := httptest.NewRequest(http.MethodPut, "/api/domain/example.com/10.0.0.1?x=1", nil)
	req.Header.Set("Authorization", "Bearer rahasia")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau %d", w.Code, http.StatusInternalServerError)
	}

	ev := decodeEvent(t, docRoot)
	if ev["message"] != "nginx reload gagal" {
		t.Errorf("message = %v", ev["message"])
	}
	if ev["controller"] != "/api/domain/:domain/:ipAddress" {
		t.Errorf("controller = %v", ev["controller"])
	}
	ctx, _ := ev["context"].(map[string]interface{})
	headers, _ := ctx["headers"].(map[string]interface{})
	if headers["authorization"] != "[redacted]" {
		t.Errorf("headers.authorization = %v, mau [redacted]", headers["authorization"])
	}
}

func TestRecoveryTidakMengubahApaPunBilaTidakPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, docRoot := newTestCollector(t)

	r := gin.New()
	r.Use(Recovery(c))
	r.GET("/api/info", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q, mau 200 \"ok\"", w.Code, w.Body.String())
	}
	if eventFileContent(t, docRoot) != "" {
		t.Fatalf("tidak seharusnya ada event tertulis")
	}
}

func TestContextFromMemetakanRuteDanQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	var captured collector.Context
	r.GET("/api/domain/status/:domain", func(c *gin.Context) {
		captured = ContextFrom(c)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/domain/status/example.com?verbose=1", nil)
	req.Host = "gate.tribelio.page"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if captured.Controller != "/api/domain/status/:domain" {
		t.Errorf("controller = %q", captured.Controller)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.Domain != "gate.tribelio.page" {
		t.Errorf("domain = %q", captured.Domain)
	}
	if captured.QueryString != "verbose=1" {
		t.Errorf("queryString = %q", captured.QueryString)
	}
	args, _ := captured.Arguments.(map[string]string)
	if args["domain"] != "example.com" {
		t.Errorf("arguments.domain = %q", args["domain"])
	}
}

// Regresi: Recovery(nil) - pemanggil yang belum sempat menginisialisasi Collector-nya
// (mis. lupa memanggil LoadCollector sebelum InitializeRouter) tetap harus memulihkan
// panic dan menjawab 500 seperti biasa, bukan ikut panic kedua yang lolos dari sini.
func TestRecoveryDenganCollectorNilTetapMemulihkanPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(Recovery(nil))
	r.GET("/api/info", func(*gin.Context) {
		panic(errors.New("boom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau %d", w.Code, http.StatusInternalServerError)
	}
}
