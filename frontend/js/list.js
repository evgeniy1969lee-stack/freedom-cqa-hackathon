import { api } from './api.js'
import { h, toast, fail, total, chip, empty, loading, errorBox, versionSelect, savedVersion } from './ui.js'

export async function listView(root, _arg, _params, ctx) {
  const selected = new Set()
  let convs = []
  let failed = new Map()
  let snapshot = ''
  let devReady = false

  const counts = h('div', { class: 'counts', 'aria-live': 'polite' })
  const body = h('div', null, loading())
  const fileInput = h('input', { type: 'file', accept: '.json,application/json', class: 'visually-hidden', id: 'import-file', onchange: importFile })
  const group = h('input', { type: 'text', id: 'group-id', placeholder: 'group_id', size: 10, 'aria-label': 'ID группы DEV' })
  const devBtn = h('button', { class: 'btn', onclick: importDev }, 'Загрузить DEV')
  const force = h('input', { type: 'checkbox', id: 'force-new' })
  const selBtn = h('button', { class: 'btn primary', onclick: () => evaluate([...selected]), disabled: true }, 'Оценить выбранные')
  const allBtn = h('button', { class: 'btn', onclick: () => evaluate([]) }, 'Оценить все')
  const versionBox = h('span', null)

  root.append(
    h('header', { class: 'page-head' }, h('h1', null, 'Диалоги'), counts),
    h('div', { class: 'commandbar', role: 'toolbar', 'aria-label': 'Действия' },
      h('div', { class: 'group' },
        h('label', { class: 'btn', for: 'import-file', tabindex: 0, onkeydown: e => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), fileInput.click()) }, 'Импорт JSON'),
        fileInput, group, devBtn),
      h('div', { class: 'group' },
        h('label', { for: 'checklist-version' }, 'Чек-лист'), versionBox,
        h('label', { class: 'check' }, force, 'Новый прогон'),
        selBtn, allBtn),
      h('div', { class: 'group' },
        h('a', { class: 'btn', href: api.exportURL({ format: 'json' }), download: '' }, 'Экспорт JSON'),
        h('a', { class: 'btn', href: api.exportURL({ format: 'csv' }), download: '' }, 'Экспорт CSV'))),
    body)

  async function load() {
    try {
      const [list, jobs, hl] = await Promise.all([api.conversations(), api.jobs({ status: 'failed' }), api.health()])
      if (!ctx.alive()) return
      const snap = JSON.stringify([list, jobs.jobs.map(j => [j.id, j.error]), hl.jobs, hl.dev_url_configured])
      devReady = hl.dev_url_configured
      devBtn.disabled = !devReady
      devBtn.title = devReady ? '' : 'DEV-ссылка не настроена на сервере (CQA_DEV_URL)'
      group.disabled = !devReady
      const c = hl.jobs
      counts.replaceChildren(
        h('span', null, `Диалогов: ${list.length}`),
        h('span', null, `В очереди: ${c.queued || 0}`),
        h('span', null, `Выполняется: ${c.running || 0}`),
        h('span', null, `Готово: ${c.completed || 0}`),
        h('span', { class: c.failed ? 'bad' : '' }, `Ошибок: ${c.failed || 0}`))
      if (snap === snapshot) return
      snapshot = snap
      convs = list
      failed = new Map(jobs.jobs.map(j => [j.id, j]))
      for (const id of [...selected]) if (!convs.some(c => c.conversation_id === id)) selected.delete(id)
      render()
    } catch (err) {
      if (ctx.alive()) body.replaceChildren(errorBox(err, load))
    }
  }

  async function loadVersions() {
    try {
      const list = await api.checklists()
      if (ctx.alive()) versionBox.replaceChildren(versionSelect(list, null, 'checklist-version'))
    } catch (err) {
      fail(err)
    }
  }

  function render() {
    selBtn.disabled = selected.size === 0
    selBtn.textContent = selected.size ? `Оценить выбранные (${selected.size})` : 'Оценить выбранные'
    if (!convs.length) {
      body.replaceChildren(empty('Диалогов пока нет', 'Загрузите JSON-файл с диалогами или группу DEV.',
        h('label', { class: 'btn primary', for: 'import-file' }, 'Импорт JSON')))
      return
    }
    const all = h('input', { type: 'checkbox', 'aria-label': 'Выбрать все', checked: selected.size === convs.length, onchange: e => {
      selected.clear()
      if (e.target.checked) convs.forEach(c => selected.add(c.conversation_id))
      render()
    } })
    body.replaceChildren(h('div', { class: 'table-wrap' }, h('table', { class: 'grid' },
      h('thead', null, h('tr', null,
        h('th', { class: 'w-check' }, all), h('th', null, 'ID диалога'), h('th', null, 'Канал'), h('th', null, 'Язык'),
        h('th', { class: 'num' }, 'Реплик'), h('th', null, 'Статус'), h('th', { class: 'num' }, 'Итог'), h('th', null, 'Грубая ошибка'), h('th', null, ''))),
      h('tbody', null, convs.map(row)))))
  }

  function row(c) {
    const id = c.conversation_id
    const job = c.latest_job_status === 'failed' ? failed.get(c.latest_job_id) : null
    const busy = c.latest_job_status === 'queued' || c.latest_job_status === 'running'
    return h('tr', { class: selected.has(id) ? 'selected' : '' },
      h('td', null, h('input', { type: 'checkbox', 'aria-label': `Выбрать ${id}`, checked: selected.has(id), onchange: e => {
        e.target.checked ? selected.add(id) : selected.delete(id)
        render()
      } })),
      h('td', null, h('a', { href: `#/c/${encodeURIComponent(id)}`, class: 'strong' }, id)),
      h('td', null, c.channel || '—', c.audio_url && h('span', { class: 'tag', title: 'Есть запись' }, '♪')),
      h('td', null, c.language || '—'),
      h('td', { class: 'num' }, c.turn_count),
      h('td', null, chip(c.latest_job_status), job && h('div', { class: 'err-text', title: job.error }, job.error)),
      h('td', { class: 'num strong' }, c.latest_evaluation_id ? total(c.latest_total) : '—'),
      h('td', null, c.latest_gross_error ? h('span', { class: 'chip failed' }, 'Да') : c.latest_evaluation_id ? 'Нет' : '—'),
      h('td', { class: 'actions' },
        job && h('button', { class: 'btn small', onclick: () => retry(job.id) }, 'Повторить'),
        h('button', { class: 'btn small', disabled: busy, onclick: () => evaluate([id]) }, 'Оценить')))
  }

  async function evaluate(ids) {
    try {
      const res = await api.evaluateBatch({ checklist_version: savedVersion(), conversation_ids: ids, force_new: force.checked })
      const errs = res.jobs.filter(j => j.error)
      const reused = res.jobs.length - res.created - errs.length
      toast(`Чек-лист v${res.checklist_version}: создано заданий ${res.created}` + (reused ? `, уже в работе или готово ${reused}` : ''))
      if (errs.length) toast(`${errs[0].conversation_id}: ${errs[0].error}` + (errs.length > 1 ? ` (и ещё ${errs.length - 1})` : ''), 'warn')
      selected.clear()
      snapshot = ''
      load()
      ctx.health()
    } catch (err) {
      fail(err)
    }
  }

  async function retry(id) {
    try {
      await api.retry(id)
      toast(`Задание #${id} поставлено в очередь повторно`)
      load()
    } catch (err) {
      fail(err)
    }
  }

  async function importFile() {
    const file = fileInput.files[0]
    fileInput.value = ''
    if (!file) return
    try {
      report(await api.importJSON(await file.text()), file.name)
    } catch (err) {
      fail(err)
    }
  }

  async function importDev() {
    devBtn.disabled = true
    devBtn.textContent = 'Загрузка…'
    try {
      report(await api.importDev(group.value.trim()), group.value.trim() ? `DEV ${group.value.trim()}` : 'DEV')
    } catch (err) {
      fail(err)
    } finally {
      devBtn.disabled = !devReady
      devBtn.textContent = 'Загрузить DEV'
    }
  }

  function report(res, source) {
    const n = { created: 0, updated: 0, unchanged: 0 }
    res.imported.forEach(r => n[r.status]++)
    toast(`${source}: новых ${n.created}, обновлено ${n.updated}, без изменений ${n.unchanged}`)
    snapshot = ''
    load()
  }

  loadVersions()
  await load()
  ctx.every(load, 2500)
}
