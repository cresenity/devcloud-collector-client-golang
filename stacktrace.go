package collector

import (
	"path/filepath"
	"runtime"
	"strings"
)

// frame satu baris jejak panggilan, bentuknya sepadan dengan yang ditulis klien
// Node.js/Java supaya halaman rincian devcloud membacanya dengan cara yang sama.
type frame struct {
	path          string // jalur absolut di cakram, kosong bila tidak diketahui (mis. aset Go internal)
	file          string // relatif terhadap appRoot, atau apa adanya bila di luar appRoot
	line          int
	class         string // bagian paket/tipe penerima dari nama fungsi
	method        string // segmen terakhir nama fungsi
	isApplication bool
}

// captureFrames merekam call stack goroutine saat ini. skip menentukan berapa
// bingkai teratas (milik paket ini sendiri) yang dibuang supaya bingkai pertama
// pada hasilnya adalah kode pemanggil yang sesungguhnya.
//
// Ini juga berfungsi benar saat dipanggil dari dalam sebuah recover(): panic Go
// tidak melepas bingkai-bingkai di antara titik panic dan defer yang me-recover-nya
// sampai defer itu selesai, jadi runtime.Callers yang dipanggil dari sana tetap
// melihat seluruh rantai sampai ke titik panic aslinya - bukan cuma bingkai
// milik fungsi recover-nya sendiri.
func captureFrames(skip int, appRoot string, maxFrames int) []frame {
	if maxFrames <= 0 {
		maxFrames = 100
	}

	pcs := make([]uintptr, maxFrames+skip+2)
	n := runtime.Callers(skip+2, pcs) // +2: buang frame runtime.Callers dan captureFrames sendiri
	if n == 0 {
		return nil
	}

	framesIter := runtime.CallersFrames(pcs[:n])
	out := make([]frame, 0, n)
	for {
		rf, more := framesIter.Next()
		out = append(out, buildFrame(rf, appRoot))
		if !more || len(out) >= maxFrames {
			break
		}
	}

	return out
}

func buildFrame(rf runtime.Frame, appRoot string) frame {
	class, method := splitFunction(rf.Function)

	return frame{
		path:          rf.File,
		file:          relativeFile(rf.File, appRoot),
		line:          rf.Line,
		class:         class,
		method:        method,
		isApplication: isApplicationFrame(rf.File, appRoot),
	}
}

// relativeFile jalur relatif terhadap akar aplikasi. Dipakai supaya galat yang sama
// pada beberapa peladen menghasilkan hash yang sama di devcloud, dan supaya jalurnya
// langsung cocok dengan jalur di dalam repo git.
func relativeFile(filePath, appRoot string) string {
	if filePath == "" {
		return ""
	}
	rel, err := filepath.Rel(appRoot, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filePath
	}

	return rel
}

// isApplicationFrame menolak dependensi pihak ketiga (module cache Go) dan pustaka
// standar (GOROOT); sisanya, bila berada di bawah appRoot, dianggap kode aplikasi.
func isApplicationFrame(filePath, appRoot string) bool {
	if filePath == "" {
		return false
	}
	sep := string(filepath.Separator)
	if strings.Contains(filePath, sep+"pkg"+sep+"mod"+sep) {
		return false
	}
	if goRoot := runtime.GOROOT(); goRoot != "" && strings.HasPrefix(filePath, goRoot) {
		return false
	}
	rel, err := filepath.Rel(appRoot, filePath)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(rel, "..")
}

// splitFunction memecah nama fungsi lengkap Go ("github.com/x/y/pkg.Foo" atau
// "github.com/x/y/pkg.(*Type).Method") menjadi paket (dianggap "class") dan nama
// fungsi/method-nya sendiri.
func splitFunction(full string) (class, method string) {
	prefix := ""
	base := full
	if idx := strings.LastIndex(full, "/"); idx >= 0 {
		prefix = full[:idx+1]
		base = full[idx+1:]
	}
	dot := strings.Index(base, ".")
	if dot < 0 {
		return "", prefix + base
	}

	return prefix + base[:dot], base[dot+1:]
}
