import Link from 'next/link'

import SignInForm from 'components/interfaces/SignIn/SignInForm'
import { SignInLayout } from 'components/layouts'
import type { NextPageWithLayout } from 'types'

/**
 * SupaDash Sign-In Page
 * 
 * GitHub OAuth and SSO have been removed since SupaDash uses its own
 * GoTrue-compatible auth backend. Only email/password sign-in is supported.
 */
const SignInPage: NextPageWithLayout = () => {
  return (
    <>
      <div className="flex flex-col gap-5">
        <SignInForm />
      </div>

      <div className="my-8 self-center text-sm">
        <div>
          <span className="text-foreground-light">Don't have an account?</span>{' '}
          <Link
            href="/sign-up"
            className="underline text-foreground transition-colors hover:text-foreground-light"
          >
            Sign Up
          </Link>
        </div>
      </div>
    </>
  )
}

SignInPage.getLayout = (page) => (
  <SignInLayout
    heading="Welcome back"
    subheading="Sign in to your account"
    logoLinkToMarketingSite={true}
  >
    {page}
  </SignInLayout>
)

export default SignInPage
