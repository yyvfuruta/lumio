'use strict';
const $ = id => document.getElementById(id);
const state = { photos: [], favorite: false, cursor: '', query: '', selected: -1, maxUpload: 32 << 20, uploading: false, loading: false, request: 0 };
const dateFormat = new Intl.DateTimeFormat('pt-BR', { day: 'numeric', month: 'long', year: 'numeric' });
const sizeFormat = n => n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB`;

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { 'X-Lumio-Request': '1', ...(options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }), ...options.headers } });
  if (response.status === 401 && path !== '/api/login') { showLogin(); throw new Error('Sua sessão terminou. Entre novamente.'); }
  if (response.status === 204) return null;
  const data = await response.json();
  if (!response.ok) { const error = new Error(data.error || 'Não foi possível concluir.'); error.status = response.status; throw error; }
  return data;
}

function status(message, error = false) { $('status').textContent = message; $('status').hidden = !message; $('status').classList.toggle('error', error); }
function showLogin() { state.request++; state.photos = []; $('gallery').replaceChildren(); $('viewer').close(); $('viewer-image').removeAttribute('src'); $('app').hidden = true; $('login-screen').hidden = false; $('password').focus(); }
function showApp() { $('login-screen').hidden = true; $('app').hidden = false; $('password').value = ''; }

$('login-form').addEventListener('submit', async event => {
  event.preventDefault(); $('login-button').disabled = true; $('login-error').textContent = '';
  try { await api('/api/login', { method: 'POST', body: JSON.stringify({ password: $('password').value }) }); showApp(); await load(true); }
  catch (error) { $('login-error').textContent = error.message; }
  finally { $('login-button').disabled = false; }
});
$('logout').addEventListener('click', async () => {
  try { await api('/api/logout', { method: 'POST' }); showLogin(); } catch (error) { status(error.message, true); }
});

function setView(favorite) {
  state.favorite = favorite; $('nav-all').classList.toggle('active', !favorite); $('nav-favorites').classList.toggle('active', favorite);
  $('nav-all').setAttribute('aria-current', favorite ? 'false' : 'page'); $('nav-favorites').setAttribute('aria-current', favorite ? 'page' : 'false');
  const title = favorite ? 'Favoritas' : 'Todas as fotos'; $('page-title').replaceChildren(document.createTextNode(title), Object.assign(document.createElement('span'), { textContent: '.' })); $('crumb').textContent = title;
  $('subtitle').textContent = favorite ? 'Aquelas lembranças que merecem um lugar especial.' : 'Um espaço para os seus melhores instantes.';
  load(true);
}
$('nav-all').addEventListener('click', () => setView(false)); $('nav-favorites').addEventListener('click', () => setView(true));
let searchTimer;
$('search').addEventListener('input', () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => { state.query = $('search').value.trim(); load(true); }, 250); });

async function load(reset = false) {
  if (!reset && state.loading) return;
  const request = ++state.request; state.loading = true;
  $('load-more').disabled = true;
  if (reset) { state.photos = []; state.cursor = ''; render(); }
  const params = new URLSearchParams({ q: state.query, favorite: String(state.favorite) });
  if (state.cursor) params.set('cursor', state.cursor);
  try {
    const data = await api(`/api/photos?${params}`); if (request !== state.request) return;
    state.photos.push(...data.photos); state.cursor = data.nextCursor; render();
  } catch (error) { if (request === state.request) status(error.message, true); }
  finally { if (request === state.request) { state.loading = false; $('load-more').disabled = false; $('empty').hidden = state.photos.length !== 0; } }
}
$('load-more').addEventListener('click', () => load());

function render() {
  const gallery = $('gallery'); gallery.replaceChildren();
  const groups = new Map();
  state.photos.forEach((photo, index) => {
    const date = dateFormat.format(new Date(photo.addedAt));
    if (!groups.has(date)) groups.set(date, []); groups.get(date).push({ photo, index });
  });
  for (const [date, items] of groups) {
    const section = document.createElement('section'); section.className = 'date-group';
    const heading = document.createElement('h2'); heading.className = 'date-heading'; heading.textContent = date;
    const count = document.createElement('small'); count.textContent = `${items.length} ${items.length === 1 ? 'foto' : 'fotos'}`; heading.append(count);
    const grid = document.createElement('div'); grid.className = 'photo-grid'; section.append(heading, grid);
    for (const { photo, index } of items) {
      const card = document.createElement('article'); card.className = 'photo';
      const open = document.createElement('button'); open.className = 'photo-open'; open.setAttribute('aria-label', `Abrir ${photo.name}`);
      const img = document.createElement('img'); img.src = `/media/${photo.id}/thumb.jpg`; img.alt = photo.name; img.loading = 'lazy'; img.decoding = 'async'; open.append(img); open.addEventListener('click', () => showPhoto(index));
      const favorite = document.createElement('button'); favorite.className = `photo-fav${photo.favorite ? ' saved' : ''}`; favorite.textContent = photo.favorite ? '♥' : '♡'; favorite.setAttribute('aria-label', photo.favorite ? 'Remover das favoritas' : 'Favoritar'); favorite.setAttribute('aria-pressed', String(photo.favorite)); favorite.addEventListener('click', () => toggleFavorite(photo));
      const name = document.createElement('span'); name.className = 'photo-name'; name.textContent = photo.name; card.append(open, favorite, name); grid.append(card);
    }
    gallery.append(section);
  }
  $('load-more').hidden = !state.cursor; $('empty').hidden = state.photos.length !== 0 || state.loading;
  $('empty-title').textContent = state.query ? 'Nenhuma foto por aqui.' : state.favorite ? 'Um lugar para as suas favoritas.' : 'Toda história começa com uma foto.';
  $('empty-text').textContent = state.query ? 'Tente buscar por outro nome.' : state.favorite ? 'Toque no coração de uma foto para guardá-la aqui.' : 'Adicione suas primeiras lembranças. Elas vão se sentir em casa por aqui.';
  $('empty-upload').hidden = state.favorite || !!state.query;
}

for (const id of ['upload-button', 'empty-upload']) $(id).addEventListener('click', () => $('file-input').click());
$('file-input').addEventListener('change', async () => {
  const files = [...$('file-input').files]; $('file-input').value = '';
  if (!files.length || state.uploading) return;
  state.uploading = true; $('upload-button').disabled = true; $('empty-upload').disabled = true;
  let added = 0, duplicates = 0; const errors = [];
  for (const [index, file] of files.entries()) {
    if ($('app').hidden) break;
    status(`Enviando ${index + 1} de ${files.length}: ${file.name}`);
    if (file.size > state.maxUpload) { errors.push(`${file.name}: limite de ${sizeFormat(state.maxUpload)}.`); continue; }
    try {
      let data;
      for (let attempt = 0; attempt < 4; attempt++) {
        const body = new FormData(); body.append('photo', file);
        try { data = await api('/api/photos', { method: 'POST', body }); break; }
        catch (error) { if (error.status !== 503 || attempt === 3) throw error; await new Promise(resolve => setTimeout(resolve, 3000)); }
      }
      if (data.duplicate) duplicates++; else added++;
    } catch (error) { errors.push(`${file.name}: ${error.message}`); }
  }
  state.uploading = false; $('upload-button').disabled = false; $('empty-upload').disabled = false;
  await load(true);
  status(`${added} ${added === 1 ? 'foto adicionada' : 'fotos adicionadas'}.${duplicates ? ` ${duplicates} já estavam na biblioteca.` : ''}${errors.length ? '\n' + errors.join('\n') : ''}`, errors.length > 0);
});

function showPhoto(index) {
  if (index < 0 || index >= state.photos.length) return;
  state.selected = index; const photo = state.photos[index];
  $('viewer-title').textContent = photo.name; $('viewer-meta').textContent = `${photo.width} × ${photo.height} · ${sizeFormat(photo.size)} · Adicionada em ${dateFormat.format(new Date(photo.addedAt))}`;
  $('viewer-image').src = `/media/${photo.id}/original`; $('viewer-image').alt = photo.name;
  $('viewer-favorite').textContent = photo.favorite ? '♥ Favorita' : '♡ Favoritar'; $('viewer-favorite').setAttribute('aria-pressed', String(photo.favorite));
  $('download').href = `/media/${photo.id}/original?download=1`; $('viewer-prev').disabled = index === 0; $('viewer-next').disabled = index === state.photos.length - 1;
  $('viewer-error').textContent = ''; if (!$('viewer').open) $('viewer').showModal();
}
$('viewer-close').addEventListener('click', () => $('viewer').close());
$('viewer').addEventListener('close', () => { $('viewer-image').removeAttribute('src'); });
$('viewer-prev').addEventListener('click', () => showPhoto(state.selected - 1)); $('viewer-next').addEventListener('click', () => showPhoto(state.selected + 1));
document.addEventListener('keydown', event => { if (!$('viewer').open) return; if (event.key === 'ArrowLeft') showPhoto(state.selected - 1); if (event.key === 'ArrowRight') showPhoto(state.selected + 1); });
async function toggleFavorite(photo) {
  if (!photo || photo.busy) return; photo.busy = true;
  try {
    const updated = await api(`/api/photos/${photo.id}`, { method: 'PATCH', body: JSON.stringify({ favorite: !photo.favorite }) }); Object.assign(photo, updated);
    if (state.favorite && !photo.favorite) { state.photos = state.photos.filter(p => p.id !== photo.id); $('viewer').close(); }
    render(); if ($('viewer').open) showPhoto(state.selected);
  } catch (error) { if ($('viewer').open) $('viewer-error').textContent = error.message; else status(error.message, true); }
  finally { photo.busy = false; }
}
$('viewer-favorite').addEventListener('click', () => toggleFavorite(state.photos[state.selected]));
$('delete').addEventListener('click', async () => {
  const photo = state.photos[state.selected]; if (!photo || !confirm(`Excluir “${photo.name}” permanentemente?`)) return;
  $('delete').disabled = true;
  try { await api(`/api/photos/${photo.id}`, { method: 'DELETE' }); $('viewer').close(); state.photos = state.photos.filter(p => p.id !== photo.id); render(); status('Foto excluída.'); }
  catch (error) { $('viewer-error').textContent = error.message; } finally { $('delete').disabled = false; }
});

(async () => {
  try { const session = await api('/api/session'); state.maxUpload = session.maxUploadBytes; if (session.authenticated) { showApp(); await load(true); } else showLogin(); }
  catch (error) { showLogin(); $('login-error').textContent = 'Não foi possível conectar ao Lumio. Tente recarregar a página.'; }
})();
