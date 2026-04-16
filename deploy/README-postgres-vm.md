# PostgreSQL sur une VM dédiée

Le script [`setup-postgres-vm.sh`](setup-postgres-vm.sh) automatise la création d’un rôle et d’une base vides pour BookStorage.

## Utilisation

Depuis la racine du dépôt cloné (`BookStorage/`) :

```bash
chmod +x deploy/setup-postgres-vm.sh
# Optionnel : installation des paquets Debian/Ubuntu (sudo requis)
sudo ./deploy/setup-postgres-vm.sh --install-packages
# Si apt reste bloqué sur « Waiting for headers » (souvent IPv6 ou réseau vers archive.ubuntu.com) :
sudo ./deploy/setup-postgres-vm.sh --install-packages --apt-ipv4
# Ou sur une instance où PostgreSQL est déjà installé (pas besoin d’apt) :
./deploy/setup-postgres-vm.sh
```

Évitez `sudo deploy/...` sans `./` : selon le répertoire courant, le shell peut ne pas résoudre le chemin comme prévu.

## Dépannage : `apt-get` très lent ou bloqué

- **Symptôme** : `0% [Waiting for headers]` pendant de longues minutes sur `archive.ubuntu.com` ou `security.ubuntu.com`.
- **À essayer** : relancer avec **`--apt-ipv4`** (force IPv4 pour apt).
- **Manuel** : `sudo apt-get update` puis `sudo apt-get install -y postgresql postgresql-contrib` ; si ça bloque encore, vérifier DNS, pare-feu sortant, proxy, ou remplacer temporairement les miroirs dans `/etc/apt/sources.list` par un miroir plus proche (documentation Ubuntu « Mirror »).
- **Contournement** : installer PostgreSQL par les moyens habituels de la VM (image cloud avec Postgres, paquet déjà présent), puis exécuter **`./deploy/setup-postgres-vm.sh`** sans `--install-packages` : le script ne fait alors que créer l’utilisateur, la base et afficher l’URL.

Variables optionnelles : `BS_PG_DB`, `BS_PG_USER`, `BS_PG_HOST` (affichage dans l’URL), `BS_PG_PORT`, `BS_PG_SSLMODE`.

## Après l’exécution

1. Copiez la ligne `BOOKSTORAGE_POSTGRES_URL=...` (ou les champs affichés) vers votre `.env` sur la **VM applicative**, ou saisissez-les dans l’assistant **Admin → PostgreSQL** (superadmin, migration depuis SQLite).
2. Sur la VM PostgreSQL, configurez `listen_addresses` et `pg_hba.conf` pour n’autoriser que l’hôte ou le réseau de l’application (évitez d’exposer 5432 sur Internet sans TLS et sans filtrage strict).
3. Le schéma des tables est créée par BookStorage au premier démarrage (`EnsureSchema`) ; ce script ne duplique pas le schéma applicatif.
