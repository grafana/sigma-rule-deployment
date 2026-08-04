/**
 * GraphQL queries and mutations used to manage PR comments.
 */

export const oldCommentQuery = `query GetPRComments($owner: String!, $name: String!, $number: Int!) {
        repository(owner: $owner, name: $name) {
          id
          pullRequest(number: $number) {
            title
            comments(last: 100) {
              nodes {
                id
                body
                bodyText
                isMinimized
                author {
                  login
                }
              }
            }
          }
        }
    }`;

export const minimizeCommentMutation = `mutation MinimizeComment($subjectId: ID!) {
      minimizeComment(input: {
        subjectId: $subjectId,
        classifier: OUTDATED
      }) {
        clientMutationId
      }
    }`;

export const addCommentMutation = `mutation AddComment($body: String!, $subjectId: ID!) {
      addComment(input: {
        body: $body,
        subjectId: $subjectId,
      }) {
        subject {
          id
          ... on PullRequest {
            number
          }
        }
      }
    }`;
