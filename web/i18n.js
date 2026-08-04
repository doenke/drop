/*
 * drop — i18n. Englisch ist die Quelle der Wahrheit; die Sprache wird
 * ausschließlich automatisch aus der Browser-/Systemsprache erkannt, es
 * gibt bewusst keinen manuellen Umschalter (anders als beim Farbschema).
 */
(() => {
  'use strict';

  const MESSAGES = {
    en: {
      hero_title: 'Fast, right across.',
      hero_lede: 'Text, links, passwords, and files between your devices — no detour through messengers or mail. Nothing is stored.',
      btn_create: 'Create room',
      hint_create: 'The room disappears once everyone has left it.',
      btn_login: 'Sign in',
      hint_login: 'You need to sign in to create a room. Joining works without one.',
      join_heading: 'Join a room',
      hint_join: 'Scan the QR code or type the three words.',
      join_placeholder: 'acorn maple otter',
      join_aria_label: 'Join code',
      btn_join: 'Join',
      conn_title: 'Connection',
      conn_offline: 'offline',
      conn_connecting: 'connecting…',
      conn_online: 'connected',
      theme_toggle_aria_label: 'Toggle color scheme',
      qr_alt: 'QR code to join',
      qr_fallback: 'QR code unavailable — use the three words instead.',
      invite_heading: 'Add a second device',
      invite_hint: 'Scan the QR code — or type the three words on the main page.',
      btn_copy_link: 'Copy link',
      btn_copy_code: 'Copy code',
      btn_leave: 'Leave room',
      live_heading: 'Live text box',
      live_state_ready: 'ready',
      live_hint: "What's typed here is seen by everyone in the room instantly. It isn't kept in a history.",
      live_placeholder: 'Type or paste — appears instantly on the other device.',
      live_aria_label: 'Live text box',
      transfers_heading: 'Transfers',
      btn_paste: 'Paste',
      btn_file: 'Send file',
      feed_hint: 'Drag files anywhere onto the window, or press Ctrl/Cmd+V.',
      feed_empty: 'Nothing sent yet.',
      footer_opensource: 'is open source.',
      footer_github: 'Source code on GitHub',
      dropzone_text: 'Drop to send',
      noscript: 'drop needs JavaScript — the transfer runs entirely over a WebSocket connection.',

      toast_peer_joined: '{name} joined',
      toast_peer_left: '{name} left the room',
      toast_room_open: 'Room is open — scan the QR code or share the words',
      toast_left_room: 'Left room',
      toast_error_generic: 'Something went wrong',
      toast_copied: 'Copied to clipboard',
      toast_copy_denied: 'Copying was blocked — select the text and press Ctrl/Cmd+C',
      toast_image_copied: 'Image copied to clipboard',
      toast_image_copy_unsupported: "This browser can't copy images — use Download instead",
      toast_transfer_aborted: 'Transfer of "{name}" was aborted',
      toast_clipboard_unavailable: "Can't read the clipboard here — use Ctrl/Cmd+V or the file button",
      toast_clipboard_unreadable: 'Clipboard not readable — use Ctrl/Cmd+V or the file button',
      toast_clipboard_empty: 'Nothing usable was in the clipboard',
      toast_file_empty: '"{name}" is empty and was skipped',
      toast_file_send_failed: '"{name}" could not be sent',
      toast_connection_lost: 'Connection lost — "{name}" was not sent',

      members_only_you: 'Only you are connected',
      members_count: '{count} devices connected',
      live_state_typing: 'typing…',
      live_state_synced: 'synced',
      live_state_updated: 'updated',
      item_title_own: 'Sent by you',
      item_title_from: 'Text from {name}',
      action_copy: 'Copy',
      action_download: 'Download',
      action_copy_image: 'Copy image',
      action_resend: 'Resend',
      meta_from_you: 'from you',
      meta_from_name: 'from {name}',
      avatar_alt_named: 'Profile picture of {name}',
      avatar_alt: 'Profile picture',
      theme_light: 'Light',
      theme_dark: 'Dark',
      theme_toggle_title_to_light: 'Switch to light',
      theme_toggle_title_to_dark: 'Switch to dark',

      // Übersetzt den WebSocket-Fehlercode aus errorMsg.code — siehe
      // internal/ws/protocol.go für die Bedeutung jedes Codes.
      errors: {
        'unauthorized': 'Please sign in to create a room',
        'room-not-found': 'This room is not (or no longer) open',
        'room-full': 'This room is full',
        'rate-limited': 'Too many attempts, please wait a moment',
        'invalid-json': 'Something went wrong — please reload',
        'unknown-type': 'Something went wrong — please reload',
        'already-in-room': 'This connection is already in a room',
        'create-failed': 'Could not create room',
        'missing-code-or-token': 'Please enter a join code',
        'not-in-room': 'Join a room first',
        'text-sync-empty': 'Something went wrong — please reload',
        'live-text-too-large': 'The live text box content is too long',
        'item-text-empty': 'Something went wrong — please reload',
        'text-item-too-large': 'The text snippet is too large',
        'file-id-invalid': 'This file could not be sent',
        'upload-duplicate-id': 'This transfer is already in progress',
        'too-many-uploads': 'Too many concurrent transfers',
        'file-too-large': 'The file is too large',
        'binary-frame-invalid': 'Something went wrong — please reload',
        'chunk-unannounced': 'Something went wrong — please reload',
        'chunk-too-large': 'The file could not be sent',
        'upload-overflow': 'The file could not be sent',
        'file-end-unknown': 'Something went wrong — please reload',
      },
    },
    de: {
      hero_title: 'Schnell rübergeschoben.',
      hero_lede: 'Text, Links, Passwörter und Dateien zwischen deinen Geräten — ohne Umweg über Messenger oder Mail. Nichts wird gespeichert.',
      btn_create: 'Raum erstellen',
      hint_create: 'Der Raum verschwindet, sobald ihn alle verlassen haben.',
      btn_login: 'Anmelden',
      hint_login: 'Zum Erstellen eines Raums brauchst du einen Login. Beitreten geht ohne.',
      join_heading: 'Einem Raum beitreten',
      hint_join: 'Scanne den QR-Code oder tippe die drei Wörter ein.',
      join_placeholder: 'apfel tanne kerze',
      join_aria_label: 'Beitrittscode',
      btn_join: 'Beitreten',
      conn_title: 'Verbindung',
      conn_offline: 'getrennt',
      conn_connecting: 'verbinde …',
      conn_online: 'verbunden',
      theme_toggle_aria_label: 'Farbschema umschalten',
      qr_alt: 'QR-Code zum Beitreten',
      qr_fallback: 'QR-Code nicht verfügbar — nutze die drei Wörter.',
      invite_heading: 'Zweites Gerät dazuholen',
      invite_hint: 'QR scannen — oder die drei Wörter auf Hauptseite eintippen.',
      btn_copy_link: 'Link kopieren',
      btn_copy_code: 'Code kopieren',
      btn_leave: 'Raum verlassen',
      live_heading: 'Live-Textbox',
      live_state_ready: 'bereit',
      live_hint: 'Was hier steht, sehen alle im Raum sofort. Landet nicht im Verlauf.',
      live_placeholder: 'Tippen oder einfügen — erscheint direkt auf dem anderen Gerät.',
      live_aria_label: 'Live-Textbox',
      transfers_heading: 'Übertragungen',
      btn_paste: 'Einfügen',
      btn_file: 'Datei senden',
      feed_hint: 'Ziehe Dateien irgendwo auf das Fenster, oder drücke Strg/Cmd + V.',
      feed_empty: 'Noch nichts übertragen.',
      footer_opensource: 'ist quelloffen.',
      footer_github: 'Quellcode auf GitHub',
      dropzone_text: 'Loslassen zum Senden',
      noscript: 'drop braucht JavaScript — die Übertragung läuft komplett über eine WebSocket-Verbindung.',

      toast_peer_joined: '{name} ist dazugekommen',
      toast_peer_left: '{name} hat den Raum verlassen',
      toast_room_open: 'Raum ist offen — QR scannen oder Wörter durchgeben',
      toast_left_room: 'Raum verlassen',
      toast_error_generic: 'Es ist etwas schiefgegangen',
      toast_copied: 'In die Zwischenablage kopiert',
      toast_copy_denied: 'Kopieren wurde abgelehnt — Text markieren und Strg/Cmd+C',
      toast_image_copied: 'Bild in die Zwischenablage kopiert',
      toast_image_copy_unsupported: 'Dieser Browser kopiert keine Bilder — nutze Herunterladen',
      toast_transfer_aborted: 'Übertragung von „{name}“ abgebrochen',
      toast_clipboard_unavailable: 'Zwischenablage lesen geht hier nicht — nimm Strg/Cmd+V oder den Datei-Button',
      toast_clipboard_unreadable: 'Zwischenablage nicht lesbar — nimm Strg/Cmd+V oder den Datei-Button',
      toast_clipboard_empty: 'In der Zwischenablage war nichts Brauchbares',
      toast_file_empty: '„{name}“ ist leer und wurde übersprungen',
      toast_file_send_failed: '„{name}“ konnte nicht gesendet werden',
      toast_connection_lost: 'Verbindung weg — „{name}“ wurde nicht gesendet',

      members_only_you: 'Nur du bist verbunden',
      members_count: '{count} Geräte verbunden',
      live_state_typing: 'tippt …',
      live_state_synced: 'synchron',
      live_state_updated: 'aktualisiert',
      item_title_own: 'Von dir gesendet',
      item_title_from: 'Text von {name}',
      action_copy: 'Kopieren',
      action_download: 'Herunterladen',
      action_copy_image: 'Bild kopieren',
      action_resend: 'Erneut senden',
      meta_from_you: 'von dir',
      meta_from_name: 'von {name}',
      avatar_alt_named: 'Profilbild von {name}',
      avatar_alt: 'Profilbild',
      theme_light: 'Hell',
      theme_dark: 'Dunkel',
      theme_toggle_title_to_light: 'Zu hell wechseln',
      theme_toggle_title_to_dark: 'Zu dunkel wechseln',

      errors: {
        'unauthorized': 'Zum Anlegen eines Raums bitte anmelden',
        'room-not-found': 'Dieser Raum ist nicht (mehr) offen',
        'room-full': 'Der Raum ist voll',
        'rate-limited': 'Zu viele Versuche, bitte kurz warten',
        'invalid-json': 'Es ist etwas schiefgegangen — bitte neu laden',
        'unknown-type': 'Es ist etwas schiefgegangen — bitte neu laden',
        'already-in-room': 'Diese Verbindung ist schon in einem Raum',
        'create-failed': 'Raum konnte nicht angelegt werden',
        'missing-code-or-token': 'Bitte einen Beitrittscode eingeben',
        'not-in-room': 'Erst einem Raum beitreten',
        'text-sync-empty': 'Es ist etwas schiefgegangen — bitte neu laden',
        'live-text-too-large': 'Der Text in der Live-Box ist zu lang',
        'item-text-empty': 'Es ist etwas schiefgegangen — bitte neu laden',
        'text-item-too-large': 'Das Text-Snippet ist zu groß',
        'file-id-invalid': 'Diese Datei konnte nicht gesendet werden',
        'upload-duplicate-id': 'Diese Übertragung läuft bereits',
        'too-many-uploads': 'Zu viele gleichzeitige Übertragungen',
        'file-too-large': 'Die Datei ist zu groß',
        'binary-frame-invalid': 'Es ist etwas schiefgegangen — bitte neu laden',
        'chunk-unannounced': 'Es ist etwas schiefgegangen — bitte neu laden',
        'chunk-too-large': 'Die Datei konnte nicht gesendet werden',
        'upload-overflow': 'Die Datei konnte nicht gesendet werden',
        'file-end-unknown': 'Es ist etwas schiefgegangen — bitte neu laden',
      },
    },
  };

  // detectLang liest navigator.languages (Fallback: navigator.language) und
  // matcht gegen die unterstützten Sprachen; nicht erkannt oder leer landet
  // auf Englisch, dem neuen Standard der App.
  function detectLang() {
    const supported = Object.keys(MESSAGES);
    const candidates = (navigator.languages && navigator.languages.length)
      ? navigator.languages
      : [navigator.language || 'en'];
    for (const raw of candidates) {
      const tag = String(raw).toLowerCase();
      const match = supported.find((s) => tag === s || tag.startsWith(s + '-'));
      if (match) return match;
    }
    return 'en';
  }

  const lang = detectLang();

  // t liefert den Text zu key in der erkannten Sprache; {platzhalter} in
  // params werden eingesetzt. Ein fehlender Schlüssel fällt auf Englisch,
  // dann auf den Schlüssel selbst zurück, statt eine leere Stelle zu zeigen.
  function t(key, params) {
    let msg = (MESSAGES[lang] && MESSAGES[lang][key]) || MESSAGES.en[key] || key;
    if (params) {
      for (const k in params) {
        msg = msg.split(`{${k}}`).join(params[k]);
      }
    }
    return msg;
  }

  // errorText übersetzt einen WebSocket-Fehlercode (errorMsg.code); ohne
  // Treffer liefert sie null, damit der Aufrufer auf msg.message zurückfallen
  // kann (etwa bei einem Code, den diese Version des Frontends noch nicht
  // kennt).
  function errorText(code) {
    return (MESSAGES[lang].errors && MESSAGES[lang].errors[code])
      || (MESSAGES.en.errors && MESSAGES.en.errors[code])
      || null;
  }

  window.I18N = { lang, t, detectLang, errorText };
})();
