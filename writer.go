package collector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// eventWriter menambahkan satu baris JSON per kejadian ke
// <docRoot>/temp/collector/exception/YYYYMMDD.txt.
//
// Ditulis lewat OpenFile mode append (bukan lewat buffer yang di-flush belakangan):
// pemanggil terbesarnya adalah jalur penanganan panic, dan tulisan yang tertunda di
// sana tidak pernah sampai ke cakram. Satu os.Write mode O_APPEND di bawah ukuran pipa
// (PIPE_BUF, minimal 512 byte di POSIX) atomik, jadi beberapa goroutine/proses yang
// menulis ke berkas yang sama tidak saling memotong.
//
// Direktorinya persis yang dibaca dan dihapus DCron_Method_Collector_Exception lewat
// SSH - kedua sisi harus sepakat kalau jalur ini berubah.
type eventWriter struct {
	directory string
}

func newEventWriter(docRoot string) *eventWriter {
	return &eventWriter{directory: filepath.Join(docRoot, "temp", "collector", "exception")}
}

func fileNameFor(t time.Time) string {
	return fmt.Sprintf("%04d%02d%02d.txt", t.Year(), t.Month(), t.Day())
}

// write mengembalikan false pada kegagalan apa pun - pengumpul yang rusak tidak boleh
// merusak aplikasi yang diawasinya, jadi ia tidak pernah mengembalikan error ke pemanggil.
func (w *eventWriter) write(event map[string]interface{}) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	encoded, err := json.Marshal(event)
	if err != nil {
		return false
	}

	if err := os.MkdirAll(w.directory, 0o755); err != nil {
		return false
	}

	path := filepath.Join(w.directory, fileNameFor(time.Now()))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	defer f.Close()

	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return false
	}

	return true
}
