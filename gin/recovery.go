package gincollector

import (
	"net/http"

	collector "github.com/cresenity/devcloud-collector-client-golang"
	"github.com/gin-gonic/gin"
)

// Recovery mengembalikan middleware Gin yang memulihkan panic, melaporkannya lewat
// collectorInstance dengan konteks permintaan yang memicunya, lalu menjawab 500.
//
// Pasang ini menggantikan gin.Recovery() (mis. pakai gin.New() + Logger() + Recovery
// ini, bukan gin.Default()) supaya sebuah panic yang sebelumnya cuma tercatat di log
// proses sekarang juga tercatat di devcloud - termasuk tiga bekas log.Fatal/
// log.Panicln di handler/domain.go yang sudah diganti balasan JSON biasa: kalau nanti
// ada jalur serupa yang lolos jadi panic sungguhan, ini jaring pengamannya.
func Recovery(collectorInstance *collector.Collector) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				collectorInstance.Report(r, ContextFrom(c))
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
