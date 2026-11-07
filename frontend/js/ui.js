export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag)
  let value
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue
    if (k === 'value') value = v
    else if (k.startsWith('on')) el.addEventListener(k.slice(2), v)
    else if (k === 'class') el.className = v
    else el.setAttribute(k, v === true ? '' : v)
  }
  add(el, kids)
  if (value !== undefined) el.value = value
  return el
}

export function put(el, ...kids) {
  add(el, kids)
  return el
}

function add(el, kids) {
  for (const k of kids) {
    if (k == null || k === false) continue
    if (Array.isArray(k)) add(el, k)
    else el.append(k instanceof Node ? k : String(k))
  }
}

export function toast(msg, kind = 'ok') {
  const box = document.getElementById('toasts')
  const t = h('div', { class: `toast ${kind}` }, msg)
  box.append(t)
  setTimeout(() => t.remove(), kind === 'error' ? 8000 : 4000)
}

export function fail(err) {
  toast(err && err.message ? err.message : String(err), 'error')
}

export function total(n) {
  return n == null ? '—' : Number(n).toFixed(2)
}

export function num(n) {
  return Number.isInteger(n) ? String(n) : Number(n).toFixed(2).replace(/\.?0+$/, '')
}

export function when(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' })
}

const statusText = { queued: 'В очереди', running: 'Выполняется', completed: 'Готово', failed: 'Ошибка', open: 'Открыта', resolved: 'Решена' }

export function stamp(t) {
  const m = /^\d{4}-\d{2}-\d{2}[T ](\d{2}:\d{2}:\d{2})/.exec(t || '')
  return m ? m[1] : t
}

export function chip(status) {
  if (!status) return h('span', { class: 'chip none' }, 'Не оценён')
  return h('span', { class: `chip ${status}` }, statusText[status] || status)
}

const answerText = { YES: 'Да', NO: 'Нет', 'N/A': 'N/A' }

export function answerLabel(a, crit) {
  if (a == null) return '—'
  if (answerText[a]) return answerText[a]
  if (crit && crit.type === 'scale') {
    const max = Math.max(...crit.steps.map(s => s.value))
    const step = crit.steps.find(s => String(s.value) === String(a))
    return `${a} из ${max}${step && step.label ? ' · ' + step.label : ''}`
  }
  return String(a)
}

export function answer(a, crit) {
  const cls = a === 'YES' ? 'yes' : a === 'NO' ? 'no' : a === 'N/A' ? 'na' : 'step'
  return h('span', { class: `ans ${cls}` }, answerLabel(a, crit))
}

export function answerOptions(crit) {
  const opts = crit.type === 'scale' ? crit.steps.map(s => String(s.value)) : ['YES', 'NO']
  if (crit.na_allowed) opts.push('N/A')
  return opts
}

export function evidenceTurns(ev) {
  if (!ev) return []
  if (ev.kind === 'turns') return ev.turn_ids || []
  if (ev.kind === 'range' && ev.from != null && ev.to != null) {
    const out = []
    for (let i = ev.from; i <= ev.to; i++) out.push(i)
    return out
  }
  return []
}

export function evidenceLabel(ev) {
  if (!ev || !ev.kind || ev.kind === 'none') return null
  if (ev.kind === 'range') return `реплики #${ev.from}–#${ev.to}`
  return (ev.turn_ids || []).map(id => `#${id}`).join(', ')
}

const roles = { operator: 'Оператор', client: 'Клиент', bot: 'Бот', agent: 'Оператор', customer: 'Клиент' }

export function role(speaker) {
  return roles[String(speaker).toLowerCase()] || speaker || '—'
}

export function empty(title, text, ...actions) {
  return h('div', { class: 'empty' }, h('strong', null, title), text && h('p', null, text), actions)
}

export function loading() {
  return h('div', { class: 'loading', role: 'status' }, 'Загрузка…')
}

export function errorBox(err, retry) {
  return h('div', { class: 'banner danger', role: 'alert' },
    h('strong', null, 'Не удалось загрузить данные. '), err.message || String(err),
    retry && h('button', { class: 'btn small', onclick: retry }, 'Повторить'))
}

const pref = 'cqa.checklist'

export function savedVersion() {
  try { return Number(localStorage.getItem(pref)) || 0 } catch { return 0 }
}

export function saveVersion(v) {
  try { localStorage.setItem(pref, String(v)) } catch {}
}

export function versionSelect(list, onchange, id) {
  const cur = savedVersion()
  const sel = h('select', { id, onchange: () => { saveVersion(sel.value); onchange && onchange(Number(sel.value)) } },
    h('option', { value: '0' }, `Последняя (v${list[0] ? list[0].version : '—'})`),
    list.map(c => h('option', { value: String(c.version) }, `v${c.version} · ${c.criteria_count} крит.`)))
  sel.value = list.some(c => c.version === cur) ? String(cur) : '0'
  return sel
}
