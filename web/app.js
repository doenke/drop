/*
 * drop — Frontend.
 *
 * Alles laeuft ueber eine einzige WebSocket-Verbindung: Steuernachrichten als
 * JSON-Textframes, Dateiinhalte als Binaerframes mit ID-Kopf
 * (uint16-Laenge | ID | Rohdaten).
 */
(() => {
  'use strict';

  // Service-Worker-Registrierung ganz an den Anfang, vor allem, was später
  // im Skript noch werfen könnte: ein Deploy kann eine bereits gecachte
  // app.js unbrauchbar machen (fehlendes Element o. Ä.), und genau dann muss
  // trotzdem noch ein neuer Service Worker installiert werden können. Läuft
  // die Seite schon unter einem Controller (Wiederkehrer), lädt sie sich
  // automatisch genau einmal neu, sobald ein neuer Service Worker übernimmt
  // — sonst bräuchte es von Hand mehrere Reloads, bis der alte Cache durch
  // ist. Beim allerersten Besuch gibt es noch keinen Controller, und ein
  // Reload direkt nach dem ersten Laden wäre nur störend.
  if ('serviceWorker' in navigator) {
    if (navigator.serviceWorker.controller) {
      let reloadedForUpdate = false;
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (reloadedForUpdate) return;
        reloadedForUpdate = true;
        location.reload();
      });
    }
    window.addEventListener('load', () => {
      navigator.serviceWorker.register('/sw.js').catch(() => { /* ohne SW laeuft alles weiter */ });
    });
  }

  const CHUNK_SIZE = 64 * 1024;
  // Ab dieser Fuellhoehe des Sendepuffers wird pausiert. Das ist die
  // Client-Seite der Backpressure: ohne sie wuerde eine grosse Datei den
  // Speicher des Browsers und den Server-Puffer gleichermassen fluten.
  const BUFFER_LIMIT = 1024 * 1024;
  const TEXT_DEBOUNCE = 150;
  const RECONNECT_MAX = 15000;

  const el = {
    conn: document.getElementById('conn'),
    account: document.getElementById('account'),
    accountName: document.getElementById('account-name'),
    avatar: document.getElementById('avatar'),
    landing: document.getElementById('view-landing'),
    room: document.getElementById('view-room'),
    createBox: document.getElementById('create-box'),
    loginBox: document.getElementById('login-box'),
    btnCreate: document.getElementById('btn-create'),
    btnLogin: document.getElementById('btn-login'),
    joinForm: document.getElementById('join-form'),
    joinCode: document.getElementById('join-code'),
    qr: document.getElementById('qr'),
    qrFallback: document.getElementById('qr-fallback'),
    roomCode: document.getElementById('room-code'),
    members: document.getElementById('members'),
    btnCopyLink: document.getElementById('btn-copy-link'),
    btnCopyCode: document.getElementById('btn-copy-code'),
    btnLeave: document.getElementById('btn-leave'),
    liveText: document.getElementById('live-text'),
    liveState: document.getElementById('live-state'),
    btnPaste: document.getElementById('btn-paste'),
    btnFile: document.getElementById('btn-file'),
    fileInput: document.getElementById('file-input'),
    feed: document.getElementById('feed'),
    feedEmpty: document.getElementById('feed-empty'),
    dropzone: document.getElementById('dropzone'),
    toasts: document.getElementById('toasts'),
  };

  const state = {
    ws: null,
    room: null,
    me: null,
    members: 1,
    pending: null,        // was nach dem Verbindungsaufbau gesendet wird
    outbox: [],
    lastTextSeq: 0,
    textTimer: null,
    incoming: new Map(),  // Datei-ID → laufender Empfang
    uploadChain: Promise.resolve(),
    reconnectDelay: 500,
    reconnectTimer: null,
    leaving: false,
  };

  /* ---------------------------------------------------------------- Hilfen */

  const encoder = new TextEncoder();
  const decoder = new TextDecoder();

  function formatBytes(n) {
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB'];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
  }

  function toast(message, kind = 'info') {
    const node = document.createElement('div');
    node.className = 'toast';
    node.dataset.kind = kind;
    node.textContent = message;
    el.toasts.append(node);
    setTimeout(() => node.remove(), 4000);
  }

  const TIME_LOCALE = { en: 'en-GB', de: 'de-DE' };

  function timestamp() {
    return new Date().toLocaleTimeString(TIME_LOCALE[I18N.lang] || 'en-GB', { hour: '2-digit', minute: '2-digit' });
  }

  // applyI18n übersetzt alle statischen [data-i18n*]-Knoten. Läuft ganz am
  // Anfang von start(), bevor die Ansicht aus ihrem hidden-Zustand geholt
  // wird — dadurch gibt es kein sichtbares Sprachwechsel-Flackern.
  function applyI18n() {
    document.querySelectorAll('[data-i18n]').forEach((n) => { n.textContent = I18N.t(n.dataset.i18n); });
    document.querySelectorAll('[data-i18n-placeholder]').forEach((n) => { n.placeholder = I18N.t(n.dataset.i18nPlaceholder); });
    document.querySelectorAll('[data-i18n-aria-label]').forEach((n) => { n.setAttribute('aria-label', I18N.t(n.dataset.i18nAriaLabel)); });
    document.querySelectorAll('[data-i18n-title]').forEach((n) => { n.title = I18N.t(n.dataset.i18nTitle); });
    document.querySelectorAll('[data-i18n-alt]').forEach((n) => { n.alt = I18N.t(n.dataset.i18nAlt); });
    document.documentElement.lang = I18N.lang;
  }

  function setConnState(stateName, label) {
    el.conn.dataset.state = stateName;
    el.conn.querySelector('.conn-label').textContent = label;
  }

  function inRoom() { return state.room !== null; }

  /* ----------------------------------------------------------- Verbindung */

  function socketURL() {
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${scheme}//${location.host}/ws`;
  }

  function connect() {
    if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
      return;
    }
    setConnState('connecting', I18N.t('conn_connecting'));
    const ws = new WebSocket(socketURL());
    ws.binaryType = 'arraybuffer';
    state.ws = ws;

    ws.addEventListener('open', () => {
      state.reconnectDelay = 500;
      setConnState('online', I18N.t('conn_online'));
      // Nach einem Abriss zurueck in denselben Raum — der lebt noch, solange
      // die Grace-Time laeuft.
      if (state.room) {
        ws.send(JSON.stringify({ type: 'join', token: state.room.token }));
      } else if (state.pending) {
        ws.send(JSON.stringify(state.pending));
      }
      state.pending = null;
      const queued = state.outbox;
      state.outbox = [];
      queued.forEach((msg) => ws.send(JSON.stringify(msg)));
    });

    ws.addEventListener('message', (event) => {
      if (typeof event.data === 'string') {
        let msg;
        try { msg = JSON.parse(event.data); } catch { return; }
        handleMessage(msg);
      } else {
        handleBinary(event.data);
      }
    });

    ws.addEventListener('close', () => {
      setConnState('offline', I18N.t('conn_offline'));
      if (state.leaving) return;
      if (!state.room && !state.pending) return;
      state.reconnectTimer = setTimeout(connect, state.reconnectDelay);
      state.reconnectDelay = Math.min(state.reconnectDelay * 2, RECONNECT_MAX);
    });

    ws.addEventListener('error', () => { /* close folgt und uebernimmt */ });
  }

  // send stellt sicher, dass die Nachricht rausgeht, sobald eine Verbindung
  // steht — der Aufrufer muss sich nicht um den Zustand kuemmern.
  function send(msg) {
    if (state.ws && state.ws.readyState === WebSocket.OPEN) {
      state.ws.send(JSON.stringify(msg));
      return;
    }
    if (msg.type === 'create' || msg.type === 'join') {
      state.pending = msg;
    } else {
      state.outbox.push(msg);
    }
    connect();
  }

  function sendBinary(bytes) {
    if (state.ws && state.ws.readyState === WebSocket.OPEN) state.ws.send(bytes);
  }

  /* ------------------------------------------------- Eingehende Nachrichten */

  function handleMessage(msg) {
    switch (msg.type) {
      case 'room': enterRoom(msg); break;
      case 'peer-joined':
        state.members = msg.members;
        updateMembers();
        toast(I18N.t('toast_peer_joined', { name: msg.peer.name }));
        break;
      case 'peer-left':
        state.members = msg.members;
        updateMembers();
        toast(I18N.t('toast_peer_left', { name: msg.peer.name }));
        break;
      case 'text-sync': applyRemoteText(msg); break;
      case 'item-text': addTextItem({ content: msg.content, from: msg.from.name, own: false }); break;
      case 'file-meta': beginIncomingFile(msg); break;
      case 'file-end': finishIncomingFile(msg.id); break;
      case 'file-abort': abortIncomingFile(msg.id); break;
      case 'error': handleServerError(msg); break;
      default: break;
    }
  }

  function handleServerError(msg) {
    toast(I18N.errorText(msg.code) || msg.message || I18N.t('toast_error_generic'), 'error');
    if (msg.code === 'room-not-found' || msg.code === 'room-full') {
      leaveRoom({ silent: true });
    }
  }

  function handleBinary(buffer) {
    const view = new DataView(buffer);
    if (buffer.byteLength < 2) return;
    const idLen = view.getUint16(0);
    if (buffer.byteLength < 2 + idLen) return;
    const id = decoder.decode(new Uint8Array(buffer, 2, idLen));
    const entry = state.incoming.get(id);
    if (!entry) return;

    const payload = buffer.slice(2 + idLen);
    entry.chunks.push(payload);
    entry.received += payload.byteLength;
    setProgress(entry.node, entry.received / entry.size);
  }

  /* -------------------------------------------------------------- Ansichten */

  function showLanding() {
    el.landing.hidden = false;
    el.room.hidden = true;
  }

  function showRoom() {
    el.landing.hidden = true;
    el.room.hidden = false;
  }

  function enterRoom(msg) {
    const rejoined = state.room !== null;
    state.room = { id: msg.id, token: msg.token, code: msg.code, url: msg.url };
    state.me = msg.you;
    state.members = (msg.peers ? msg.peers.length : 0) + 1;
    state.lastTextSeq = msg.textSeq || 0;

    el.roomCode.textContent = msg.code.replace(/-/g, ' ');
    el.qr.hidden = false;
    el.qrFallback.hidden = true;
    el.qr.src = `/api/qr?token=${encodeURIComponent(msg.token)}`;
    updateMembers();

    // Bei einem Reconnect steht in der Box vielleicht schon Getipptes, das
    // noch nicht raus ist — das darf der Snapshot nicht ueberschreiben.
    if (!rejoined || !state.textTimer) {
      el.liveText.value = msg.text || '';
    }

    const path = `/r/${msg.token}`;
    if (location.pathname !== path) history.replaceState(null, '', path);
    showRoom();

    if (msg.created) toast(I18N.t('toast_room_open'), 'ok');
  }

  function leaveRoom({ silent = false } = {}) {
    state.leaving = true;
    clearTimeout(state.reconnectTimer);
    if (state.ws) state.ws.close();
    state.ws = null;
    state.room = null;
    state.me = null;
    state.pending = null;
    state.outbox = [];
    state.incoming.forEach((entry) => entry.node.remove());
    state.incoming.clear();
    el.feed.replaceChildren();
    el.liveText.value = '';
    updateFeedEmpty();
    setConnState('offline', I18N.t('conn_offline'));
    history.replaceState(null, '', '/');
    showLanding();
    state.leaving = false;
    if (!silent) toast(I18N.t('toast_left_room'));
  }

  function updateMembers() {
    el.members.textContent = state.members === 1
      ? I18N.t('members_only_you')
      : I18N.t('members_count', { count: state.members });
  }

  function updateFeedEmpty() {
    el.feedEmpty.hidden = el.feed.childElementCount > 0;
  }

  /* ------------------------------------------------------------ Live-Textbox */

  el.liveText.addEventListener('input', () => {
    el.liveState.textContent = I18N.t('live_state_typing');
    clearTimeout(state.textTimer);
    state.textTimer = setTimeout(flushText, TEXT_DEBOUNCE);
  });

  function flushText() {
    state.textTimer = null;
    if (!inRoom()) return;
    send({ type: 'text-sync', full: el.liveText.value });
    el.liveState.textContent = I18N.t('live_state_synced');
  }

  function applyRemoteText(msg) {
    if (msg.seq <= state.lastTextSeq) return;
    state.lastTextSeq = msg.seq;
    // Wer gerade selbst tippt, gewinnt: die eigene Aenderung ist schon
    // unterwegs und wuerde sonst mitten im Wort zurueckgesetzt.
    if (state.textTimer) return;

    const box = el.liveText;
    const focused = document.activeElement === box;
    const atEnd = box.selectionStart === box.value.length;
    const caret = box.selectionStart;
    box.value = msg.full;
    if (focused) {
      const pos = atEnd ? msg.full.length : Math.min(caret, msg.full.length);
      box.setSelectionRange(pos, pos);
    }
    el.liveState.textContent = I18N.t('live_state_updated');
  }

  /* ------------------------------------------------------------- Feed-Items */

  function addItemNode({ title, meta, own }) {
    const li = document.createElement('li');
    li.className = 'item';
    li.dataset.own = String(own);

    const head = document.createElement('div');
    head.className = 'item-head';
    const titleNode = document.createElement('span');
    titleNode.className = 'item-title';
    titleNode.textContent = title;
    const metaNode = document.createElement('span');
    metaNode.textContent = meta;
    head.append(titleNode, metaNode);
    li.append(head);

    el.feed.prepend(li);
    updateFeedEmpty();
    return li;
  }

  function addActions(li) {
    const actions = document.createElement('div');
    actions.className = 'item-actions';
    li.append(actions);
    return actions;
  }

  function actionButton(label, handler, primary = false) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = primary ? 'btn btn-primary' : 'btn';
    btn.textContent = label;
    btn.addEventListener('click', handler);
    return btn;
  }

  function addTextItem({ content, from, own }) {
    const li = addItemNode({
      title: own ? I18N.t('item_title_own') : I18N.t('item_title_from', { name: from }),
      meta: timestamp(),
      own,
    });

    const pre = document.createElement('pre');
    pre.className = 'item-text';
    pre.textContent = content;
    li.append(pre);

    const actions = addActions(li);
    actions.append(actionButton(I18N.t('action_copy'), async () => {
      await copyText(content);
    }, true));
  }

  function setProgress(li, ratio) {
    const bar = li.querySelector('.progress span');
    if (bar) bar.style.width = `${Math.min(100, Math.round(ratio * 100))}%`;
  }

  function addProgress(li) {
    const wrap = document.createElement('div');
    wrap.className = 'progress';
    wrap.append(document.createElement('span'));
    li.append(wrap);
    return wrap;
  }

  function beginIncomingFile(msg) {
    const li = addItemNode({
      title: msg.name,
      meta: `${formatBytes(msg.size)} · ${I18N.t('meta_from_name', { name: msg.from.name })}`,
      own: false,
    });
    addProgress(li);
    state.incoming.set(msg.id, {
      name: msg.name,
      mime: msg.mime || 'application/octet-stream',
      size: msg.size,
      received: 0,
      chunks: [],
      node: li,
    });
  }

  function finishIncomingFile(id) {
    const entry = state.incoming.get(id);
    if (!entry) return;
    state.incoming.delete(id);

    const blob = new Blob(entry.chunks, { type: entry.mime });
    entry.chunks = [];
    const url = URL.createObjectURL(blob);
    const li = entry.node;
    li.querySelector('.progress')?.remove();

    if (entry.mime.startsWith('image/')) {
      const img = document.createElement('img');
      img.className = 'item-preview';
      img.alt = entry.name;
      img.src = url;
      li.append(img);
    }

    const actions = addActions(li);
    const download = document.createElement('a');
    download.className = 'btn btn-primary';
    download.textContent = I18N.t('action_download');
    download.href = url;
    download.download = entry.name;
    actions.append(download);

    // Text kann jeder Browser in die Zwischenablage legen, Bilder meistens,
    // beliebige Dateien nicht — deshalb gibt es Copy nur dort, wo es klappt.
    if (entry.mime.startsWith('text/')) {
      actions.append(actionButton(I18N.t('action_copy'), async () => {
        await copyText(await blob.text());
      }));
    } else if (entry.mime.startsWith('image/')) {
      actions.append(actionButton(I18N.t('action_copy_image'), () => copyImage(blob)));
    }
  }

  function abortIncomingFile(id) {
    const entry = state.incoming.get(id);
    if (!entry) return;
    state.incoming.delete(id);
    entry.node.remove();
    updateFeedEmpty();
    toast(I18N.t('toast_transfer_aborted', { name: entry.name }), 'error');
  }

  /* ------------------------------------------------------------ Zwischenablage */

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      toast(I18N.t('toast_copied'), 'ok');
    } catch {
      toast(I18N.t('toast_copy_denied'), 'error');
    }
  }

  async function copyImage(blob) {
    try {
      await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })]);
      toast(I18N.t('toast_image_copied'), 'ok');
    } catch {
      toast(I18N.t('toast_image_copy_unsupported'), 'error');
    }
  }

  async function pasteFromClipboard() {
    if (!navigator.clipboard || !navigator.clipboard.read) {
      toast(I18N.t('toast_clipboard_unavailable'), 'error');
      return;
    }
    let items;
    try {
      items = await navigator.clipboard.read();
    } catch {
      // Auf iOS-Safari und ohne Nutzerfreigabe scheitert das regelmaessig.
      toast(I18N.t('toast_clipboard_unreadable'), 'error');
      return;
    }
    let handled = 0;
    for (const item of items) {
      const imageType = item.types.find((t) => t.startsWith('image/'));
      if (imageType) {
        const blob = await item.getType(imageType);
        const ext = imageType.split('/')[1] || 'png';
        queueFile(new File([blob], `image-${Date.now()}.${ext}`, { type: imageType }));
        handled++;
        continue;
      }
      if (item.types.includes('text/plain')) {
        const blob = await item.getType('text/plain');
        const text = await blob.text();
        if (text.trim()) { sendTextItem(text); handled++; }
      }
    }
    if (!handled) toast(I18N.t('toast_clipboard_empty'), 'error');
  }

  /* ---------------------------------------------------------------- Senden */

  function sendTextItem(content) {
    if (!inRoom()) return;
    send({ type: 'item-text', content });
    addTextItem({ content, from: 'you', own: true });
  }

  function queueFile(file) {
    if (!inRoom()) return;
    if (file.size === 0) {
      toast(I18N.t('toast_file_empty', { name: file.name }), 'error');
      return;
    }
    // Uploads laufen streng nacheinander: gleichzeitige Transfers wuerden
    // sich nur die Bandbreite und den Sendepuffer streitig machen.
    state.uploadChain = state.uploadChain.then(() => sendFile(file)).catch((err) => {
      console.error(err);
      toast(I18N.t('toast_file_send_failed', { name: file.name }), 'error');
    });
  }

  async function sendFile(file) {
    if (!inRoom()) return;
    const id = (crypto.randomUUID && crypto.randomUUID()) || String(Date.now() + Math.random());
    const idBytes = encoder.encode(id);
    const header = new Uint8Array(2 + idBytes.length);
    new DataView(header.buffer).setUint16(0, idBytes.length);
    header.set(idBytes, 2);

    send({ type: 'file-meta', id, name: file.name, mime: file.type, size: file.size });

    const li = addItemNode({
      title: file.name,
      meta: `${formatBytes(file.size)} · ${I18N.t('meta_from_you')}`,
      own: true,
    });
    addProgress(li);

    let sent = 0;
    for (let offset = 0; offset < file.size; offset += CHUNK_SIZE) {
      const buffer = await file.slice(offset, offset + CHUNK_SIZE).arrayBuffer();
      const frame = new Uint8Array(header.length + buffer.byteLength);
      frame.set(header, 0);
      frame.set(new Uint8Array(buffer), header.length);

      if (!(await waitForBuffer())) {
        li.remove();
        updateFeedEmpty();
        toast(I18N.t('toast_connection_lost', { name: file.name }), 'error');
        return;
      }
      sendBinary(frame);
      sent += buffer.byteLength;
      setProgress(li, sent / file.size);
    }

    send({ type: 'file-end', id });
    li.querySelector('.progress')?.remove();
    const actions = addActions(li);
    actions.append(actionButton(I18N.t('action_resend'), () => queueFile(file)));
  }

  // waitForBuffer haelt an, solange der Sendepuffer voll ist, und meldet
  // false, wenn die Verbindung dabei wegbricht.
  function waitForBuffer() {
    return new Promise((resolve) => {
      const check = () => {
        const ws = state.ws;
        if (!ws || ws.readyState !== WebSocket.OPEN) { resolve(false); return; }
        if (ws.bufferedAmount <= BUFFER_LIMIT) { resolve(true); return; }
        setTimeout(check, 25);
      };
      check();
    });
  }

  /* ---------------------------------------------------------- Bedienelemente */

  el.btnCreate.addEventListener('click', () => {
    el.btnCreate.disabled = true;
    setTimeout(() => { el.btnCreate.disabled = false; }, 1000);
    send({ type: 'create', lang: I18N.lang });
  });

  el.joinForm.addEventListener('submit', (event) => {
    event.preventDefault();
    const code = el.joinCode.value.trim();
    if (!code) return;
    send({ type: 'join', code });
  });

  el.avatar.addEventListener('load', () => { el.avatar.hidden = false; });
  el.avatar.addEventListener('error', () => { el.avatar.hidden = true; });

  el.qr.addEventListener('error', () => {
    el.qr.hidden = true;
    el.qrFallback.hidden = false;
  });

  el.btnCopyLink.addEventListener('click', () => copyText(state.room ? state.room.url : ''));
  el.btnCopyCode.addEventListener('click', () => copyText(state.room ? state.room.code.replace(/-/g, ' ') : ''));
  el.btnLeave.addEventListener('click', () => leaveRoom());

  el.btnPaste.addEventListener('click', pasteFromClipboard);
  el.btnFile.addEventListener('click', () => el.fileInput.click());
  el.fileInput.addEventListener('change', () => {
    Array.from(el.fileInput.files).forEach(queueFile);
    el.fileInput.value = '';
  });

  // Das native Paste-Event ist auf dem Desktop der zuverlaessigere Weg fuer
  // Screenshots; in der Live-Box soll Einfuegen aber ganz normal wirken.
  document.addEventListener('paste', (event) => {
    if (!inRoom() || event.target === el.liveText) return;
    const data = event.clipboardData;
    if (!data) return;
    const files = Array.from(data.files || []);
    if (files.length) {
      event.preventDefault();
      files.forEach(queueFile);
      return;
    }
    const text = data.getData('text/plain');
    if (text && text.trim()) {
      event.preventDefault();
      sendTextItem(text);
    }
  });

  let dragDepth = 0;
  document.addEventListener('dragenter', (event) => {
    if (!inRoom()) return;
    event.preventDefault();
    dragDepth++;
    el.dropzone.hidden = false;
  });
  document.addEventListener('dragover', (event) => { if (inRoom()) event.preventDefault(); });
  document.addEventListener('dragleave', () => {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) el.dropzone.hidden = true;
  });
  document.addEventListener('drop', (event) => {
    if (!inRoom()) return;
    event.preventDefault();
    dragDepth = 0;
    el.dropzone.hidden = true;
    const files = Array.from(event.dataTransfer.files || []);
    if (files.length) { files.forEach(queueFile); return; }
    const text = event.dataTransfer.getData('text/plain');
    if (text && text.trim()) sendTextItem(text);
  });

  /* ------------------------------------------------------------------ Start */

  async function loadAccount() {
    try {
      const res = await fetch('/api/me', { headers: { Accept: 'application/json' } });
      const me = await res.json();
      el.createBox.hidden = !me.authenticated;
      el.loginBox.hidden = me.authenticated;
      if (me.authenticated && me.name) {
        el.account.hidden = false;
        el.accountName.textContent = me.name;
      }
      // Das Bild kommt über den eigenen Proxy; ohne Bild bleibt die Stelle
      // einfach leer, es gibt keinen Ersatz.
      if (me.authenticated && me.avatar) {
        el.avatar.alt = me.name ? I18N.t('avatar_alt_named', { name: me.name }) : I18N.t('avatar_alt');
        el.avatar.src = '/api/avatar';
      }
    } catch {
      el.createBox.hidden = true;
      el.loginBox.hidden = false;
    }
  }

  function start() {
    applyI18n();
    showLanding();
    updateFeedEmpty();
    loadAccount();

    const match = location.pathname.match(/^\/r\/(.+)$/);
    if (match) {
      // Der Login soll nach dem Anmelden wieder hier landen.
      el.btnLogin.href = `/auth/login?next=${encodeURIComponent(location.pathname)}`;
      send({ type: 'join', token: decodeURIComponent(match[1]) });
    }
  }

  start();
})();
