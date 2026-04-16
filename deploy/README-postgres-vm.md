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
# Si « 0 % [Waiting for headers] » sans savoir si c’est lent ou bloqué : journal HTTP très verbeux
sudo ./deploy/setup-postgres-vm.sh --install-packages --apt-ipv4 --apt-debug-http
# Ou sur une instance où PostgreSQL est déjà installé (pas besoin d’apt) :
./deploy/setup-postgres-vm.sh
```

Évitez `sudo deploy/...` sans `./` : selon le répertoire courant, le shell peut ne pas résoudre le chemin comme prévu.

Lancer le script avec **`sudo ./deploy/...`** est supporté : les commandes SQL passent par **`sudo -u postgres psql`** (l’auth « peer » ne donne pas de rôle PostgreSQL à l’utilisateur `root`).

## Dépannage : `apt-get` très lent ou bloqué

- **Symptôme** : `0% [Waiting for headers]` sur `archive.ubuntu.com` ou `security.ubuntu.com` — parfois **`apt-get update` réussit** puis **`apt-get install` reste à 0 %** (téléchargement des paquets, pas les mêmes connexions que pour les index).
- **Script récent** : le dépôt force des options apt plus tolérantes (pas de pipelining HTTP, peu de connexions parallèles, timeouts plus longs). Mettez à jour avec `git pull` puis relancez la même commande.
- **À essayer** : **`--apt-ipv4`** si vous suspectez un souci IPv6.
- **Lent mais normal** : à ~50–60 kB/s, ~40–45 Mo de paquets peuvent prendre **10–20 minutes** sans être bloqués ; laissez tourner si le pourcentage avance par à-coups.
- **`git pull` refuse de fusionner** : soit `git stash push -m wip` (ou supprimez les modifications locales sur `deploy/setup-postgres-vm.sh`), soit reclonez le dépôt.
- **Manuel** : `sudo apt-get update` puis `sudo apt-get install -y postgresql postgresql-contrib` avec les mêmes options réseau que votre politique (miroir, proxy, `-o Acquire::http::Pipeline-Depth=0`, etc.).
- **Contournement** : installer PostgreSQL par les moyens habituels de la VM, puis **`./deploy/setup-postgres-vm.sh`** sans `--install-packages`.

Variables optionnelles : `BS_PG_DB`, `BS_PG_USER`, `BS_PG_HOST` (affichage dans l’URL), `BS_PG_PORT`, `BS_PG_SSLMODE`, `BS_PG_APT_WATCHDOG_SECS` (intervalle des lignes `[watchdog …]` pendant `apt`, défaut 25).

Pendant `apt`, le script affiche toutes les **25 s** (par défaut) une ligne **`[watchdog +…s]`** sur la sortie d’erreur : si elle continue d’apparaître, le processus **n’est pas figé** (souvent attente ou très faible débit). **`--apt-debug-http`** demande à apt de journaliser chaque requête HTTP (beaucoup de texte, mais on voit tout de suite si quelque chose bouge).

## Après l’exécution

1. Copiez la ligne `BOOKSTORAGE_POSTGRES_URL=...` (ou les champs affichés) vers votre `.env` sur la **VM applicative**, ou saisissez-les dans l’assistant **Admin → PostgreSQL** (superadmin, migration depuis SQLite).
2. Sur la VM PostgreSQL, configurez `listen_addresses` et `pg_hba.conf` pour n’autoriser que l’hôte ou le réseau de l’application (évitez d’exposer 5432 sur Internet sans TLS et sans filtrage strict).
3. Le schéma des tables est créée par BookStorage au premier démarrage (`EnsureSchema`) ; ce script ne duplique pas le schéma applicatif.
