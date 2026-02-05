# BookStorage

Gestionnaire de lectures personnelles — version Go.

## Prérequis

- Go 1.22+
- GCC (pour la compilation de SQLite)

## Développement

```bash
# Installer les dépendances
go mod tidy

# Lancer en mode développement
go run .
```

Le serveur démarre sur `http://127.0.0.1:5000`.

## Déploiement en Production (VM Linux)

### Installation initiale

```bash
# Cloner le repo
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage

# Lancer l'installation (en root)
sudo ./deploy/install.sh

# Démarrer le service
sudo systemctl start bookstorage
```

### Commandes utiles

```bash
# Gérer le service
sudo systemctl start bookstorage    # Démarrer
sudo systemctl stop bookstorage     # Arrêter
sudo systemctl restart bookstorage  # Redémarrer
sudo systemctl status bookstorage   # Statut

# Voir les logs
sudo journalctl -u bookstorage -f
```

### Mise à jour

```bash
cd /opt/bookstorage
sudo make update
```

Cette commande va :
1. Pull les dernières modifications
2. Recompiler l'application
3. Redémarrer le service

## Configuration

Variables d'environnement (à mettre dans `/opt/bookstorage/.env` ou dans le service) :

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

1. Copiez votre fichier `database.db` de l'ancienne version dans `/opt/bookstorage/`
2. Redémarrez le service : `sudo systemctl restart bookstorage`
3. Les mots de passe hashés (format Werkzeug) sont automatiquement reconnus

## Structure

```
BookStorage/
├── main.go          # Point d'entrée
├── config.go        # Configuration et variables d'environnement
├── db.go            # Schéma SQLite et migrations
├── handlers.go      # Routes HTTP et logique métier
├── go.mod           # Dépendances Go
├── Makefile         # Commandes de build/deploy
├── deploy/          # Scripts et config de déploiement
│   ├── install.sh   # Script d'installation
│   └── bookstorage.service  # Service systemd
├── templates/       # Templates HTML (Go html/template)
└── static/          # CSS et fichiers statiques
```
