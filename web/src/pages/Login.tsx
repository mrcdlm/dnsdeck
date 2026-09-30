import { Loader2, LogIn } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Navigate, useNavigate } from 'react-router'

import { Logo } from '@/components/Logo'
import { Preferences } from '@/components/Preferences'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useLogin, useSession } from '@/lib/queries'

export function Login() {
  const { t } = useTranslation()
  const session = useSession()
  const login = useLogin()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')

  if (session.data?.authenticated) {
    return <Navigate to="/" replace />
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    login.mutate(password, {
      onSuccess: () => navigate('/', { replace: true }),
      onError: () => setPassword(''),
    })
  }

  return (
    <div className="relative grid min-h-svh place-items-center px-4">
      <Preferences className="absolute top-4 right-4" />
      <div className="w-full max-w-sm">
        <Logo className="mb-6 justify-center text-lg" />
        <Card>
          <CardHeader>
            <CardTitle>{t('auth.title')}</CardTitle>
            <CardDescription>{t('auth.description')}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit} className="flex flex-col gap-4">
              <Input
                type="password"
                name="password"
                autoComplete="current-password"
                placeholder={t('auth.password')}
                aria-label={t('auth.password')}
                autoFocus
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                aria-invalid={login.isError || undefined}
              />
              {login.isError && (
                <p className="text-destructive text-sm" role="alert">
                  {login.error.message}
                </p>
              )}
              <Button type="submit" disabled={login.isPending || password === ''}>
                {login.isPending ? <Loader2 className="animate-spin" /> : <LogIn />}
                {t('auth.submit')}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
