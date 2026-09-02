package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const maxSnippetLineLength = 250

// readCodeSnippet cuplikan baris sumber di sekitar sebuah bingkai. Bentuknya mengikuti
// CException_Stacktrace_CodeSnippet di sisi PHP - nomor baris sebagai kunci string, isi
// barisnya sebagai nilai - karena halaman rincian devcloud sudah dibangun di atas bentuk
// itu. Berkas yang tak terbaca (terhapus, di luar izin baca, dibundel ke biner) mengembalikan
// map kosong: cuplikan memang tambahan, ketiadaannya tidak boleh menggagalkan laporan.
func readCodeSnippet(filePath string, surroundingLine, snippetLineCount int) map[string]string {
	empty := map[string]string{}
	if filePath == "" || surroundingLine <= 0 {
		return empty
	}

	file, err := os.Open(filePath)
	if err != nil {
		return empty
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil || len(lines) == 0 {
		return empty
	}

	start, end := snippetBounds(surroundingLine, snippetLineCount, len(lines))
	snippet := make(map[string]string, end-start+1)
	for current := start; current <= end; current++ {
		text := lines[current-1]
		if len(text) > maxSnippetLineLength {
			text = text[:maxSnippetLineLength]
		}
		snippet[strconv.Itoa(current)] = strings.TrimRight(text, " \t\r\n")
	}

	return snippet
}

func snippetBounds(surroundingLine, snippetLineCount, totalLines int) (start, end int) {
	start = surroundingLine - snippetLineCount/2
	if start < 1 {
		start = 1
	}
	end = start + (snippetLineCount - 1)
	if end > totalLines {
		end = totalLines
		start = end - (snippetLineCount - 1)
		if start < 1 {
			start = 1
		}
	}

	return start, end
}
