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
- **Gestion des médias** : dépôt des images dans `static/images` et affichage automatique sur le tableau de bord.
- **Interface d'administration** : vue consolidée des comptes pour approuver, promouvoir ou supprimer des utilisateurs, avec garde-fous selon les privilèges de l'administrateur courant.
- **Annuaire communautaire** : liste des profils publics avec moteur de recherche, consultation des bibliothèques partagées et import d'œuvres inspirantes dans sa propre liste.

## Architecture globale
- `app.py` : cœur de l'application Flask, contenant les routes utilisateur et administrateur, les décorateurs d'authentification et de permissions, ainsi que la logique de gestion des œuvres.
- `templates/` : gabarits Jinja2 pour les pages publiques, le tableau de bord et l'interface d'administration.
- `static/` : ressources statiques (feuilles de style, scripts, images importées par les utilisateurs).
- `init_db.py` : script d'initialisation de la base de données SQLite et création du compte super-administrateur par défaut.
- `database.db` : base de données SQLite générée par l'application (peut être supprimée puis régénérée via `init_db.py`).

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
- Virtualenv conseillé pour isoler l'environnement.
- Bibliothèques listées dans `requirements.txt` (Flask 2.2.2 et Werkzeug 2.2.2).

## Installation et lancement
1. **Cloner le dépôt et se placer dans le dossier du projet** :
   ```bash
   git clone <votre-url-git>
   cd BookStorage
   ```
2. **Créer et activer un environnement virtuel** (facultatif mais recommandé) :
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate  # Sous Windows : .venv\Scripts\activate
   ```
3. **Installer les dépendances** :
   ```bash
   pip install -r requirements.txt
   ```
4. **Initialiser la base de données** :
   ```bash
   python init_db.py
   ```
   Ce script crée les tables nécessaires et ajoute, si besoin, un super-administrateur par défaut (`superadmin` / `SuperAdmin!2023`). Changez ce mot de passe immédiatement en production.
5. **Configurer la clé secrète Flask** :
   - Modifiez `app.py` pour définir `app.secret_key` avec une valeur forte issue, par exemple, de `secrets.token_hex(32)`.
   - En production, préférez stocker cette clé dans une variable d'environnement et la charger dynamiquement.
6. **Lancer le serveur de développement** :
   ```bash
   flask --app app run
   ```
   ou
   ```bash
   python app.py
   ```
   L'application est disponible sur `http://127.0.0.1:5000/`.

## Parcours utilisateur
1. **Inscription** (`/register`) : le compte est créé avec `validated=0` et ne peut pas se connecter tant qu'un administrateur ne l'a pas approuvé.
2. **Connexion** (`/login`) : seules les identifiants valides sont autorisés. Les administrateurs et super-admins peuvent se connecter même si `validated=0`.
3. **Tableau de bord** (`/dashboard`) : liste des œuvres de l'utilisateur connecté avec options d'ajout, de filtrage par statut/type, de mise à jour rapide des chapitres et de suppression.
4. **Ajout d'œuvre** (`/add_work`) : formulaire pour saisir titre, lien, statut, type de lecture, chapitre et téléverser une image (formats autorisés : PNG, JPG, JPEG, GIF). Les fichiers sont stockés dans `static/images`.
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
- Les images sont enregistrées dans `static/images`. Assurez-vous que ce dossier existe et dispose des droits en écriture.
- Les avatars utilisateurs sont stockés dans `static/avatars` (ou dans le dossier configuré via `PROFILE_UPLOAD_FOLDER`), avec un nom aléatoire pour éviter les collisions.
- Les noms de fichiers sont sécurisés via `werkzeug.utils.secure_filename`.
- Pensez à mettre en place un mécanisme de nettoyage périodique ou une limite de taille en production.

## Personnalisation et déploiement
- **Styles** : adaptez les feuilles de style dans `static/css/` (`admin.css`, `dashboard.css`, etc.) pour ajuster l'apparence.
- **Sécurité** :
  - Activez `SESSION_COOKIE_SECURE`, utilisez HTTPS et configurez un serveur WSGI (Gunicorn, uWSGI) pour un déploiement réel.
  - Remplacez les messages flash et validations côté client par des validations plus robustes côté serveur au besoin.
- **Base de données** : pour un usage multi-utilisateur à grande échelle, migrez vers un SGBD plus robuste (PostgreSQL, MySQL) et mettez en place des migrations (Alembic, Flask-Migrate).

## Dépannage
- **"Compte non validé" lors de la connexion** : connectez-vous avec un administrateur et approuvez le compte via `/admin/accounts`.
- **Accès refusé aux actions d'administration** : vérifiez que la session contient `is_admin` ou `is_superadmin`. Une reconnexion actualise ces drapeaux.
- **Images manquantes** : assurez-vous que le fichier a été téléchargé sans erreur et que le chemin stocké dans la base pointe vers `static/images/<fichier>`.
- **Base de données corrompue** : supprimez `database.db` et relancez `python init_db.py` (perte des données existantes).

## Tests et maintenance
- Utilisez `python -m compileall app.py` pour vérifier la syntaxe Python rapidement.
- Lancez la suite de tests automatisés avec Pytest :
  ```bash
  pytest
  ```
  Les tests fournis valident la gestion des privilèges d'administration, le blocage des comptes non validés, la rétrocompatibilité des anciens mots de passe `scrypt`, les scénarios complets de mise à jour du profil (changement d'informations, mot de passe, avatar, confidentialité) ainsi que les parcours communautaires (recherche d'utilisateurs, accès aux profils publics/privés et import d'œuvres partagées).
  Des cas vérifient également la conservation du type de lecture lors d'un import et la présence des filtres côté tableau de bord.

## Licence
Indiquez ici la licence souhaitée (par défaut non spécifiée). Ajoutez un fichier `LICENSE` si nécessaire.
