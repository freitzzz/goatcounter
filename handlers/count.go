// Copyright © Martin Tournoij – This file is part of GoatCounter and published
// under the terms of a slightly modified EUPL v1.2 license, which can be found
// in the LICENSE file or at https://license.goatcounter.com

package handlers

import (
	"fmt"
	"net/http"
	"encoding/json"
	"strings"

	"golang.org/x/text/language"
	"zgo.at/goatcounter/v2"
	"zgo.at/goatcounter/v2/metrics"
	"zgo.at/isbot"
	"zgo.at/zhttp"
	"zgo.at/zstd/ztime"
)

var forwardedForHeader = http.CanonicalHeaderKey("X-Forwarded-For")

// Use GIF because it's the smallest filesize (PNG is 116 bytes, vs 43 for GIF).
var gif = []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x1, 0x0, 0x1, 0x0, 0x80,
	0x1, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x4, 0x1, 0xa, 0x0,
	0x1, 0x0, 0x2c, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x1, 0x0, 0x0, 0x2, 0x2, 0x4c,
	0x1, 0x0, 0x3b}

func (h backend) count(w http.ResponseWriter, r *http.Request) error {
	m := metrics.Start("/count")
	defer m.Done()

	if r.Method == "GET" {
		metrics.Start("/count GET").Done()
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

	// Note this works in both HTTP/1.1 and HTTP/2, as the Go HTTP/2 server
	// picks up on this and sends the GOAWAY frame.
	// TODO: it would be better to set a short idle timeout, but this isn't
	// really something that can be configured per-handler at the moment.
	// https://github.com/golang/go/issues/16100
	w.Header().Set("Connection", "close")

	bot := isbot.Bot(r)
	// Don't track pages fetched with the browser's prefetch algorithm.
	if bot == isbot.BotPrefetch {
		return zhttp.Bytes(w, gif)
	}

	cip := extractClientIP(r)

	site := Site(r.Context())
	for _, ip := range site.Settings.IgnoreIPs {
		if ip == cip {
			w.Header().Add("X-Goatcounter", fmt.Sprintf("ignored because %q is in the IP ignore list", ip))
			w.WriteHeader(http.StatusAccepted)
			return zhttp.Bytes(w, gif)
		}
	}

	hit := goatcounter.Hit{
		Site:            site.ID,
		UserAgentHeader: r.UserAgent(),
		CreatedAt:       ztime.Now(),
		RemoteAddr:      cip,
	}
	if site.Settings.Collect.Has(goatcounter.CollectLocation) {
		var l goatcounter.Location
		hit.Location = l.LookupIP(r.Context(), cip)
	}

	if site.Settings.Collect.Has(goatcounter.CollectLanguage) {
		tags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
		if len(tags) > 0 {
			base, c := tags[0].Base()
			if c == language.Exact || c == language.High {
				l := base.ISO3()
				hit.Language = &l
			}
		}
	}

	err := json.NewDecoder(r.Body).Decode(&hit)
	if err != nil {
		w.Header().Add("X-Goatcounter", fmt.Sprintf("error decoding parameters: %s", err))
		w.WriteHeader(400)
		return zhttp.Bytes(w, gif)
	}
	if hit.Bot > 0 && hit.Bot < 150 {
		w.Header().Add("X-Goatcounter", fmt.Sprintf("wrong value: b=%d", hit.Bot))
		w.WriteHeader(400)
		return zhttp.Bytes(w, gif)
	}
	if len(hit.Path) > 2048 {
		w.Header().Add("X-Goatcounter", fmt.Sprintf("ignored because path is longer than 2048 bytes (%d bytes)",
			len(r.RequestURI)))
		w.WriteHeader(http.StatusRequestURITooLong)
		return zhttp.Bytes(w, gif)
	}

	if isbot.Is(bot) { // Prefer the backend detection.
		hit.Bot = int(bot)
	}

	err = hit.Validate(r.Context(), true)
	if err != nil {
		w.Header().Add("X-Goatcounter", fmt.Sprintf("not valid: %s", err))
		w.WriteHeader(400)
		return zhttp.Bytes(w, gif)
	}

	goatcounter.Memstore.Append(hit)
	return zhttp.Bytes(w, gif)
}

// Extract client IP in case of goatcounter sitting on top of one or more proxies
// https://gist.github.com/17twenty/c815680c9c585cd9c16e62cbee7317b6
func extractClientIP(r *http.Request) string {
	ffips := r.Header.Get(forwardedForHeader)
	rip := r.RemoteAddr

	if ffips == "" {
		return rip
	}

	rip = ffips
	ips := strings.Split(rip, ", ")
	if len(ips) > 1 {
		rip = ips[len(ips) - 1]
	}

	return rip
}
