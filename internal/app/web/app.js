'use strict';
const $ = id => document.getElementById(id);
const state = { photos: [], favorite: false, cursor: '', query: '', selected: -1, maxUpload: 32 << 20, uploading: false, loading: false, request: 0, view: 'photos', period: '', timelineRequest: 0 };
const dateFormat = new Intl.DateTimeFormat('en-CA', { day: '2-digit', month: '2-digit', year: 'numeric' });
const sizeFormat = n => n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.ceil(n / 1024)} KB`;

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, headers: { 'X-Lumio-Request': '1', ...(options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }), ...options.headers } });
  if (response.status === 401 && path !== '/api/v1/sessions') { showLogin(); throw new Error('Your session expired. Sign in again.'); }
  if (response.status === 204) return null;
  const data = await response.json();
  if (!response.ok) { const error = new Error(data.error || 'Could not complete the request.'); error.status = response.status; throw error; }
  return data;
}

function status(message, error = false) { $('status').textContent = message; $('status').hidden = !message; $('status').classList.toggle('error', error); }
function showLogin() { state.request++; state.photos = []; $('gallery').replaceChildren(); $('viewer').close(); $('viewer-image').removeAttribute('src'); $('app').hidden = true; $('login-screen').hidden = false; $('password').focus(); }
function showApp() { $('login-screen').hidden = true; $('app').hidden = false; $('password').value = ''; }

$('login-form').addEventListener('submit', async event => {
  event.preventDefault(); $('login-button').disabled = true; $('login-error').textContent = '';
  try { await api('/api/v1/sessions', { method: 'POST', body: JSON.stringify({ password: $('password').value }) }); showApp(); await load(true); }
  catch (error) { $('login-error').textContent = error.message; }
  finally { $('login-button').disabled = false; }
});
$('logout').addEventListener('click', async () => {
  try { await api('/api/v1/sessions/current', { method: 'DELETE' }); showLogin(); } catch (error) { status(error.message, true); }
});

function setView(favorite) {
  state.view = 'photos'; $('photo-panel').hidden = false; $('photo-path').hidden = false; $('status-panel').hidden = true; $('nav-status').classList.remove('active'); $('nav-status').removeAttribute('aria-current'); $('search').hidden = false;
  state.favorite = favorite; $('nav-all').classList.toggle('active', !favorite); $('nav-favorites').classList.toggle('active', favorite);
  $('nav-all').setAttribute('aria-current', favorite ? 'false' : 'page'); $('nav-favorites').setAttribute('aria-current', favorite ? 'page' : 'false');
  const title = favorite ? 'favorites' : 'photos'; $('page-title').textContent = title; $('crumb').textContent = title;
  load(true);
}
$('nav-all').addEventListener('click', () => setView(false)); $('nav-favorites').addEventListener('click', () => setView(true));
let searchTimer;
$('search').addEventListener('input', () => { clearTimeout(searchTimer); searchTimer = setTimeout(() => { state.query = $('search').value.trim(); load(true); }, 250); });

async function load(reset = false) {
  if (!reset && state.loading) return;
  const request = ++state.request; state.loading = true;
  $('load-more').disabled = true;
  if (reset) { state.photos = []; state.cursor = ''; render(); loadTimeline(); }
  const params = new URLSearchParams({ q: state.query, favorite: String(state.favorite) });
  if (state.cursor) params.set('cursor', state.cursor);
  if (state.period) { const range = periodRange(state.period); params.set('from', range.from); params.set('until', range.until); }
  try {
    const data = await api(`/api/v1/photos?${params}`); if (request !== state.request) return;
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
    const grid = document.createElement('div'); grid.className = 'photo-grid'; section.append(heading, grid);
    for (const { photo, index } of items) {
      const card = document.createElement('article'); card.className = 'photo';
      const open = document.createElement('button'); open.className = 'photo-open'; open.setAttribute('aria-label', `Open ${photo.name}`);
      const img = document.createElement('img'); img.src = `/api/v1/photos/${photo.id}/thumbnail`; img.alt = photo.name; img.loading = 'lazy'; img.decoding = 'async'; open.append(img); open.addEventListener('click', () => showPhoto(index));
      const favorite = document.createElement('button'); favorite.className = `photo-fav${photo.favorite ? ' saved' : ''}`; favorite.textContent = photo.favorite ? '★' : '☆'; favorite.setAttribute('aria-label', photo.favorite ? 'Remove from favorites' : 'Add to favorites'); favorite.setAttribute('aria-pressed', String(photo.favorite)); favorite.addEventListener('click', () => toggleFavorite(photo));
      const name = document.createElement('span'); name.className = 'photo-name'; name.textContent = photo.name; card.append(open, favorite, name); grid.append(card);
    }
    gallery.append(section);
  }
  $('crumb').textContent = `${state.favorite ? 'favorites' : 'photos'}${state.period ? ' / ' + state.period : ''}`;
  $('load-more').hidden = !state.cursor; $('empty').hidden = state.photos.length !== 0 || state.loading;
  $('empty-title').textContent = state.query ? 'No matching photos.' : state.favorite ? 'No favorites yet.' : state.period ? 'No photos in this period.' : 'No photos yet.';
  $('empty-text').textContent = state.query ? 'Try a different filename.' : state.favorite ? 'Use the star on a photo to save it here.' : state.period ? 'Choose another date or select all.' : 'Upload a few photos to get started.';
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
    status(`Uploading ${index + 1} of ${files.length}: ${file.name}`);
    if (file.size > state.maxUpload) { errors.push(`${file.name}: limit is ${sizeFormat(state.maxUpload)}.`); continue; }
    try {
      let data;
      for (let attempt = 0; attempt < 4; attempt++) {
        const body = new FormData(); body.append('photo', file);
        try { data = await api('/api/v1/photos', { method: 'POST', body }); break; }
        catch (error) { if (error.status !== 503 || attempt === 3) throw error; await new Promise(resolve => setTimeout(resolve, 3000)); }
      }
      if (data.duplicate) duplicates++; else added++;
    } catch (error) { errors.push(`${file.name}: ${error.message}`); }
  }
  state.uploading = false; $('upload-button').disabled = false; $('empty-upload').disabled = false;
  await load(true);
  status(`${added} ${added === 1 ? 'photo added' : 'photos added'}.${duplicates ? ` ${duplicates} already in the library.` : ''}${errors.length ? '\n' + errors.join('\n') : ''}`, errors.length > 0);
});

function showPhoto(index) {
  if (index < 0 || index >= state.photos.length) return;
  $('viewer-position').textContent = `${index + 1} / ${state.photos.length}`;
  state.selected = index; const photo = state.photos[index];
  $('viewer-title').textContent = photo.name; $('viewer-meta').textContent = `${photo.width} × ${photo.height} · ${sizeFormat(photo.size)} · Added ${dateFormat.format(new Date(photo.addedAt))}`;
  $('viewer-image').src = `/api/v1/photos/${photo.id}/original`; $('viewer-image').alt = photo.name;
  $('viewer-favorite').textContent = photo.favorite ? '★ starred' : '☆ favorite'; $('viewer-favorite').setAttribute('aria-pressed', String(photo.favorite));
  $('download').href = `/api/v1/photos/${photo.id}/original?download=1`; $('viewer-menu').open = false; $('viewer').setAttribute('aria-label', photo.name);
  $('viewer-error').textContent = ''; if (!$('viewer').open) $('viewer').showModal(); $('viewer-image').focus({ preventScroll: true });
}
$('viewer-close').addEventListener('click', () => $('viewer').close());
$('viewer').addEventListener('close', () => { $('viewer-image').removeAttribute('src'); });


async function toggleFavorite(photo) {
  if (!photo || photo.busy) return; photo.busy = true;
  try {
    const updated = await api(`/api/v1/photos/${photo.id}`, { method: 'PATCH', body: JSON.stringify({ favorite: !photo.favorite }) }); Object.assign(photo, updated);
    if (state.favorite && !photo.favorite) { state.photos = state.photos.filter(p => p.id !== photo.id); $('viewer').close(); }
    render(); if ($('viewer').open) showPhoto(state.selected);
  } catch (error) { if ($('viewer').open) { $('viewer-menu').open = true; $('viewer-error').textContent = error.message; } else status(error.message, true); }
  finally { photo.busy = false; }
}
$('viewer-favorite').addEventListener('click', () => toggleFavorite(state.photos[state.selected]));
$('delete').addEventListener('click', async () => {
  const photo = state.photos[state.selected]; if (!photo || !confirm(`Permanently delete “${photo.name}”?`)) return;
  $('delete').disabled = true;
  try { await api(`/api/v1/photos/${photo.id}`, { method: 'DELETE' }); $('viewer').close(); state.photos = state.photos.filter(p => p.id !== photo.id); render(); status('Photo deleted.'); }
  catch (error) { $('viewer-error').textContent = error.message; } finally { $('delete').disabled = false; }
});

(async () => {
  try { const session = await api('/api/v1/sessions/current'); state.maxUpload = session.maxUploadBytes; if (session.authenticated) { showApp(); await load(true); } else showLogin(); }
  catch (error) { showLogin(); $('login-error').textContent = 'Could not connect to Lumio. Try reloading.'; }
})();

$('help-button').addEventListener('click', () => $('help').showModal());
$('help-close').addEventListener('click', () => $('help').close());
document.addEventListener('keydown', event => {
  if (event.ctrlKey || event.metaKey || event.altKey || event.target.matches('input, textarea, select, [contenteditable]')) return;
  if ($('app').hidden || $('help').open) return;
  if ($('viewer').open) {
    if (['h', 'ArrowLeft'].includes(event.key)) { event.preventDefault(); showPhoto(state.selected - 1); }
    if (['l', 'ArrowRight'].includes(event.key)) { event.preventDefault(); showPhoto(state.selected + 1); }
    if (event.key === ' ' && !event.target.closest('button, a, summary')) { event.preventDefault(); toggleFavorite(state.photos[state.selected]); }
    return;
  }
  if (event.key === '/') { event.preventDefault(); if (state.view === 'status') setView(state.favorite); $('search').focus(); return; }
  if (event.key === 'u') { event.preventDefault(); if (!state.uploading) $('file-input').click(); return; }
  if (event.key === 'f') { event.preventDefault(); setView(!state.favorite); return; }
  if (event.key === '?') { event.preventDefault(); $('help').showModal(); return; }
  if (state.view === 'status') return;
  const cards = [...document.querySelectorAll('.photo-open')];
  if (!cards.length || !['h', 'j', 'k', 'l', 'ArrowLeft', 'ArrowDown', 'ArrowUp', 'ArrowRight'].includes(event.key)) return;
  event.preventDefault();
  const current = cards.indexOf(document.activeElement);
  if (current < 0) { cards[0].focus(); return; }
  const columns = getComputedStyle(cards[current].parentElement.parentElement).gridTemplateColumns.split(' ').length;
  const step = { h: -1, l: 1, j: columns, k: -columns, ArrowLeft: -1, ArrowRight: 1, ArrowDown: columns, ArrowUp: -columns }[event.key];
  cards[Math.max(0, Math.min(cards.length - 1, current + step))].focus();
});
$('search').addEventListener('keydown', event => {
  if (event.key === 'Escape') { $('search').value = ''; state.query = ''; clearTimeout(searchTimer); $('search').blur(); load(true); }
});

const byteSize = n => {
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let index = 0;
  while (n >= 1024 && index < units.length - 1) { n /= 1024; index++; }
  return `${n.toFixed(index ? 1 : 0)} ${units[index]}`;
};
async function refreshStatus() {
  $('refresh-status').disabled = true; $('status-error').textContent = '';
  try {
    const data = await api('/api/v1/status');
    const values = { photos: data.photos, favorites: data.favorites, originals: byteSize(data.originalBytes), thumbnails: byteSize(data.thumbnailBytes), library: byteSize(data.originalBytes + data.thumbnailBytes), pending: data.pendingDeletions, database: data.database, storage: data.storage, access: data.storageAccessible ? 'available' : 'unavailable' };
    for (const [key, value] of Object.entries(values)) $(`usage-${key}`).textContent = value;
    document.querySelectorAll('.disk-row').forEach(row => { row.hidden = !data.disk; });
    if (data.disk) {
      $('usage-total').textContent = byteSize(data.disk.total);
      $('usage-used').textContent = `${byteSize(data.disk.used)} (${data.disk.total ? (data.disk.used / data.disk.total * 100).toFixed(1) : '0'}%)`;
      $('usage-available').textContent = byteSize(data.disk.available);
    }
    $('disk-note').textContent = data.disk ? 'Volume usage includes everything on the filesystem, not just this library.' : 'Filesystem capacity is not reported for this storage provider or platform.';
    $('status-checked').textContent = `checked ${new Date(data.checkedAt).toLocaleTimeString('en-GB')}`;
  } catch (error) { $('status-error').textContent = error.message; $('status-checked').textContent = 'status unavailable'; }
  finally { $('refresh-status').disabled = false; }
}
$('nav-status').addEventListener('click', () => {
  state.view = 'status'; $('photo-panel').hidden = true; $('photo-path').hidden = true; $('status-panel').hidden = false; $('search').hidden = true;
  for (const id of ['nav-all', 'nav-favorites']) { $(id).classList.remove('active'); $(id).removeAttribute('aria-current'); }
  $('nav-status').classList.add('active'); $('nav-status').setAttribute('aria-current', 'page');
  refreshStatus();
});
$('refresh-status').addEventListener('click', refreshStatus);
setInterval(() => { if (state.view === 'status' && !$('app').hidden && document.visibilityState === 'visible') refreshStatus(); }, 30000);
let swipeStart;
$('viewer-image').addEventListener('touchstart', event => { swipeStart = event.touches.length === 1 ? { x: event.touches[0].clientX, y: event.touches[0].clientY } : null; }, { passive: true });
$('viewer-image').addEventListener('touchend', event => {
  if (!swipeStart || event.touches.length) { swipeStart = null; return; }
  const touch = event.changedTouches[0], dx = touch.clientX - swipeStart.x, dy = touch.clientY - swipeStart.y;
  if (Math.abs(dx) > 70 && Math.abs(dx) > Math.abs(dy) * 2) showPhoto(state.selected + (dx < 0 ? 1 : -1));
  swipeStart = null;
}, { passive: true });

function periodRange(period) {
  const [year, month] = period.split('-').map(Number);
  const from = new Date(year, month ? month - 1 : 0, 1);
  const until = new Date(month ? year : year + 1, month || 0, 1);
  return { from: from.toISOString(), until: until.toISOString() };
}
function choosePeriod(period) {
  state.period = period;
  load(true);
  window.scrollTo({ top: 0, behavior: 'instant' });
}
async function loadTimeline() {
  const request = ++state.timelineRequest;
  const params = new URLSearchParams({ q: state.query, favorite: String(state.favorite), tz: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' });
  try {
    const data = await api(`/api/v1/photos/timeline?${params}`);
    if (request !== state.timelineRequest || $('app').hidden) return;
    const rail = $('timeline'); rail.replaceChildren();
    const button = (text, period, label, className) => {
      const node = document.createElement('button'); node.textContent = text; node.className = className;
      node.setAttribute('aria-label', label); node.title = label;
      if (period === state.period) { node.classList.add('selected'); node.setAttribute('aria-current', 'date'); }
      node.addEventListener('click', () => choosePeriod(period)); return node;
    };
    rail.append(button('all', '', 'Show all upload dates', 'timeline-all'));
    let year;
    const monthFormat = new Intl.DateTimeFormat('en', { month: 'short' });
    for (const item of data.months) {
      const [itemYear, month] = item.month.split('-');
      if (itemYear !== year) { year = itemYear; rail.append(button(year, year, `Uploads in ${year}`, 'timeline-year')); }
      const label = monthFormat.format(new Date(Number(year), Number(month) - 1, 1));
      rail.append(button(label, item.month, `${label} ${year}: ${item.count} ${item.count === 1 ? 'photo' : 'photos'}`, 'timeline-month'));
    }
  } catch (error) { if (request === state.timelineRequest && !$('app').hidden) status(error.message, true); }
}
