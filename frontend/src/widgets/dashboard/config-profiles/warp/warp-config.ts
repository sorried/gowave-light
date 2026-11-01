import { x25519 } from '@noble/curves/ed25519.js'
import { decode, encode } from '@shared/lib/base64'
import { isRecord } from '@shared/lib/guards'
import { z } from 'zod'

export interface WarpSettings {
    address: string
    endpoint: string
    privateKey: string
    publicKey: string
    reserved: string
    tag: string
}

export type WarpRoute = 'domains' | 'none' | 'remaining'

export function generateWarpKeys() {
    const privateKey = x25519.utils.randomSecretKey().map((byte, index) => {
        if (index === 0) return byte & 248
        if (index === 31) return (byte & 127) | 64
        return byte
    })
    return { privateKey: encode(privateKey), publicKey: encode(x25519.getPublicKey(privateKey)) }
}

function validKey(value: string) {
    try {
        const key = decode(value)
        return key.length === 32 && key.some((byte) => byte !== 0)
    } catch {
        return false
    }
}

function list(value: string) {
    return [
        ...new Set(
            value
                .split(/[,\n]/)
                .map((item) => item.trim())
                .filter(Boolean)
        )
    ]
}

export function addWarpOutbound(
    value: unknown,
    settings: WarpSettings,
    route: WarpRoute,
    domains: string
): Record<string, unknown> {
    if (!isRecord(value)) throw new Error('Config must be a JSON object.')
    const tag = settings.tag.trim()
    if (!/^[\w.-]{1,64}$/.test(tag))
        throw new Error('Use 1-64 letters, digits, dots or hyphens for the tag.')
    if (!validKey(settings.privateKey.trim()) || !validKey(settings.publicKey.trim())) {
        throw new Error('WireGuard keys must contain 32 bytes in standard base64.')
    }
    const endpoint = settings.endpoint.trim()
    if (!/^(?:\[[\da-f:]+\]|[\w.-]+):\d{1,5}$/i.test(endpoint))
        throw new Error('Endpoint must be a hostname or IP with a port.')
    if (
        endpoint.startsWith('[') &&
        !z.ipv6().safeParse(endpoint.slice(1, endpoint.indexOf(']'))).success
    ) {
        throw new Error('Endpoint contains an invalid IPv6 address.')
    }
    const port = Number(endpoint.slice(endpoint.lastIndexOf(':') + 1))
    if (port < 1 || port > 65535) throw new Error('Endpoint port must be between 1 and 65535.')
    const address = list(settings.address).map((entry) => {
        const [ip = '', mask] = entry.split('/')
        const bits = z.ipv4().safeParse(ip).success ? 32 : z.ipv6().safeParse(ip).success ? 128 : 0
        if (
            !bits ||
            (mask !== undefined && (!/^\d+$/.test(mask) || Number(mask) > bits)) ||
            entry.split('/').length > 2
        ) {
            throw new Error(
                'Enter valid IPv4 or IPv6 tunnel addresses, with optional CIDR prefixes.'
            )
        }
        return `${ip}/${mask ?? bits}`
    })
    if (!address.length) throw new Error('Enter at least one tunnel address.')
    const reserved = settings.reserved.split(',').map((part) => part.trim())
    if (
        reserved.length !== 3 ||
        reserved.some((part) => !/^\d+$/.test(part) || Number(part) > 255)
    ) {
        throw new Error('Reserved must contain three comma-separated bytes from 0 to 255.')
    }
    if (
        value['outbounds'] !== undefined &&
        (!Array.isArray(value['outbounds']) || !value['outbounds'].every(isRecord))
    ) {
        throw new Error('Outbounds must be an array of objects.')
    }
    const outbounds: Record<string, unknown>[] = [...(value['outbounds'] ?? [])]
    if (outbounds.some((outbound) => outbound['tag'] === tag))
        throw new Error('This outbound tag already exists. Choose another tag.')
    if (!outbounds.length) {
        if (Array.isArray(value['snippets']) && value['snippets'].length) {
            throw new Error(
                'Define outbounds explicitly before adding WARP to a root snippet config.'
            )
        }
        if (tag === 'direct')
            throw new Error('The direct tag is reserved for the default connection.')
        outbounds.push({ tag: 'direct', protocol: 'freedom' })
    }
    outbounds.push({
        tag,
        protocol: 'wireguard',
        streamSettings: { sockopt: { domainStrategy: 'ForceIPv4v6' } },
        targetStrategy: 'ForceIPv4v6',
        settings: {
            secretKey: settings.privateKey.trim(),
            address,
            peers: [
                {
                    publicKey: settings.publicKey.trim(),
                    endpoint,
                    allowedIPs: ['0.0.0.0/0', '::/0']
                }
            ],
            reserved: reserved.map(Number),
            mtu: 1420,
            noKernelTun: true
        }
    })
    if (route === 'none') return { ...value, outbounds }
    const routing = value['routing'] ?? {}
    if (!isRecord(routing)) throw new Error('Routing must be a JSON object.')
    if (
        routing['rules'] !== undefined &&
        (!Array.isArray(routing['rules']) || !routing['rules'].every(isRecord))
    ) {
        throw new Error('Routing rules must be an array of objects.')
    }
    const rules: Record<string, unknown>[] = [...(routing['rules'] ?? [])]
    const rule: Record<string, unknown> = { type: 'field', outboundTag: tag }
    if (route === 'remaining') {
        rule['network'] = 'tcp,udp'
        rules.push(rule)
    } else {
        const names = list(domains)
        if (!names.length) throw new Error('Enter at least one domain or geosite selector.')
        rule['domain'] = names
        const systemTags = new Set(['api', 'metrics'])
        for (const key of ['api', 'metrics']) {
            const part = value[key]
            if (isRecord(part) && typeof part['tag'] === 'string') systemTags.add(part['tag'])
        }
        let position = 0
        rules.forEach((existing, index) => {
            if (
                (typeof existing['outboundTag'] === 'string' &&
                    systemTags.has(existing['outboundTag'])) ||
                (Array.isArray(existing['inboundTag']) &&
                    existing['inboundTag'].some(
                        (inbound) => typeof inbound === 'string' && systemTags.has(inbound)
                    ))
            )
                position = index + 1
        })
        rules.splice(position, 0, rule)
    }
    return { ...value, outbounds, routing: { ...routing, rules } }
}
