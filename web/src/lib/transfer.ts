// Перенос игрока в VK-бот: когда сайт недоступен («белые списки»), игрок
// продолжает тем же игроком в сообщениях сообщества VK.
//
// Код считается из токена прямо в телефоне — сервер для этого не нужен.
// Тот же алгоритм — core.TransferCode на сервере.

const ALPHABET = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789'

async function sha256(text: string): Promise<Uint8Array> {
  return new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text)))
}

const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')

/** Код QA-XXXX-XXXX для токена "<run>.<player>.<secret>"; null — посчитать нельзя. */
export async function transferCode(token: string): Promise<string | null> {
  const secret = token.split('.')[2]
  if (!secret || !globalThis.crypto?.subtle) return null
  const tokenHash = hex(await sha256(secret))
  const h = await sha256('questaround-vk:' + tokenHash)
  let c = ''
  for (let i = 0; i < 8; i++) c += ALPHABET[h[i] % ALPHABET.length]
  return `QA-${c.slice(0, 4)}-${c.slice(4)}`
}

// Ссылку на сообщество запоминаем: без сети /state не загрузить.
const key = (code: string) => `qa:vk:${code.toUpperCase()}`

export function saveVKUrl(code: string, url: string | undefined) {
  try {
    if (url) localStorage.setItem(key(code), url)
    else localStorage.removeItem(key(code))
  } catch {
    /* ignore */
  }
}

export function loadVKUrl(code: string): string | null {
  try {
    return localStorage.getItem(key(code))
  } catch {
    return null
  }
}
