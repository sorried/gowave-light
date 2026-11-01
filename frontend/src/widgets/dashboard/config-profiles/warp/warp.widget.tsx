import {
    Alert,
    Anchor,
    Button,
    Checkbox,
    Divider,
    Group,
    PasswordInput,
    Select,
    Stack,
    Text,
    Textarea,
    TextInput
} from '@mantine/core'
import { useRegisterWarp } from '@shared/api/hooks/config-profiles/warp.mutation.hooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { addWarpOutbound, generateWarpKeys, type WarpRoute, type WarpSettings } from './warp-config'

interface Props {
    getConfig: () => unknown
    onApply: (config: Record<string, unknown>) => void
}

export function WarpWidget({ getConfig, onApply }: Props) {
    const { t } = useTranslation()
    const [settings, setSettings] = useState<WarpSettings>({
        tag: 'warp',
        privateKey: '',
        publicKey: '',
        endpoint: '',
        address: '',
        reserved: '0,0,0'
    })
    const [acceptTerms, setAcceptTerms] = useState(false)
    const [route, setRoute] = useState<WarpRoute>('none')
    const [domains, setDomains] = useState('')
    const [error, setError] = useState('')
    const [registered, setRegistered] = useState(false)
    const { mutate: register, isPending } = useRegisterWarp({
        mutationFns: { onError: (failure) => setError(failure.message) }
    })

    const create = () => {
        setError('')
        const keys = generateWarpKeys()
        register({
            variables: { publicKey: keys.publicKey, acceptTerms: true },
            mutationFns: {
                onSuccess: (result) => {
                    setSettings((current) => ({
                        ...current,
                        privateKey: keys.privateKey,
                        publicKey: result.publicKey,
                        endpoint: result.endpoint,
                        address: result.address.join(', '),
                        reserved: result.reserved.join(',')
                    }))
                    setRegistered(true)
                }
            }
        })
    }

    const apply = () => {
        setError('')
        try {
            onApply(addWarpOutbound(getConfig(), settings, route, domains))
        } catch (failure) {
            setError(failure instanceof Error ? failure.message : t('warp.invalid-config'))
        }
    }

    return (
        <Stack gap="sm">
            <Text c="dimmed" size="sm">
                {t('warp.description')}
            </Text>
            {error && (
                <Alert color="red" role="alert" title={t('common.message.error')}>
                    {error}
                </Alert>
            )}
            {registered && <Alert color="teal">{t('warp.registered')}</Alert>}
            <Checkbox
                checked={acceptTerms}
                disabled={isPending || registered}
                label={
                    <>
                        {t('warp.accept-terms')}{' '}
                        <Anchor
                            href="https://www.cloudflare.com/application/terms/"
                            rel="noopener noreferrer"
                            target="_blank"
                        >
                            Cloudflare
                        </Anchor>
                    </>
                }
                onChange={(event) => setAcceptTerms(event.currentTarget.checked)}
            />
            <Button
                disabled={!acceptTerms || registered}
                loading={isPending}
                onClick={create}
                variant="light"
            >
                {t('warp.register')}
            </Button>
            <Divider label={t('warp.settings')} />
            <Text c="dimmed" size="xs">
                {t('warp.manual')}
            </Text>
            <TextInput
                disabled={isPending}
                label={t('warp.tag')}
                onChange={(event) => setSettings({ ...settings, tag: event.currentTarget.value })}
                value={settings.tag}
            />
            <PasswordInput
                autoComplete="off"
                disabled={isPending}
                label={t('warp.private-key')}
                onChange={(event) =>
                    setSettings({ ...settings, privateKey: event.currentTarget.value })
                }
                value={settings.privateKey}
            />
            <TextInput
                disabled={isPending}
                label={t('warp.public-key')}
                onChange={(event) =>
                    setSettings({ ...settings, publicKey: event.currentTarget.value })
                }
                value={settings.publicKey}
            />
            <TextInput
                disabled={isPending}
                label={t('warp.address')}
                onChange={(event) =>
                    setSettings({ ...settings, address: event.currentTarget.value })
                }
                placeholder="IPv4/32, IPv6/128"
                value={settings.address}
            />
            <TextInput
                disabled={isPending}
                label={t('warp.endpoint')}
                onChange={(event) =>
                    setSettings({ ...settings, endpoint: event.currentTarget.value })
                }
                placeholder="engage.cloudflareclient.com:2408"
                value={settings.endpoint}
            />
            <TextInput
                disabled={isPending}
                label={t('warp.reserved')}
                onChange={(event) =>
                    setSettings({ ...settings, reserved: event.currentTarget.value })
                }
                value={settings.reserved}
            />
            <Select
                allowDeselect={false}
                data={[
                    { value: 'none', label: t('warp.route-none') },
                    { value: 'domains', label: t('warp.route-domains') },
                    { value: 'remaining', label: t('warp.route-remaining') }
                ]}
                label={t('warp.routing')}
                onChange={(value) => {
                    if (value === 'none' || value === 'domains' || value === 'remaining')
                        setRoute(value)
                }}
                value={route}
            />
            {route === 'domains' && (
                <Textarea
                    autosize
                    label={t('warp.domains')}
                    minRows={3}
                    onChange={(event) => setDomains(event.currentTarget.value)}
                    placeholder={'domain:example.com\ngeosite:google'}
                    value={domains}
                />
            )}
            <Text c="dimmed" size="xs">
                {t(route === 'remaining' ? 'warp.remaining-hint' : 'warp.save-hint')}
            </Text>
            <Group justify="flex-end">
                <Button disabled={isPending} onClick={apply}>
                    {t('warp.add')}
                </Button>
            </Group>
        </Stack>
    )
}
