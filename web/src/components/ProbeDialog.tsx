import { Loader2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { Probe } from '@/lib/api'
import { useSaveProbe } from '@/lib/queries'

interface Props {
  /** Zu bearbeitende Prüfung; undefined = Dialog geschlossen */
  probe?: Probe
  onClose: () => void
}

export function ProbeDialog({ probe, onClose }: Props) {
  return (
    <Dialog open={probe !== undefined} onOpenChange={(open) => !open && onClose()}>
      <DialogContent>{probe && <ProbeForm key={probe.id} probe={probe} onDone={onClose} />}</DialogContent>
    </Dialog>
  )
}

function ProbeForm({ probe, onDone }: { probe: Probe; onDone: () => void }) {
  const { t } = useTranslation()
  const save = useSaveProbe()
  const [url, setURL] = useState(probe.url)
  const [expected, setExpected] = useState(probe.expected_status ?? '')
  const fromRecord = probe.record_id !== undefined

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate(
      { id: probe.id, input: { ...(fromRecord ? {} : { url }), expected_status: expected.trim() } },
      { onSuccess: onDone },
    )
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{t('checks.editTitle')}</DialogTitle>
        <DialogDescription>{t('checks.editHint')}</DialogDescription>
      </DialogHeader>

      <div className="grid gap-2">
        <Label htmlFor="probe-url">{t('checks.url')}</Label>
        <Input
          id="probe-url"
          value={url}
          onChange={(e) => setURL(e.target.value)}
          disabled={fromRecord}
          autoComplete="off"
          spellCheck={false}
          inputMode="url"
        />
        {fromRecord && <p className="text-muted-foreground text-xs">{t('checks.urlFromRecord')}</p>}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="probe-expected">{t('checks.expected')}</Label>
        <Input
          id="probe-expected"
          value={expected}
          onChange={(e) => setExpected(e.target.value)}
          placeholder={t('checks.expectedPlaceholder')}
          autoComplete="off"
          spellCheck={false}
        />
        <p className="text-muted-foreground text-xs">{t('checks.expectedHint')}</p>
      </div>

      {save.isError && (
        <p className="text-destructive text-sm" role="alert">
          {save.error.message}
        </p>
      )}

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={save.isPending || (!fromRecord && !url.trim())}>
          {save.isPending && <Loader2 className="animate-spin" />}
          {t('common.save')}
        </Button>
      </DialogFooter>
    </form>
  )
}
