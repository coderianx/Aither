/* Aither — web client.
   Talks to the local Aither API and renders a streaming chat experience. */

'use strict';

const API = '/api/v1';
const MOBILE = window.matchMedia('(max-width: 860px)');
const DEFAULT_TITLE = 'New Chat';

/* ── State ─────────────────────────────────────────────────────────────── */

const state = {
  providers: [],
  conversations: [],
  models: new Map(), // provider id -> model list
  conversation: null, // active conversation record (null in draft mode)
  messages: [],
  draft: { providerId: null, model: '' },
  controller: null,
  streaming: false,
};

/* ── DOM ───────────────────────────────────────────────────────────────── */

const $ = (id) => document.getElementById(id);

const app = $('app');
const sidebar = $('sidebar');
const scrim = $('scrim');
const convList = $('convList');
const convEmpty = $('convEmpty');
const chatScroll = $('chatScroll');
const chat = $('chat');
const main = $('main');
const chatTitle = $('chatTitle');

const composerForm = $('composerForm');
const input = $('messageInput');
const submitBtn = $('submitBtn');

const modelBtn = $('modelBtn');
const modelBtnText = $('modelBtnText');
const popover = $('modelPopover');
const modelList = $('modelList');
const modelSearch = $('modelSearch');

const providersDialog = $('providersDialog');
const providerForm = $('providerForm');
const providerList = $('providerList');
const providerEmpty = $('providerEmpty');
const providerName = $('providerName');
const providerKey = $('providerKey');
const providerBaseURL = $('providerBaseURL');
const baseURLField = $('baseURLField');
const providerError = $('providerError');
const providerSubmit = $('providerSubmit');

const toasts = $('toasts');
const liveStatus = $('liveStatus');

/* ── Utilities ─────────────────────────────────────────────────────────── */

function escapeHtml(value) {
  return String(value).replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[char]);
}

function announce(message) {
  liveStatus.textContent = message;
}

function showToast(message, variant = 'info') {
  const toast = document.createElement('div');
  toast.className = variant === 'error' ? 'toast toast--error' : 'toast';
  toast.textContent = message;

  toasts.append(toast);

  window.setTimeout(() => toast.remove(), 5000);
}

/* ── Markdown ──────────────────────────────────────────────────────────── */

/* ── Syntax highlighting ───────────────────────────────────────────────── */

/* A dependency-free highlighter: each language compiles to a single global
   regex whose alternatives are numbered capture groups. Scanning left to
   right means every character belongs to at most one token, so no nesting or
   re-scanning is needed. All output is escaped before it reaches the DOM. */

const TOKEN_CLASS = {
  comment: 'tok-comment',
  string: 'tok-string',
  keyword: 'tok-keyword',
  number: 'tok-number',
  function: 'tok-function',
  variable: 'tok-variable',
  tag: 'tok-tag',
  attr: 'tok-attr',
  property: 'tok-property',
  punct: 'tok-punct',
};

/** Compiles [tokenClass, pattern] pairs into one regex plus a class lookup. */
function buildPattern(parts, flags = '') {
  const classes = parts.map(([name]) => TOKEN_CLASS[name]);
  const source = parts.map(([, re]) => `(${re.source})`).join('|');

  return { re: new RegExp(source, `g${flags}`), classes };
}

/* Reusable fragments. Regex literals are used throughout so escaping stays
   readable; string sources are composed with .source. */

const RE_COMMENT_C = /\/\/[^\n]*|\/\*[\s\S]*?\*\//;
const RE_COMMENT_HASH = /#[^\n]*/;
const RE_QUOTED =
  /"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*'/;
const RE_TEMPLATE = /`(?:\\.|[^`\\])*`/;
const RE_DOUBLE_QUOTED = /"(?:\\.|[^"\\\n])*"/;
const RE_NUMBER =
  /\b(?:0[xXbBoO][0-9a-fA-F_]+|\d[\d_]*(?:\.\d+)?(?:[eE][+-]?\d+)?)\b/;
const RE_CALL = /[A-Za-z_$][\w$]*(?=\s*\()/;
const RE_PUNCT = /[{}()[\];,.:?=+\-*/%<>!&|^~]+/;

/** C-family languages: // and /* comments, quoted strings, {} punctuation. */
function cLike(keywords, extra = [], flags = '') {
  return buildPattern(
    [
      ['comment', RE_COMMENT_C],
      ['string', new RegExp(`${RE_QUOTED.source}|${RE_TEMPLATE.source}`)],
      ['keyword', keywords],
      ...extra,
      ['number', RE_NUMBER],
      ['function', RE_CALL],
      ['punct', RE_PUNCT],
    ],
    flags,
  );
}

const LANGUAGE_PATTERNS = {
  javascript: cLike(
    /\b(?:as|async|await|break|case|catch|class|const|continue|default|delete|do|else|export|extends|finally|for|from|function|get|if|import|in|instanceof|let|new|of|return|set|static|super|switch|this|throw|try|typeof|var|void|while|with|yield|true|false|null|undefined)\b/,
  ),

  typescript: cLike(
    /\b(?:abstract|any|as|asserts|async|await|bigint|boolean|break|case|catch|class|const|constructor|continue|declare|default|delete|do|else|enum|export|extends|false|finally|for|from|function|get|if|implements|import|in|infer|instanceof|interface|is|keyof|let|namespace|never|new|null|number|object|of|override|private|protected|public|readonly|return|satisfies|set|static|string|super|switch|symbol|this|throw|true|try|type|typeof|undefined|unique|unknown|var|void|while|with|yield)\b/,
  ),

  python: buildPattern([
    ['comment', RE_COMMENT_HASH],
    [
      'string',
      /"""[\s\S]*?"""|'''[\s\S]*?'''|[rRbBuUfF]{0,2}"(?:\\.|[^"\\\n])*"|[rRbBuUfF]{0,2}'(?:\\.|[^'\\\n])*'/,
    ],
    ['variable', /@[\w.]+/],
    [
      'keyword',
      /\b(?:and|as|assert|async|await|break|case|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|match|nonlocal|not|or|pass|raise|return|try|while|with|yield|True|False|None)\b/,
    ],
    ['number', RE_NUMBER],
    ['function', /[A-Za-z_]\w*(?=\s*\()/],
    ['punct', RE_PUNCT],
  ]),

  go: cLike(
    /\b(?:append|break|cap|case|chan|close|complex|const|continue|copy|default|defer|delete|else|error|fallthrough|false|for|func|go|goto|if|imag|import|interface|len|make|map|new|nil|package|panic|range|real|recover|return|select|string|struct|switch|true|type|var)\b/,
  ),

  rust: buildPattern([
    ['comment', RE_COMMENT_C],
    // A char literal holds exactly one character, which keeps lifetimes such
    // as <'a, 'b> from being consumed as a string.
    ['string', /"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])'/],
    [
      'keyword',
      /\b(?:as|async|await|box|break|const|continue|crate|dyn|else|enum|extern|false|fn|for|if|impl|in|let|loop|match|mod|move|mut|pub|ref|return|self|Self|static|struct|super|trait|true|type|unsafe|use|where|while)\b/,
    ],
    ['number', RE_NUMBER],
    ['function', RE_CALL],
    ['punct', RE_PUNCT],
  ]),

  java: cLike(
    /\b(?:abstract|assert|boolean|break|byte|case|catch|char|class|const|continue|default|do|double|else|enum|extends|false|final|finally|float|for|goto|if|implements|import|instanceof|int|interface|long|native|new|null|package|private|protected|public|record|return|short|static|strictfp|super|switch|synchronized|this|throw|throws|transient|true|try|var|void|volatile|while)\b/,
  ),

  c: cLike(
    /\b(?:auto|break|case|char|const|continue|default|do|double|else|enum|extern|false|float|for|goto|if|inline|int|long|register|restrict|return|short|signed|sizeof|static|struct|switch|typedef|union|unsigned|void|volatile|while|NULL)\b/,
    [['keyword', /^[ \t]*#\s*[a-z]+/]],
    'm',
  ),

  cpp: cLike(
    /\b(?:alignas|alignof|auto|bool|break|case|catch|char|class|const|constexpr|continue|decltype|default|delete|do|double|else|enum|explicit|export|extern|false|float|for|friend|goto|if|inline|int|long|mutable|namespace|nullptr|operator|private|protected|public|register|return|short|signed|sizeof|static|struct|switch|template|this|throw|true|try|typedef|typename|union|unsigned|using|virtual|void|volatile|while)\b/,
    [['keyword', /^[ \t]*#\s*[a-z]+/]],
    'm',
  ),

  csharp: cLike(
    /\b(?:abstract|as|async|await|base|bool|break|byte|case|catch|char|class|const|continue|decimal|default|delegate|do|double|else|enum|event|explicit|extern|false|finally|fixed|float|for|foreach|get|goto|if|implicit|in|int|interface|internal|is|lock|long|namespace|new|null|object|operator|out|override|params|private|protected|public|readonly|record|ref|return|sbyte|sealed|set|short|sizeof|stackalloc|static|string|struct|switch|this|throw|true|try|typeof|uint|ulong|unchecked|unsafe|ushort|using|var|virtual|void|volatile|while)\b/,
  ),

  php: buildPattern([
    ['comment', /\/\/[^\n]*|#[^\n]*|\/\*[\s\S]*?\*\//],
    ['string', new RegExp(`${RE_QUOTED.source}|${RE_TEMPLATE.source}`)],
    ['variable', /\$\w+/],
    [
      'keyword',
      /\b(?:abstract|and|array|as|break|callable|case|catch|class|clone|const|continue|declare|default|do|echo|else|elseif|empty|enddeclare|endfor|endforeach|endif|endswitch|endwhile|enum|extends|final|finally|fn|for|foreach|function|global|goto|if|implements|include|instanceof|insteadof|interface|isset|list|match|namespace|new|or|print|private|protected|public|readonly|require|return|static|switch|throw|trait|try|unset|use|var|while|xor|yield|true|false|null)\b/,
    ],
    ['number', RE_NUMBER],
    ['function', RE_CALL],
    ['punct', RE_PUNCT],
  ]),

  bash: buildPattern([
    ['comment', RE_COMMENT_HASH],
    ['string', /"(?:\\.|[^"\\])*"|'[^']*'/],
    ['variable', /\$\{[^}]*\}|\$[A-Za-z_]\w*|\$[?#@!*$0-9]/],
    [
      'keyword',
      /\b(?:alias|case|declare|do|done|elif|else|esac|eval|exec|exit|export|fi|for|function|if|in|local|readonly|return|set|shift|source|then|trap|unset|until|while)\b/,
    ],
    ['number', RE_NUMBER],
    ['function', RE_CALL],
    ['punct', /[|&;<>(){}[\]=!+-]+/],
  ]),

  json: buildPattern([
    ['property', /"(?:\\.|[^"\\])*"(?=\s*:)/],
    ['string', RE_DOUBLE_QUOTED],
    ['keyword', /\b(?:true|false|null)\b/],
    ['number', /-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b/],
    ['punct', /[{}[\],:]/],
  ]),

  sql: buildPattern(
    [
      ['comment', /--[^\n]*|\/\*[\s\S]*?\*\//],
      ['string', /'(?:''|[^'])*'/],
      [
        'keyword',
        /\b(?:add|all|alter|and|as|asc|begin|between|by|case|cast|check|column|commit|constraint|create|delete|desc|distinct|drop|else|end|except|exists|foreign|from|full|group|having|if|in|index|inner|insert|intersect|into|is|join|key|left|like|limit|not|null|offset|on|or|order|outer|primary|references|right|rollback|select|set|table|then|top|transaction|union|unique|update|values|view|when|where|with)\b/,
      ],
      ['number', /\b\d+(?:\.\d+)?\b/],
      ['punct', /[(),;.*=<>+/-]/],
    ],
    'i',
  ),

  html: buildPattern([
    ['comment', /<!--[\s\S]*?-->|<!\[CDATA\[[\s\S]*?\]\]>/],
    ['tag', /<\/?[A-Za-z][\w:.-]*|\/?>/],
    ['attr', /[A-Za-z_][\w:.-]*(?=\s*=)|=/],
    ['string', RE_QUOTED],
    ['punct', /[{}()[\];,:]+/],
  ]),

  css: buildPattern([
    ['comment', /\/\*[\s\S]*?\*\//],
    ['string', RE_QUOTED],
    ['keyword', /@[\w-]+|!important\b/],
    ['variable', /#[0-9a-fA-F]{3,8}\b/],
    ['property', /[-\w]+(?=\s*:)/],
    ['number', /\b\d+(?:\.\d+)?(?:px|em|rem|%|vh|vw|vmin|vmax|s|ms|deg|turn|fr|ch|pt)?\b/],
    ['function', /[-\w]+(?=\()/],
    ['punct', /[{}();:,]/],
  ]),

  yaml: buildPattern(
    [
      ['comment', RE_COMMENT_HASH],
      ['string', new RegExp(`${RE_QUOTED.source}|'(?:''|[^'])*'`)],
      ['property', /[\w.-]+(?=\s*:(?:\s|$))/],
      ['keyword', /\b(?:true|false|null|yes|no|on|off)\b/],
      ['number', RE_NUMBER],
      ['punct', /^\s*-\s|[{}\[\],&*!|>]+/],
    ],
    'm',
  ),

  // Line-anchored patterns need the multiline flag on the compiled regex:
  // flags written on the fragments below are discarded by .source.
  diff: buildPattern(
    [
      ['comment', /^(?:diff|index|@@|---|\+\+\+).*$/],
      ['string', /^\+.*$/],
      ['variable', /^-.*$/],
    ],
    'm',
  ),

  // Unknown languages still get comments, strings and numbers, which is
  // harmless for prose-like or esoteric formats.
  _generic: buildPattern([
    ['comment', /#[^\n]*|\/\/[^\n]*|\/\*[\s\S]*?\*\//],
    ['string', RE_QUOTED],
    ['number', RE_NUMBER],
    ['punct', RE_PUNCT],
  ]),
};

const LANGUAGE_ALIASES = {
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  node: 'javascript',
  ts: 'typescript',
  tsx: 'typescript',
  py: 'python',
  python3: 'python',
  sh: 'bash',
  shell: 'bash',
  zsh: 'bash',
  console: 'bash',
  golang: 'go',
  rs: 'rust',
  yml: 'yaml',
  htm: 'html',
  xml: 'html',
  svg: 'html',
  vue: 'html',
  jsonc: 'json',
  json5: 'json',
  postgres: 'sql',
  postgresql: 'sql',
  mysql: 'sql',
  sqlite: 'sql',
  scss: 'css',
  less: 'css',
  cs: 'csharp',
  'c++': 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  h: 'c',
  patch: 'diff',
};

function normalizeLanguage(lang) {
  const key = String(lang || '').trim().toLowerCase();
  return LANGUAGE_ALIASES[key] || key;
}

/** Wraps recognised tokens in <span> elements; everything else is escaped. */
function highlightCode(code, lang) {
  const pattern = LANGUAGE_PATTERNS[normalizeLanguage(lang)] || LANGUAGE_PATTERNS._generic;

  let html = '';
  let last = 0;
  let match;

  pattern.re.lastIndex = 0;

  while ((match = pattern.re.exec(code)) !== null) {
    if (match.index > last) html += escapeHtml(code.slice(last, match.index));

    let className = null;

    for (let i = 0; i < pattern.classes.length; i += 1) {
      if (match[i + 1] !== undefined) {
        className = pattern.classes[i];
        break;
      }
    }

    html += className
      ? `<span class="${className}">${escapeHtml(match[0])}</span>`
      : escapeHtml(match[0]);

    last = match.index + match[0].length;

    // Every alternative matches at least one character, so this only guards
    // against a future pattern edit introducing an empty match.
    if (match[0].length === 0) pattern.re.lastIndex += 1;
  }

  return html + escapeHtml(code.slice(last));
}

function renderCodeBlock({ lang, code }) {
  const normalized = normalizeLanguage(lang);
  const label = normalized && LANGUAGE_PATTERNS[normalized] ? normalized : lang || 'code';

  return (
    '<figure class="code">' +
    '<figcaption class="code__bar">' +
    `<span class="code__lang">${escapeHtml(label)}</span>` +
    '<button type="button" class="code__copy" data-copy>' +
    '<svg class="icon" aria-hidden="true"><use href="#i-copy" /></svg>' +
    '<span>Copy</span>' +
    '</button>' +
    '</figcaption>' +
    `<pre><code>${highlightCode(code, normalized)}</code></pre>` +
    '</figure>'
  );
}

/**
 * Renders a safe subset of Markdown: fenced and inline code, headings,
 * lists, blockquotes, rules, bold, italic and links. Input is escaped before
 * any markup is produced, so raw HTML in a message can never be executed.
 */
function renderMarkdown(source) {
  const codeBlocks = [];

  const stash = (lang, code) => {
    codeBlocks.push({ lang, code });
    return `\u0000CB${codeBlocks.length - 1}\u0000`;
  };

  let text = String(source).replace(
    /```([^\n`]*)\n([\s\S]*?)```/g,
    (_, lang, code) => stash(lang.trim(), code.replace(/\n$/, '')),
  );

  // An unterminated fence appears while a reply is still streaming.
  text = text.replace(/```([^\n`]*)\n([\s\S]*)$/, (_, lang, code) => stash(lang.trim(), code));

  text = escapeHtml(text);

  const inlineCode = [];
  text = text.replace(/`([^`\n]+)`/g, (_, code) => {
    inlineCode.push(code);
    return `\u0000IC${inlineCode.length - 1}\u0000`;
  });

  const inline = (value) =>
    value
      .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[\s(])\*([^*\n]+)\*/g, '$1<em>$2</em>')
      .replace(/(^|[\s(])_([^_\n]+)_/g, '$1<em>$2</em>')
      .replace(
        /\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)/g,
        '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>',
      )
      .replace(/\u0000IC(\d+)\u0000/g, (_, index) => `<code>${inlineCode[Number(index)]}</code>`);

  let html = '';
  let listType = null;
  let paragraph = [];

  const flushParagraph = () => {
    if (paragraph.length) {
      html += `<p>${inline(paragraph.join('<br />'))}</p>`;
      paragraph = [];
    }
  };

  const closeList = () => {
    if (listType) {
      html += `</${listType}>`;
      listType = null;
    }
  };

  for (const rawLine of text.split('\n')) {
    const line = rawLine.trimEnd();
    const block = line.match(/^\u0000CB(\d+)\u0000$/);

    if (block) {
      flushParagraph();
      closeList();
      html += renderCodeBlock(codeBlocks[Number(block[1])]);
      continue;
    }

    if (line.trim() === '') {
      flushParagraph();
      closeList();
      continue;
    }

    const heading = line.match(/^(#{1,6})\s+(.*)$/);

    if (heading) {
      flushParagraph();
      closeList();
      const level = heading[1].length;
      html += `<h${level}>${inline(heading[2])}</h${level}>`;
      continue;
    }

    if (/^\s*(?:-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      flushParagraph();
      closeList();
      html += '<hr />';
      continue;
    }

    if (line.startsWith('&gt;')) {
      flushParagraph();
      closeList();
      html += `<blockquote><p>${inline(line.slice(4).trim())}</p></blockquote>`;
      continue;
    }

    const bullet = line.match(/^\s*[-*+]\s+(.*)$/);
    const numbered = line.match(/^\s*\d+[.)]\s+(.*)$/);

    if (bullet || numbered) {
      flushParagraph();

      const wanted = bullet ? 'ul' : 'ol';

      if (listType !== wanted) {
        closeList();
        html += `<${wanted}>`;
        listType = wanted;
      }

      html += `<li>${inline((bullet || numbered)[1])}</li>`;
      continue;
    }

    paragraph.push(line);
  }

  flushParagraph();
  closeList();

  return html;
}

/* ── API ───────────────────────────────────────────────────────────────── */

async function request(method, path, body) {
  const response = await fetch(API + path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const raw = await response.text();
  let data = null;

  if (raw) {
    try {
      data = JSON.parse(raw);
    } catch {
      data = null;
    }
  }

  if (!response.ok) {
    throw new Error((data && data.error) || `Request failed (${response.status})`);
  }

  return data;
}

function parseEvent(raw) {
  let name = 'message';
  const chunks = [];

  for (const line of raw.split('\n')) {
    if (!line || line.startsWith(':')) continue;
    if (line.startsWith('event:')) name = line.slice(6).trim();
    else if (line.startsWith('data:')) chunks.push(line.slice(5).trim());
  }

  if (!chunks.length) return null;

  try {
    return { name, data: JSON.parse(chunks.join('\n')) };
  } catch {
    return null;
  }
}

/** Sends a message and dispatches server-sent events until the stream ends. */
async function streamMessage(conversationId, content, onEvent) {
  state.controller = new AbortController();

  const response = await fetch(`${API}/conversations/${conversationId}/messages`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content, stream: true }),
    signal: state.controller.signal,
  });

  if (!response.ok) {
    let message = `Request failed (${response.status})`;

    try {
      const data = await response.json();
      if (data && data.error) message = data.error;
    } catch {
      /* keep the default message */
    }

    throw new Error(message);
  }

  if (!response.body) throw new Error('Streaming is not supported in this browser');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    let boundary = buffer.indexOf('\n\n');

    while (boundary !== -1) {
      const parsed = parseEvent(buffer.slice(0, boundary));
      buffer = buffer.slice(boundary + 2);
      if (parsed) onEvent(parsed.name, parsed.data);
      boundary = buffer.indexOf('\n\n');
    }
  }
}

/* ── Rendering: conversations ──────────────────────────────────────────── */

function renderConversations() {
  convList.replaceChildren();
  convEmpty.hidden = state.conversations.length > 0;

  for (const conversation of state.conversations) {
    const item = document.createElement('li');
    item.className = 'conv';
    if (conversation.id === state.conversation?.id) item.classList.add('is-active');

    const open = document.createElement('button');
    open.type = 'button';
    open.className = 'conv__btn';
    open.textContent = conversation.title || DEFAULT_TITLE;
    open.title = conversation.title || DEFAULT_TITLE;
    open.addEventListener('click', () => {
      closeSidebar();
      openConversation(conversation.id);
    });

    const actions = document.createElement('div');
    actions.className = 'conv__actions';

    const rename = iconButton('i-pencil', 'Rename conversation', 'icon-btn');
    rename.addEventListener('click', (event) => {
      event.stopPropagation();
      startRename(item, conversation);
    });

    const remove = iconButton('i-trash', 'Delete conversation', 'icon-btn icon-btn--danger');
    remove.addEventListener('click', (event) => {
      event.stopPropagation();
      deleteConversation(conversation);
    });

    actions.append(rename, remove);
    item.append(open, actions);
    convList.append(item);
  }
}

function iconButton(icon, label, className) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = className;
  button.setAttribute('aria-label', label);
  button.innerHTML = `<svg class="icon" aria-hidden="true"><use href="#${icon}" /></svg>`;
  return button;
}

function startRename(item, conversation) {
  const field = document.createElement('input');
  field.className = 'conv__rename';
  field.value = conversation.title;
  field.setAttribute('aria-label', 'Conversation title');

  item.replaceChildren(field);
  field.focus();
  field.select();

  let settled = false;

  const commit = async () => {
    if (settled) return;
    settled = true;

    const title = field.value.trim();

    if (!title || title === conversation.title) {
      renderConversations();
      return;
    }

    try {
      const updated = await request('PATCH', `/conversations/${conversation.id}`, { title });
      Object.assign(conversation, updated);

      if (state.conversation?.id === conversation.id) {
        state.conversation = updated;
        setTitle(updated.title);
      }
    } catch (error) {
      showToast(error.message, 'error');
    }

    renderConversations();
  };

  field.addEventListener('blur', commit);
  field.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      field.blur();
    } else if (event.key === 'Escape') {
      settled = true;
      renderConversations();
    }
  });
}

async function deleteConversation(conversation) {
  try {
    await request('DELETE', `/conversations/${conversation.id}`);
  } catch (error) {
    showToast(error.message, 'error');
    return;
  }

  state.conversations = state.conversations.filter((item) => item.id !== conversation.id);

  if (state.conversation?.id === conversation.id) startNewChat();
  else renderConversations();

  showToast('Conversation deleted');
}

/* ── Rendering: messages ───────────────────────────────────────────────── */

function messageElement(role, content) {
  const article = document.createElement('article');
  article.className = `msg msg--${role}`;

  const label = document.createElement('span');
  label.className = 'sr-only';
  label.textContent = role === 'user' ? 'You said' : 'Aither said';

  const body = document.createElement('div');
  body.className = 'msg__body';

  if (role === 'user') body.textContent = content;
  else body.innerHTML = renderMarkdown(content);

  article.append(label, body);
  return article;
}

function errorElement(message) {
  const article = document.createElement('article');
  article.className = 'msg msg--error';
  article.innerHTML =
    '<div class="msg__body">' +
    '<svg class="icon" aria-hidden="true"><use href="#i-alert" /></svg>' +
    '<span></span>' +
    '</div>';
  article.querySelector('span').textContent = message;
  return article;
}

function renderMessages() {
  chat.replaceChildren();

  for (const message of state.messages) {
    if (message.role === 'system') continue;
    chat.append(messageElement(message.role, message.content));
  }

  setEmpty(state.messages.length === 0);
}

function setEmpty(empty) {
  main.classList.toggle('is-empty', empty);
}

function setTitle(title) {
  const value = title || DEFAULT_TITLE;
  chatTitle.textContent = value;
  document.title = value === DEFAULT_TITLE ? 'Aither' : `${value} · Aither`;
}

function isPinnedToBottom() {
  return chatScroll.scrollHeight - chatScroll.scrollTop - chatScroll.clientHeight < 80;
}

function scrollToBottom(force = false) {
  if (force) chatScroll.scrollTop = chatScroll.scrollHeight;
  else if (isPinnedToBottom()) chatScroll.scrollTop = chatScroll.scrollHeight;
}

/* ── Rendering: model picker ───────────────────────────────────────────── */

function currentSelection() {
  if (state.conversation) {
    return { providerId: state.conversation.provider_id, model: state.conversation.model };
  }

  return state.draft;
}

function updateModelButton() {
  const { model } = currentSelection();
  modelBtnText.textContent = model || 'Select a model';
}

async function loadModels(providerId) {
  if (state.models.has(providerId)) return state.models.get(providerId);

  const models = await request('GET', `/providers/${providerId}/models`);
  state.models.set(providerId, models);

  return models;
}

async function openModelPicker() {
  if (!state.providers.length) {
    showToast('Connect a provider first', 'error');
    openProviders();
    return;
  }

  popover.hidden = false;
  modelBtn.setAttribute('aria-expanded', 'true');
  positionPopover();

  await renderModelOptions();
  positionPopover();
  modelSearch.focus();
}

function closeModelPicker() {
  if (popover.hidden) return;
  popover.hidden = true;
  modelBtn.setAttribute('aria-expanded', 'false');
}

function positionPopover() {
  const rect = modelBtn.getBoundingClientRect();
  const width = popover.offsetWidth || 360;
  const left = Math.min(Math.max(rect.right - width, 8), window.innerWidth - width - 8);

  popover.style.top = `${rect.bottom + 6}px`;
  popover.style.left = `${left}px`;
}

async function renderModelOptions() {
  const note = (text) => {
    const element = document.createElement('p');
    element.className = 'popover__note';
    element.textContent = text;
    return element;
  };

  modelList.replaceChildren(note('Loading models…'));

  const results = await Promise.all(
    state.providers.map(async (provider) => {
      try {
        return { provider, models: await loadModels(provider.id) };
      } catch (error) {
        return { provider, error: error.message };
      }
    }),
  );

  const filter = modelSearch.value.trim().toLowerCase();
  const selected = currentSelection();
  modelList.replaceChildren();

  let shown = 0;

  for (const { provider, models, error } of results) {
    const matches = (models || []).filter(
      (model) =>
        !filter ||
        model.id.toLowerCase().includes(filter) ||
        (model.name || '').toLowerCase().includes(filter),
    );

    if (!matches.length && !error) continue;

    const group = document.createElement('p');
    group.className = 'popover__group';
    group.textContent = provider.provider_name;
    modelList.append(group);
    shown += 1;

    if (error) {
      modelList.append(note(error));
      continue;
    }

    for (const model of matches) {
      const option = document.createElement('button');
      option.type = 'button';
      option.className = 'model-option';
      option.setAttribute('role', 'option');
      option.setAttribute(
        'aria-selected',
        String(provider.id === selected.providerId && model.id === selected.model),
      );

      const name = document.createElement('span');
      name.className = 'model-option__name';
      name.textContent = model.name || model.id;

      const id = document.createElement('span');
      id.className = 'model-option__id';
      id.textContent = model.id;

      option.append(name, id);
      option.addEventListener('click', () => selectModel(provider.id, model.id));
      modelList.append(option);
    }
  }

  if (!shown) modelList.append(note('No models found.'));
}

async function selectModel(providerId, model) {
  closeModelPicker();

  if (!state.conversation) {
    state.draft = { providerId, model };
    updateModelButton();
    return;
  }

  try {
    const updated = await request('PATCH', `/conversations/${state.conversation.id}`, {
      provider_id: providerId,
      model,
    });

    state.conversation = updated;

    const cached = state.conversations.find((item) => item.id === updated.id);
    if (cached) Object.assign(cached, updated);
  } catch (error) {
    showToast(error.message, 'error');
    return;
  }

  updateModelButton();
  renderConversations();
}

/* ── Rendering: providers dialog ───────────────────────────────────────── */

function renderProviders() {
  providerList.replaceChildren();
  providerEmpty.hidden = state.providers.length > 0;

  for (const provider of state.providers) {
    const item = document.createElement('li');
    item.className = 'provider';

    const main = document.createElement('div');
    main.className = 'provider__main';

    const name = document.createElement('p');
    name.className = 'provider__name';
    name.textContent = provider.provider_name;

    const url = document.createElement('p');
    url.className = 'provider__url';
    url.textContent = provider.base_url;

    main.append(name, url);

    const remove = iconButton('i-trash', `Remove ${provider.provider_name}`, 'icon-btn icon-btn--danger');
    remove.addEventListener('click', () => removeProvider(provider));

    item.append(main, remove);
    providerList.append(item);
  }
}

function openProviders() {
  closeModelPicker();
  providerError.hidden = true;
  providersDialog.showModal();
}

async function refreshProviders() {
  state.providers = (await request('GET', '/providers')) || [];
  renderProviders();
}

async function removeProvider(provider) {
  try {
    await request('DELETE', `/providers/${provider.id}`);
  } catch (error) {
    showToast(error.message, 'error');
    return;
  }

  state.models.delete(provider.id);
  state.providers = state.providers.filter((item) => item.id !== provider.id);
  renderProviders();
  showToast('Provider removed');
}

/* ── Actions ───────────────────────────────────────────────────────────── */

function startNewChat() {
  state.conversation = null;
  state.messages = [];
  state.draft = {
    providerId: state.providers.length === 1 ? state.providers[0].id : null,
    model: '',
  };

  chat.replaceChildren();
  setEmpty(true);
  setTitle('');
  updateModelButton();
  renderConversations();
  input.value = '';
  autoResize();
  updateComposerState();
  input.focus();
}

async function openConversation(id) {
  // Declared outside the try block: the catch must be able to leave early
  // while the success path still reads both values afterwards.
  let conversation;
  let messages;

  try {
    [conversation, messages] = await Promise.all([
      request('GET', `/conversations/${id}`),
      request('GET', `/conversations/${id}/messages`),
    ]);
  } catch (error) {
    showToast(error.message, 'error');
    return;
  }

  state.conversation = conversation;
  state.messages = messages || [];

  setTitle(conversation.title);
  renderMessages();
  updateModelButton();
  renderConversations();
  scrollToBottom(true);
  input.focus();
}

/** Creates the conversation on first send so drafts never reach the database. */
async function ensureConversation() {
  if (state.conversation) return state.conversation;

  if (!state.draft.providerId || !state.draft.model) {
    throw new Error('Select a provider and a model first');
  }

  const created = await request('POST', '/conversations', {
    provider_id: state.draft.providerId,
    model: state.draft.model,
  });

  state.conversation = created;
  state.conversations.unshift(created);
  renderConversations();

  return created;
}

/** Names a fresh conversation after its first message. */
async function autoTitle(conversationId, text) {
  const conversation = state.conversations.find((item) => item.id === conversationId);

  if (!conversation || conversation.title !== DEFAULT_TITLE) return;

  const title = text.replace(/\s+/g, ' ').trim().slice(0, 60);
  if (!title) return;

  try {
    const updated = await request('PATCH', `/conversations/${conversationId}`, { title });
    Object.assign(conversation, updated);

    if (state.conversation?.id === conversationId) {
      state.conversation = updated;
      setTitle(updated.title);
    }

    renderConversations();
  } catch {
    /* The title is cosmetic, a failure must not interrupt the chat. */
  }
}

function setStreaming(streaming) {
  state.streaming = streaming;
  state.controller = streaming ? state.controller : null;

  submitBtn.classList.toggle('is-stop', streaming);
  submitBtn.setAttribute('aria-label', streaming ? 'Stop generating' : 'Send message');
  submitBtn.innerHTML = streaming
    ? '<svg class="icon" aria-hidden="true"><use href="#i-stop" /></svg>'
    : '<svg class="icon" aria-hidden="true"><use href="#i-send" /></svg>';

  updateComposerState();
}

function updateComposerState() {
  submitBtn.disabled = !state.streaming && input.value.trim().length === 0;
}

function stopStreaming() {
  if (state.controller) state.controller.abort();
}

async function send(rawContent) {
  const content = rawContent.trim();
  if (!content || state.streaming) return;

  let conversation;

  try {
    conversation = await ensureConversation();
  } catch (error) {
    showToast(error.message, 'error');
    openModelPicker();
    return;
  }

  const shouldStick = isPinnedToBottom();

  chat.append(messageElement('user', content));
  setEmpty(false);

  input.value = '';
  autoResize();
  updateComposerState();
  scrollToBottom(true);

  const assistant = messageElement('assistant', '');
  assistant.classList.add('is-streaming');

  const body = assistant.querySelector('.msg__body');
  chat.append(assistant);
  scrollToBottom(true);

  setStreaming(true);
  announce('Aither is replying');

  void autoTitle(conversation.id, content);

  let reply = '';
  let failure = null;
  let stopped = false;

  try {
    await streamMessage(conversation.id, content, (name, data) => {
      if (name === 'delta' && data && typeof data.content === 'string') {
        reply += data.content;
        body.innerHTML = renderMarkdown(reply);
        if (shouldStick) scrollToBottom(true);
      } else if (name === 'error' && data && data.error) {
        failure = data.error;
      }
    });
  } catch (error) {
    if (error.name === 'AbortError') stopped = true;
    else failure = error.message;
  }

  assistant.classList.remove('is-streaming');
  setStreaming(false);

  if (failure) {
    assistant.remove();
    chat.append(errorElement(failure));
    showToast(failure, 'error');
    scrollToBottom(true);
    return;
  }

  if (stopped) {
    if (!reply.trim()) {
      assistant.remove();
      return;
    }

    const note = document.createElement('p');
    note.className = 'msg__stopped';
    note.textContent = 'Stopped';
    assistant.append(note);
    announce('Generation stopped');
    refreshConversations();
    return;
  }

  body.innerHTML = renderMarkdown(reply);
  state.messages.push({ role: 'user', content });
  state.messages.push({ role: 'assistant', content: reply });
  announce('Aither replied');
  scrollToBottom();

  refreshConversations({ silent: true });
}

async function refreshConversations() {
  state.conversations = (await request('GET', '/conversations')) || [];
  renderConversations();
}

/* ── Composer ──────────────────────────────────────────────────────────── */

function autoResize() {
  input.style.height = 'auto';
  input.style.height = `${Math.min(input.scrollHeight, 200)}px`;
}

/* ── Sidebar ───────────────────────────────────────────────────────────── */

function closeSidebar() {
  if (!MOBILE.matches) return;
  app.classList.remove('is-open');
  scrim.hidden = true;
  $('sidebarToggle').setAttribute('aria-expanded', 'false');
}

function toggleSidebar() {
  if (MOBILE.matches) {
    const open = app.classList.toggle('is-open');
    scrim.hidden = !open;
    $('sidebarToggle').setAttribute('aria-expanded', String(open));
    return;
  }

  const collapsed = app.classList.toggle('is-collapsed');
  $('sidebarToggle').setAttribute('aria-expanded', String(!collapsed));
}

/* ── Events ────────────────────────────────────────────────────────────── */

function wireEvents() {
  $('newChatBtn').addEventListener('click', () => {
    closeSidebar();
    startNewChat();
  });

  $('sidebarToggle').addEventListener('click', toggleSidebar);
  $('collapseBtn').addEventListener('click', toggleSidebar);
  scrim.addEventListener('click', closeSidebar);
  $('providersBtn').addEventListener('click', openProviders);
  $('providersClose').addEventListener('click', () => providersDialog.close());

  composerForm.addEventListener('submit', (event) => {
    event.preventDefault();
    if (state.streaming) stopStreaming();
    else send(input.value);
  });

  input.addEventListener('input', () => {
    autoResize();
    updateComposerState();
  });

  input.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      if (!state.streaming) send(input.value);
    }
  });

  modelBtn.addEventListener('click', () => {
    if (popover.hidden) openModelPicker();
    else closeModelPicker();
  });

  modelSearch.addEventListener('input', renderModelOptions);

  document.addEventListener('click', (event) => {
    if (popover.hidden) return;
    if (popover.contains(event.target) || modelBtn.contains(event.target)) return;
    closeModelPicker();
  });

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') closeModelPicker();
  });

  window.addEventListener('resize', closeModelPicker);
  chatScroll.addEventListener('scroll', closeModelPicker, { passive: true });

  document.addEventListener('click', async (event) => {
    const button = event.target.closest('[data-copy]');
    if (!button) return;

    const code = button.closest('.code')?.querySelector('code');
    if (!code) return;

    try {
      await navigator.clipboard.writeText(code.textContent);
      button.querySelector('span').textContent = 'Copied';
      window.setTimeout(() => {
        button.querySelector('span').textContent = 'Copy';
      }, 1600);
    } catch {
      showToast('Could not copy to clipboard', 'error');
    }
  });

  providerName.addEventListener('change', () => {
    baseURLField.hidden = providerName.value !== 'custom';
  });

  providerForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    providerError.hidden = true;
    providerSubmit.disabled = true;

    const name = providerName.value;
    const apiKey = providerKey.value.trim();

    if (!apiKey) {
      providerError.textContent = 'An API key is required.';
      providerError.hidden = false;
      providerSubmit.disabled = false;
      return;
    }

    const body = { provider_name: name, api_key: apiKey, base_url: '' };

    if (name === 'custom') {
      const baseURL = providerBaseURL.value.trim();

      if (!baseURL) {
        providerError.textContent = 'A base URL is required for custom providers.';
        providerError.hidden = false;
        providerSubmit.disabled = false;
        return;
      }

      body.base_url = baseURL;
    }

    try {
      await request('POST', '/providers', body);
      await refreshProviders();

      providerKey.value = '';
      providerBaseURL.value = '';
      showToast('Provider added');
    } catch (error) {
      providerError.textContent = error.message;
      providerError.hidden = false;
    } finally {
      providerSubmit.disabled = false;
    }
  });

  MOBILE.addEventListener('change', () => {
    if (!MOBILE.matches) {
      app.classList.remove('is-open');
      scrim.hidden = true;
    }
  });
}

/* ── Boot ──────────────────────────────────────────────────────────────── */

async function init() {
  wireEvents();

  try {
    await Promise.all([refreshProviders(), refreshConversations()]);
  } catch (error) {
    showToast(error.message, 'error');
  }

  startNewChat();

  if (!state.providers.length) {
    showToast('Connect a provider to start chatting');
    openProviders();
  }
}

init();
