import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

import { KodaService } from '@/gen/koda/v1/service_pb'

const defaultBaseUrl = window.location.origin

const transport = createConnectTransport({
  baseUrl: import.meta.env.VITE_KODA_API_URL ?? defaultBaseUrl,
})

export const kodaClient = createClient(KodaService, transport)
