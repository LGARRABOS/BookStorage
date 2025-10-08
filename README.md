# BookStorage

## Overview
BookStorage is a Flask web application that helps readers track what they are currently reading, organise their backlog, and optionally share favourite works with the community. Accounts are validated by administrators, super-administrators keep the platform healthy, and every member controls whether their profile stays private or appears in the public directory.

## Highlights
- Full profile management: update the display name, e-mail, biography, avatar, and password from a dedicated profile page.
- Privacy controls: toggle whether the profile and personal library are visible in the community directory.
- Reading dashboard: add works with covers, status, chapter counters, reading type (novel, manga, comics, manhwa, etc.), and quick update actions.
- Community sharing: browse public profiles, search for users, and import works from their libraries into your own.
- Administration console: approve registrations, promote accounts, and remove users with safeguards for administrator and super-administrator roles.

## Requirements
- Python 3.9 or newer.
- SQLite (bundled with Python on most systems).
- The dependencies listed in `requirements.txt` (`Flask`, `Werkzeug`, `waitress`, `gunicorn`, `python-dotenv`, `pytest`).
- Optional for Linux deployments: `systemd` if you plan to install the service unit through the helper script.

## Installation
### Development setup
1. **Clone the repository and enter the project directory**
   ```bash
   git clone https://github.com/LGARRABOS/BookStorage.git
   cd BookStorage
   ```
2. **Create and activate a virtual environment**
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # On Windows: .venv\Scripts\activate
   ```
3. **Copy the environment template**
   ```bash
   cp .env.example .env
   ```
   Edit `.env` to set `BOOKSTORAGE_SECRET_KEY`, adjust the storage paths, and switch `BOOKSTORAGE_ENV` to `development` or `production` as needed.
4. **Install the dependencies**
   ```bash
   pip install -r requirements.txt
   ```
5. **Initialise the database**
   ```bash
   python init_db.py
   ```
6. **Run the application**
   ```bash
   flask --app wsgi --debug run       # development server
   # or
   BOOKSTORAGE_ENV=production python app.py  # serves through Waitress
   ```

### Automated deployment script (Ubuntu / Rocky Linux)
On Linux servers you can let the project bootstrap itself through the provided helper:

```bash
bash setup_and_run.sh
```

The script will:
- create the `.venv` virtual environment if it does not exist yet;
- upgrade `pip` when possible and install `requirements.txt`;
- copy `.env.example` to `.env` on first run and remind you to customise secrets;
- run `init_db.py` to create the SQLite schema and seed the default super-administrator;
- start BookStorage with Waitress in production mode.

Re-run the script whenever you pull updates; it reuses the existing environment and restarts the service.

#### Optional: install a `systemd` service
If the host uses `systemd`, the script can register and enable a unit that keeps BookStorage running:

```bash
sudo BOOKSTORAGE_SERVICE_USER=bookstorage bash setup_and_run.sh --install-service
```

Environment variables such as `BOOKSTORAGE_SERVICE_NAME`, `BOOKSTORAGE_SERVICE_USER`, `BOOKSTORAGE_SERVICE_GROUP`, and `BOOKSTORAGE_SERVICE_ENV_FILE` allow you to customise the generated unit. Ensure the target user owns the application folder and the media directories before enabling the service.

### Container image
BookStorage also ships with a Dockerfile so you can run the application in an isolated container.

1. **Build the image**
   ```bash
   docker build -t bookstorage:latest .
   ```
2. **Run the container**
   ```bash
   docker run -d \
     --name bookstorage \
     -p 5000:5000 \
     -e BOOKSTORAGE_SECRET_KEY="change-me" \
     -e BOOKSTORAGE_SUPERADMIN_PASSWORD="ChooseAStrongPassword" \
     -v $(pwd)/bookstorage-data:/data \
     bookstorage:latest
   ```

The container starts in production mode and serves the app with Waitress on port 5000. The `/data` volume stores the SQLite database as well as uploaded covers and avatars, ensuring uploads persist across image upgrades. Adjust the host path on `-v` to match your environment or switch to a named Docker volume, for example `-v bookstorage_data:/data`.

#### Run the published image without cloning the repository

Once the image is stored on a registry (for example Docker Hub), it already contains every file required by the application. That means you can deploy it on a fresh host without checking out the Git repository—just pull the image and mount a volume dedicated to persistent data:

```bash
docker pull moncompte/bookstorage:latest
docker volume create bookstorage_data
docker run -d \
  --name bookstorage \
  -p 5000:5000 \
  -e BOOKSTORAGE_SECRET_KEY="change-me" \
  -e BOOKSTORAGE_SUPERADMIN_PASSWORD="ChooseAStrongPassword" \
  -v bookstorage_data:/data \
  moncompte/bookstorage:latest
```

Replace `moncompte/bookstorage:latest` with the name of your Docker Hub repository. The bind mount to the Git checkout is no longer necessary because the container image embeds the source code, templates, and static assets. Providing `/data` as a persistent volume keeps the database and uploaded files safe across upgrades.

The application code now lives inside the container under `/srv/bookstorage`. Avoid mounting an empty host directory over that path (or `/app`), otherwise the bundled sources would be hidden and the container would fail to start.

Set additional configuration through environment variables (`BOOKSTORAGE_PORT`, `BOOKSTORAGE_UPLOAD_URL_PATH`, …). `docker-entrypoint.sh` initialises the database on each boot, creating the default super-administrator when needed.

## Configuration
All configuration is provided through environment variables. When `python-dotenv` is installed, values defined in `.env` are automatically loaded.

| Variable | Purpose | Default |
| --- | --- | --- |
| `BOOKSTORAGE_ENV` | Execution profile (`development` or `production`). | `development` |
| `BOOKSTORAGE_SECRET_KEY` | Flask secret key used to sign sessions. **Mandatory in production.** | `dev-secret-change-me` |
| `BOOKSTORAGE_DATA_DIR` | Base directory that contains the SQLite database file. | project directory |
| `BOOKSTORAGE_DATABASE` | Filename or absolute path for the SQLite database. | `database.db` |
| `BOOKSTORAGE_UPLOAD_DIR` | Directory where book covers are stored. | `static/images` |
| `BOOKSTORAGE_UPLOAD_URL_PATH` | Relative URL segment for cover images. | `images` |
| `BOOKSTORAGE_AVATAR_DIR` | Directory where profile avatars are stored. | `static/avatars` |
| `BOOKSTORAGE_AVATAR_URL_PATH` | Relative URL segment for avatars. | `avatars` |
| `BOOKSTORAGE_SUPERADMIN_USERNAME` | Username created for the default super-administrator. | `superadmin` |
| `BOOKSTORAGE_SUPERADMIN_PASSWORD` | Password assigned to the default super-administrator. Change it immediately in production. | `SuperAdmin!2023` |
| `BOOKSTORAGE_HOST` | Network interface bound by the application when run via `app.py`. | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | HTTP port exposed by the application when run via `app.py`. | `5000` |

When `BOOKSTORAGE_ENV=production`, the application refuses to start if `BOOKSTORAGE_SECRET_KEY` is missing.

## Testing
Run the automated test suite with:
```bash
pytest
```

## Repository layout
- `app.py` – Flask application with routes, background helpers, and file management.
- `config.py` – centralised configuration loader shared by the app and scripts.
- `wsgi.py` – production entry point for WSGI servers (Gunicorn, uWSGI, Waitress, etc.).
- `templates/` – Jinja2 templates for the UI.
- `static/` – stylesheets and uploaded media.
- `init_db.py` – CLI helper to bootstrap the SQLite database.
- `tests/` – Pytest suite covering accounts, profile management, works, and the community directory.

---

# BookStorage (Français)

## Aperçu
BookStorage est une application web Flask qui aide les lecteurs à suivre leurs lectures en cours, organiser leur pile à lire et, s’ils le souhaitent, partager des œuvres avec la communauté. Les comptes sont validés par des administrateurs, les super-administrateurs veillent au bon fonctionnement de la plateforme et chaque membre choisit si son profil reste privé ou apparaît dans l’annuaire public.

## Points clés
- Gestion complète du profil : nom affiché, e-mail, biographie, avatar et mot de passe se mettent à jour depuis une page dédiée.
- Contrôle de confidentialité : activer ou désactiver la visibilité du profil et de la bibliothèque dans l’annuaire communautaire.
- Tableau de bord de lecture : ajouter des œuvres avec couverture, statut, compteur de chapitres, type de lecture (roman, manga, BD, manhwa, etc.) et actions rapides.
- Partage communautaire : parcourir les profils publics, rechercher un utilisateur et importer des œuvres depuis sa bibliothèque.
- Console d’administration : approuver les inscriptions, promouvoir des comptes et supprimer des utilisateurs avec des garde-fous pour les rôles administrateur et super-administrateur.

## Prérequis
- Python 3.9 ou supérieur.
- SQLite (installé par défaut avec Python sur la plupart des systèmes).
- Dépendances listées dans `requirements.txt` (`Flask`, `Werkzeug`, `waitress`, `gunicorn`, `python-dotenv`, `pytest`).
- Optionnel pour Linux : `systemd` si vous souhaitez installer le service via le script d’aide.

## Installation
### Mise en place pour le développement
1. **Cloner le dépôt et entrer dans le dossier**
   ```bash
   git clone https://github.com/LGARRABOS/BookStorage.git
   cd BookStorage
   ```
2. **Créer et activer un environnement virtuel** (fortement recommandé) :
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # Sous Windows : .venv\Scripts\activate
   ```
3. **Copier le modèle d’environnement**
   ```bash
   cp .env.example .env
   ```
   Modifiez `.env` pour définir `BOOKSTORAGE_SECRET_KEY`, ajuster les chemins de stockage et choisir `BOOKSTORAGE_ENV=development` ou `production`.
4. **Installer les dépendances**
   ```bash
   pip install -r requirements.txt
   ```
5. **Initialiser la base de données**
   ```bash
   python init_db.py
   ```
6. **Lancer l’application**
   ```bash
   flask --app wsgi --debug run       # serveur de développement
   # ou
   BOOKSTORAGE_ENV=production python app.py  # servi via Waitress
   ```

### Script de déploiement automatisé (Ubuntu / Rocky Linux)
Sur un serveur Linux, le projet peut se préparer tout seul grâce au script fourni :

```bash
bash setup_and_run.sh
```

Le script :
- crée l’environnement virtuel `.venv` si nécessaire ;
- met `pip` à jour lorsque possible et installe `requirements.txt` ;
- copie `.env.example` vers `.env` lors du premier lancement et rappelle de personnaliser les secrets ;
- exécute `init_db.py` pour créer le schéma SQLite et créer le super-administrateur par défaut ;
- démarre BookStorage avec Waitress en mode production.

Relancez le script après chaque mise à jour : il réutilise l’environnement existant et redémarre le service.

#### Option : installer un service `systemd`
Si l’hôte utilise `systemd`, le script peut enregistrer et activer une unité pour garder BookStorage en fonctionnement :

```bash
sudo BOOKSTORAGE_SERVICE_USER=bookstorage bash setup_and_run.sh --install-service
```

Les variables d’environnement `BOOKSTORAGE_SERVICE_NAME`, `BOOKSTORAGE_SERVICE_USER`, `BOOKSTORAGE_SERVICE_GROUP` et `BOOKSTORAGE_SERVICE_ENV_FILE` permettent d’ajuster l’unité générée. Assurez-vous que l’utilisateur ciblé possède le dossier de l’application et les répertoires média avant d’activer le service.

### Image conteneurisée
Un Dockerfile est fourni pour exécuter BookStorage dans un conteneur isolé.

1. **Construire l’image**
   ```bash
   docker build -t bookstorage:latest .
   ```
2. **Lancer le conteneur**
   ```bash
   docker run -d \
     --name bookstorage \
     -p 5000:5000 \
     -e BOOKSTORAGE_SECRET_KEY="modifiez-moi" \
     -e BOOKSTORAGE_SUPERADMIN_PASSWORD="ChoisissezUnMotDePasseFort" \
     -v $(pwd)/bookstorage-data:/data \
     bookstorage:latest
   ```

Le conteneur démarre en mode production et sert l’application via Waitress sur le port 5000. Le volume `/data` conserve la base SQLite ainsi que les couvertures et avatars importés, ce qui garantit la persistance des fichiers lors d’une mise à jour de l’image. Adaptez le chemin hôte dans `-v` selon votre infrastructure ou utilisez un volume nommé comme `-v bookstorage_data:/data`.

#### Utiliser l’image publiée sans cloner le dépôt

Si vous publiez l’image sur un registre (Docker Hub, par exemple), elle contient déjà tous les fichiers nécessaires au fonctionnement de BookStorage. Vous pouvez donc la déployer sur une machine vierge sans cloner le dépôt Git : récupérez l’image, créez un volume persistant et lancez le conteneur.

```bash
docker pull moncompte/bookstorage:latest
docker volume create bookstorage_data
docker run -d \
  --name bookstorage \
  -p 5000:5000 \
  -e BOOKSTORAGE_SECRET_KEY="modifiez-moi" \
  -e BOOKSTORAGE_SUPERADMIN_PASSWORD="ChoisissezUnMotDePasseFort" \
  -v bookstorage_data:/data \
  moncompte/bookstorage:latest
```

Remplacez `moncompte/bookstorage:latest` par le nom de votre dépôt Docker Hub. Le montage du code source n’est pas requis : l’image embarque le projet ainsi que les gabarits. Il suffit de fournir un volume `/data` pour préserver la base SQLite et les fichiers téléversés lors des mises à jour.

Le code de l’application est désormais stocké dans le conteneur sous `/srv/bookstorage`. Évitez de monter un répertoire hôte vide à cet emplacement (ou sur `/app`), sinon les sources incluses seront masquées et le conteneur ne pourra pas démarrer.

Toutes les autres variables de configuration (`BOOKSTORAGE_PORT`, `BOOKSTORAGE_UPLOAD_URL_PATH`, …) peuvent être injectées via `docker run`. Le script `docker-entrypoint.sh` initialise la base de données à chaque démarrage et crée le super-administrateur par défaut si nécessaire.

## Configuration
Toute la configuration passe par des variables d’environnement. Avec `python-dotenv`, les valeurs définies dans `.env` sont chargées automatiquement.

| Variable | Rôle | Valeur par défaut |
| --- | --- | --- |
| `BOOKSTORAGE_ENV` | Profil d’exécution (`development` ou `production`). | `development` |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète Flask pour signer les sessions. **Obligatoire en production.** | `dev-secret-change-me` |
| `BOOKSTORAGE_DATA_DIR` | Dossier de base qui contient le fichier SQLite. | dossier du projet |
| `BOOKSTORAGE_DATABASE` | Nom ou chemin absolu de la base SQLite. | `database.db` |
| `BOOKSTORAGE_UPLOAD_DIR` | Répertoire de stockage des couvertures. | `static/images` |
| `BOOKSTORAGE_UPLOAD_URL_PATH` | Segment d’URL relatif pour les couvertures. | `images` |
| `BOOKSTORAGE_AVATAR_DIR` | Répertoire de stockage des avatars. | `static/avatars` |
| `BOOKSTORAGE_AVATAR_URL_PATH` | Segment d’URL relatif pour les avatars. | `avatars` |
| `BOOKSTORAGE_SUPERADMIN_USERNAME` | Identifiant créé pour le super-administrateur par défaut. | `superadmin` |
| `BOOKSTORAGE_SUPERADMIN_PASSWORD` | Mot de passe associé au super-administrateur par défaut. À changer immédiatement en production. | `SuperAdmin!2023` |
| `BOOKSTORAGE_HOST` | Interface réseau écoutée lorsque l’application est lancée via `app.py`. | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | Port HTTP exposé lorsque l’application est lancée via `app.py`. | `5000` |

Lorsque `BOOKSTORAGE_ENV=production`, l’application refuse de démarrer si `BOOKSTORAGE_SECRET_KEY` n’est pas défini.

## Tests
Lancer la suite automatisée avec :
```bash
pytest
```

## Structure du dépôt
- `app.py` – application Flask avec les routes, aides et gestion des fichiers.
- `config.py` – chargeur de configuration centralisé partagé entre l’application et les scripts.
- `wsgi.py` – point d’entrée recommandé pour les serveurs WSGI (Gunicorn, uWSGI, Waitress, etc.).
- `templates/` – gabarits Jinja2 pour l’interface.
- `static/` – feuilles de style et médias téléversés.
- `init_db.py` – outil CLI pour initialiser la base SQLite.
- `tests/` – suite Pytest couvrant les comptes, la gestion du profil, les œuvres et l’annuaire communautaire.
