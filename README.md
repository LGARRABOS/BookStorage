# BookStorage

## Présentation
BookStorage est une application web Flask permettant de gérer une bibliothèque personnelle en ligne. Chaque utilisateur peut suivre la progression de ses lectures, tandis que des administrateurs valident les inscriptions et supervisent l'ensemble des comptes. Une interface spécifique aux administrateurs offre des actions dédiées (validation, promotion, suppression) avec un niveau de privilège supplémentaire pour les super-administrateurs.

## Fonctionnalités principales
- **Gestion des comptes utilisateurs** : inscription, connexion, déconnexion et validation par un administrateur avant l'accès au tableau de bord.
- **Gestion complète du profil** : modification du pseudo, du nom affiché, de l'adresse e-mail, de la biographie et téléversement d'un avatar avec contrôle de sécurité.
- **Confidentialité maîtrisée** : chaque membre choisit si son profil est public ou privé directement depuis la page de gestion du compte.
- **Rôles différenciés** : utilisateurs standard, administrateurs et super-administrateurs disposant de permissions étendues.
- **Tableau de bord personnel** : ajout d'œuvres avec statut, lien externe, nombre de chapitres et image illustrant la fiche.
- **Typologie des lectures** : chaque ajout comporte un type (roman, manga, BD, manhwa, etc.) pour catégoriser facilement ses lectures.
- **Mises à jour rapides** : incrémentation/décrémentation du chapitre courant par requêtes AJAX sans recharger la page.
- **Gestion des médias** : dépôt des images et avatars avec purge automatique des anciens fichiers inutilisés.
- **Annuaire communautaire** : liste des profils publics avec moteur de recherche, consultation des bibliothèques partagées et import direct d'œuvres inspirantes.
- **Interface d'administration** : vue consolidée des comptes pour approuver, promouvoir ou supprimer des utilisateurs, avec garde-fous selon les privilèges de l'administrateur courant.

## Architecture globale
- `app.py` : cœur de l'application Flask, contenant les routes utilisateur et administrateur, les décorateurs d'authentification et de permissions, ainsi que la logique de gestion des œuvres.
- `config.py` : résolution centralisée de la configuration (chemins, variables d'environnement, sécurité) partagée entre l'application et les scripts utilitaires.
- `wsgi.py` : point d'entrée WSGI recommandé pour les serveurs en production (Gunicorn, uWSGI, Waitress...).
- `templates/` : gabarits Jinja2 pour les pages publiques, le tableau de bord, la gestion de profil et l'administration.
- `static/` : ressources statiques (feuilles de style, scripts, images importées par les utilisateurs).
- `init_db.py` : script d'initialisation de la base SQLite qui respecte la même configuration que l'application.
- `tests/` : batterie de tests Pytest couvrant la gestion des comptes, du profil, des œuvres et de l'annuaire communautaire.
- `.env.example` : gabarit de configuration à copier avant un déploiement.

## Modèle de données
### Table `users`
| Colonne | Type | Description |
| --- | --- | --- |
| `id` | INTEGER (PK) | Identifiant unique. |
| `username` | TEXT (unique) | Nom d'utilisateur. |
| `password` | TEXT | Hash du mot de passe (Werkzeug). |
| `validated` | INTEGER | 0 = compte en attente, 1 = compte validé. |
| `is_admin` | INTEGER | 1 si l'utilisateur est administrateur. |
| `is_superadmin` | INTEGER | 1 si l'utilisateur dispose des privilèges super-admin. |
| `display_name` | TEXT | Nom public facultatif affiché sur le profil. |
| `email` | TEXT | Adresse de contact facultative. |
| `bio` | TEXT | Présentation courte affichée sur le profil. |
| `avatar_path` | TEXT | Chemin relatif vers l'image de profil téléversée. |
| `is_public` | INTEGER | 1 = profil visible dans l'annuaire et accessible aux autres membres. |

### Table `works`
| Colonne | Type | Description |
| --- | --- | --- |
| `id` | INTEGER (PK) | Identifiant unique de l'œuvre. |
| `title` | TEXT | Titre de l'œuvre (limité à 30 caractères côté formulaire). |
| `chapter` | INTEGER | Chapitre actuellement atteint. |
| `link` | TEXT | Lien externe optionnel (site de lecture, fiche détaillée, etc.). |
| `status` | TEXT | Statut de lecture (En cours, Terminé, etc.). |
| `image_path` | TEXT | Chemin relatif vers l'image téléversée. |
| `reading_type` | TEXT | Type de lecture sélectionné (Roman, Manga, BD, Manhwa, etc.). |
| `user_id` | INTEGER (FK) | Identifiant de l'utilisateur propriétaire. |

## Prérequis
- Python 3.9 ou supérieur.
- SQLite (installé par défaut avec Python sur la plupart des plateformes).
- Accès à un serveur WSGI (Waitress est intégré, Gunicorn + Nginx restent recommandés pour Linux).
- Dépendances Python listées dans `requirements.txt` (`Flask`, `Werkzeug`, `gunicorn`, `waitress`, `python-dotenv`, `pytest`).

## Installation rapide (mode développement)
1. **Cloner le dépôt et se placer dans le dossier du projet** :
   ```bash
   git clone <votre-url-git>
   cd BookStorage
   ```
2. **Créer et activer un environnement virtuel** (fortement recommandé) :
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # Sous Windows : .venv\Scripts\activate
   ```
3. **Préparer la configuration** :
   ```bash
   cp .env.example .env
   # Modifier ensuite .env pour adapter BOOKSTORAGE_SECRET_KEY, BOOKSTORAGE_ENV, etc.
   ```
   Pour un développement local, définissez `BOOKSTORAGE_ENV=development` et utilisez une clé secrète générée par `python -c "import secrets; print(secrets.token_hex(32))"`.
4. **Installer les dépendances** :
   ```bash
   pip install -r requirements.txt
   ```
5. **Initialiser la base de données** (honore les chemins définis dans `.env`) :
   ```bash
   python init_db.py
   ```
6. **Lancer le serveur de développement** :
   ```bash
   flask --app wsgi --debug run
   ```
   ou
   ```bash
   python app.py  # Waitress est utilisé automatiquement si BOOKSTORAGE_ENV=production
   ```
   L'application est disponible sur `http://127.0.0.1:5000/`.

## Configuration
Toutes les options se pilotent par variables d'environnement (chargées automatiquement depuis `.env` si `python-dotenv` est présent).

| Variable | Description | Valeur par défaut |
| --- | --- | --- |
| `BOOKSTORAGE_ENV` | Profil d'exécution (`development` ou `production`). | `development` |
| `BOOKSTORAGE_SECRET_KEY` | Clé secrète Flask pour sécuriser les sessions. **Obligatoire en production.** | `dev-secret-change-me` |
| `BOOKSTORAGE_DATA_DIR` | Dossier racine pour la base SQLite (absolu ou relatif au projet). | répertoire du projet |
| `BOOKSTORAGE_DATABASE` | Nom ou chemin du fichier SQLite. | `database.db` |
| `BOOKSTORAGE_UPLOAD_DIR` | Répertoire de stockage des couvertures. | `static/images` |
| `BOOKSTORAGE_UPLOAD_URL_PATH` | Chemin relatif utilisé dans les URLs statiques pour les couvertures. | `images` |
| `BOOKSTORAGE_AVATAR_DIR` | Répertoire de stockage des avatars. | `static/avatars` |
| `BOOKSTORAGE_AVATAR_URL_PATH` | Chemin relatif utilisé pour les avatars. | `avatars` |
| `BOOKSTORAGE_SUPERADMIN_USERNAME` | Identifiant du super-administrateur créé au bootstrap. | `superadmin` |
| `BOOKSTORAGE_SUPERADMIN_PASSWORD` | Mot de passe associé (à changer impérativement en production). | `SuperAdmin!2023` |
| `BOOKSTORAGE_HOST` | Interface réseau écoutée par l'application. | `127.0.0.1` |
| `BOOKSTORAGE_PORT` | Port HTTP exposé par l'application. | `5000` |

> ⚠️ Lorsque `BOOKSTORAGE_ENV=production`, l'application refuse de démarrer si `BOOKSTORAGE_SECRET_KEY` n'est pas défini explicitement.

## Déploiement sur un serveur
1. **Créer un utilisateur système et les répertoires dédiés** (exemple sous Linux) :
   ```bash
   sudo useradd --system --create-home --shell /bin/false bookstorage
   sudo mkdir -p /var/lib/bookstorage/uploads /var/lib/bookstorage/avatars
   sudo chown -R bookstorage:bookstorage /var/lib/bookstorage
   ```
2. **Cloner le dépôt dans le home de l'utilisateur et préparer l'environnement** :
   ```bash
   sudo -u bookstorage git clone <votre-url-git> /home/bookstorage/app
   cd /home/bookstorage/app
   sudo -u bookstorage python3 -m venv .venv
   sudo -u bookstorage bash -c 'source .venv/bin/activate && pip install -r requirements.txt'
   cp .env.example .env
   ```
3. **Configurer `.env` pour la production** (exemple) :
   ```env
   BOOKSTORAGE_ENV=production
   BOOKSTORAGE_SECRET_KEY=<clé générée>
   BOOKSTORAGE_DATA_DIR=/var/lib/bookstorage
   BOOKSTORAGE_UPLOAD_DIR=/var/lib/bookstorage/uploads
   BOOKSTORAGE_UPLOAD_URL_PATH=uploads
   BOOKSTORAGE_AVATAR_DIR=/var/lib/bookstorage/avatars
   BOOKSTORAGE_AVATAR_URL_PATH=avatars
   BOOKSTORAGE_SUPERADMIN_PASSWORD=<mot de passe fort>
   FLASK_APP=wsgi:app
   ```
   Assurez-vous que les dossiers pointés par `BOOKSTORAGE_UPLOAD_URL_PATH` sont servis par votre serveur HTTP (via un alias ou un lien symbolique vers `static/`).
4. **Initialiser la base de données** :
   ```bash
   sudo -u bookstorage bash -c 'cd /home/bookstorage/app && source .venv/bin/activate && python init_db.py'
   ```
5. **Lancer l'application avec Waitress** (test manuel rapide) :
   ```bash
   sudo -u bookstorage bash -c 'cd /home/bookstorage/app && source .venv/bin/activate && BOOKSTORAGE_ENV=production python app.py'
   ```
   ou, pour un processus détaché contrôlé différemment, utilisez Gunicorn :
   ```bash
   sudo -u bookstorage bash -c 'cd /home/bookstorage/app && source .venv/bin/activate && gunicorn --bind 0.0.0.0:8000 wsgi:app'
   ```
6. **Optionnel : créer une unité systemd** `/etc/systemd/system/bookstorage.service` :
   ```ini
   [Unit]
   Description=BookStorage
   After=network.target

   [Service]
   User=bookstorage
   Group=bookstorage
   WorkingDirectory=/home/bookstorage/app
   EnvironmentFile=/home/bookstorage/app/.env
   ExecStart=/home/bookstorage/app/.venv/bin/gunicorn --bind unix:/run/bookstorage.sock wsgi:app
   ExecReload=/bin/kill -HUP $MAINPID
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target
   ```
   Puis activer et démarrer le service :
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now bookstorage
   ```
7. **Placer un serveur HTTP en frontal** (Nginx/Apache) pour servir les fichiers statiques (`static/` et les dossiers configurés) et faire suivre le trafic HTTP vers le socket/port exposé par Gunicorn.

## Parcours utilisateur
1. **Inscription** (`/register`) : le compte est créé avec `validated=0` et ne peut pas se connecter tant qu'un administrateur ne l'a pas approuvé.
2. **Connexion** (`/login`) : seules les identifiants valides sont autorisés. Les administrateurs et super-admins peuvent se connecter même si `validated=0`.
3. **Tableau de bord** (`/dashboard`) : liste des œuvres de l'utilisateur connecté avec options d'ajout, de filtrage par statut/type, de mise à jour rapide des chapitres et de suppression.
4. **Ajout d'œuvre** (`/add_work`) : formulaire pour saisir titre, lien, statut, type de lecture, chapitre et téléverser une image (formats autorisés : PNG, JPG, JPEG, GIF). Les fichiers sont stockés dans le dossier configuré par `BOOKSTORAGE_UPLOAD_DIR`.
5. **Mises à jour AJAX** (`/api/increment/<id>` et `/api/decrement/<id>`) : endpoints JSON utilisés par le tableau de bord pour ajuster le chapitre courant sans rechargement.
6. **Suppression d'œuvre** (`/delete/<id>`) : retire définitivement l'œuvre du catalogue personnel.
7. **Gestion du profil** (`/profile`) : page dédiée pour modifier les informations personnelles, changer de mot de passe après confirmation du mot de passe actuel, déposer une image de profil sécurisée et définir la visibilité du compte (public ou privé).
8. **Découverte communautaire** (`/users`) : annuaire filtrable des profils publics permettant de consulter le détail d'un lecteur et d'importer ses lectures dans sa propre bibliothèque.

## Administration
- **Accès restreint** : toutes les routes préfixées par `/admin` nécessitent l'authentification et le rôle administrateur (`admin_required`).
- **Validation des comptes** (`/admin/approve/<id>`) : marque un compte comme validé, lui ouvrant l'accès au tableau de bord.
- **Promotion** (`/admin/promote/<id>`) : transforme un utilisateur standard en administrateur et le valide automatiquement.
- **Suppression** (`/admin/delete_account/<id>`) :
  - Les administrateurs ne peuvent supprimer que des comptes non administrateurs.
  - Les super-administrateurs peuvent supprimer des administrateurs (sauf autres super-admins) et voient l'état de leurs propres privilèges directement dans la page.
- **Vue globale** (`/admin/accounts`) : tableau listant tous les comptes avec indicateurs de validation, de rôle et actions disponibles selon les privilèges du spectateur.

## Gestion des fichiers téléversés
- Les images de couvertures et avatars sont stockées dans les répertoires indiqués par `BOOKSTORAGE_UPLOAD_DIR` et `BOOKSTORAGE_AVATAR_DIR`.
- Les noms de fichiers sont sécurisés via `werkzeug.utils.secure_filename` et précédés d'un identifiant unique pour éviter les collisions.
- Lorsqu'un avatar ou une couverture est remplacé(e), l'ancien fichier est automatiquement supprimé s'il n'est plus référencé.
- En production, exposez ces répertoires via votre serveur web (alias Nginx/Apache ou lien symbolique vers `static/`).

## Dépannage
- **"Compte non validé" lors de la connexion** : connectez-vous avec un administrateur et approuvez le compte via `/admin/accounts`.
- **Accès refusé aux actions d'administration** : vérifiez que la session contient `is_admin` ou `is_superadmin`. Une reconnexion actualise ces drapeaux.
- **Images manquantes** : assurez-vous que le fichier a été téléversé sans erreur, que les dossiers configurés existent et que le serveur web les expose correctement.
- **Base de données corrompue** : supprimez le fichier SQLite configuré puis relancez `python init_db.py` (perte des données existantes).

## Tests et maintenance
- Vérifiez rapidement la syntaxe Python :
  ```bash
  python -m compileall app.py
  ```
- Lancez la suite de tests automatisés avec Pytest :
  ```bash
  pytest
  ```
  Les tests fournis valident la gestion des privilèges d'administration, le blocage des comptes non validés, la rétrocompatibilité des anciens mots de passe `scrypt`, les scénarios complets de mise à jour du profil (informations, mot de passe, avatar, confidentialité) ainsi que les parcours communautaires (recherche d'utilisateurs, accès aux profils publics/privés et import d'œuvres partagées). Des cas vérifient également la conservation du type de lecture lors d'un import et la présence des filtres côté tableau de bord.

## Licence
Indiquez ici la licence souhaitée (par défaut non spécifiée). Ajoutez un fichier `LICENSE` si nécessaire.
