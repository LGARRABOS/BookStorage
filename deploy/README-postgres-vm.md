# PostgreSQL sur une VM dédiée

Le script [`setup-postgres-vm.sh`](setup-postgres-vm.sh) automatise la création d’un rôle et d’une base vides pour BookStorage.

## Modèle réseau (auto-hébergement)

BookStorage suppose ici que **PostgreSQL tourne sur une autre machine joignable depuis la VM applicative sur un réseau privé** (LAN, VLAN, VPN site-à-site, etc.) — **sans exposition du port 5432 sur Internet**, ce qui est la bonne pratique pour la sécurité.

Conséquences pratiques :

- L’URL `BOOKSTORAGE_POSTGRES_URL` doit cibler une **IP privée** (ex. `192.168.x.x`) ou un **nom résolu par la VM app** (fichier **`/etc/hosts`**, DNS interne d’entreprise, etc.).
- Un **nom de machine connu seulement sur la VM Postgres** (ex. `BookStorageDB`) **ne sera pas** résolu par les DNS publics (8.8.8.8, etc.) : d’où l’erreur `lookup … no such host` si vous ne faites pas `/etc/hosts` ou IP dans l’URL.
- Sur la VM **Postgres**, il faut quand même **`listen_addresses`** + **`pg_hba.conf`** adaptés au **sous-réseau de la VM app** (voir plus bas) ; le pare-feu n’autorise que ce trafic interne.

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

Lancer le script avec **`sudo ./deploy/...`** est supporté : les commandes SQL passent par **`cd /tmp` puis `sudo -H -u postgres psql`** (libpq tente de se placer dans le répertoire courant du shell ; sous `postgres`, `/home/autreuser/...` est en général interdit — d’où l’ancien message « could not change directory » sans ce `cd`).

## Dépannage : `apt-get` très lent ou bloqué

- **Symptôme** : `0% [Waiting for headers]` sur `archive.ubuntu.com` ou `security.ubuntu.com` — parfois **`apt-get update` réussit** puis **`apt-get install` reste à 0 %** (téléchargement des paquets, pas les mêmes connexions que pour les index).
- **Script récent** : le dépôt force des options apt plus tolérantes (pas de pipelining HTTP, peu de connexions parallèles, timeouts plus longs). Mettez à jour avec `git pull` puis relancez la même commande.
- **À essayer** : **`--apt-ipv4`** si vous suspectez un souci IPv6.
- **Lent mais normal** : à ~50–60 kB/s, ~40–45 Mo de paquets peuvent prendre **10–20 minutes** sans être bloqués ; laissez tourner si le pourcentage avance par à-coups.
- **`git pull` refuse de fusionner** : soit `git stash push -m wip` (ou supprimez les modifications locales sur `deploy/setup-postgres-vm.sh`), soit reclonez le dépôt.
- **Manuel** : `sudo apt-get update` puis `sudo apt-get install -y postgresql postgresql-contrib` avec les mêmes options réseau que votre politique (miroir, proxy, `-o Acquire::http::Pipeline-Depth=0`, etc.).
- **Contournement** : installer PostgreSQL par les moyens habituels de la VM, puis **`./deploy/setup-postgres-vm.sh`** sans `--install-packages`.

Variables optionnelles : `BS_PG_DB`, `BS_PG_USER`, `BS_PG_HOST` (hôte dans l’URL ; **si non défini**, le script tente d’utiliser l’**IPv4 LAN** détectée pour que la VM applicative n’ait pas besoin du DNS interne), `BS_PG_PORT`, `BS_PG_SSLMODE`, `BS_PG_APT_WATCHDOG_SECS`, `BS_PG_PSQL_CWD`.

Pendant `apt`, le script affiche toutes les **25 s** (par défaut) une ligne **`[watchdog +…s]`** sur la sortie d’erreur : si elle continue d’apparaître, le processus **n’est pas figé** (souvent attente ou très faible débit). **`--apt-debug-http`** demande à apt de journaliser chaque requête HTTP (beaucoup de texte, mais on voit tout de suite si quelque chose bouge).

## PostgreSQL ne répond pas (`No such file or directory` sur le socket)

Le script utilise le socket local. Si le serveur **n’écoute pas**, le socket peut être absent.

### Ubuntu / Debian : `postgresql.service` en « active (exited) »

C’est **normal** : `postgresql.service` est un méta-service (`ExecStart=/bin/true`) ; il ne démarre **pas** le moteur PostgreSQL.

Listez les clusters et leur état :

```bash
pg_lsclusters
```

Démarrez le **cluster** (adaptez `14` / `main` selon la sortie) :

```bash
sudo systemctl start postgresql@14-main
sudo systemctl status postgresql@14-main
sudo systemctl enable postgresql@14-main   # au boot
```

Puis relancez `./deploy/setup-postgres-vm.sh` (le script tente aussi de démarrer automatiquement les clusters marqués `down` dans `pg_lsclusters`).

### Le cluster refuse de démarrer (`could not bind IPv4`, `could not create any listen sockets`)

`systemctl` tronque les lignes : lisez le journal Postgres (là est la cause exacte) :

```bash
sudo tail -100 /var/log/postgresql/postgresql-14-main.log
# ou
sudo journalctl -u postgresql@14-main -n 80 --no-pager
```

Causes fréquentes :

1. **Port 5432 déjà utilisé** (autre Postgres, Docker, autre outil) :
   ```bash
   sudo ss -lntp | grep 5432
   ```
   Arrêtez le processus concurrent ou changez `port` dans `/etc/postgresql/14/main/postgresql.conf`.

2. **`listen_addresses` pointe vers une IP que cette VM n’a pas** (erreur du type *Cannot assign requested address*). Ouvrez `/etc/postgresql/14/main/postgresql.conf` et utilisez au minimum pour valider le démarrage :
   - `listen_addresses = 'localhost'` ou `'*'`
   Puis : `sudo systemctl restart postgresql@14-main`.

3. **Espace disque** sur `/var` : `df -h /var/lib/postgresql`.

## Après l’exécution

1. Copiez la ligne `BOOKSTORAGE_POSTGRES_URL=...` (ou les champs affichés) vers votre `.env` sur la **VM applicative**, ou saisissez-les dans l’assistant **Admin → PostgreSQL** (superadmin, migration depuis SQLite). Vérifiez que l’URL est **complète** (ex. `sslmode=prefer`, pas tronquée).
2. **Connexion depuis une autre machine** : sur Ubuntu/Debian, PostgreSQL écoute souvent **uniquement sur `127.0.0.1`**. Il faut alors :
   - dans **`postgresql.conf`** (souvent `/etc/postgresql/14/main/postgresql.conf`) : `listen_addresses = '*'` ou l’IP LAN de la VM ;
   - dans **`pg_hba.conf`** (même répertoire) : une ligne du type `host  all  all  192.168.1.0/24  scram-sha-256` (adaptez le sous-réseau à votre LAN) ;
   - **`sudo systemctl restart postgresql`** ;
   - ouvrir le **pare-feu** (port `5432/tcp`) depuis l’IP de la VM applicative.
3. Si l’appli affiche **`connect_failed`** : utilisez l’**IP** dans l’URL si le **nom d’hôte** de la VM Postgres n’est pas dans le DNS de la VM app (sinon `dial tcp: lookup …`). Après mise à jour BookStorage, le test d’admin affiche aussi un **détail** (`detail`) avec le message d’erreur PostgreSQL/driver.
4. Le schéma des tables est créée par BookStorage au premier démarrage (`EnsureSchema`) ; ce script ne duplique pas le schéma applicatif.

Pour générer l’URL avec l’IP LAN dans le champ hôte (au lieu du hostname), sur la VM Postgres :  
`sudo env BS_PG_HOST=192.168.1.117 ./deploy/setup-postgres-vm.sh` (adaptez l’IP).
