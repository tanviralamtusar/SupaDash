import AccountLayout from 'components/layouts/AccountLayout/AccountLayout'
import { AppLayout } from 'components/layouts/AppLayout/AppLayout'
import { DefaultLayout } from 'components/layouts/DefaultLayout'
import { useProfileQuery } from 'data/profile/profile-query'
import { Smartphone, Info, Loader2, QrCode } from 'lucide-react'
import type { NextPageWithLayout } from 'types'
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Badge,
  Button,
  cn,
  Collapsible_Shadcn_,
  CollapsibleContent_Shadcn_,
  CollapsibleTrigger_Shadcn_,
  Input,
  AlertDescription_Shadcn_,
  AlertTitle_Shadcn_,
  Alert_Shadcn_,
} from 'ui'
import { PageContainer } from 'ui-patterns/PageContainer'
import {
  PageHeader,
  PageHeaderDescription,
  PageHeaderMeta,
  PageHeaderSummary,
  PageHeaderTitle,
} from 'ui-patterns/PageHeader'
import { profileKeys } from 'data/profile/keys'

const collapsibleClasses = [
  'bg-surface-100',
  'hover:bg-surface-200',
  'data-open:bg-surface-200',
  'border-default',
  'hover:border-strong data-open:border-strong',
  'data-open:pb-px col-span-12 rounded',
  '-space-y-px overflow-hidden',
  'border shadow',
  'transition',
  'hover:z-50',
]

const Security: NextPageWithLayout = () => {
  const { data: profile, isLoading: isProfileLoading } = useProfileQuery()
  const queryClient = useQueryClient()

  const [isSettingUp, setIsSettingUp] = useState(false)
  const [setupData, setSetupData] = useState<{ secret: string; qr_uri: string } | null>(null)
  const [totpCode, setTotpCode] = useState('')
  const [isVerifying, setIsVerifying] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')
  const [isDisabling, setIsDisabling] = useState(false)

  const isEnabled = profile?.totp_enabled === true

  const handleSetup = async () => {
    setIsSettingUp(true)
    setErrorMsg('')
    try {
      const res = await fetch('/api/v1/auth/mfa/setup', {
        method: 'POST',
      })
      if (!res.ok) {
        const err = await res.json()
        throw new Error(err.error || 'Failed to initialize setup')
      }
      const data = await res.json()
      setSetupData(data)
    } catch (err: any) {
      setErrorMsg(err.message)
    } finally {
      setIsSettingUp(false)
    }
  }

  const handleVerify = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsVerifying(true)
    setErrorMsg('')
    try {
      const res = await fetch('/api/v1/auth/mfa/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ totp_code: totpCode }),
      })
      if (!res.ok) {
        const err = await res.json()
        throw new Error(err.error || 'Invalid TOTP code')
      }
      
      // Success! Refetch profile to update UI
      await queryClient.invalidateQueries(profileKeys.profile())
      setSetupData(null)
      setTotpCode('')
    } catch (err: any) {
      setErrorMsg(err.message)
    } finally {
      setIsVerifying(false)
    }
  }

  const handleDisable = async () => {
    if (!confirm('Are you sure you want to disable Two-Factor Authentication?')) return
    setIsDisabling(true)
    setErrorMsg('')
    try {
      const res = await fetch('/api/v1/auth/mfa', {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new Error('Failed to disable 2FA')
      }
      await queryClient.invalidateQueries(profileKeys.profile())
    } catch (err: any) {
      setErrorMsg(err.message)
    } finally {
      setIsDisabling(false)
    }
  }

  return (
    <>
      <PageHeader size="small">
        <PageHeaderMeta>
          <PageHeaderSummary>
            <PageHeaderTitle>Security</PageHeaderTitle>
            <PageHeaderDescription>
              Manage your SupaDash account security settings and two-step verification.
            </PageHeaderDescription>
          </PageHeaderSummary>
        </PageHeaderMeta>
      </PageHeader>
      
      <PageContainer size="small">
        <Collapsible_Shadcn_ className={cn('mt-8', collapsibleClasses)} defaultOpen>
          <CollapsibleTrigger_Shadcn_ asChild>
            <button
              type="button"
              className="group flex w-full items-center justify-between rounded py-3 px-4 md:px-6 text-foreground"
            >
              <div className="flex flex-row gap-4 items-center py-1">
                <Smartphone strokeWidth={1.5} />
                <span className="text-sm">Two-Factor Authentication (TOTP)</span>
              </div>

              {!isProfileLoading && (
                <Badge variant={isEnabled ? 'success' : 'default'}>
                  {isEnabled ? 'Enabled' : 'Disabled'}
                </Badge>
              )}
            </button>
          </CollapsibleTrigger_Shadcn_>
          
          <CollapsibleContent_Shadcn_ className="group border-t border-default bg-surface-100 py-6 px-4 md:px-6 text-foreground">
            {isProfileLoading ? (
               <div className="flex items-center justify-center py-8">
                 <Loader2 className="animate-spin text-foreground-light" />
               </div>
            ) : isEnabled ? (
              <div className="flex flex-col gap-6">
                <div>
                  <p className="text-sm text-foreground-light">
                    Two-factor authentication is currently enabled for your account. You will be required to enter a code from your authenticator app when signing in.
                  </p>
                </div>
                <div className="flex border-t border-default pt-6 justify-end">
                  <Button type="default" danger onClick={handleDisable} loading={isDisabling}>
                    Disable 2FA
                  </Button>
                </div>
              </div>
            ) : (
              <div className="flex flex-col gap-6">
                <div>
                   <p className="text-sm text-foreground-light">
                     Add an extra layer of security to your account. Once enabled, you will be required to enter both your password and a code generated by your authenticator app to sign in.
                   </p>
                </div>

                {!setupData && (
                  <div>
                    <Button onClick={handleSetup} loading={isSettingUp}>
                      Set up 2FA
                    </Button>
                  </div>
                )}

                {setupData && (
                  <form onSubmit={handleVerify} className="flex flex-col gap-6 border-t border-default pt-6">
                    <Alert_Shadcn_>
                      <Info className="h-4 w-4" />
                      <AlertTitle_Shadcn_>Scan this QR code</AlertTitle_Shadcn_>
                      <AlertDescription_Shadcn_>
                        Scan the QR code below with your authenticator app (like Google Authenticator, Authy, or 1Password), then enter the 6-digit code to verify setup.
                      </AlertDescription_Shadcn_>
                    </Alert_Shadcn_>

                    <div className="flex flex-col items-center gap-4 bg-surface-200 p-8 rounded-md border border-default border-dashed">
                      {/* Normally we'd use a QR library like qrcode.react, but we can generate an img using a public chart API for simplicity, or just show the secret */}
                      <img 
                        src={`https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(setupData.qr_uri)}`} 
                        alt="QR Code" 
                        className="bg-white p-2 rounded-lg"
                        style={{ width: 200, height: 200 }} 
                      />
                      <div className="text-center mt-2">
                        <p className="text-xs text-foreground-light mb-1">Cannot scan the code?</p>
                        <code className="text-sm bg-surface-300 px-2 py-1 rounded select-all font-mono">
                          {setupData.secret}
                        </code>
                      </div>
                    </div>

                    <div className="flex flex-col gap-2 max-w-sm">
                      <label htmlFor="code" className="text-sm font-medium">Verification Code</label>
                      <Input
                        id="code"
                        placeholder="000000"
                        value={totpCode}
                        onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                        autoComplete="one-time-code"
                        maxLength={6}
                      />
                    </div>

                    {errorMsg && (
                      <p className="text-destructive text-sm">{errorMsg}</p>
                    )}

                    <div className="flex gap-2 justify-end">
                      <Button type="default" onClick={() => { setSetupData(null); setErrorMsg(''); setTotpCode(''); }}>
                        Cancel
                      </Button>
                      <Button htmlType="submit" disabled={totpCode.length !== 6} loading={isVerifying}>
                        Verify & Enable
                      </Button>
                    </div>
                  </form>
                )}
              </div>
            )}
          </CollapsibleContent_Shadcn_>
        </Collapsible_Shadcn_>
      </PageContainer>
    </>
  )
}

Security.getLayout = (page) => (
  <AppLayout>
    <DefaultLayout headerTitle="Account">
      <AccountLayout title="Security">{page}</AccountLayout>
    </DefaultLayout>
  </AppLayout>
)

export default Security
