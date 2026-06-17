# Assets de marque BookStorage

Dossier central pour remplacer visuellement la marque sans modifier le code.

## Fichiers principaux (vos PNG propres)

| Fichier | Usage |
|---------|--------|
| `logos/logo.png` | Logo navbar (mascottes + livre + texte) |
| `banners/banners.png` | Bannière accueil (landing) |
| `mascots/femmal.png` | Avatar mascotte femme (icône carré) |
| `mascots/male.png` | Avatar mascotte homme (icône carré) |
| `mascots/hero-female.png` | Portrait grand format — accueil, CTA, dashboard vide |
| `mascots/hero-male.png` | Portrait grand format — accueil, CTA, dashboard vide |
| `pwa/app-icon.png` | **Icône PWA** — source pour l'écran d'accueil téléphone |
| `pwa/icon-192.png` | Généré depuis `app-icon.png` |
| `pwa/icon-512.png` | Généré depuis `app-icon.png` |
| `favicon/favicon.png` | Optionnel — favicon navigateur (sinon dérivé de `app-icon.png`) |

Placez vos PNG **finis** (transparence, détourage, etc.) directement dans ces chemins. Aucun script ne retouche les mascottes ni l'icône source.

## Fichiers générés (ne pas éditer à la main)

Après avoir mis à jour `app-icon.png` ou `logo.png`, exécutez :

```bash
python scripts/prepare_brand_assets.py
```

Cela crée ou met à jour uniquement les **dérivés redimensionnés** :

| Fichier | Usage |
|---------|--------|
| `favicon/favicon-16.png` | Onglet navigateur |
| `favicon/favicon-32.png` | Onglet navigateur |
| `favicon/favicon-64.png` | Source interne |
| `favicon/favicon.ico` | Compatibilité legacy |
| `pwa/icon-192.png` | PWA / Apple touch |
| `pwa/icon-512.png` | PWA install |
| `logos/logo-email.png` | Emails HTML (96×96) |

Le script redimensionne aussi `banners/banners.png` si sa largeur dépasse 1920 px.

## Ancien kit source (découpe auto)

`source/brand-kit.png` + `scripts/crop_brand_kit.py` : conservés pour archive, non utilisés si vous fournissez les PNG propres.

## Configuration email

Dans `config/site.json` :

```json
"mail": {
  "logo_url": "/static/brand/logos/logo-email.png"
}
```
