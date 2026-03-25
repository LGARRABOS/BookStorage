# Héberger BookStorage soi-même

Faire tourner BookStorage sur votre propre machine Linux. Pour le développement local et la CI, voir [Développement](developpement.md).

---

## Sommaire

- [Installation en production (Linux)](#installation-en-production-linux)
- [bsctl — service et mises à jour](#bsctl--service-et-mises-à-jour)
- [Configuration](#configuration)
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
- Le service systemd
- La configuration du pare-feu

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

**Mise à jour sans menu :** définir `BSCTL_UPDATE_TAG=v4.3.1` puis `sudo -E bsctl update`. Le dépôt local est aligné sur la release ou sur `origin/<branche>` (les modifs locales sur fichiers suivis sont écrasées).

Si vous déployez depuis un artefact GitHub Actions plutôt qu’un clone, extrayez l’archive, copiez `bookstorage`, `bsctl` et `deploy/bookstorage.service` aux bons emplacements, puis utilisez `bsctl install` / `bsctl update` comme d’habitude. Voir [Développement — Workflow de déploiement](developpement.md#workflow-de-déploiement).

---

## Configuration

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

## Utilisation de l’application

### Raccourcis clavier

Sur le tableau de bord :

| Touche | Action                  |
|--------|-------------------------|
| `N`    | Ajouter une nouvelle œuvre |
| `/`    | Focaliser la barre de recherche |
| `S`    | Aller aux statistiques  |
| `P`    | Aller au profil         |
| `?`    | Afficher l’aide         |
| `Esc`  | Fermer / retirer le focus |

### Export / import

**Export :** Profil → télécharger la bibliothèque en CSV, ou **JSON** pour une sauvegarde versionnée (`export_version`) réimportable.

**Import :** Profil → importer un export CSV ou JSON. Le CSV utilise le point-virgule ; les colonnes optionnelles `CatalogID`, `IsAdult`, `ImagePath` peuvent suivre `Notes`. Choisissez si les titres déjà présents sont **ignorés** ou **mis à jour**.

```csv
Title;Chapter;Link;Status;Type;Rating;Notes;CatalogID;IsAdult;ImagePath
My Manga;42;https://...;En cours;Webtoon;4;Great series;;;0;
```

**Valeurs de statut** : En cours, Terminé, En pause, Abandonné, À lire  
**Valeurs de type** : Webtoon, Manga, Roman, Light Novel, Manhwa, Manhua, Autre

---

## Dépannage

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
