import { api } from './api.js'
import { h, put, toast, fail, total, num, when, chip, answer, answerLabel, evidenceTurns, evidenceLabel, role, stamp, empty, loading, errorBox, versionSelect, savedVersion } from './ui.js'

export async function cardView(root, id, params, ctx) {
  root.append(loading())
  let conv, evals, jobs, versions
  try {
    ;[conv, evals, jobs, versions] = await Promise.all([api.conversation(id), api.evaluations(id), api.jobs({ conversation_id: id }), api.checklists()])
  } catch (err) {
    if (!ctx.alive()) return
    root.replaceChildren(
      h('a', { class: 'back', href: '#/' }, '← Диалоги'),
      err.status === 404 ? empty('Диалог не найден', `Диалога «${id}» нет в базе.`) : errorBox(err, ctx.refresh))
    return
  }
  if (!ctx.alive()) return

  const ev = evals.find(e => String(e.id) === params.e) || evals[0] || null
  const cl = ev ? await api.checklist(ev.checklist_version).catch(() => null) : null
  if (!ctx.alive()) return
  const crits = new Map((cl ? cl.criteria : []).map(c => [c.criterion_id, c]))
  const active = jobs.jobs.find(j => j.status === 'queued' || j.status === 'running')
  const lastFailed = jobs.jobs[0] && jobs.jobs[0].status === 'failed' ? jobs.jobs[0] : null

  const turns = h('ol', { class: 'turns' }, conv.transcript.map(t =>
    h('li', { id: `turn-${t.turn_id}`, class: `turn r-${String(t.speaker).toLowerCase()}`, tabindex: -1 },
      h('div', { class: 'turn-meta' }, h('span', { class: 'who' }, role(t.speaker)), h('span', { title: t.timestamp }, stamp(t.timestamp)), h('span', { class: 'tid' }, `#${t.turn_id}`)),
      h('div', { class: 'turn-text' }, t.text))))

  function highlight(ids, from) {
    turns.querySelectorAll('.hl').forEach(n => n.classList.remove('hl'))
    root.querySelectorAll('.crit.active').forEach(n => n.classList.remove('active'))
    if (from) from.classList.add('active')
    const nodes = ids.map(i => document.getElementById(`turn-${i}`)).filter(Boolean)
    if (!nodes.length) return toast('Реплики с такими ID нет в транскрипте', 'warn')
    nodes.forEach(n => n.classList.add('hl'))
    nodes[0].scrollIntoView({ behavior: 'smooth', block: 'center' })
    nodes[0].focus({ preventScroll: true })
  }

  function evidence(e, from) {
    const label = evidenceLabel(e)
    if (!label) return h('span', { class: 'muted' }, 'без привязки к репликам')
    return h('button', { class: 'link', onclick: () => highlight(evidenceTurns(e), from) }, label)
  }

  const force = h('input', { type: 'checkbox' })
  const head = h('header', { class: 'page-head' },
    h('div', null,
      h('a', { class: 'back', href: '#/' }, '← Диалоги'),
      h('h1', null, conv.conversation_id),
      h('div', { class: 'meta' },
        h('span', { class: 'tag' }, conv.channel || 'канал не указан'), h('span', { class: 'tag' }, conv.language || '—'),
        h('span', null, `${conv.transcript.length} реплик`), h('span', null, `импорт ${when(conv.updated_at)}`))),
    h('div', { class: 'commandbar' }, h('div', { class: 'group' },
      h('label', { for: 'card-version' }, 'Чек-лист'), versionSelect(versions, null, 'card-version'),
      h('label', { class: 'check' }, force, 'Новый прогон'),
      h('button', { class: 'btn primary', disabled: !!active, onclick: run }, active ? 'Оценка идёт…' : 'Оценить'))))

  async function run() {
    try {
      const res = await api.evaluate(id, { checklist_version: savedVersion(), force_new: force.checked })
      toast(res.created ? `Задание #${res.job.id} создано` : `Задание #${res.job.id} уже существует (${res.job.status})`)
      reload()
    } catch (err) {
      fail(err)
    }
  }

  function reload(evId) {
    const target = `#/c/${encodeURIComponent(id)}` + (evId ? `?e=${evId}` : '')
    location.hash === target ? ctx.refresh() : ctx.go(target)
  }

  const audio = conv.audio_url && h('div', { class: 'audio' },
    h('audio', { controls: true, preload: 'none', src: conv.audio_url }),
    h('a', { href: conv.audio_url, target: '_blank', rel: 'noopener noreferrer' }, 'Открыть запись'))

  const right = h('section', { class: 'pane result', 'aria-label': 'Результат оценки' })
  if (active) {
    right.append(h('div', { class: 'banner info', role: 'status' }, h('strong', null, `Задание #${active.id}: `), chip(active.status),
      ` попытка ${active.attempts} из ${active.max_attempts}, чек-лист v${active.checklist_version}`))
    let known = active.status
    ctx.every(async () => {
      const j = await api.jobs({ conversation_id: id }).catch(() => null)
      const cur = j && j.jobs.find(x => x.id === active.id)
      if (!cur || !ctx.alive()) return
      if (cur.status === 'completed') return toast('Оценка готова') || reload(cur.evaluation_id)
      if (cur.status === 'failed') return reload()
      if (cur.status !== known) reload()
      known = cur.status
    }, 2000)
  }
  if (lastFailed && !active) {
    right.append(h('div', { class: 'banner danger', role: 'alert' }, h('strong', null, `Задание #${lastFailed.id} не выполнено: `), lastFailed.error,
      h('button', { class: 'btn small', onclick: async () => {
        try { await api.retry(lastFailed.id); toast('Задание поставлено в очередь'); reload() } catch (err) { fail(err) }
      } }, 'Повторить')))
  }

  if (!ev) {
    right.append(empty('Оценки ещё нет', 'Выберите версию чек-листа и нажмите «Оценить».'))
  } else {
    if (evals.length > 1) {
      const sel = h('select', { id: 'eval-pick', onchange: () => ctx.go(`#/c/${encodeURIComponent(id)}?e=${sel.value}`) },
        evals.map(e => h('option', { value: String(e.id) }, `Оценка #${e.id} · v${e.checklist_version} · ${e.model} · ${when(e.created_at)}`)))
      sel.value = String(ev.id)
      right.append(h('div', { class: 'field inline' }, h('label', { for: 'eval-pick' }, 'История'), sel))
    }
    if (ev.model === 'stub') right.append(h('div', { class: 'banner warn' }, h('strong', null, 'Результат-заглушка. '), 'ML-сервис не подключён, ответы ненастоящие.'))
    if (cl && cl.draft) right.append(h('div', { class: 'banner warn' }, h('strong', null, 'Черновой чек-лист. '), 'Веса и правила N/A не подтверждены по официальному PDF.'))
    right.append(scorePanel(ev), gross(ev), h('ol', { class: 'crits' }, ev.items.map(item)))
  }

  function scorePanel(ev) {
    const revised = ev.revised_at != null
    const shown = revised ? ev.final_total : ev.total
    return h('div', { class: 'score' },
      h('div', { class: 'score-main' },
        h('div', { class: 'score-num' }, total(shown)),
        h('div', { class: 'score-sub' }, revised ? 'итог после апелляций' : 'итоговый балл из 100'),
        shown == null && h('div', { class: 'muted small' }, 'Нет применимых критериев с ненулевым весом (W = 0).')),
      h('dl', { class: 'score-facts' },
        revised && [h('dt', null, 'Исходный итог'), h('dd', null, total(ev.total))],
        h('dt', null, 'W'), h('dd', null, num(revised ? ev.final_w : ev.w)),
        h('dt', null, 'Чек-лист'), h('dd', null, `v${ev.checklist_version}`),
        h('dt', null, 'Модель'), h('dd', null, ev.model || '—'),
        h('dt', null, 'Промпт'), h('dd', null, ev.prompt_version || '—'),
        h('dt', null, 'Оценка'), h('dd', null, `#${ev.id} · ${when(ev.created_at)}`)))
  }

  function gross(ev) {
    const g = ev.gross_error
    if (!g || !g.flag) return h('div', { class: 'gross none' }, h('strong', null, 'Грубая ошибка: '), 'не выявлена')
    const box = h('div', { class: 'gross flag crit', role: 'alert' })
    put(box, h('strong', null, 'Грубая ошибка'), g.category && h('span', { class: 'tag' }, g.category),
      g.reason && h('p', null, g.reason), h('div', null, 'Доказательство: ', evidence(g.evidence, box)),
      h('p', { class: 'muted small' }, 'Флаг отображается отдельно и не меняет арифметику итога.'))
    return box
  }

  function item(it) {
    const c = crits.get(it.criterion_id) || { criterion_id: it.criterion_id, name: it.criterion_id, type: 'yes_no', steps: [] }
    const appeals = (ev.appeals || []).filter(a => a.criterion_id === it.criterion_id)
    const open = appeals.find(a => a.status === 'open')
    const changed = it.final_answer != null && it.final_answer !== it.answer
    const eff = changed ? it.final_answer : it.answer
    const applicable = changed ? eff !== 'N/A' : it.applicable
    const max = c.type === 'scale' ? Math.max(...c.steps.map(s => s.value)) : 1
    const points = changed ? (eff === 'YES' ? 1 : c.type === 'scale' && eff !== 'NO' ? Number(eff) / max : 0) * it.weight : it.points
    const li = h('li', { class: `crit${applicable ? '' : ' na'}` })
    const comment = h('textarea', { id: `ap-${it.criterion_id}`, rows: 3, required: true, placeholder: 'Почему оценка неверна' })
    const form = h('form', { class: 'appeal-form', hidden: true, onsubmit: async e => {
      e.preventDefault()
      const text = comment.value.trim()
      if (!text) return comment.focus()
      try {
        const a = await api.createAppeal(ev.id, { criterion_id: it.criterion_id, comment: text })
        toast(`Апелляция #${a.id} создана`)
        reload(ev.id)
      } catch (err) {
        fail(err)
      }
    } },
      h('label', { for: `ap-${it.criterion_id}` }, 'Комментарий к оценке'),
      comment,
      h('div', { class: 'row' }, h('button', { class: 'btn primary small' }, 'Отправить'), h('button', { type: 'button', class: 'btn small', onclick: () => { form.hidden = true } }, 'Отмена')))
    put(li,
      h('div', { class: 'crit-head' },
        h('span', { class: 'cid' }, c.criterion_id),
        h('span', { class: 'cname' }, c.name),
        h('span', { class: 'cans' }, changed && h('s', { class: 'muted', title: 'Исходный ответ' }, answerLabel(it.answer, c)), answer(changed ? it.final_answer : it.answer, c))),
      h('div', { class: 'crit-facts' },
        applicable ? h('span', null, `Баллы ${num(points)} из ${num(it.weight)}`) : h('span', null, 'N/A — не входит в W'),
        h('span', null, `вес ${num(it.weight)}`), c.section && h('span', null, c.section)),
      !applicable && c.na_rule && h('p', { class: 'na-rule' }, h('strong', null, 'Правило N/A: '), c.na_rule),
      it.reason && h('p', { class: 'reason' }, it.reason),
      h('div', { class: 'crit-evidence' }, 'Доказательство: ', evidence(it.evidence, li)),
      c.description && h('details', null, h('summary', null, 'Формулировка критерия'), h('p', null, c.description)),
      appeals.length > 0 && h('ul', { class: 'appeal-list' }, appeals.map(a => h('li', null,
        h('a', { href: `#/appeals/${a.id}` }, `Апелляция #${a.id}`), ' ', chip(a.status),
        a.status === 'resolved' && ` ${a.decision === 'changed' ? 'изменено' : 'подтверждено'}: ${a.reason}`))),
      h('div', { class: 'row' }, h('button', { class: 'btn small', disabled: !!open, title: open ? 'По критерию уже есть открытая апелляция' : null, onclick: () => {
        form.hidden = false
        comment.focus()
      } }, 'Обжаловать')),
      form)
    return li
  }

  root.replaceChildren(head, h('div', { class: 'split' },
    h('section', { class: 'pane transcript', 'aria-label': 'Транскрипт' }, h('h2', null, 'Транскрипт'), audio, turns),
    right))
}
