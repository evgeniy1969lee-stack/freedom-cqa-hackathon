import { api } from './api.js'
import { h, toast, fail, num, when, loading, errorBox, saveVersion } from './ui.js'

const clone = v => JSON.parse(JSON.stringify(v))

export async function editorView(root, _arg, params, ctx) {
  root.append(loading())
  let list, base
  try {
    list = await api.checklists()
    base = await api.checklist(Number(params.v) || 0)
  } catch (err) {
    if (ctx.alive()) root.replaceChildren(errorBox(err, ctx.refresh))
    return
  }
  if (!ctx.alive()) return

  const baseById = new Map(base.criteria.map(c => [c.criterion_id, JSON.stringify(c)]))
  const cl = clone(base)
  cl.criteria.forEach(c => { c.steps = c.steps || []; c._key = Math.random() })
  let saved = null

  const errors = h('div', { 'aria-live': 'assertive' })
  const criteria = h('ol', { class: 'editor-list' })
  const weightSum = h('span', { class: 'strong' })
  const after = h('div')

  const pick = h('select', { id: 'base-version', onchange: () => ctx.go(`#/checklists?v=${pick.value}`) },
    list.map(c => h('option', { value: String(c.version) }, `v${c.version} · ${when(c.created_at)} · ${c.criteria_count} крит.`)))
  pick.value = String(base.version)

  root.replaceChildren(
    h('header', { class: 'page-head' },
      h('div', null, h('h1', null, 'Редактор чек-листа'),
        h('p', { class: 'muted' }, 'Каждое сохранение создаёт новую неизменяемую версию. Прошлые оценки остаются привязаны к своей версии.')),
      h('div', { class: 'commandbar' }, h('div', { class: 'group' }, h('label', { for: 'base-version' }, 'Основа'), pick))),
    h('section', { class: 'pane' },
      h('div', { class: 'form-grid' },
        h('div', { class: 'field wide' }, h('label', { for: 'cl-note' }, 'Описание версии'),
          h('textarea', { id: 'cl-note', rows: 2, value: cl.note || '', oninput: e => { cl.note = e.target.value } })),
        h('div', { class: 'field' }, h('label', { for: 'cl-gross' }, 'Категории грубых ошибок (через запятую)'),
          h('input', { id: 'cl-gross', type: 'text', value: (cl.gross_error_categories || []).join(', '), oninput: e => {
            cl.gross_error_categories = e.target.value.split(',').map(s => s.trim()).filter(Boolean)
          } })),
        h('div', { class: 'field' }, h('span', { class: 'label' }, 'Статус'),
          h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: !!cl.draft, onchange: e => { cl.draft = e.target.checked } }), 'Черновик (веса не подтверждены)')))),
    h('div', { class: 'list-head' }, h('h2', null, 'Критерии'), h('span', null, 'Сумма весов: ', weightSum)),
    criteria,
    h('div', { class: 'row' }, h('button', { class: 'btn', onclick: () => { cl.criteria.push(blank(nextId())); draw(true) } }, '+ Добавить критерий')),
    errors,
    h('div', { class: 'savebar' }, h('button', { class: 'btn primary', onclick: save }, 'Сохранить как новую версию'), h('span', { class: 'muted' }, `Сейчас последняя: v${list[0].version}`)),
    after)

  function nextId() {
    const nums = cl.criteria.map(c => parseInt(c.criterion_id, 10)).filter(n => !isNaN(n))
    const n = (nums.length ? Math.max(...nums) : 0) + 1
    return String(n).padStart(2, '0')
  }

  function blank(id) {
    return { criterion_id: id, name: '', description: '', weight: 0, type: 'yes_no', steps: [], na_allowed: false, na_rule: '', section: '', _key: Math.random() }
  }

  function state(c) {
    const was = baseById.get(c.criterion_id)
    if (!was) return 'new'
    const { _key, ...rest } = c
    return JSON.stringify(strip(rest)) === JSON.stringify(strip(JSON.parse(was))) ? '' : 'changed'
  }

  function strip(c) {
    const out = { ...c, steps: c.type === 'scale' ? (c.steps || []) : [] }
    if (!out.na_allowed) out.na_rule = ''
    for (const k of ['na_rule', 'section', 'description']) out[k] = out[k] || ''
    return out
  }

  function badge(c) {
    const s = state(c)
    return h('span', { class: `badge ${s}` }, s === 'new' ? 'новый' : s === 'changed' ? 'изменён' : '')
  }

  function draw(focusLast) {
    weightSum.textContent = num(cl.criteria.reduce((a, c) => a + (Number(c.weight) || 0), 0))
    criteria.replaceChildren(...cl.criteria.map((c, i) => card(c, i)))
    if (focusLast) criteria.lastElementChild.querySelector('input').focus()
  }

  function card(c, i) {
    const k = c._key
    const mark = h('span', null, badge(c))
    const touch = () => {
      mark.replaceChildren(badge(c))
      weightSum.textContent = num(cl.criteria.reduce((a, x) => a + (Number(x.weight) || 0), 0))
    }
    const field = (label, input, cls = '') => h('div', { class: `field ${cls}` }, h('label', { for: `${input.id}` }, label), input)
    const text = (prop, attrs = {}) => h('input', { id: `${prop}-${k}`, type: 'text', value: c[prop] || '', oninput: e => { c[prop] = e.target.value; touch() }, ...attrs })
    const steps = c.type === 'scale' && h('fieldset', { class: 'steps' }, h('legend', null, 'Ступени шкалы'),
      c.steps.map((s, j) => h('div', { class: 'step-row' },
        h('input', { type: 'number', min: 0, step: 1, value: String(s.value), 'aria-label': `Значение ступени ${j + 1}`, oninput: e => { s.value = parseInt(e.target.value, 10); touch() } }),
        h('input', { type: 'text', value: s.label || '', placeholder: 'Описание ступени', 'aria-label': `Описание ступени ${j + 1}`, oninput: e => { s.label = e.target.value; touch() } }),
        h('button', { class: 'btn small', type: 'button', 'aria-label': `Удалить ступень ${j + 1}`, onclick: () => { c.steps.splice(j, 1); draw() } }, '✕'))),
      h('button', { class: 'btn small', type: 'button', onclick: () => {
        const max = c.steps.length ? Math.max(...c.steps.map(s => s.value || 0)) : -1
        c.steps.push({ value: max + 1, label: '' })
        draw()
      } }, '+ Ступень'))
    return h('li', { class: 'editor-card' },
      h('div', { class: 'editor-card-head' },
        h('strong', null, `${c.criterion_id || '?'} ${c.name || 'Новый критерий'}`), mark,
        h('div', { class: 'row' },
          h('button', { class: 'btn small', type: 'button', disabled: i === 0, 'aria-label': 'Выше', onclick: () => { cl.criteria.splice(i - 1, 0, cl.criteria.splice(i, 1)[0]); draw() } }, '↑'),
          h('button', { class: 'btn small', type: 'button', disabled: i === cl.criteria.length - 1, 'aria-label': 'Ниже', onclick: () => { cl.criteria.splice(i + 1, 0, cl.criteria.splice(i, 1)[0]); draw() } }, '↓'),
          h('button', { class: 'btn small', type: 'button', title: 'Заменить новым критерием с новым ID на этом месте', onclick: () => {
            const n = blank(nextId())
            n.weight = c.weight
            n.section = c.section
            cl.criteria[i] = n
            draw()
            document.getElementById(`name-${n._key}`).focus()
          } }, 'Заменить'),
          h('button', { class: 'btn small danger', type: 'button', onclick: () => { cl.criteria.splice(i, 1); draw() } }, 'Удалить'))),
      h('div', { class: 'form-grid' },
        field('ID', text('criterion_id', { onchange: () => draw() }), 'narrow'),
        field('Название', text('name')),
        field('Раздел', text('section')),
        field('Вес', h('input', { id: `weight-${k}`, type: 'number', min: 0, step: 'any', value: String(c.weight ?? 0), oninput: e => { c.weight = e.target.value === '' ? 0 : Number(e.target.value); touch() } }), 'narrow'),
        field('Тип ответа', h('select', { id: `type-${k}`, value: c.type, onchange: e => {
          c.type = e.target.value
          if (c.type === 'scale' && c.steps.length < 2) c.steps = [{ value: 0, label: '' }, { value: 1, label: '' }]
          draw()
        } }, h('option', { value: 'yes_no' }, 'Да / Нет'), h('option', { value: 'scale' }, 'Шкала')), 'narrow'),
        field('Формулировка', h('textarea', { id: `description-${k}`, rows: 2, value: c.description || '', oninput: e => { c.description = e.target.value; touch() } }), 'wide'),
        steps && h('div', { class: 'wide' }, steps),
        h('div', { class: 'field' }, h('span', { class: 'label' }, 'N/A'),
          h('label', { class: 'check' }, h('input', { type: 'checkbox', checked: !!c.na_allowed, onchange: e => { c.na_allowed = e.target.checked; draw() } }), 'Разрешён ответ N/A')),
        c.na_allowed && field('Условие N/A', h('textarea', { id: `na_rule-${k}`, rows: 2, value: c.na_rule || '', oninput: e => { c.na_rule = e.target.value; touch() } }), 'wide')))
  }

  function validate() {
    const errs = []
    const seen = new Set()
    if (!cl.criteria.length) errs.push('Нужен хотя бы один критерий.')
    for (const c of cl.criteria) {
      const id = (c.criterion_id || '').trim()
      const at = id || '(без ID)'
      if (!id) errs.push('У критерия пустой ID.')
      else if (seen.has(id)) errs.push(`ID ${id} повторяется.`)
      seen.add(id)
      if (!(c.name || '').trim()) errs.push(`${at}: укажите название.`)
      if (!(Number(c.weight) >= 0)) errs.push(`${at}: вес должен быть неотрицательным числом.`)
      if (c.type === 'scale') {
        const vals = c.steps.map(s => s.value)
        if (vals.length < 2) errs.push(`${at}: у шкалы нужно минимум две ступени.`)
        if (vals.some(v => !Number.isInteger(v) || v < 0)) errs.push(`${at}: ступени — целые неотрицательные числа.`)
        if (new Set(vals).size !== vals.length) errs.push(`${at}: значения ступеней повторяются.`)
        if (Math.max(...vals) <= 0) errs.push(`${at}: максимальная ступень должна быть больше нуля.`)
      }
    }
    return errs
  }

  function payload() {
    return {
      note: cl.note || '',
      draft: !!cl.draft,
      gross_error_categories: cl.gross_error_categories || [],
      criteria: cl.criteria.map(c => {
        const out = {
          criterion_id: c.criterion_id.trim(), name: c.name.trim(), description: c.description || '', weight: Number(c.weight) || 0,
          type: c.type, na_allowed: !!c.na_allowed, na_rule: c.na_allowed ? c.na_rule || '' : '', section: c.section || '',
        }
        if (c.type === 'scale') out.steps = [...c.steps].sort((a, b) => a.value - b.value).map(s => ({ value: s.value, label: s.label || '' }))
        return out
      }),
    }
  }

  function showErrors(errs) {
    errors.replaceChildren(errs.length ? h('div', { class: 'banner danger', role: 'alert' }, h('strong', null, 'Исправьте ошибки:'), h('ul', null, errs.map(e => h('li', null, e)))) : '')
  }

  async function save() {
    const errs = validate()
    showErrors(errs)
    if (errs.length) return errors.scrollIntoView({ block: 'center' })
    try {
      saved = await api.saveChecklist(payload())
      saveVersion(saved.version)
      toast(`Сохранена версия v${saved.version}`)
      applyPanel()
    } catch (err) {
      showErrors([err.message])
    }
  }

  async function applyPanel() {
    const convs = await api.conversations().catch(() => [])
    if (!ctx.alive()) return
    const sel = h('select', { id: 'apply-conv' }, convs.map(c => h('option', { value: c.conversation_id }, `${c.conversation_id} · ${c.channel} · ${c.language}`)))
    if (params.c) sel.value = params.c
    after.replaceChildren(h('section', { class: 'banner ok' },
      h('strong', null, `Версия v${saved.version} сохранена и выбрана для новых оценок. `),
      convs.length ? [
        h('label', { for: 'apply-conv' }, 'Применить к диалогу: '), sel,
        h('button', { class: 'btn primary small', onclick: async () => {
          try {
            await api.evaluate(sel.value, { checklist_version: saved.version })
            ctx.go(`#/c/${encodeURIComponent(sel.value)}`)
          } catch (err) { fail(err) }
        } }, 'Оценить'),
        h('button', { class: 'btn small', onclick: async () => {
          try {
            const r = await api.evaluateBatch({ checklist_version: saved.version })
            toast(`Создано заданий: ${r.created}`)
            ctx.go('#/')
          } catch (err) { fail(err) }
        } }, 'Оценить все'),
      ] : 'Диалогов для оценки пока нет.',
      h('button', { class: 'btn small', onclick: () => ctx.go(`#/checklists?v=${saved.version}`) }, 'Открыть v' + saved.version)))
    after.scrollIntoView({ block: 'center' })
  }

  draw()
}
