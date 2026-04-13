package i18n

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed locales/*.json
var localesFS embed.FS

// Translations holds all text for a single language.
type Translations map[string]string

// LangInfo describes an available language for template dropdowns.
type LangInfo struct {
	Code string
	Name string
	Flag string
}

// langFlags maps language codes to country flag emojis.
var langFlags = map[string]string{
	"de": "🇩🇪",
	"en": "🇬🇧",
	"es": "🇪🇸",
	"fr": "🇫🇷",
	"it": "🇮🇹",
	"pt": "🇵🇹",
}

const (
	LangFR      = "fr"
	LangEN      = "en"
	DefaultLang = LangEN
)

var (
	allTranslations = map[string]Translations{}
	languages       []LangInfo
)

// statusKeyMap maps DB-stored status values (French) to i18n keys.
var statusKeyMap = map[string]string{
	"En cours":  "status.reading",
	"Terminé":   "status.completed",
	"En pause":  "status.on_hold",
	"Abandonné": "status.dropped",
	"À lire":    "status.plan_to_read",
}

func init() {
	Load()
}

// Load reads all JSON locale files from the embedded filesystem.
func Load() {
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		panic("i18n: cannot read embedded locales: " + err.Error())
	}

	loaded := map[string]Translations{}
	var langs []LangInfo

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		code := strings.TrimSuffix(e.Name(), ".json")

		data, err := localesFS.ReadFile("locales/" + e.Name())
		if err != nil {
			panic("i18n: cannot read " + e.Name() + ": " + err.Error())
		}

		var t Translations
		if err := json.Unmarshal(data, &t); err != nil {
			panic("i18n: invalid JSON in " + e.Name() + ": " + err.Error())
		}

		name := t["_lang.name"]
		if name == "" {
			name = code
		}
		delete(t, "_lang.name")

		loaded[code] = t
		flag := langFlags[code]
		langs = append(langs, LangInfo{Code: code, Name: name, Flag: flag})
	}

	sort.Slice(langs, func(i, j int) bool {
		return langs[i].Code < langs[j].Code
	})

	allTranslations = loaded
	languages = langs
}

// T returns translations for the given language, falling back to DefaultLang.
func T(lang string) Translations {
	if t, ok := allTranslations[lang]; ok {
		return t
	}
	return allTranslations[DefaultLang]
}

// Languages returns the list of available languages (sorted by code).
func Languages() []LangInfo {
	return languages
}

// ValidLang reports whether lang is a loaded language.
func ValidLang(lang string) bool {
	_, ok := allTranslations[lang]
	return ok
}

// TranslateStatus converts a DB-stored status (French) to the target language
// using the translations map.
func TranslateStatus(status string, t Translations) string {
	if key, ok := statusKeyMap[status]; ok {
		if val, ok := t[key]; ok {
			return val
		}
	}
	return status
}
