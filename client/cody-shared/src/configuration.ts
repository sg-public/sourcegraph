export type ConfigurationUseContext = 'embeddings' | 'keyword' | 'none' | 'blended'

export interface Configuration {
    enabled: boolean
    serverEndpoint: string
    appEndpoint: string | null
    codebase?: string
    debug: boolean
    useContext: ConfigurationUseContext
    experimentalSuggest: boolean
    experimentalChatPredictions: boolean
    anthropicKey: string | null
    customHeaders: Record<string, string>
}

export interface ConfigurationWithAccessToken extends Configuration {
    /** The access token, which is stored in the secret storage (not configuration). */
    accessToken: string | null
}
