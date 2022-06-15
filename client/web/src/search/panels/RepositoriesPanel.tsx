import React, { useCallback, useEffect, useState, useMemo } from 'react'

import { gql } from '@apollo/client'
import classNames from 'classnames'
import { of } from 'rxjs'

import { SyntaxHighlightedSearchQuery } from '@sourcegraph/search-ui'
import { scanSearchQuery } from '@sourcegraph/shared/src/search/query/scanner'
import { isRepoFilter } from '@sourcegraph/shared/src/search/query/validate'
import { TelemetryProps } from '@sourcegraph/shared/src/telemetry/telemetryService'
import { Link, Text, useObservable } from '@sourcegraph/wildcard'

import { parseSearchURLQuery } from '..'
import { streamComputeQuery } from '../../../../shared/src/search/stream'
import { AuthenticatedUser } from '../../auth'
import { RecentlySearchedRepositoriesFragment } from '../../graphql-operations'
import { EventLogResult } from '../backend'

import { EmptyPanelContainer } from './EmptyPanelContainer'
import { HomePanelsFetchMore, RECENTLY_SEARCHED_REPOSITORIES_TO_LOAD } from './HomePanels'
import { LoadingPanelView } from './LoadingPanelView'
import { PanelContainer } from './PanelContainer'

interface Props extends TelemetryProps {
    className?: string
    authenticatedUser: AuthenticatedUser | null
    recentlySearchedRepositories: RecentlySearchedRepositoriesFragment | null
    fetchMore: HomePanelsFetchMore
}

export const recentlySearchedRepositoriesFragment = gql`
    fragment RecentlySearchedRepositoriesFragment on User {
        recentlySearchedRepositoriesLogs: eventLogs(
            first: $firstRecentlySearchedRepositories
            eventName: "SearchResultsQueried"
        ) {
            nodes {
                argument
                timestamp
                url
            }
            pageInfo {
                hasNextPage
            }
            totalCount
        }
    }
`

type ComputeParseResult = [{ kind: string; value: string }]

export const RepositoriesPanel: React.FunctionComponent<React.PropsWithChildren<Props>> = ({
    className,
    telemetryService,
    recentlySearchedRepositories,
    authenticatedUser,
}) => {
    const [searchEventLogs, setSearchEventLogs] = useState<
        null | RecentlySearchedRepositoriesFragment['recentlySearchedRepositoriesLogs']
    >(recentlySearchedRepositories?.recentlySearchedRepositoriesLogs ?? null)
    useEffect(() => setSearchEventLogs(recentlySearchedRepositories?.recentlySearchedRepositoriesLogs ?? null), [
        recentlySearchedRepositories?.recentlySearchedRepositoriesLogs,
    ])

    const [itemsToLoad] = useState(RECENTLY_SEARCHED_REPOSITORIES_TO_LOAD)

    const logRepoClicked = useCallback(() => telemetryService.log('RepositoriesPanelRepoFilterClicked'), [
        telemetryService,
    ])

    const loadingDisplay = <LoadingPanelView text="Loading recently searched repositories" />

    const emptyDisplay = (
        <EmptyPanelContainer className="text-muted">
            <small className="mb-2">
                <Text className="mb-1">Recently searched repositories will be displayed here.</Text>
                <Text className="mb-1">
                    Search in repositories with the <strong>repo:</strong> filter:
                </Text>
                <Text className="mb-1">
                    <SyntaxHighlightedSearchQuery query="repo:sourcegraph/sourcegraph" />
                </Text>
                <Text className="mb-1">Add the code host to scope to a single repository:</Text>
                <Text className="mb-1">
                    <SyntaxHighlightedSearchQuery query="repo:^git\.local/my/repo$" />
                </Text>
            </small>
        </EmptyPanelContainer>
    )

    const [repoFilterValues, setRepoFilterValues] = useState<string[] | null>(null)

    useEffect(() => {
        if (searchEventLogs) {
            const recentlySearchedRepos = processRepositories(searchEventLogs)
            setRepoFilterValues(recentlySearchedRepos)
        }
    }, [searchEventLogs])

    useEffect(() => {
        // Only log the first load (when items to load is equal to the page size)
        if (repoFilterValues && itemsToLoad === RECENTLY_SEARCHED_REPOSITORIES_TO_LOAD) {
            telemetryService.log(
                'RepositoriesPanelLoaded',
                { empty: repoFilterValues.length === 0 },
                { empty: repoFilterValues.length === 0 }
            )
        }
    }, [repoFilterValues, telemetryService, itemsToLoad])

    const contentDisplay = (
        <div className="mt-2">
            <div className="d-flex mb-1">
                <small>Search</small>
            </div>
            {repoFilterValues?.length && (
                <ul className="list-group">
                    {repoFilterValues.map((repoFilterValue, index) => (
                        // eslint-disable-next-line react/no-array-index-key
                        <li key={`${repoFilterValue}-${index}`} className="text-monospace text-break mb-2">
                            <small>
                                <Link to={`/search?q=repo:${repoFilterValue}`} onClick={logRepoClicked}>
                                    <SyntaxHighlightedSearchQuery query={`repo:${repoFilterValue}`} />
                                </Link>
                            </small>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    )

    // a constant to hold git commits history for the repo filter
    // call streamComputeQuery from stream

    const gitRepository = useObservable(
        useMemo(
            () =>
                authenticatedUser
                    ? streamComputeQuery(
                          `content:output((.|\n)* -> $repo) author:${authenticatedUser.email} type:commit after:"1 year ago" count:all`
                      )
                    : of([]),
            [authenticatedUser]
        )
    )

    const gitSet = useMemo(() => {
        let gitRepositoryParsedString: ComputeParseResult[] = []
        if (gitRepository) {
            gitRepositoryParsedString = gitRepository.map(value => JSON.parse(value) as ComputeParseResult)
        }
        const gitReposList = gitRepositoryParsedString?.flat()

        const gitSet = new Set<string>()
        if (gitReposList) {
            for (const git of gitReposList) {
                if (git.value) {
                    gitSet.add(git.value)
                }
            }
        }

        return gitSet
    }, [gitRepository])

    // A new display for git history
    const gitHistoryDisplay = (
        <div className="mt-2">
            <div className="d-flex mb-1">
                <small>Git history</small>
            </div>
            {gitSet.size > 0 && (
                <ul className="list-group">
                    {Array.from(gitSet).map(repo => (
                        <li key={`${repo}`} className="text-monospace text-break mb-2">
                            <small>
                                <Link to={`/search?q=repo:${repo}`} onClick={logRepoClicked}>
                                    <SyntaxHighlightedSearchQuery query={`repo:${repo}`} />
                                </Link>
                            </small>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    )

    // Wait for both the search event logs and the git history to be loaded
    const isLoading = !gitRepository || !repoFilterValues
    // If neither search event logs or git history have items, then display the empty display
    const isEmpty = repoFilterValues?.length === 0 && gitSet.size === 0

    return (
        <PanelContainer
            className={classNames(className, 'repositories-panel')}
            title="Repositories"
            state={isLoading ? 'loading' : isEmpty ? 'empty' : 'populated'}
            loadingContent={loadingDisplay}
            populatedContent={gitSet.size > 0 ? gitHistoryDisplay : contentDisplay}
            emptyContent={emptyDisplay}
        />
    )
}

function processRepositories(eventLogResult: EventLogResult): string[] | null {
    if (!eventLogResult) {
        return null
    }

    const recentlySearchedRepos: string[] = []

    for (const node of eventLogResult.nodes) {
        if (node.url) {
            const url = new URL(node.url)
            const queryFromURL = parseSearchURLQuery(url.search)
            const scannedQuery = scanSearchQuery(queryFromURL || '')
            if (scannedQuery.type === 'success') {
                for (const token of scannedQuery.term) {
                    if (isRepoFilter(token) && token.value && !recentlySearchedRepos.includes(token.value.value)) {
                        recentlySearchedRepos.push(token.value.value)
                    }
                }
            }
        }
    }
    return recentlySearchedRepos
}
