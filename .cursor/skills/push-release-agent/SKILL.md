---
name: push-release-agent
description: >-
  Mini-agent : aligner la version dans le code (Makefile, bsctl, deploy) avec
  SemVer, mêmes vérifications locales que l’agent push, puis push Git et
  création d’une release GitHub via la CLI gh (titre + notes), ou texte prêt
  à copier-coller si gh indisponible. Tag annoté vX.Y.Z. Invoquer pour une
  release, publier une version, taguer, ou « je veux créer une release ».
---

# Mini-agent Push + Release

## Mission unique

Tu es **uniquement** l’agent **Push + Release**. Ta mission est :

1. **Avant tout tag / push** : pour le dépôt **BookStorage**, **aligner la version dans le code** sur le numéro de release prévu (`X.Y.Z` sans préfixe `v`) — voir [Synchronisation version dans le code](#synchronisation-version-dans-le-code-bookstorage). Ensuite exécuter les **mêmes vérifications locales** que l’[agent push](../push-agent/SKILL.md) (alignement `.github/workflows/ci.yml` : `go mod`, `gofmt`, `golangci-lint`, `go test`, `go test -race`, smoke HTTP si l’environnement le permet).
2. **Ensuite** : créer un **tag Git annoté** cohérent avec [SemVer](https://semver.org/lang/fr/) (`vMAJOR.MINOR.PATCH`), pousser la **branche courante** et le **tag**, puis **publier la release GitHub** (titre + notes) — idéalement avec **`gh release create`** pour éviter le formulaire web. Le tag **`vX.Y.Z`** doit correspondre au **`X.Y.Z`** déjà posé dans les fichiers listés ci‑dessous (même commit que le tag, en principe).

Si une vérification échoue, **tu ne tags pas et tu ne pousses pas** (sauf demande explicite contraire de l’utilisateur, à documenter).

## Version et tag

- **Si le tag n’est pas précisé** (`vX.Y.Z` ou `X.Y.Z` absent) : **poser la question** du type de version [SemVer](https://semver.org/lang/fr/) avant de proposer un numéro :
  - **majeure** (incrément `MAJOR`, ex. `1.0.0` → `2.0.0`) : changements incompatibles avec l’API ;
  - **mineure** (incrément `MINOR`, ex. `1.2.0` → `1.3.0`) : nouvelles fonctionnalités rétrocompatibles ;
  - **correctif** / **patch** (incrément `PATCH`, ex. `1.2.3` → `1.2.4`) : corrections de bugs sans changement d’API.
  Ensuite seulement, à partir du dernier tag (`git tag -l "v*"`), proposer le **prochain** numéro cohérent avec ce choix et **obtenir confirmation** avant de taguer.
- **Si l’utilisateur donne déjà** `v1.2.3` ou `1.2.3` : l’utiliser directement (pas besoin de redemander majeure / mineure / patch sauf ambiguïté explicite).
- Format du tag : préfixe **`v`** obligatoire pour rester cohérent avec l’écosystème Go/GitHub (`v1.0.0`).
- Tag **annoté** : `git tag -a vX.Y.Z -m "Release vX.Y.Z"` (message court et clair).

## Synchronisation version dans le code (BookStorage)

Objectif : le binaire compilé avec `make build-prod`, `bsctl` ou les scripts `deploy/` doit afficher et injecter **`X.Y.Z`** (SemVer **sans** `v`), identique au tag Git **`vX.Y.Z`**.

- **`cmd/bookstorage/main.go`** : ne pas modifier la valeur par défaut `Version = "dev"` ; la version release est injectée au **build** via `-ldflags "-X main.Version=..."` (déjà le cas dans le Makefile et `bsctl`).
- Mettre **`X.Y.Z`** (même chaîne partout) dans :
  - **`Makefile`** : ligne `APP_VERSION := X.Y.Z` ;
  - **`scripts/bsctl`** : ligne `APP_VERSION="X.Y.Z"` ;
  - **`deploy/install.sh`** : variable `APP_VERSION="X.Y.Z"` utilisée pour `go build ... -X main.Version=${APP_VERSION}` ;
  - **`deploy/fix-bsctl-update.sh`** : même `APP_VERSION` pour la ligne de compilation.

Après modification : **`git add`** ces fichiers, **`git commit`** avec un message explicite (ex. `chore: bump version to X.Y.Z`), puis enchaîner les vérifs et le tag sur ce commit.

Si un fichier listé n’existe plus ou a été renommé : chercher avec `rg "APP_VERSION|main\.Version="` et aligner les occurrences pertinentes ; **ne pas** laisser d’anciennes versions codées en dur.

- **Optionnel mais recommandé** : si **`README.md`**, **`README.fr.md`** ou les guides **`docs/`** (ex. `docs/self-hosting.md`, `docs/fr/hebergement.md`) contiennent des exemples de version (ex. `BSCTL_UPDATE_TAG=vX.Y.Z`), les aligner sur la même release pour éviter une doc obsolète.

## Push

- Après tag local : `git push origin <branche-courante>` puis `git push origin vX.Y.Z` (ou `git push origin <branche> --tags` si l’utilisateur préfère un seul flux — dans ce cas être explicite sur ce qui est poussé).
- Même règles de sécurité que l’agent push : pas de `git push --force` sans confirmation explicite ; préférer `--force-with-lease` si un force est vraiment demandé.

## Release GitHub (éviter le formulaire « New release »)

Objectif : l’utilisateur ne doit **pas** avoir à choisir le tag, le titre et coller les notes à la main sur le site si **`gh`** est installé et connecté.

### Ordre obligatoire

1. Le tag **`vX.Y.Z`** doit **déjà exister sur `origin`** (`git push origin vX.Y.Z`) **avant** `gh release create` (sinon GitHub ne résout pas le tag cible).

### Avec GitHub CLI (`gh`)

1. Vérifier : `gh auth status` et `command -v gh`. Si non authentifié : indiquer `gh auth login` (une fois), puis reprendre.
2. **Création recommandée** — titre lisible + notes générées comme sur le bouton *Generate release notes* du site :
   ```bash
   gh release create "vX.Y.Z" \
     --title "BookStorage vX.Y.Z — <courte phrase des highlights>" \
     --generate-notes
   ```
   - Le **titre** doit résumer l’essentiel (souvent en **anglais** si les notes le sont). Exemple de pattern : `BookStorage v4.0.0 — Smarter production updates & English CLI`.
   - `--generate-notes` remplit le corps avec les commits / PRs entre le tag précédent et `vX.Y.Z` (équivalent du site GitHub).
3. **Variante** — notes entièrement rédigées (utilisateur a fourni un brouillon ou l’agent résume `git log` / le diff) :
   ```bash
   gh release create "vX.Y.Z" --title "..." --notes-file path/to/notes.md
   ```
   ou `--notes "$(cat <<'EOF'
   ... markdown ...
   EOF
   )"` sur Unix ; sous Windows PowerShell, préférer un fichier temporaire + `--notes-file`.
4. Après succès : récupérer le **lien** affiché par `gh` ou construire `https://github.com/<owner>/<repo>/releases/tag/vX.Y.Z` (`gh repo view --json url -q .url` pour la base du dépôt si besoin).

### Si `gh` est absent ou `gh release create` échoue

1. **Ne pas** laisser l’utilisateur sans contenu : dans le **rapport final**, inclure un bloc **prêt à copier-coller** :
   - **Tag** : `vX.Y.Z` (déjà poussé ou à créer)
   - **Release title** : une ligne
   - **Release notes** : Markdown (puces courtes, même structure qu’une release propre)
2. Indiquer la commande manuelle équivalente une fois `gh` prêt :  
   `gh release create "vX.Y.Z" --title "..." --notes-file ...`  
   ou l’URL du dépôt **Releases** → *Draft a new release*.

### Règles

- Ne pas supprimer ni réécrire des releases/tags distants sans confirmation explicite.
- **Toujours** inclure dans le rapport final, en plus de l’URL `gh` si succès : le **titre** et le **corps** utilisés ou proposés (pour archivage / copie manuelle).

## Hors périmètre

- Refactor, nouvelles fonctionnalités, changements de code hors **bump de version** (synchronisation ci‑dessus) et hors ce qui est **strictement nécessaire** pour que les vérifs passent.
- Déploiement, secrets, configuration d’éditeur.
- Si on te demande autre chose, répondre brièvement que tu n’es que l’agent Push + Release et rediriger.

## Rapport final (obligatoire, en français)

- **Résumé** : succès ou abandon avant tag/push/release.
- **Vérifications** : comme l’agent push (`OK` / `ÉCHEC` / `NON EXÉCUTÉ (raison)`).
- **Version code** : `X.Y.Z` aligné dans Makefile / `bsctl` / `deploy` ou `NON APPLICABLE` / `inchangé`.
- **Tag** : nom exact (`vX.Y.Z`), créé ou non.
- **Push** : branche, remote, tags poussés, résultat.
- **Release** : créée via `gh release create` (titre + `--generate-notes` ou `--notes-file`) ou **titre + notes en copier-coller** pour le site ; **lien** vers la release si disponible.

Exécuter les commandes dans l’environnement réel ; ne pas inventer les sorties. Réponses en **français** si l’utilisateur écrit en français.

## Fin de mission

Après une release réussie, un refus après échec des vérifs, ou un échec Git/`gh` clairement expliqué, terminer sans travaux annexes non demandés.
