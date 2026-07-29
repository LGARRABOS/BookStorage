package database

const createUsersTableSQL = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password TEXT,
    validated INTEGER DEFAULT 0,
    is_admin INTEGER DEFAULT 0,
    is_superadmin INTEGER DEFAULT 0,
    display_name TEXT,
    email TEXT,
    bio TEXT,
    avatar_path TEXT,
    is_public INTEGER DEFAULT 1,
    google_sub TEXT UNIQUE,
    google_email TEXT
);`

const createWorksTableSQL = `
CREATE TABLE IF NOT EXISTS works (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    chapter INTEGER DEFAULT 0,
    link TEXT,
    status TEXT,
    image_path TEXT,
    reading_type TEXT,
    user_id INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id)
);`

const createAnimeWorksTableSQL = `
CREATE TABLE IF NOT EXISTS anime_works (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    episode INTEGER DEFAULT 0,
    total_episodes INTEGER,
    status TEXT,
    anime_type TEXT,
    link TEXT,
    image_path TEXT,
    rating INTEGER DEFAULT 0,
    notes TEXT,
    is_adult INTEGER DEFAULT 0,
    source TEXT DEFAULT 'manual',
    external_id TEXT,
    user_id INTEGER NOT NULL,
    updated_at DATETIME,
    started_at DATETIME,
    finished_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_anime_works_user_id ON anime_works(user_id);`

const createBdWorksTableSQL = `
CREATE TABLE IF NOT EXISTS bd_works (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    tome INTEGER DEFAULT 0,
    total_tomes INTEGER,
    status TEXT,
    bd_type TEXT,
    link TEXT,
    image_path TEXT,
    rating INTEGER DEFAULT 0,
    notes TEXT,
    is_adult INTEGER DEFAULT 0,
    source TEXT DEFAULT 'manual',
    external_id TEXT,
    isbn TEXT,
    user_id INTEGER NOT NULL,
    updated_at DATETIME,
    started_at DATETIME,
    finished_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_bd_works_user_id ON bd_works(user_id);`

const createMangaPhysWorksTableSQL = `
CREATE TABLE IF NOT EXISTS manga_phys_works (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    tome INTEGER DEFAULT 0,
    total_tomes INTEGER,
    status TEXT,
    manga_type TEXT,
    link TEXT,
    image_path TEXT,
    rating INTEGER DEFAULT 0,
    notes TEXT,
    is_adult INTEGER DEFAULT 0,
    source TEXT DEFAULT 'manual',
    external_id TEXT,
    user_id INTEGER NOT NULL,
    updated_at DATETIME,
    started_at DATETIME,
    finished_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_manga_phys_works_user_id ON manga_phys_works(user_id);`

const createLibraryFurnitureTableSQL = `
CREATE TABLE IF NOT EXISTS library_furniture (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    room_label TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_library_furniture_user_id ON library_furniture(user_id);`

const createLibraryShelvesTableSQL = `
CREATE TABLE IF NOT EXISTS library_shelves (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    furniture_id INTEGER NOT NULL,
    label TEXT NOT NULL,
    case_count INTEGER NOT NULL DEFAULT 1,
    books_per_case INTEGER NOT NULL DEFAULT 8,
    sort_order INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (furniture_id) REFERENCES library_furniture (id) ON DELETE CASCADE,
    UNIQUE (furniture_id, label)
);
CREATE INDEX IF NOT EXISTS idx_library_shelves_furniture_id ON library_shelves(furniture_id);`

const createLibraryPlacementsTableSQL = `
CREATE TABLE IF NOT EXISTS library_placements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    shelf_id INTEGER NOT NULL,
    case_num INTEGER NOT NULL,
    position INTEGER NOT NULL DEFAULT 1,
    media_kind TEXT NOT NULL,
    work_id INTEGER NOT NULL,
    volume INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id),
    FOREIGN KEY (shelf_id) REFERENCES library_shelves (id) ON DELETE CASCADE,
    UNIQUE (user_id, media_kind, work_id, volume),
    UNIQUE (shelf_id, case_num, position)
);
CREATE INDEX IF NOT EXISTS idx_library_placements_user_media
    ON library_placements(user_id, media_kind, work_id, volume);
CREATE INDEX IF NOT EXISTS idx_library_placements_shelf_case
    ON library_placements(shelf_id, case_num, position);`

const createCatalogTableSQL = `
CREATE TABLE IF NOT EXISTS catalog (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    reading_type TEXT NOT NULL,
    image_url TEXT,
    source TEXT NOT NULL DEFAULT 'manual',
    external_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createUserCatalogBlocklistSQL = `
CREATE TABLE IF NOT EXISTS user_catalog_blocklist (
    user_id INTEGER NOT NULL,
    label_type TEXT NOT NULL,
    label_name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, label_type, label_name),
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_user_catalog_blocklist_user_id ON user_catalog_blocklist(user_id);`

var catalogColumns = map[string]string{
	"synopsis":   "TEXT",
	"alt_titles": "TEXT",
	"genres":     "TEXT",
	"tags":       "TEXT",
	"fetched_at": "DATETIME",
}

const createDismissedRecommendationsTableSQL = `
CREATE TABLE IF NOT EXISTS dismissed_recommendations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, source, external_id),
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_dismissed_recommendations_user_source
    ON dismissed_recommendations(user_id, source);`

const createReadingSitesTableSQL = `
CREATE TABLE IF NOT EXISTS reading_sites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    last_probe_at DATETIME,
    probe_status TEXT DEFAULT 'unknown',
    probe_http_status INTEGER,
    probe_detail TEXT,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_reading_sites_user_id ON reading_sites(user_id);`

const createSessionsTableSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    ip TEXT,
    user_agent TEXT,
    revoked_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users (id)
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_revoked ON sessions(user_id, revoked_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`

var profileColumns = map[string]string{
	"display_name":  "TEXT",
	"email":         "TEXT",
	"bio":           "TEXT",
	"avatar_path":   "TEXT",
	"is_public":     "INTEGER DEFAULT 1",
	"home_section": "TEXT DEFAULT 'hub'",
}

var workColumns = map[string]string{
	"reading_type": "TEXT",
	"rating":       "INTEGER DEFAULT 0",
	"notes":        "TEXT",
	"updated_at":   "DATETIME",
	"is_adult":     "INTEGER DEFAULT 0",
	"catalog_id":   "INTEGER REFERENCES catalog(id)",
	// 1 = exclue du lot / file enrichissement AniList (œuvre sans correspondance catalogue).
	"anilist_enrich_opt_out": "INTEGER DEFAULT 0",
	// 1 = suivi ; 0 = non-suivi (filtre tableau de bord), pertinent surtout pour « En cours ».
	"notify_new_chapters":    "INTEGER DEFAULT 1",
	"reading_site_id":        "INTEGER REFERENCES reading_sites(id)",
	"started_at":             "DATETIME",
	"last_chapter_at":        "DATETIME",
	"finished_at":            "DATETIME",
	"link_probe_status":      "TEXT DEFAULT 'unknown'",
	"link_probe_at":          "DATETIME",
	"link_probe_http_status": "INTEGER",
	"link_probe_detail":      "TEXT",
}

var bdWorksColumns = map[string]string{
	"isbn": "TEXT",
}
