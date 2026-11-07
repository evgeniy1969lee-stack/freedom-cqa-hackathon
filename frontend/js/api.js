export class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

function qs(params) {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params || {})) if (v !== undefined && v !== null && v !== '') q.set(k, v)
  const s = q.toString()
  return s ? '?' + s : ''
}

async function req(method, path, body) {
  const opt = { method, headers: {} }
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json'
    opt.body = typeof body === 'string' ? body : JSON.stringify(body)
  }
  let res
  try {
    res = await fetch('api/' + path, opt)
  } catch {
    throw new ApiError(0, 'Сервер недоступен')
  }
  const text = await res.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) throw new ApiError(res.status, (data && data.error) || `HTTP ${res.status}`)
  return data
}

const e = encodeURIComponent
const checklists = new Map()

export const api = {
  health: () => req('GET', 'health'),
  conversations: () => req('GET', 'conversations'),
  conversation: id => req('GET', `conversations/${e(id)}`),
  evaluations: id => req('GET', `conversations/${e(id)}/evaluations`),
  evaluation: id => req('GET', `evaluations/${id}`),
  evaluate: (id, body) => req('POST', `conversations/${e(id)}/evaluate`, body),
  evaluateBatch: body => req('POST', 'evaluate/batch', body),
  jobs: params => req('GET', 'jobs' + qs(params)),
  retry: id => req('POST', `jobs/${id}/retry`),
  importJSON: text => req('POST', 'import', text),
  importDev: groupId => req('POST', 'import/dev' + qs({ group_id: groupId })),
  checklists: () => req('GET', 'checklists'),
  checklist: async v => {
    if (v && checklists.has(v)) return checklists.get(v)
    const cl = await req('GET', `checklists/${v || 'latest'}`)
    checklists.set(cl.version, cl)
    return cl
  },
  saveChecklist: cl => req('POST', 'checklists', cl),
  createAppeal: (evId, body) => req('POST', `evaluations/${evId}/appeals`, body),
  appeals: params => req('GET', 'appeals' + qs(params)),
  appeal: id => req('GET', `appeals/${id}`),
  decide: (id, body) => req('POST', `appeals/${id}/decide`, body),
  exportURL: params => 'api/export' + qs(params),
}
