import { api } from './api.js'
import { h, icon, toast, fail, total, chip, empty, loading, errorBox, versionSelect } from './ui.js'

export async function listView(root, _arg, _params, ctx) {
  const selected = new Set()
  let convs = []
  let failed = new Map()
  let snapshot = ''
  let loadID = 0
  let devReady = false
  let importingDev = false
  let pending = false
  let introducedRows = false

  const counts = h('dl', { class: 'counts', 'aria-label': 'Сводка' })
  const body = h('div', null, loading())
  const file = h('input', { type: 'file', accept: '.json,application/json', hidden: true, onchange: importFile })
  const importButton = h('button', { class: 'btn primary', onclick: () => file.click() }, icon('upload'), 'Импорт JSON')
  const group = h('input', { type: 'text', id: 'group-id', placeholder: 'group_id', size: 10, disabled: true, 'aria-label': 'ID группы DEV' })
  const devBtn = h('button', { class: 'btn', disabled: true, onclick: importDev }, 'Загрузить DEV')
  const devHint = h('span', { class: 'muted small' }, 'Проверяем настройки DEV…')
  const force = h('input', { type: 'checkbox', id: 'force-new' })
  const selBtn = h('button', { class: 'btn primary', onclick: () => evaluate([...selected]), disabled: true }, icon('play'), h('span', null, 'Оценить выбранные'))
  const allBtn = h('button', { class: 'btn', onclick: () => evaluate([]), disabled: true }, 'Оценить все')
  const versionBox = h('span', { class: 'version-box' }, h('span', { class: 'muted' }, 'Загрузка…'))
  const found = h('span', { class: 'muted small' })
  const search = h('input', { type: 'search', placeholder: 'Поиск по диалогам', 'aria-label': 'Поиск по диалогам', oninput: () => render() })
  const filter = h('select', { 'aria-label': 'Статус оценки', onchange: () => render() },
    [['', 'Все статусы'], ['none', 'Не оценён'], ['queued', 'В очереди'], ['running', 'Выполняется'], ['completed', 'Готово'], ['failed', 'Ошибка']]
      .map(([value, name]) => h('option', { value }, name)))

  root.append(
    h('header', { class: 'hero' },
      h('div', { class: 'hero-art', 'aria-hidden': 'true' }),
      h('div', { class: 'hero-sweep', 'aria-hidden': 'true' }),
      h('div', { class: 'hero-copy' },
        h('h1', null, 'Диалоги поддержки'),
        h('p', null, 'Загружайте диалоги, запускайте AI-оценку и проверяйте выводы по репликам.'),
        h('div', { class: 'hero-actions' }, importButton, file,
          h('a', { class: 'hero-link', href: '#/checklists' }, 'Чек-лист оценки', icon('arrow'))))),
    counts,
    h('section', { class: 'worklist', 'aria-label': 'Реестр диалогов' },
      h('div', { class: 'worklist-head' },
        h('div', null, h('h2', null, 'Реестр диалогов'), found),
        h('div', { class: 'group filters' }, h('div', { class: 'search' }, icon('search'), search), filter)),
      h('div', { class: 'worklist-actions' },
        h('div', { class: 'group' },
          h('label', { for: 'checklist-version' }, 'Чек-лист'), versionBox,
          h('label', { class: 'check' }, force, 'Новый прогон'), selBtn, allBtn),
        h('div', { class: 'group exports', 'aria-label': 'Экспорт оценок' },
          icon('download'),
          h('a', { class: 'btn small', href: api.exportURL({ format: 'json' }), download: '', 'aria-label': 'Экспорт JSON' }, 'JSON'),
          h('a', { class: 'btn small', href: api.exportURL({ format: 'csv' }), download: '', 'aria-label': 'Экспорт CSV' }, 'CSV'))),
      body,
      h('details', { class: 'dev-tools' },
        h('summary', null, 'Импорт из DEV'),
        h('div', { class: 'group' }, group, devBtn, devHint))))

  function updateDev(status) {
    if (!ctx.alive()) return
    devReady = Boolean(status?.dev_url_configured)
    devBtn.disabled = !devReady || importingDev
    group.disabled = devBtn.disabled
    devHint.textContent = devReady ? 'Оставьте поле пустым для импорта всей выборки.' : 'DEV-источник не настроен на сервере.'
  }

  async function load() {
    const mine = ++loadID
    try {
      const [list, jobs] = await Promise.all([api.conversations(), api.jobs({ status: 'failed' })])
      if (!ctx.alive() || mine !== loadID) return
      const c = jobs.counts
      const snap = JSON.stringify([list, jobs.jobs.map(j => [j.id, j.error]), c])
      if (snap === snapshot) return
      snapshot = snap
      convs = list
      failed = new Map(jobs.jobs.map(j => [j.id, j]))
      const ids = new Set(convs.map(x => x.conversation_id))
      for (const id of selected) if (!ids.has(id)) selected.delete(id)
      counts.replaceChildren(
        metric('Всего диалогов', convs.length, 'В реестре'),
        metric('Оценено', convs.filter(x => x.latest_evaluation_id).length, 'Диалогов с результатом'),
        metric('В обработке', (c.queued || 0) + (c.running || 0), `${c.queued || 0} в очереди · ${c.running || 0} выполняется`, 'in-progress'),
        metric('Ошибки заданий', c.failed || 0, 'По всем прогонам', c.failed ? 'bad' : ''))
      render()
    } catch (err) {
      if (!ctx.alive() || mine !== loadID) return
      snapshot = ''
      body.replaceChildren(errorBox(err, load))
    }
  }

  function metric(label, value, description, tone = '') {
    return h('div', { class: `metric ${tone}` }, h('dt', null, label), h('dd', { class: 'metric-value' }, value), h('dd', { class: 'metric-detail' }, description))
  }

  async function loadVersions() {
    try {
      const list = await api.checklists()
      if (ctx.alive()) versionBox.replaceChildren(versionSelect(list, null, 'checklist-version'))
    } catch (err) {
      if (ctx.alive()) fail(err)
    }
  }

  function updateSelection(visible, table) {
    const checked = visible.filter(c => selected.has(c.conversation_id)).length
    const all = table.querySelector('thead input')
    all.checked = checked > 0 && checked === visible.length
    all.indeterminate = checked > 0 && checked < visible.length
    table.querySelectorAll('tbody input[type="checkbox"]').forEach((box, index) => {
      box.checked = selected.has(visible[index].conversation_id)
      box.closest('tr').classList.toggle('selected', box.checked)
    })
    updateActions()
  }

  function updateActions() {
    selBtn.disabled = pending || !selected.size
    selBtn.lastElementChild.textContent = selected.size ? `Оценить выбранные (${selected.size})` : 'Оценить выбранные'
    allBtn.disabled = pending || !convs.length
  }

  function render() {
    const query = search.value.trim().toLowerCase()
    const visible = convs.filter(c => (!filter.value || (c.latest_job_status || 'none') === filter.value)
      && (!query || [c.conversation_id, c.language, c.channel].some(value => value?.toLowerCase().includes(query))))
    found.textContent = visible.length === convs.length ? `Всего ${convs.length}` : `Показано ${visible.length} из ${convs.length}`
    updateActions()
    if (!convs.length) {
      body.replaceChildren(empty('Диалогов пока нет', 'Первый JSON-файл можно загрузить прямо сейчас.',
        h('button', { class: 'btn primary', onclick: () => file.click() }, icon('upload'), 'Импорт JSON')))
      return
    }
    if (!visible.length) {
      body.replaceChildren(empty('Диалоги не найдены', 'Измените запрос или сбросьте фильтры.',
        h('button', { class: 'btn', onclick: () => { search.value = ''; filter.value = ''; search.focus(); render() } }, 'Сбросить фильтры')))
      return
    }

    const focused = body.contains(document.activeElement) && document.activeElement.dataset.focus
    const all = h('input', { type: 'checkbox', 'aria-label': 'Выбрать показанные диалоги', 'data-focus': 'all', onchange: e => {
      for (const { conversation_id: id } of visible) {
        if (e.target.checked) selected.add(id)
        else selected.delete(id)
      }
      updateSelection(visible, table)
    } })
    const table = h('table', { class: 'grid' },
      h('thead', null, h('tr', null,
        h('th', { class: 'w-check', scope: 'col' }, all), h('th', { scope: 'col' }, 'Диалог'), h('th', { scope: 'col' }, 'Канал'), h('th', { scope: 'col' }, 'Язык'),
        h('th', { class: 'num', scope: 'col' }, 'Реплик'), h('th', { scope: 'col' }, 'Статус'), h('th', { class: 'num', scope: 'col' }, 'Балл'), h('th', { scope: 'col' }, 'Грубая ошибка'), h('th', { scope: 'col' }, h('span', { class: 'visually-hidden' }, 'Действия')))),
      h('tbody', { class: introducedRows ? 'dialog-rows' : 'dialog-rows intro' }, visible.map(c => row(c, () => updateSelection(visible, table)))))
    introducedRows = true
    const scroll = body.firstElementChild?.scrollLeft || 0
    const wrap = h('div', { class: 'table-wrap', tabindex: 0, role: 'region', 'aria-label': 'Таблица диалогов' }, table)
    body.replaceChildren(wrap)
    wrap.scrollLeft = scroll
    updateSelection(visible, table)
    if (focused) [...table.querySelectorAll('[data-focus]')].find(el => el.dataset.focus === focused)?.focus({ preventScroll: true })
  }

  function row(c, changed) {
    const id = c.conversation_id
    const job = c.latest_job_status === 'failed' ? failed.get(c.latest_job_id) : null
    const busy = c.latest_job_status === 'queued' || c.latest_job_status === 'running'
    return h('tr', null,
      h('td', null, h('input', { type: 'checkbox', 'aria-label': `Выбрать ${id}`, 'data-focus': `pick-${id}`, onchange: e => {
        if (e.target.checked) selected.add(id)
        else selected.delete(id)
        changed()
      } })),
      h('td', null, h('a', { href: `#/c/${encodeURIComponent(id)}`, class: 'dialog-link', 'data-focus': `view-${id}` }, icon('chat'), h('span', null, id))),
      h('td', null, c.channel || '—', c.audio_url && h('span', { class: 'tag', title: 'Есть запись' }, 'Аудио')),
      h('td', null, c.language || '—'),
      h('td', { class: 'num' }, c.turn_count),
      h('td', null, chip(c.latest_job_status), job && h('div', { class: 'err-text', title: job.error }, job.error)),
      h('td', { class: 'num strong' }, c.latest_evaluation_id ? total(c.latest_total) : '—'),
      h('td', null, c.latest_gross_error ? h('span', { class: 'chip failed' }, 'Да') : c.latest_evaluation_id ? 'Нет' : '—'),
      h('td', { class: 'actions' },
        job && h('button', { class: 'btn small', onclick: e => retry(job.id, e.currentTarget) }, 'Повторить'),
        h('button', { class: 'btn small', disabled: busy, 'data-focus': `run-${id}`, onclick: () => evaluate([id]) }, icon('play'), 'Оценить')))
  }

  async function evaluate(ids) {
    if (pending) return
    pending = true
    updateActions()
    try {
      const res = await api.evaluateBatch({ checklist_version: Number(document.getElementById('checklist-version')?.value) || 0, conversation_ids: ids, force_new: force.checked })
      const errs = res.jobs.filter(j => j.error)
      const reused = res.jobs.length - res.created - errs.length
      toast(`Чек-лист v${res.checklist_version} · новых заданий ${res.created}` + (reused ? `, уже в работе или готово ${reused}` : ''))
      if (errs.length) toast(`${errs[0].conversation_id}: ${errs[0].error}` + (errs.length > 1 ? ` (и ещё ${errs.length - 1})` : ''), 'warn')
      selected.clear()
      snapshot = ''
      if (ctx.alive()) await load()
    } catch (err) {
      fail(err)
    } finally {
      pending = false
      updateActions()
    }
  }

  async function retry(id, button) {
    button.disabled = true
    try {
      await api.retry(id)
      toast(`Задание #${id} поставлено в очередь повторно`)
      if (ctx.alive()) await load()
    } catch (err) {
      fail(err)
    } finally {
      button.disabled = false
    }
  }

  async function importFile() {
    const input = file.files[0]
    file.value = ''
    if (!input) return
    importButton.disabled = true
    try {
      report(await api.importJSON(await input.text()), input.name)
    } catch (err) {
      fail(err)
    } finally {
      importButton.disabled = false
    }
  }

  async function importDev() {
    importingDev = true
    devBtn.disabled = true
    const id = group.value.trim()
    group.disabled = true
    devBtn.textContent = 'Загрузка…'
    try {
      report(await api.importDev(id), id ? `DEV ${id}` : 'DEV')
    } catch (err) {
      fail(err)
    } finally {
      importingDev = false
      devBtn.disabled = !devReady
      group.disabled = !devReady
      devBtn.textContent = 'Загрузить DEV'
    }
  }

  function report(res, source) {
    const n = { created: 0, updated: 0, unchanged: 0 }
    res.imported.forEach(r => n[r.status]++)
    toast(`${source} · новых ${n.created}, обновлено ${n.updated}, без изменений ${n.unchanged}`)
    if (ctx.alive()) load()
  }

  loadVersions()
  ctx.health().then(updateDev)
  await load()
  ctx.every(load, 2500)
  ctx.every(async () => updateDev(await ctx.health()), 10000)
}
