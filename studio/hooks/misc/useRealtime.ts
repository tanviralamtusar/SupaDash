import { useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useParams } from 'common'
import { API_URL } from 'lib/constants'
import { useSetProjectStatus } from 'data/projects/project-detail-query'

export function useRealtime() {
  const { ref: projectRef } = useParams()
  const queryClient = useQueryClient()
  const socketRef = useRef<WebSocket | null>(null)
  const { setProjectStatus } = useSetProjectStatus()

  useEffect(() => {
    if (!projectRef) return

    // Derive WS URL from API_URL
    // API_URL might be '/api' (relative) or 'http://...'
    let wsUrl = ''
    if (API_URL.startsWith('http')) {
      wsUrl = API_URL.replace('http', 'ws') + '/ws'
    } else {
      // Relative path, use window.location
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const host = window.location.host
      wsUrl = `${protocol}//${host}${API_URL}/ws`
    }
    
    console.log('Connecting to real-time updates at:', wsUrl)
    const socket = new WebSocket(wsUrl)
    socketRef.current = socket

    socket.onopen = () => {
      console.log('Real-time connection opened')
      // Subscribe to project stats and status
      socket.send(JSON.stringify({ type: 'subscribe', project_ref: projectRef }))
    }

    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data)
        
        if (message.type === 'status_update') {
          console.log('Received status update:', message.data.status)
          setProjectStatus({
            ref: projectRef,
            status: message.data.status,
          })
        } else if (message.type === 'stats') {
          // Update stats in cache if there's a specific query for it
          // For now, we mainly want to trigger a refresh of the resource usage charts if they are visible
          queryClient.invalidateQueries(['projects', projectRef, 'resource-usage'])
          // Also set data directly if we want instant feedback
          queryClient.setQueryData(['projects', projectRef, 'realtime-stats', message.data.service_name], message.data)
        }
      } catch (err) {
        console.error('Failed to parse realtime message', err)
      }
    }

    socket.onerror = (error) => {
      console.error('WebSocket error:', error)
    }

    socket.onclose = () => {
      console.log('Real-time connection closed')
    }

    return () => {
      if (socketRef.current) {
        socketRef.current.close()
      }
    }
  }, [projectRef, queryClient, setProjectStatus])
}
