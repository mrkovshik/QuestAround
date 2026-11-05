// Минимальный роутер на History API.
//   /                         — ввод кода игры
//   /g/КОД                    — игра: регистрация или экран игры
//   /g/КОД/join/ИНВАЙТ        — вступление в команду по ссылке
//   /admin, /admin/runs/ID    — ведущий

export type Route = { page: 'home' } | { page: 'game'; code: string; invite?: string } | { page: 'admin'; runId?: string }

function parse(path: string): Route {
  const a = path.match(/^\/admin(?:\/runs\/([^/]+))?\/?$/)
  if (a) return { page: 'admin', runId: a[1] }
  const m = path.match(/^\/g\/([^/]+)(?:\/join\/([^/]+))?\/?$/)
  if (m) return { page: 'game', code: decodeURIComponent(m[1]).toUpperCase(), invite: m[2] && decodeURIComponent(m[2]).toUpperCase() }
  return { page: 'home' }
}

class Router {
  route = $state<Route>(parse(location.pathname))

  constructor() {
    addEventListener('popstate', () => (this.route = parse(location.pathname)))
  }

  go(path: string, replace = false) {
    if (replace) history.replaceState(null, '', path)
    else history.pushState(null, '', path)
    this.route = parse(path)
  }
}

export const router = new Router()
