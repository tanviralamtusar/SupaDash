import { zodResolver } from '@hookform/resolvers/zod'
import { buildDefaultPrivilegesSql } from '@supabase/pg-meta'
import { PermissionAction } from '@supabase/shared-types/out/constants'
import { LOCAL_STORAGE_KEYS, useFlag, useParams } from 'common'
import { AUTO_ENABLE_RLS_EVENT_TRIGGER_SQL } from 'components/interfaces/Database/Triggers/EventTriggersList/EventTriggers.constants'
import { AdvancedConfiguration } from 'components/interfaces/ProjectCreation/AdvancedConfiguration'
import { CloudProviderSelector } from 'components/interfaces/ProjectCreation/CloudProviderSelector'
import { CustomPostgresVersionInput } from 'components/interfaces/ProjectCreation/CustomPostgresVersionInput'
import { DatabasePasswordInput } from 'components/interfaces/ProjectCreation/DatabasePasswordInput'
import { DisabledWarningDueToIncident } from 'components/interfaces/ProjectCreation/DisabledWarningDueToIncident'
import { HighAvailabilityInput } from 'components/interfaces/ProjectCreation/HighAvailabilityInput'
import { OrganizationSelector } from 'components/interfaces/ProjectCreation/OrganizationSelector'
import {
  extractPostgresVersionDetails,
  PostgresVersionSelector,
} from 'components/interfaces/ProjectCreation/PostgresVersionSelector'

import { FormSchema } from 'components/interfaces/ProjectCreation/ProjectCreation.schema'
import {
  smartRegionToExactRegion,
} from 'components/interfaces/ProjectCreation/ProjectCreation.utils'
import { ProjectCreationFooter } from 'components/interfaces/ProjectCreation/ProjectCreationFooter'
import { ProjectNameInput } from 'components/interfaces/ProjectCreation/ProjectNameInput'
import { RegionSelector } from 'components/interfaces/ProjectCreation/RegionSelector'
import { SecurityOptions } from 'components/interfaces/ProjectCreation/SecurityOptions'
import DefaultLayout from 'components/layouts/DefaultLayout'
import { WizardLayoutWithoutAuth } from 'components/layouts/WizardLayout'
import Panel from 'components/ui/Panel'
import { useAvailableOrioleImageVersion } from 'data/config/project-creation-postgres-versions-query'
import { useDefaultRegionQuery } from 'data/misc/get-default-region-query'

import { useOrganizationAvailableRegionsQuery } from 'data/organizations/organization-available-regions-query'
import { useOrganizationsQuery } from 'data/organizations/organizations-query'
import { DesiredInstanceSize } from 'data/projects/new-project.constants'
import {
  ProjectCreateVariables,
  useProjectCreateMutation,
} from 'data/projects/project-create-mutation'
import { useCustomContent } from 'hooks/custom-content/useCustomContent'
import { useAsyncCheckPermissions } from 'hooks/misc/useCheckPermissions'
import { useDataApiGrantTogglesEnabled } from 'hooks/misc/useDataApiGrantTogglesEnabled'
import { useIsFeatureEnabled } from 'hooks/misc/useIsFeatureEnabled'
import { useLocalStorageQuery } from 'hooks/misc/useLocalStorage'
import { useSelectedOrganizationQuery } from 'hooks/misc/useSelectedOrganization'
import { withAuth } from 'hooks/misc/withAuth'
import { usePHFlag } from 'hooks/ui/useFlag'
import { DOCS_URL, PROJECT_STATUS, PROVIDERS, useDefaultProvider } from 'lib/constants'
import { buildStudioPageTitle } from 'lib/page-title'
import { useProfile } from 'lib/profile'
import { useTrack } from 'lib/telemetry/track'
import Head from 'next/head'
import Link from 'next/link'
import { useRouter } from 'next/router'
import { PropsWithChildren, useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { AWS_REGIONS, type CloudProvider } from 'shared-data'
import { toast } from 'sonner'
import type { NextPageWithLayout } from 'types'
import { Button, Form_Shadcn_, FormField_Shadcn_, useWatch_Shadcn_ } from 'ui'

import { z } from 'zod'



const Wizard: NextPageWithLayout = () => {
  const track = useTrack()
  const router = useRouter()
  const { slug, projectName } = useParams()
  const { appTitle } = useCustomContent(['app:title'])
  const defaultProvider = useDefaultProvider()
  const { profile } = useProfile()
  const pageTitle = buildStudioPageTitle({
    section: 'New Project',
    brand: appTitle || 'SupaDash',
  })

  const { data: currentOrg } = useSelectedOrganizationQuery()
  const isFreePlan = false // SupaDash self-hosted: no billing tiers
  const [lastVisitedOrganization] = useLocalStorageQuery(
    LOCAL_STORAGE_KEYS.LAST_VISITED_ORGANIZATION,
    ''
  )
  const { can: isAdmin } = useAsyncCheckPermissions(PermissionAction.CREATE, 'projects')
  const showAdvancedConfig = useIsFeatureEnabled('project_creation:show_advanced_config')

  const smartRegionEnabled = useFlag('enableSmartRegion')
  const projectCreationDisabled = useFlag('disableProjectCreationAndUpdate')
  const showPostgresVersionSelector = useFlag('showPostgresVersionSelector')
  const cloudProviderEnabled = useFlag('enableFlyCloudProvider')
  const isDataApiGrantTogglesEnabled = useDataApiGrantTogglesEnabled()
  // Read the raw flag for telemetry — useDataApiGrantTogglesEnabled coerces undefined→false,
  // which would record false for users whose flags haven't loaded yet. The raw value preserves
  // undefined (omitted from PostHog) so we only record true/false when the flag is resolved.
  const tableEditorApiAccessToggleFlag = usePHFlag<boolean>('tableEditorApiAccessToggle')

  const showNonProdFields = true // SupaDash: always show postgres version etc.

  // This is to make the database.new redirect work correctly. The database.new redirect should be set to supadash.io/dashboard/new/last-visited-org
  if (slug === 'last-visited-org') {
    if (lastVisitedOrganization) {
      router.replace(`/new/${lastVisitedOrganization}`, undefined, { shallow: true })
    } else {
      router.replace(`/new/_`, undefined, { shallow: true })
    }
  }


  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
    defaultValues: {
      organization: slug,
      projectName: projectName || '',
      highAvailability: false,
      postgresVersion: '',
      cloudProvider: PROVIDERS[defaultProvider].id,
      dbPass: '',
      dbPassStrength: 0,
      instanceSize: undefined,
      dbPassStrengthMessage: '',
      dbRegion: undefined,
      dataApi: true,
      enableRlsEventTrigger: false,
      postgresVersionSelection: '',
      useOrioleDb: false,
    },
  })
  const {
    instanceSize: watchedInstanceSize,
    cloudProvider,
    dbRegion,
    organization,
    highAvailability,
  } = useWatch_Shadcn_({ control: form.control })

  const instanceSize = undefined // SupaDash: no instance sizing

  const { data: organizations = [], isSuccess: isOrganizationsSuccess } = useOrganizationsQuery()
  const isEmptyOrganizations = isOrganizationsSuccess && organizations.length <= 0


  const { data: _defaultRegion, error: defaultRegionError } = useDefaultRegionQuery(
    {
      cloudProvider: PROVIDERS[defaultProvider].id,
    },
    {
      enabled: !smartRegionEnabled,
      refetchOnMount: false,
      refetchOnWindowFocus: false,
      refetchInterval: false,
      refetchOnReconnect: false,
      retry: false,
    }
  )

  const { data: availableRegionsData, error: availableRegionsError } =
    useOrganizationAvailableRegionsQuery(
      {
        slug: slug,
        cloudProvider: PROVIDERS[cloudProvider as CloudProvider].id,
        desiredInstanceSize: instanceSize as DesiredInstanceSize,
      },
      {
        enabled: smartRegionEnabled,
        refetchOnMount: false,
        refetchOnWindowFocus: false,
        refetchInterval: false,
        refetchOnReconnect: false,
      }
    )
  const recommendedSmartRegion = smartRegionEnabled
    ? availableRegionsData?.recommendations.smartGroup.name
    : undefined
  const regionError =
    smartRegionEnabled && defaultProvider !== 'AWS_NIMBUS'
      ? availableRegionsError
      : defaultRegionError
  const defaultRegion =
    defaultProvider === 'AWS_NIMBUS'
      ? AWS_REGIONS.EAST_US.displayName
      : smartRegionEnabled
        ? availableRegionsData?.recommendations.smartGroup.name
        : _defaultRegion

  const canCreateProject = isAdmin // SupaDash: no billing checks

  const dbRegionExact = smartRegionToExactRegion(dbRegion ?? '')

  const availableOrioleVersion = useAvailableOrioleImageVersion(
    {
      cloudProvider: cloudProvider as CloudProvider,
      dbRegion: smartRegionEnabled ? dbRegionExact : (dbRegion ?? ''),
      organizationSlug: organization,
    },
    { enabled: currentOrg !== null }
  )



  const {
    mutate: createProject,
    isPending: isCreatingNewProject,
    isSuccess: isSuccessNewProject,
  } = useProjectCreateMutation({
    onSuccess: (res) => {
      track(
        'project_creation_simple_version_submitted',
        {
          instanceSize: form.getValues('instanceSize'),
          enableRlsEventTrigger: form.getValues('enableRlsEventTrigger'),
          dataApiEnabled: form.getValues('dataApi'),
          useOrioleDb: form.getValues('useOrioleDb'),
          ...(tableEditorApiAccessToggleFlag !== undefined && {
            tableEditorApiAccessToggleEnabled: tableEditorApiAccessToggleFlag,
          }),
        },
        {
          project: res.ref,
          organization: res.organization_slug,
        }
      )
      router.push(`/project/${res.ref}`)
    },
  })

  // SupaDash: no compute cost confirmation needed
  const onSubmitWithComputeCostsConfirmation = async (values: z.infer<typeof FormSchema>) => {
    await onSubmit(values)
  }

  const onSubmit = async (values: z.infer<typeof FormSchema>) => {
    if (!currentOrg) return console.error('Unable to retrieve current organization')

    const {
      cloudProvider,
      projectName,
      highAvailability,
      dbPass,
      dbRegion,
      postgresVersion,
      instanceSize,
      dataApi,
      enableRlsEventTrigger,
      postgresVersionSelection,
      useOrioleDb,
    } = values

    if (useOrioleDb && !availableOrioleVersion) {
      return toast.error('No available OrioleDB image found, only Postgres is available')
    }

    const { postgresEngine, releaseChannel } =
      extractPostgresVersionDetails(postgresVersionSelection)

    const { smartGroup = [], specific = [] } = availableRegionsData?.all ?? {}
    const selectedRegion = smartRegionEnabled
      ? (smartGroup.find((x) => x.name === dbRegion) ?? specific.find((x) => x.name === dbRegion))
      : undefined

    const data: ProjectCreateVariables = {
      dbPass,
      cloudProvider,
      organizationSlug: currentOrg.slug,
      name: projectName,
      highAvailability,
      // gets ignored due to org billing subscription anyway
      dbPricingTierId: 'tier_free',
      // only set the compute size on pro+ plans. Free plans always use micro (nano in the future) size.
      dbInstanceSize: undefined, // SupaDash: no instance sizing
      dataApiExposedSchemas: !dataApi ? [] : undefined,
      dataApiUseApiSchema: false,
      postgresEngine: useOrioleDb ? availableOrioleVersion?.postgres_engine : postgresEngine,
      releaseChannel: useOrioleDb ? availableOrioleVersion?.release_channel : releaseChannel,
      ...(smartRegionEnabled ? { regionSelection: selectedRegion } : { dbRegion }),
      dbSql:
        [
          enableRlsEventTrigger && AUTO_ENABLE_RLS_EVENT_TRIGGER_SQL,
          // [Alaister]: temporarily disable the default secure sql
          // To re-enable, remove the false &&
          false && isDataApiGrantTogglesEnabled && buildDefaultPrivilegesSql('revoke'),
        ]
          .filter(Boolean)
          .join('\n') || undefined,
    }

    if (postgresVersion) {
      if (!postgresVersion.match(/1[2-9]\..*/)) {
        toast.error(
          `Invalid Postgres version, should start with a number between 12-19, a dot and additional characters, i.e. 15.2 or 15.2.0-3`
        )
      }

      data['customSupabaseRequest'] = {
        ami: { search_tags: { 'tag:postgresVersion': postgresVersion } },
      }
    }

    createProject(data)
  }



  useEffect(() => {
    // Handle no org: redirect to new org route
    if (isEmptyOrganizations) {
      router.push(`/new`)
    }
  }, [isEmptyOrganizations, router])

  useEffect(() => {
    // [Joshen] Cause slug depends on router which doesnt load immediately on render
    // While the form data does load immediately
    if (slug && slug !== '_') form.setValue('organization', slug)
    if (projectName) form.setValue('projectName', projectName || '')
  }, [slug])

  useEffect(() => {
    if (form.getValues('dbRegion') === undefined && defaultRegion) {
      form.setValue('dbRegion', defaultRegion)
    }
  }, [defaultRegion])

  useEffect(() => {
    if (regionError) {
      form.setValue('dbRegion', PROVIDERS[defaultProvider].default_region.displayName)
    }
  }, [regionError])

  useEffect(() => {
    if (recommendedSmartRegion) {
      form.setValue('dbRegion', recommendedSmartRegion)
    }
  }, [recommendedSmartRegion])



  return (
    <>
      {/* Wizard layouts set the visual header but not the browser tab title. */}
      <Head>
        <title>{pageTitle}</title>
        <meta name="description" content="SupaDash Studio" />
      </Head>
      <Form_Shadcn_ {...form}>
        <form onSubmit={form.handleSubmit(onSubmitWithComputeCostsConfirmation)}>
          <Panel
            loading={!isOrganizationsSuccess}
            title={
              <div key="panel-title">
                <h3>Create a new project</h3>
                <p className="text-sm text-foreground-lighter text-balance">
                  Your project will have its own dedicated instance and full Postgres database. An
                  API will be set up so you can easily interact with your new database.
                </p>
              </div>
            }
            footer={
              <ProjectCreationFooter
                form={form}
                canCreateProject={canCreateProject}
                instanceSize={undefined}
                organizationProjects={[]}
                isCreatingNewProject={isCreatingNewProject}
                isSuccessNewProject={isSuccessNewProject}
              />
            }
          >
            <>
              {projectCreationDisabled ? (
                <DisabledWarningDueToIncident title="Project creation is currently disabled" />
              ) : (
                <div className="divide-y divide-border-muted">
                  <OrganizationSelector form={form} />

                  {canCreateProject && (
                    <>
                      <ProjectNameInput form={form} />
                      <HighAvailabilityInput form={form} />

                      {cloudProviderEnabled && showNonProdFields && (
                        <CloudProviderSelector form={form} />
                      )}

  

                      <DatabasePasswordInput form={form} />

                      <RegionSelector
                        form={form}
                        instanceSize={instanceSize as DesiredInstanceSize}
                      />

                      {showPostgresVersionSelector && (
                        <Panel.Content>
                          <FormField_Shadcn_
                            control={form.control}
                            name="postgresVersionSelection"
                            render={({ field }) => (
                              <PostgresVersionSelector
                                field={field}
                                form={form}
                                cloudProvider={form.getValues('cloudProvider') as CloudProvider}
                                organizationSlug={slug}
                                dbRegion={form.getValues('dbRegion')}
                              />
                            )}
                          />
                        </Panel.Content>
                      )}

                      <CustomPostgresVersionInput form={form} />

                      <SecurityOptions form={form} />
                      {showAdvancedConfig && !!availableOrioleVersion && (
                        <AdvancedConfiguration form={form} />
                      )}

                    </>
                  )}


                </div>
              )}
            </>
          </Panel>


        </form>
      </Form_Shadcn_>
    </>
  )
}

const PageLayout = withAuth(({ children }: PropsWithChildren) => {
  return <WizardLayoutWithoutAuth>{children}</WizardLayoutWithoutAuth>
})

Wizard.getLayout = (page) => (
  <DefaultLayout hideMobileMenu headerTitle="New project">
    <PageLayout>{page}</PageLayout>
  </DefaultLayout>
)

export default Wizard
