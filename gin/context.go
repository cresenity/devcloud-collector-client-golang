// Package gincollector menjembatani Collector ke Gin: mengambil konteks dari sebuah
// *gin.Context, middleware pemulihan panic yang melaporkannya, dan penolong pelaporan
// eksplisit di dalam handler.
//
// Terpisah dari package collector utama supaya package itu sendiri tetap tidak
// membawa dependensi gin-gonic/gin - konsumen yang tidak memakai Gin tidak pernah
// menariknya.
package gincollector

import (
	"strings"

	collector "github.com/cresenity/devcloud-collector-client-golang"
	"github.com/gin-gonic/gin"
)

var redactedHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"x-api-key":     true,
	"api-key":       true,
	"secret-key":    true,
}

// ContextFrom membangun collector.Context selengkap yang bisa diambil dari satu
// *gin.Context.
//
// Controller diisi pola rute (c.FullPath(), mis. "/api/domain/:domain/:ipAddress")
// dan Method diisi kata kerja HTTP-nya - pasangan itulah yang paling mendekati arti
// kedua kolom tersebut di devcloud untuk servis tanpa kelas controller.
//
// Badan permintaan sengaja tidak dibaca di sini: membacanya lewat io.ReadAll akan
// mengosongkan body untuk handler/middleware lain yang belum sempat membacanya, dan
// pada titik sebuah panic biasanya sudah terbaca (atau tidak lagi relevan) apa pun
// yang terjadi. Isi PostData sendiri di titik tangkap bila benar-benar diperlukan.
func ContextFrom(c *gin.Context) collector.Context {
	if c == nil || c.Request == nil {
		return collector.Context{}
	}

	req := c.Request
	headers := map[string]string{}
	for name, values := range req.Header {
		key := strings.ToLower(name)
		if redactedHeaders[key] {
			headers[key] = "[redacted]"
		} else if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	args := map[string]string{}
	for _, p := range c.Params {
		args[p.Key] = p.Value
	}

	controller := c.FullPath()
	if controller == "" {
		controller = req.URL.Path
	}

	scheme := "http"
	if req.TLS != nil {
		scheme = "https"
	}
	fullURL := req.URL.RequestURI()
	if req.Host != "" {
		fullURL = scheme + "://" + req.Host + req.URL.RequestURI()
	}

	return collector.Context{
		Controller:    controller,
		Method:        req.Method,
		Domain:        req.Host,
		UserAgent:     req.UserAgent(),
		HTTPReferer:   req.Referer(),
		RemoteAddress: c.ClientIP(),
		FullURL:       fullURL,
		Protocol:      req.Proto,

		RequestURL:    req.URL.RequestURI(),
		RequestMethod: req.Method,
		QueryString:   req.URL.RawQuery,
		Headers:       headers,
		Arguments:     args,
	}
}

// Report adalah penolong ringkas: collectorInstance.Report(err, ContextFrom(c)).
func Report(collectorInstance *collector.Collector, c *gin.Context, err interface{}) bool {
	if collectorInstance == nil {
		return false
	}

	return collectorInstance.Report(err, ContextFrom(c))
}
