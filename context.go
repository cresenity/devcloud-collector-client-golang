package collector

// Context rincian tambahan yang menemani satu laporan. Setiap ruas bersifat opsional -
// yang tidak relevan untuk satu pemanggilan cukup dibiarkan nol/kosong.
//
// Nama ruas datar (Controller..PostData) dibaca langsung sebagai kolom tingkat atas
// oleh DCron_Method_Collector_Exception di devcloud - sepadan dengan FLAT_FIELDS pada
// klien Node.js. Periksa kelas itu sebelum mengganti nama apa pun di sini.
type Context struct {
	Controller    string
	Method        string
	Domain        string
	User          string
	Role          string
	OrgID         string
	OrgCode       string
	UserAgent     string
	HTTPReferer   string
	RemoteAddress string
	FullURL       string
	Protocol      string
	PostData      string

	// RequestURL/RequestMethod/QueryString/Body/Files masuk ke context.request dan
	// context.request_data - dipisah dari ruas datar di atas karena begitulah bentuk
	// yang sudah dibaca devcloud dari klien Node.js/Java.
	RequestURL    string
	RequestMethod string
	QueryString   string
	Body          interface{}
	Files         interface{}

	Arguments interface{}
	Headers   interface{}
	Session   interface{}
	Cookies   interface{}
	Git       interface{}
	App       interface{}
	// Debug ditimpa/ditambahi Collector sendiri (pid, uptime, memori, environment,
	// hostname, privateIp) setelah ruas ini - isi di sini tetap dipertahankan untuk
	// kunci yang tidak ditimpa.
	Debug map[string]interface{}
}

func toContextMap(ctx Context) map[string]interface{} {
	m := map[string]interface{}{
		"request": map[string]interface{}{
			"url":    ctx.RequestURL,
			"method": ctx.RequestMethod,
		},
		"request_data": map[string]interface{}{
			"queryString": ctx.QueryString,
			"body":        ctx.Body,
			"files":       ctx.Files,
		},
	}
	if ctx.Arguments != nil {
		m["arguments"] = ctx.Arguments
	}
	if ctx.Headers != nil {
		m["headers"] = ctx.Headers
	}
	if ctx.Session != nil {
		m["session"] = ctx.Session
	}
	if ctx.Cookies != nil {
		m["cookies"] = ctx.Cookies
	}
	if ctx.Git != nil {
		m["git"] = ctx.Git
	}
	if ctx.App != nil {
		m["app"] = ctx.App
	}
	if ctx.Debug != nil {
		debug := make(map[string]interface{}, len(ctx.Debug))
		for k, v := range ctx.Debug {
			debug[k] = v
		}
		m["debug"] = debug
	}

	return m
}
