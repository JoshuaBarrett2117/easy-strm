export const createActionTrace = () => {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  let milliseconds = Date.now()
  for (let index = 5; index >= 0; index--) {
    bytes[index] = milliseconds % 256
    milliseconds = Math.floor(milliseconds / 256)
  }
  bytes[6] = (bytes[6] & 15) | 112
  bytes[8] = (bytes[8] & 63) | 128
  const hexadecimal = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
  return `${hexadecimal.slice(0, 8)}-${hexadecimal.slice(8, 12)}-${hexadecimal.slice(12, 16)}-${hexadecimal.slice(16, 20)}-${hexadecimal.slice(20)}`
}
