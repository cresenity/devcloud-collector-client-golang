// Package collector adalah producer Go untuk pipeline exception collector devcloud.
//
// Ia menulis bentuk berkas dan direktori yang sama persis
// (<docRoot>/temp/collector/exception/YYYYMMDD.txt) yang ditulis pengumpul PHP CF
// sendiri dan yang sudah dibaca lewat SSH oleh DCron_Method_Collector_Exception di
// devcloud - kontrak berkas itu tidak boleh menyimpang dari sana.
// devcloud-collector-client-nodejs dan devcloud-collector-client-java adalah klien
// sepadan untuk pipeline yang sama; jaga nama ruas dan perilaku (jendela dedupe,
// DontReport/DontReportMessages) tetap sinkron di antara ketiganya supaya perubahan
// kontrak pipeline cukup dipikirkan sekali.
package collector

import (
	"crypto/rand"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Collector satu instance per proses. Aman dipakai bersamaan dari banyak goroutine.
type Collector struct {
	config     Config
	writer     *eventWriter
	instanceID string
	startedAt  time.Time

	mu           sync.Mutex
	recentHashes map[string]time.Time
}

// New membangun Collector dari Config. Config yang tidak diisi jatuh ke variabel
// lingkungan DEVCLOUD_COLLECTOR_*, lalu ke nilai baku (lihat normalizeConfig).
func New(cfg Config) *Collector {
	normalized := normalizeConfig(cfg)

	return &Collector{
		config:       normalized,
		writer:       newEventWriter(normalized.DocRoot),
		instanceID:   newInstanceID(),
		startedAt:    time.Now(),
		recentHashes: make(map[string]time.Time),
	}
}

// Enabled melaporkan apakah Report() akan benar-benar menulis apa pun.
func (c *Collector) Enabled() bool {
	return c != nil && c.config.Enabled != nil && *c.config.Enabled
}

// Report melaporkan satu galat. err boleh berupa error biasa, atau nilai apa pun yang
// datang dari recover() (string, atau tipe lain) - nilai bukan-error dibungkus serupa
// perlakuan klien Node.js terhadap unhandledRejection yang bukan Error.
//
// Report dan semua yang dipanggilnya tidak pernah panic ke pemanggil: fungsi ini
// dipanggil dari jalur penanganan galat/panic, dan kegagalan di sini akan menutupi
// galat yang sebenarnya. Mengembalikan true hanya bila satu baris benar-benar tertulis.
func (c *Collector) Report(err interface{}, ctx Context) (wrote bool) {
	defer func() {
		if recover() != nil {
			wrote = false
		}
	}()

	// c == nil bisa terjadi kalau pemanggil (mis. gincollector.Recovery) belum sempat
	// diberi Collector yang sungguhan - dijaga di sini juga, bukan cuma di gincollector,
	// supaya kontrak "Report tidak pernah panic" tetap berlaku apa pun jalur pemanggilannya.
	if c == nil || !c.Enabled() || err == nil {
		return false
	}

	name, message := normalizeError(err)
	if c.isIgnored(name, message) {
		return false
	}

	// skip=1: buang bingkai Report ini sendiri, supaya bingkai pertama adalah kode
	// pemanggil. Dipanggil dari dalam sebuah defer/recover, ini tetap menampilkan
	// seluruh rantai sampai ke titik panic aslinya - lihat catatan di captureFrames.
	frames := captureFrames(1, c.config.AppRoot, c.config.MaxFrames)
	event := c.buildEvent(name, message, frames, ctx)
	if c.isDuplicate(event) {
		return false
	}

	return c.writer.write(event)
}

func normalizeError(v interface{}) (name, message string) {
	switch e := v.(type) {
	case error:
		return errorTypeName(e), e.Error()
	case string:
		return "NonError", e
	default:
		return "NonError", fmt.Sprintf("%v", e)
	}
}

func errorTypeName(err error) string {
	t := reflect.TypeOf(err)
	if t == nil {
		return "error"
	}

	return t.String()
}

func (c *Collector) isIgnored(name, message string) bool {
	for _, n := range c.config.DontReport {
		if n == name {
			return true
		}
	}
	for _, needle := range c.config.DontReportMessages {
		if needle != "" && strings.Contains(message, needle) {
			return true
		}
	}

	return false
}

// isDuplicate: galat identik (nama+pesan+file+baris) yang berulang dalam jendela
// dedupe ditulis sekali saja. Devcloud menggabungkannya lagi lewat hash, tetapi
// penggabungan itu terjadi sesudah berkasnya dibaca cron - satu perulangan yang gagal
// terus di antara dua jalannya cron sudah cukup membengkakkan berkasnya.
func (c *Collector) isDuplicate(event map[string]interface{}) bool {
	window := time.Duration(0)
	if c.config.DedupeWindow != nil {
		window = *c.config.DedupeWindow
	}
	if window <= 0 {
		return false
	}

	key := fmt.Sprintf("%v|%v|%v|%v", event["error"], event["message"], event["file"], event["line"])
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if seenAt, ok := c.recentHashes[key]; ok && now.Sub(seenAt) < window {
		return true
	}
	c.recentHashes[key] = now
	for k, t := range c.recentHashes {
		if now.Sub(t) >= window {
			delete(c.recentHashes, k)
		}
	}

	return false
}

func (c *Collector) buildEvent(name, message string, frames []frame, ctx Context) map[string]interface{} {
	origin := applicationOrigin(frames)
	var file string
	var line int
	if origin != nil {
		file = origin.file
		line = origin.line
	}

	event := map[string]interface{}{
		"appCode":          c.config.AppCode,
		"appId":            c.instanceID,
		"datetime":         time.Now().Format("2006-01-02 15:04:05"),
		"error":            name,
		"message":          message,
		"file":             file,
		"line":             line,
		"stacktrace":       c.buildStacktrace(frames),
		"language":         "Go",
		"language_version": runtime.Version(),

		"controller":    ctx.Controller,
		"method":        ctx.Method,
		"domain":        ctx.Domain,
		"user":          ctx.User,
		"role":          ctx.Role,
		"orgId":         ctx.OrgID,
		"orgCode":       ctx.OrgCode,
		"userAgent":     ctx.UserAgent,
		"httpReferer":   ctx.HTTPReferer,
		"remoteAddress": ctx.RemoteAddress,
		"fullUrl":       ctx.FullURL,
		"protocol":      ctx.Protocol,
		"postData":      ctx.PostData,
	}

	contextMap := toContextMap(ctx)
	debug, _ := contextMap["debug"].(map[string]interface{})
	if debug == nil {
		debug = map[string]interface{}{}
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	debug["pid"] = os.Getpid()
	debug["uptimeSeconds"] = int(time.Since(c.startedAt).Seconds())
	debug["memoryAllocMb"] = int(mem.Alloc / 1048576)
	debug["numGoroutine"] = runtime.NumGoroutine()
	debug["environment"] = c.config.Environment
	debug["hostname"] = c.config.Hostname
	debug["privateIp"] = c.config.PrivateIP
	contextMap["debug"] = debug
	event["context"] = contextMap

	return event
}

func applicationOrigin(frames []frame) *frame {
	for i := range frames {
		if frames[i].isApplication {
			return &frames[i]
		}
	}
	if len(frames) > 0 {
		return &frames[0]
	}

	return nil
}

func (c *Collector) buildStacktrace(frames []frame) []map[string]interface{} {
	limit := c.config.MaxFrames
	if limit <= 0 || limit > len(frames) {
		limit = len(frames)
	}

	out := make([]map[string]interface{}, 0, limit)
	for _, f := range frames[:limit] {
		snippet := map[string]string{}
		if f.isApplication {
			snippet = readCodeSnippet(f.path, f.line, c.config.CodeSnippetLineCount)
		}
		out = append(out, map[string]interface{}{
			"file":                 f.file,
			"line_number":          f.line,
			"class":                f.class,
			"method":               f.method,
			"code_snippet":         snippet,
			"is_application_frame": f.isApplication,
		})
	}

	return out
}

// newInstanceID menandai satu proses yang sedang berjalan, sepadan dengan app instance
// id CF - dipakai sebagai appId pada tiap kejadian yang ditulis proses ini.
func newInstanceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
