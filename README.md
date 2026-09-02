# devcloud-collector-client-golang

Go producer for devcloud's exception collector. Writes one JSON line per exception to
`<docRoot>/temp/collector/exception/yyyymmdd.txt` on the app server — the same file
shape and directory `DCron_Method_Collector_Exception` (devcloud, CF/PHP) already lists
and parses over SSH for CF apps, and the same one `devcloud-collector-client-nodejs` and
`devcloud-collector-client-java` write. A Go service then shows up in the same devcloud
exception dashboard, gets the same Discord message, and the same project task, without
any change on the devcloud side — as long as the site is registered there with the
correct `doc_root` and SSH access.

The core package (`github.com/cresenity/devcloud-collector-client-golang`) has zero
runtime dependencies. Gin integration lives in a separate `gin` subpackage
(`github.com/cresenity/devcloud-collector-client-golang/gin`) so a consumer that doesn't
use Gin never pulls in `gin-gonic/gin`.

## Install

```bash
go get github.com/cresenity/devcloud-collector-client-golang
go get github.com/cresenity/devcloud-collector-client-golang/gin  # only if you use Gin
```

The repo is public on GitHub — no registry token needed, unlike the Node/Java siblings.
Versions are plain git tags (`v1.0.0`, ...); there's no separate publish step.

## Configure

Every option can come from the `collector.Config` literal or from the environment,
prefixed `DEVCLOUD_COLLECTOR_`:

| Option | Env | Default |
|---|---|---|
| `Enabled` | `DEVCLOUD_COLLECTOR_ENABLED` | `false` |
| `AppCode` | `DEVCLOUD_COLLECTOR_APP_CODE` | `""` |
| `DocRoot` | `DEVCLOUD_COLLECTOR_DOC_ROOT` | `os.Getwd()` |
| `AppRoot` | `DEVCLOUD_COLLECTOR_APP_ROOT` | same as `DocRoot` |
| `MaxFrames` | `DEVCLOUD_COLLECTOR_MAX_FRAMES` | `100` |
| `CodeSnippetLineCount` | `DEVCLOUD_COLLECTOR_CODE_SNIPPET_LINE_COUNT` | `31` |
| `DontReport` | `DEVCLOUD_COLLECTOR_DONT_REPORT` | empty |
| `DontReportMessages` | `DEVCLOUD_COLLECTOR_DONT_REPORT_MESSAGES` | empty |
| `DedupeWindow` | `DEVCLOUD_COLLECTOR_DEDUPE_WINDOW_MS` | `60000` |
| `Hostname` | `DEVCLOUD_COLLECTOR_HOSTNAME` | `os.Hostname()` |
| `PrivateIP` | `DEVCLOUD_COLLECTOR_PRIVATE_IP` | first non-internal IPv4 |

`Enabled` and `DedupeWindow` are `*bool`/`*time.Duration` on purpose: leaving them `nil`
in code means "let the environment decide" (or fall back to the default if the
environment doesn't say either), which is the normal way to deploy this — hardcode
`AppCode`/`DocRoot` in code, control on/off entirely through `DEVCLOUD_COLLECTOR_ENABLED`
at the environment level without a rebuild. Use the `collector.Bool(true)` /
`collector.Duration(0)` helpers when you do want to force a value from code.

`Enabled` defaults to `false` — same as CF's own `collector.exception` config, which is
off outside production.

`DocRoot` must match the `DModel_Site.doc_root` registered on devcloud exactly, or the
cron looks in the wrong place and finds nothing. `AppRoot` is only separate when the
deployed tree is not the repo root; it decides which stack frames count as the
application's own and what paths look like.

## Use

```go
import collector "github.com/cresenity/devcloud-collector-client-golang"

c := collector.New(collector.Config{
    AppCode: "gate",
    DocRoot: "/app/data",
})
```

Two places are worth wiring, and the second is the one that matters most in practice:

```go
// 1. panic recovery (see the gin subpackage below for the Gin-specific version)
defer func() {
    if r := recover(); r != nil {
        c.Report(r, collector.Context{Controller: "worker", Method: "run"})
        panic(r) // hand it back, don't swallow it
    }
}()

// 2. anywhere an error is caught and turned into a response instead of returned/panicked
if err != nil {
    c.Report(err, collector.Context{Controller: "UpdateDomain", Method: "PUT"})
    // ... still answer the caller normally
}
```

`Report()` never panics and never returns an error — a broken collector must never break
the app it watches. It returns `true` only when a line was actually written.

## Gin

```go
import (
    collector "github.com/cresenity/devcloud-collector-client-golang"
    gincollector "github.com/cresenity/devcloud-collector-client-golang/gin"
)

c := collector.New(collector.Config{AppCode: "gate", DocRoot: "/app/data"})

router := gin.New()
router.Use(gin.Logger(), gincollector.Recovery(c)) // instead of gin.Default()
```

`gincollector.Recovery` replaces `gin.Recovery()`: it recovers a panic, reports it with
the request's context (route pattern, method, host, headers with `Authorization`/
`Cookie`/`*-api-key`/`*-secret-key` redacted, route params), then answers 500 — same
outcome as `gin.Recovery()`, plus devcloud now knows about it.

For an error a handler catches and turns into a normal JSON error response instead of a
panic — the case that matters most, same as the Node/Java clients — report it explicitly
with the request context:

```go
func UpdateDomain(c *gin.Context) {
    if err := doSomething(); err != nil {
        gincollector.Report(collectorInstance, c, err)
        c.JSON(http.StatusInternalServerError, dtf.Response{Status: false, Message: err.Error()})
        return
    }
}
```

`gincollector.ContextFrom(c)` builds the same context `Recovery` uses, if you want to
pass extra fields on top of it via `collector.Context{...}` directly instead.

The request body is deliberately never read here: consuming it would empty it for any
handler/middleware that hasn't read it yet, and by the time a panic or error is caught
it's usually already been read (or no longer relevant) anyway. Set `PostData` yourself at
the catch site if you need it.

## Not reporting an exception

`DontReport` matches the Go type name of the error (`reflect.TypeOf(err).String()`, e.g.
`"*myapp.ValidationError"` — check with a quick `fmt.Printf("%T", err)` if unsure).
`DontReportMessages` matches a substring of `err.Error()`.

```go
DontReportMessages: []string{"not found", "context canceled"}
```

Silence a type only when it says nothing about the app's own correctness — a wrong id
from the caller, a validation failure. An error the app itself could have prevented
belongs in devcloud.

## Repeated errors

An identical error — same type, message, file and line — is written once per
`DedupeWindow` (default 60s). "File and line" here is **where `Report()` was called
from**, not where the error value was constructed — Go errors don't carry their own
creation-site stack the way a JS `Error` or a Java `Throwable` does. In practice this is
exactly what you want: the same handler line failing repeatedly across many requests
dedupes; two unrelated call sites returning a similarly-worded error do not.

Devcloud merges by hash too, but only after it reads the file, and one failing loop
between two cron runs is enough to grow the dump past the 50 MB the cron will simply
delete unread.

## Field shape

Top level mirrors CF's own producer and the Node.js/Java clients: `appCode`, `appId`,
`datetime`, `error`, `message`, `file`, `line`, `stacktrace`, `language` (`"Go"`),
`language_version` (`runtime.Version()`), plus the flat request fields (`controller`,
`method`, `domain`, `user`, `role`, `orgId`, `orgCode`, `userAgent`, `httpReferer`,
`remoteAddress`, `fullUrl`, `protocol`, `postData`) and a `context` object holding
`request`, `request_data`, `arguments`, `headers`, `session`, `cookies`, `git`, `app`,
`debug`.

`file` and every frame path are **relative to `AppRoot`**. That is deliberate: the same
error on three servers then hashes to one devcloud row, and the path matches the path
inside the git repo. A frame under Go's module cache (`.../pkg/mod/...`) or `GOROOT` is
never treated as an application frame and never carries a code snippet.

`context.debug` always carries `pid`, `uptimeSeconds` (since the `Collector` was
constructed), `memoryAllocMb` (`runtime.MemStats.Alloc` — Go has no portable RSS without
an OS-specific call, this is the closest zero-dependency approximation), `numGoroutine`,
plus `environment`/`hostname`/`privateIp` from `Config`.

## Why a shared package instead of embedding per app

Same reason as the Node.js and Java clients: more than one Go service will want this, and
the pipeline's file shape is a contract with devcloud, not app code. Keeping it in one
package means a change to that contract is made once.

## Devcloud-side prerequisites (not part of this repo)

1. The server registered as a `DModel_Site` with the correct `doc_root` and a working SSH
   credential (`DModel_ServerRemote`). For a Dockerized service, `doc_root` must be a path
   on the **host**, not inside the container — point it at whatever host directory is
   volume-mounted to the container path passed as `DocRoot`.
2. A cron running `DCron_Method_Collector_Exception` for that `site_id`, `*/5 * * * *`
   recommended.
3. An `app` row whose `app_code` matches `AppCode`, linked to a project. Without the
   project link the exception is still recorded but **no Discord message is sent** —
   `DCollector_Discord::webhookUrl()` resolves the webhook through
   `project → team → teamDiscord`.
