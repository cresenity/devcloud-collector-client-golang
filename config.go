package collector

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const envPrefix = "DEVCLOUD_COLLECTOR_"

// Config konfigurasi Collector. Setiap ruas bisa datang dari kode pemanggil atau dari
// variabel lingkungan berprefiks DEVCLOUD_COLLECTOR_ - nama ruasnya sengaja semirip
// mungkin dengan devcloud.collector.* pada klien Java dan klien Node.js, supaya tiga
// bahasa yang menulis ke pipeline yang sama tidak perlu diingat dengan nama berbeda.
type Config struct {
	// Enabled mati kecuali dinyalakan eksplisit, sama seperti config collector.exception
	// di CF. Pointer supaya "tidak diisi di kode" (nil, jatuh ke env/default) bisa dibedakan
	// dari "sengaja diisi false" (tetap false apa pun isi env-nya) - pemanggil yang tidak
	// mengisi ruas ini sama sekali membiarkan env DEVCLOUD_COLLECTOR_ENABLED yang memutuskan.
	Enabled *bool

	// AppCode harus sama dengan app.app_code di devcloud supaya exception-nya tertaut
	// ke project.
	AppCode string

	// DocRoot harus sama persis dengan DModel_Site.doc_root, itu yang dibaca cron devcloud.
	DocRoot string

	// AppRoot menentukan bingkai stack mana yang dianggap milik aplikasi sendiri, dan
	// jalur apa yang tertulis pada tiap bingkai. Default sama dengan DocRoot.
	AppRoot string

	MaxFrames            int
	CodeSnippetLineCount int

	// DontReport menyaring lewat nama tipe galat (mis. "*MyApp.ValidationError").
	DontReport []string
	// DontReportMessages menyaring lewat potongan pesan galat.
	DontReportMessages []string

	// DedupeWindow: galat identik yang berulang dalam jendela ini ditulis sekali saja.
	// Pointer dengan alasan sama seperti Enabled - nil jatuh ke env/default 60 detik;
	// 0 eksplisit (lewat collector.Duration(0)) mematikan penggabungan sama sekali.
	DedupeWindow *time.Duration

	Environment string
	Hostname    string
	// PrivateIP: identitas peladen tempat proses ini berjalan, supaya sebuah exception
	// langsung menunjuk server mana tanpa perlu ditelusuri lewat server_remote di devcloud.
	PrivateIP string
}

// Bool adalah penolong ringkas untuk mengisi Config.Enabled dari literal bool.
func Bool(v bool) *bool { return &v }

// Duration adalah penolong ringkas untuk mengisi Config.DedupeWindow dari literal durasi.
func Duration(v time.Duration) *time.Duration { return &v }

func envBool(name string) (bool, bool) {
	v, ok := os.LookupEnv(envPrefix + name)
	if !ok || v == "" {
		return false, false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

func envString(name, fallback string) string {
	v := os.Getenv(envPrefix + name)
	if v == "" {
		return fallback
	}

	return v
}

func envInt(name string, fallback int) int {
	v := os.Getenv(envPrefix + name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}

	return n
}

func envList(name string) []string {
	v := os.Getenv(envPrefix + name)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// detectPrivateIP mengembalikan IP non-internal pertama yang ditemukan - di server
// dengan NAT (ip publik tidak terikat ke interface manapun di dalam OS), inilah IP LAN
// sungguhan yang bisa dipakai untuk mencocokkan ke server_remote.ip_private di devcloud.
func detectPrivateIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}

	return ""
}

func normalizeConfig(cfg Config) Config {
	out := cfg

	if out.Enabled == nil {
		v, _ := envBool("ENABLED")
		out.Enabled = &v
	}
	if out.AppCode == "" {
		out.AppCode = envString("APP_CODE", "")
	}
	if out.DocRoot == "" {
		cwd, _ := os.Getwd()
		out.DocRoot = envString("DOC_ROOT", cwd)
	}
	if abs, err := filepath.Abs(out.DocRoot); err == nil {
		out.DocRoot = abs
	}
	if out.AppRoot == "" {
		out.AppRoot = envString("APP_ROOT", out.DocRoot)
	}
	if abs, err := filepath.Abs(out.AppRoot); err == nil {
		out.AppRoot = abs
	}
	if out.MaxFrames == 0 {
		out.MaxFrames = envInt("MAX_FRAMES", 100)
	}
	if out.CodeSnippetLineCount == 0 {
		out.CodeSnippetLineCount = envInt("CODE_SNIPPET_LINE_COUNT", 31)
	}
	if out.DontReport == nil {
		out.DontReport = envList("DONT_REPORT")
	}
	if out.DontReportMessages == nil {
		out.DontReportMessages = envList("DONT_REPORT_MESSAGES")
	}
	if out.DedupeWindow == nil {
		ms := envInt("DEDUPE_WINDOW_MS", 60000)
		d := time.Duration(ms) * time.Millisecond
		out.DedupeWindow = &d
	}
	if out.Environment == "" {
		out.Environment = envString("ENVIRONMENT", "")
	}
	if out.Hostname == "" {
		h, _ := os.Hostname()
		out.Hostname = envString("HOSTNAME", h)
	}
	if out.PrivateIP == "" {
		out.PrivateIP = envString("PRIVATE_IP", detectPrivateIP())
	}

	return out
}
