import { api } from './api.js'
import { empty, errorBox, h } from './ui.js'
import { listView } from './list.js'
import { cardView } from './card.js'
import { editorView } from './editor.js'
import { appealsView, appealView } from './appeals.js'

const routes = [
  [/^\/?$/, 'list', listView],
  [/^\/c\/(.+)$/, 'list', cardView],
  [/^\/checklists$/, 'checklists', editorView],
  [/^\/appeals$/, 'appeals', appealsView],
  [/^\/appeals\/(\d+)$/, 'appeals', appealView],
]

const sectionNames = { list: 'Диалоги', checklists: 'Чек-лист', appeals: 'Апелляции' }
const main = document.getElementById('main')
let token = 0
let timers = []
let healthRequest
let lastStatus = ''

async function route(event) {
  token++
  timers.forEach(clearInterval)
  timers = []
  const [path, query] = location.hash.slice(1).split('?')
  const params = Object.fromEntries(new URLSearchParams(query || ''))
  let found = null
  for (const [re, nav, view] of routes) {
    const m = path.match(re)
    if (m) {
      try {
        found = { nav, view, arg: m[1] && decodeURIComponent(m[1]) }
      } catch {
        // A malformed encoded ID falls through to the missing-page state.
      }
      break
    }
  }
  document.querySelectorAll('[data-nav]').forEach(a => {
    if (found && a.dataset.nav === found.nav) a.setAttribute('aria-current', 'page')
    else a.removeAttribute('aria-current')
  })
  const section = found ? sectionNames[found.nav] : 'Страница не найдена'
  document.getElementById('section-name').textContent = section
  document.title = `${section} · Freedom CQA`
  main.replaceChildren()
  if (event?.type === 'hashchange') {
    main.focus({ preventScroll: true })
    window.scrollTo(0, 0)
  }
  if (!found) {
    main.append(empty('Страница не найдена', '', h('a', { href: '#/', class: 'btn' }, 'К списку диалогов')))
    return
  }
  const mine = token
  const alive = () => mine === token
  const ctx = {
    alive,
    every(fn, ms) {
      if (!alive()) return
      let running = false
      timers.push(setInterval(async () => {
        if (!alive() || running) return
        running = true
        try { await fn() } finally { running = false }
      }, ms))
    },
    go: hash => { if (alive()) location.hash = hash },
    refresh: () => { if (alive()) route() },
    health,
  }
  health()
  try {
    await found.view(main, found.arg, params, ctx)
  } catch (err) {
    if (alive()) main.replaceChildren(errorBox(err, route))
  }
}

function health() {
  if (!healthRequest) {
    healthRequest = api.health().catch(() => null).then(status => {
      showHealth(status)
      return status
    }).finally(() => { healthRequest = null })
  }
  return healthRequest
}

function showHealth(status) {
  const box = document.getElementById('health')
  const stub = status?.ml === 'stub'
  const state = status?.ml_ready ? 'ready' : stub ? 'stub' : 'offline'
  const model = status?.ml_model
  const label = status?.ml_ready ? `${model || 'ML'} · готова`
    : stub ? 'ML · тестовая заглушка'
    : status ? 'ML недоступна' : 'API недоступен'
  if (label === lastStatus) return
  lastStatus = label
  box.dataset.state = state
  box.replaceChildren(h('span', { class: 'engine-dot', 'aria-hidden': 'true' }), label)
  document.getElementById('rail-status').textContent = status ? 'API на связи' : 'Нет связи с API'
}

document.querySelector('.skip').addEventListener('click', e => {
  e.preventDefault()
  main.focus()
})
window.addEventListener('hashchange', route)
setInterval(health, 10000)
route()
