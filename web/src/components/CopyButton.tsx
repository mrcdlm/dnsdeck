import { Check, Copy } from 'lucide-react'
import { useEffect, useState } from 'react'

import { Button } from '@/components/ui/button'

async function copyText(text: string) {
  // navigator.clipboard gibt es nur in sicheren Kontexten (HTTPS/localhost);
  // im LAN per http greift der Fallback.
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  document.execCommand('copy')
  ta.remove()
}

export function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!copied) return
    const id = setTimeout(() => setCopied(false), 1500)
    return () => clearTimeout(id)
  }, [copied])

  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      title={label}
      onClick={() => copyText(value).then(() => setCopied(true), () => {})}
    >
      {copied ? <Check className="text-success" /> : <Copy />}
    </Button>
  )
}
