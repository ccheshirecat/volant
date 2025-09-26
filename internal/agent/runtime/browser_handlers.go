package runtime

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/go-chi/chi/v5"
)

func (b *browserRuntime) mountBrowserRoutes(r chi.Router) {
	r.Post("/navigate", func(w http.ResponseWriter, r *http.Request) {
		var req navigateRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.URL) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("url is required"))
			return
		}
		if err := b.real.Navigate(b.duration(req.TimeoutMs), req.URL); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		okJSON(w)
	})

	r.Post("/reload", func(w http.ResponseWriter, r *http.Request) {
		var req reloadRequest
		_ = decodeRequest(r, &req)
		if err := b.real.Reload(b.duration(req.TimeoutMs), req.IgnoreCache); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		okJSON(w)
	})

	r.Post("/back", func(w http.ResponseWriter, r *http.Request) {
		if err := b.real.Back(b.duration(queryTimeout(r))); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		okJSON(w)
	})

	r.Post("/forward", func(w http.ResponseWriter, r *http.Request) {
		if err := b.real.Forward(b.duration(queryTimeout(r))); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		okJSON(w)
	})

	r.Post("/viewport", func(w http.ResponseWriter, r *http.Request) {
		var req viewportRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := b.real.SetViewport(b.duration(req.TimeoutMs), req.Width, req.Height, req.Scale, req.Mobile); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})

	r.Post("/user-agent", func(w http.ResponseWriter, r *http.Request) {
		var req userAgentRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := b.real.SetUserAgent(b.duration(req.TimeoutMs), req.UserAgent, req.AcceptLanguage, req.Platform); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})

	r.Post("/wait-navigation", func(w http.ResponseWriter, r *http.Request) {
		var req waitNavigationRequest
		_ = decodeRequest(r, &req)
		if err := b.real.WaitForNavigation(b.duration(req.TimeoutMs)); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})

	r.Post("/screenshot", func(w http.ResponseWriter, r *http.Request) {
		var req screenshotRequest
		_ = decodeRequest(r, &req)
		data, err := b.real.Screenshot(b.duration(req.TimeoutMs), req.FullPage, req.Format, req.Quality)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"data":        base64.StdEncoding.EncodeToString(data),
			"format":      strings.ToLower(req.Format),
			"full_page":   req.FullPage,
			"byte_length": len(data),
			"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	})
}

func (b *browserRuntime) mountDOMRoutes(r chi.Router) {
	r.Post("/click", func(w http.ResponseWriter, r *http.Request) {
		var req clickRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := b.real.Click(b.duration(req.TimeoutMs), req.Selector, req.Button); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})

	r.Post("/type", func(w http.ResponseWriter, r *http.Request) {
		var req typeRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := b.real.Type(b.duration(req.TimeoutMs), req.Selector, req.Value, req.Clear); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})

	r.Post("/get-text", func(w http.ResponseWriter, r *http.Request) {
		var req textRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		text, err := b.real.GetText(b.duration(req.TimeoutMs), req.Selector, req.Visible)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"text": text})
	})

	r.Post("/get-html", func(w http.ResponseWriter, r *http.Request) {
		var req htmlRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		html, err := b.real.GetHTML(b.duration(req.TimeoutMs), req.Selector)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"html": html})
	})

	r.Post("/get-attribute", func(w http.ResponseWriter, r *http.Request) {
		var req attributeRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		value, ok, err := b.real.GetAttribute(b.duration(req.TimeoutMs), req.Selector, req.Name)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"value": value, "exists": ok})
	})

	r.Post("/wait-selector", func(w http.ResponseWriter, r *http.Request) {
		var req waitSelectorRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := b.real.WaitForSelector(b.duration(req.TimeoutMs), req.Selector, req.Visible); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		okJSON(w)
	})
}

func (b *browserRuntime) mountScriptRoutes(r chi.Router) {
	r.Post("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		var req evaluateRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Expression) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("expression is required"))
			return
		}
		result, err := b.real.Evaluate(b.duration(req.TimeoutMs), req.Expression, req.AwaitPromise)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"result": result})
	})
}

func (b *browserRuntime) mountActionRoutes(r chi.Router) {
	r.Post("/navigate", func(w http.ResponseWriter, r *http.Request) {
		var req navigateRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.URL) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("url is required"))
			return
		}
		if err := b.real.Navigate(b.duration(req.TimeoutMs), req.URL); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		okJSON(w)
	})

	r.Post("/screenshot", func(w http.ResponseWriter, r *http.Request) {
		var req screenshotRequest
		_ = decodeRequest(r, &req)
		data, err := b.real.Screenshot(b.duration(req.TimeoutMs), req.FullPage, req.Format, req.Quality)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"data":        base64.StdEncoding.EncodeToString(data),
			"format":      strings.ToLower(req.Format),
			"full_page":   req.FullPage,
			"byte_length": len(data),
			"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	r.Post("/scrape", func(w http.ResponseWriter, r *http.Request) {
		var req scrapeRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Selector) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("selector is required"))
			return
		}
		timeout := b.duration(req.TimeoutMs)
		if attr := strings.TrimSpace(req.Attribute); attr != "" {
			value, exists, err := b.real.GetAttribute(timeout, req.Selector, attr)
			if err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			respondJSON(w, http.StatusOK, map[string]any{"value": value, "exists": exists})
			return
		}
		text, err := b.real.GetText(timeout, req.Selector, true)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"value": text, "exists": true})
	})

	r.Post("/evaluate", func(w http.ResponseWriter, r *http.Request) {
		var req evaluateRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Expression) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("expression is required"))
			return
		}
		result, err := b.real.Evaluate(b.duration(req.TimeoutMs), req.Expression, req.AwaitPromise)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"result": result})
	})

	r.Post("/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.Endpoint) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("endpoint is required"))
			return
		}
		if strings.TrimSpace(req.Query) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("query is required"))
			return
		}
		if req.Variables == nil {
			req.Variables = map[string]any{}
		}
		response, err := b.real.GraphQL(b.duration(req.TimeoutMs), req.Endpoint, req.Query, req.Variables)
		if err != nil {
			errorJSON(w, http.StatusBadGateway, err)
			return
		}
		respondJSON(w, http.StatusOK, response)
	})
}

func (b *browserRuntime) mountProfileRoutes(r chi.Router) {
	r.Post("/attach", func(w http.ResponseWriter, r *http.Request) {
		var req profileAttachRequest
		if err := decodeRequest(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}

		var cookies []*network.CookieParam
		for _, c := range req.Cookies {
			cookie, err := convertCookieParam(c)
			if err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			cookies = append(cookies, cookie)
		}

		timeout := b.duration(req.Timeout)
		if len(cookies) > 0 {
			if err := b.real.SetCookies(timeout, cookies); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
		}

		if len(req.Local) > 0 || len(req.Session) > 0 {
			payload := StoragePayload{Local: req.Local, Session: req.Session}
			if err := b.real.SetStorage(timeout, payload); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
		}

		okJSON(w)
	})

	r.Get("/extract", func(w http.ResponseWriter, r *http.Request) {
		cookies, err := b.real.GetCookies(b.duration(queryTimeout(r)))
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		storage, err := b.real.GetStorage(b.defaultTimeout)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"cookies":         mapCookies(cookies),
			"local_storage":   storage.Local,
			"session_storage": storage.Session,
		})
	})
}

// Helpers

func decodeRequest[T any](r *http.Request, dest *T) error {
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(dest)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorJSON(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]any{"error": err.Error()})
}

func okJSON(w http.ResponseWriter) {
	respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func queryTimeout(r *http.Request) int64 {
	if value := r.URL.Query().Get("timeout_ms"); value != "" {
		if ms, err := strconv.ParseInt(value, 10, 64); err == nil && ms > 0 {
			return ms
		}
	}
	return 0
}

// Request payloads reused from legacy handlers

// navigateRequest mirrors the legacy App request payload.
type navigateRequest struct {
	URL       string `json:"url"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// reloadRequest mirrors the legacy App request payload.
type reloadRequest struct {
	IgnoreCache bool  `json:"ignore_cache"`
	TimeoutMs   int64 `json:"timeout_ms"`
}

// viewportRequest mirrors the legacy App request payload.
type viewportRequest struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Scale     float64 `json:"scale"`
	Mobile    bool    `json:"mobile"`
	TimeoutMs int64   `json:"timeout_ms"`
}

// userAgentRequest mirrors the legacy App request payload.
type userAgentRequest struct {
	UserAgent      string `json:"user_agent"`
	AcceptLanguage string `json:"accept_language"`
	Platform       string `json:"platform"`
	TimeoutMs      int64  `json:"timeout_ms"`
}

// waitNavigationRequest mirrors the legacy App request payload.
type waitNavigationRequest struct {
	TimeoutMs int64 `json:"timeout_ms"`
}

// screenshotRequest mirrors the legacy App request payload.
type screenshotRequest struct {
	FullPage  bool   `json:"full_page"`
	Format    string `json:"format"`
	Quality   int    `json:"quality"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// clickRequest mirrors the legacy App request payload.
type clickRequest struct {
	Selector  string `json:"selector"`
	Button    string `json:"button"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// typeRequest mirrors the legacy App request payload.
type typeRequest struct {
	Selector  string `json:"selector"`
	Value     string `json:"value"`
	Clear     bool   `json:"clear"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// textRequest mirrors the legacy App request payload.
type textRequest struct {
	Selector  string `json:"selector"`
	Visible   bool   `json:"visible"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// htmlRequest mirrors the legacy App request payload.
type htmlRequest struct {
	Selector  string `json:"selector"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// attributeRequest mirrors the legacy App request payload.
type attributeRequest struct {
	Selector  string `json:"selector"`
	Name      string `json:"name"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// waitSelectorRequest mirrors the legacy App request payload.
type waitSelectorRequest struct {
	Selector  string `json:"selector"`
	Visible   bool   `json:"visible"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// evaluateRequest mirrors the legacy App request payload.
type evaluateRequest struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise"`
	TimeoutMs    int64  `json:"timeout_ms"`
}

// scrapeRequest mirrors the legacy App request payload.
type scrapeRequest struct {
	Selector  string `json:"selector"`
	Attribute string `json:"attribute"`
	TimeoutMs int64  `json:"timeout_ms"`
}

// graphqlRequest mirrors the legacy App request payload.
type graphqlRequest struct {
	Endpoint  string         `json:"endpoint"`
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
	TimeoutMs int64          `json:"timeout_ms"`
}

// cookieParamRequest mirrors the legacy App request payload.
type cookieParamRequest struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Domain   string   `json:"domain"`
	Path     string   `json:"path"`
	Expires  *float64 `json:"expires"`
	HTTPOnly bool     `json:"http_only"`
	Secure   bool     `json:"secure"`
	SameSite string   `json:"same_site"`
}

// profileAttachRequest mirrors the legacy App request payload.
type profileAttachRequest struct {
	Cookies []cookieParamRequest `json:"cookies"`
	Local   map[string]string    `json:"local_storage"`
	Session map[string]string    `json:"session_storage"`
	Timeout int64                `json:"timeout_ms"`
}

// Helper functions reused from legacy app

func convertCookieParam(req cookieParamRequest) (*network.CookieParam, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("cookie name required")
	}
	cookie := &network.CookieParam{
		Name:     req.Name,
		Value:    req.Value,
		Domain:   req.Domain,
		Path:     req.Path,
		HTTPOnly: req.HTTPOnly,
		Secure:   req.Secure,
	}
	if req.SameSite != "" {
		switch strings.ToLower(req.SameSite) {
		case "lax":
			cookie.SameSite = network.CookieSameSiteLax
		case "strict":
			cookie.SameSite = network.CookieSameSiteStrict
		case "none":
			cookie.SameSite = network.CookieSameSiteNone
		default:
			return nil, errors.New("invalid same_site")
		}
	}
	if req.Expires != nil {
		epoch := cdp.TimeSinceEpoch(time.Unix(int64(*req.Expires), 0).UTC())
		cookie.Expires = &epoch
	}
	return cookie, nil
}

func mapCookies(cookies []*network.Cookie) []map[string]any {
	result := make([]map[string]any, 0, len(cookies))
	for _, c := range cookies {
		item := map[string]any{
			"name":      c.Name,
			"value":     c.Value,
			"domain":    c.Domain,
			"path":      c.Path,
			"http_only": c.HTTPOnly,
			"secure":    c.Secure,
			"same_site": strings.ToLower(string(c.SameSite)),
		}
		item["expires"] = c.Expires
		result = append(result, item)
	}
	return result
}
