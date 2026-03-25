# BookStorage (FR)

**BookStorage** est une application web de suivi de lectures personnelles. Suivez vos romans, mangas, webtoons, light novels et plus encore.

_🇬🇧 [English version](./README.md)_

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-3-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green)

---

## Présentation

BookStorage est une application web **auto-hébergée** pour centraliser ce que vous lisez—romans, mangas, webtoons, light novels, etc.—et **suivre votre progression** (à lire, en cours, terminé, abandonné). Vous pouvez ajouter **évaluations et notes de lecture**, consulter des **statistiques** sur vos habitudes, et parcourir les **bibliothèques publiques** des autres lecteurs lorsqu’ils les partagent, tout en gardant la main sur vos données.

L’interface vise un **usage quotidien** : mise en page adaptée, **mode sombre**, **français et anglais**, **PWA** optionnelle, **raccourcis clavier** pour aller vite. Tout s’exécute **chez vous** : une base **SQLite** conserve votre bibliothèque localement, avec **export et import** pour sauvegardes ou migration.

---

## Fonctionnalités

- Bibliothèque multi-formats (romans, mangas, webtoons, light novels…)
- Notes, statistiques, bibliothèques publiques des autres lecteurs
- Mode sombre, interface multilingue (FR/EN), PWA
- Export/import (CSV, JSON), raccourcis clavier

---

## Documentation

| | |
|---|---|
| **Hébergement** (serveur Linux, systemd, configuration, usage quotidien) | [docs/fr/hebergement.md](docs/fr/hebergement.md) |
| **Développement** (dév local, CI/CD, référence `bsctl`, structure du dépôt) | [docs/fr/developpement.md](docs/fr/developpement.md) |

Index (anglais) : [docs/README.md](docs/README.md)

---

## Démarrage rapide

**Prérequis :** Go 1.22+, GCC (CGO pour SQLite).

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
go run ./cmd/bookstorage
```

Ouvrez **http://127.0.0.1:5000**

---

## Production (Linux)

```bash
git clone https://github.com/LGARRABOS/BookStorage.git
cd BookStorage
sudo ./deploy/install.sh
bsctl start
```

Détails : [guide Hébergement](docs/fr/hebergement.md).

---

## Licence

Licence MIT

---

<p align="center">
  Fait avec ❤️ pour les lecteurs
</p>
