# BookStorage (FR)

**BookStorage** est une application web de suivi de lectures personnelles. Suivez vos romans, mangas, webtoons, light novels et plus encore.

_🇬🇧 [English version](./README.md)_

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## Présentation

BookStorage est une application web **auto-hébergée** pour centraliser et suivre vos lectures. Évaluations, notes, statistiques, bibliothèques communautaires, mode sombre, PWA, raccourcis clavier — tout s'exécute **chez vous** avec une base **SQLite** ou **PostgreSQL**.

### Fonctionnalités

- Bibliothèque multi-formats (romans, mangas, webtoons, light novels…)
- Notes, statistiques, bibliothèques publiques
- Mode sombre, interface multilingue (FR/EN/DE/ES/IT/PT), PWA installable
- PWA mobile avec tableau de bord simplifié et chapitres +/-
- Export/import (CSV, JSON) + import MyAnimeList et AniList
- Recommandations AniList, intégration catalogue
- Panel admin, métriques Prometheus, Google OAuth

---

## Démarrage rapide

**Prérequis :** Go 1.22+, GCC (CGO pour SQLite).

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Ouvrez **http://127.0.0.1:5000**

---

## Production (Linux)

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
bsctl start
```

`install.sh` génère un `.env` avec `BOOKSTORAGE_ENV=production`, une clé secrète et un mot de passe superadmin aléatoires (affiché une seule fois). Vérifiez aussi :

| Variable | Recommandation |
|----------|----------------|
| `BOOKSTORAGE_ENABLE_HSTS` | `true` derrière HTTPS |
| `BOOKSTORAGE_TRUST_PROXY` | `true` si reverse-proxy de confiance |
| `BOOKSTORAGE_POSTGRES_URL` | `sslmode=require` si la DB est sur Internet ; `disable` OK sur IP LAN privées |

Checklist post-install : changer le mot de passe superadmin si besoin, activer HSTS, lancer `./scripts/ci/security_smoke.sh` contre l’instance.

---

## Documentation

La documentation complète est disponible sur le **[Wiki](https://github.com/LGARRABOS/BookStorage/wiki)** :

- [Installation](https://github.com/LGARRABOS/BookStorage/wiki/Installation) — mise en place développement et production
- [Configuration](https://github.com/LGARRABOS/BookStorage/wiki/Configuration) — variables d'environnement, OAuth, PostgreSQL
- [Usage](https://github.com/LGARRABOS/BookStorage/wiki/Usage) — tableau de bord, PWA, export/import, raccourcis
- [API Reference](https://github.com/LGARRABOS/BookStorage/wiki/API-Reference) — endpoints de l'API REST
- [Architecture](https://github.com/LGARRABOS/BookStorage/wiki/Architecture) — stack technique, structure du projet
- [Database](https://github.com/LGARRABOS/BookStorage/wiki/Database) — schéma, migrations, recherche plein texte
- [Authentication & Security](https://github.com/LGARRABOS/BookStorage/wiki/Authentication-and-Security) — authentification, sessions, sécurité
- [CI / CD](https://github.com/LGARRABOS/BookStorage/wiki/CI-CD) — pipeline, déploiement, CLI bsctl
- [Troubleshooting](https://github.com/LGARRABOS/BookStorage/wiki/Troubleshooting) — problèmes courants et solutions

---

## Licence

[MIT License](./LICENSE)
