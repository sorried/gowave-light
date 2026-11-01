import { panelUrl } from '@shared/lib/panel-path'
export const app = {
    name: 'GoWave',
    githubRepo: 'https://github.com/sorried/gowave-light',
    githubStars: 'https://github.com/sorried/gowave-light/stargazers',
    githubIssues: 'https://github.com/sorried/gowave-light/issues',
    githubOrg: 'https://github.com/sorried',
    configEditor: {
        wasmUrl: panelUrl('/assets/main.wasm'),
        wasmJsUrl: panelUrl('/assets/wasm_exec.js'),
        jsonSchemaUrl: panelUrl('/assets/xray.schema.json'),
        jsonSchemaCnUrl: panelUrl('/assets/xray.schema.cn.json')
    },
    templateEditor: {
        singboxJsonSchemaUrl: panelUrl('/assets/singbox.schema.json'),
        mihomoYamlSchemaUrl: panelUrl('/assets/mihomo.schema.json')
    }
}
