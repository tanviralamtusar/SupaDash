import { NextPageWithLayout } from 'types'
import { OrganizationLayout } from 'components/layouts'
import { ServerOverview } from 'components/interfaces/Resources/ServerOverview'
import { ScaffoldContainer, ScaffoldDivider, ScaffoldHeader, ScaffoldTitle } from 'components/layouts/Scaffold'

const ServerResourcesPage: NextPageWithLayout = () => {
  return (
    <ScaffoldContainer>
      <ScaffoldHeader>
        <ScaffoldTitle>Server Overview</ScaffoldTitle>
      </ScaffoldHeader>
      <ScaffoldDivider />
      <div className="py-6">
        <ServerOverview />
      </div>
    </ScaffoldContainer>
  )
}

ServerResourcesPage.getLayout = (page) => (
  <OrganizationLayout title="Server Resources">{page}</OrganizationLayout>
)

export default ServerResourcesPage
