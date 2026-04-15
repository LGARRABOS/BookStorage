# Héberger BookStorage soi-même

Faire tourner BookStorage sur votre propre machine Linux. Pour le développement local et la CI, voir [Développement](developpement.md).

---

## Sommaire

- [Installation en production (Linux)](#installation-en-production-linux)
- [bsctl — service et mises à jour](#bsctl--service-et-mises-à-jour)
- [Configuration](#configuration)
- [Metriques Prometheus (optionnel)](#metriques-prometheus-optionnel)
- [Utilisation de l’application](#utilisation-de-lapplication)
- [Dépannage](#dépannage)

---

## Installation en production (Linux)

### Installation automatique

```bash
# Cloner et installer (en root)
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

Le script installe notamment :

- L’application compilée
- La CLI `bsctl` pour gérer le service
- Le service systemd (charge en option `EnvironmentFile=-/opt/bookstorage/.env`)
- La configuration du pare-feu

**Prometheus (optionnel) :** définissez `INSTALL_WITH_PROMETHEUS=1` lors de l’installation pour installer le paquet `prometheus` de la distribution, générer `BOOKSTORAGE_METRICS_TOKEN` s’il manque, et activer l’unité systemd `bookstorage-prometheus` (UI sur `http://127.0.0.1:9091`, scrape avec fichier bearer). Après ajout du jeton dans `.env`, exécutez `systemctl restart bookstorage`. `bsctl update` **ne relance pas** cette étape ; sur le serveur : `INSTALL_APP_DIR=/opt/bookstorage bash /opt/bookstorage/deploy/setup-bookstorage-prometheus.sh` (préférez `bash` pour éviter l’erreur « Permission non accordée » si le script n’est pas exécutable).

### Démarrer le service

```bash
bsctl start
```

---

## bsctl — service et mises à jour

`bsctl` (BookStorage Control) sert à piloter le service et appliquer les mises à jour. Pour les commandes **développement** (`build`, `run`, `clean`, …), voir [Développement — bsctl côté développement](developpement.md#bsctl-dev).

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

### Commandes production / maintenance

| Commande            | Description                               |
|---------------------|-------------------------------------------|
| `bsctl install`     | Installer le service systemd              |
| `bsctl uninstall`   | Désinstaller le service                   |
| `bsctl update`      | Release interactive : **1** / **2** = deux dernières tags **majeures** `vX.0.0`, **3** = saisir un tag ; ou `BSCTL_UPDATE_TAG=vX.Y.Z` sans menu + build + restart |
| `bsctl update main` | Mettre à jour depuis `origin/main` (fast-forward) + build + restart |
| `bsctl update <branche>` | Avancé : depuis `origin/<branche>` (fast-forward) + build + restart |
| `bsctl fix-perms`   | Corriger les permissions des fichiers     |
| `bsctl backup`      | Copie du fichier SQLite indiqué par `BOOKSTORAGE_DATABASE` dans le `.env` (`sqlite3 .backup` si disponible, sinon `cp`), rétention via `BOOKSTORAGE_BACKUP_RETENTION_DAYS` (défaut 14), répertoire `BOOKSTORAGE_BACKUP_DIR` (défaut `/var/lib/bookstorage/backups`) |

**Sauvegardes planifiées :** définir `INSTALL_WITH_BACKUP_TIMER=1` lors de l’exécution de [`deploy/install.sh`](../../deploy/install.sh) pour installer et activer `bookstorage-backup.timer` (snapshot quotidien ; adaptez l’unité timer si besoin). Journaux : `journalctl -u bookstorage-backup.service`.

**Mise à jour sans menu :** définir `BSCTL_UPDATE_TAG=v5.6.1` puis `sudo -E bsctl update`. Le dépôt local est aligné sur la release ou sur `origin/<branche>` (les modifs locales sur fichiers suivis sont écrasées).

Si vous déployez depuis un artefact GitHub Actions plutôt qu’un clone, extrayez l’archive, copiez `bookstorage`, `bsctl` et `deploy/bookstorage.service` aux bons emplacements, puis utilisez `bsctl install` / `bsctl update` comme d’habitude. Voir [Développement — Workflow de déploiement](developpement.md#workflow-de-déploiement).

---

## Configuration

### Variables d’environnement

Copiez le fichier d’exemple puis éditez-le (ne commitez jamais le `.env` réel) :

```bash
cp .env.example .env
```

Sur un serveur, utilisez le même principe (ex. `/opt/bookstorage/.env`). Le fichier `deploy/bookstorage.service` fourni inclut `EnvironmentFile=-/opt/bookstorage/.env` (le tiret devant le chemin ignore un fichier absent).

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
| `BOOKSTORAGE_METRICS_TOKEN` | Si défini, sécurise `GET /metrics` avec `Authorization: Bearer …` ou `?token=…`. Si vide, seul le loopback peut scraper `/metrics`. | (vide) |
| `BOOKSTORAGE_PROMETHEUS_QUERY_URL` | URL de base de l’API HTTP Prometheus pour le **résumé intégré** (Admin → Monitoring). Défaut `http://127.0.0.1:9091`. **Hôtes loopback uniquement** (`127.0.0.1`, `localhost`, `::1`). | (défaut) |

### Durée de vie des sessions

Les sessions utilisent un **TTL glissant de 2 heures** et un **TTL absolu de 24 heures**. Si un utilisateur est inactif pendant plus de 2 heures, il doit se reconnecter. Quelle que soit l'activité, chaque session expire 24 heures après sa création.

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

## Metriques Prometheus (optionnel)

BookStorage expose **`GET /metrics`** au format texte Prometheus (compteurs et histogrammes préfixés `bookstorage_http_*`).

- **Sans `BOOKSTORAGE_METRICS_TOKEN` :** le scraper doit se connecter en **loopback** (`127.0.0.1` / `::1`). Adapté à un Prometheus sur la même machine.
- **Avec `BOOKSTORAGE_METRICS_TOKEN` :** envoyez `Authorization: Bearer <jeton>` ou `GET /metrics?token=<jeton>` (préférez Bearer dans `bearer_token` / `bearer_token_file` côté Prometheus).

**Installation automatique (installateur Linux) :** `INSTALL_WITH_PROMETHEUS=1 sudo -E ./deploy/install.sh` lance `deploy/setup-bookstorage-prometheus.sh` via `bash`, installe le paquet `prometheus`, écrit `/etc/bookstorage/prometheus-bs.yml` et le fichier bearer, et active **`bookstorage-prometheus`** (`127.0.0.1:9091`, TSDB sous `/var/lib/prometheus-bookstorage`). Puis :

```bash
sudo systemctl restart bookstorage   # recharge .env si le jeton vient d’être ajouté
sudo systemctl status bookstorage-prometheus
```

Le script tente ensuite le **binaire officiel** (archive GitHub) si aucun paquet n’est disponible (variable `PROMETHEUS_VERSION` pour choisir la release, ex. `2.55.2`). Nécessite `curl` ou `wget` et HTTPS sortant.

**Installation manuelle** (paquet absent, script en échec, conteneurs) :

1. Définissez le même secret dans BookStorage et Prometheus, ex. `BOOKSTORAGE_METRICS_TOKEN=<long-aléatoire>` dans `/opt/bookstorage/.env`, puis `systemctl restart bookstorage`.
2. Créez `/etc/bookstorage/bookstorage-metrics.token` contenant **uniquement** ce jeton (une ligne), `chmod 640`, `root:prometheus`.
3. Exemple de job `scrape_configs` :

```yaml
scrape_configs:
  - job_name: bookstorage
    metrics_path: /metrics
    scheme: http
    bearer_token_file: /etc/bookstorage/bookstorage-metrics.token
    static_configs:
      - targets: ['127.0.0.1:5000']   # alignez sur BOOKSTORAGE_PORT
```

4. Reliez Grafana à votre Prometheus comme d’habitude.

La page **Admin → Monitoring** affiche un **résumé intégré** (état du scrape, compteur de requêtes, débit sur 5 min) via l’API HTTP Prometheus sur le serveur (`BOOKSTORAGE_PROMETHEUS_QUERY_URL`, défaut `http://127.0.0.1:9091`), ainsi que l’URL `/metrics` et le mode jeton. Le bloc se rafraîchit automatiquement dans le navigateur.

---

## Utilisation de l’application

### Raccourcis clavier (bureau)

Sur le tableau de bord (vue web bureau uniquement — non disponibles sur la PWA mobile) :

| Touche | Action                  |
|--------|-------------------------|
| `N`    | Ajouter une nouvelle œuvre |
| `/`    | Focaliser la barre de recherche |
| `S`    | Aller aux statistiques  |
| `P`    | Aller au profil         |
| `?`    | Afficher l’aide         |
| `Esc`  | Fermer / retirer le focus |

### PWA mobile

La vue mobile offre une **expérience simplifiée** centrée sur le suivi quotidien. Fonctionnalités disponibles : tableau de bord (recherche, filtres, tri), ajout/édition d'œuvres, et boutons rapides +/- pour les chapitres. Les pages Statistiques, Profil, Outils, Utilisateurs, Admin, Export/Import et Mentions légales ne sont **accessibles que depuis la vue web bureau** et redirigent vers le tableau de bord sur mobile.

L'application mobile **se rafraîchit automatiquement** lorsqu'elle revient au premier plan (ex. après être passé sur la version web bureau), les modifications se synchronisent donc automatiquement.

### Export / import (bureau)

**Export :** Profil → télécharger la bibliothèque en CSV, ou **JSON** pour une sauvegarde versionnée (`export_version`) réimportable.

**Import :** Profil → importer un export CSV ou JSON. Le CSV utilise le point-virgule ; les colonnes optionnelles `CatalogID`, `IsAdult`, `ImagePath` peuvent suivre `Notes`. Choisissez si les titres déjà présents sont **ignorés** ou **mis à jour**.

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Valeurs de statut** : En cours, Terminé, En pause, Abandonné, À lire  
**Valeurs de type** : Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

> **Note :** L'export et l'import ne sont accessibles que depuis la vue web bureau (page Profil).

---

## Dépannage

### `BOOKSTORAGE_SECRET_KEY must be at least 32 bytes when BOOKSTORAGE_ENV=production`

L’unité systemd charge `/opt/bookstorage/.env` via `EnvironmentFile`. En **production**, `BOOKSTORAGE_SECRET_KEY` doit être une chaîne **d’au moins 32 caractères** (et différente de la valeur de développement par défaut).

**Correctif :**

```bash
openssl rand -base64 48
```

Copiez le résultat dans `/opt/bookstorage/.env` : `BOOKSTORAGE_SECRET_KEY=...` (évitez guillemets et espaces en fin de ligne). Puis :

```bash
chmod 600 /opt/bookstorage/.env
systemctl restart bookstorage
```

### `sudo: bsctl : commande introuvable`

`bsctl` est installé dans `/usr/local/bin`. Certaines configurations **sudo** restreignent le `PATH` (`secure_path` dans `/etc/sudoers`) et **excluent** `/usr/local/bin`, d’où l’échec de `sudo bsctl …` alors que `bsctl` fonctionne dans un shell root interactif.

**Correctif :** utilisez le chemin complet, par exemple :

```bash
sudo /usr/local/bin/bsctl install
sudo /usr/local/bin/bsctl update main
```

Si vous êtes **déjà connecté en root**, lancez `bsctl install` **sans** `sudo`.

### Erreur « readonly database »

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

[Index documentation (EN)](../README.md) · [Développement](developpement.md) · [Self-hosting (EN)](../self-hosting.md)
