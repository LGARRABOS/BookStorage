package database

import (
	"path/filepath"
	"testing"

	"bookstorage/internal/config"
)

func testDBSettings(t *testing.T, dir string) *config.Settings {
	t.Helper()
	return &config.Settings{
		Database:             filepath.Join(dir, "test.db"),
		SecretKey:            "0123456789abcdef0123456789abcdef",
		Environment:          "development",
		SuperadminUsername:   "admin",
		SuperadminPassword:   "TestAdmin!99",
		DataDirectory:        dir,
		UploadFolder:         filepath.Join(dir, "img"),
		ProfileUploadFolder:  filepath.Join(dir, "av"),
		UploadURLPath:        "images",
		ProfileUploadURLPath: "avatars",
	}
}

func copySpecCols(table string) []string {
	for _, spec := range sqliteToPostgresCopyTables {
		if spec.table == table {
			return spec.cols
		}
	}
	return nil
}

func TestCopyTable_preservesWorksDatesAndAnime(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcSettings := testDBSettings(t, srcDir)
	dstSettings := testDBSettings(t, dstDir)

	src, err := Open(srcSettings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	if err := EnsureSchema(src, srcSettings); err != nil {
		t.Fatal(err)
	}

	dst, err := Open(dstSettings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	if err := EnsureSchema(dst, dstSettings); err != nil {
		t.Fatal(err)
	}

	_, err = src.Exec(`
		INSERT INTO works (title, chapter, user_id, status, started_at, last_chapter_at, finished_at, link_probe_status)
		VALUES ('Dated', 4, 1, 'Terminé', '2026-01-02 03:04:05', '2026-02-03 04:05:06', '2026-03-04 05:06:07', 'up')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Exec(`
		INSERT INTO anime_works (title, episode, status, anime_type, user_id, updated_at)
		VALUES ('Frieren', 12, 'En cours', 'TV', 1, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := dst.Exec(`DELETE FROM works`); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`DELETE FROM anime_works`); err != nil {
		t.Fatal(err)
	}

	if err := copyTable(src.Std(), dst, "works", copySpecCols("works")); err != nil {
		t.Fatal(err)
	}
	if err := copyTable(src.Std(), dst, "anime_works", copySpecCols("anime_works")); err != nil {
		t.Fatal(err)
	}

	var started, last, finished, probe string
	if err := dst.QueryRow(`
		SELECT COALESCE(started_at,''), COALESCE(last_chapter_at,''), COALESCE(finished_at,''), COALESCE(link_probe_status,'')
		FROM works WHERE title = 'Dated'`).Scan(&started, &last, &finished, &probe); err != nil {
		t.Fatal(err)
	}
	if started == "" || last == "" || finished == "" {
		t.Fatalf("date columns not copied: started=%q last=%q finished=%q", started, last, finished)
	}
	if probe != "up" {
		t.Fatalf("link_probe_status=%q", probe)
	}

	var animeCount int
	if err := dst.QueryRow(`SELECT COUNT(*) FROM anime_works WHERE title = 'Frieren'`).Scan(&animeCount); err != nil {
		t.Fatal(err)
	}
	if animeCount != 1 {
		t.Fatalf("expected copied anime row, got %d", animeCount)
	}

	if err := verifyMigrationCounts(src.Std(), dst); err != nil {
		t.Fatal(err)
	}
}

func TestSqliteToPostgresCopyTables_coverDomainModules(t *testing.T) {
	need := map[string]bool{
		"anime_works": true, "bd_works": true, "manga_phys_works": true,
		"library_furniture": true, "library_shelves": true, "library_placements": true,
		"reading_activity_daily": true, "api_tokens": true, "webauthn_credentials": true,
	}
	for _, spec := range sqliteToPostgresCopyTables {
		delete(need, spec.table)
	}
	if len(need) > 0 {
		t.Fatalf("migration copy list missing tables: %v", need)
	}
	worksCols := map[string]bool{}
	for _, c := range copySpecCols("works") {
		worksCols[c] = true
	}
	for _, c := range []string{"started_at", "last_chapter_at", "finished_at", "link_probe_status"} {
		if !worksCols[c] {
			t.Fatalf("works copy missing column %s", c)
		}
	}
}
