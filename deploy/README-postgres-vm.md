# PostgreSQL sur une VM dédiée

Le script [`setup-postgres-vm.sh`](setup-postgres-vm.sh) automatise la création d’un rôle et d’une base vides pour BookStorage.

## Utilisation

```bash
chmod +x deploy/setup-postgres-vm.sh
# Optionnel : installation des paquets Debian/Ubuntu (sudo requis)
sudo deploy/setup-postgres-vm.sh --install-packages
# Ou sur une instance où PostgreSQL est déjà installé :
deploy/setup-postgres-vm.sh
```

Variables optionnelles : `BS_PG_DB`, `BS_PG_USER`, `BS_PG_HOST` (affichage dans l’URL), `BS_PG_PORT`, `BS_PG_SSLMODE`.

## Après l’exécution

1. Copiez la ligne `BOOKSTORAGE_POSTGRES_URL=...` (ou les champs affichés) vers votre `.env` sur la **VM applicative**, ou saisissez-les dans l’assistant **Admin → PostgreSQL** (superadmin, migration depuis SQLite).
2. Sur la VM PostgreSQL, configurez `listen_addresses` et `pg_hba.conf` pour n’autoriser que l’hôte ou le réseau de l’application (évitez d’exposer 5432 sur Internet sans TLS et sans filtrage strict).
3. Le schéma des tables est créée par BookStorage au premier démarrage (`EnsureSchema`) ; ce script ne duplique pas le schéma applicatif.
