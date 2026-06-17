# Assets de marque BookStorage

Dossier central pour remplacer visuellement la marque sans modifier le code.

## Structure

| Dossier / fichier | Usage | Dimensions recommandées |
|-------------------|-------|-------------------------|
| `source/brand-kit.png` | Archive du kit source | — |
| `logos/logo-full.png` | Logo complet (mascottes + texte) | ~266×130 |
| `logos/wordmark.png` | Navbar (texte seul) | ~250×40 |
| `logos/logo-email.png` | Emails HTML (reset mot de passe) | 96×96 |
| `mascots/profile-female.png` | Mascotte (usage futur) | 90×90 |
| `mascots/profile-male.png` | Mascotte (usage futur) | 90×90 |
| `ui/search.png` | Icône recherche mobile | 90×90 |
| `ui/add-book.png` | Bouton ajouter (nav mobile) | 90×90 |
| `ui/settings.png` | Icône réglages mobile | 90×90 |
| `favicon/favicon-16.png` | Favicon navigateur | 16×16 |
| `favicon/favicon-32.png` | Favicon navigateur | 32×32 |
| `favicon/favicon-64.png` | Source favicon / PWA | 64×64 |
| `favicon/favicon.ico` | Compatibilité legacy | 16+32 |
| `banners/hero-webtoon.png` | Landing + pages auth | 1920×823 (21:9) |
| `pwa/icon-192.png` | PWA / Apple touch | 192×192 |
| `pwa/icon-512.png` | PWA install | 512×512 |

## Remplacer les assets

1. Remplacez les PNG en **conservant les mêmes noms de fichiers**.
2. Pour régénérer depuis un nouveau kit source, placez-le dans `source/brand-kit.png` puis exécutez :

   ```bash
   python scripts/crop_brand_kit.py
   ```

   Ajustez les coordonnées dans le script si les proportions du kit changent.

3. Rechargez le navigateur (vider le cache PWA si les icônes ne se mettent pas à jour).

## Configuration email

Dans `config/site.json` :

```json
"mail": {
  "logo_url": "/static/brand/logos/logo-email.png"
}
```

Les SVG ne sont pas affichés dans les clients mail ; utilisez un PNG.
