// Zélie et son cheval — un jeu de concours hippique en three.js
// Tu es SUR le cheval : tu vois ses oreilles et sa tête. Accélère, ralentis,
// choisis ton chemin à gauche ou à droite, et saute au bon moment !
'use strict';

const $ = (id) => document.getElementById(id);

// ---------------------------------------------------------------- données

const ROBES = [
  { id: 'alezan',   nom: 'Alezan',        corps: 0x9a6a3f, criniere: 0x53331a },
  { id: 'noir',     nom: 'Noir',          corps: 0x2e2b2f, criniere: 0x141216 },
  { id: 'blanc',    nom: 'Blanc',         corps: 0xf0ece2, criniere: 0xd6cfbf },
  { id: 'gris',     nom: 'Gris pommelé',  corps: 0xa6a6b0, criniere: 0x6d6d78 },
  { id: 'palomino', nom: 'Palomino',      corps: 0xd9a95e, criniere: 0xf6e8c8 },
];

const LIEUX = [
  { id: 'carriere', nom: 'La carrière', sol: 0xd8c294, ciel: 0x8ecdf0, brume: 0xcfe6f4, arbres: 8 },
  { id: 'plage',    nom: 'La plage',    sol: 0xe9d7a5, ciel: 0xaee2f7, brume: 0xd9eef8, arbres: 4, mer: true },
  { id: 'foret',    nom: 'La forêt',    sol: 0x7d9b58, ciel: 0x9fc7b8, brume: 0xbcd8c9, arbres: 26 },
];

const MISSIONS = [
  { nom: 'Premier concours',  obstacles: 6,  gap: 48, hauteur: 0.85, pleines: 1, vitesse: 11 },
  { nom: 'Concours régional', obstacles: 8,  gap: 42, hauteur: 1.00, pleines: 2, vitesse: 12 },
  { nom: 'Grand Prix',        obstacles: 10, gap: 38, hauteur: 1.15, pleines: 3, vitesse: 13 },
];

const LANES = [-4, 0, 4];        // les trois chemins possibles
const BORD = 6.5;                // limite gauche/droite (la lice)
const BAR_COLORS = [0xe4572e, 0x2e86de, 0xf6c445, 0x8e44ad, 0x2ecc71];

// pseudo-hasard déterministe : le même parcours à chaque partie de la mission
const rnd = (n) => { const x = Math.sin(n * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); };

// ---------------------------------------------------------------- état

const state = {
  mode: 'menu',                 // menu | course | fini
  robe: ROBES[0], lieu: LIEUX[0], mission: 0,
  vitesse: 0, x: 0, y: 0, vy: 0, z: 0, phase: 0,
  fautes: 0, sautsReussis: 0, chrono: 0,
  lignes: [], finZ: 0, dip: 0, msgT: 0,
};

const input = { accel: false, frein: false, gauche: false, droite: false, saut: false };

// ---------------------------------------------------------------- three

const canvas = $('c');
const renderer = new THREE.WebGLRenderer({ canvas, antialias: true });
renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
const scene = new THREE.Scene();
const camera = new THREE.PerspectiveCamera(70, 2, 0.1, 600);

function resize() {
  renderer.setSize(window.innerWidth, window.innerHeight);
  camera.aspect = window.innerWidth / window.innerHeight;
  camera.updateProjectionMatrix();
}
window.addEventListener('resize', resize);
resize();

const lum = new THREE.HemisphereLight(0xffffff, 0x777766, 1.1);
scene.add(lum);
const soleil = new THREE.DirectionalLight(0xfff3d6, 1.6);
soleil.position.set(30, 60, 20);
scene.add(soleil);

const mat = (color) => new THREE.MeshLambertMaterial({ color });
const boite = (w, h, d, color) => new THREE.Mesh(new THREE.BoxGeometry(w, h, d), mat(color));

// --- le cheval (vu du cavalier : encolure, tête, oreilles, crinière) ------

const cheval = new THREE.Group();          // le groupe qui avance ; la caméra est dedans
scene.add(cheval);
cheval.add(camera);
camera.position.set(0, 2.3, 0.5);
camera.rotation.x = -0.08;

const buste = new THREE.Group();           // encolure + tête, pour l'animation
cheval.add(buste);

let matCorps, matCriniere;
function construireCheval() {
  while (buste.children.length) buste.remove(buste.children[0]);
  matCorps = mat(state.robe.corps);
  matCriniere = mat(state.robe.criniere);

  // le garrot, juste devant la selle, bien en dessous du regard
  const garrot = new THREE.Mesh(new THREE.CylinderGeometry(0.24, 0.28, 0.6, 12), matCorps);
  garrot.rotation.x = Math.PI / 2;
  garrot.position.set(0, 1.28, -0.75);
  buste.add(garrot);

  // l'encolure, qui monte vers l'avant
  const encolure = new THREE.Mesh(new THREE.CylinderGeometry(0.12, 0.24, 1.1, 12), matCorps);
  encolure.position.set(0, 1.47, -1.45);
  encolure.rotation.x = Math.PI / 2 - 0.42;
  buste.add(encolure);

  // la tête, penchée vers l'avant — on la voit par-dessus, entre les oreilles
  const tete = new THREE.Group();
  tete.position.set(0, 1.66, -1.95);
  tete.rotation.x = -0.85;
  buste.add(tete);
  const crane = boite(0.21, 0.23, 0.36, state.robe.corps);
  crane.material = matCorps;
  tete.add(crane);
  const chanfrein = boite(0.15, 0.17, 0.32, state.robe.corps);
  chanfrein.material = matCorps;
  chanfrein.position.set(0, -0.04, -0.30);
  tete.add(chanfrein);

  // les deux oreilles !
  for (const cote of [-1, 1]) {
    const oreille = new THREE.Mesh(new THREE.ConeGeometry(0.05, 0.18, 8), matCorps);
    oreille.position.set(cote * 0.09, 0.20, 0.10);
    oreille.rotation.z = cote * -0.15;
    oreille.name = 'oreille' + (cote > 0 ? 'D' : 'G');
    tete.add(oreille);
  }

  // le toupet, entre les oreilles
  const toupet = boite(0.10, 0.06, 0.16, state.robe.criniere);
  toupet.material = matCriniere;
  toupet.position.set(0, 0.16, -0.02);
  tete.add(toupet);

  // la crinière, le long de l'encolure
  for (let i = 0; i < 5; i++) {
    const meche = boite(0.08, 0.18, 0.15, state.robe.criniere);
    meche.material = matCriniere;
    const t = i / 4;
    meche.position.set(0, 1.42 + t * 0.30 + 0.14, -1.02 - t * 0.72);
    meche.rotation.x = -0.42;
    buste.add(meche);
  }

  // les rênes, des mains du cavalier jusqu'à la tête
  const matRene = mat(0x4a3324);
  for (const cote of [-1, 1]) {
    const depart = new THREE.Vector3(cote * 0.16, 1.45, -0.35);
    const arrivee = new THREE.Vector3(cote * 0.11, 1.66, -1.85);
    const dir = arrivee.clone().sub(depart);
    const rene = new THREE.Mesh(
      new THREE.CylinderGeometry(0.012, 0.012, dir.length(), 5), matRene);
    rene.position.copy(depart).add(arrivee).multiplyScalar(0.5);
    rene.quaternion.setFromUnitVectors(new THREE.Vector3(0, 1, 0), dir.normalize());
    buste.add(rene);
  }
}

// --- le monde -------------------------------------------------------------

const monde = new THREE.Group();
scene.add(monde);

function arbre(x, z, grand) {
  const g = new THREE.Group();
  const tronc = new THREE.Mesh(new THREE.CylinderGeometry(0.14, 0.2, grand ? 2.6 : 1.6, 8), mat(0x7a5230));
  tronc.position.y = (grand ? 2.6 : 1.6) / 2;
  g.add(tronc);
  const feuilles = new THREE.Mesh(new THREE.ConeGeometry(grand ? 1.5 : 1.1, grand ? 3.2 : 2.4, 10), mat(0x3f7d3a));
  feuilles.position.y = grand ? 3.9 : 2.6;
  g.add(feuilles);
  g.position.set(x, 0, z);
  return g;
}

function banniere(z, texte, couleur) {
  const g = new THREE.Group();
  for (const cote of [-1, 1]) {
    const poteau = new THREE.Mesh(new THREE.CylinderGeometry(0.09, 0.09, 3.4, 8), mat(0xffffff));
    poteau.position.set(cote * (BORD + 0.8), 1.7, 0);
    g.add(poteau);
  }
  const toile = boite((BORD + 0.8) * 2, 0.7, 0.05, couleur);
  toile.position.set(0, 3.1, 0);
  g.add(toile);
  const cv = document.createElement('canvas');
  cv.width = 512; cv.height = 48;
  const cx = cv.getContext('2d');
  cx.fillStyle = '#ffffff'; cx.font = 'bold 40px sans-serif'; cx.textAlign = 'center';
  cx.fillText(texte, 256, 40);
  const tex = new THREE.CanvasTexture(cv);
  const panneau = new THREE.Mesh(
    new THREE.PlaneGeometry(10, 0.94),
    new THREE.MeshBasicMaterial({ map: tex, transparent: true }));
  panneau.position.set(0, 3.1, 0.06);
  g.add(panneau);
  g.position.z = z;
  return g;
}

function construireMonde() {
  while (monde.children.length) monde.remove(monde.children[0]);
  const lieu = state.lieu;
  const m = MISSIONS[state.mission];
  scene.background = new THREE.Color(lieu.ciel);
  scene.fog = new THREE.Fog(lieu.brume, 60, 320);

  const longueur = 80 + m.obstacles * m.gap + 120;
  state.finZ = -(40 + m.obstacles * m.gap + 30);

  // le sol
  const sol = new THREE.Mesh(new THREE.PlaneGeometry(400, longueur + 400), mat(lieu.sol));
  sol.rotation.x = -Math.PI / 2;
  sol.position.z = -longueur / 2 + 60;
  monde.add(sol);

  // la mer, à droite de la plage
  if (lieu.mer) {
    const mer = new THREE.Mesh(new THREE.PlaneGeometry(300, longueur + 400),
      new THREE.MeshLambertMaterial({ color: 0x2f8fce }));
    mer.rotation.x = -Math.PI / 2;
    mer.position.set(168, 0.04, -longueur / 2 + 60);
    monde.add(mer);
  }

  // la lice (la barrière blanche de la carrière), des deux côtés
  const railGeo = new THREE.BoxGeometry(0.08, 0.1, longueur);
  for (const cote of [-1, 1]) {
    for (const h of [0.6, 1.1]) {
      const rail = new THREE.Mesh(railGeo, mat(0xf5f5f0));
      rail.position.set(cote * (BORD + 1.0), h, -longueur / 2 + 60);
      monde.add(rail);
    }
    for (let z = 20; z > -longueur + 60; z -= 8) {
      const poteau = new THREE.Mesh(new THREE.CylinderGeometry(0.07, 0.07, 1.3, 6), mat(0xf5f5f0));
      poteau.position.set(cote * (BORD + 1.0), 0.65, z);
      monde.add(poteau);
    }
  }

  // les arbres du décor
  for (let i = 0; i < lieu.arbres * 14; i++) {
    const cote = i % 2 ? 1 : -1;
    if (lieu.mer && cote > 0) continue;   // pas d'arbres dans la mer
    const x = cote * (12 + rnd(i * 3 + 1) * 26);
    const z = 30 - rnd(i * 3 + 2) * longueur;
    monde.add(arbre(x, z, rnd(i * 3 + 3) > 0.5));
  }

  monde.add(banniere(-8, 'DÉPART', 0x2e86de));
  monde.add(banniere(state.finZ, 'ARRIVÉE', 0xe4572e));

  // les obstacles : chaque ligne bloque 1, 2 ou 3 chemins
  state.lignes = [];
  for (let i = 0; i < m.obstacles; i++) {
    const z = -40 - i * m.gap;
    const graine = state.mission * 100 + i;
    const ordre = [0, 1, 2].sort((a, b) => rnd(graine * 7 + a) - rnd(graine * 7 + b));
    const pleines = Math.min(3, m.pleines + (rnd(graine * 13) > 0.6 ? 1 : 0));
    const sauts = [];
    for (let k = 0; k < pleines; k++) {
      const lane = ordre[k];
      const h = m.hauteur + (rnd(graine * 17 + lane) - 0.5) * 0.2;
      sauts.push({ lane, h, tombee: false, groupe: construireObstacle(LANES[lane], z, h, graine + k) });
    }
    state.lignes.push({ z, sauts, jugee: false });
  }
}

function construireObstacle(x, z, h, graine) {
  const g = new THREE.Group();
  for (const cote of [-1, 1]) {
    const poteau = boite(0.16, h + 0.5, 0.16, 0xffffff);
    poteau.position.set(cote * 1.7, (h + 0.5) / 2, 0);
    g.add(poteau);
    const drapeau = new THREE.Mesh(new THREE.ConeGeometry(0.09, 0.25, 6),
      mat(cote > 0 ? 0xe4572e : 0xffffff));
    drapeau.position.set(cote * 1.7, h + 0.62, 0);
    g.add(drapeau);
  }
  const barre = new THREE.Mesh(new THREE.CylinderGeometry(0.07, 0.07, 3.3, 10),
    mat(BAR_COLORS[Math.floor(rnd(graine * 23) * BAR_COLORS.length)]));
  barre.rotation.z = Math.PI / 2;
  barre.position.set(0, h, 0);
  barre.name = 'barre';
  g.add(barre);
  const barreBasse = new THREE.Mesh(new THREE.CylinderGeometry(0.06, 0.06, 3.3, 10), mat(0xffffff));
  barreBasse.rotation.z = Math.PI / 2;
  barreBasse.position.set(0, h * 0.55, 0);
  g.add(barreBasse);
  g.position.set(x, 0, z);
  monde.add(g);
  return g;
}

// ---------------------------------------------------------------- sons

let audio = null;
function sons() {
  if (!audio) {
    audio = new (window.AudioContext || window.webkitAudioContext)();
    const n = audio.sampleRate * 0.06;
    const buf = audio.createBuffer(1, n, audio.sampleRate);
    const d = buf.getChannelData(0);
    for (let i = 0; i < n; i++) d[i] = (Math.random() * 2 - 1) * (1 - i / n);
    audio.sabot = buf;
  }
  if (audio.state === 'suspended') audio.resume();
}
function sabot(force) {
  if (!audio) return;
  const src = audio.createBufferSource();
  src.buffer = audio.sabot;
  const f = audio.createBiquadFilter();
  f.type = 'lowpass'; f.frequency.value = 400 + force * 500;
  const g = audio.createGain();
  g.gain.value = 0.10 + force * 0.15;
  src.connect(f); f.connect(g); g.connect(audio.destination);
  src.start();
}
function note(freq, t0, dur, vol) {
  if (!audio) return;
  const o = audio.createOscillator(); o.frequency.value = freq;
  const g = audio.createGain();
  g.gain.setValueAtTime(vol, audio.currentTime + t0);
  g.gain.exponentialRampToValueAtTime(0.001, audio.currentTime + t0 + dur);
  o.connect(g); g.connect(audio.destination);
  o.start(audio.currentTime + t0); o.stop(audio.currentTime + t0 + dur);
}
const dingBravo = () => { note(660, 0, 0.18, 0.2); note(880, 0.12, 0.25, 0.2); };
const boumBarre = () => { note(95, 0, 0.3, 0.4); };

// ---------------------------------------------------------------- partie

function demarrer(mission) {
  state.mission = mission;
  state.mode = 'course';
  state.vitesse = 0; state.x = 0; state.y = 0; state.vy = 0; state.z = 6;
  state.fautes = 0; state.sautsReussis = 0; state.chrono = 0; state.phase = 0;
  construireCheval();
  construireMonde();
  $('menu').hidden = true;
  $('resultats').hidden = true;
  $('hud').hidden = false;
  $('mission-nom').textContent = 'Mission ' + (mission + 1) + ' — ' + MISSIONS[mission].nom;
  majHud();
  message('En avant ! ⬆️ pour galoper', 2.5);
  sons();
}

function message(texte, duree) {
  const el = $('message');
  el.textContent = texte;
  el.classList.add('visible');
  state.msgT = duree;
}

function majHud() {
  const m = MISSIONS[state.mission];
  $('fautes').textContent = state.fautes;
  $('sauts').textContent = state.sautsReussis + ' / ' + m.obstacles;
}

function finDeCourse() {
  state.mode = 'fini';
  const etoiles = state.fautes === 0 ? 3 : state.fautes <= 4 ? 2 : 1;
  $('hud').hidden = true;
  $('resultats').hidden = false;
  $('res-titre').textContent = state.fautes === 0 ? 'Sans faute ! 🏆' : 'Concours terminé ! 🏁';
  $('res-etoiles').textContent = '⭐'.repeat(etoiles) + '☆'.repeat(3 - etoiles);
  $('res-detail').textContent =
    state.sautsReussis + ' saut' + (state.sautsReussis > 1 ? 's' : '') + ' réussi' + (state.sautsReussis > 1 ? 's' : '') +
    ' · ' + state.fautes + ' faute' + (state.fautes > 1 ? 's' : '') +
    ' · ' + state.chrono.toFixed(1) + ' s';
  $('btn-suivante').hidden = state.mission >= MISSIONS.length - 1;
  dingBravo();
}

// --- la boucle -------------------------------------------------------------

let tPrec = performance.now();
let sabotPrec = 0;

function boucle(t) {
  requestAnimationFrame(boucle);
  const dt = Math.min((t - tPrec) / 1000, 0.05);
  tPrec = t;

  if (state.mode === 'course') {
    const m = MISSIONS[state.mission];
    state.chrono += dt;
    $('chrono').textContent = state.chrono.toFixed(1) + ' s';

    // accélérer / ralentir
    if (input.accel) state.vitesse += 5.5 * dt;
    else if (input.frein) state.vitesse -= 8 * dt;
    else state.vitesse -= 0.8 * dt;
    state.vitesse = Math.max(0, Math.min(m.vitesse, state.vitesse));

    // choisir son chemin, gauche ou droite
    const lat = (input.droite ? 1 : 0) - (input.gauche ? 1 : 0);
    state.x += lat * (2.2 + state.vitesse * 0.28) * dt;
    state.x = Math.max(-BORD, Math.min(BORD, state.x));

    // sauter au bon moment
    const auSol = state.y <= 0.001;
    if (input.saut && auSol && state.vitesse > 1) {
      state.vy = 5.6;
      state.y = 0.002;
    }
    if (state.y > 0 || state.vy > 0) {
      state.y += state.vy * dt;
      state.vy -= 9.8 * dt;
      if (state.y <= 0) { state.y = 0; state.vy = 0; state.dip = 0.14; }
    }

    // avancer
    const zAvant = state.z;
    state.z -= state.vitesse * dt;

    // juger chaque ligne d'obstacles quand on la franchit
    for (const ligne of state.lignes) {
      if (ligne.jugee || !(zAvant > ligne.z && state.z <= ligne.z)) continue;
      ligne.jugee = true;
      const saut = ligne.sauts.find((s) => Math.abs(state.x - LANES[s.lane]) < 2.0);
      if (!saut) continue;                          // passé par un chemin libre
      if (state.y > saut.h - 0.35) {                // les jambes se replient !
        state.sautsReussis += 1;
        message(['Bravo ! 🎉', 'Super saut !', 'Magnifique !'][state.sautsReussis % 3], 1.4);
        dingBravo();
      } else {
        saut.tombee = true;
        state.fautes += 4;
        state.vitesse *= 0.45;
        message('Oh non, la barre est tombée… 4 fautes', 1.8);
        boumBarre();
      }
      majHud();
    }

    // les barres tombées roulent au sol
    for (const ligne of state.lignes) {
      for (const s of ligne.sauts) {
        if (!s.tombee) continue;
        const barre = s.groupe.getObjectByName('barre');
        if (barre.position.y > 0.08) {
          barre.position.y -= 2.5 * dt;
          barre.position.z += 1.2 * dt;
          barre.rotation.x += 3 * dt;
        }
      }
    }

    // l'arrivée
    if (state.z <= state.finZ) { finDeCourse(); }

    // le galop : tout le cheval ondule, la caméra suit
    state.phase += state.vitesse * dt * 1.35;
    const ratio = state.vitesse / m.vitesse;
    const bob = state.y > 0 ? 0 : Math.abs(Math.sin(state.phase)) * 0.11 * ratio;
    state.dip = Math.max(0, state.dip - dt * 0.5);
    cheval.position.set(state.x, state.y + bob - state.dip, state.z);
    cheval.rotation.z = -lat * 0.03;
    camera.rotation.z = -lat * 0.02;

    // l'encolure s'étire pendant le saut, se balance au galop
    buste.rotation.x = state.y > 0 ? -0.22 : Math.sin(state.phase * 2) * 0.045 * ratio;

    // les oreilles bougent toutes seules de temps en temps
    const oG = buste.getObjectByName('oreilleG');
    const oD = buste.getObjectByName('oreilleD');
    if (oG) {
      oG.rotation.x = Math.sin(t * 0.0021) > 0.995 ? 0.5 : 0;
      oD.rotation.x = Math.sin(t * 0.0017 + 3) > 0.995 ? 0.5 : 0;
    }

    // le bruit des sabots
    if (state.y <= 0 && state.vitesse > 1.5) {
      const cycle = Math.floor(state.phase / Math.PI);
      if (cycle !== sabotPrec) { sabotPrec = cycle; sabot(ratio); }
    }

    // le message s'efface tout seul
    if (state.msgT > 0) {
      state.msgT -= dt;
      if (state.msgT <= 0) $('message').classList.remove('visible');
    }
  }

  renderer.render(scene, camera);
}
requestAnimationFrame(boucle);

// ---------------------------------------------------------------- entrées

const TOUCHES = {
  ArrowUp: 'accel', ArrowDown: 'frein',
  ArrowLeft: 'gauche', ArrowRight: 'droite',
  ' ': 'saut', Spacebar: 'saut',
};
window.addEventListener('keydown', (e) => {
  const quoi = TOUCHES[e.key];
  if (quoi) { input[quoi] = true; e.preventDefault(); }
});
window.addEventListener('keyup', (e) => {
  const quoi = TOUCHES[e.key];
  if (quoi) input[quoi] = false;
});

// boutons tactiles (tablette / téléphone)
for (const [id, quoi] of [
  ['t-gauche', 'gauche'], ['t-droite', 'droite'],
  ['t-plus', 'accel'], ['t-moins', 'frein'], ['t-saut', 'saut'],
]) {
  const el = $(id);
  const on = (e) => { e.preventDefault(); input[quoi] = true; sons(); };
  const off = (e) => { e.preventDefault(); input[quoi] = false; };
  el.addEventListener('pointerdown', on);
  el.addEventListener('pointerup', off);
  el.addEventListener('pointercancel', off);
  el.addEventListener('pointerleave', off);
}
if (!('ontouchstart' in window)) $('tactile').classList.add('clavier');

// ---------------------------------------------------------------- menu

function construireMenu() {
  const robes = $('choix-robe');
  for (const robe of ROBES) {
    const b = document.createElement('button');
    b.className = 'pastille';
    b.style.background = '#' + robe.corps.toString(16).padStart(6, '0');
    b.title = robe.nom;
    b.addEventListener('click', () => {
      state.robe = robe;
      $('robe-nom').textContent = robe.nom;
      for (const x of robes.children) x.classList.remove('choisi');
      b.classList.add('choisi');
    });
    robes.appendChild(b);
  }
  robes.children[0].classList.add('choisi');
  $('robe-nom').textContent = ROBES[0].nom;

  const lieux = $('choix-lieu');
  for (const lieu of LIEUX) {
    const b = document.createElement('button');
    b.className = 'lieu';
    b.textContent = lieu.nom;
    b.addEventListener('click', () => {
      state.lieu = lieu;
      for (const x of lieux.children) x.classList.remove('choisi');
      b.classList.add('choisi');
    });
    lieux.appendChild(b);
  }
  lieux.children[0].classList.add('choisi');

  const missions = $('choix-mission');
  MISSIONS.forEach((m, i) => {
    const b = document.createElement('button');
    b.className = 'grand';
    b.textContent = 'Mission ' + (i + 1) + ' · ' + m.nom;
    b.addEventListener('click', () => demarrer(i));
    missions.appendChild(b);
  });
}
construireMenu();

$('btn-rejouer').addEventListener('click', () => demarrer(state.mission));
$('btn-suivante').addEventListener('click', () => demarrer(state.mission + 1));
$('btn-menu').addEventListener('click', () => {
  state.mode = 'menu';
  $('resultats').hidden = true;
  $('hud').hidden = true;
  $('menu').hidden = false;
});

// une jolie scène derrière le menu
construireCheval();
construireMonde();
cheval.position.set(0, 0, 6);
