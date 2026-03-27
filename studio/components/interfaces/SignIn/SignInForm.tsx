import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { useState } from 'react'
import { type SubmitHandler, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import z from 'zod'

import { useSendEventMutation } from 'data/telemetry/send-event-mutation'
import { auth } from 'lib/gotrue'
import { Button, Form_Shadcn_, FormControl_Shadcn_, FormField_Shadcn_, Input_Shadcn_ } from 'ui'
import { FormItemLayout } from 'ui-patterns/form/FormItemLayout/FormItemLayout'
import { Eye, EyeOff } from 'lucide-react'

// SupaDash API URL fallback
const AUTH_URL = process.env.NEXT_PUBLIC_SUPADASH_API_URL || 'http://localhost:8080'

const schema = z.object({
  email: z.string().min(1, 'Email is required').email('Must be a valid email'),
  password: z.string().min(1, 'Password is required'),
})

export const SignInForm = () => {
  const router = useRouter()
  const queryClient = useQueryClient()

  const [passwordHidden, setPasswordHidden] = useState(true)
  const [requires2FA, setRequires2FA] = useState(false)
  const [tempToken, setTempToken] = useState('')
  const [totpCode, setTotpCode] = useState('')
  
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  })
  
  const isSubmitting = form.formState.isSubmitting || requires2FA

  const { mutate: sendEvent } = useSendEventMutation()

  const handle2FA = async (e: React.FormEvent) => {
    e.preventDefault()
    if (totpCode.length !== 6) return
    const toastId = toast.loading('Verifying 2FA...')
    try {
      const res = await fetch(`${AUTH_URL}/auth/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ temp_token: tempToken, totp_code: totpCode }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Failed to verify')
      
      // Store session in supabase js to hydrate the rest of the application
      await auth.setSession({ access_token: data.access_token, refresh_token: data.refresh_token })
      toast.success('Signed in successfully!', { id: toastId })
      await queryClient.resetQueries()
      router.push('/organizations')
    } catch (err: any) {
      toast.error(`2FA failed: ${err.message}`, { id: toastId })
    }
  }

  const onSubmit: SubmitHandler<z.infer<typeof schema>> = async ({ email, password }) => {
    const toastId = toast.loading('Signing in...')
    try {
      const res = await fetch(`${AUTH_URL}/auth/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })
      const data = await res.json()
      
      if (!res.ok) throw new Error(data.error || 'Login failed')
      
      if (data.requires_2fa) {
        toast.success('Password correct. Enter 2FA code.', { id: toastId })
        setRequires2FA(true)
        setTempToken(data.temp_token)
        return
      }

      await auth.setSession({ access_token: data.access_token, refresh_token: data.refresh_token })
      toast.success('Signed in successfully!', { id: toastId })
      
      sendEvent({
        action: 'sign_in',
        properties: { category: 'account', method: 'email' },
      })

      await queryClient.resetQueries()
      router.push('/organizations')
    } catch (error: any) {
      toast.error(`Failed to sign in: ${error.message}`, { id: toastId })
    }
  }

  if (requires2FA) {
    return (
      <form className="flex flex-col gap-4" onSubmit={handle2FA}>
        <div className="flex flex-col gap-2">
          <label className="text-sm text-foreground">Authenticator Code</label>
          <Input_Shadcn_
            type="text"
            placeholder="000000"
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value)}
            disabled={requires2FA && isSubmitting}
            maxLength={6}
          />
        </div>
        <div className="flex gap-2">
          <Button block type="primary" htmlType="submit" size="large" disabled={totpCode.length !== 6}>
            Verify
          </Button>
          <Button block type="default" size="large" onClick={() => setRequires2FA(false)}>
            Cancel
          </Button>
        </div>
      </form>
    )
  }

  return (
    <Form_Shadcn_ {...form}>
      <form id="sign-in-form" className="flex flex-col gap-4" onSubmit={form.handleSubmit(onSubmit)}>
        <FormField_Shadcn_
          key="email"
          name="email"
          control={form.control}
          render={({ field }) => (
            <FormItemLayout name="email" label="Email">
              <FormControl_Shadcn_>
                <Input_Shadcn_
                  id="email"
                  type="email"
                  autoComplete="email"
                  {...field}
                  placeholder="you@example.com"
                  disabled={isSubmitting}
                />
              </FormControl_Shadcn_>
            </FormItemLayout>
          )}
        />

        <div className="relative">
          <FormField_Shadcn_
            key="password"
            name="password"
            control={form.control}
            render={({ field }) => (
              <FormItemLayout name="password" label="Password">
                <FormControl_Shadcn_>
                  <div className="relative">
                    <Input_Shadcn_
                      id="password"
                      type={passwordHidden ? 'password' : 'text'}
                      autoComplete="current-password"
                      {...field}
                      placeholder="&bull;&bull;&bull;&bull;&bull;&bull;&bull;&bull;"
                      disabled={isSubmitting}
                      className="pr-10"
                    />
                    <Button
                      type="default"
                      title={passwordHidden ? `Show password` : `Hide password`}
                      aria-label={passwordHidden ? `Show password` : `Hide password`}
                      className="absolute right-1 top-1 px-1.5"
                      icon={passwordHidden ? <Eye /> : <EyeOff />}
                      disabled={isSubmitting}
                      onClick={() => setPasswordHidden((prev) => !prev)}
                    />
                  </div>
                </FormControl_Shadcn_>
              </FormItemLayout>
            )}
          />

          <Link
            href="/forgot-password"
            className="absolute top-0 right-0 text-sm text-foreground-lighter"
          >
            Forgot password?
          </Link>
        </div>

        <Button block form="sign-in-form" htmlType="submit" size="large" loading={isSubmitting}>
          Sign in
        </Button>
      </form>
    </Form_Shadcn_>
  )
}
