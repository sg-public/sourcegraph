import { removeStopwords } from 'stopword'
import * as vscode from 'vscode'

import { getContextMessageWithResponse, populateCodeContextTemplate } from '../chat/prompt'
import { Message } from '../sourcegraph-api'

import { getTermScore, KeywordContextFetcher } from '.'

export class LocalKeywordContextFetcher implements KeywordContextFetcher {
    constructor() {}

    public async getContextMessages(query: string): Promise<Message[]> {
        console.log('fetching keyword matches')
        const rootPath = this.getRootPath()
        if (!rootPath) {
            return []
        }
        const filesnamesWithScores = await this.fetchKeywordFiles(rootPath, query)
        const top10 = filesnamesWithScores.slice(0, 10).reverse()
        const messagePairs = await Promise.all(
            top10.map(async ({ filename }) => {
                const uri = vscode.Uri.file(filename)
                const text = (await vscode.workspace.openTextDocument(uri)).getText()
                // Remove root path from file path
                const file = filename.replace(`${rootPath}/`, '')
                const messageText = populateCodeContextTemplate(text, file)
                return getContextMessageWithResponse(messageText, file)
            })
        )
        return messagePairs.reverse().flat()
    }

    // Get file matches from symbol search through vs code API
    private async fetchFileMatches(
        keywords: string[],
        rootPath: string
    ): Promise<{
        totalFiles: number
        fileTermCounts: { [filename: string]: { [term: string]: number } }
        termTotalFiles: { [term: string]: number }
    }> {
        // The total number of files searched across all keywords
        let totalFilesSearched = 0
        const wsRootPath = rootPath.replace('file://', '')
        const termFileCountsArr: { fileCounts: { [filename: string]: number }; filesSearched: number }[] =
            await Promise.all(
                keywords.map(async term => {
                    // Get a list of files that match the term as symbol
                    const symbols = await this.nativeSymbolSearcher(term)
                    // Mapping filenames to match counts
                    const fileCounts: { [filename: string]: number } = {}
                    symbols.forEach(symbol => {
                        const symbolPath = symbol.location.uri.fsPath
                        // filter files that are not in current workspace
                        if (symbolPath.includes(wsRootPath)) {
                            fileCounts[symbolPath] = (fileCounts[symbolPath] || 0) + 1
                        }
                    })
                    // Set the length of fileCounts as files searched
                    const filesSearched = Object.entries(fileCounts).length || 0
                    totalFilesSearched += filesSearched
                    return { fileCounts, filesSearched }
                })
            )

        // Map filenames to an object mapping terms to match counts for that filename
        const fileTermCounts: { [filename: string]: { [term: string]: number } } = {}
        // Map terms to the number of files they matched
        const termTotalFiles: { [term: string]: number } = {}
        for (let i = 0; i < keywords.length; i++) {
            const term = keywords[i]
            const fileCounts = termFileCountsArr[i].fileCounts
            termTotalFiles[term] = Object.keys(fileCounts).length

            for (const [filename, count] of Object.entries(fileCounts)) {
                if (!fileTermCounts[filename]) {
                    fileTermCounts[filename] = {}
                }
                fileTermCounts[filename][term] = count
            }
        }
        return {
            totalFiles: totalFilesSearched,
            termTotalFiles,
            fileTermCounts,
        }
    }

    private async fetchKeywordFiles(rootPath: string, query: string): Promise<{ filename: string; score: number }[]> {
        const terms = query.split(/\W+/)
        // TODO: Stemming using the `natural` package was introducing failing licensing checks. Find a replacement stemming package.
        const stemmedTerms = terms.map(term => this.escapeRegex(term))
        // unique stemmed keywords, our representation of the user query
        const filteredTerms = Array.from(new Set(removeStopwords(stemmedTerms).filter(term => term.length >= 3)))
        const { fileTermCounts, termTotalFiles, totalFiles } = await this.fetchFileMatches(filteredTerms, rootPath)
        const idfDict = this.idf(termTotalFiles, totalFiles)
        const filenamesWithScores = Object.entries(fileTermCounts)
            .map(([filename, termCounts]) => {
                const { score, scoreComponents } = this.idfLogScore(filteredTerms, termCounts, idfDict)
                return {
                    filename,
                    score,
                    scoreComponents,
                }
            })
            .sort(({ score: score1 }, { score: score2 }) => score2 - score1)
        return filenamesWithScores
    }

    public getRootPath(): string | null {
        const docUri = vscode.window.activeTextEditor?.document.uri
        const wsFolderUri = docUri ? vscode.workspace.getWorkspaceFolder(docUri)?.uri.fsPath : null
        const wsRootUri = vscode.workspace.workspaceFolders?.[0]?.uri?.fsPath || null

        return wsFolderUri === undefined ? wsRootUri : wsFolderUri
    }

    private async nativeSymbolSearcher(term: string): Promise<vscode.SymbolInformation[]> {
        const command = 'vscode.executeWorkspaceSymbolProvider'
        return await vscode.commands.executeCommand<vscode.SymbolInformation[]>(command, term)
    }

    private idfLogScore(
        terms: string[],
        termCounts: { [term: string]: number },
        idfDict: { [term: string]: number }
    ): { score: number; scoreComponents: { [term: string]: number } } {
        let score = 0
        const scoreComponents: { [term: string]: number } = {}
        for (const term of terms) {
            const ct = termCounts[term] || 0
            // Assume terms with both upper and lower letters are symbols
            // as symbols should have higher priority than non-symbols
            const termScore = getTermScore(term)
            const logScore = ct === 0 ? 0 : Math.log10(termScore) + 1
            const idfLogScore = (idfDict[term] || 1) * logScore
            score += idfLogScore
            scoreComponents[term] = idfLogScore
        }
        return { score, scoreComponents }
    }

    private idf(termTotalFiles: { [term: string]: number }, totalFiles: number): { [term: string]: number } {
        const logTotal = Math.log(totalFiles)
        const e = Object.entries(termTotalFiles).map(([term, count]) => [term, logTotal - Math.log(count)])
        return Object.fromEntries(e)
    }

    private escapeRegex(s: string) {
        return s.replace(/[$()*+./?[\\\]^{|}\-]/g, '\\$&')
    }
}
