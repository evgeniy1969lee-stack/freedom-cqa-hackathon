import { api } from './api.js'
import { h } from './ui.js'
import { listView } from './list.js'
import { cardView } from './card.js'
import { editorView } from './editor.js'
import { appealsView, appealView } from './appeals.js'

const routes = [
  [/^\/?$/, 'list', () => listView],
  [/^\/c\/(.+)$/, 'list', () => cardView],
  [/^\/checklists$/, 'checklists', () => editorView],
  [/^\/appeals$/, 'appeals', () => appealsView],
  [/^\/appeals\/(\d+)$/, 'appeals', () => appealView],
]

const main = document.getElementById('main')
let token = 0
let timers = []

function route() {
  token++
  timers.forEach(clearInterval)
  timers = []
  const [path, query] = location.hash.replace(/^#/, '').split('?')
  const params = Object.fromEntries(new URLSearchParams(query || ''))
  let found = null
  for (const [re, nav, view] of routes) {
    const m = path.match(re)
    if (m) {
      found = { nav, view: view(), arg: m[1] && decodeURIComponent(m[1]) }
      break
    }
  }
  document.querySelectorAll('[data-nav]').forEach(a => {
    if (found && a.dataset.nav === found.nav) a.setAttribute('aria-current', 'page')
    else a.removeAttribute('aria-current')
  })
  main.replaceChildren()
  if (!found) {
    main.append(h('div', { class: 'empty' }, h('strong', null, 'Страница не найдена'), h('p', null, h('a', { href: '#/' }, 'К списку диалогов'))))
    return
  }
  const mine = token
  const ctx = {
    alive: () => mine === token,
    every: (fn, ms) => timers.push(setInterval(() => mine === token && fn(), ms)),
    go: hash => { location.hash = hash },
    refresh: route,
    health,
  }
  found.view(main, found.arg, params, ctx)
  health()
}

async function health() {
  const box = document.getElementById('health')
  try {
    const hl = await api.health()
    const stub = hl.ml === 'stub'
    box.replaceChildren(
      h('div', { class: stub ? 'warn' : 'ok' }, stub ? 'ML: заглушка' : 'ML: подключена'),
      h('div', null, hl.dev_url_configured ? 'DEV: настроен' : 'DEV: не настроен'),
      h('div', null, `Очередь: ${hl.jobs.queued || 0} · в работе: ${hl.jobs.running || 0}`))
  } catch {
    box.replaceChildren(h('div', { class: 'warn' }, 'Сервер недоступен'))
  }
}

window.addEventListener('hashchange', route)
setInterval(health, 10000)
route()
