// Токен игрока хранится в localStorage отдельно для каждой игры.

const key = (code: string) => `ql:token:${code.toUpperCase()}`

export function getToken(code: string): string | null {
  try {
    return localStorage.getItem(key(code))
  } catch {
    return null
  }
}

export function setToken(code: string, token: string) {
  try {
    localStorage.setItem(key(code), token)
  } catch {
    // приватный режим Safari: живём без сохранения
  }
}

export function clearToken(code: string) {
  try {
    localStorage.removeItem(key(code))
  } catch {
    /* ignore */
  }
}
