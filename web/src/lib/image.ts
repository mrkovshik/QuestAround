// Сжатие фото перед отправкой: длинная сторона ≤ 1600px, JPEG 0.8.
// Экономит трафик и время загрузки на мобильной сети; заодно превращает
// HEIC с iPhone (если браузер умеет его декодировать) в JPEG.

const MAX_SIDE = 1600
const QUALITY = 0.8

export async function compressImage(file: File): Promise<Blob> {
  let bitmap: ImageBitmap | HTMLImageElement
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch {
    bitmap = await loadImage(file)
  }
  const w = bitmap.width
  const h = bitmap.height
  const scale = Math.min(1, MAX_SIDE / Math.max(w, h))
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(w * scale)
  canvas.height = Math.round(h * scale)
  const ctx = canvas.getContext('2d')
  if (!ctx) return file
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
  if ('close' in bitmap) bitmap.close()

  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', QUALITY))
  // Если сжатие не удалось или вышло больше оригинала — отправляем как есть.
  return blob && blob.size < file.size ? blob : file
}

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve(img)
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Не удалось открыть изображение'))
    }
    img.src = url
  })
}
