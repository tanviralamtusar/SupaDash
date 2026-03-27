import { ProjectLayout } from 'components/layouts'
import { ResourceManager } from 'components/interfaces/Resources/ResourceManager'
import { useRouter } from 'next/router'

const ProjectResourcesPage = () => {
  const router = useRouter()
  const { ref } = router.query

  return (
    <ProjectLayout
      title="Resources"
      product="Resources"
    >
      <div className="p-4">
        <ResourceManager projectRef={ref as string} />
      </div>
    </ProjectLayout>
  )
}

export default ProjectResourcesPage
