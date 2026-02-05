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

## 🚀 Démarrage rapide

### Prérequis

- **Go 1.22+** 
- **GCC** (pour la compilation de SQLite avec CGO)

### Lancer en développement

```bash
# Cloner le projet
git clone https://github.com/VOTRE_USERNAME/BookStorage.git
cd BookStorage

# Installer les dépendances
go mod tidy

# Lancer le serveur
go run .
```

Le serveur démarre sur **http://127.0.0.1:5000**

### Voir toutes les commandes disponibles

```bash
make help
```

---

## 📦 Installation en Production (Linux)

### Installation automatique

```bash
# Cloner et installer
git clone https://github.com/VOTRE_USERNAME/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
```

Le script configure automatiquement :
- Compilation de l'application
- Service systemd
- Configuration du firewall
- Fichier `.env` avec clé secrète générée

### Commandes du service

```bash
sudo systemctl start bookstorage     # Démarrer
sudo systemctl stop bookstorage      # Arrêter
sudo systemctl restart bookstorage   # Redémarrer
sudo systemctl status bookstorage    # Voir le statut
```

### Mise à jour

```bash
cd /opt/bookstorage
sudo make update
```

### Logs

```bash
# Logs en temps réel
sudo journalctl -u bookstorage -f

# Dernières 50 lignes
sudo journalctl -u bookstorage -n 50
```

---

## 🛠️ Commandes Make

Utilisez `make help` pour voir toutes les commandes :

| Commande | Description |
|----------|-------------|
| `make build` | Compile l'application |
| `make build-prod` | Compile en mode production (binaire optimisé) |
| `make run` | Lance en mode développement |
| `make clean` | Supprime les fichiers compilés |
| `make install` | Installe le service systemd |
| `make uninstall` | Désinstalle le service |
| `make update` | Met à jour (pull + rebuild + restart) |
| `make fix-perms` | Corrige les permissions des fichiers |
| `make help` | Affiche l'aide |

---

## ⚙️ Configuration

### Variables d'environnement

Créez un fichier `.env` à la racine du projet ou définissez ces variables :

| Variable | Description | Défaut |
|----------|-------------|--------|
| `BOOKSTORAGE_HOST` | Adresse d'écoute | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | Port | `5000` |
| `BOOKSTORAGE_DATABASE` | Chemin base SQLite | `database.db` |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète pour les sessions | `dev-secret-change-me` |
| `BOOKSTORAGE_SUPERADMIN_USERNAME` | Nom du super administrateur | `superadmin` |
| `BOOKSTORAGE_SUPERADMIN_PASSWORD` | Mot de passe super admin | `SuperAdmin!2023` |

### Exemple de fichier `.env`

```env
BOOKSTORAGE_HOST=0.0.0.0
BOOKSTORAGE_PORT=5000
BOOKSTORAGE_DATABASE=/opt/bookstorage/database.db
BOOKSTORAGE_SECRET_KEY=votre-cle-secrete-tres-longue-et-complexe
BOOKSTORAGE_SUPERADMIN_USERNAME=admin
BOOKSTORAGE_SUPERADMIN_PASSWORD=MotDePasseSecurise123!
```

---

## 📁 Structure du projet

```
BookStorage/
├── main.go              # Point d'entrée de l'application
├── config.go            # Configuration et variables d'environnement
├── db.go                # Schéma SQLite et migrations
├── handlers.go          # Routes HTTP et logique métier
├── go.mod / go.sum      # Dépendances Go
├── Makefile             # Commandes de build/deploy
├── .env.example         # Exemple de configuration
│
├── deploy/              # Déploiement
│   ├── install.sh       # Script d'installation Linux
│   └── bookstorage.service  # Service systemd
│
├── templates/           # Templates HTML (Go html/template)
│   ├── dashboard.gohtml
│   ├── login.gohtml
│   └── ...
│
└── static/              # Fichiers statiques
    ├── css/             # Feuilles de style
    ├── avatars/         # Avatars utilisateurs (uploads)
    └── images/          # Images des œuvres (uploads)
```

---

## 🔄 Migration depuis Python/Flask

Si vous avez une ancienne version Python de BookStorage :

1. **Copiez** votre fichier `database.db` vers `/opt/bookstorage/`
2. **Corrigez** les permissions : `sudo make fix-perms`
3. **Redémarrez** : `sudo systemctl restart bookstorage`

> Les mots de passe hashés avec Werkzeug (format `pbkdf2:sha256`) sont automatiquement reconnus.

---

## 🐛 Dépannage

### Le service ne démarre pas

```bash
# Vérifier les logs
sudo journalctl -u bookstorage -n 100

# Erreur "readonly database" → Corriger les permissions
cd /opt/bookstorage
sudo make fix-perms
sudo systemctl restart bookstorage
```

### Port déjà utilisé

```bash
# Voir quel processus utilise le port 5000
sudo lsof -i :5000

# Changer le port dans .env
BOOKSTORAGE_PORT=5001
```

### Problème de compilation (CGO)

```bash
# Installer GCC sur Rocky/RHEL/CentOS
sudo dnf install gcc

# Installer GCC sur Debian/Ubuntu
sudo apt install gcc
```

---

## 📝 Licence

MIT License - Voir [LICENSE](LICENSE) pour plus de détails.

---

## 🤝 Contribution

Les contributions sont les bienvenues ! N'hésitez pas à ouvrir une issue ou une pull request.

---

<p align="center">
  Fait avec ❤️ pour les lecteurs
</p>
