import { z } from 'zod'

import { createMutationHook } from '../../tsq-helpers'

export const useRegisterWarp = createMutationHook({
    endpoint: '/api/config-profiles/warp/register',
    requestMethod: 'post',
    bodySchema: z.object({ publicKey: z.string(), acceptTerms: z.literal(true) }),
    responseSchema: z.object({
        response: z.object({
            publicKey: z.string(),
            endpoint: z.string(),
            address: z.array(z.string()).min(1),
            reserved: z.tuple([
                z.number().int().min(0).max(255),
                z.number().int().min(0).max(255),
                z.number().int().min(0).max(255)
            ])
        })
    })
})
