# Architecture backend BookStorage

Guide pour les contributeurs et agents : où placer du code neuf, et comment le découpage limite les régressions lors des mises à jour.

## Carte des packages

```
cmd/bookstorage/          → point d’entrée HTTP
internal/
  server/                 → HTTP, sessions, jobs, import/export, rendu templates
  catalog/                → clients externes (AniList, MangaDex, Open Library, BnF, Google Books)
  database/               → SQLite / PostgreSQL, schéma, migrations
  config/                 → Settings, .env, site.json
  i18n/                   → locales JSON
  mail/                   → envoi email (reset password)
  oauthgoogle/            → OAuth Google
  recommend/              → recommandations
```

Dépendances typiques : `server` → `catalog` + `database` + `config`.  
Ne pas créer de cycles (ex. `catalog` ne doit pas importer `server`).

## Index agents

Avant de chercher dans le code, lire :

1. [`.cursor/index/MANIFEST.md`](../.cursor/index/MANIFEST.md)
2. [`.cursor/index/SYMBOLS.md`](../.cursor/index/SYMBOLS.md)
3. [`.cursor/index/ENDPOINTS.md`](../.cursor/index/ENDPOINTS.md)

Après ajout / renommage / déplacement de symboles ou routes : `repomap build`.

## Conventions de nommage (`internal/server`)

Tout reste dans **`package server`** (pas de sous-packages pour l’instant).

| Préfixe / motif | Rôle |
|-----------------|------|
| `handlers_*.go` | Handlers HTTP HTML |
| `routes_*.go` | Enregistrement des routes par module (manga, anime, bd, tools) |
| `paths_*.go` | Constantes d’URL |
| `*_row.go` | Types ligne DB + `Scan` |
| `import_export_*_parse.go` | Parsing pur (CSV, XML, JSON) |
| `import_export_*_handlers.go` | Handlers HTTP import/export |
| `*_cover_resolve.go` / `*_cover_job.go` | Résolution covers + file d’enrichissement |
| `cover_job_controller.go` | Contrôleur de file partagé anime/BD |
| `webhooks_store.go` / `_worker.go` / `_handlers.go` | Feature webhooks découpée |
| `api*.go` | API REST JSON |
| `*_test.go` | Tests à côté du fichier source |

Dans `internal/database` : `sqlite_schema.go` (DDL SQLite), `migrations_list.go` (liste), `migrations.go` (runner).

Dans `internal/config` : `settings.go` (struct + validation), `config_load.go` (`Load`).

Modèle de référence déjà en place : `webauthn.go` + `webauthn_handlers.go` + `webauthn_store.go`, et `password_reset.go` + `handlers_password_reset.go`.

## Règle d’or pour les MAJ

**Nouveau code → nouveau fichier thématique**, pas dans un fichier déjà volumineux qui mélange HTTP + SQL + parsing.

Exemples :

- Nouveau format d’import BD → `import_export_bd_parse.go` (parse) + handlers existants.
- Nouveau job background → fichier `*_job.go` dédié, pas dans un handler admin.
- Nouvelle route module → `paths_*.go` + `routes_*.go` + `handlers_*.go`.

## Modules domaine (manga / anime / BD)

Chaque module suit :

```
paths_<module>.go
routes_<module>.go
handlers_<module>_*.go
*_row.go                 (si modèle DB dédié)
import_export_<module>*.go
```

## Hors package `server`

- Clients HTTP externes → `internal/catalog/`
- DDL / migrations → `internal/database/`
- Variables d’environnement → `internal/config/`

## Vérifications après un découpage

```bash
go test ./...
repomap build
```

Comportement inchangé : déplacements mécaniques uniquement (pas de refactor logique dans le même commit si possible).
