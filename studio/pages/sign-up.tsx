import { SignUpForm } from 'components/interfaces/SignIn/SignUpForm'
import { SignInLayout } from 'components/layouts'
import Link from 'next/link'
import type { NextPageWithLayout } from 'types'

/**
 * SupaDash Sign-Up Page
 * 
 * GitHub OAuth has been removed since SupaDash uses its own
 * GoTrue-compatible auth backend. Only email/password sign-up is supported.
 */
const SignUpPage: NextPageWithLayout = () => {
  return (
    <>
      <div className="flex flex-col gap-5">
        <SignUpForm />
      </div>

      <div className="my-8 self-center text-sm">
        <div>
          <span className="text-foreground-light">Have an account?</span>{' '}
          <Link
            href="/sign-in"
            className="underline text-foreground transition-colors hover:text-foreground-light"
          >
            Sign In
          </Link>
        </div>
      </div>
    </>
  )
}

SignUpPage.getLayout = (page) => (
  <SignInLayout
    heading="Get started"
    subheading="Create a new account"
    logoLinkToMarketingSite={true}
  >
    {page}
  </SignInLayout>
)

export default SignUpPage
