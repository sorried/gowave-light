import assert from 'node:assert/strict'
import { test } from 'node:test'
import { x25519 } from '@noble/curves/ed25519.js'
import { decode } from '@shared/lib/base64'

import { addWarpOutbound, generateWarpKeys, type WarpSettings } from './warp-config'

const keys = generateWarpKeys()
const settings: WarpSettings = {
    tag: 'warp',
    privateKey: keys.privateKey,
    publicKey: keys.publicKey,
    address: '172.16.0.2, 2606:4700:110:1234::2/128',
    endpoint: 'engage.cloudflareclient.com:2408',
    reserved: '1,2,3'
}

void test('WARP keys use standard WireGuard base64 and form a valid X25519 pair', () => {
    assert.equal(decode(keys.privateKey).length, 32)
    assert.ok(
        decode(keys.privateKey)
            .slice(0, 1)
            .every((byte) => (byte & 7) === 0)
    )
    assert.ok(
        decode(keys.privateKey)
            .slice(31)
            .every((byte) => (byte & 128) === 0 && (byte & 64) === 64)
    )
    assert.deepEqual(decode(keys.publicKey), x25519.getPublicKey(decode(keys.privateKey)))
})

void test('adding WARP preserves the default outbound, existing config and input object', () => {
    const config = {
        outbounds: [{ tag: 'direct', protocol: 'freedom' }],
        routing: { rules: [] },
        dns: { servers: ['1.1.1.1'] }
    }
    const before = JSON.stringify(config)
    const next = addWarpOutbound(config, settings, 'none', '')
    assert.equal(JSON.stringify(config), before)
    assert.deepEqual(next, {
        ...config,
        outbounds: [
            config.outbounds[0],
            {
                tag: 'warp',
                protocol: 'wireguard',
                streamSettings: { sockopt: { domainStrategy: 'ForceIPv4v6' } },
                targetStrategy: 'ForceIPv4v6',
                settings: {
                    secretKey: keys.privateKey,
                    address: ['172.16.0.2/32', '2606:4700:110:1234::2/128'],
                    peers: [
                        {
                            publicKey: keys.publicKey,
                            endpoint: settings.endpoint,
                            allowedIPs: ['0.0.0.0/0', '::/0']
                        }
                    ],
                    reserved: [1, 2, 3],
                    mtu: 1420,
                    noKernelTun: true
                }
            }
        ]
    })
    const empty = addWarpOutbound({}, settings, 'none', '')
    assert.equal((empty['outbounds'] as { tag: string }[])[0]?.tag, 'direct')
})

void test('domain routing preserves API and metrics priority and all existing rules', () => {
    const api = { type: 'field', inboundTag: ['admin-api'], outboundTag: 'control' }
    const metrics = { type: 'field', inboundTag: ['metrics'], outboundTag: 'metrics' }
    const block = { type: 'field', protocol: ['bittorrent'], outboundTag: 'block' }
    const config = {
        api: { tag: 'control' },
        outbounds: [{ tag: 'direct', protocol: 'freedom' }],
        routing: { domainStrategy: 'AsIs', rules: [api, metrics, block] }
    }
    const next = addWarpOutbound(
        config,
        settings,
        'domains',
        'domain:example.com\ngeosite:google,domain:example.com'
    )
    assert.deepEqual(next['routing'], {
        domainStrategy: 'AsIs',
        rules: [
            api,
            metrics,
            {
                type: 'field',
                outboundTag: 'warp',
                domain: ['domain:example.com', 'geosite:google']
            },
            block
        ]
    })
    assert.deepEqual(config.routing.rules, [api, metrics, block])
})

void test('remaining traffic rule is appended after existing routing decisions', () => {
    const existing = { type: 'field', ip: ['geoip:private'], outboundTag: 'block' }
    const next = addWarpOutbound({ routing: { rules: [existing] } }, settings, 'remaining', '')
    assert.deepEqual(next['routing'], {
        rules: [existing, { type: 'field', outboundTag: 'warp', network: 'tcp,udp' }]
    })
})

void test('WARP rejects incomplete settings, duplicate tags and unsafe config rewrites', () => {
    for (const invalid of [
        { privateKey: 'invalid' },
        { publicKey: 'invalid' },
        { address: '' },
        { address: '172.16.0.2/33' },
        { address: '172.16.0.2/32/1' },
        { endpoint: 'https://example.com:2408' },
        { endpoint: '[:::]:2408' },
        { endpoint: 'example.com:0' },
        { endpoint: 'example.com:65536' },
        { reserved: '1,2,256' },
        { reserved: '1,,3' },
        { tag: '' }
    ])
        assert.throws(() => addWarpOutbound({}, { ...settings, ...invalid }, 'none', ''))
    for (const config of [
        null,
        [],
        { outbounds: {} },
        { outbounds: [null] },
        { snippets: ['root'] },
        { outbounds: [{ tag: 'warp' }] }
    ]) {
        assert.throws(() => addWarpOutbound(config, settings, 'none', ''))
    }
    assert.throws(() => addWarpOutbound({}, settings, 'domains', ''))
    assert.throws(() => addWarpOutbound({ routing: [] }, settings, 'remaining', ''))
    assert.throws(() => addWarpOutbound({ routing: { rules: {} } }, settings, 'remaining', ''))
    assert.doesNotThrow(() =>
        addWarpOutbound({}, { ...settings, endpoint: '[2606:4700:d0::a29f:c001]:2408' }, 'none', '')
    )
})
