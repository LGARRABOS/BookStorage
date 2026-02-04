# BookStorage

Gestionnaire de lectures personnelles — version Go.

## Prérequis

- Go 1.22+
- GCC (pour la compilation de SQLite)

## Installation

```bash
go mod tidy
```

## Lancement

```bash
go run .
```

Le serveur démarre sur `http://127.0.0.1:5000`.

## Configuration (optionnelle)

Variables d'environnement supportées :

| Variable | Description | Défaut |
|----------|-------------|--------|
| `BOOKSTORAGE_HOST` | Adresse d'écoute | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | Port | `5000` |
| `BOOKSTORAGE_DATABASE` | Chemin de la base SQLite | `database.db` |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète | `dev-secret-change-me` |
| `BOOKSTORAGE_SUPERADMIN_USERNAME` | Username superadmin | `superadmin` |
| `BOOKSTORAGE_SUPERADMIN_PASSWORD` | Password superadmin | `SuperAdmin!2023` |

## Migration depuis la version Python

Pour importer une base de données existante :

1. Copiez votre fichier `database.db` de l'ancienne version
2. Lancez le serveur avec `go run .`
3. Les mots de passe hashés (format Werkzeug) sont automatiquement reconnus

## Structure

```
BookStorage/
├── main.go          # Point d'entrée
├── config.go        # Configuration et variables d'environnement
├── db.go            # Schéma SQLite et migrations
├── handlers.go      # Routes HTTP et logique métier
├── go.mod           # Dépendances Go
├── templates/       # Templates HTML (Go html/template)
└── static/          # CSS et fichiers statiques
```
