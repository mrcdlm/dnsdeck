// Setzt Hell/Dunkel vor dem ersten Zeichnen (verhindert Aufblitzen).
// Extern statt inline, weil die Content-Security-Policy Inline-Skripte verbietet.
// Logik wie in src/lib/theme.ts.
;(function () {
  var pref = 'system'
  try {
    pref = localStorage.getItem('dnsdeck.theme') || 'system'
  } catch {
    // Speicher nicht verfügbar
  }
  var dark = pref === 'dark' || (pref !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
})()
