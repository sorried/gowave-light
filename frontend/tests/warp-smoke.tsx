import { MantineProvider } from '@mantine/core'
import { instance } from '@shared/api'
import { isRecord } from '@shared/lib/guards'
import { requireValue } from '@shared/lib/invariant'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { WarpWidget } from '@widgets/dashboard/config-profiles/warp/warp.widget'
import { flushSync } from 'react-dom'
import type { Root } from 'react-dom/client'

export async function runWarpSmokeTests(root: Root, container: HTMLElement) {
    const adapter = instance.defaults.adapter
    const client = new QueryClient()
    const applied: Record<string, unknown>[] = []
    let registrations = 0
    instance.defaults.adapter = (config) => {
        const payload = JSON.parse(String(config.data)) as Record<string, unknown>
        if (
            Object.keys(payload).sort().join(',') !== 'acceptTerms,publicKey' ||
            payload['acceptTerms'] !== true ||
            typeof payload['publicKey'] !== 'string' ||
            atob(payload['publicKey']).length !== 32
        ) {
            throw new Error('WARP registration sent more than the public key and terms acceptance')
        }
        registrations++
        return Promise.resolve({
            config,
            status: 200,
            statusText: 'OK',
            headers: {},
            data: {
                response: {
                    publicKey: btoa(String.fromCharCode(...new Uint8Array(32).fill(2))),
                    endpoint: 'engage.cloudflareclient.com:2408',
                    address: ['172.16.0.2/32'],
                    reserved: [1, 2, 3]
                }
            }
        })
    }
    const button = (text: string) =>
        requireValue(
            [...container.querySelectorAll('button')].find((entry) =>
                entry.textContent?.includes(text)
            ),
            `Missing WARP button: ${text}`
        )
    try {
        flushSync(() =>
            root.render(
                <QueryClientProvider client={client}>
                    <MantineProvider>
                        <WarpWidget
                            getConfig={() => ({
                                outbounds: [{ tag: 'direct', protocol: 'freedom' }]
                            })}
                            onApply={(config) => applied.push(config)}
                        />
                    </MantineProvider>
                </QueryClientProvider>
            )
        )
        if (!button('warp.register').disabled || registrations > 0)
            throw new Error('WARP registration did not wait for terms acceptance')
        const terms = requireValue(
            container.querySelector<HTMLInputElement>('input[type=checkbox]'),
            'Missing WARP terms checkbox'
        )
        flushSync(() => terms.click())
        flushSync(() => button('warp.register').click())
        for (let attempt = 0; attempt < 100; attempt++) {
            if (container.textContent?.includes('warp.registered')) break
            await new Promise<void>((resolve) => setTimeout(resolve, 25))
        }
        if (registrations !== 1 || !container.textContent?.includes('warp.registered'))
            throw new Error('WARP registration did not populate the form')
        flushSync(() => button('warp.add').click())
        const result = requireValue(applied[0], 'WARP was not added to the editor')
        const outbounds = result['outbounds']
        if (
            !Array.isArray(outbounds) ||
            !isRecord(outbounds[0]) ||
            !isRecord(outbounds[1]) ||
            outbounds[0]['tag'] !== 'direct' ||
            outbounds[1]['protocol'] !== 'wireguard' ||
            result['routing'] !== undefined
        ) {
            throw new Error('Adding WARP changed default routing')
        }
    } finally {
        if (adapter === undefined) delete instance.defaults.adapter
        else instance.defaults.adapter = adapter
        client.clear()
    }
}
