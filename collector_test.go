package collector

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func makeDocRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "collector-test-")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	return dir
}

func readEvents(t *testing.T, docRoot string) []map[string]interface{} {
	t.Helper()
	dir := filepath.Join(docRoot, "temp", "collector", "exception")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []map[string]interface{}
	for _, e := range entries {
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("baca %s: %v", e.Name(), err)
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var ev map[string]interface{}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("baris bukan JSON sah: %v (%s)", err, line)
			}
			out = append(out, ev)
		}
	}

	return out
}

func appRootHere(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return wd
}

func TestMatiSecaraBawaan(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})

	if got := c.Report(errors.New("boom"), Context{}); got != false {
		t.Fatalf("Report() = %v, mau false", got)
	}
	if events := readEvents(t, docRoot); len(events) != 0 {
		t.Fatalf("events = %v, mau kosong", events)
	}
}

func TestMenulisSatuBarisJSON(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})

	if got := c.Report(errors.New("gagal total"), Context{}); got != true {
		t.Fatalf("Report() = %v, mau true", got)
	}

	events := readEvents(t, docRoot)
	if len(events) != 1 {
		t.Fatalf("jumlah events = %d, mau 1", len(events))
	}
	ev := events[0]
	if ev["appCode"] != "gate" {
		t.Errorf("appCode = %v", ev["appCode"])
	}
	if ev["error"] != "*errors.errorString" {
		t.Errorf("error = %v", ev["error"])
	}
	if ev["message"] != "gagal total" {
		t.Errorf("message = %v", ev["message"])
	}
	if ev["language"] != "Go" {
		t.Errorf("language = %v", ev["language"])
	}
	if ev["language_version"] != runtime.Version() {
		t.Errorf("language_version = %v", ev["language_version"])
	}
	dt, _ := ev["datetime"].(string)
	if _, err := time.Parse("2006-01-02 15:04:05", dt); err != nil {
		t.Errorf("datetime = %q tidak sesuai format: %v", dt, err)
	}
}

func TestBerkasDinamaiPerTanggal(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})
	c.Report(errors.New("boom"), Context{})

	dir := filepath.Join(docRoot, "temp", "collector", "exception")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("jumlah berkas = %d, mau 1", len(entries))
	}
	want := fileNameFor(time.Now())
	if entries[0].Name() != want {
		t.Errorf("nama berkas = %q, mau %q", entries[0].Name(), want)
	}
}

func TestBingkaiAplikasiPakaiJalurRelatifDanCuplikanKode(t *testing.T) {
	docRoot := makeDocRoot(t)
	appRoot := appRootHere(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRoot})
	c.Report(errors.New("boom"), Context{})

	events := readEvents(t, docRoot)
	ev := events[0]
	if ev["file"] != "collector_test.go" {
		t.Errorf("file = %v", ev["file"])
	}
	line, _ := ev["line"].(float64)
	if line <= 0 {
		t.Errorf("line = %v", ev["line"])
	}

	stacktrace, _ := ev["stacktrace"].([]interface{})
	if len(stacktrace) == 0 {
		t.Fatal("stacktrace kosong")
	}
	frame0, _ := stacktrace[0].(map[string]interface{})
	if frame0["is_application_frame"] != true {
		t.Errorf("frame0.is_application_frame = %v", frame0["is_application_frame"])
	}
	if frame0["file"] != "collector_test.go" {
		t.Errorf("frame0.file = %v", frame0["file"])
	}
	snippet, _ := frame0["code_snippet"].(map[string]interface{})
	if len(snippet) <= 1 {
		t.Errorf("code_snippet terlalu pendek: %v", snippet)
	}
}

func TestMaxFramesMembatasiJumlahBingkai(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t), MaxFrames: 2})
	c.Report(errors.New("boom"), Context{})

	events := readEvents(t, docRoot)
	stacktrace, _ := events[0]["stacktrace"].([]interface{})
	if len(stacktrace) > 2 {
		t.Fatalf("panjang stacktrace = %d, mau <= 2", len(stacktrace))
	}
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func TestDontReportMenyaringLewatNamaTipe(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{
		Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t),
		DontReport: []string{"*collector.validationError"},
	})

	if got := c.Report(&validationError{msg: "tidak sah"}, Context{}); got != false {
		t.Fatalf("Report(validationError) = %v, mau false", got)
	}
	if got := c.Report(errors.New("tidak sah"), Context{}); got != true {
		t.Fatalf("Report(errors.New) = %v, mau true", got)
	}
}

func TestDontReportMessagesMenyaringLewatPotonganPesan(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{
		Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t),
		DontReportMessages: []string{"not found"},
	})

	if got := c.Report(errors.New("instance with id 5 not found"), Context{}); got != false {
		t.Fatalf("Report() = %v, mau false", got)
	}
	if events := readEvents(t, docRoot); len(events) != 0 {
		t.Fatalf("events = %v, mau kosong", events)
	}
}

func TestGalatIdentikYangBerulangHanyaDitulisSekali(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})
	// Di Go, titik yang direkam sebagai "asal" galat adalah baris yang memanggil
	// Report - bukan baris yang membuat nilai error, seperti pada klien Node.js/Java
	// (di sana error.stack sudah melekat sejak error dibuat). Jadi supaya dua
	// kejadian ini dianggap sama, keduanya harus benar-benar dipanggil dari baris
	// yang sama - sepadan dengan skenario nyata: satu baris kode yang sama terpicu
	// berulang kali oleh dua permintaan berbeda.
	report := func() bool { return c.Report(errors.New("berulang"), Context{}) }

	if got := report(); got != true {
		t.Fatalf("Report() pertama = %v, mau true", got)
	}
	if got := report(); got != false {
		t.Fatalf("Report() kedua = %v, mau false (dedupe)", got)
	}
	if events := readEvents(t, docRoot); len(events) != 1 {
		t.Fatalf("jumlah events = %d, mau 1", len(events))
	}
}

func TestDedupeWindowNolMematikanPenggabungan(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{
		Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t),
		DedupeWindow: Duration(0),
	})
	boom := func() error { return errors.New("berulang") }

	c.Report(boom(), Context{})
	c.Report(boom(), Context{})
	if events := readEvents(t, docRoot); len(events) != 2 {
		t.Fatalf("jumlah events = %d, mau 2", len(events))
	}
}

func TestNilaiYangBukanErrorTetapTerlaporkan(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})

	if got := c.Report("socket ditutup sepihak", Context{}); got != true {
		t.Fatalf("Report(string) = %v, mau true", got)
	}

	events := readEvents(t, docRoot)
	if events[0]["error"] != "NonError" {
		t.Errorf("error = %v", events[0]["error"])
	}
	if events[0]["message"] != "socket ditutup sepihak" {
		t.Errorf("message = %v", events[0]["message"])
	}
}

func TestKonteksDipetakanKeBentukYangDibacaDevcloud(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})
	c.Report(errors.New("boom"), Context{
		Controller:    "/api/domain/:domain/:ipAddress",
		Method:        "PUT",
		RequestURL:    "/api/domain/example.com/10.0.0.1",
		RequestMethod: "PUT",
		QueryString:   "x=1",
		Headers:       map[string]string{"host": "gate.tribelio.page"},
	})

	events := readEvents(t, docRoot)
	ev := events[0]
	if ev["controller"] != "/api/domain/:domain/:ipAddress" {
		t.Errorf("controller = %v", ev["controller"])
	}
	if ev["method"] != "PUT" {
		t.Errorf("method = %v", ev["method"])
	}
	ctx, _ := ev["context"].(map[string]interface{})
	req, _ := ctx["request"].(map[string]interface{})
	if req["url"] != "/api/domain/example.com/10.0.0.1" {
		t.Errorf("context.request.url = %v", req["url"])
	}
	reqData, _ := ctx["request_data"].(map[string]interface{})
	if reqData["queryString"] != "x=1" {
		t.Errorf("context.request_data.queryString = %v", reqData["queryString"])
	}
	headers, _ := ctx["headers"].(map[string]interface{})
	if headers["host"] != "gate.tribelio.page" {
		t.Errorf("context.headers.host = %v", headers["host"])
	}
	debug, _ := ctx["debug"].(map[string]interface{})
	pid, _ := debug["pid"].(float64)
	if int(pid) != os.Getpid() {
		t.Errorf("context.debug.pid = %v, mau %d", debug["pid"], os.Getpid())
	}
}

func TestDebugMenyertakanHostnameDanBisaDitimpa(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{
		Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t),
		Hostname: "gate-server-prod",
	})
	c.Report(errors.New("boom"), Context{})

	events := readEvents(t, docRoot)
	ctx, _ := events[0]["context"].(map[string]interface{})
	debug, _ := ctx["debug"].(map[string]interface{})
	if debug["hostname"] != "gate-server-prod" {
		t.Errorf("hostname = %v", debug["hostname"])
	}
}

func TestKegagalanMenulisTidakPanicKePemanggil(t *testing.T) {
	// docRoot menunjuk ke sebuah berkas biasa: MkdirAll di bawahnya pasti gagal
	blocked := filepath.Join(makeDocRoot(t), "berkas")
	if err := os.WriteFile(blocked, []byte(""), 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: blocked, AppRoot: appRootHere(t)})

	if got := c.Report(errors.New("boom"), Context{}); got != false {
		t.Fatalf("Report() = %v, mau false", got)
	}
}

func TestReportDariDalamRecoverTetapMenangkapRantaiPanic(t *testing.T) {
	docRoot := makeDocRoot(t)
	c := New(Config{Enabled: Bool(true), AppCode: "gate", DocRoot: docRoot, AppRoot: appRootHere(t)})

	func() {
		defer func() {
			if r := recover(); r != nil {
				c.Report(r, Context{})
			}
		}()
		panicDalamAplikasi()
	}()

	events := readEvents(t, docRoot)
	if len(events) != 1 {
		t.Fatalf("jumlah events = %d, mau 1", len(events))
	}
	ev := events[0]
	if ev["message"] != "panic dari fungsi aplikasi" {
		t.Errorf("message = %v", ev["message"])
	}
	stacktrace, _ := ev["stacktrace"].([]interface{})
	var methods []string
	for _, item := range stacktrace {
		f, _ := item.(map[string]interface{})
		methods = append(methods, fmt.Sprintf("%v", f["method"]))
	}
	found := false
	for _, m := range methods {
		if m == "panicDalamAplikasi" {
			found = true
		}
	}
	if !found {
		t.Errorf("panicDalamAplikasi tidak ada di stacktrace: %v", methods)
	}
}

func panicDalamAplikasi() {
	panic("panic dari fungsi aplikasi")
}

// Regresi: Recovery pada package gin memanggil collectorInstance.Report(...) langsung
// dari dalam recover()-nya sendiri - kalau Report tidak menjaga diri dari receiver nil,
// pemanggil yang belum sempat menginisialisasi Collector-nya (mis. lupa memanggil
// LoadCollector sebelum InitializeRouter) akan membuat Report ikut panic di dalam
// recover(), lolos dari jaring pengaman yang justru sedang dibangunnya sendiri.
func TestReportPadaCollectorNilTidakPanic(t *testing.T) {
	var c *Collector

	if got := c.Report(errors.New("boom"), Context{}); got != false {
		t.Fatalf("Report() pada Collector nil = %v, mau false", got)
	}
	if got := c.Enabled(); got != false {
		t.Fatalf("Enabled() pada Collector nil = %v, mau false", got)
	}
}
