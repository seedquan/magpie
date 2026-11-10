package gateway

// Legacy archive settings are deliberately inert in this fork. These hooks
// keep the gateway's request lifecycle compatible without retaining an exporter.
import (
	"context"
	"github.com/yetone/magpie/internal/redact"
	"net/http"
	"net/url"
	"time"
)

type Putter interface {
	Put(context.Context, string, []byte) error
}

// ArchiveBucket is retained for compatibility; it is never called.
var ArchiveBucket func() (Putter, bool)

const ArchiveHeader = "X-Magpie-Archive-Id"

type wire struct {
	name string
	body []byte
}

func archiving(*http.Request, *captureResponseWriter, time.Time, []byte) *wire { return nil }
func archiveOwn(Call, *http.Request, []byte, int, http.Header, []byte) string  { return "" }
func (c *Call) archiveName() string                                            { return "" }
func archive(Call)                                                             {}
func discardArchive(res *captureResponseWriter) {
	if res.full != nil {
		res.full.discard()
	}
}

func scrubHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, vs := range h {
		s := make([]string, len(vs))
		for i, v := range vs {
			s[i] = redact.ScrubHeader(k, v)
		}
		out[k] = s
	}
	return out
}

// scrubQuery is a query string with the values of secret-named fields
// (key=, access_token=) and any secret in the others taken out.
func scrubQuery(q string) string {
	vs, err := url.ParseQuery(q)
	if err != nil {
		return redact.Scrub(q)
	}
	for k, v := range vs {
		for i := range v {
			if redact.SecretName(k) {
				v[i] = redact.Scrubbed
			} else {
				v[i] = redact.Scrub(v[i])
			}
		}
	}
	return vs.Encode()
}
