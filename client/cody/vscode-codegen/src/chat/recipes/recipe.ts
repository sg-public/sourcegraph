import { Editor } from '../../editor'
import { Message } from '../../sourcegraph-api'
import { ContextSearchOptions } from '../context-search-options'

export interface RecipePrompt {
    displayText: string
    contextMessages: Message[]
    promptMessage: Message
    botResponsePrefix: string
}

export interface Recipe {
    getID(): string
    getPrompt(
        maxTokens: number,
        editor: Editor,
        getEmbeddingsContextMessages: (query: string, options: ContextSearchOptions) => Promise<Message[]>
    ): Promise<RecipePrompt | null>
}
