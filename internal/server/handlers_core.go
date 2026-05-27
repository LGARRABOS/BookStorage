package server

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bookstorage/internal/config"
	"bookstorage/internal/database"
	"bookstorage/internal/i18n"
)

type App struct {
	Settings           *config.Settings
	SiteConfig         *config.SiteConfig
	DB                 *database.Conn
	TemplatesWeb       *template.Template
	TemplatesMobile    *template.Template
	Version            string
	ProcessStartedAt   time.Time
	webAuthnChallenges *webauthnChallengeStore
	dbProbe            dbAvailabilityProbe
}

func NewApp(settings *config.Settings, siteConfig *config.SiteConfig, db *database.Conn, version string) *App {
	funcMap := template.FuncMap{
		"work_image_url": func(stored string) string {
			return workImageURL(settings, stored)
		},
		"url_for": func(name string, args ...string) string {
			switch name {
			case "static":
				if len(args) > 0 {
					filename := strings.TrimLeft(args[0], "/")
					return "/static/" + filename
				}
				return "/static/"
			default:
				return "/" + strings.TrimLeft(name, "/")
			}
		},
		// Generate a sequence of numbers (for star ratings)
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i + 1
			}
			return s
		},
		// Comparisons for star ratings
		"le": func(a, b int) bool {
			return a <= b
		},
		"ge": func(a, b int) bool {
			return a >= b
		},
		// Math for stats
		"divf": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b)
		},
		"mulf": func(a, b float64) float64 {
			return a * b
		},
		// Translation function
		"t": func(translations i18n.Translations, key string) string {
			if val, ok := translations[key]; ok {
				return val
			}
			return key
		},
		// Safe JavaScript string literal for <script> (avoid printf "%q" + html/template double-escaping).
		"jsstr": func(s string) template.JS {
			b, err := json.Marshal(s)
			if err != nil {
				return template.JS(`""`)
			}
			return template.JS(b)
		},
		"toJSON": func(v any) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		// Translate status (database stores French values)
		"hasPrefix": strings.HasPrefix,
		"translateStatus": func(status string, t i18n.Translations) string {
			return i18n.TranslateStatus(status, t)
		},
		"upper": strings.ToUpper,
		"join":  strings.Join,
		"int":   func(v int64) int { return int(v) },
		"fmtDateDisplay": func(n nullFlexTime) string {
			if !n.Valid || n.String == "" {
				return ""
			}
			for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339, "2006-01-02"} {
				if t, err := time.Parse(layout, n.String); err == nil {
					return t.Format("02/01/2006")
				}
			}
			if len(n.String) >= 10 {
				return n.String[:10]
			}
			return n.String
		},
		"fmtDateInput": func(n nullFlexTime) string {
			if !n.Valid || n.String == "" {
				return ""
			}
			for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z", time.RFC3339} {
				if t, err := time.Parse(layout, n.String); err == nil {
					return t.Format("2006-01-02")
				}
			}
			if len(n.String) >= 10 {
				return n.String[:10]
			}
			return n.String
		},
		"fmtProbeTime": func(s sql.NullString) string {
			if !s.Valid || s.String == "" {
				return "—"
			}
			t, err := time.Parse("2006-01-02 15:04:05", s.String)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02T15:04:05Z", s.String)
				if err2 != nil {
					return s.String
				}
				t = t2
			}
			if loc, err := time.LoadLocation(settings.Timezone); err == nil {
				t = t.In(loc)
			}
			return t.Format("02/01/2006 15:04")
		},
		"probeTimeISO": func(s sql.NullString) string {
			if !s.Valid || s.String == "" {
				return ""
			}
			t, err := time.Parse("2006-01-02 15:04:05", s.String)
			if err != nil {
				t2, err2 := time.Parse("2006-01-02T15:04:05Z", s.String)
				if err2 != nil {
					return ""
				}
				t = t2
			}
			return t.UTC().Format(time.RFC3339)
		},
	}
	webTpl := mustLoadTemplates(funcMap, []string{
		filepath.Join("templates", "shared"),
		"templates",
		filepath.Join("templates", "web"),
	})
	mobileTpl := mustLoadTemplates(funcMap, []string{
		filepath.Join("templates", "shared"),
		"templates",
		filepath.Join("templates", "web"),
		filepath.Join("templates", "mobile"),
	})
	return &App{
		Settings:           settings,
		SiteConfig:         siteConfig,
		DB:                 db,
		TemplatesWeb:       webTpl,
		TemplatesMobile:    mobileTpl,
		Version:            strings.TrimSpace(version),
		webAuthnChallenges: newWebAuthnChallengeStore(),
	}
}

func mustLoadTemplates(funcMap template.FuncMap, directories []string) *template.Template {
	files := collectTemplateFiles(directories...)
	if len(files) == 0 {
		log.Fatal("no templates found")
	}
	tpl, err := template.New("").Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}
	return tpl
}

func collectTemplateFiles(dirs ...string) []string {
	var files []string
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		rootIsTemplates := filepath.Clean(dir) == filepath.Clean("templates")
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if rootIsTemplates && filepath.Clean(p) != filepath.Clean(dir) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(d.Name()) != ".gohtml" {
				return nil
			}
			clean := filepath.Clean(p)
			if _, ok := seen[clean]; ok {
				return nil
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
			return nil
		})
	}
	return files
}
