import { api } from './api.js'
import { h, toast, total, when, chip, answer, answerLabel, answerOptions, evidenceTurns, evidenceLabel, role, stamp, empty, loading, errorBox } from './ui.js'

async function context(evIds) {
  const evals = new Map()
  await Promise.all([...new Set(evIds)].map(async id => evals.set(id, await api.evaluation(id))))
  const cls = new Map()
  await Promise.all([...new Set([...evals.values()].map(e => e.checklist_version))].map(async v => cls.set(v, await api.checklist(v))))
  const crit = (ev, cid) => (cls.get(ev.checklist_version).criteria.find(c => c.criterion_id === cid)) || { criterion_id: cid, name: cid, type: 'yes_no', steps: [] }
  return { evals, crit }
}

export async function appealsView(root, _arg, params, ctx) {
  const status = params.status || ''
  const tabs = [['', 'Все'], ['open', 'Открытые'], ['resolved', 'Решённые']]
  root.append(
    h('header', { class: 'page-head' }, h('h1', null, 'Апелляции'),
      h('nav', { class: 'tabs', 'aria-label': 'Фильтр' }, tabs.map(([v, l]) =>
        h('a', { href: v ? `#/appeals?status=${v}` : '#/appeals', 'aria-current': v === status ? 'page' : null }, l)))),
    loading())
  try {
    const list = await api.appeals({ status })
    const { evals, crit } = await context(list.map(a => a.evaluation_id))
    if (!ctx.alive()) return
    root.lastChild.replaceWith(list.length ? h('div', { class: 'table-wrap', role: 'region', tabindex: 0, 'aria-label': 'Таблица апелляций' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null, ['№', 'Диалог', 'Критерий', 'Исходный ответ', 'Решение', 'Итоговый ответ', 'Создана', 'Статус'].map(t => h('th', null, t)))),
      h('tbody', null, list.map(a => {
        const ev = evals.get(a.evaluation_id)
        const c = crit(ev, a.criterion_id)
        return h('tr', null,
          h('td', null, h('a', { href: `#/appeals/${a.id}`, class: 'strong' }, `#${a.id}`)),
          h('td', null, h('a', { href: `#/c/${encodeURIComponent(ev.conversation_id)}?e=${ev.id}` }, ev.conversation_id)),
          h('td', null, `${c.criterion_id} ${c.name}`),
          h('td', null, answer(a.original_answer, c)),
          h('td', null, a.status === 'resolved' ? (a.decision === 'changed' ? 'Изменено' : 'Подтверждено') : '—'),
          h('td', null, a.status === 'resolved' ? answer(a.final_answer ?? a.original_answer, c) : '—'),
          h('td', null, when(a.created_at)),
          h('td', null, chip(a.status)))
      }))))
      : empty('Апелляций нет', 'Апелляцию можно подать из карточки диалога: кнопка «Обжаловать» у критерия.'))
  } catch (err) {
    if (ctx.alive()) root.lastChild.replaceWith(errorBox(err, ctx.refresh))
  }
}

export async function appealView(root, id, _params, ctx) {
  root.append(loading())
  let a, ev, conv, c
  try {
    a = await api.appeal(id)
    const res = await context([a.evaluation_id])
    ev = res.evals.get(a.evaluation_id)
    c = res.crit(ev, a.criterion_id)
    conv = await api.conversation(ev.conversation_id)
  } catch (err) {
    if (ctx.alive()) root.replaceChildren(h('a', { class: 'back', href: '#/appeals' }, '← Апелляции'),
      err.status === 404 ? empty('Апелляция не найдена') : errorBox(err, ctx.refresh))
    return
  }
  if (!ctx.alive()) return
  render()

  function render() {
    const it = ev.items.find(x => x.criterion_id === a.criterion_id)
    const historical = ev.input_hash !== conv.source_hash
    const ids = new Set(historical ? [] : evidenceTurns(it && it.evidence))
    const quoted = conv.transcript.filter(t => ids.has(t.turn_id))
    const revised = ev.revised_at != null
    const cardLink = `#/c/${encodeURIComponent(ev.conversation_id)}?e=${ev.id}`
    root.replaceChildren(
      h('header', { class: 'page-head' }, h('div', null,
        h('a', { class: 'back', href: '#/appeals' }, '← Апелляции'),
        h('h1', null, `Апелляция #${a.id} `, chip(a.status)),
        h('div', { class: 'meta' }, h('a', { href: cardLink }, `Диалог ${ev.conversation_id}`), h('span', null, `оценка #${ev.id}`),
          h('span', null, `чек-лист v${ev.checklist_version}`), h('span', null, `подана ${when(a.created_at)}`)))),
      h('div', { class: 'split' },
        h('section', { class: 'pane' },
          h('h2', null, `${c.criterion_id} ${c.name}`),
          c.description && h('p', null, c.description),
          h('dl', { class: 'facts' },
            h('dt', null, 'Тип'), h('dd', null, c.type === 'scale' ? `шкала ${c.steps.map(s => s.value).join(' / ')}` : 'да / нет'),
            h('dt', null, 'Вес'), h('dd', null, String(c.weight)),
            c.na_allowed && [h('dt', null, 'Условие N/A'), h('dd', null, c.na_rule || 'разрешён')],
            h('dt', null, 'Ответ модели'), h('dd', null, answer(a.original_answer, c)),
            it && it.reason && [h('dt', null, 'Причина модели'), h('dd', null, it.reason)]),
          h('h3', null, 'Доказательство', !historical && it && evidenceLabel(it.evidence) ? ` · ${evidenceLabel(it.evidence)}` : ''),
          quoted.length ? h('ol', { class: 'turns compact' }, quoted.map(t =>
            h('li', { class: `turn hl r-${String(t.speaker).toLowerCase()}` },
              h('div', { class: 'turn-meta' }, h('span', { class: 'who' }, role(t.speaker)), h('span', { title: t.timestamp }, stamp(t.timestamp)), h('span', { class: 'tid' }, `#${t.turn_id}`)),
              h('div', { class: 'turn-text' }, t.text))))
            : h('p', { class: 'muted' }, historical ? 'Эта оценка относится к предыдущей версии текста. Цитаты из неё недоступны.' : 'Модель не привязала ответ к репликам.'),
          h('a', { class: 'btn small', href: cardLink }, 'Открыть в карточке')),
        h('section', { class: 'pane' },
          h('h2', null, 'Комментарий'),
          h('blockquote', null, a.comment),
          h('div', { class: 'totals' },
            h('div', null, h('span', null, 'Исходный итог'), h('strong', null, total(ev.total))),
            h('div', null, h('span', null, revised ? 'Пересмотренный итог' : 'Текущий итог'), h('strong', null, total(revised ? ev.final_total : ev.total)))),
          a.status === 'resolved' ? decided() : decideForm())))
  }

  function decided() {
    return h('div', { class: `banner ${a.decision === 'changed' ? 'warn' : 'ok'}` },
      h('strong', null, a.decision === 'changed' ? 'Оценка изменена' : 'Оценка подтверждена'),
      h('p', null, answerLabel(a.original_answer, c), a.decision === 'changed' ? ` → ${answerLabel(a.final_answer, c)}` : ' без изменений'),
      h('p', null, h('strong', null, 'Причина: '), a.reason),
      h('p', { class: 'muted small' }, `Решена ${when(a.resolved_at)}`))
  }

  function decideForm() {
    const opts = answerOptions(c).filter(o => o !== String(a.original_answer))
    const pick = h('select', { id: 'new-answer', name: 'new_answer', disabled: true }, opts.map(o => h('option', { value: o }, answerLabel(o, c))))
    const err = h('div', { 'aria-live': 'assertive' })
    const reasonBox = h('textarea', { id: 'reason', rows: 3, required: true })
    const confirmed = h('input', { type: 'radio', name: 'decision', value: 'confirmed', onchange: () => { pick.disabled = true } })
    const changed = h('input', { type: 'radio', name: 'decision', value: 'changed', disabled: !opts.length, onchange: () => { pick.disabled = false } })
    const submit = h('button', { class: 'btn primary' }, 'Сохранить решение')
    return h('form', { class: 'decide', onsubmit: async e => {
      e.preventDefault()
      const decision = confirmed.checked ? 'confirmed' : changed.checked ? 'changed' : ''
      const reason = reasonBox.value.trim()
      if (!decision) return err.replaceChildren(h('p', { class: 'err-text' }, 'Выберите решение.'))
      if (!reason) return err.replaceChildren(h('p', { class: 'err-text' }, 'Укажите причину решения.')) || reasonBox.focus()
      const body = { decision, reason }
      if (decision === 'changed') body.new_answer = /^\d+$/.test(pick.value) ? Number(pick.value) : pick.value
      submit.disabled = true
      try {
        const res = await api.decide(a.id, body)
        if (!ctx.alive()) return
        a = res.appeal
        ev = res.evaluation
        toast(`Решение сохранено. Итог: ${total(ev.final_total)}`)
        ctx.health()
        render()
      } catch (e2) {
        err.replaceChildren(h('p', { class: 'err-text' }, e2.message))
      } finally {
        submit.disabled = false
      }
    } },
      h('h2', null, 'Решение проверяющего'),
      h('fieldset', null, h('legend', null, 'Решение'),
        h('label', { class: 'check' }, confirmed, 'Подтвердить исходную оценку'),
        h('label', { class: 'check' }, changed, 'Изменить ответ')),
      h('div', { class: 'field' }, h('label', { for: 'new-answer' }, 'Новый ответ'), pick),
      h('div', { class: 'field' }, h('label', { for: 'reason' }, 'Причина'), reasonBox),
      err,
      submit)
  }
}
