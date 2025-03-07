import { Observable, of } from 'rxjs'
import { map } from 'rxjs/operators'

import { createAggregateError } from '@sourcegraph/common'
import { dataOrThrowErrors, gql } from '@sourcegraph/http-client'

import { queryGraphQL } from '../../../../backend/graphql'
import {
    UseShowMorePaginationResult,
    useShowMorePagination,
} from '../../../../components/FilteredConnection/hooks/useShowMorePagination'
import {
    ProductLicensesResult,
    ProductLicenseFields,
    ProductLicensesVariables,
    ProductSubscriptionsDotComResult,
    ProductSubscriptionsDotComVariables,
    DotComProductLicensesResult,
    DotComProductLicensesVariables,
} from '../../../../graphql-operations'

const siteAdminProductSubscriptionFragment = gql`
    fragment SiteAdminProductSubscriptionFields on ProductSubscription {
        id
        name
        account {
            id
            username
            displayName
            emails {
                email
                isPrimary
            }
        }
        activeLicense {
            id
            info {
                productNameWithBrand
                tags
                userCount
                expiresAt
            }
            licenseKey
            createdAt
        }
        createdAt
        isArchived
        urlForSiteAdmin
    }
`

export const DOTCOM_PRODUCT_SUBSCRIPTION = gql`
    query DotComProductSubscription($uuid: String!) {
        dotcom {
            productSubscription(uuid: $uuid) {
                id
                name
                account {
                    ...DotComProductSubscriptionEmailFields
                }
                productLicenses {
                    nodes {
                        id
                        info {
                            tags
                            userCount
                            expiresAt
                        }
                        licenseKey
                        createdAt
                    }
                    totalCount
                    pageInfo {
                        hasNextPage
                    }
                }
                activeLicense {
                    id
                    info {
                        productNameWithBrand
                        tags
                        userCount
                        expiresAt
                    }
                    licenseKey
                    createdAt
                }
                currentSourcegraphAccessToken
                llmProxyAccess {
                    ...LLMProxyAccessFields
                }
                createdAt
                isArchived
                url
            }
        }
    }
    fragment LLMProxyAccessFields on LLMProxyAccess {
        enabled
        rateLimit {
            ...LLMProxyRateLimitFields
        }
    }

    fragment LLMProxyRateLimitFields on LLMProxyRateLimit {
        source
        limit
        intervalSeconds
    }
    fragment DotComProductSubscriptionEmailFields on User {
        id
        username
        displayName
        emails {
            email
            verified
        }
    }
`

export const ARCHIVE_PRODUCT_SUBSCRIPTION = gql`
    mutation ArchiveProductSubscription($id: ID!) {
        dotcom {
            archiveProductSubscription(id: $id) {
                alwaysNil
            }
        }
    }
`

export const UPDATE_LLM_PROXY_CONFIG = gql`
    mutation UpdateLLMProxyConfig($productSubscriptionID: ID!, $llmProxyAccess: UpdateLLMProxyAccessInput!) {
        dotcom {
            updateProductSubscription(id: $productSubscriptionID, update: { llmProxyAccess: $llmProxyAccess }) {
                alwaysNil
            }
        }
    }
`

export const GENERATE_PRODUCT_LICENSE = gql`
    mutation GenerateProductLicenseForSubscription($productSubscriptionID: ID!, $license: ProductLicenseInput!) {
        dotcom {
            generateProductLicenseForSubscription(productSubscriptionID: $productSubscriptionID, license: $license) {
                id
            }
        }
    }
`

const siteAdminProductLicenseFragment = gql`
    fragment ProductLicenseFields on ProductLicense {
        id
        subscription {
            id
            name
            account {
                ...ProductLicenseSubscriptionAccount
            }
            activeLicense {
                id
            }
            urlForSiteAdmin
        }
        licenseKey
        info {
            ...ProductLicenseInfoFields
        }
        createdAt
    }

    fragment ProductLicenseInfoFields on ProductLicenseInfo {
        productNameWithBrand
        tags
        userCount
        expiresAt
    }

    fragment ProductLicenseSubscriptionAccount on User {
        id
        username
        displayName
    }
`

export const PRODUCT_LICENSES = gql`
    query ProductLicenses($first: Int, $subscriptionUUID: String!) {
        dotcom {
            productSubscription(uuid: $subscriptionUUID) {
                productLicenses(first: $first) {
                    nodes {
                        ...ProductLicenseFields
                    }
                    totalCount
                    pageInfo {
                        hasNextPage
                    }
                }
            }
        }
    }
    ${siteAdminProductLicenseFragment}
`

export const useProductSubscriptionLicensesConnection = (
    subscriptionUUID: string,
    first: number
): UseShowMorePaginationResult<ProductLicensesResult, ProductLicenseFields> =>
    useShowMorePagination<ProductLicensesResult, ProductLicensesVariables, ProductLicenseFields>({
        query: PRODUCT_LICENSES,
        variables: {
            first: first ?? 20,
            after: null,
            subscriptionUUID,
        },
        getConnection: result => {
            const { dotcom } = dataOrThrowErrors(result)
            return dotcom.productSubscription.productLicenses
        },
        options: {
            fetchPolicy: 'cache-and-network',
        },
    })

export function queryProductSubscriptions(args: {
    first?: number
    query?: string
}): Observable<ProductSubscriptionsDotComResult['dotcom']['productSubscriptions']> {
    return queryGraphQL<ProductSubscriptionsDotComResult>(
        gql`
            query ProductSubscriptionsDotCom($first: Int, $account: ID, $query: String) {
                dotcom {
                    productSubscriptions(first: $first, account: $account, query: $query) {
                        nodes {
                            ...SiteAdminProductSubscriptionFields
                        }
                        totalCount
                        pageInfo {
                            hasNextPage
                        }
                    }
                }
            }
            ${siteAdminProductSubscriptionFragment}
        `,
        {
            first: args.first,
            query: args.query,
        } as Partial<ProductSubscriptionsDotComVariables>
    ).pipe(
        map(dataOrThrowErrors),
        map(data => data.dotcom.productSubscriptions)
    )
}

export function queryLicenses(args: {
    first?: number
    query?: string
}): Observable<DotComProductLicensesResult['dotcom']['productLicenses']> {
    const variables: Partial<DotComProductLicensesVariables> = {
        first: args.first,
        licenseKeySubstring: args.query,
    }
    return args.query
        ? queryGraphQL<DotComProductLicensesResult>(
              gql`
                  query DotComProductLicenses($first: Int, $licenseKeySubstring: String) {
                      dotcom {
                          productLicenses(first: $first, licenseKeySubstring: $licenseKeySubstring) {
                              nodes {
                                  ...ProductLicenseFields
                              }
                              totalCount
                              pageInfo {
                                  hasNextPage
                              }
                          }
                      }
                  }
                  ${siteAdminProductLicenseFragment}
              `,
              variables
          ).pipe(
              map(({ data, errors }) => {
                  if (!data?.dotcom?.productLicenses || (errors && errors.length > 0)) {
                      throw createAggregateError(errors)
                  }
                  return data.dotcom.productLicenses
              })
          )
        : of({
              __typename: 'ProductLicenseConnection' as const,
              nodes: [],
              totalCount: 0,
              pageInfo: { __typename: 'PageInfo' as const, hasNextPage: false, endCursor: null },
          })
}
