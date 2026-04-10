import { NextApiRequest, NextApiResponse } from 'next'

// SupaDash Global API Proxy
// This replaces ALL official Supabase Studio API endpoints.
// Requests to /api/* are seamlessly forwarded to the SupaDash Go Backend.

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
  const { supadash } = req.query
  
  // Construct the targeted Go API URL
  const backendUrl = process.env.SUPADASH_API_URL || 'http://localhost:8080'
  const path = Array.isArray(supadash) ? supadash.join('/') : (supadash || '')
  
  // Create the full URL preserving query string
  // Remove the `supadash` query parameter Next.js added
  const queryParams = new URLSearchParams(req.query as any)
  queryParams.delete('supadash')
  
  const queryString = queryParams.toString()
  const targetUrl = `${backendUrl}/${path}${queryString ? `?${queryString}` : ''}`

  const options: RequestInit = {
    method: req.method,
    headers: {
      ...req.headers as any,
      host: undefined, // Let Node/fetch set the host header natively
    } as any,
  }

  // Next.js body parser already parsed req.body, so we stringify it back
  if (req.method !== 'GET' && req.method !== 'HEAD' && req.body) {
    if (typeof req.body === 'object') {
      options.body = JSON.stringify(req.body)
      const headers = options.headers as any
      // Ensure content-type is application/json if we stringified it
      if (!headers['content-type']) {
        headers['content-type'] = 'application/json'
      }
    } else {
      options.body = req.body
    }
  }

  try {
    const backendRes = await fetch(targetUrl, options)
    
    // Copy the status
    res.status(backendRes.status)
    
    // Copy the headers
    backendRes.headers.forEach((value, key) => {
      // Don't copy chunked encoding
      if (key.toLowerCase() !== 'transfer-encoding') {
        res.setHeader(key, value)
      }
    })
    
    // Read and pipe the response
    const data = await backendRes.text()
    
    // If it's JSON, send as JSON. Otherwise send text.
    try {
      res.send(JSON.parse(data))
    } catch {
      res.send(data)
    }
  } catch (err: any) {
    console.error('[SupaDash Proxy Error] ', err.message)
    res.status(500).json({ error: 'SupaDash Proxy Failed: ' + err.message })
  }
}
