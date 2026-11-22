// Today's date in the browser's time zone as YYYY-MM-DD, the day that memories
// are taken for, so that it matches the dates the interface shows.
export function today(now = new Date()) {
  const month = String(now.getMonth() + 1).padStart(2, '0')
  const day = String(now.getDate()).padStart(2, '0')
  return `${now.getFullYear()}-${month}-${day}`
}
