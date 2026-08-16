# 🐴 Zélie et son cheval

Un jeu de concours hippique en three.js, fait pour Zélie.

Tu es **sur** ton cheval : tu vois ses oreilles et sa tête. Choisis la
couleur de ton cheval et l'endroit (la carrière, la plage ou la forêt),
puis fais les trois missions : Premier concours, Concours régional et
Grand Prix.

## Comment jouer

Ouvre **`index.html`** — un double-clic suffit, il n'y a besoin de rien
d'autre (pas d'internet, pas de serveur).

- ⬆️ **galoper** (reste appuyé pour aller plus vite)
- ⬇️ **ralentir**
- ⬅️ ➡️ **choisir ton chemin**, à gauche ou à droite
- **ESPACE** : **sauter au bon moment !**

Sur une tablette, des boutons s'affichent sur l'écran (🐢 ralentir,
🐎 galoper, et le gros bouton **SAUTE !**).

Une barre tombée = 4 fautes, et le parcours sans faute vaut trois
étoiles ⭐⭐⭐. Chaque mission est toujours le même parcours : tu peux
l'apprendre par cœur !

## For developers

`index.html` is generated — edit the sources and rebuild:

```
zelie/
  game.js       the game (readable, commented in French)
  shell.html    the page: menu, HUD, touch controls, styles
  vendor/       three.module.min.js r160, vendored from the npm registry
  build.py      inlines everything into index.html  →  python3 build.py
  index.html    the deliverable: one self-contained file, playable from file://
```

The build rewrites three's module `export{…}` into `window.THREE = {…}`
inside an IIFE so the whole game works as classic scripts from `file://`
(ES modules do not load from `file://` in Chrome).

Every design decision, experiment, and wrong turn of this game's build is
recorded in the repo's own design organ — see `lessons/zelie-design/`:
learn it into a self instance and open `/design` to read the build story.
