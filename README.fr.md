# 📚 BookStorage (FR)

**BookStorage** est une application web de suivi de lectures personnelles. Suivez vos romans, mangas, webtoons, light novels et plus encore.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## 📑 Sommaire

- [Fonctionnalités](#-fonctionnalités)
- [Démarrage rapide](#-démarrage-rapide)
- [Installation en production (Linux)](#-installation-en-production-linux)
- [Intégration & Déploiement continus](#-intégration--déploiement-continus)
- [CLI bsctl](#-cli-bsctl)
- [Configuration](#-configuration)
- [Raccourcis clavier](#-raccourcis-clavier)
- [Export / Import](#-export--import)
- [Structure du projet](#-structure-du-projet)
- [Dépannage](#-dépannage)
- [Licence](#-licence)

---

## ✨ Fonctionnalités

- 📖 **Multi-formats** : Romans, mangas, manhwas, webtoons, light novels...
- ⭐ **Notes & avis** : Notez vos œuvres de 1 à 5 étoiles avec des notes personnelles
- 📊 **Statistiques** : Visualisez vos habitudes de lecture
- 👥 **Communauté** : Explorez les bibliothèques publiques des autres lecteurs
- 🌓 **Mode sombre** : Interface claire ou sombre selon vos préférences
- 🔐 **Vie privée** : Profil public ou privé, vous choisissez
- 🌍 **Multilingue** : Interface française et anglaise
- 📱 **PWA** : Installable comme application mobile sur iOS/Android
- 📦 **Export/Import** : Sauvegardez et restaurez votre bibliothèque via CSV
- ⌨️ **Raccourcis clavier** : Naviguez rapidement (N, /, S, P, ?)

---

## 🚀 Démarrage rapide

### Prérequis

- **Go 1.22+**
- **GCC** (pour la compilation de SQLite avec CGO)

### Lancer en développement

```bash
# Cloner le projet
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage

# Lancer le serveur
go run .
```

Le serveur démarre sur **http://127.0.0.1:5000**

---

## 📦 Installation en production (Linux)

### Installation automatique

```bash
# Cloner et installer (en root)
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

Le script installe automatiquement :

- L’application compilée
- La CLI `bsctl` pour gérer le service
- Le service systemd
- La configuration du pare-feu

### Démarrer le service

```bash
bsctl start
```

---

## ✅ Intégration & Déploiement continus

### CI (GitHub Actions)

À chaque **push** et pour chaque **pull request** vers `main`, le workflow `.github/workflows/ci.yml` exécute plusieurs jobs :

- `lint` : vérification du formatage (`gofmt`) et lint avancé avec `golangci-lint`
- `tests` : tests unitaires avec couverture (`go test ./... -coverprofile=coverage.out`, uploadée comme artefact)
- `race-tests` : tests avec détecteur de conditions de course (`go test -race ./...`)
- `smoke-http` : démarrage de l’application puis vérification HTTP de quelques routes clés (par ex. `/`, `/login`, `/register`)

Tous les jobs doivent passer pour que la PR soit **verte** et mergeable en toute sécurité.

### Workflow de déploiement

Le workflow `.github/workflows/deploy.yml` fournit une base de déploiement :

- Déclenchement manuel via **“Run workflow”** dans l’onglet GitHub Actions (`workflow_dispatch`)
- Build d’un binaire **Linux amd64** avec CGO activé
- Création d’une archive contenant :
  - le binaire `bookstorage`
  - la CLI `bsctl`
  - le fichier de service `deploy/bookstorage.service`
- Upload d’un artefact `bookstorage-linux-amd64` (`.tar.gz`)

Sur votre serveur, vous pouvez :

1. Télécharger l’artefact
2. Extraire l’archive
3. Copier `bookstorage`, `bsctl` et `bookstorage.service` aux bons emplacements
4. Utiliser `bsctl install` / `bsctl update` (par défaut : **release** interactive ; `bsctl update main` pour le dernier `origin/main` ; branche optionnelle : `bsctl update ma-branche`) pour gérer le service

---

## 🛠️ CLI bsctl

`bsctl` (BookStorage Control) est la CLI pour gérer BookStorage.

```bash
bsctl help     # Afficher l'aide
```

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
| `bsctl update`      | Release interactive : choix parmi les dernières tags **majeures** `vX.0.0`, ou `BSCTL_UPDATE_TAG=vX.Y.Z` sans menu + build + restart |
| `bsctl update main` | Mettre à jour depuis `origin/main` (fast-forward) + build + restart |
| `bsctl update <branche>` | Avancé : depuis `origin/<branche>` (fast-forward) + build + restart |
| `bsctl fix-perms`   | Corriger les permissions des fichiers     |

**Mise à jour sans menu :** définir `BSCTL_UPDATE_TAG=v3.1.1` puis `sudo -E bsctl update`. Le dépôt local est aligné sur la release ou sur `origin/<branche>` (les modifs locales sur fichiers suivis sont écrasées).

### Commande inconnue

Si le premier argument n’est pas une sous-commande reconnue, `bsctl` affiche l’aide complète (code de sortie `1`).

### Complétion par Tab (bash)

La complétion **bash** (shell interactif) est installée dans `/etc/bash_completion.d/bsctl` après `sudo bsctl install` ou `./deploy/install.sh`. Ouvrez un nouveau terminal, ou exécutez :

```bash
source /etc/bash_completion.d/bsctl
```

Depuis un clone en développement :

```bash
source scripts/bsctl.completion.bash
```

Ensuite, tapez `bsctl` puis Tab pour compléter les sous-commandes. Après `bsctl update`, Tab peut proposer **`main`**, des **tags** récents et des **branches** si le répertoire courant est un dépôt du projet.

---

## ⚙️ Configuration

### Variables d’environnement

Copiez le fichier d’exemple puis éditez-le (ne commitez jamais le `.env` réel) :

```bash
cp .env.example .env
```

Sur un serveur, utilisez le même principe (ex. `/opt/bookstorage/.env`). Avec **systemd**, ajoutez `EnvironmentFile=/opt/bookstorage/.env` dans l’unité pour injecter les variables dans le processus.

Exemple de contenu `.env` :

```env
# Serveur
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000

# Base de données
BOOKSTORAGE_DATABASE=/opt/bookstorage/database.db

# Sécurité (clé longue et aléatoire en production)
BOOKSTORAGE_SECRET_KEY=your-very-long-secret-key

# Super administrateur
BOOKSTORAGE_SUPERADMIN_USERNAME=admin
BOOKSTORAGE_SUPERADMIN_PASSWORD=SecurePassword123!
```

| Variable                  | Description                      | Valeur par défaut          |
|---------------------------|----------------------------------|----------------------------|
| `BOOKSTORAGE_HOST`       | Adresse d’écoute                 | `127.0.0.1`                |
| `BOOKSTORAGE_PORT`       | Port                             | `5000`                     |
| `BOOKSTORAGE_DATABASE`   | Chemin de la base SQLite         | `database.db`              |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète de session (min. 32 octets si `BOOKSTORAGE_ENV=production`) | `dev-secret-change-me`     |
| `BOOKSTORAGE_ENV` | `development` ou `production` (la production interdit la clé par défaut) | `development` |
| `BOOKSTORAGE_ENABLE_HSTS` | `true` ou `1` pour l’en-tête HSTS (uniquement derrière HTTPS) | (désactivé) |

### Mentions légales

Pour personnaliser la page légale (`/legal`), copiez la configuration d’exemple :

```bash
cp config/site.json.example config/site.json
```

Puis éditez `config/site.json` avec vos informations :

```json
{
  "site_name": "BookStorage",
  "site_url": "https://your-domain.com",
  "legal": {
    "owner_name": "Votre nom",
    "owner_email": "contact@example.com",
    "owner_address": "Votre adresse",
    "hosting_provider": "Nom de l'hébergeur",
    "hosting_address": "Adresse de l'hébergeur",
    "data_retention": "Politique de conservation des données...",
    "data_usage": "Comment les données sont utilisées...",
    "custom_sections": []
  }
}
```

---

## ⌨️ Raccourcis clavier

Sur le tableau de bord, vous pouvez utiliser ces raccourcis :

| Touche | Action                  |
|--------|-------------------------|
| `N`    | Ajouter une nouvelle œuvre |
| `/`    | Focaliser la barre de recherche |
| `S`    | Aller aux statistiques  |
| `P`    | Aller au profil         |
| `?`    | Afficher l’aide         |
| `Esc`  | Fermer / retirer le focus |

---

## 📦 Export / Import

### Export

Allez dans **Profil** → Téléchargez votre bibliothèque au format CSV, ou **JSON** pour une sauvegarde versionnée (`export_version`) réimportable.

### Import

Allez dans **Profil** → Importez un export CSV ou JSON. Le CSV utilise le point-virgule ; les colonnes optionnelles `CatalogID`, `IsAdult`, `ImagePath` peuvent suivre `Notes`. Choisissez si les titres déjà présents sont **ignorés** ou **mis à jour**.

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Valeurs de statut** : En cours, Terminé, En pause, Abandonné, À lire  
**Valeurs de type** : Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

---

## 🗂 Structure du projet

```text
BookStorage/
├── cmd/bookstorage/     # Point d'entrée
│   └── main.go
├── scripts/
│   ├── bsctl                    # CLI de gestion
│   └── bsctl.completion.bash    # Complétion bash (source ou install)
├── Makefile             # Commandes Make
│
├── .env.example         # Modèle d’environnement (copier vers .env)
├── internal/            # Packages internes
│   ├── config/          # Gestion de la configuration
│   │   ├── config.go    # Paramètres de l’application
│   │   └── site.go      # Config site / mentions légales
│   ├── database/        # Gestion de la base de données
│   │   └── database.go  # Schéma SQLite & opérations
│   └── i18n/            # Internationalisation
│       └── i18n.go      # Traductions (FR/EN)
│
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

## 🐛 Dépannage

### Erreur "readonly database"

```bash
bsctl fix-perms
bsctl restart
```

### Port déjà utilisé

```bash
# Voir quel processus utilise le port
sudo lsof -i :5000

# Changer le port dans .env
BOOKSTORAGE_PORT=5001
```

### Voir les logs détaillés

```bash
bsctl logs
```

---

## 📝 Licence

Licence MIT

---

<p align="center">
  Fait avec ❤️ pour les lecteurs
</p>

