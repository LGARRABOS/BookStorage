# Développement

Développement local, outillage, CI et organisation du code. Pour faire tourner BookStorage comme service sous Linux (systemd, mises à jour, `.env` en production), voir [Hébergement](hebergement.md).

---

## Sommaire

- [Démarrage rapide](#démarrage-rapide)
- [Environnement local](#environnement-local)
- [Makefile](#makefile)
- [Tests et lint en local](#tests-et-lint-en-local)
- [Intégration & déploiement continus](#intégration--déploiement-continus)
- [bsctl côté développement](#bsctl-dev)
- [Complétion par Tab (bash)](#complétion-par-tab-bash)
- [Structure du projet et architecture](#structure-du-projet-et-architecture)
- [Contribuer](#contribuer)

---

## Démarrage rapide

### Prérequis

- **Go 1.22+**
- **GCC** (pour la compilation de SQLite avec CGO)

### Lancer l’application

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Le serveur écoute par défaut sur **http://127.0.0.1:5000**.

Autres options : `make run`, `bsctl run`, ou lancer le package `cmd/bookstorage` depuis l’IDE.

---

## Environnement local

En développement, copiez le modèle d’environnement puis adaptez-le :

```bash
cp .env.example .env
```

Les valeurs par défaut suffisent souvent en local ; pour la liste complète des variables (chemins prod, secrets, `site.json`), voir [Hébergement — Configuration](hebergement.md#configuration).

---

## Makefile

Les cibles `make` recouvrent en partie `bsctl`. Préférez **`bsctl help`** comme référence CLI en anglais (voir [`scripts/bsctl`](../../scripts/bsctl)).

| Cible | Rôle |
|-------|------|
| `make build` | Build debug : `go build -o bookstorage ./cmd/bookstorage` |
| `make build-prod` | Binaire optimisé avec `-ldflags` et `APP_VERSION` du Makefile |
| `make run` | `go run ./cmd/bookstorage` |
| `make clean` | Supprime le binaire `bookstorage` à la racine du dépôt |
| `make test` | Tests unitaires avec profil de couverture (`coverage.out`) |
| `make test-race` | Suite de tests avec détecteur de race |
| `make lint` | Vérifie `gofmt -l .` + exécute `golangci-lint` |
| `make ci-local` | Parité CI locale (`lint`, `test`, `test-race`) |
| `make help` | Aide courte (messages parfois en français) |

Les cibles orientées production (`install`, `uninstall`, `update`, etc.) nécessitent root ou une machine configurée ; elles sont décrites côté opérateur dans [Hébergement](hebergement.md).

---

## Tests et lint en local

Alignez-vous sur [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) avant d’ouvrir une PR :

```bash
go mod download
go test ./... -coverprofile=coverage.out
go test -race ./...
```

Formatage :

```bash
gofmt -w .
# ou : gofmt -l .   # liste les fichiers à formater
```

Lint ([golangci-lint](https://golangci-lint.run/) à installer si besoin) :

```bash
golangci-lint run
```

La CI exécute aussi `gofmt` en mode strict et un job **smoke-http** qui lance `go run ./cmd/bookstorage` et interroge `/`, `/login`, `/register`.

### Notes API et import

- `GET /metrics` expose les métriques au format texte Prometheus (`bookstorage_http_*`). Sans `BOOKSTORAGE_METRICS_TOKEN`, seul le loopback peut scraper ; sinon `Authorization: Bearer …` ou `?token=…`. Voir [Hébergement — Metriques Prometheus](hebergement.md#metriques-prometheus-optionnel).
- `GET /api/admin/prometheus/summary` (admin, vue bureau) renvoie un petit JSON issu de l’API HTTP Prometheus locale (`BOOKSTORAGE_PROMETHEUS_QUERY_URL`, loopback uniquement) pour la page Monitoring.
- `GET /api/works` supporte la pagination (`page`, `limit`), des filtres (`status`, `reading_type`, `search`) et le tri (`sort`).
- La réponse inclut `data` et `meta` (`total`, `total_pages`, `has_next`, `has_prev`).
- L’import accepte en plus des exports BookStorage les formats externes courants **MyAnimeList** (CSV) et **AniList** (JSON/CSV).

### Durcissement HTTP

- Les requêtes mutatrices authentifiées (POST/PATCH/DELETE/PUT) sont filtrées via vérification d’origine (`Origin`/`Referer`) pour limiter les risques CSRF.
- Un rate limiting léger est appliqué sur les endpoints sensibles (authentification et écritures fréquentes).
- Le serveur HTTP est démarré avec des timeouts explicites (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`) dans `cmd/bookstorage/main.go` pour une meilleure résilience sous charge et face au trafic de type slowloris.
- L'implémentation actuelle de la recherche utilise des patterns SQL `LIKE` ; si la bibliothèque atteint une taille significative, un chemin FTS5 peut être introduit tout en conservant le contrat API stable.

---

## Intégration & déploiement continus

### CI (GitHub Actions)

À chaque **push** (toutes branches) et pour les **pull requests** vers `main`, le fichier `.github/workflows/ci.yml` lance :

| Job | Contenu |
|-----|---------|
| **Lint** | `gofmt -l .` doit être vide ; `golangci-lint` |
| **Unit tests** | `go test ./... -coverprofile=coverage.out` (artefact de couverture) |
| **Race tests** | `go test -race ./...` |
| **Build Linux binary** | `go build -o bookstorage ./cmd/bookstorage` (binaire publié en artefact CI) |
| **HTTP smoke tests** | Téléchargement du binaire artefact CI, démarrage de l’app, attente sur `/`, puis requêtes sur `/`, `/login`, `/register` |

L’ordre d’exécution est organisé par étapes : **Lint** d’abord, puis **Unit tests** et **Race tests** en parallèle, ensuite **Build Linux binary**, puis **HTTP smoke tests**.

La CI active aussi la concurrence de workflow (`cancel-in-progress`) pour annuler automatiquement les exécutions obsolètes sur une même branche.

Tous les jobs de cette chaîne doivent passer pour merger.

### Tests de sécurité en CI

Le même déclencheur (push / PR) lance aussi des **jobs orientés sécurité** en parallèle du pipeline principal. Ces jobs utilisent `continue-on-error` et ne **bloquent jamais un merge** (mode warn-only / observabilité).

| Job | Outil | Ce qu'il vérifie |
|-----|-------|------------------|
| **SAST** | `gosec` | Analyse statique du code Go pour les problèmes de sécurité courants |
| **Vulnérabilités dépendances** | `govulncheck` | CVE connues dans les modules Go |
| **Scan de secrets** | `gitleaks` | Identifiants, clés API ou tokens dans l'historique git |
| **DAST smoke** | `scripts/ci/security_smoke.sh` | Vérifications live sur l'app en fonctionnement : en-têtes sécurité, auth API (401), mauvaises méthodes (405), blocage CSRF par origin (403), rate limiting auth (429), protection des routes admin |

Chaque job publie un rapport en artefact (JSON ou TXT) consultable après exécution.

#### Trajectoire de durcissement

Les jobs sécurité sont conçus pour une montée en rigueur progressive :

- **Phase 1 (actuelle) :** Observabilité uniquement -- tous les jobs sont en `continue-on-error: true`. Consulter les artefacts après chaque run pour évaluer le bruit de base.
- **Phase 2 :** Retirer `continue-on-error` sur `gosec` et `govulncheck` pour que les alertes High/Critical bloquent les PR. Possibilité d'ajouter des seuils de sévérité (`gosec -severity=high`, code de sortie `govulncheck`).
- **Phase 3 :** Resserrer à Medium+ une fois la base propre ; ajouter `gitleaks` aux checks obligatoires.

<a id="workflow-de-déploiement"></a>
### Workflow de déploiement

Le workflow `.github/workflows/deploy.yml` produit un binaire **Linux amd64** avec CGO, empaquette `bookstorage`, `bsctl` et `deploy/bookstorage.service`, et publie une archive `bookstorage-linux-amd64` (`.tar.gz`) via un déclenchement manuel **Run workflow**.

L’usage de cet artefact sur un serveur (`bsctl install`, chemins, mises à jour) est décrit dans [Hébergement](hebergement.md).

---

<a id="bsctl-dev"></a>
## bsctl côté développement

`bsctl` est le même script qu’en production ; pour les commandes **service** (`start`, `stop`, `update`, …) et **installation**, voir [Hébergement — bsctl](hebergement.md#bsctl--service-et-mises-à-jour).

| Commande | Description |
|----------|-------------|
| `bsctl build` | Compiler l’application |
| `bsctl build-prod` | Binaire optimisé avec `-ldflags` et `APP_VERSION` de `scripts/bsctl` |
| `bsctl run` | Serveur de développement (`go run ./cmd/bookstorage`) |
| `bsctl clean` | Supprimer les binaires générés |

Exécutez `bsctl help` pour la liste complète des sous-commandes.

Si le premier argument n’est pas une sous-commande reconnue, `bsctl` affiche l’aide complète (code de sortie `1`).

### Chaîne de version dans les builds

Les champs `APP_VERSION` du [`Makefile`](../../Makefile) et de [`scripts/bsctl`](../../scripts/bsctl) doivent rester alignés pour les builds de release (`-X main.Version=...`). Mainteneurs : tag annoté SemVer `vX.Y.Z`, push, publication de la release GitHub, et version injectée alignée sur ce tag.

---

## Complétion par Tab (bash)

Depuis un **clone de développement** :

```bash
source scripts/bsctl.completion.bash
```

Après `sudo bsctl install` ou `./deploy/install.sh`, la complétion est copiée dans `/etc/bash_completion.d/bsctl` si ce répertoire existe, et dans `/usr/share/bash-completion/completions/bsctl` si l’arborescence du paquet **bash-completion** est présente (souvent sous Debian/Ubuntu). Ouvrez une nouvelle session de connexion, ou exécutez `source /etc/bash_completion.d/bsctl`, ou `hash -r`. Puis `bsctl` + Tab ; après `bsctl update`, Tab peut proposer `main`, des tags et des branches si Git voit le dépôt BookStorage (répertoire courant ou `/opt/bookstorage`).

---

## Structure du projet et architecture

```text
BookStorage/
├── cmd/bookstorage/     # Point d’entrée (flags, démarrage HTTP)
│   └── main.go
├── internal/
│   ├── server/          # Handlers HTTP, routes HTML/API, Web Push, import/export
│   ├── config/          # Chargement env, config site / légal JSON
│   ├── database/        # Accès SQLite, migrations de schéma
│   ├── catalog/         # Intégrations catalogues externes (AniList, MangaDex)
│   └── i18n/            # Chaînes FR / EN
├── scripts/
│   ├── bsctl
│   └── bsctl.completion.bash
├── Makefile
├── .env.example
├── config/site.json.example
├── deploy/
├── templates/           # Templates (.gohtml)
└── static/              # CSS, avatars, images, PWA
```

---

## Contribuer

1. Fork et branche à partir de `main` (ou PR avec un nom de branche explicite).
2. Lancer **tests**, **gofmt** et **golangci-lint** en local ; la CI doit rester verte.
3. Ne pas commiter `.env` ni de secrets ; s’appuyer sur `.env.example` et `config/site.json.example`.
4. Pour un changement visible côté utilisateur, mettre à jour la doc sous `docs/` (et `docs/fr/` si vous maintenez le français).

---

[Index documentation (EN)](../README.md) · [Hébergement](hebergement.md) · [Development (EN)](../development.md)
