// Package i18n provides minimal UI translations (English + Czech) for the
// server-rendered views and the client-side JS snippets. Catalogs live in the
// JSON files next to this package and are embedded into the binary.
package i18n

import (
	"embed"
	"encoding/json"
	"log"
	"strings"
	"sync"
)

//go:embed en.json cs.json
var catalogFS embed.FS

var (
	loadOnce sync.Once
	catalogs map[string]map[string]string
)

func load() {
	catalogs = make(map[string]map[string]string)
	for _, lang := range []string{"en", "cs"} {
		data, err := catalogFS.ReadFile(lang + ".json")
		if err != nil {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			log.Printf("i18n: failed to load %s.json: %v", lang, err)
			continue
		}
		catalogs[lang] = m
	}
	if catalogs["en"] == nil {
		catalogs["en"] = map[string]string{}
	}
}

// Langs returns the supported language codes in preference order.
func Langs() []string { return []string{"en", "cs"} }

// IsValid reports whether code is a supported language.
func IsValid(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, l := range Langs() {
		if code == l {
			return true
		}
	}
	return false
}

// Normalize maps a raw language code to a supported one ("en" fallback).
func Normalize(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if i := strings.IndexAny(code, ",;-_"); i > 0 {
		code = code[:i]
	}
	if IsValid(code) {
		return code
	}
	return "en"
}

// T translates key for lang, falling back to English and then to the key.
func T(lang, key string) string {
	loadOnce.Do(load)
	if m, ok := catalogs[lang]; ok {
		if v, ok := m[key]; ok && v != "" {
			return v
		}
	}
	if v, ok := catalogs["en"][key]; ok && v != "" {
		return v
	}
	return key
}

// Catalog returns a copy of the full translation map for lang (never empty).
// Used to inject window._translations into pages for client-side strings.
func Catalog(lang string) map[string]string {
	loadOnce.Do(load)
	out := make(map[string]string, len(catalogs["en"]))
	for k, v := range catalogs["en"] {
		out[k] = v
	}
	if m, ok := catalogs[lang]; ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// CatalogJSON marshals the merged catalog for lang into compact JSON.
func CatalogJSON(lang string) string {
	data, err := json.Marshal(Catalog(lang))
	if err != nil {
		return "{}"
	}
	return string(data)
}
