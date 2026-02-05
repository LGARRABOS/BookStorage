# 📚 BookStorage

**BookStorage** est une application web de suivi de lectures personnelles. Suivez vos romans, mangas, webtoons, light novels et plus encore.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

## ✨ Fonctionnalités

- 📖 **Multi-formats** : Romans, mangas, manhwas, webtoons, light novels...
- ⭐ **Notes & avis** : Notez vos œuvres de 1 à 5 étoiles avec des notes personnelles
- 📊 **Statistiques** : Visualisez vos habitudes de lecture
- 👥 **Communauté** : Explorez les bibliothèques publiques des autres lecteurs
- 🌓 **Mode sombre** : Interface claire ou sombre selon vos préférences
- 🔐 **Vie privée** : Profil public ou privé, vous choisissez

---

## 🚀 Démarrage rapide

### Prérequis

- **Go 1.22+** 
- **GCC** (pour la compilation de SQLite avec CGO)

### Lancer en développement

```bash
# Cloner le projet
git clone https://github.com/VOTRE_USERNAME/BookStorage.git
cd BookStorage

# Lancer le serveur
go run .
```

Le serveur démarre sur **http://127.0.0.1:5000**

---

## 📦 Installation en Production (Linux)

### Installation automatique

```bash
# Cloner et installer (en root)
git clone https://github.com/VOTRE_USERNAME/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

Le script installe automatiquement :
- L'application compilée
- Le CLI `bsctl` pour gérer le service
- Le service systemd
- La configuration du firewall

### Démarrer le service

```bash
bsctl start
```

---

## 🛠️ Commandes bsctl

`bsctl` (BookStorage Control) est le CLI pour gérer BookStorage.

```bash
bsctl help     # Afficher l'aide
```

### Service

| Commande | Description |
|----------|-------------|
| `bsctl start` | Démarre le service |
| `bsctl stop` | Arrête le service |
| `bsctl restart` | Redémarre le service |
| `bsctl status` | Affiche le statut |
| `bsctl logs` | Affiche les logs en temps réel |

### Développement

| Commande | Description |
|----------|-------------|
| `bsctl build` | Compile l'application |
| `bsctl build-prod` | Compile en mode production |
| `bsctl run` | Lance le serveur de dev |
| `bsctl clean` | Supprime les fichiers compilés |

### Production

| Commande | Description |
|----------|-------------|
| `bsctl install` | Installe le service systemd |
| `bsctl uninstall` | Désinstalle le service |
| `bsctl update` | Met à jour (pull + build + restart) |
| `bsctl fix-perms` | Corrige les permissions |

---

## ⚙️ Configuration

### Variables d'environnement

Créez un fichier `.env` à la racine ou dans `/opt/bookstorage/` :

```env
# Serveur
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000

# Base de données
BOOKSTORAGE_DATABASE=/opt/bookstorage/database.db

# Sécurité (généré automatiquement à l'installation)
BOOKSTORAGE_SECRET_KEY=votre-cle-secrete-tres-longue

# Super administrateur
BOOKSTORAGE_SUPERADMIN_USERNAME=admin
BOOKSTORAGE_SUPERADMIN_PASSWORD=MotDePasseSecurise123!
```

| Variable | Description | Défaut |
|----------|-------------|--------|
| `BOOKSTORAGE_HOST` | Adresse d'écoute | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | Port | `5000` |
| `BOOKSTORAGE_DATABASE` | Chemin base SQLite | `database.db` |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète sessions | `dev-secret-change-me` |

---

## 📁 Structure du projet

```
BookStorage/
├── main.go              # Point d'entrée
├── config.go            # Configuration
├── db.go                # Schéma SQLite
├── handlers.go          # Routes HTTP
├── bsctl                # CLI de gestion
├── Makefile             # Commandes make
├── go.mod / go.sum      # Dépendances Go
│
├── deploy/
│   ├── install.sh       # Script d'installation
│   └── bookstorage.service
│
├── templates/           # Templates HTML
└── static/              # CSS, images, avatars
```

---

## 🔄 Migration depuis Python/Flask

Si vous avez une ancienne version Python :

```bash
# Copier la base de données
cp /ancien/chemin/database.db /opt/bookstorage/

# Corriger les permissions et redémarrer
bsctl fix-perms
bsctl restart
```

> Les mots de passe Werkzeug (`pbkdf2:sha256`) sont automatiquement reconnus.

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

MIT License

---

<p align="center">
  Fait avec ❤️ pour les lecteurs
</p>
