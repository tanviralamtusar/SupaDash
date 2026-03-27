import { useParams } from 'next/navigation'
import ProjectLayout from 'components/layouts/ProjectLayout'
import ResourceManager from 'components/interfaces/Resources/ResourceManager'

const ProjectResourcesPage = () => {
  const params = useParams()
  const ref = params?.ref as string

  return (
    <ProjectLayout
      title="Resources"
      product="Resources"
    >
      <div className="max-w-7xl px-5 py-8 mx-auto">
        <ResourceManager projectRef={ref} />
      </div>
    </ProjectLayout>
  )
}

export default ProjectResourcesPage
