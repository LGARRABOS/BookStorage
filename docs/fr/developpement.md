# Développement

Développement local, CI/CD et organisation du dépôt. Pour installer BookStorage sur un serveur, voir [Hébergement](hebergement.md).

---

## Sommaire

- [Démarrage rapide](#démarrage-rapide)
- [Intégration & déploiement continus](#intégration--déploiement-continus)
- [CLI bsctl](#cli-bsctl)
- [Structure du projet](#structure-du-projet)

---

## Démarrage rapide

### Prérequis

- **Go 1.22+**
- **GCC** (pour la compilation de SQLite avec CGO)

### Lancer en développement

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Le serveur écoute par défaut sur **http://127.0.0.1:5000**.

Vous pouvez aussi utiliser `make run`, `bsctl run`, ou lancer le package `cmd/bookstorage` depuis l’IDE.

---

## Intégration & déploiement continus

### CI (GitHub Actions)

À chaque **push** et pour chaque **pull request** vers `main`, le workflow `.github/workflows/ci.yml` exécute notamment :

- `lint` : formatage (`gofmt`) et `golangci-lint`
- `tests` : tests unitaires avec couverture (`go test ./... -coverprofile=coverage.out`, artefact)
- `race-tests` : `go test -race ./...`
- `smoke-http` : démarrage de l’application puis vérification HTTP de quelques routes (par ex. `/`, `/login`, `/register`)

Tous les jobs doivent passer pour merger en toute sécurité.

### Workflow de déploiement {#workflow-de-déploiement}

Le workflow `.github/workflows/deploy.yml` sert de base pour livrer des binaires :

- Déclenchement manuel via **« Run workflow »** dans l’onglet Actions (`workflow_dispatch`)
- Build **Linux amd64** avec CGO
- Empaquetage : `bookstorage`, `bsctl`, `deploy/bookstorage.service`
- Upload d’un artefact `bookstorage-linux-amd64` (`.tar.gz`)

Sur un serveur : télécharger l’artefact, extraire, copier les fichiers, puis `bsctl install` / `bsctl update` comme décrit dans [Hébergement](hebergement.md).

---

## CLI bsctl {#cli-bsctl}

`bsctl` (BookStorage Control) gère la compilation, le serveur de dev et les installations production. Exécutez `bsctl help` pour l’aide complète en anglais.

### Commandes service

| Commande        | Description                    |
|-----------------|--------------------------------|
| `bsctl start`   | Démarrer le service           |
| `bsctl stop`    | Arrêter le service            |
| `bsctl restart` | Redémarrer le service         |
| `bsctl status`  | Afficher le statut du service |
| `bsctl logs`    | Voir les logs en temps réel   |

### Commandes développement

| Commande            | Description                      |
|---------------------|----------------------------------|
| `bsctl build`       | Compiler l’application          |
| `bsctl build-prod`  | Compiler pour la production     |
| `bsctl run`         | Lancer le serveur de dev        |
| `bsctl clean`       | Supprimer les binaires générés  |

### Commandes production / maintenance

| Commande            | Description                               |
|---------------------|-------------------------------------------|
| `bsctl install`     | Installer le service systemd              |
| `bsctl uninstall`   | Désinstaller le service                   |
| `bsctl update`      | Release interactive : **1** / **2** = deux dernières tags **majeures** `vX.0.0`, **3** = saisir un tag ; ou `BSCTL_UPDATE_TAG=vX.Y.Z` sans menu + build + restart |
| `bsctl update main` | Mettre à jour depuis `origin/main` (fast-forward) + build + restart |
| `bsctl update <branche>` | Avancé : depuis `origin/<branche>` (fast-forward) + build + restart |
| `bsctl fix-perms`   | Corriger les permissions des fichiers     |

**Mise à jour sans menu :** définir `BSCTL_UPDATE_TAG=v4.0.1` puis `sudo -E bsctl update`. Le dépôt local est aligné sur la release ou sur `origin/<branche>` (les modifs locales sur fichiers suivis sont écrasées).

### Commande inconnue

Si le premier argument n’est pas une sous-commande reconnue, `bsctl` affiche l’aide complète (code de sortie `1`).

### Complétion par Tab (bash)

La complétion **bash** est installée dans `/etc/bash_completion.d/bsctl` après `sudo bsctl install` ou `./deploy/install.sh`. Ouvrez un nouveau terminal, ou :

```bash
source /etc/bash_completion.d/bsctl
```

Depuis un clone en développement :

```bash
source scripts/bsctl.completion.bash
```

Ensuite, tapez `bsctl` puis Tab pour compléter les sous-commandes. Après `bsctl update`, Tab peut proposer **`main`**, des **tags** récents et des **branches** si le répertoire courant est un dépôt du projet.

---

## Structure du projet

```text
BookStorage/
├── cmd/bookstorage/     # Point d'entrée
│   └── main.go
├── internal/            # Packages internes
│   ├── server/         # Handlers HTTP, API, Push
│   ├── config/         # Configuration
│   ├── database/       # SQLite
│   ├── catalog/        # AniList, MangaDex
│   └── i18n/           # Internationalisation
│
├── scripts/
│   ├── bsctl                    # CLI de gestion
│   └── bsctl.completion.bash    # Complétion bash (source ou install)
├── Makefile             # Commandes Make
│
├── .env.example         # Modèle d’environnement (copier vers .env)
├── config/
│   └── site.json.example  # Modèle de config légale
├── go.mod / go.sum      # Dépendances Go
│
├── deploy/
│   ├── install.sh       # Script d’installation
│   └── bookstorage.service
│
├── templates/           # Templates HTML (.gohtml)
└── static/
    ├── css/             # Feuilles de style
    ├── avatars/         # Avatars utilisateurs
    ├── images/          # Images de l’application
    ├── icons/           # Favicon & icônes
    └── pwa/             # Fichiers PWA
        ├── manifest.json
        └── sw.js
```

---

[Index documentation (EN)](../README.md) · [Hébergement](hebergement.md) · [Development (EN)](../development.md)
