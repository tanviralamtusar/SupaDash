import { useQueryClient } from '@tanstack/react-query'
import { useRouter } from 'next/router'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'

import { useParams } from 'common'
import Panel from 'components/ui/Panel'
import { useOrganizationCreateMutation } from 'data/organizations/organization-create-mutation'
import { invalidateOrganizationsQuery } from 'data/organizations/organizations-query'
import { Button, Input } from 'ui'

/**
 * SupaDash Simplified Org Creation
 *
 * Stripped of all Stripe/billing logic:
 * - No pricing plan selection
 * - No payment method collection
 * - No spend cap toggle
 * - No org kind/size selectors
 * - Always creates with enterprise tier (self-hosted)
 */

interface NewOrgFormProps {
  onPaymentMethodReset?: () => void
}

const NewOrgForm = ({ onPaymentMethodReset }: NewOrgFormProps) => {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { name } = useParams()

  const [orgName, setOrgName] = useState(name || '')
  const [newOrgLoading, setNewOrgLoading] = useState(false)

  useEffect(() => {
    const query: Record<string, string> = {}
    if (orgName) query.name = orgName
    router.push({ query })
  }, [orgName])

  const { mutateAsync: createOrganization } = useOrganizationCreateMutation({
    onSuccess: async (org: any) => {
      toast.success('Organization created successfully')
      await invalidateOrganizationsQuery(queryClient)
      router.push(`/new/${org.slug}`)
    },
    onError: (error: any) => {
      setNewOrgLoading(false)
      toast.error(error?.message ?? 'Failed to create organization')
    },
  })

  function onOrgNameChange(e: any) {
    setOrgName(e.target.value)
  }

  async function createOrg() {
    try {
      await createOrganization({
        name: orgName,
        kind: 'PERSONAL',
        tier: 'tier_enterprise',
        size: '1',
      })
    } catch (error) {
      setNewOrgLoading(false)
    }
  }

  async function onClickSubmit() {
    if (orgName.length === 0) {
      return toast.error('Organization name is empty')
    }

    setNewOrgLoading(true)
    await createOrg()
  }

  return (
    <>
      <Panel
        title={
          <div key="panel-title">
            <h4>Create a new organization</h4>
          </div>
        }
        footer={
          <div key="panel-footer" className="flex w-full items-center justify-end">
            <Button
              type="primary"
              onClick={onClickSubmit}
              loading={newOrgLoading}
              disabled={newOrgLoading}
            >
              Create organization
            </Button>
          </div>
        }
      >
        <Panel.Content className="Form section-block--body has-inputs-centered">
          <Input
            autoFocus
            id="orgName"
            label="Name"
            layout="horizontal"
            placeholder="Organization name"
            value={orgName}
            onChange={onOrgNameChange}
          />
        </Panel.Content>
      </Panel>
    </>
  )
}

export default NewOrgForm
